package api

import (
	"context"
	"encoding/json"
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

// A page that ends on the card it was asked to start behind carries a walk no
// further: the next request would be the one just sent. Such an answer ends the
// walk with what it read, rather than sending the same question for ever.
func TestAWalkEndsWhereAPageCarriesItNoFurther(t *testing.T) {
	const bothCards = `[{"id":"a","shortId":null,"title":"One","archivedAt":null,"archivedBy":null,"appUrl":"u"},
		{"id":"b","shortId":null,"title":"Two","archivedAt":null,"archivedBy":null,"appUrl":"u"}]`
	const theBoundaryItself = `[{"id":"b","shortId":null,"title":"Two","archivedAt":null,"archivedBy":null,"appUrl":"u"}]`
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		// A server that takes the card of the cursor into the page it answers,
		// which the address says it does not.
		if r.URL.Query().Get("before") == "" {
			_, _ = io.WriteString(w, bothCards)
			return
		}
		_, _ = io.WriteString(w, theBoundaryItself)
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")

	run, err := client.WholeArchiveOf(context.Background(), "b", "", 2)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "unexpectedResponse" {
		t.Fatalf("err %v", err)
	}
	if len(run.Cards) != 2 || run.Cards[0].ID != "a" || run.Cards[1].ID != "b" {
		t.Errorf("a walk that ended hands back what it read: %+v", run.Cards)
	}
	if requests != 2 {
		t.Fatalf("%d requests, want the page and the one that carried nothing", requests)
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

// The JSON of a page is cut into the JSON of its cards, so that a field the API
// gains reaches a reader of several pages as well. An answer that does not give
// one piece per card is none to read cards out of, whatever it is instead.
func TestAPageThatDoesNotGiveOnePiecePerCardIsRefused(t *testing.T) {
	for _, page := range []struct {
		raw   string
		cards int
	}{
		{`{"id":"a"}`, 1},
		{`[{"id":"a"}]`, 2},
	} {
		each, err := eachCardOf(json.RawMessage(page.raw), page.cards)
		var apiErr *Error
		if each != nil || !errors.As(err, &apiErr) || apiErr.Code != "unexpectedResponse" {
			t.Errorf("%s as %d cards: %v, %v", page.raw, page.cards, each, err)
		}
	}

	each, err := eachCardOf(json.RawMessage(`[{"id":"a"},{"id":"b"}]`), 2)
	if err != nil || len(each) != 2 || string(each[1]) != `{"id":"b"}` {
		t.Fatalf("a page of two cards: %v, %v", each, err)
	}
}
