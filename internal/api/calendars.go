package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// The calendars of a project and their events. A timed event carries two
// moments in UTC, an all-day one a first and a last day; a series is one
// event with a schedule, and a span of days lists it once with the times it
// takes place at there. Nothing of it is guarded: the last write wins.

// Calendar is one calendar of a project.
type Calendar struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	AppURL string `json:"appUrl"`
}

// CalendarDetails is a calendar read by its own address, with what it shows
// beside its events: CalendarContentEventsAndDatedItems or
// CalendarContentEventsOnly.
type CalendarDetails struct {
	Tool
	Content string `json:"content"`
}

// What a calendar shows, in Minka's words.
const (
	CalendarContentEventsAndDatedItems = "eventsAndDatedItems"
	CalendarContentEventsOnly          = "eventsOnly"
)

// CalendarReference names the calendar an event stands in.
type CalendarReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// EventTime is when an event, or one occurrence of it, takes place: two
// moments for a timed one, a first and a last day for an all-day one, the
// other pair nil.
type EventTime struct {
	StartsAt *string `json:"startsAt"`
	EndsAt   *string `json:"endsAt"`
	FirstDay *string `json:"firstDay"`
	LastDay  *string `json:"lastDay"` // the first day again for one day
}

// EventLine is an event as a span of days lists it: without its description,
// and with the times it takes place at in the span. Its own time is that of
// the event, for a series its first occurrence, which may lie before the span.
type EventLine struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	AllDay    bool   `json:"allDay"`
	EventTime
	Schedule     *RepetitionSchedule `json:"schedule"` // nil for an event that does not repeat
	TimeZone     *string             `json:"timeZone"` // the zone a timed series keeps its clock time in
	JoinURL      *string             `json:"joinUrl"`
	Highlighted  bool                `json:"highlighted"`
	Participants []PersonReference   `json:"participants"`
	CommentCount int                 `json:"commentCount"`
	Occurrences  []EventTime         `json:"occurrences"` // in the order they start, never empty
	AppURL       string              `json:"appUrl"`
}

// EventSpan is every event with an occurrence between two days, both
// included, and beside them the to-dos and cards of the project due on those
// days where the calendar's Content shows them. The days are those of the
// person of the token, in TimeZone.
type EventSpan struct {
	From       string      `json:"from"`
	To         string      `json:"to"`
	TimeZone   string      `json:"timeZone"`
	Content    string      `json:"content"`
	Events     []EventLine `json:"events"`
	DatedItems []DatedItem `json:"datedItems"`
}

// DatedItem is something due on a day that a calendar shows beside its
// events. Kind is "todo", "card" or, on My calendar only, "step"; ToolID is
// the To-dos tool or the board, CardID the card of a step. PlaceTitle names
// the list of a to-do, the board of a card, the card of a step. Project is
// there on My calendar only.
type DatedItem struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	ProjectID  string            `json:"projectId"`
	ToolID     string            `json:"toolId"`
	CardID     *string           `json:"cardId"`
	Title      string            `json:"title"`
	DueOn      string            `json:"dueOn"`
	PlaceTitle string            `json:"placeTitle"`
	AppURL     string            `json:"appUrl"`
	Project    *ProjectReference `json:"project,omitempty"`
}

// MyEventLine is an event of My calendar, which gathers several calendars and
// so names its calendar and project.
type MyEventLine struct {
	EventLine
	ToolID  string           `json:"toolId"`
	Project ProjectReference `json:"project"`
}

// MyCalendar is what is on the calendar of the person of the token between
// two days, both included, across their projects that are not archived: the
// events they take part in, and the to-dos, cards and steps assigned to them
// that are due.
type MyCalendar struct {
	From       string        `json:"from"`
	To         string        `json:"to"`
	TimeZone   string        `json:"timeZone"`
	Events     []MyEventLine `json:"events"`
	DatedItems []DatedItem   `json:"datedItems"`
}

// Event is one event read by itself.
type Event struct {
	ID        string            `json:"id"`
	ProjectID string            `json:"projectId"`
	ToolID    string            `json:"toolId"`
	Project   ProjectReference  `json:"project"`
	Tool      CalendarReference `json:"tool"`
	Title     string            `json:"title"`
	AllDay    bool              `json:"allDay"`
	EventTime
	Schedule          *RepetitionSchedule `json:"schedule"`
	TimeZone          *string             `json:"timeZone"`
	Description       string              `json:"description"`       // the stored HTML
	DescriptionSource string              `json:"descriptionSource"` // the description as the editor opens it
	Attachments       []Attachment        `json:"attachments"`
	JoinURL           *string             `json:"joinUrl"`
	Highlighted       bool                `json:"highlighted"`
	Participants      []PersonReference   `json:"participants"` // in the order they were put on
	CommentCount      int                 `json:"commentCount"`
	CreatedAt         string              `json:"createdAt"`
	Creator           PersonReference     `json:"creator"`
	AppURL            string              `json:"appUrl"`
}

// EventInput is what a write says about an event; only what is set goes
// out. The time is either StartsAt and EndsAt or FirstDay with LastDay, and a
// change that names any of the four names the whole time anew. An empty
// JoinURL takes the link off. Schedule makes the event repeat; on a change,
// StopsRepeating sends it as null instead, which stops the repetition.
// TimeZone is read only together with a schedule.
type EventInput struct {
	Title          *string
	StartsAt       *string
	EndsAt         *string
	FirstDay       *string
	LastDay        *string
	Description    *string
	JoinURL        *string
	Highlighted    *bool
	ParticipantIDs *[]string
	Schedule       *ScheduleInput
	StopsRepeating bool
	TimeZone       *string
}

func (in EventInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	for name, value := range map[string]*string{
		"title": in.Title, "startsAt": in.StartsAt, "endsAt": in.EndsAt, "firstDay": in.FirstDay, "lastDay": in.LastDay,
		"description": in.Description, "joinUrl": in.JoinURL, "timeZone": in.TimeZone,
	} {
		if value != nil {
			fields[name] = *value
		}
	}
	if in.Highlighted != nil {
		fields["highlighted"] = *in.Highlighted
	}
	if in.ParticipantIDs != nil {
		fields["participantIds"] = append([]string{}, *in.ParticipantIDs...)
	}
	switch {
	case in.StopsRepeating:
		fields["schedule"] = nil
	case in.Schedule != nil:
		fields["schedule"] = *in.Schedule
	}
	return json.Marshal(fields)
}

func calendarPath(toolID string) string {
	return "/calendars/" + url.PathEscape(toolID)
}

func eventPath(eventID string) string {
	return "/events/" + url.PathEscape(eventID)
}

// Calendars lists the calendars of a project that are in use, in the order
// of the project page.
func (c *Client) Calendars(ctx context.Context, projectID string) (Result[[]Calendar], error) {
	return request[[]Calendar](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/calendars", nil)
}

// Calendar reads a calendar by its identifier alone, with its project.
func (c *Client) Calendar(ctx context.Context, toolID string) (Result[CalendarDetails], error) {
	return request[CalendarDetails](ctx, c, http.MethodGet, calendarPath(toolID), nil)
}

// ChooseCalendarContent sets what a calendar shows beside its events, for
// everybody who reads it.
func (c *Client) ChooseCalendarContent(ctx context.Context, toolID, content string) (Result[CalendarDetails], error) {
	return request[CalendarDetails](ctx, c, http.MethodPatch, calendarPath(toolID), map[string]string{"content": content})
}

// Events lists the events of a calendar between two days, both included.
func (c *Client) Events(ctx context.Context, toolID, from, to string) (Result[EventSpan], error) {
	return request[EventSpan](ctx, c, http.MethodGet, calendarPath(toolID)+"/events?"+spanQuery(from, to), nil)
}

// MyEvents reads My calendar between two days, both included.
func (c *Client) MyEvents(ctx context.Context, from, to string) (Result[MyCalendar], error) {
	return request[MyCalendar](ctx, c, http.MethodGet, "/me/events?"+spanQuery(from, to), nil)
}

func spanQuery(from, to string) string {
	values := url.Values{}
	values.Set("from", from)
	values.Set("to", to)
	return values.Encode()
}

func (c *Client) Event(ctx context.Context, eventID string) (Result[Event], error) {
	return request[Event](ctx, c, http.MethodGet, eventPath(eventID), nil)
}

func (c *Client) CreateEvent(ctx context.Context, toolID string, input EventInput) (Result[Event], error) {
	return request[Event](ctx, c, http.MethodPost, calendarPath(toolID)+"/events", input)
}

// UpdateEvent changes what the input names; the rest stays.
func (c *Client) UpdateEvent(ctx context.Context, eventID string, input EventInput) (Result[Event], error) {
	return request[Event](ctx, c, http.MethodPatch, eventPath(eventID), input)
}

// DeleteEvent puts an event into the trash, a series with all its days.
func (c *Client) DeleteEvent(ctx context.Context, eventID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, eventPath(eventID), nil)
	return err
}
