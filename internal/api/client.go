// Package api is a client for the JSON API of Minka under /api/v1.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxResponseBytes bounds what the client reads of a response. A board lists
// at most 25 cards in Done, so real responses stay far below it.
const maxResponseBytes = 16 << 20

// bodyTimeout is what sending or fetching the bytes of a file may take. It is
// far above the timeout of the other requests, which carry nothing but JSON.
const bodyTimeout = 10 * time.Minute

// maxRedirects is how many hops the bytes of a file may take before the client
// gives up on them.
const maxRedirects = 5

// CodeRedirectRefused is what a caller reads where the bytes of a file were
// to be fetched at an address this client does not go to.
const CodeRedirectRefused = "redirectRefused"

// ErrRedirectRefused marks a redirect this client does not follow. It is a
// refusal, not a failure of the network: trying again changes nothing.
var ErrRedirectRefused = errors.New("redirect refused")

// refusedRedirect is such a refusal together with the answer that sent it on,
// so that a caller can say which status it was turned away at.
type refusedRedirect struct {
	status int
	reason string
}

func (e *refusedRedirect) Error() string { return ErrRedirectRefused.Error() + ": " + e.reason }

func (e *refusedRedirect) Is(target error) bool { return target == ErrRedirectRefused }

// Client talks to one Minka host with one access token.
type Client struct {
	host      string
	address   *url.URL // the host as an address, to hold a redirect against
	token     string
	userAgent string
	http      *http.Client
	uploads   *http.Client
	downloads *http.Client
}

// New returns a client for a host such as "https://minka.localbeach.net".
//
// No request that carries JSON follows a redirect: it would take the token to
// an address the user did not name, and the API has no reason to send one.
// The bytes of a file are the exception, because a store that serves them
// itself is answered with an address of its own; that request follows, and
// the token stays behind as soon as the address is not the host signed in to.
func New(host, token, userAgent string) *Client {
	noRedirect := func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	c := &Client{
		host:      strings.TrimRight(host, "/"),
		token:     token,
		userAgent: userAgent,
		http: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: noRedirect,
		},
		uploads: &http.Client{
			Timeout:       bodyTimeout,
			CheckRedirect: noRedirect,
		},
	}
	c.address, _ = url.Parse(c.host)
	c.downloads = &http.Client{Timeout: bodyTimeout, CheckRedirect: c.followTheBytes}
	return c
}

// followTheBytes decides where the bytes of a file may be fetched from.
//
// Go leaves the Authorization header on a redirect that stays within the same
// domain, a host below it included, so a host below ours would be handed the
// token. The header is therefore taken off here for every address that is not
// the one signed in to, whatever Go would have done with it, and an address
// that is not https is not followed at all.
func (c *Client) followTheBytes(request *http.Request, via []*http.Request) error {
	// The answer that sent us on rides on the request Go built from it.
	status := 0
	if request.Response != nil {
		status = request.Response.StatusCode
	}

	if len(via) > maxRedirects {
		return &refusedRedirect{status, fmt.Sprintf("more than %d in a row", maxRedirects)}
	}
	if c.isTheHostSignedInTo(request.URL) {
		return nil
	}
	request.Header.Del("Authorization")
	if request.URL.Scheme != "https" {
		return &refusedRedirect{status, fmt.Sprintf("it leads to %s, which is not https", request.URL.Hostname())}
	}
	return nil
}

// isTheHostSignedInTo says whether an address is the host of this client. A
// host name does not care about its case, and the standard port of a scheme
// is the same address as none at all: an answer that names one of the two
// spellings still leads home and keeps the token.
func (c *Client) isTheHostSignedInTo(address *url.URL) bool {
	if c.address == nil {
		return false
	}
	return address.Scheme == c.address.Scheme &&
		strings.EqualFold(address.Hostname(), c.address.Hostname()) &&
		portOf(address) == portOf(c.address)
}

// portOf is the port an address speaks to, written out where the scheme
// leaves it unsaid.
func portOf(address *url.URL) string {
	if port := address.Port(); port != "" {
		return port
	}
	switch address.Scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// Result is a decoded resource together with the JSON the server sent for it.
// The raw JSON is what the CLI prints for --json, so that fields the server
// adds reach a client without a change here.
type Result[T any] struct {
	Value      T
	Raw        json.RawMessage
	StatusCode int
	Header     http.Header
}

// Error is an answer of the server that is not a success.
type Error struct {
	Method     string
	StatusCode int
	Code       string // the code of the error body, or one derived from the status
	Message    string
	Body       json.RawMessage // the answer as sent, for a refusal that carries more than its error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s (HTTP %d, %s)", e.Message, e.StatusCode, e.Code)
}

// NetworkError means that no complete answer arrived: the host is
// unreachable, the TLS handshake failed, the request timed out or was
// cancelled.
type NetworkError struct {
	Method string
	Err    error
}

func (e *NetworkError) Error() string {
	return e.Err.Error()
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

func (c *Client) Me(ctx context.Context) (Result[Me], error) {
	return request[Me](ctx, c, http.MethodGet, "/me", nil)
}

// NoteAtWork tells the host which cards one client of this agent is working on
// right now. It is the whole set every time: a card left out is given up in
// the same breath, and an empty set says that the client has stopped.
//
// Only an agent may say so, and only for itself. A card the agent may not see
// and an identifier that is no UUID are passed over by the server without a
// word, so a set that names nothing it knows arrives as an empty one.
func (c *Client) NoteAtWork(ctx context.Context, clientID string, cardIDs []string) error {
	if cardIDs == nil {
		cardIDs = []string{}
	}
	body := struct {
		ClientID string   `json:"clientId"`
		CardIDs  []string `json:"cardIds"`
	}{clientID, cardIDs}
	_, err := request[struct{}](ctx, c, http.MethodPut, "/me/work", body)
	return err
}

// Projects lists the projects in use, or the archived ones instead.
func (c *Client) Projects(ctx context.Context, archived bool) (Result[[]Project], error) {
	path := "/projects"
	if archived {
		path += "?status=archived"
	}
	return request[[]Project](ctx, c, http.MethodGet, path, nil)
}

func (c *Client) Boards(ctx context.Context, projectID string) (Result[[]BoardListItem], error) {
	return request[[]BoardListItem](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/boards", nil)
}

func (c *Client) Board(ctx context.Context, boardID string) (Result[Board], error) {
	return request[Board](ctx, c, http.MethodGet, "/boards/"+url.PathEscape(boardID), nil)
}

// Trash lists what was taken out of a project: cards, columns and tools, the
// newest first. The answer carries at most 200 entries and says so in the
// header X-Minka-Truncated when there are more.
func (c *Client) Trash(ctx context.Context, projectID string) (Result[[]TrashEntry], error) {
	return request[[]TrashEntry](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/trash", nil)
}

// MyCards lists the cards the person of the token is responsible for, across
// the projects they are a member of. Cards in Done are not among them.
func (c *Client) MyCards(ctx context.Context) (Result[[]Card], error) {
	return request[[]Card](ctx, c, http.MethodGet, "/me/cards", nil)
}

func (c *Client) Card(ctx context.Context, cardID string) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodGet, "/cards/"+url.PathEscape(cardID), nil)
}

// CardByShortIdentifier finds a card under the name people call it by, such as
// "MNK-1A".
func (c *Client) CardByShortIdentifier(ctx context.Context, identifier string) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodGet, "/cards/by-short-id/"+url.PathEscape(identifier), nil)
}

func (c *Client) CreateCard(ctx context.Context, boardID string, input CreateCardInput) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodPost, "/boards/"+url.PathEscape(boardID)+"/cards", input)
}

func (c *Client) UpdateCard(ctx context.Context, cardID string, input UpdateCardInput) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodPatch, "/cards/"+url.PathEscape(cardID), input)
}

func (c *Client) MoveCard(ctx context.Context, cardID string, input MoveCardInput) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodPost, "/cards/"+url.PathEscape(cardID)+"/moves", input)
}

// SetColumnOnHold switches the area for cards on hold of a working column on
// or off and answers with the column. Switching it off hands its cards back to
// the end of the column.
func (c *Client) SetColumnOnHold(ctx context.Context, columnID string, enabled bool) (Result[Column], error) {
	body := struct {
		OnHoldEnabled bool `json:"onHoldEnabled"`
	}{enabled}
	return request[Column](ctx, c, http.MethodPut, "/columns/"+url.PathEscape(columnID)+"/on-hold", body)
}

// Subscribe puts a member of the project on the list of people a card
// notifies. Somebody on the list already succeeds as well.
func (c *Client) Subscribe(ctx context.Context, cardID, personID string) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodPost, "/cards/"+url.PathEscape(cardID)+"/subscribers/"+url.PathEscape(personID), nil)
}

// Unsubscribe takes a person off the list of people a card notifies, and keeps
// Minka from putting them back by itself. Somebody not on the list succeeds as
// well.
func (c *Client) Unsubscribe(ctx context.Context, cardID, personID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/cards/"+url.PathEscape(cardID)+"/subscribers/"+url.PathEscape(personID), nil)
	return err
}

// People lists the members of a project, the people its cards can be
// assigned to and its texts may name.
func (c *Client) People(ctx context.Context, projectID string) (Result[[]ProjectMember], error) {
	return request[[]ProjectMember](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/people", nil)
}

// ConversationPeople lists the two people of a private conversation, the ones
// its messages may name.
func (c *Client) ConversationPeople(ctx context.Context, conversationID string) (Result[[]ProjectMember], error) {
	return request[[]ProjectMember](ctx, c, http.MethodGet, "/messages/"+url.PathEscape(conversationID)+"/people", nil)
}

// AssignCard assigns a member of the project to a card. The status code of the
// result is 201 for a new assignment and 200 for one that existed already.
func (c *Client) AssignCard(ctx context.Context, cardID, personID string) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodPost, "/cards/"+url.PathEscape(cardID)+"/assignees", map[string]string{"personId": personID})
}

// UnassignCard takes a person off a card. It succeeds as well for somebody
// who is not assigned.
func (c *Client) UnassignCard(ctx context.Context, cardID, personID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/cards/"+url.PathEscape(cardID)+"/assignees/"+url.PathEscape(personID), nil)
	return err
}

// AddStep adds a step at the end of the list of a card.
// CardNotes reads the notes on the back of a card.
func (c *Client) CardNotes(ctx context.Context, cardID string) (Result[CardNotes], error) {
	return request[CardNotes](ctx, c, http.MethodGet, "/cards/"+url.PathEscape(cardID)+"/notes", nil)
}

// WriteCardNotes replaces the notes of a card as a whole. With a guard the
// server writes only over the notes the guard names and answers anything
// else with notesChanged.
func (c *Client) WriteCardNotes(ctx context.Context, cardID, body string, guard *NotesGuard) (Result[CardNotes], error) {
	fields := map[string]any{"body": body}
	if guard != nil {
		fields["expectedUpdatedAt"] = guard.UpdatedAt
	}
	return request[CardNotes](ctx, c, http.MethodPut, "/cards/"+url.PathEscape(cardID)+"/notes", fields)
}

func (c *Client) AddStep(ctx context.Context, cardID, title string) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodPost, "/cards/"+url.PathEscape(cardID)+"/steps", map[string]string{"title": title})
}

// SetStepCompleted ticks a step off or opens it again. A step that stands that
// way already succeeds as well.
func (c *Client) SetStepCompleted(ctx context.Context, cardID, stepID string, completed bool) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodPatch, "/cards/"+url.PathEscape(cardID)+"/steps/"+url.PathEscape(stepID), map[string]bool{"completed": completed})
}

// DeleteStep removes a step from a card. A step that is not on the card
// succeeds as well.
func (c *Client) DeleteStep(ctx context.Context, cardID, stepID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/cards/"+url.PathEscape(cardID)+"/steps/"+url.PathEscape(stepID), nil)
	return err
}

// Conversations lists the private conversations of the person of the token,
// the ones with the newest word first.
func (c *Client) Conversations(ctx context.Context) (Result[[]Conversation], error) {
	return request[[]Conversation](ctx, c, http.MethodGet, "/messages", nil)
}

// StartConversation begins a private conversation with somebody, or answers
// with the one these two have already. The status code is 201 for a new
// conversation and 200 for one that stood already.
func (c *Client) StartConversation(ctx context.Context, personID string) (Result[Conversation], error) {
	return request[Conversation](ctx, c, http.MethodPost, "/messages/with/"+url.PathEscape(personID), nil)
}

func (c *Client) Chats(ctx context.Context, projectID string) (Result[[]Chat], error) {
	return request[[]Chat](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/chats", nil)
}

// Messages reads messages of a chat, oldest first.
func (c *Client) Messages(ctx context.Context, chatID string, query MessagesQuery) (Result[[]ChatMessage], error) {
	values := url.Values{}
	if query.Before != nil {
		values.Set("before", strconv.Itoa(*query.Before))
	}
	if query.After != nil {
		values.Set("after", strconv.Itoa(*query.After))
	}
	if query.Limit != nil {
		values.Set("limit", strconv.Itoa(*query.Limit))
	}
	path := "/chats/" + url.PathEscape(chatID) + "/messages"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return request[[]ChatMessage](ctx, c, http.MethodGet, path, nil)
}

// PostMessage writes a message in a chat. With a client message ID, sending
// the same one again answers with status 200 and the message written before.
func (c *Client) PostMessage(ctx context.Context, chatID, body, clientMessageID string) (Result[ChatMessage], error) {
	fields := map[string]string{"body": body}
	if clientMessageID != "" {
		fields["clientMessageId"] = clientMessageID
	}
	return request[ChatMessage](ctx, c, http.MethodPost, "/chats/"+url.PathEscape(chatID)+"/messages", fields)
}

func (c *Client) EditMessage(ctx context.Context, chatID, messageID, body string) (Result[ChatMessage], error) {
	return request[ChatMessage](ctx, c, http.MethodPatch, "/chats/"+url.PathEscape(chatID)+"/messages/"+url.PathEscape(messageID), map[string]string{"body": body})
}

// DeleteMessage removes a message. Removing a removed message succeeds as well.
func (c *Client) DeleteMessage(ctx context.Context, chatID, messageID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/chats/"+url.PathEscape(chatID)+"/messages/"+url.PathEscape(messageID), nil)
	return err
}

// MarkChatRead notes how far a chat has been read. The mark only grows.
func (c *Client) MarkChatRead(ctx context.Context, chatID string, seq int) error {
	_, err := request[struct{}](ctx, c, http.MethodPost, "/chats/"+url.PathEscape(chatID)+"/read", map[string]int{"seq": seq})
	return err
}

// DeleteCard moves a card to the trash. A card already in the trash answers
// like one that never existed.
func (c *Client) DeleteCard(ctx context.Context, cardID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/cards/"+url.PathEscape(cardID), nil)
	return err
}

// RestoreCard takes a card out of the trash and puts it back on top of its
// column. A card that is not in the trash answers like one that never existed.
func (c *Client) RestoreCard(ctx context.Context, cardID string) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodPost, "/cards/"+url.PathEscape(cardID)+"/restore", nil)
}

// EditComment changes a comment of the person of the token.
func (c *Client) EditComment(ctx context.Context, cardID, commentID, body string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPatch, "/cards/"+url.PathEscape(cardID)+"/comments/"+url.PathEscape(commentID), map[string]string{"body": body})
}

// DeleteComment removes a comment of the person of the token. A removed comment
// leaves nothing behind, so removing it again answers 404.
func (c *Client) DeleteComment(ctx context.Context, cardID, commentID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/cards/"+url.PathEscape(cardID)+"/comments/"+url.PathEscape(commentID), nil)
	return err
}

func (c *Client) WriteComment(ctx context.Context, cardID string, body string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPost, "/cards/"+url.PathEscape(cardID)+"/comments", map[string]string{"body": body})
}

// ReactToComment leaves a reaction on a comment. The status code is 201 for a
// new reaction and 200 where the same one of yours stood already.
func (c *Client) ReactToComment(ctx context.Context, cardID, commentID, content string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPost,
		"/cards/"+url.PathEscape(cardID)+"/comments/"+url.PathEscape(commentID)+"/reactions", map[string]string{"content": content})
}

// UnreactFromComment takes a reaction off a comment: your own, and where you
// left none the last one somebody else left. Taking away what is not there
// succeeds as well.
func (c *Client) UnreactFromComment(ctx context.Context, cardID, commentID, content string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete,
		"/cards/"+url.PathEscape(cardID)+"/comments/"+url.PathEscape(commentID)+"/reactions?content="+url.QueryEscape(content), nil)
	return err
}

// ReactToMessage leaves a reaction on a message of a chat, 201 for a new one
// and 200 where the same one of yours stood already.
func (c *Client) ReactToMessage(ctx context.Context, chatID, messageID, content string) (Result[ChatMessage], error) {
	return request[ChatMessage](ctx, c, http.MethodPost,
		"/chats/"+url.PathEscape(chatID)+"/messages/"+url.PathEscape(messageID)+"/reactions", map[string]string{"content": content})
}

// UnreactFromMessage takes a reaction off a message, the way it comes off a
// comment.
func (c *Client) UnreactFromMessage(ctx context.Context, chatID, messageID, content string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete,
		"/chats/"+url.PathEscape(chatID)+"/messages/"+url.PathEscape(messageID)+"/reactions?content="+url.QueryEscape(content), nil)
	return err
}

// UploadToProject puts a file into a project, where it can hang in the texts of
// that project.
func (c *Client) UploadToProject(ctx context.Context, projectID, filename, contentType string, content io.Reader, size int64) (Result[Upload], error) {
	return uploadTo(ctx, c, "/projects/"+url.PathEscape(projectID)+"/uploads", filename, contentType, content, size)
}

// UploadToConversation puts a file into a private conversation, where it can
// hang in the messages of that conversation and nowhere else.
func (c *Client) UploadToConversation(ctx context.Context, conversationID, filename, contentType string, content io.Reader, size int64) (Result[Upload], error) {
	return uploadTo(ctx, c, "/messages/"+url.PathEscape(conversationID)+"/uploads", filename, contentType, content, size)
}

// uploadTo sends the bytes of a file as they are, with their media type in the
// header and the name of the file in the query. They are read while they are
// sent, so a large file is never held in memory whole.
func uploadTo(ctx context.Context, c *Client, path, filename, contentType string, content io.Reader, size int64) (Result[Upload], error) {
	return send[Upload](ctx, c, c.uploads, http.MethodPost, path+"?filename="+url.QueryEscape(filename), content, contentType, size)
}

// File says what an uploaded file is and which renderings of it exist. The
// address names the file alone: which project or conversation it went into is
// looked up by the server, and being in that circle is what decides.
func (c *Client) File(ctx context.Context, blobID string) (Result[File], error) {
	return request[File](ctx, c, http.MethodGet, "/uploads/"+url.PathEscape(blobID), nil)
}

// FileBytes opens one rendering of a file for reading. The caller closes it.
//
// The address is built here rather than taken from the answer that named the
// rendering: scheme, host and path are the client's own, so nothing a server
// writes into a field decides where the bytes are fetched from.
func (c *Client) FileBytes(ctx context.Context, blobID, variant string) (*FileBytes, error) {
	path := "/uploads/" + url.PathEscape(blobID) + "/" + url.PathEscape(variant)
	req, err := c.newRequest(ctx, http.MethodGet, path, nil, "", -1)
	if err != nil {
		return nil, err
	}
	// The answer is a file and not JSON, and this is the one address of the
	// API that says so.
	req.Header.Set("Accept", "*/*")

	resp, err := c.downloads.Do(req)
	if err != nil {
		var refusal *refusedRedirect
		if errors.As(err, &refusal) {
			return nil, &Error{
				Method:     http.MethodGet,
				StatusCode: refusal.status,
				Code:       CodeRedirectRefused,
				Message:    "The bytes were to be fetched elsewhere: " + refusal.reason + ".",
			}
		}
		return nil, &NetworkError{Method: http.MethodGet, Err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		apiErr := errorFromResponse(resp.StatusCode, raw)
		apiErr.Method = http.MethodGet
		return nil, apiErr
	}

	return &FileBytes{
		ContentType: resp.Header.Get("Content-Type"),
		ByteSize:    resp.ContentLength,
		Body:        resp.Body,
	}, nil
}

// request sends a request to a path below /api/v1. Every identifier in the
// path has to be escaped by the caller, or an identifier like "../me" would
// address another resource.
func request[T any](ctx context.Context, c *Client, method, path string, body any) (Result[T], error) {
	if body == nil {
		return send[T](ctx, c, c.http, method, path, nil, "", -1)
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		var result Result[T]
		return result, fmt.Errorf("encoding the request body: %w", err)
	}
	return send[T](ctx, c, c.http, method, path, bytes.NewReader(encoded), "application/json", int64(len(encoded)))
}

// send carries out one request. A size of less than zero means that the
// request has no body; a size of zero or more is what the server is told to
// expect, so that it can refuse a file before it reads it.
func send[T any](ctx context.Context, c *Client, httpClient *http.Client, method, path string, body io.Reader, contentType string, size int64) (Result[T], error) {
	var result Result[T]

	req, err := c.newRequest(ctx, method, path, body, contentType, size)
	if err != nil {
		return result, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return result, &NetworkError{Method: method, Err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return result, &NetworkError{Method: method, Err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := errorFromResponse(resp.StatusCode, raw)
		apiErr.Method = method
		if json.Valid(raw) {
			apiErr.Body = raw
		}
		return result, apiErr
	}

	result.StatusCode = resp.StatusCode
	result.Header = resp.Header

	// A 204 has no body, whatever its Content-Type says.
	if resp.StatusCode == http.StatusNoContent {
		return result, nil
	}

	if err := json.Unmarshal(raw, &result.Value); err != nil {
		return result, &Error{Method: method, StatusCode: resp.StatusCode, Code: "unexpectedResponse", Message: "The server answered with something that is not the expected JSON."}
	}
	result.Raw = raw

	return result, nil
}

// newRequest builds a request to a path below /api/v1, signed with the token
// of this client. A size of less than zero means that it has no body.
func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader, contentType string, size int64) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.host+"/api/v1"+path, body)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if size >= 0 {
		req.ContentLength = size
	}
	return req, nil
}

func errorFromResponse(statusCode int, raw []byte) *Error {
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &body) == nil && body.Error.Code != "" {
		return &Error{StatusCode: statusCode, Code: body.Error.Code, Message: body.Error.Message}
	}

	// Answers that do not come from the API itself, such as a redirect, Flow's
	// 403 for a missing role or a proxy error, carry no error body.
	code := "unexpectedStatus"
	switch {
	case statusCode == http.StatusUnauthorized:
		code = "unauthenticated"
	case statusCode == http.StatusForbidden:
		code = "forbidden"
	case statusCode == http.StatusNotFound:
		code = "notFound"
	case statusCode >= 300 && statusCode < 400:
		code = "unexpectedRedirect"
	case statusCode >= 500:
		code = "serverError"
	}
	message := http.StatusText(statusCode)
	if message == "" {
		message = "Unexpected answer"
	}

	return &Error{StatusCode: statusCode, Code: code, Message: message + "."}
}
