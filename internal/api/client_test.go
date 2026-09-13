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

	want := []string{"/api/v1/cards/..%2Fme", "/api/v1/boards/a%2Fb", "/api/v1/projects/x%3Fy/boards"}
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
