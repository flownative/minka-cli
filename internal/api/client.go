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
	"strings"
	"time"
)

// maxResponseBytes bounds what the client reads of a response. A board lists
// at most 25 cards in Done, so real responses stay far below it.
const maxResponseBytes = 16 << 20

// Client talks to one Minka host with one access token.
type Client struct {
	host      string
	token     string
	userAgent string
	http      *http.Client
}

// New returns a client for a host such as "https://minka.localbeach.net".
//
// The client never follows a redirect. A redirect would carry the token to an
// address the user did not name, and the API has no reason to send one.
func New(host, token, userAgent string) *Client {
	return &Client{
		host:      strings.TrimRight(host, "/"),
		token:     token,
		userAgent: userAgent,
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Result is a decoded resource together with the JSON the server sent for it.
// The raw JSON is what the CLI prints for --json, so that fields the server
// adds reach a client without a change here.
type Result[T any] struct {
	Value T
	Raw   json.RawMessage
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

// DeleteCard moves a card to the trash. A card already in the trash answers
// like one that never existed.
func (c *Client) DeleteCard(ctx context.Context, cardID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/cards/"+url.PathEscape(cardID), nil)
	return err
}

func (c *Client) WriteComment(ctx context.Context, cardID string, body string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPost, "/cards/"+url.PathEscape(cardID)+"/comments", map[string]string{"body": body})
}

// request sends a request to a path below /api/v1. Every identifier in the
// path has to be escaped by the caller, or an identifier like "../me" would
// address another resource.
func request[T any](ctx context.Context, c *Client, method, path string, body any) (Result[T], error) {
	var result Result[T]

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return result, fmt.Errorf("encoding the request body: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.host+"/api/v1"+path, requestBody)
	if err != nil {
		return result, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
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
