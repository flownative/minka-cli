package api

import (
	"context"
	"errors"
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
