package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
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
