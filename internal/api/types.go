package api

import "io"

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
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	EmailAddress     string           `json:"emailAddress"`
	Kind             string           `json:"kind"`    // "human", "agent" or "bot"
	Retired          bool             `json:"retired"` // an agent that stepped down; its tokens are gone with it
	OrganizationRole *string          `json:"organizationRole"`
	Principal        *PersonReference `json:"principal"` // the human an agent acts for, nil for anybody else
}

type PersonReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"` // "active" or "archived"
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

// Column is one column of a board. A working column can have an area for the
// cards on hold: they stay in the column, apart from the rest and in an order
// of their own.
type Column struct {
	ID              string      `json:"id"`
	Type            string      `json:"type"`
	Title           string      `json:"title"`
	CustomTitle     *string     `json:"customTitle"`
	CardCount       int         `json:"cardCount"` // the cards of the column without the ones on hold; Done lists only the ones completed last
	Cards           []BoardCard `json:"cards"`     // without the cards on hold
	OnHoldEnabled   bool        `json:"onHoldEnabled"`
	OnHoldCardCount int         `json:"onHoldCardCount"`
	OnHoldCards     []BoardCard `json:"onHoldCards"` // in the order they stand on hold; empty where the area is switched off
}

type BoardCard struct {
	ID                 string            `json:"id"`
	ShortID            *string           `json:"shortId"` // such as "MNK-1A", nil where the board shows no short identifiers
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

// TrashEntry is one thing that was taken out of a project. Only a card can be
// put back, and only while neither its column nor its board is in the trash.
type TrashEntry struct {
	Kind       string           `json:"kind"` // "card", "column" or "tool"
	ID         string           `json:"id"`
	Title      string           `json:"title"`
	ToolType   *string          `json:"toolType"` // "board" or "chat" for a tool, null for the rest
	Board      *BoardReference  `json:"board"`
	Column     *ColumnReference `json:"column"`
	TrashedAt  *string          `json:"trashedAt"`
	TrashedBy  *PersonReference `json:"trashedBy"`
	Restorable bool             `json:"restorable"`
	BlockedBy  *string          `json:"blockedBy"` // "board" or "column", null where nothing blocks
	AppURL     string           `json:"appUrl"`
}

// ProjectMember is somebody a circle has: a member of a project or one of the
// two of a private conversation. Whom a card can be assigned to and whom a text
// may name are the same list, so the mention rides along.
type ProjectMember struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"` // the e-mail address until the person gives a name
	EmailAddress string           `json:"emailAddress"`
	Kind         string           `json:"kind"`      // "human", "agent" or "bot"
	Principal    *PersonReference `json:"principal"` // the human an agent acts for, nil for anybody else
	Mention      Mention          `json:"mention"`
}

// Mention is how a text names a person: the handle the server signed, and the
// markup carrying it. The markup holds in the circle it was asked for and
// nowhere else, and goes into a descriptionSource or a bodySource unchanged.
type Mention struct {
	Handle string `json:"handle"`
	Markup string `json:"markup"`
}

type ColumnReference struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	CustomTitle *string `json:"customTitle"`
}

type Card struct {
	ID          string           `json:"id"`
	ShortID     *string          `json:"shortId"` // such as "MNK-1A", nil where the board shows no short identifiers
	ProjectID   string           `json:"projectId"`
	BoardID     string           `json:"boardId"`
	Project     ProjectReference `json:"project"`
	Board       BoardReference   `json:"board"`
	Title       string           `json:"title"`
	Description string           `json:"description"` // the stored HTML, empty when the card has none
	// DescriptionSource is the description as the editor opens it: attachments
	// and mentions freshly signed. Sending it back keeps them.
	DescriptionSource string `json:"descriptionSource"`
	// Attachments are the files hanging in the description, and in no comment
	// of the card.
	Attachments []Attachment `json:"attachments"`
	// Column is where the card lies, and for a card in the archive of its board
	// the column it was taken from, which may be gone from the board by now.
	Column ColumnReference `json:"column"`
	// Archived says the card lies in the archive of its board: it stands in no
	// column, it is readable, and every write on it is refused.
	Archived     bool              `json:"archived"`
	OnHold       bool              `json:"onHold"` // waiting in the area on hold of its column
	CommentCount int               `json:"commentCount"`
	CreatedAt    string            `json:"createdAt"`
	Creator      PersonReference   `json:"creator"`
	CompletedAt  *string           `json:"completedAt"`
	CompletedBy  *PersonReference  `json:"completedBy"`
	Assignees    []PersonReference `json:"assignees"`   // in the order they were assigned
	DueOn        *string           `json:"dueOn"`       // the day it is due as YYYY-MM-DD, nil for a card without one
	Steps        []Step            `json:"steps"`       // in the order the card shows them; only a single card carries them
	Subscribers  []PersonReference `json:"subscribers"` // everybody the card notifies, by name; only a single card carries them
	Comments     []Comment         `json:"comments"`
	AppURL       string            `json:"appUrl"`
}

type Step struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

type Comment struct {
	ID          string          `json:"id"`
	Author      PersonReference `json:"author"`
	Body        string          `json:"body"`        // the stored HTML; plain text for comments written before the editor
	BodySource  string          `json:"bodySource"`  // the body as the editor opens it
	Attachments []Attachment    `json:"attachments"` // the files hanging in the body
	CreatedAt   string          `json:"createdAt"`
	EditedAt    *string         `json:"editedAt"`
	Reactions   []Reaction      `json:"reactions"`
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
	UnreadCount int    `json:"unreadCount"` // never counts own messages, removed ones or imported lines
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

// ChatMessage is one message of a chat. Seq names it within its chat and is
// what a read mark and a Before cursor are given in. It is not the place of the
// message: a list is ordered by CreatedAt, so the numbers down it need not
// rise, and some are missing.
type ChatMessage struct {
	ID          string          `json:"id"`
	Seq         int             `json:"seq"`
	ChangeSeq   int             `json:"changeSeq"` // raised with every change of the message
	Author      PersonReference `json:"author"`
	BodyHTML    string          `json:"bodyHtml"`    // empty for a removed message
	BodyText    string          `json:"bodyText"`    // empty for a removed message
	BodySource  string          `json:"bodySource"`  // the body as the editor opens it, empty for a removed message
	Attachments []Attachment    `json:"attachments"` // the files hanging in the body
	CreatedAt   string          `json:"createdAt"`
	EditedAt    *string         `json:"editedAt"`
	Removed     bool            `json:"removed"` // true only in an answer to After: the message is gone
	Reactions   []Reaction      `json:"reactions"`
}

// MessagesQuery chooses the messages of a chat. Without Before and After it is
// the end of the transcript.
type MessagesQuery struct {
	Before *int // messages said in front of the one with this seq
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
// both the card goes to the top of the column. The place is one in the area
// the move names: without OnHold a card on hold comes off hold.
type MoveCardInput struct {
	ColumnID       string `json:"columnId"`
	Position       string `json:"position,omitempty"` // "top" or "bottom"
	PreviousCardID string `json:"previousCardId,omitempty"`
	NextCardID     string `json:"nextCardId,omitempty"`
	OnHold         bool   `json:"onHold,omitempty"` // into the area on hold, which the column must have
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

// File is an uploaded file as the API describes it: what it is, and which
// renderings of it can be fetched. ContentType and ByteSize are those of the
// file as it was uploaded; every rendering says its own.
type File struct {
	ID          string        `json:"id"`
	Filename    string        `json:"filename"`
	ContentType string        `json:"contentType"`
	ByteSize    int64         `json:"byteSize"`
	IsImage     bool          `json:"isImage"`
	Variants    []FileVariant `json:"variants"` // "original" first, then what else exists
}

// FileVariant is one rendering of a file. Only "original" is there for every
// file; "display" is the smaller rendering a large picture got.
type FileVariant struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	ByteSize    int64  `json:"byteSize"`
	URL         string `json:"url"` // where the bytes are, absolute
}

// Attachment is one file hanging in a text, named beside it so that a client
// that reads the text can fetch the file without taking anything apart. It is
// described as it was uploaded; URL leads to what the file is, not to its
// bytes.
type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	ByteSize    int64  `json:"byteSize"`
	IsImage     bool   `json:"isImage"`
	URL         string `json:"url"`
}

// FileBytes is one rendering of a file, open for reading. Whoever takes it
// closes it.
type FileBytes struct {
	ContentType string
	ByteSize    int64 // what the answer says it sends, -1 where it says nothing
	Body        io.ReadCloser
}

// CardNotes is the back of a card. Notes nobody has written have an empty
// body and neither a time nor an author.
type CardNotes struct {
	CardID    string           `json:"cardId"`
	Body      string           `json:"body"`
	UpdatedAt *string          `json:"updatedAt"`
	UpdatedBy *PersonReference `json:"updatedBy"`
	AppURL    string           `json:"appUrl"`
}

// NotesGuard names the notes a write may replace: the updatedAt they were
// read with, or nil for notes nobody had written yet.
type NotesGuard struct {
	UpdatedAt *string
}
