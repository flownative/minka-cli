package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Label is a label of a project, or of one of its boards or Files tools. The
// same shape stands in a list of a place, in the answer of a write and on an
// item.
type Label struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`  // tidied, such as "Priority::High"
	Scope     *string `json:"scope"` // the part before "::", nil for a plain label
	Value     string  `json:"value"` // the part after "::", the whole name for a plain label
	Color     string  `json:"color"`
	Icon      *string `json:"icon"` // a Lucide icon name, nil for none
	ProjectID string  `json:"projectId"`
	ToolID    *string `json:"toolId"`   // the board or Files tool, nil for a label of the project
	Position  int     `json:"position"` // counted from 1 within its own place
}

// LabelPlace is where labels are kept: a project, a board or a Files tool,
// as one segment of a path such as "boards/<id>".
type LabelPlace struct {
	Kind string // "projects", "boards" or "files-tools"
	ID   string
}

func (p LabelPlace) path() string {
	return "/" + p.Kind + "/" + url.PathEscape(p.ID) + "/labels"
}

// LabelledItem is a card, a document or an upload, which carry labels.
type LabelledItem struct {
	Kind string // "cards", "documents" or "files-uploads"
	ID   string
}

func (i LabelledItem) path() string {
	return "/" + i.Kind + "/" + url.PathEscape(i.ID) + "/labels"
}

// LabelInput names a label's new name, colour, icon or several of them; what
// is nil is not sent. A new label without a colour gets the one Minka
// suggests. NoIcon takes the icon off and is sent as null; an empty Icon is
// sent as it is, and Minka refuses it.
type LabelInput struct {
	Name   *string
	Color  *string
	Icon   *string
	NoIcon bool
}

func (in LabelInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if in.Name != nil {
		fields["name"] = *in.Name
	}
	if in.Color != nil {
		fields["color"] = *in.Color
	}
	if in.Icon != nil {
		fields["icon"] = *in.Icon
	}
	if in.NoIcon {
		fields["icon"] = nil
	}
	return json.Marshal(fields)
}

// MoveLabelInput names where a label goes within its place: right behind
// PreviousLabelID, or right in front of NextLabelID; with neither, to the
// front. Where both are sent, PreviousLabelID alone decides.
type MoveLabelInput struct {
	PreviousLabelID string `json:"previousLabelId,omitempty"`
	NextLabelID     string `json:"nextLabelId,omitempty"`
}

// Labels lists what the items of a place can take: a tool's own labels, then
// the project's.
func (c *Client) Labels(ctx context.Context, place LabelPlace) (Result[[]Label], error) {
	return request[[]Label](ctx, c, http.MethodGet, place.path(), nil)
}

// CreateLabel adds a label at the end of its place.
func (c *Client) CreateLabel(ctx context.Context, place LabelPlace, input LabelInput) (Result[Label], error) {
	return request[Label](ctx, c, http.MethodPost, place.path(), input)
}

// UpdateLabel renames or recolours a label of exactly this place.
func (c *Client) UpdateLabel(ctx context.Context, place LabelPlace, labelID string, input LabelInput) (Result[Label], error) {
	return request[Label](ctx, c, http.MethodPatch, place.path()+"/"+url.PathEscape(labelID), input)
}

// MoveLabel puts a label elsewhere within its place and answers with it.
func (c *Client) MoveLabel(ctx context.Context, place LabelPlace, labelID string, input MoveLabelInput) (Result[Label], error) {
	return request[Label](ctx, c, http.MethodPost, place.path()+"/"+url.PathEscape(labelID)+"/moves", input)
}

// RenameLabelScopeInput names the scope whose labels are renamed and the
// scope they take.
type RenameLabelScopeInput struct {
	Scope    string `json:"scope"`
	NewScope string `json:"newScope"`
}

// RenameLabelScope gives every label of one scope at exactly this place the
// new scope, or none of them, and answers with the place's own labels.
func (c *Client) RenameLabelScope(ctx context.Context, place LabelPlace, input RenameLabelScopeInput) (Result[[]Label], error) {
	return request[[]Label](ctx, c, http.MethodPost, place.path()+"/scope-renames", input)
}

// DeleteLabel deletes a label and takes it off every item; there is no trash.
func (c *Client) DeleteLabel(ctx context.Context, place LabelPlace, labelID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, place.path()+"/"+url.PathEscape(labelID), nil)
	return err
}

// AssignLabel puts a label on an item, named by its ID where labelID is set,
// else by its name. The answer is every label the item carries: 201 where
// the label is new on it, 200 where it carried it already.
func (c *Client) AssignLabel(ctx context.Context, item LabelledItem, labelID, name string) (Result[[]Label], error) {
	body := map[string]string{"name": name}
	if labelID != "" {
		body = map[string]string{"labelId": labelID}
	}
	return request[[]Label](ctx, c, http.MethodPost, item.path(), body)
}

// UnassignLabel takes a label off an item, named by its ID where labelID is
// set, else by its name. A label the item does not carry is no refusal.
func (c *Client) UnassignLabel(ctx context.Context, item LabelledItem, labelID, name string) error {
	path := item.path() + "?name=" + url.QueryEscape(name)
	if labelID != "" {
		path = item.path() + "/" + url.PathEscape(labelID)
	}
	_, err := request[struct{}](ctx, c, http.MethodDelete, path, nil)
	return err
}
