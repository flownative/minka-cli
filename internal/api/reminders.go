package api

import (
	"context"
	"net/http"
	"net/url"
)

// Reminder is one "Remind me" of the person of the token: of an item, of a
// comment or of a line of a project chat. Kind and ID are the address that
// moves or removes it.
type Reminder struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Title is the item's title; for a comment, a chat line and an answer the
	// writer and the start of the words.
	Title    string           `json:"title"`
	Project  ProjectReference `json:"project"`
	RemindAt string           `json:"remindAt"`
	IsDue    bool             `json:"isDue"`
	AppURL   string           `json:"appUrl"`
}

// RemindersMaxOnASide is the most reminders the server reads on either side of
// now, the due ones and those ahead, before it leaves out the ones whose
// subject the person no longer sees. There is no page beyond.
const RemindersMaxOnASide = 500

// SetReminderInput names the moment of a reminder: a preset of the web menu
// or a moment in ISO 8601 with its offset, exactly one of the two.
type SetReminderInput struct {
	Preset   string `json:"preset,omitempty"`
	RemindAt string `json:"remindAt,omitempty"`
}

func reminderPath(kind, id string) string {
	return "/me/reminders/" + url.PathEscape(kind) + "/" + url.PathEscape(id)
}

// MyReminders lists the reminders of the person of the token, the due ones
// first.
func (c *Client) MyReminders(ctx context.Context) (Result[[]Reminder], error) {
	return request[[]Reminder](ctx, c, http.MethodGet, "/me/reminders", nil)
}

// SetReminder sets a reminder, or moves the one already set, due or not, to
// the new moment.
func (c *Client) SetReminder(ctx context.Context, kind, id string, input SetReminderInput) (Result[Reminder], error) {
	return request[Reminder](ctx, c, http.MethodPut, reminderPath(kind, id), input)
}

// RemoveReminder takes a reminder away, whether or not the person still sees
// what it is of. An empty outcome leaves it to the server, which counts it as
// dismissed.
func (c *Client) RemoveReminder(ctx context.Context, kind, id, outcome string) error {
	path := reminderPath(kind, id)
	if outcome != "" {
		path += "?" + url.Values{"outcome": {outcome}}.Encode()
	}
	_, err := request[struct{}](ctx, c, http.MethodDelete, path, nil)
	return err
}
