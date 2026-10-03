package basecamp

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	standInToken = "secret-token-4711"
	// fakeSecret is what a Basecamp CLI that fails prints around its failure,
	// which may be the token itself.
	fakeSecret     = "FAKE-SECRET-0815"
	earlierContent = "the bytes of the earlier file"
	currentContent = "the bytes of the file the upload shows now"
	versionPath    = "/4242/buckets/1/uploads/2/versions/3/download/x%20y.pdf"
	uploadPath     = "/4242/buckets/1/uploads/2/download/x%20y.pdf"
	signedPath     = "/bc4-test-activestorage/AbC123xyz"
)

// tokenStandIn takes the place of the Basecamp CLI: it hands out its token the
// way the real one does, as the mode of the test says, and streams the file the
// upload shows now for any download, which is what the real one brings for the
// address of a version. It keeps a line per call next to itself.
func tokenStandIn(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	path := filepath.Join(t.TempDir(), "basecamp")
	script := `#!/bin/sh
echo "$*" >> "$0.calls"
case "$1 $2" in
	"auth token")
		case "$STANDIN_TOKEN" in
			signed-out) printf '{"code": "auth_required", "error": "Not authenticated", "ok": false, "retryable": false}'; exit 3 ;;
			not-found) echo "` + fakeSecret + ` on stderr" >&2; printf '{"code": "not_found", "error": "` + fakeSecret + `", "ok": false}'; exit 2 ;;
			forbidden) echo "` + fakeSecret + ` on stderr" >&2; printf '{"code": "forbidden", "error": "` + fakeSecret + `", "ok": false}'; exit 4 ;;
			offline) echo "` + fakeSecret + ` on stderr" >&2; printf '{"code": "network", "error": "` + fakeSecret + `", "ok": false, "retryable": true}'; exit 6 ;;
			leaking) echo "` + fakeSecret + ` on stderr" >&2; printf '` + fakeSecret + ` is no JSON'; exit 1 ;;
			spaced) printf '{"token": "two words"}' ;;
			none) printf 'no answer of any shape' ;;
			*) printf '{"token": "` + standInToken + `"}' ;;
		esac ;;
	"files download") printf '` + currentContent + `' ;;
	*) exit 64 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// callsOf are the calls a stand-in was made, none when it never ran.
func callsOf(t *testing.T, standIn string) []string {
	t.Helper()
	content, err := os.ReadFile(standIn + ".calls")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

// versionBasecamp stands in for the API and the signed store of Basecamp. The
// API answers the address of a version, asked with the token, with the way to
// the earlier file in the store, and the upload's own address with the file it
// shows now; the store hands out the earlier file. What each was asked is kept.
type versionBasecamp struct {
	api, store *httptest.Server

	mu         sync.Mutex
	apiAsked   []string
	apiAuth    []string
	storeAsked []string
	storeAuth  []string

	// location replaces where the API says the earlier file lies, apiStatus and
	// storeStatus what each answers instead.
	location    func(store string) string
	apiStatus   int
	storeStatus int
	storeServe  func(w http.ResponseWriter, r *http.Request)
}

func newVersionBasecamp(t *testing.T) *versionBasecamp {
	t.Helper()
	b := &versionBasecamp{}
	b.store = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.storeAsked = append(b.storeAsked, r.URL.RequestURI())
		b.storeAuth = append(b.storeAuth, r.Header.Get("Authorization"))
		b.mu.Unlock()
		switch {
		case b.storeServe != nil:
			b.storeServe(w, r)
		case b.storeStatus != 0:
			w.WriteHeader(b.storeStatus)
		case r.URL.Path == signedPath:
			_, _ = w.Write([]byte(earlierContent))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(b.store.Close)
	b.api = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.apiAsked = append(b.apiAsked, r.URL.EscapedPath())
		b.apiAuth = append(b.apiAuth, r.Header.Get("Authorization"))
		b.mu.Unlock()
		switch {
		case r.Header.Get("Authorization") != "Bearer "+standInToken:
			w.WriteHeader(http.StatusUnauthorized)
		case b.apiStatus != 0:
			w.WriteHeader(b.apiStatus)
		case r.URL.EscapedPath() == versionPath:
			location := b.store.URL + signedPath + "?X-Amz-Signature=abc&response-content-disposition=attachment"
			if b.location != nil {
				location = b.location(b.store.URL)
			}
			if location != "" {
				w.Header().Set("Location", location)
			}
			w.WriteHeader(http.StatusFound)
		case r.URL.EscapedPath() == uploadPath:
			_, _ = w.Write([]byte(currentContent))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(b.api.Close)
	return b
}

func (b *versionBasecamp) downloader(standIn string) Downloader {
	return Downloader{
		Binary:            standIn,
		versionAPIHost:    strings.TrimPrefix(b.api.URL, "https://"),
		signedStorageHost: strings.TrimPrefix(b.store.URL, "https://"),
		transport:         b.api.Client().Transport,
	}
}

// **The earlier file comes from the place the version names, and from no route
// that brings the file the upload shows now.** The Basecamp CLI is asked for its
// token and for nothing else: every download of it brings the current file for
// the address of a version and ends well. The token goes to the API host and the
// store is asked without it.
func TestAnEarlierFileComesFromTheSignedStoreItsVersionNames(t *testing.T) {
	standIn := tokenStandIn(t)
	b := newVersionBasecamp(t)
	var into bytes.Buffer
	if err := b.downloader(standIn).DownloadVersion(t.Context(), b.api.URL+versionPath, &into); err != nil {
		t.Fatal(err)
	}

	if into.String() != earlierContent {
		t.Errorf("the download brought %q, want the earlier file", into.String())
	}
	if calls := callsOf(t, standIn); len(calls) != 1 || calls[0] != "auth token --agent" {
		t.Errorf("the Basecamp CLI was called as %q, want once for its token", calls)
	}

	// The token is asked for anew for every file and kept nowhere.
	into.Reset()
	if err := b.downloader(standIn).DownloadVersion(t.Context(), b.api.URL+versionPath, &into); err != nil {
		t.Fatal(err)
	}
	if calls := callsOf(t, standIn); len(calls) != 2 || calls[1] != "auth token --agent" {
		t.Errorf("for two files the Basecamp CLI was called as %q, want once per file for its token", calls)
	}
	b.apiAsked, b.apiAuth, b.storeAsked, b.storeAuth = b.apiAsked[:1], b.apiAuth[:1], b.storeAsked[:1], b.storeAuth[:1]
	if len(b.apiAsked) != 1 || b.apiAsked[0] != versionPath || b.apiAuth[0] != "Bearer "+standInToken {
		t.Errorf("the API was asked %v with %v, want the version's address with the token", b.apiAsked, b.apiAuth)
	}
	if len(b.storeAsked) != 1 || b.storeAuth[0] != "" {
		t.Errorf("the store was asked %v with %q, want once and without any token", b.storeAsked, b.storeAuth)
	}
}

// The place the API names is held to the signed store before anybody asks it:
// https, that one host, no sign-in in the address and a path of a bucket and a
// key. Nothing else is asked, the token least of all.
func TestThePlaceAnEarlierFileLiesAtIsHeldToTheSignedStore(t *testing.T) {
	for what, location := range map[string]func(store string) string{
		"no place":              func(string) string { return "" },
		"a place without https": func(store string) string { return strings.Replace(store, "https://", "http://", 1) + signedPath },
		"another host":          func(string) string { return "https://storage.example.test" + signedPath },
		"a sign-in in it": func(store string) string {
			return strings.Replace(store, "https://", "https://user:pass@", 1) + signedPath
		},
		"a deeper path":      func(store string) string { return store + "/bc4/a/b" },
		"a path of one part": func(store string) string { return store + "/key" },
		"a fragment":         func(store string) string { return store + signedPath + "#part" },
		"a bucket of dots":   func(store string) string { return store + "/../AbC123xyz" },
		"a key of dots":      func(store string) string { return store + "/bc4-test-activestorage/.." },
		"one dot":            func(store string) string { return store + "/./AbC123xyz" },
		"a path of the API":  func(string) string { return uploadPath },
		"no address at all":  func(string) string { return "%zz" },
	} {
		standIn := tokenStandIn(t)
		b := newVersionBasecamp(t)
		b.location = location
		var into bytes.Buffer
		err := b.downloader(standIn).DownloadVersion(t.Context(), b.api.URL+versionPath, &into)

		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != API {
			t.Errorf("%s: the download ended with %v, want a failure nobody can place", what, err)
		}
		if len(b.storeAsked) != 0 || into.Len() != 0 || len(b.apiAsked) != 1 {
			t.Errorf("%s: the store was asked %v and the API %v, and %d bytes came", what, b.storeAsked, b.apiAsked, into.Len())
		}
	}
}

// A place that would hand out the file all the same is not asked where it is
// not the signed store exactly: a store over http, which carries the address in
// the clear, and one on the same machine at another port, which is another
// server.
func TestAStoreOverHTTPOrAtAnotherPortIsNeverAsked(t *testing.T) {
	var asked []string
	serving := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		_, _ = w.Write([]byte(earlierContent))
	})
	plain := httptest.NewServer(serving)
	t.Cleanup(plain.Close)
	otherPort := httptest.NewTLSServer(serving)
	t.Cleanup(otherPort.Close)

	for what, place := range map[string]func(b *versionBasecamp) (host, location string){
		"over http": func(*versionBasecamp) (string, string) {
			host := strings.TrimPrefix(plain.URL, "http://")
			return host, "http://" + host + signedPath
		},
		"at another port": func(b *versionBasecamp) (string, string) {
			return strings.TrimPrefix(b.store.URL, "https://"), otherPort.URL + signedPath
		},
	} {
		asked = nil
		standIn := tokenStandIn(t)
		b := newVersionBasecamp(t)
		downloader := b.downloader(standIn)
		host, location := place(b)
		downloader.signedStorageHost = host
		b.location = func(string) string { return location }
		var into bytes.Buffer
		err := downloader.DownloadVersion(t.Context(), b.api.URL+versionPath, &into)

		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != API {
			t.Errorf("%s: the download ended with %v, want a failure nobody can place", what, err)
		}
		if len(asked) != 0 || into.Len() != 0 {
			t.Errorf("%s: the place was asked %v and %d bytes came", what, asked, into.Len())
		}
	}
}

// The store is asked once and its redirect is never followed: the signed place is
// the one destination the API named.
func TestTheSignedStoreIsFollowedNowhere(t *testing.T) {
	standIn := tokenStandIn(t)
	b := newVersionBasecamp(t)
	b.storeServe = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, b.api.URL+uploadPath, http.StatusFound)
	}
	var into bytes.Buffer
	err := b.downloader(standIn).DownloadVersion(t.Context(), b.api.URL+versionPath, &into)

	var failed *Error
	if !errors.As(err, &failed) || failed.Kind != API {
		t.Errorf("a redirect of the store ended the download with %v, want a failure nobody can place", err)
	}
	if len(b.apiAsked) != 1 || into.Len() != 0 {
		t.Errorf("the API was asked %v after the store redirected, and %d bytes came", b.apiAsked, into.Len())
	}
}

// Only the address of a version on the API host gets the token, checked right
// before it is asked for: an address of any other shape is refused before the
// Basecamp CLI is asked for anything and before any request goes out.
func TestAnAddressThatIsNoVersionsOwnGetsNoToken(t *testing.T) {
	b := newVersionBasecamp(t)
	for what, address := range map[string]string{
		"the upload's own address":         b.api.URL + uploadPath,
		"another host":                     "https://3.basecampapi.example" + versionPath,
		"no https":                         strings.Replace(b.api.URL, "https://", "http://", 1) + versionPath,
		"a name that is a path of its own": b.api.URL + "/4242/buckets/1/uploads/2/versions/3/download/..%2F..%2Fx",
		"a name that is the parent":        b.api.URL + "/4242/buckets/1/uploads/2/versions/3/download/..",
		"a query":                          b.api.URL + versionPath + "?x=1",
		"a version of letters":             b.api.URL + "/4242/buckets/1/uploads/2/versions/v3/download/x.pdf",
		"a sign-in in it":                  strings.Replace(b.api.URL, "https://", "https://user@", 1) + versionPath,
	} {
		standIn := tokenStandIn(t)
		var into bytes.Buffer
		err := b.downloader(standIn).DownloadVersion(t.Context(), address, &into)

		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != API {
			t.Errorf("%s: the download ended with %v, want a refusal", what, err)
		}
		if strings.Contains(err.Error(), "x.pdf") || strings.Contains(err.Error(), "x%20y") {
			t.Errorf("%s: the refusal quotes the address: %v", what, err)
		}
		if calls := callsOf(t, standIn); len(calls) != 0 {
			t.Errorf("%s: the Basecamp CLI was called as %q", what, calls)
		}
	}
	if len(b.apiAsked) != 0 || len(b.storeAsked) != 0 {
		t.Errorf("the API was asked %v and the store %v", b.apiAsked, b.storeAsked)
	}
}

// What the API answers in place of the way to the file keeps its class, as the
// answers of every request do, and says nothing of the file or the token.
func TestWhatTheAPIAnswersInPlaceOfTheWayToAnEarlierFileKeepsItsClass(t *testing.T) {
	for status, want := range map[int]Kind{
		http.StatusUnauthorized:        AuthRequired,
		http.StatusForbidden:           Forbidden,
		http.StatusNotFound:            NotFound,
		http.StatusTooManyRequests:     RateLimited,
		http.StatusRequestTimeout:      Unavailable,
		http.StatusInternalServerError: Unavailable,
		http.StatusBadGateway:          Unavailable,
		http.StatusOK:                  API,
		http.StatusTeapot:              API,
	} {
		standIn := tokenStandIn(t)
		b := newVersionBasecamp(t)
		b.apiStatus = status
		var into bytes.Buffer
		err := b.downloader(standIn).DownloadVersion(t.Context(), b.api.URL+versionPath, &into)

		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != want {
			t.Errorf("HTTP %d ended the download with %v, want %s", status, err, want)
			continue
		}
		if strings.Contains(failed.Message, standInToken) || strings.Contains(failed.Message, "x%20y") || strings.Contains(failed.Message, "x y") {
			t.Errorf("HTTP %d says %q", status, failed.Message)
		}
		if len(b.storeAsked) != 0 || into.Len() != 0 {
			t.Errorf("HTTP %d: the store was asked %v and %d bytes came", status, b.storeAsked, into.Len())
		}
	}
}

// A failure of the signed store may pass where the store failed, and is one
// nobody can place otherwise: the signature is Basecamp's own and fresh, so a
// refusal of it says nothing about the file.
func TestWhatTheSignedStoreAnswersInPlaceOfTheFileKeepsItsClass(t *testing.T) {
	for status, want := range map[int]Kind{
		http.StatusInternalServerError: Unavailable,
		http.StatusServiceUnavailable:  Unavailable,
		http.StatusTooManyRequests:     Unavailable,
		http.StatusForbidden:           API,
		http.StatusNotFound:            API,
	} {
		standIn := tokenStandIn(t)
		b := newVersionBasecamp(t)
		b.storeStatus = status
		var into bytes.Buffer
		err := b.downloader(standIn).DownloadVersion(t.Context(), b.api.URL+versionPath, &into)

		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != want {
			t.Errorf("HTTP %d of the store ended the download with %v, want %s", status, err, want)
		}
		if into.Len() != 0 {
			t.Errorf("HTTP %d of the store brought %d bytes", status, into.Len())
		}
	}
}

// A token the Basecamp CLI does not hand out ends the download with its failure:
// a Basecamp CLI that is not signed in ends the run, and an answer that is no
// token is sent nowhere. No message quotes what the Basecamp CLI printed.
func TestATokenTheBasecampCLIDoesNotHandOutIsSentNowhere(t *testing.T) {
	for mode, want := range map[string]Kind{
		"signed-out": AuthRequired,
		"spaced":     API,
		"none":       API,
		// The call asks nothing about the file, so neither of these may pass for
		// Basecamp's word on it: the one would write the file off as gone, the
		// other as refused.
		"not-found": API,
		"forbidden": API,
		"offline":   Network,
		"leaking":   API,
	} {
		t.Setenv("STANDIN_TOKEN", mode)
		standIn := tokenStandIn(t)
		b := newVersionBasecamp(t)
		var into bytes.Buffer
		err := b.downloader(standIn).DownloadVersion(t.Context(), b.api.URL+versionPath, &into)

		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != want {
			t.Errorf("%s: the download ended with %v, want %s", mode, err, want)
		}
		if err != nil && (strings.Contains(err.Error(), "two words") || strings.Contains(err.Error(), "no answer of any shape") ||
			strings.Contains(err.Error(), fakeSecret) || strings.Contains(err.Error(), "Not authenticated")) {
			t.Errorf("%s: the failure quotes what the Basecamp CLI printed: %v", mode, err)
		}
		if len(b.apiAsked) != 0 {
			t.Errorf("%s: the API was asked %v", mode, b.apiAsked)
		}
	}
}

// A store that stops bringing bytes is given up, as every download is, and counts
// as a failure of the network. The bound is one only a break trips.
func TestAnEarlierFileThatStopsComingIsGivenUp(t *testing.T) {
	standIn := tokenStandIn(t)
	b := newVersionBasecamp(t)
	b.storeServe = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("a beginning"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}
	downloader := b.downloader(standIn)
	downloader.IdleTimeout = 100 * time.Millisecond
	started := time.Now()
	err := downloader.DownloadVersion(t.Context(), b.api.URL+versionPath, &bytes.Buffer{})

	var failed *Error
	if !errors.As(err, &failed) || failed.Kind != Network || !strings.Contains(failed.Message, "given up") {
		t.Errorf("a store that stopped ended the download with %v, want it given up", err)
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Errorf("the download was given up after %s", elapsed)
	}
}
