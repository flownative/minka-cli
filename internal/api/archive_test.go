package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// The server cuts a page above its largest down without a word, and the only
// sign its answer carries for the end of the archive is fewer cards than were
// asked for. A page beyond the bound is therefore refused here, before a request
// goes out, and so is one below it: nothing of this client can reach the silent
// cut.
func TestNoPageOfAnArchiveIsAskedForBeyondWhatTheServerServes(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = io.WriteString(w, "[]")
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")

	for _, limit := range []int{0, -1, ArchivePageMax + 1, 500} {
		if _, err := client.ArchivedCards(context.Background(), "b", "", limit); !errors.Is(err, ErrArchivePageOutOfBounds) {
			t.Errorf("a page of %d: %v", limit, err)
		}
		if _, err := client.WholeArchiveOf(context.Background(), "b", "", limit); !errors.Is(err, ErrArchivePageOutOfBounds) {
			t.Errorf("a walk in pages of %d: %v", limit, err)
		}
	}
	if len(queries) != 0 {
		t.Fatalf("requests went out for a page nobody may ask for: %q", queries)
	}

	// The page size rides in the query on every request, so the server is never
	// left to choose it.
	if _, err := client.ArchivedCards(context.Background(), "b", "0199", ArchivePageMax); err != nil {
		t.Fatal(err)
	}
	if _, err := client.WholeArchiveOf(context.Background(), "b", "", ArchivePageDefault); err != nil {
		t.Fatal(err)
	}
	want := []string{"before=0199&limit=100", "limit=50"}
	if !slices.Equal(queries, want) {
		t.Errorf("queries\n got: %q\nwant: %q", queries, want)
	}
}

// A walk over a board that is not there ends on the answer of the server, and
// the cards it read before are handed back with it: nothing of a walk that broke
// off is thrown away.
// A page of one card leaves no other card of that page to carry on behind, so a
// cursor the server will not read behind sends the walk back to where it began,
// once, and then ends it.
func TestAWalkHandsBackWhatItReadBeforeItBrokeOff(t *testing.T) {
	const theOneCard = `[{"id":"a","shortId":null,"title":"One","archivedAt":null,"archivedBy":null,"appUrl":"u"}]`
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("before") == "" {
			_, _ = io.WriteString(w, theOneCard)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":{"code":"archivedCardsBeforeUnknown","message":"gone"}}`)
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")

	run, err := client.WholeArchiveOf(context.Background(), "b", "", 1)
	if !isBeforeUnknown(err) {
		t.Fatalf("err %v", err)
	}
	// What it read is handed back, the card it met twice counted and listed
	// once, and the walk did not go round: it began again exactly once.
	if len(run.Cards) != 1 || run.Cards[0].ID != "a" || len(run.Raw) != 1 {
		t.Fatalf("run %+v", run)
	}
	if !run.BeganAgain || run.Duplicates != 1 || run.Pages != 2 {
		t.Fatalf("run %+v", run)
	}
	if requests != 4 {
		t.Fatalf("%d requests, want the page, the refusal, the page again and the refusal again", requests)
	}
}
