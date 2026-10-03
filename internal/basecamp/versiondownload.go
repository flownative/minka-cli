package basecamp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// versionAPIHost is the one host the token of the Basecamp CLI is sent to
	// by this program: the API Basecamp signs it for.
	versionAPIHost = "3.basecampapi.com"
	// signedStorageHost is the one host the address of an earlier file may
	// lead to. It takes no token: the address carries its own signature.
	signedStorageHost = "storage.basecamp.com"
	// versionAskTimeout is how long the API may take to say where an earlier
	// file lies.
	versionAskTimeout = 60 * time.Second
	// maxTokenOutput is what the Basecamp CLI may print when it hands out its
	// token; anything larger is no token.
	maxTokenOutput = 64 << 10
)

// signedPathPattern is the path of the signed address an earlier file lies at:
// a bucket of the store and the key of the file in it, neither of which is dots
// alone, which a store reads as a way up and out of its bucket.
var signedPathPattern = regexp.MustCompile(`^/` + signedSegment + `/` + signedSegment + `$`)

const signedSegment = `[A-Za-z0-9._-]*[A-Za-z0-9_-][A-Za-z0-9._-]*`

// versionAddressPattern is the address of an earlier file of an upload on the
// API, built of the account, the project, the upload, the version and one
// segment for the name.
func versionAddressPattern(host string) *regexp.Regexp {
	return regexp.MustCompile(`^https://` + regexp.QuoteMeta(host) +
		`/[0-9]+/buckets/[0-9]+/uploads/[0-9]+/versions/[0-9]+/download/(` + nameSegmentCharacters + `)$`)
}

// checkVersionAddress refuses whatever is no address of an earlier file on the
// API host: the token travels with the request, so the check stands right
// before it, whoever built the address. A refusal never quotes the address,
// which names the file.
func checkVersionAddress(address, host string) error {
	refused := &Error{Kind: API, Message: "an earlier file is named at an address that is no address of a version on Basecamp's API"}
	match := versionAddressPattern(host).FindStringSubmatch(address)
	if match == nil || !aSingleSegment(match[1]) {
		return refused
	}
	return nil
}

// aSingleSegment tells that an escaped name stays one segment of a path once the
// escapes are taken away: a name that holds a slash then is a path of its own,
// which leads out of the file and as far as another account.
func aSingleSegment(escaped string) bool {
	name, err := url.PathUnescape(escaped)
	return err == nil && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

// checkSignedAddress refuses a place an earlier file is said to lie at that is
// not the signed store of Basecamp: https, that one host, no sign-in in the
// address and a path of a bucket and a key.
func checkSignedAddress(address *url.URL, host string) error {
	if address.Scheme != "https" || address.Host != host || address.User != nil || address.Fragment != "" ||
		!signedPathPattern.MatchString(address.EscapedPath()) {
		return &Error{Kind: API, Message: "Basecamp says an earlier file lies at a place that is not its signed store"}
	}
	return nil
}

// DownloadVersion writes an earlier file of an upload into a writer, asked for
// at the address of that version on the API.
//
// **No call of the Basecamp CLI brings such a file.** `files download` takes a
// version's address and serves the file the upload shows now, ending well, and
// `api get` follows the way to the file and cannot hand out its bytes. So this
// takes the two steps itself: the API, asked with the token of the Basecamp
// CLI and following nothing, says where the file lies, and the signed store is
// asked without any token. The token goes to the API host alone, and the place
// the API names is held to the signed store before it is asked.
//
// It answers the way Download does, a file the source no longer has as
// NotFound, and is not asked again within a run.
func (d Downloader) DownloadVersion(ctx context.Context, address string, into io.Writer) error {
	if ctx.Err() != nil {
		return &Error{Kind: Interrupted, Message: "interrupted"}
	}
	if err := checkVersionAddress(address, d.apiHostForVersions()); err != nil {
		return err
	}
	token, err := d.token(ctx)
	if err != nil {
		return err
	}
	signed, err := d.whereTheVersionLies(ctx, address, token)
	if err != nil {
		return err
	}
	return d.fetchSigned(ctx, signed, into)
}

func (d Downloader) apiHostForVersions() string {
	if d.versionAPIHost != "" {
		return d.versionAPIHost
	}
	return versionAPIHost
}

func (d Downloader) storageHostForVersions() string {
	if d.signedStorageHost != "" {
		return d.signedStorageHost
	}
	return signedStorageHost
}

// roundTrip asks one request and follows nothing: no redirect, no cookie. An
// answer that points elsewhere is read here, never acted on by the transport.
func (d Downloader) roundTrip(request *http.Request) (*http.Response, error) {
	transport := d.transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(request)
}

// token asks the Basecamp CLI for the token it is signed in with, which it
// renews where it is about to run out. It is asked anew for every file and
// kept nowhere, and no message ever carries it or anything else the Basecamp
// CLI printed: what it says around a failure may be the token itself.
func (d Downloader) token(ctx context.Context) (string, error) {
	binary := d.Binary
	if binary == "" {
		binary = "basecamp"
	}
	cmd := exec.CommandContext(ctx, binary, "auth", "token", "--agent")
	cmd.Env = append(os.Environ(), "BASECAMP_NONINTERACTIVE=1")
	cmd.WaitDelay = childWaitDelay
	stdout := limitedBuffer{limit: maxTokenOutput}
	cmd.Stdout = &stdout

	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return "", &Error{Kind: Interrupted, Message: "interrupted"}
	case err == nil:
	case errors.As(err, &exitErr) && exitErr.ExitCode() == -1:
		return "", endedBySignal(exitErr)
	case errors.As(err, &exitErr):
		return "", tokenRefused(exitErr.ExitCode(), stdout.content)
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return "", &Error{Kind: NotInstalled, Message: "the Basecamp CLI was not found: " + err.Error()}
	default:
		return "", &Error{Kind: API, Message: "the Basecamp CLI could not be run: " + err.Error()}
	}

	var answer struct {
		Token string `json:"token"`
	}
	if stdout.over || json.Unmarshal(stdout.content, &answer) != nil || answer.Token == "" ||
		strings.ContainsFunc(answer.Token, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return "", &Error{Kind: API, Message: "the Basecamp CLI handed out no token that can be sent"}
	}
	return answer.Token, nil
}

// tokenFailures is what a reason says per class of a failure to hand out the
// token. Only these classes are kept: the call asks nothing about the file, so
// a "not found" or a "forbidden" of it must never pass for Basecamp's word on
// one, and is a failure nobody can place.
var tokenFailures = map[Kind]string{
	AuthRequired: "the Basecamp CLI is not signed in",
	Network:      "the network failed while the Basecamp CLI renewed its sign-in",
	Unavailable:  "Basecamp failed while the Basecamp CLI renewed its sign-in",
}

// tokenRefused reads why the Basecamp CLI handed out no token, by the class of
// its failure alone.
func tokenRefused(exitCode int, stdout []byte) *Error {
	kind := failure(exitCode, stdout, nil).Kind
	if reason, kept := tokenFailures[kind]; kept {
		return &Error{Kind: kind, Message: reason}
	}
	return &Error{Kind: API, Message: fmt.Sprintf("the Basecamp CLI handed out no token and ended with exit code %d", exitCode)}
}

// limitedBuffer keeps what a program prints up to a limit and says whether
// there was more.
type limitedBuffer struct {
	content []byte
	limit   int
	over    bool
}

func (b *limitedBuffer) Write(content []byte) (int, error) {
	room := b.limit - len(b.content)
	if len(content) > room {
		b.over = true
		content = content[:max(room, 0)]
	}
	b.content = append(b.content, content...)
	return len(content), nil
}

// statusKinds are the classes of an answer of the API that is no way to the
// file. A refusal of the sign-in ends the run as it does for every request.
var statusKinds = map[int]Kind{
	http.StatusUnauthorized:    AuthRequired,
	http.StatusForbidden:       Forbidden,
	http.StatusNotFound:        NotFound,
	http.StatusTooManyRequests: RateLimited,
	http.StatusRequestTimeout:  Unavailable,
}

// versionFailures is what a reason says per class. It never says what came
// back: an answer may quote the address, which names the file.
var versionFailures = map[Kind]string{
	AuthRequired: "Basecamp refuses the sign-in of the Basecamp CLI",
	Forbidden:    "Basecamp forbids the download of the earlier file",
	NotFound:     "Basecamp no longer has the earlier file",
	RateLimited:  "Basecamp limits the requests of the account",
	Unavailable:  "Basecamp, or a gateway in front of it, failed",
	Network:      "the network failed while asking where the earlier file lies",
}

// whereTheVersionLies asks the API, with the token, where an earlier file lies,
// and follows nothing: the answer is a redirect, and the place it names is held
// to the signed store before anybody asks it.
func (d Downloader) whereTheVersionLies(ctx context.Context, address, token string) (*url.URL, error) {
	asking, end := context.WithTimeout(ctx, versionAskTimeout)
	defer end()
	request, err := http.NewRequestWithContext(asking, http.MethodGet, address, nil)
	if err != nil {
		return nil, &Error{Kind: API, Message: "the address of an earlier file cannot be asked for"}
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", "minka")

	response, err := d.roundTrip(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, &Error{Kind: Interrupted, Message: "interrupted"}
		}
		return nil, &Error{Kind: Network, Message: versionFailures[Network]}
	}
	_ = response.Body.Close()

	switch status := response.StatusCode; {
	case status == http.StatusFound || status == http.StatusMovedPermanently || status == http.StatusSeeOther ||
		status == http.StatusTemporaryRedirect || status == http.StatusPermanentRedirect:
		location, err := url.Parse(response.Header.Get("Location"))
		if err != nil || response.Header.Get("Location") == "" {
			return nil, &Error{Kind: API, Message: "Basecamp redirects an earlier file to no place that can be read"}
		}
		if err := checkSignedAddress(location, d.storageHostForVersions()); err != nil {
			return nil, err
		}
		return location, nil
	default:
		kind, known := statusKinds[status]
		if !known && status >= 500 && status <= 599 {
			kind, known = Unavailable, true
		}
		if !known {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered HTTP %d where it says where an earlier file lies", status)}
		}
		return nil, &Error{Kind: kind, Message: versionFailures[kind]}
	}
}

// fetchSigned streams an earlier file from the signed store: no token, no
// redirect followed, and the limits of time every download has.
func (d Downloader) fetchSigned(ctx context.Context, address *url.URL, into io.Writer) error {
	idleTimeout, timeLimit := d.IdleTimeout, d.TimeLimit
	if idleTimeout == 0 {
		idleTimeout = fileIdleTimeout
	}
	if timeLimit == 0 {
		timeLimit = fileTimeLimit
	}
	running, end := context.WithCancel(ctx)
	defer end()
	var givenUp atomic.Pointer[string]
	giveUp := func(reason string) func() {
		return func() {
			givenUp.CompareAndSwap(nil, &reason)
			end()
		}
	}
	idle := time.AfterFunc(idleTimeout, giveUp(fmt.Sprintf("the download brought no byte for %s and was given up", idleTimeout)))
	defer idle.Stop()
	limit := time.AfterFunc(timeLimit, giveUp(fmt.Sprintf("the download did not end within %s and was given up", timeLimit)))
	defer limit.Stop()

	request, err := http.NewRequestWithContext(running, http.MethodGet, address.String(), nil)
	if err != nil {
		return &Error{Kind: API, Message: "the signed address of an earlier file cannot be asked for"}
	}
	request.Header.Set("User-Agent", "minka")

	interrupted := func(err error) error {
		switch {
		case ctx.Err() != nil:
			return &Error{Kind: Interrupted, Message: "interrupted"}
		case givenUp.Load() != nil:
			return &Error{Kind: Network, Message: *givenUp.Load()}
		default:
			return err
		}
	}
	response, err := d.roundTrip(request)
	if err != nil {
		return interrupted(&Error{Kind: Network, Message: "the network failed during the download"})
	}
	defer response.Body.Close()
	switch status := response.StatusCode; {
	case status == http.StatusOK:
	case status >= 500 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests:
		return &Error{Kind: Unavailable, Message: fmt.Sprintf("the signed store of Basecamp answered HTTP %d", status)}
	default:
		// The signature is Basecamp's own and fresh, so a refusal of it says
		// nothing this run can place about the file.
		return &Error{Kind: API, Message: fmt.Sprintf("the signed store of Basecamp answered HTTP %d", status)}
	}

	written := &watchedOutput{into: into, alive: func() { idle.Reset(idleTimeout) }}
	if _, err := io.Copy(written, response.Body); err != nil {
		if written.failed != nil {
			return written.failed
		}
		return interrupted(&Error{Kind: Network, Message: "the network failed during the download"})
	}
	return nil
}
