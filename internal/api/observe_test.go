package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTheAnswersOfTheHostAreObservedRefusalsIncluded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Minka-CLI-Latest", "0.41.0")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"notFound","message":"no"}}`))
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")
	var seen []string
	client.ObserveAnswers(func(header http.Header) { seen = append(seen, header.Get("Minka-CLI-Latest")) })

	_, _ = client.Me(context.Background())
	_, _ = client.FileBytes(context.Background(), "b", "original")

	if len(seen) != 2 || seen[0] != "0.41.0" || seen[1] != "0.41.0" {
		t.Fatalf("observed %v", seen)
	}
}

func TestAnAnswerFromAnotherAddressIsNotObserved(t *testing.T) {
	client := New("https://minka.example.com", "minka_pat_x", "")
	observed := false
	client.ObserveAnswers(func(http.Header) { observed = true })

	store, _ := http.NewRequest(http.MethodGet, "https://store.example.com/blob", nil)
	client.observed(&http.Response{Request: store, Header: http.Header{"Minka-Cli-Latest": {"9.9.9"}}})
	if observed {
		t.Fatal("the answer of a store was observed")
	}

	home, _ := http.NewRequest(http.MethodGet, "https://minka.example.com/api/v1/me", nil)
	client.observed(&http.Response{Request: home, Header: http.Header{}})
	if !observed {
		t.Fatal("the answer of the host was not observed")
	}
}
