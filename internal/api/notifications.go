package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// NotificationsWaitMax is the longest wait the list of notifications takes,
// in seconds. NotificationsHeldAtMost is the longest Minka holds a request
// by default: a longer wait is answered empty after that, below the read
// timeout of a proxy in front of it. NotificationsPageMax is the most lines
// one answer carries.
const (
	NotificationsWaitMax    = 300
	NotificationsHeldAtMost = 50
	NotificationsPageMax    = 200
)

// CodeCursorUnknown is what the list of notifications answers for a position
// it does not know, or no longer knows.
const CodeCursorUnknown = "cursorUnknown"

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
	// ShortURL is the short address of a card, as on the card itself: nil for
	// anything else, for a card without a short identifier, and from a Minka
	// that does not hand it out yet.
	ShortURL *string `json:"shortUrl"`
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

// UnreadCount is the number on the bell: the unread lines, and the unread
// messages of private conversations one by one.
type UnreadCount struct {
	UnreadCount int `json:"unreadCount"`
}

// OneNotification is a line of the bell as the push about it named it. Its ID
// is the id of the same line in the list of notifications.
type OneNotification struct {
	ID            string `json:"id"`
	Title         string `json:"title"` // plain text, the push's own sentence
	Body          string `json:"body"`  // the project, empty for a private conversation
	AppURL        string `json:"appUrl"`
	CreatedAt     string `json:"createdAt"`
	Read          bool   `json:"read"`
	ThreadID      string `json:"threadId"`      // the item, else the chat or conversation
	TimeSensitive bool   `json:"timeSensitive"` // names the person or reminds them
	UnreadCount   int    `json:"unreadCount"`
}

// NotificationsCount reads the number on the bell.
func (c *Client) NotificationsCount(ctx context.Context) (Result[UnreadCount], error) {
	return request[UnreadCount](ctx, c, http.MethodGet, "/me/notifications/count", nil)
}

// Notification reads one line of the bell of the person of the token. A line
// of somebody else, one the person may no longer see and one that is gone are
// all not found.
func (c *Client) Notification(ctx context.Context, notificationID string) (Result[OneNotification], error) {
	return request[OneNotification](ctx, c, http.MethodGet, "/notifications/"+url.PathEscape(notificationID), nil)
}

// MarkNotificationRead marks one line of the bell read and answers the number
// on the bell after it. A line already read stays as it was.
func (c *Client) MarkNotificationRead(ctx context.Context, notificationID string) (Result[UnreadCount], error) {
	return request[UnreadCount](ctx, c, http.MethodPost, "/notifications/"+url.PathEscape(notificationID)+"/read", nil)
}

// Coordinator is the entry of the session of a person that coordinates a
// board; every person has one of their own, and only they see it.
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

// ClaimCoordinator makes a session of the person of the token coordinate a
// board, or renews its entry. Another session of the same person takes the
// entry over; other people's entries play no part.
func (c *Client) ClaimCoordinator(ctx context.Context, boardID, session string) (Result[Coordinator], error) {
	return request[Coordinator](ctx, c, http.MethodPut, "/boards/"+url.PathEscape(boardID)+"/coordinator", map[string]string{"session": session})
}

// ReleaseCoordinator removes the entry of the person of the token from a
// board, and nobody else's. Without one it succeeds as well.
func (c *Client) ReleaseCoordinator(ctx context.Context, boardID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, "/boards/"+url.PathEscape(boardID)+"/coordinator", nil)
	return err
}
