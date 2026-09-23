package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestIdentifiersAreEscapedInThePath(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")

	_, _ = client.Card(context.Background(), "../me")
	_, _ = client.Board(context.Background(), "a/b")
	_, _ = client.Boards(context.Background(), "x?y")
	_ = client.UnassignCard(context.Background(), "c/d", "../e")
	_, _ = client.People(context.Background(), "p/q")
	_, _ = client.AddStep(context.Background(), "s/t", "x")
	_ = client.DeleteStep(context.Background(), "c", "../s")
	_, _ = client.Messages(context.Background(), "x/y", MessagesQuery{})
	_ = client.DeleteMessage(context.Background(), "c", "../m")
	_ = client.Unsubscribe(context.Background(), "c", "../p")
	_ = client.DeleteComment(context.Background(), "c", "../k")
	_, _ = client.RestoreCard(context.Background(), "../x")
	_, _ = client.StartConversation(context.Background(), "../p")
	_, _ = client.UploadToConversation(context.Background(), "../c", "plan.png", "image/png", strings.NewReader("x"), 1)
	_, _ = client.ConversationPeople(context.Background(), "../p")
	_, _ = client.Trash(context.Background(), "p/t")
	_, _ = client.UploadToProject(context.Background(), "p/q", "../plan.png", "image/png", strings.NewReader("x"), 1)

	want := []string{
		"/api/v1/cards/..%2Fme",
		"/api/v1/boards/a%2Fb",
		"/api/v1/projects/x%3Fy/boards",
		"/api/v1/cards/c%2Fd/assignees/..%2Fe",
		"/api/v1/projects/p%2Fq/people",
		"/api/v1/cards/s%2Ft/steps",
		"/api/v1/cards/c/steps/..%2Fs",
		"/api/v1/chats/x%2Fy/messages",
		"/api/v1/chats/c/messages/..%2Fm",
		"/api/v1/cards/c/subscribers/..%2Fp",
		"/api/v1/cards/c/comments/..%2Fk",
		"/api/v1/cards/..%2Fx/restore",
		"/api/v1/messages/with/..%2Fp",
		"/api/v1/messages/..%2Fc/uploads",
		"/api/v1/messages/..%2Fp/people",
		"/api/v1/projects/p%2Ft/trash",
		"/api/v1/projects/p%2Fq/uploads",
	}
	if !slices.Equal(paths, want) {
		t.Errorf("paths\n got: %q\nwant: %q", paths, want)
	}
}

func TestErrorsNameTheMethodOfTheRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")

	_, err := client.WriteComment(context.Background(), "c", "hi")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Method != http.MethodPost || apiErr.Code != "serverError" {
		t.Errorf("got %#v", err)
	}

	server.Close()
	_, err = client.Me(context.Background())
	var networkErr *NetworkError
	if !errors.As(err, &networkErr) || networkErr.Method != http.MethodGet {
		t.Errorf("got %#v", err)
	}
}

func TestAnEmptyAnswerIsASuccess(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := New(server.URL, "minka_pat_x", "").DeleteCard(context.Background(), "a/b"); err != nil {
		t.Errorf("got %v", err)
	}
	if method != http.MethodDelete || path != "/api/v1/cards/a%2Fb" {
		t.Errorf("request %s %s", method, path)
	}
}

// What rides in the query is escaped, so that a slash or an ampersand of its
// own does not start something else: the name of a file, the content of a
// reaction.
func TestWhatRidesInTheQueryIsEscaped(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")

	for _, filename := range []string{"../plan.png", "a b&limit=1.png"} {
		_, _ = client.UploadToProject(context.Background(), "p", filename, "image/png", strings.NewReader("x"), 1)
	}
	_ = client.UnreactFromComment(context.Background(), "c", "k", "a b&x=1")
	_ = client.UnreactFromMessage(context.Background(), "c", "m", "👍&x=1")

	want := []string{
		"filename=..%2Fplan.png", "filename=a+b%26limit%3D1.png",
		"content=a+b%26x%3D1", "content=%F0%9F%91%8D%26x%3D1",
	}
	if !slices.Equal(queries, want) {
		t.Errorf("queries\n got: %q\nwant: %q", queries, want)
	}
}

// The bytes of a file may come from a store of their own, and the token must
// not travel there: the store knows nothing of Minka's tokens, and one that
// arrives is a secret in a log that does not belong to it.
func TestTheTokenStaysBehindWhereTheBytesComeFromSomewhereElse(t *testing.T) {
	var sentToTheStore http.Header
	store := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sentToTheStore = r.Header.Clone()
		_, _ = io.WriteString(w, "the bytes")
	}))
	defer store.Close()

	minka := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, store.URL+"/signed", http.StatusFound)
	}))
	defer minka.Close()

	client := New(minka.URL, "minka_pat_x", "")
	client.downloads.Transport = store.Client().Transport

	bytes, err := client.FileBytes(context.Background(), "b", "original")
	if err != nil {
		t.Fatalf("got %v", err)
	}
	defer bytes.Body.Close()

	if got := sentToTheStore.Get("Authorization"); got != "" {
		t.Errorf("the store was sent %q", got)
	}
	content, _ := io.ReadAll(bytes.Body)
	if string(content) != "the bytes" {
		t.Errorf("content %q", content)
	}
}

// Within the host that was signed in to the token belongs on the request, so
// a redirect that stays there carries it.
func TestTheTokenTravelsOnWithinTheHostSignedInTo(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if r.URL.Path != "/elsewhere" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "the bytes")
	}))
	defer server.Close()

	bytes, err := New(server.URL, "minka_pat_x", "").FileBytes(context.Background(), "b", "original")
	if err != nil {
		t.Fatalf("got %v", err)
	}
	defer bytes.Body.Close()

	if len(seen) != 2 || seen[0] != "Bearer minka_pat_x" || seen[1] != "Bearer minka_pat_x" {
		t.Errorf("authorization headers %q", seen)
	}
}

// A refused redirect is a refusal and not a failure of the network: trying
// again leads to the same address.
func TestTheBytesAreNotFetchedFromAnAddressThatIsNotHttps(t *testing.T) {
	asked := 0
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked++ }))
	defer elsewhere.Close()

	minka := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/signed", http.StatusFound)
	}))
	defer minka.Close()

	_, err := New(minka.URL, "minka_pat_x", "").FileBytes(context.Background(), "b", "original")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "redirectRefused" {
		t.Fatalf("got %#v", err)
	}
	if strings.Contains(apiErr.Message, elsewhere.URL) {
		t.Errorf("the refusal repeats the address: %q", apiErr.Message)
	}
	if asked != 0 {
		t.Errorf("%d requests to an address that is not https", asked)
	}
}

// A client with a redirect check of its own gets none of Go's, so the number
// of hops is counted here or not at all. The server gives up sending people on
// long before a client without a limit would, so the test says how many hops
// were taken and does not wait for a timeout to say it.
func TestTheBytesFollowAtMostFiveRedirects(t *testing.T) {
	hops := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hops++; hops > maxRedirects+3 {
			_, _ = io.WriteString(w, "the bytes")
			return
		}
		http.Redirect(w, r, "/again", http.StatusFound)
	}))
	defer server.Close()

	_, err := New(server.URL, "minka_pat_x", "").FileBytes(context.Background(), "b", "original")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "redirectRefused" {
		t.Fatalf("got %#v after %d hops", err, hops)
	}
	if hops > maxRedirects+1 {
		t.Errorf("%d hops, want no more than %d", hops, maxRedirects+1)
	}
}
