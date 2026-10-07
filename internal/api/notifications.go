package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// NotificationsWaitMax is the longest the list of notifications holds a
// request open for, in seconds. NotificationsPageMax is the most lines one
// answer carries.
const (
	NotificationsWaitMax = 300
	NotificationsPageMax = 200
)

// CodeCursorUnknown is what the list of notifications answers for a position
// it does not know, or no longer knows.
const CodeCursorUnknown = "cursorUnknown"

// CodeCoordinatorTaken is what claiming the coordinator of a board answers
// while somebody else holds it. The answer carries the holder.
const CodeCoordinatorTaken = "coordinatorTaken"

// waitGrace is what an answer to a held request may take beyond the wait it
// asked for: the server answers at the end of the wait, and the answer still
// has to travel.
const waitGrace = 30 * time.Second

// Notification is one line of the bell of the person of the token.
type Notification struct {
	ID        string              `json:"id"`
	Cursor    string              `json:"cursor"`
	Occasion  string              `json:"occasion"`
	CreatedAt string              `json:"createdAt"`
	Read      bool                `json:"read"`
	Actor     *PersonReference    `json:"actor"`   // nil where nobody did it
	Summary   string              `json:"summary"` // one line of plain text, in the person's language
	Subject   NotificationSubject `json:"subject"`
	Card      *NotificationCard   `json:"card"` // nil for anything but a card on a board
	Project   *ProjectReference   `json:"project"`
}

// NotificationSubject is what a notification is about.
type NotificationSubject struct {
	Type    string  `json:"type"` // card, todo, post, document, message, …
	ID      string  `json:"id"`
	ShortID *string `json:"shortId"`
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	AppURL  string  `json:"appUrl"`
}

// NotificationCard is where the card of a notification stands now, which is
// not always where it stood when the notification was written.
type NotificationCard struct {
	BoardID string          `json:"boardId"`
	Column  ColumnReference `json:"column"`
}

// NotificationsPage is one answer of the list of notifications. Cursor is the
// position behind its last line, or, where it carries none, the position it
// was asked from; it is passed back unchanged to read on.
type NotificationsPage struct {
	Notifications []Notification `json:"notifications"`
	Cursor        string         `json:"cursor"`
}

// LinesOf cuts the JSON of a page into the JSON of its lines, which is what a
// reader of several pages is handed instead of one answer of the server.
func LinesOf(page Result[NotificationsPage]) ([]json.RawMessage, error) {
	var lines struct {
		Notifications []json.RawMessage `json:"notifications"`
	}
	if err := json.Unmarshal(page.Raw, &lines); err != nil {
		return nil, &Error{
			Method:     http.MethodGet,
			StatusCode: page.StatusCode,
			Code:       "unexpectedResponse",
			Message:    "The server answered the list of notifications with something that is not a list of lines.",
		}
	}
	return lines.Notifications, nil
}

// Notifications reads the lines of the bell newer than the position after,
// the oldest first, at most limit of them. Without a position it reads none
// and answers where the list stands now. Where nothing newer is there, the
// server holds the request for up to wait seconds and answers as soon as a
// line arrives; a wait counts as a sign of life of every board the person
// coordinates.
func (c *Client) Notifications(ctx context.Context, after string, wait, limit int) (Result[NotificationsPage], error) {
	values := url.Values{}
	if after != "" {
		values.Set("after", after)
	}
	values.Set("wait", strconv.Itoa(wait))
	values.Set("limit", strconv.Itoa(limit))

	ctx, cancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second+waitGrace)
	defer cancel()
	return send[NotificationsPage](ctx, c, c.waits, http.MethodGet, "/me/notifications?"+values.Encode(), nil, "", -1)
}

// Coordinator is the one session of a person that coordinates a board.
type Coordinator struct {
	Person struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Kind string `json:"kind"` // human, agent or bot
	} `json:"person"`
	Session    string `json:"session"`
	Since      string `json:"since"`
	LastSeenAt string `json:"lastSeenAt"`
	ExpiresAt  string `json:"expiresAt"`
}

// ClaimCoordinator makes a session of the person of the token the coordinator
// of a board, or renews it. Another session of the same person takes it over;
// while another person holds it, the answer is a conflict that names them.
func (c *Client) ClaimCoordinator(ctx context.Context, boardID, session string) (Result[Coordinator], error) {
	return request[Coordinator](ctx, c, http.MethodPut, "/boards/"+url.PathEscape(boardID)+"/coordinator", map[string]string{"session": session})
}

// ReleaseCoordinator takes the coordinator off a board. A board nobody
// coordinates succeeds as well.
func (c *Client) ReleaseCoordinator(ctx context.Context, boardID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/boards/"+url.PathEscape(boardID)+"/coordinator", nil)
	return err
}
