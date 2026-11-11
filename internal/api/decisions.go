package api

import (
	"context"
	"net/http"
	"net/url"
)

// The Decisions tool of a project: what was decided, in a status of the
// tool, with who decided, why, what led to it and what follows from it.

// DecisionsTool is one Decisions tool of a project.
type DecisionsTool struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	AppURL string `json:"appUrl"`
}

// DecisionStatus is one status of a Decisions tool, or the one a decision
// stands in. ID is nil for a decision written before its tool had statuses,
// which holds; Role is nil for a status the team added.
type DecisionStatus struct {
	ID         *string `json:"id"`
	Name       string  `json:"name"` // what to show; a status going by its role is named in English
	CustomName *string `json:"customName"`
	Role       *string `json:"role"`
}

// Decider is one of who decided: a member of the project, or a name given
// freely, never both.
type Decider struct {
	Person *PersonReference `json:"person"`
	Name   *string          `json:"name"`
}

// DecisionEvent is the event a decision was taken in.
type DecisionEvent struct {
	ID         string `json:"id"`
	CalendarID string `json:"calendarId"`
	Title      string `json:"title"`
	StartsOn   string `json:"startsOn"` // a series' first day
	AppURL     string `json:"appUrl"`
}

// DecisionLink is another decision of the same tool that a decision names.
type DecisionLink struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Status   DecisionStatus `json:"status"`
	Archived bool           `json:"archived"`
	AppURL   string         `json:"appUrl"`
	ShortID  *string        `json:"shortId"` // such as "ENT-14", nil before its tool numbered it
}

// DecisionStatusSetBy is who set the status a decision stands in, and when.
// With Minka true, Minka moved it, and Person is whom its line names; Agent
// is an agent that set it by hand. Person is nil once purged.
type DecisionStatusSetBy struct {
	Person *PersonReference `json:"person"`
	Agent  bool             `json:"agent"`
	Minka  bool             `json:"minka"`
	At     string           `json:"at"`
}

// DecisionSubject is the event or card a decision applies to.
type DecisionSubject struct {
	Kind      string `json:"kind"` // "event" or "card"
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	ToolID    string `json:"toolId"`
	Title     string `json:"title"`
	InTrash   bool   `json:"inTrash"`
	AppURL    string `json:"appUrl"`
}

// DecisionEntry is one entry of the history or the consequences of a
// decision: a record of the instance, a comment on one, or a link.
type DecisionEntry struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`  // a kind of record such as "card" or "todo", "comment", or "link"
	Title     string  `json:"title"` // the target's title now, or a link's description
	TargetID  *string `json:"targetId"`
	CommentID *string `json:"commentId"`
	ProjectID *string `json:"projectId"`
	AppURL    string  `json:"appUrl"`
	Done      *bool   `json:"done"`    // a consequence done; nil in the history
	ShortID   *string `json:"shortId"` // of a card or decision named, where it has one
}

// DecisionLine is one decision as the list of its tool names it.
type DecisionLine struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"projectId"`
	Title        string          `json:"title"` // the sentence that holds
	Reason       *string         `json:"reason"`
	Status       DecisionStatus  `json:"status"`
	DecidedOn    *string         `json:"decidedOn"`
	Labels       []Label         `json:"labels"`
	CommentCount int             `json:"commentCount"`
	CreatedAt    string          `json:"createdAt"`
	Creator      PersonReference `json:"creator"`
	AppURL       string          `json:"appUrl"`
	DecidedBy    []Decider       `json:"decidedBy"`
	ValidUntil   *string         `json:"validUntil"`
	ReviewOn     *string         `json:"reviewOn"`
	// StatusSetBy is nil where nobody is known to have set the status.
	StatusSetBy *DecisionStatusSetBy `json:"statusSetBy"`
	ShortID     *string              `json:"shortId"`  // such as "ENT-14", nil before its tool numbered it
	ShortURL    *string              `json:"shortUrl"` // its go/decision address, nil where ShortID is

	// AgendaNumber is its place among the decisions written down with it,
	// counted from 1; nil for a decision written down alone.
	AgendaNumber *int `json:"agendaNumber"`
}

// Decision is one decision with everything said about it.
type Decision struct {
	ID          string                 `json:"id"`
	ProjectID   string                 `json:"projectId"`
	ToolID      string                 `json:"toolId"`
	Project     ProjectReference       `json:"project"`
	Tool        DecisionsToolReference `json:"tool"`
	Title       string                 `json:"title"`
	Reason      *string                `json:"reason"`
	Status      DecisionStatus         `json:"status"`
	Body        string                 `json:"body"` // empty when nothing more was written down
	BodySource  string                 `json:"bodySource"`
	Attachments []Attachment           `json:"attachments"`
	DecidedOn   *string                `json:"decidedOn"`
	Labels      []Label                `json:"labels"`
	CreatedAt   string                 `json:"createdAt"`
	EditedAt    *string                `json:"editedAt"`
	Creator     PersonReference        `json:"creator"`
	Comments    []Comment              `json:"comments"` // oldest first
	AppURL      string                 `json:"appUrl"`
	// Archived says the decision lies in the archive of its tool: readable,
	// and not changed.
	Archived     bool             `json:"archived"`
	DecidedBy    []Decider        `json:"decidedBy"`
	DecidedIn    *DecisionEvent   `json:"decidedIn"`
	Votes        *string          `json:"votes"`
	Supersedes   *DecisionLink    `json:"supersedes"`
	Revokes      *DecisionLink    `json:"revokes"`
	SupersededBy []DecisionLink   `json:"supersededBy"`
	RevokedBy    []DecisionLink   `json:"revokedBy"`
	History      []DecisionEntry  `json:"history"`
	Consequences []DecisionEntry  `json:"consequences"`
	AppliesTo    *DecisionSubject `json:"appliesTo"`
	ValidUntil   *string          `json:"validUntil"`
	ReviewOn     *string          `json:"reviewOn"`
	// StatusSetBy is nil where nobody is known to have set the status.
	StatusSetBy *DecisionStatusSetBy `json:"statusSetBy"`
	ShortID     *string              `json:"shortId"`  // such as "ENT-14", nil before its tool numbered it
	ShortURL    *string              `json:"shortUrl"` // its go/decision address, nil where ShortID is

	// AgendaNumber is its place among the decisions written down with it,
	// counted from 1; nil for a decision written down alone.
	AgendaNumber *int `json:"agendaNumber"`
}

// DecisionsToolReference names the tool a decision stands in.
type DecisionsToolReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// DecisionVersionLine is one version in the list of a decision's versions.
type DecisionVersionLine struct {
	ID         string           `json:"id"`
	DecisionID string           `json:"decisionId"`
	Number     int              `json:"number"`
	Current    bool             `json:"current"`
	Author     *PersonReference `json:"author"`
	WrittenAt  string           `json:"writtenAt"`
}

// DecisionVersion is what a decision said in one of its versions.
type DecisionVersion struct {
	DecisionVersionLine
	Title       string         `json:"title"`
	Reason      *string        `json:"reason"`
	Body        string         `json:"body"`
	Attachments []Attachment   `json:"attachments"`
	DecidedOn   *string        `json:"decidedOn"`
	AppURL      string         `json:"appUrl"`
	DecidedBy   []Decider      `json:"decidedBy"`
	DecidedIn   *DecisionEvent `json:"decidedIn"`
	Votes       *string        `json:"votes"`
}

// DecisionsQuery narrows the list of a tool; what is empty narrows nothing.
type DecisionsQuery struct {
	Status    string // an ID or a role
	LabelID   string
	AppliesTo string // the ID of an event or a card
	// SetByAnAgent narrows to the decisions whose status an agent set by
	// hand.
	SetByAnAgent bool
}

// DecisionInput is a write of a decision by the API's names of its fields.
// A change sends only the fields it holds, each as it stands, so that nil
// goes out as null and clears the field. A body is HTML.
type DecisionInput map[string]any

// DeciderInput names one of who decided: a member by PersonID, or a free
// Name.
type DeciderInput struct {
	PersonID string `json:"personId,omitempty"`
	Name     string `json:"name,omitempty"`
}

// MoveDecisionEntryInput names where an entry goes within its part: behind
// PreviousEntryID, or in front of NextEntryID; with neither, to the top.
type MoveDecisionEntryInput struct {
	PreviousEntryID string `json:"previousEntryId,omitempty"`
	NextEntryID     string `json:"nextEntryId,omitempty"`
}

func decisionPath(decisionID string) string {
	return "/decisions/" + url.PathEscape(decisionID)
}

func decisionsToolPath(toolID string) string {
	return "/decisions-tools/" + url.PathEscape(toolID)
}

// DecisionsTools lists the Decisions tools of a project that are in use, in
// the order of the project page.
func (c *Client) DecisionsTools(ctx context.Context, projectID string) (Result[[]DecisionsTool], error) {
	return request[[]DecisionsTool](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/decisions", nil)
}

// DecisionsTool reads a Decisions tool by its identifier alone.
func (c *Client) DecisionsTool(ctx context.Context, toolID string) (Result[Tool], error) {
	return request[Tool](ctx, c, http.MethodGet, decisionsToolPath(toolID), nil)
}

// DecisionStatuses lists the statuses of a tool in their order; a client is
// offered no draft.
func (c *Client) DecisionStatuses(ctx context.Context, toolID string) (Result[[]DecisionStatus], error) {
	return request[[]DecisionStatus](ctx, c, http.MethodGet, decisionsToolPath(toolID)+"/statuses", nil)
}

// Decisions lists the decisions of a tool, the day decided on last first.
// Nothing of it is paged.
func (c *Client) Decisions(ctx context.Context, toolID string, query DecisionsQuery) (Result[[]DecisionLine], error) {
	values := url.Values{}
	if query.Status != "" {
		values.Set("status", query.Status)
	}
	if query.LabelID != "" {
		values.Set("label", query.LabelID)
	}
	if query.AppliesTo != "" {
		values.Set("appliesTo", query.AppliesTo)
	}
	if query.SetByAnAgent {
		values.Set("setBy", "agent")
	}
	path := decisionsToolPath(toolID) + "/decisions"
	if len(values) > 0 {
		path += "?" + values.Encode()
	}
	return request[[]DecisionLine](ctx, c, http.MethodGet, path, nil)
}

// Decision reads one decision, in the list or in the archive.
func (c *Client) Decision(ctx context.Context, decisionID string) (Result[Decision], error) {
	return request[Decision](ctx, c, http.MethodGet, decisionPath(decisionID), nil)
}

// DecisionByShortIdentifier reads a decision by its short identifier, such as
// "ENT-14", also under a former prefix of its tool.
func (c *Client) DecisionByShortIdentifier(ctx context.Context, identifier string) (Result[Decision], error) {
	return request[Decision](ctx, c, http.MethodGet, "/decisions/by-short-id/"+url.PathEscape(identifier), nil)
}

func (c *Client) CreateDecision(ctx context.Context, toolID string, input DecisionInput) (Result[Decision], error) {
	return request[Decision](ctx, c, http.MethodPost, decisionsToolPath(toolID)+"/decisions", map[string]any(input))
}

// DecisionsWritten are the decisions written down at once, in their order.
type DecisionsWritten struct {
	Decisions []Decision `json:"decisions"`
}

// CreateDecisions writes several decisions down at once from "titles", all
// or none, sharing the other fields of the input.
func (c *Client) CreateDecisions(ctx context.Context, toolID string, input DecisionInput) (Result[DecisionsWritten], error) {
	return request[DecisionsWritten](ctx, c, http.MethodPost, decisionsToolPath(toolID)+"/decisions/batch", map[string]any(input))
}

// UpdateDecision changes what the input names; what it leaves out stays.
func (c *Client) UpdateDecision(ctx context.Context, decisionID string, input DecisionInput) (Result[Decision], error) {
	return request[Decision](ctx, c, http.MethodPatch, decisionPath(decisionID), map[string]any(input))
}

// DeleteDecision puts a decision into the trash.
func (c *Client) DeleteDecision(ctx context.Context, decisionID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, decisionPath(decisionID), nil)
	return err
}

// DecisionVersions lists the versions of a decision, oldest first.
func (c *Client) DecisionVersions(ctx context.Context, decisionID string) (Result[[]DecisionVersionLine], error) {
	return request[[]DecisionVersionLine](ctx, c, http.MethodGet, decisionPath(decisionID)+"/versions", nil)
}

func (c *Client) DecisionVersionOf(ctx context.Context, decisionID, versionID string) (Result[DecisionVersion], error) {
	return request[DecisionVersion](ctx, c, http.MethodGet, decisionPath(decisionID)+"/versions/"+url.PathEscape(versionID), nil)
}

// AddDecisionEntry appends what an address names to the history or, with
// consequence, to the consequences. The answer is the entry, also where one
// named it there already.
func (c *Client) AddDecisionEntry(ctx context.Context, decisionID string, consequence bool, address, description string) (Result[DecisionEntry], error) {
	part := "/history"
	if consequence {
		part = "/consequences"
	}
	body := map[string]string{"address": address}
	if description != "" {
		body["description"] = description
	}
	return request[DecisionEntry](ctx, c, http.MethodPost, decisionPath(decisionID)+part, body)
}

// MoveDecisionEntry puts an entry elsewhere within its part and answers with
// the decision.
func (c *Client) MoveDecisionEntry(ctx context.Context, decisionID, entryID string, input MoveDecisionEntryInput) (Result[Decision], error) {
	return request[Decision](ctx, c, http.MethodPost, decisionPath(decisionID)+"/entries/"+url.PathEscape(entryID)+"/moves", input)
}

// DeleteDecisionEntry takes an entry off a decision; what it names stays.
func (c *Client) DeleteDecisionEntry(ctx context.Context, decisionID, entryID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, decisionPath(decisionID)+"/entries/"+url.PathEscape(entryID), nil)
	return err
}
