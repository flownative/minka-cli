package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// searchServer answers every request for a page of a search with the answer
// for its number, the first request being 0; past the last answer it fails, so
// a run that keeps asking ends rather than hangs.
func searchServer(t *testing.T, answers ...string) (*Client, *int) {
	t.Helper()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { requests++ }()
		if requests >= len(answers) {
			http.Error(w, "asked once too often", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, answers[requests])
	}))
	t.Cleanup(server.Close)
	return New(server.URL, "minka_pat_x", ""), &requests
}

func fullPage(next string) string {
	hits := make([]string, SearchPageSize)
	for i := range hits {
		hits[i] = fmt.Sprintf(`{"kind":"document","id":"d%d","title":"Lamp"}`, i)
	}
	return `{"hits":[` + strings.Join(hits, ",") + `],"next":` + next + `}`
}

func isUnexpected(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Code == "unexpectedResponse"
}

// A page without hits ends a run whatever it names behind it: an answer that
// does not move on cannot keep the run asking.
func TestAPageWithoutHitsEndsASearch(t *testing.T) {
	empty := `{"hits":[],"next":"25"}`
	client, requests := searchServer(t, empty, empty, empty)

	run, err := client.SearchUpTo(context.Background(), SearchQuery{Term: "lamp", ToolID: "t"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if *requests != 1 || len(run.Hits) != 0 {
		t.Fatalf("%d requests and %d hits, want one request and none", *requests, len(run.Hits))
	}
}

// A next page that is named by anything but a number of hits is no answer the
// run can go on from, and says so instead of guessing.
func TestANextThatIsNoNumberIsRefused(t *testing.T) {
	for _, next := range []string{`"abc"`, `"-25"`, `"2.5"`, `""`} {
		t.Run(next, func(t *testing.T) {
			// With room for more and with a run that ends on this page alike:
			// the next is handed on in either case.
			for _, limit := range []int{50, 25} {
				client, requests := searchServer(t, fullPage(next), fullPage("null"))
				_, err := client.SearchUpTo(context.Background(), SearchQuery{Term: "lamp", ToolID: "t"}, limit)
				if !isUnexpected(err) {
					t.Fatalf("limit %d: error %v, want unexpectedResponse", limit, err)
				}
				if *requests != 1 {
					t.Errorf("limit %d: %d requests, the run went on from %s", limit, *requests, next)
				}
			}
		})
	}
}

// A hit that is no hit is refused, and so is a page whose hits are no list.
func TestHitsThatAreNoHitsAreRefused(t *testing.T) {
	for _, answer := range []string{
		`{"hits":[1],"next":null}`,
		`{"hits":[null],"next":null}`,
		`{"hits":["d1"],"next":null}`,
		`{"hits":{"id":"d1"},"next":null}`,
	} {
		t.Run(answer, func(t *testing.T) {
			client, _ := searchServer(t, answer)
			if _, err := client.SearchUpTo(context.Background(), SearchQuery{Term: "lamp", ToolID: "t"}, 25); !isUnexpected(err) {
				t.Fatalf("error %v, want unexpectedResponse", err)
			}
		})
	}
}

// The offset a page was asked with is what a run cut inside the page counts
// from, so one that is no number is refused rather than counted as none.
func TestAnOffsetThatIsNoNumberIsNotCountedFrom(t *testing.T) {
	client, _ := searchServer(t, fullPage("null"))
	if _, err := client.SearchUpTo(context.Background(), SearchQuery{Term: "lamp", ToolID: "t", Offset: "x"}, 10); !isUnexpected(err) {
		t.Fatalf("error %v, want unexpectedResponse", err)
	}
}
