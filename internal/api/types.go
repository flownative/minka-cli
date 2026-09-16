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
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	HasDescription     bool              `json:"hasDescription"`
	CommentCount       int               `json:"commentCount"`
	StepCount          int               `json:"stepCount"` // the steps themselves come only with the single card
	CompletedStepCount int               `json:"completedStepCount"`
	CreatedAt          string            `json:"createdAt"`
	Creator            PersonReference   `json:"creator"`
	CompletedAt        *string           `json:"completedAt"`
	CompletedBy        *PersonReference  `json:"completedBy"`
	Assignees          []PersonReference `json:"assignees"` // in the order they were assigned
	DueOn              *string           `json:"dueOn"`     // the day it is due as YYYY-MM-DD, nil for a card without one
	AppURL             string            `json:"appUrl"`
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
	ID          string           `json:"id"`
	ProjectID   string           `json:"projectId"`
	BoardID     string           `json:"boardId"`
	Project     ProjectReference `json:"project"`
	Board       BoardReference   `json:"board"`
	Title       string           `json:"title"`
	Description string           `json:"description"` // the stored HTML, empty when the card has none
	// DescriptionSource is the description as the editor opens it: attachments
	// and mentions freshly signed. Sending it back keeps them.
	DescriptionSource string            `json:"descriptionSource"`
	Column            ColumnReference   `json:"column"`
	CommentCount      int               `json:"commentCount"`
	CreatedAt         string            `json:"createdAt"`
	Creator           PersonReference   `json:"creator"`
	CompletedAt       *string           `json:"completedAt"`
	CompletedBy       *PersonReference  `json:"completedBy"`
	Assignees         []PersonReference `json:"assignees"`   // in the order they were assigned
	DueOn             *string           `json:"dueOn"`       // the day it is due as YYYY-MM-DD, nil for a card without one
	Steps             []Step            `json:"steps"`       // in the order the card shows them; only a single card carries them
	Subscribers       []PersonReference `json:"subscribers"` // everybody the card notifies, by name; only a single card carries them
	Comments          []Comment         `json:"comments"`
	AppURL            string            `json:"appUrl"`
}

type Step struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

type Comment struct {
	ID         string          `json:"id"`
	Author     PersonReference `json:"author"`
	Body       string          `json:"body"`       // the stored HTML; plain text for comments written before the editor
	BodySource string          `json:"bodySource"` // the body as the editor opens it
	CreatedAt  string          `json:"createdAt"`
	EditedAt   *string         `json:"editedAt"`
	Reactions  []Reaction      `json:"reactions"`
}

// Reaction is what people said about a comment or a message, the same content
// counted together, in the order it was first left in.
type Reaction struct {
	Content string            `json:"content"` // mostly an emoji, at most 20 characters
	People  []PersonReference `json:"people"`
}

type Chat struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	UnreadCount int    `json:"unreadCount"` // counts own messages, never removed ones
	AppURL      string `json:"appUrl"`
}

// ProjectReference and BoardReference say where a card hangs, which matters
// for a list that crosses projects.
type ProjectReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BoardReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Conversation is a private talk between two people. It carries no project,
// and its ID addresses the same endpoints a chat of a project does.
type Conversation struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"` // the other person, which is what a conversation is called
	Counterpart     PersonReference `json:"counterpart"`
	LastMessageText *string         `json:"lastMessageText"` // the beginning of the last word, null while none was said
	LastMessageAt   *string         `json:"lastMessageAt"`
	UnreadCount     int             `json:"unreadCount"`
	AppURL          string          `json:"appUrl"`
}

type ChatMessage struct {
	ID         string          `json:"id"`
	Seq        int             `json:"seq"`       // the place in the transcript, which never changes
	ChangeSeq  int             `json:"changeSeq"` // raised with every change of the message
	Author     PersonReference `json:"author"`
	BodyHTML   string          `json:"bodyHtml"`   // empty for a removed message
	BodyText   string          `json:"bodyText"`   // empty for a removed message
	BodySource string          `json:"bodySource"` // the body as the editor opens it, empty for a removed message
	CreatedAt  string          `json:"createdAt"`
	EditedAt   *string         `json:"editedAt"`
	Removed    bool            `json:"removed"` // true only in an answer to After: the message is gone
	Reactions  []Reaction      `json:"reactions"`
}

// MessagesQuery chooses the messages of a chat. Without Before and After it is
// the end of the transcript.
type MessagesQuery struct {
	Before *int // messages with a seq below this one
	After  *int // messages that changed after this changeSeq
	Limit  *int // at most this many, the server allows up to 200
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

// Upload is a file that was put into a project. AttachmentSource is the one
// thing a client cannot build for itself: the element that hangs the file in a
// text, with a handle signed by the server. Put into a descriptionSource or a
// bodySource unchanged, it makes the file part of that text.
type Upload struct {
	ID               string `json:"id"`
	Filename         string `json:"filename"`
	ContentType      string `json:"contentType"`
	Size             int64  `json:"size"`
	Kind             string `json:"kind"` // "image" is shown where it stands, "file" is handed over
	AppURL           string `json:"appUrl"`
	AttachmentSource string `json:"attachmentSource"`
}
