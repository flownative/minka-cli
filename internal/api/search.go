package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// SearchPageSize is the number of hits Minka puts on a page of a search. The
// address takes no size of its own.
const SearchPageSize = 25

// SearchTermMaxLength is where Minka cuts a search term, without a word.
const SearchTermMaxLength = 200

// SearchOffsetPattern is the shape of an offset Minka takes: the next of a page
// before, a whole number of at most six digits.
var SearchOffsetPattern = regexp.MustCompile(`^\d{1,6}$`)

// SearchDeepestOffset is the last offset Minka serves a page from. What lies
// behind the page there is reached only by a narrower search.
const SearchDeepestOffset = 500

// SearchKinds are the words a search is narrowed to tools with. A comment
// counts toward the tool of what it stands under.
var SearchKinds = []string{"board", "posts", "todos", "files", "calendar", "decisions", "chat", "conversation"}

// SearchKindFiles is the kind of the documents and uploads, which are all a
// search within a Files tool or a folder finds.
const SearchKindFiles = "files"

// The kinds of hit a search finds.
const (
	SearchHitCard         = "card"
	SearchHitComment      = "comment"
	SearchHitPost         = "post"
	SearchHitTodoList     = "todoList"
	SearchHitTodo         = "todo"
	SearchHitTodoGroup    = "todoGroup"
	SearchHitFolder       = "folder"
	SearchHitDocument     = "document"
	SearchHitUpload       = "upload"
	SearchHitEvent        = "event"
	SearchHitDecision     = "decision"
	SearchHitChat         = "chat"
	SearchHitConversation = "conversation"
)

// SearchHit is one thing a search found: an item, a comment under one, or a
// line of a chat or a private conversation.
type SearchHit struct {
	Kind string `json:"kind"`
	ID   string `json:"id"` // the hit itself, such as the comment or the message
	// TargetKind and TargetID name what the addresses lead to: the item a
	// comment stands under, the chat or conversation of a line, else the hit
	// itself. A Minka that searches only within files sends neither.
	TargetKind string `json:"targetKind"`
	TargetID   string `json:"targetId"`
	// Title is the item's, for a comment the one of its item, for a line of a
	// chat the chat's and for a private conversation the other person's name.
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"` // plain text around the match
	// Section is the heading above the match in a document; nil for every
	// other kind and for a match before the first heading.
	Section   *SearchSection `json:"section"`
	ProjectID string         `json:"projectId"` // empty for a private conversation
	ToolID    string         `json:"toolId"`
	Project   string         `json:"project"` // the name of the project
	// ShortID is the card's, or the one of the card a comment stands under,
	// where its board shows short identifiers.
	ShortID   *string `json:"shortId"`
	UpdatedAt string  `json:"updatedAt"`
	// Archived marks an item in the archive, or one in an archived folder or
	// list: it is found as on the web, and takes no write.
	Archived bool `json:"archived"`
	// AppURL ends in the anchor of the section, the comment or the line where
	// there is one.
	AppURL string `json:"appUrl"`
	// APIURL is nil where the API shows the target nowhere on its own.
	APIURL *string `json:"apiUrl"`
}

// Target is the kind and ID of what the hit leads to. A Minka that names no
// target found the item itself.
func (h SearchHit) Target() (kind, id string) {
	if h.TargetKind == "" {
		return h.Kind, h.ID
	}
	return h.TargetKind, h.TargetID
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

// SearchQuery is a search across everything the person may read, narrowed to
// kinds and a project, or within a Docs & Files tool, a folder and everything
// beneath it, or both.
type SearchQuery struct {
	Term      string
	Kinds     []string // none for every kind
	ProjectID string
	ToolID    string
	FolderID  string
	Sort      string // best or recent; Minka takes anything else as best
	Offset    string // the next of the page before; the first page without it
}

// searchPageOf reads one page of hits.
func (c *Client) searchPageOf(ctx context.Context, query SearchQuery) (Result[searchPage], error) {
	values := url.Values{}
	values.Set("q", query.Term)
	if len(query.Kinds) > 0 {
		values.Set("kinds", strings.Join(query.Kinds, ","))
	}
	if query.ProjectID != "" {
		values.Set("projectId", query.ProjectID)
	}
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
	// more or where it lies beyond the deepest offset. Where the run stopped
	// inside a page, it points at the first hit of that page the run left
	// behind.
	Next *string
	// Deepest says that the run met the deepest offset Minka serves, with
	// hits behind it or a full page there that may have more behind it.
	Deepest bool
}

// SearchUpTo reads hits from the offset of the query on until it holds limit
// of them or there are no more, a page at a time. It asks for a page only
// while it still lacks hits, so a limit within one page costs one request.
//
// An offset counts hits, so the run can stop inside a page and still name
// where the rest begins. Whatever changes between two pages can shift a hit
// across the border: the run is no picture of one moment.
//
// No offset beyond the deepest one is asked for or handed on, whether Minka
// names one or not: Deepest says so instead.
func (c *Client) SearchUpTo(ctx context.Context, query SearchQuery, limit int) (SearchRun, error) {
	var run SearchRun
	for {
		offset, err := offsetOf(query.Offset)
		if err != nil {
			return run, err
		}
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
			run.Hits = append(run.Hits, hits[:wanted]...)
			run.Raw = append(run.Raw, raw[:wanted]...)
			run.setNext(offset + wanted)
			return run, nil
		}

		run.Hits = append(run.Hits, hits...)
		run.Raw = append(run.Raw, raw...)
		run.Next = page.Value.Next
		// A next page is named by a number of hits and nothing else: an empty
		// one would start over, and it is handed on even where the run ends.
		if run.Next != nil {
			if !SearchOffsetPattern.MatchString(*run.Next) {
				return run, notANumberOfHits(*run.Next)
			}
			next, _ := strconv.Atoi(*run.Next)
			run.setNext(next)
		}
		// A full page that ends behind the deepest offset may have more behind
		// it, although Minka names no next page there.
		if run.Next == nil && len(hits) >= SearchPageSize && offset+SearchPageSize > SearchDeepestOffset {
			run.Deepest = true
		}
		// A page without hits ends the run whatever it says behind it, so an
		// answer that does not move on cannot keep it asking.
		if run.Next == nil || len(run.Hits) >= limit || len(hits) == 0 {
			return run, nil
		}
		query.Offset = *run.Next
	}
}

// setNext hands on the offset reading goes on from, or none where Minka would
// refuse it.
func (r *SearchRun) setNext(offset int) {
	if offset > SearchDeepestOffset {
		r.Next, r.Deepest = nil, true
		return
	}
	next := strconv.Itoa(offset)
	r.Next = &next
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
