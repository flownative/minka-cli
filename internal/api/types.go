package api

// Column types as the API names them.
const (
	ColumnTriage = "triage"
	ColumnNotNow = "not_now"
	ColumnWork   = "column"
	ColumnDone   = "done"
)

type Person struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
}

type Me struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	EmailAddress     string  `json:"emailAddress"`
	OrganizationRole *string `json:"organizationRole"`
	Principal        *Person `json:"principal"` // the person an agent account acts for, nil for a person
}

type PersonReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	AppURL      string `json:"appUrl"`
}

type BoardListItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	AppURL string `json:"appUrl"`
}

type Board struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"projectId"`
	Title     string   `json:"title"`
	AppURL    string   `json:"appUrl"`
	Columns   []Column `json:"columns"`
}

type Column struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Title       string      `json:"title"`
	CustomTitle *string     `json:"customTitle"`
	CardCount   int         `json:"cardCount"` // all cards in the column; Done lists only the ones completed last
	Cards       []BoardCard `json:"cards"`
}

type BoardCard struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	HasDescription bool              `json:"hasDescription"`
	CommentCount   int               `json:"commentCount"`
	CreatedAt      string            `json:"createdAt"`
	Creator        PersonReference   `json:"creator"`
	CompletedAt    *string           `json:"completedAt"`
	CompletedBy    *PersonReference  `json:"completedBy"`
	Assignees      []PersonReference `json:"assignees"` // in the order they were assigned
	DueOn          *string           `json:"dueOn"`     // the day it is due as YYYY-MM-DD, nil for a card without one
	AppURL         string            `json:"appUrl"`
}

type ProjectMember struct {
	ID           string `json:"id"`
	Name         string `json:"name"` // the e-mail address until the person gives a name
	EmailAddress string `json:"emailAddress"`
}

type ColumnReference struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	CustomTitle *string `json:"customTitle"`
}

type Card struct {
	ID           string            `json:"id"`
	ProjectID    string            `json:"projectId"`
	BoardID      string            `json:"boardId"`
	Title        string            `json:"title"`
	Description  string            `json:"description"` // sanitized HTML, empty when the card has none
	Column       ColumnReference   `json:"column"`
	CommentCount int               `json:"commentCount"`
	CreatedAt    string            `json:"createdAt"`
	Creator      PersonReference   `json:"creator"`
	CompletedAt  *string           `json:"completedAt"`
	CompletedBy  *PersonReference  `json:"completedBy"`
	Assignees    []PersonReference `json:"assignees"` // in the order they were assigned
	DueOn        *string           `json:"dueOn"`     // the day it is due as YYYY-MM-DD, nil for a card without one
	Steps        []Step            `json:"steps"`     // in the order the card shows them; only a single card carries them
	Comments     []Comment         `json:"comments"`
	AppURL       string            `json:"appUrl"`
}

type Step struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

type Comment struct {
	ID        string          `json:"id"`
	Author    PersonReference `json:"author"`
	Body      string          `json:"body"` // plain text
	CreatedAt string          `json:"createdAt"`
	EditedAt  *string         `json:"editedAt"`
}

// CreateCardInput creates a card with its description, due date and people in
// one transaction.
type CreateCardInput struct {
	Title       string   `json:"title"`
	ColumnID    *string  `json:"columnId,omitempty"`    // nil lands the card in Triage
	Description *string  `json:"description,omitempty"` // HTML
	DueOn       *string  `json:"dueOn,omitempty"`       // YYYY-MM-DD
	AssigneeIDs []string `json:"assigneeIds,omitempty"` // members of the project
}

type UpdateCardInput struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"` // HTML; an empty string removes the description
	DueOn       *string `json:"dueOn,omitempty"`       // YYYY-MM-DD; an empty string removes the due date
}

// MoveCardInput names a column and either a position or one neighbour. Without
// both the card goes to the top of the column.
type MoveCardInput struct {
	ColumnID       string `json:"columnId"`
	Position       string `json:"position,omitempty"` // "top" or "bottom"
	PreviousCardID string `json:"previousCardId,omitempty"`
	NextCardID     string `json:"nextCardId,omitempty"`
}
