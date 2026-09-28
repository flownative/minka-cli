package basecamp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"
)

const (
	avatarTimeout  = 30 * time.Second
	maxAvatarBytes = 5 << 20
	maxRedirects   = 5
)

// Avatar is the picture of a person as it was downloaded.
type Avatar struct {
	Content     []byte
	ContentType string
}

type AvatarFetcher interface {
	Fetch(ctx context.Context, address string) (Avatar, error)
}

// avatarFilenames names the file of an avatar after its content type. What is
// no picture is no avatar: an error page must not end up in a bundle.
var avatarFilenames = map[string]string{
	"image/webp": "avatar.webp",
	"image/png":  "avatar.png",
	"image/jpeg": "avatar.jpg",
	"image/gif":  "avatar.gif",
	"image/avif": "avatar.avif",
}

// errRedirectRefused marks a redirect this client does not follow. It is a
// refusal, not a failure of the network: trying again changes nothing.
var errRedirectRefused = errors.New("redirect refused")

// AvatarClient downloads avatars. Basecamp serves them without a sign-in, so
// no token travels with the request; the address comes out of an API answer,
// and the client still takes nothing but https, neither at first nor after a
// redirect.
type AvatarClient struct {
	http *http.Client
}

func NewAvatarClient() *AvatarClient {
	return newAvatarClient(&http.Client{}, avatarTimeout)
}

// newAvatarClient sets the limits on a client. With a redirect check of its
// own a client follows redirects without end, so the number is limited here.
func newAvatarClient(client *http.Client, timeout time.Duration) *AvatarClient {
	client.Timeout = timeout
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return fmt.Errorf("%w: it leads to %s, which is not https", errRedirectRefused, request.URL.Host)
		}
		if len(via) > maxRedirects {
			return fmt.Errorf("%w: more than %d in a row", errRedirectRefused, maxRedirects)
		}
		return nil
	}
	return &AvatarClient{http: client}
}

// cause leaves the address out of a failed request: the address of an avatar
// names a person, and a reason ends up in reports.
func cause(err error) string {
	var failed *url.Error
	if errors.As(err, &failed) {
		return failed.Err.Error()
	}
	return err.Error()
}

func (c *AvatarClient) Fetch(ctx context.Context, address string) (Avatar, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return Avatar{}, &Error{Kind: API, Message: "Basecamp names an avatar at an address that is not https"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return Avatar{}, &Error{Kind: API, Message: "the address of an avatar cannot be used: " + err.Error()}
	}

	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return Avatar{}, &Error{Kind: Interrupted, Message: "interrupted"}
		}
		if errors.Is(err, errRedirectRefused) {
			return Avatar{}, &Error{Kind: API, Message: "downloading an avatar: " + cause(err)}
		}
		return Avatar{}, &Error{Kind: Network, Message: "downloading an avatar: " + cause(err)}
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return Avatar{}, &Error{Kind: kindOfAnAvatarStatus(response.StatusCode),
			Message: fmt.Sprintf("downloading an avatar from %s: HTTP %d", parsed.Host, response.StatusCode)}
	}
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if _, ok := avatarFilenames[contentType]; !ok {
		return Avatar{}, &Error{Kind: API, Message: fmt.Sprintf("%s answered for an avatar with %q, which is no picture the bundle takes", parsed.Host, contentType)}
	}

	content, err := io.ReadAll(io.LimitReader(response.Body, maxAvatarBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return Avatar{}, &Error{Kind: Interrupted, Message: "interrupted"}
		}
		return Avatar{}, &Error{Kind: Network, Message: "downloading an avatar: " + err.Error()}
	}
	if len(content) > maxAvatarBytes {
		return Avatar{}, &Error{Kind: API, Message: fmt.Sprintf("an avatar from %s is larger than %d bytes", parsed.Host, maxAvatarBytes)}
	}
	return Avatar{Content: content, ContentType: contentType}, nil
}

// kindOfAnAvatarStatus is the class of a status that is no 200. What the class
// decides is what becomes of the person and of the run, so a status that says
// the picture is not there for anybody is Basecamp's own word about it, one of a
// server or a limit may pass, and anything else is an answer this run cannot
// place. The statuses that may pass are the ones a request of the API takes for
// the same, so that a file and a request are not read by two rules.
//
// **A 401 is no word about the picture**: it says this run is not signed in, and
// a sign-in that is gone would lose every avatar of an account the same way. It
// ends the run, as it does for a file, rather than writing off what nobody
// refused. A 403 **is** the source's word, and only because the host is
// Basecamp's own: were avatars ever served from somewhere else against a signed
// address, one that has run out would answer 403 and reach the reading that
// writes off.
func kindOfAnAvatarStatus(status int) Kind {
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		return NotFound
	case status == http.StatusUnauthorized:
		return AuthRequired
	case status == http.StatusForbidden:
		return Forbidden
	case status == http.StatusTooManyRequests:
		return RateLimited
	case status >= 500 || status == http.StatusRequestTimeout:
		return Unavailable
	default:
		return API
	}
}
