package basecamp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// avatarServer serves pictures over https, the only way an avatar is fetched.
func avatarServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *AvatarClient) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return server, newAvatarClient(server.Client(), avatarTimeout)
}

func expectKind(t *testing.T, err error, want Kind) {
	t.Helper()
	var failed *Error
	if !errors.As(err, &failed) || failed.Kind != want {
		t.Fatalf("got %v, want an error of kind %q", err, want)
	}
}

func TestAnAvatarComesWithItsContentType(t *testing.T) {
	server, client := avatarServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/webp; charset=binary")
		_, _ = w.Write([]byte("a picture"))
	})

	avatar, err := client.Fetch(t.Context(), server.URL+"/people/1/avatar")
	if err != nil {
		t.Fatal(err)
	}
	if avatar.ContentType != "image/webp" || !bytes.Equal(avatar.Content, []byte("a picture")) {
		t.Errorf("got %q as %q", avatar.Content, avatar.ContentType)
	}
}

func TestAnAvatarIsOnlyFetchedOverHTTPS(t *testing.T) {
	asked := false
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked = true
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("a picture"))
	}))
	t.Cleanup(plain.Close)

	_, err := NewAvatarClient().Fetch(t.Context(), plain.URL+"/avatar")
	expectKind(t, err, API)
	if asked {
		t.Error("a request went out over http")
	}

	server, client := avatarServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/avatar", http.StatusFound)
	})
	// A redirect that is refused is no failure of the network: trying again
	// changes nothing.
	_, err = client.Fetch(t.Context(), server.URL+"/people/secret-of-a-person/avatar")
	expectKind(t, err, API)
	if asked {
		t.Error("a redirect was followed to http")
	}
	if strings.Contains(err.Error(), "secret-of-a-person") {
		t.Errorf("the reason names the address of the avatar: %s", err)
	}
}

func TestRedirectsAreFollowedOnlySoFar(t *testing.T) {
	hops := 0
	server, client := avatarServer(t, func(w http.ResponseWriter, r *http.Request) {
		hops++
		if hops > 4*maxRedirects {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("a picture at the end of a long way"))
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", hops), http.StatusFound)
	})

	_, err := client.Fetch(t.Context(), server.URL+"/avatar")
	expectKind(t, err, API)
	if hops != maxRedirects+1 {
		t.Errorf("asked %d times, want the first request and %d redirects", hops, maxRedirects)
	}
}

func TestAnAvatarThatDoesNotComeIsGivenUpAfterTheTimeout(t *testing.T) {
	if got := NewAvatarClient().http.Timeout; got != avatarTimeout || got <= 0 {
		t.Errorf("the timeout of the client is %s, want %s", got, avatarTimeout)
	}

	released := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-released:
		case <-time.After(3 * time.Second):
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("a picture that came late"))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(released) })

	_, err := newAvatarClient(server.Client(), 50*time.Millisecond).Fetch(t.Context(), server.URL+"/avatar")
	expectKind(t, err, Network)
}

func TestAnInterruptedDownloadIsNoFailureOfTheNetwork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	server, client := avatarServer(t, func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	})
	_, err := client.Fetch(ctx, server.URL+"/avatar")
	expectKind(t, err, Interrupted)

	// The same when the picture has already begun to come.
	ctx, cancel = context.WithCancel(t.Context())
	server, _ = avatarServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("the beginning of a picture"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		// Returning would end the body properly, and the client can still read
		// that end: closing TLS tells the server first and cuts the connection
		// only afterwards. The picture must stay unfinished.
		panic(http.ErrAbortHandler)
	})
	client = newAvatarClient(&http.Client{Transport: stopOnceAnswered{server.Client().Transport, cancel}}, avatarTimeout)
	_, err = client.Fetch(ctx, server.URL+"/avatar")
	expectKind(t, err, Interrupted)
}

// stopOnceAnswered interrupts a request when its answer has begun, so that
// the interruption meets the reading of the body and nothing before it.
type stopOnceAnswered struct {
	inner http.RoundTripper
	stop  context.CancelFunc
}

func (s stopOnceAnswered) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := s.inner.RoundTrip(request)
	s.stop()
	return response, err
}

func TestAnAvatarFollowsARedirectThatStaysOnHTTPS(t *testing.T) {
	server, client := avatarServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/avatar" {
			http.Redirect(w, r, "/stored/avatar.png", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("a picture"))
	})
	avatar, err := client.Fetch(t.Context(), server.URL+"/avatar")
	if err != nil {
		t.Fatal(err)
	}
	if avatar.ContentType != "image/png" {
		t.Errorf("content type %q", avatar.ContentType)
	}
}

func TestWhatIsNoPictureIsNoAvatar(t *testing.T) {
	server, client := avatarServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>Sign in</html>"))
		case "/gone":
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("a placeholder"))
		}
	})
	_, err := client.Fetch(t.Context(), server.URL+"/page")
	expectKind(t, err, API)
	_, err = client.Fetch(t.Context(), server.URL+"/gone")
	expectKind(t, err, API)
}

// A server that never stops sending must not fill the memory: reading ends
// one byte behind the largest size, not when the server is done.
func TestAnAvatarIsNotReadBeyondItsLargestSize(t *testing.T) {
	var sent atomic.Int64
	server, client := avatarServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		chunk := []byte(strings.Repeat("x", 64<<10))
		for sent.Load() < 12*maxAvatarBytes {
			n, err := w.Write(chunk)
			if err != nil {
				return
			}
			sent.Add(int64(n))
		}
	})

	_, err := client.Fetch(t.Context(), server.URL+"/avatar")
	expectKind(t, err, API)
	if got := sent.Load(); got >= 12*maxAvatarBytes {
		t.Errorf("the server got rid of all %d bytes, want the download to end near %d", got, maxAvatarBytes)
	}
}

func TestAnAvatarHasALargestSize(t *testing.T) {
	server, client := avatarServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(strings.Repeat("x", maxAvatarBytes+1)))
	})
	_, err := client.Fetch(t.Context(), server.URL+"/avatar")
	expectKind(t, err, API)
}

func TestAServerThatDoesNotAnswerIsANetworkFailure(t *testing.T) {
	server, client := avatarServer(t, func(http.ResponseWriter, *http.Request) {})
	address := server.URL + "/people/secret-of-a-person/avatar"
	server.Close()

	_, err := client.Fetch(t.Context(), address)
	expectKind(t, err, Network)
	// The address of an avatar names a person, and the reason goes into reports.
	if strings.Contains(err.Error(), "secret-of-a-person") {
		t.Errorf("the reason names the address of the avatar: %s", err)
	}
}
