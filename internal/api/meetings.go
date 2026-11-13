package api

import (
	"context"
	"net/http"
	"net/url"
)

// Every event has a meeting: the agenda of the decisions it is to take, what
// it made of each, and whether it is closed. A series meets on each of its
// days, so its meeting is named by the event and the day; an event that does
// not repeat has one meeting and reads no day.

// Meeting is the meeting of an event on one day, as its reader is shown it.
type Meeting struct {
	EventID   string           `json:"eventId"`
	ProjectID string           `json:"projectId"`
	ToolID    string           `json:"toolId"` // the calendar
	Title     string           `json:"title"`  // the event's
	Day       *string          `json:"day"`    // the day of the series, nil for an event that does not repeat
	Cancelled bool             `json:"cancelled"`
	Closed    bool             `json:"closed"`
	ClosedAt  *string          `json:"closedAt"`
	ClosedBy  *PersonReference `json:"closedBy"` // nil while open, and where the person was purged
	MayClose  bool             `json:"mayClose"`
	Items     []AgendaItem     `json:"items"`
	AppURL    string           `json:"appUrl"`
}

// AgendaItem is a decision on the agenda. Number counts the items the reader
// is shown from 1, and Current marks the first of them without an outcome.
type AgendaItem struct {
	ID               string          `json:"id"` // the item's own, not the decision's
	Number           int             `json:"number"`
	Current          bool            `json:"current"`
	Decision         DecisionLink    `json:"decision"`
	Outcome          *MeetingOutcome `json:"outcome"` // nil until recorded
	MayMove          bool            `json:"mayMove"`
	MayTakeOff       bool            `json:"mayTakeOff"`
	MayRecordOutcome bool            `json:"mayRecordOutcome"`
}

// MeetingOutcome is what a meeting made of an item and the votes counted
// there, whatever the decision says since. Outcome is nil for a word only a
// newer Minka knows.
type MeetingOutcome struct {
	Outcome      *string          `json:"outcome"`
	Status       DecisionStatus   `json:"status"` // the status it put the decision in
	WithChanges  bool             `json:"withChanges"`
	Votes        *string          `json:"votes"` // the note
	VotesFor     *int             `json:"votesFor"`
	VotesAgainst *int             `json:"votesAgainst"`
	Abstentions  *int             `json:"abstentions"`
	RecordedBy   *PersonReference `json:"recordedBy"`
	RecordedAt   string           `json:"recordedAt"`
}

// The outcomes of an agenda item, in Minka's words.
const (
	OutcomeAccepted            = "accepted"
	OutcomeAcceptedWithChanges = "acceptedWithChanges"
	OutcomeRejected            = "rejected"
	OutcomeDeferred            = "deferred"
)

// MoveAgendaItemInput names the place of an item by one neighbour; with
// neither it goes first.
type MoveAgendaItemInput struct {
	PreviousItemID string `json:"previousItemId,omitempty"`
	NextItemID     string `json:"nextItemId,omitempty"`
}

// MeetingInput is the body of a write about a meeting: only what is set goes
// out, and a field set to nil goes out as null.
type MeetingInput map[string]any

// meetingPath is an address of a meeting. The day goes into the query and
// never into a body, where Minka might read it as well.
func meetingPath(eventID, below, day string) string {
	path := eventPath(eventID) + "/meeting" + below
	if day != "" {
		path += "?" + url.Values{"day": {day}}.Encode()
	}
	return path
}

func agendaItemPath(eventID, itemID, below, day string) string {
	return meetingPath(eventID, "/agenda/"+url.PathEscape(itemID)+below, day)
}

// Meeting reads the meeting of an event; day names the day of a series and
// is empty for an event that does not repeat.
func (c *Client) Meeting(ctx context.Context, eventID, day string) (Result[Meeting], error) {
	return request[Meeting](ctx, c, http.MethodGet, meetingPath(eventID, "", day), nil)
}

// PutOnAgenda puts decisions at the end of the agenda in the order given,
// all or none, and answers the meeting.
func (c *Client) PutOnAgenda(ctx context.Context, eventID, day string, decisionIDs []string) (Result[Meeting], error) {
	return request[Meeting](ctx, c, http.MethodPost, meetingPath(eventID, "/agenda", day), map[string][]string{"decisionIds": decisionIDs})
}

// MoveAgendaItem puts an item elsewhere on its agenda and answers the meeting.
func (c *Client) MoveAgendaItem(ctx context.Context, eventID, itemID, day string, input MoveAgendaItemInput) (Result[Meeting], error) {
	return request[Meeting](ctx, c, http.MethodPost, agendaItemPath(eventID, itemID, "/moves", day), input)
}

// TakeOffAgenda takes an item off the agenda; its decision stays as it is.
// An item that is gone is not found, a second time too.
func (c *Client) TakeOffAgenda(ctx context.Context, eventID, itemID, day string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, agendaItemPath(eventID, itemID, "", day), nil)
	return err
}

// RecordOutcome records what the meeting made of an item, or corrects it,
// and answers the meeting. The last outcome written stands.
func (c *Client) RecordOutcome(ctx context.Context, eventID, itemID, day string, input MeetingInput) (Result[Meeting], error) {
	return request[Meeting](ctx, c, http.MethodPut, agendaItemPath(eventID, itemID, "/outcome", day), input)
}

// CloseMeeting closes a meeting for good, tells its audience and answers
// the meeting.
func (c *Client) CloseMeeting(ctx context.Context, eventID, day string, input MeetingInput) (Result[Meeting], error) {
	return request[Meeting](ctx, c, http.MethodPost, meetingPath(eventID, "/close", day), input)
}
