package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// UpdateColumnInput changes the title of a column, its colour, its role or
// any of them. An empty Title gives Triage, Not now and Done back the name of
// their type. NoColor takes the colour away and is sent as null, which Minka
// shows as white; an empty Color is sent as it is, and Minka refuses it. Role
// is sent as it is too, and NoRole takes the role away as null.
type UpdateColumnInput struct {
	Title   *string
	Color   *string
	NoColor bool
	Role    *string
	NoRole  bool
}

func (in UpdateColumnInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if in.Title != nil {
		fields["title"] = *in.Title
	}
	if in.Color != nil {
		fields["color"] = *in.Color
	}
	if in.NoColor {
		fields["color"] = nil
	}
	if in.Role != nil {
		fields["role"] = *in.Role
	}
	if in.NoRole {
		fields["role"] = nil
	}
	return json.Marshal(fields)
}

// MoveColumnInput names where a working column goes among the working columns
// of its board: behind PreviousColumnID, or in front of NextColumnID. With
// neither it goes to the front.
type MoveColumnInput struct {
	PreviousColumnID string `json:"previousColumnId,omitempty"`
	NextColumnID     string `json:"nextColumnId,omitempty"`
}

// CreateColumn adds a working column behind the others of a board, in the
// colour the board holds least of.
func (c *Client) CreateColumn(ctx context.Context, boardID, title string) (Result[Column], error) {
	body := struct {
		Title string `json:"title"`
	}{title}
	return request[Column](ctx, c, http.MethodPost, "/boards/"+url.PathEscape(boardID)+"/columns", body)
}

// UpdateColumn renames a column, recolours it or both.
func (c *Client) UpdateColumn(ctx context.Context, columnID string, input UpdateColumnInput) (Result[Column], error) {
	return request[Column](ctx, c, http.MethodPatch, columnPath(columnID), input)
}

// MoveColumn puts a working column elsewhere among the working columns of its
// board.
func (c *Client) MoveColumn(ctx context.Context, columnID string, input MoveColumnInput) (Result[Column], error) {
	return request[Column](ctx, c, http.MethodPost, columnPath(columnID)+"/moves", input)
}

// DeleteColumn puts a working column into the trash of its project, its cards
// with it.
func (c *Client) DeleteColumn(ctx context.Context, columnID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, columnPath(columnID), nil)
	return err
}

func columnPath(columnID string) string {
	return "/columns/" + url.PathEscape(columnID)
}

// WriteConventions saves a new version of how the team works on a board. The
// text in force again writes nothing; the answer is the version in force, and
// a 204 without a body while there is none.
func (c *Client) WriteConventions(ctx context.Context, boardID, text string) (Result[Conventions], error) {
	body := struct {
		Text string `json:"text"`
	}{text}
	return request[Conventions](ctx, c, http.MethodPut, "/boards/"+url.PathEscape(boardID)+"/conventions", body)
}
