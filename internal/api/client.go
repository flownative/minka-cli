// Package api is a client for the JSON API of Minka under /api/v1.
package api

import (
	"bytes"
	"context"
	"encoding/json"
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

// uploadTimeout is what sending the bytes of a file may take. It is far above
// the timeout of the other requests, which carry nothing but JSON.
const uploadTimeout = 10 * time.Minute

// Client talks to one Minka host with one access token.
type Client struct {
	host      string
	token     string
	userAgent string
	http      *http.Client
	uploads   *http.Client
}

// New returns a client for a host such as "https://minka.localbeach.net".
//
// The client never follows a redirect. A redirect would carry the token to an
// address the user did not name, and the API has no reason to send one.
func New(host, token, userAgent string) *Client {
	noRedirect := func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{
		host:      strings.TrimRight(host, "/"),
		token:     token,
		userAgent: userAgent,
		http: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: noRedirect,
		},
		uploads: &http.Client{
			Timeout:       uploadTimeout,
			CheckRedirect: noRedirect,
		},
	}
}

// Result is a decoded resource together with the JSON the server sent for it.
// The raw JSON is what the CLI prints for --json, so that fields the server
// adds reach a client without a change here.
type Result[T any] struct {
	Value      T
	Raw        json.RawMessage
	StatusCode int
}

// Error is an answer of the server that is not a success.
type Error struct {
	Method     string
	StatusCode int
	Code       string // the code of the error body, or one derived from the status
	Message    string
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

func (c *Client) Projects(ctx context.Context) (Result[[]Project], error) {
	return request[[]Project](ctx, c, http.MethodGet, "/projects", nil)
}

func (c *Client) Boards(ctx context.Context, projectID string) (Result[[]BoardListItem], error) {
	return request[[]BoardListItem](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/boards", nil)
}

func (c *Client) Board(ctx context.Context, boardID string) (Result[Board], error) {
	return request[Board](ctx, c, http.MethodGet, "/boards/"+url.PathEscape(boardID), nil)
}

// MyCards lists the cards the person of the token is responsible for, across
// the projects they are a member of. Cards in Done are not among them.
func (c *Client) MyCards(ctx context.Context) (Result[[]Card], error) {
	return request[[]Card](ctx, c, http.MethodGet, "/me/cards", nil)
}

func (c *Client) Card(ctx context.Context, cardID string) (Result[Card], error) {
	return request[Card](ctx, c, http.MethodGet, "/cards/"+url.PathEscape(cardID), nil)
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

	req, err := http.NewRequestWithContext(ctx, method, c.host+"/api/v1"+path, body)
	if err != nil {
		return result, fmt.Errorf("building the request: %w", err)
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
		return result, apiErr
	}

	result.StatusCode = resp.StatusCode

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
