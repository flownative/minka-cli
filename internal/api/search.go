package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

// SearchPageSize is the number of hits Minka puts on a page of a search. The
// address takes no size of its own.
const SearchPageSize = 25

// SearchTermMaxLength is where Minka cuts a search term, without a word.
const SearchTermMaxLength = 200

// SearchOffsetPattern is the shape of an offset Minka takes: the next of a page
// before, a whole number of at most six digits.
var SearchOffsetPattern = regexp.MustCompile(`^\d{1,6}$`)

// The kinds of entry a search finds.
const (
	SearchHitDocument = "document"
	SearchHitUpload   = "upload"
)

// SearchHit is one document or upload a search found.
type SearchHit struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"` // plain text around the match
	// Section is the heading above the match in a document; nil for an upload
	// and for a match before the first heading.
	Section   *SearchSection `json:"section"`
	ProjectID string         `json:"projectId"`
	ToolID    string         `json:"toolId"`
	Project   string         `json:"project"` // the name of the project
	UpdatedAt string         `json:"updatedAt"`
	// Archived marks an entry in the archive, or one in an archived folder:
	// it is found as on the web, and takes no write.
	Archived bool `json:"archived"`
	// AppURL ends in the anchor of the section where there is one.
	AppURL string `json:"appUrl"`
}

// SearchSection is the heading a document hit stands under, and the anchor
// the page gives it.
type SearchSection struct {
	Heading string `json:"heading"`
	Anchor  string `json:"anchor"`
}

// searchPage is one page of hits as the server sent them. Next is the offset
// of the page after it, and nil on the last one.
type searchPage struct {
	Hits []json.RawMessage `json:"hits"`
	Next *string           `json:"next"`
}

// SearchQuery is a search within a Docs & Files tool, a folder and everything
// beneath it, or both.
type SearchQuery struct {
	Term     string
	ToolID   string
	FolderID string
	Sort     string // best or recent; Minka takes anything else as best
	Offset   string // the next of the page before; the first page without it
}

// searchPageOf reads one page of hits.
func (c *Client) searchPageOf(ctx context.Context, query SearchQuery) (Result[searchPage], error) {
	values := url.Values{}
	values.Set("q", query.Term)
	if query.ToolID != "" {
		values.Set("toolId", query.ToolID)
	}
	if query.FolderID != "" {
		values.Set("folderId", query.FolderID)
	}
	if query.Sort != "" {
		values.Set("sort", query.Sort)
	}
	if query.Offset != "" {
		values.Set("offset", query.Offset)
	}
	return request[searchPage](ctx, c, http.MethodGet, "/search?"+values.Encode(), nil)
}

// SearchRun is what a search up to a number of hits brought back.
type SearchRun struct {
	Hits []SearchHit
	// Raw is the JSON of each hit as the server sent it, in the order of Hits,
	// so that a field the API gains reaches a reader across several pages.
	Raw []json.RawMessage
	// Next is the offset reading goes on from, nil where there is nothing
	// more. Where the run stopped inside a page, it points at the first hit of
	// that page the run left behind.
	Next *string
}

// SearchUpTo reads hits from the offset of the query on until it holds limit
// of them or there are no more, a page at a time. It asks for a page only
// while it still lacks hits, so a limit within one page costs one request.
//
// An offset counts hits, so the run can stop inside a page and still name
// where the rest begins. Whatever changes between two pages can shift a hit
// across the border: the run is no picture of one moment.
func (c *Client) SearchUpTo(ctx context.Context, query SearchQuery, limit int) (SearchRun, error) {
	var run SearchRun
	for {
		page, err := c.searchPageOf(ctx, query)
		if err != nil {
			return run, err
		}
		raw := page.Value.Hits
		hits, err := hitsOf(raw)
		if err != nil {
			return run, err
		}

		wanted := limit - len(run.Hits)
		if wanted < len(hits) {
			offset, err := offsetOf(query.Offset)
			if err != nil {
				return run, err
			}
			next := strconv.Itoa(offset + wanted)
			run.Hits = append(run.Hits, hits[:wanted]...)
			run.Raw = append(run.Raw, raw[:wanted]...)
			run.Next = &next
			return run, nil
		}

		run.Hits = append(run.Hits, hits...)
		run.Raw = append(run.Raw, raw...)
		run.Next = page.Value.Next
		// A next page is named by a number of hits and nothing else: an empty
		// one would start over, and it is handed on even where the run ends.
		if run.Next != nil && !SearchOffsetPattern.MatchString(*run.Next) {
			return run, notANumberOfHits(*run.Next)
		}
		// A page without hits ends the run whatever it says behind it, so an
		// answer that does not move on cannot keep it asking.
		if run.Next == nil || len(run.Hits) >= limit || len(hits) == 0 {
			return run, nil
		}
		query.Offset = *run.Next
	}
}

// offsetOf reads the offset a page was asked for with as the number of hits
// before it. The first page has none.
func offsetOf(offset string) (int, error) {
	if offset == "" {
		return 0, nil
	}
	if !SearchOffsetPattern.MatchString(offset) {
		return 0, notANumberOfHits(offset)
	}
	return strconv.Atoi(offset)
}

func notANumberOfHits(offset string) *Error {
	return &Error{
		Method:     http.MethodGet,
		StatusCode: http.StatusOK,
		Code:       "unexpectedResponse",
		Message:    "The server named a next page of a search with something that is not a number of hits: " + strconv.Quote(offset),
	}
}

// hitsOf reads each hit of a page. The page keeps them as the server sent
// them, so a hit that is no hit is told here rather than passed on.
func hitsOf(raw []json.RawMessage) ([]SearchHit, error) {
	hits := make([]SearchHit, len(raw))
	for i, hit := range raw {
		// Only an object is a hit; null would pass for an empty one.
		if trimmed := bytes.TrimSpace(hit); len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &hits[i]) != nil {
			return nil, &Error{
				Method:     http.MethodGet,
				StatusCode: http.StatusOK,
				Code:       "unexpectedResponse",
				Message:    "The server answered a search with something that is not a page of hits.",
			}
		}
	}
	return hits, nil
}
