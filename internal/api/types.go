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
	// Coordinator is the reader's own entry as coordinator of the board, nil
	// where they hold none or it has expired. Other people's are never named.
	Coordinator *Coordinator `json:"coordinator"`
	// Conventions say how the team works on the board, nil where nobody has
	// written any yet; a client of the project is not sent the field.
	Conventions *Conventions `json:"conventions"`
}

// Column roles say what a column is for in the way a team works. Triage
// carries the inbox and Done done by type; Not now carries none.
const (
	RoleInbox              = "inbox"
	RoleReady              = "ready"
	RoleInProgress         = "in_progress"
	RoleWaitingForDecision = "waiting_for_decision"
	RoleDone               = "done"
)

// Conventions are the version in force of how a team works on a board, a
// Markdown text written only by a human owner or admin of the organization.
type Conventions struct {
	Text      string `json:"text"` // Markdown, "" for an emptied version
	UpdatedAt string `json:"updatedAt"`
	UpdatedBy struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Kind string `json:"kind"` // human, agent or bot
	} `json:"updatedBy"`
	Version int `json:"version"` // counted from 1
}

// Column is one column of a board. A working column can have an area for the
// cards on hold: they stay in the column, apart from the rest and in an order
// of their own.
type Column struct {
	ID              string      `json:"id"`
	Type            string      `json:"type"`
	Title           string      `json:"title"`
	CustomTitle     *string     `json:"customTitle"`
	Color           *string     `json:"color"`     // nil for Triage and Not now; Done is always green
	CardCount       int         `json:"cardCount"` // the cards of the column without the ones on hold; Done lists only the ones completed last
	Cards           []BoardCard `json:"cards"`     // without the cards on hold
	OnHoldEnabled   bool        `json:"onHoldEnabled"`
	OnHoldCardCount int         `json:"onHoldCardCount"`
	OnHoldCards     []BoardCard `json:"onHoldCards"` // in the order they stand on hold; empty where the area is switched off
	Role            *string     `json:"role"`        // one of the Role constants, nil for none and from a Minka without roles
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
	Labels             []Label           `json:"labels"`    // the board's labels first, then the project's; nil from a Minka without labels
	AppURL             string            `json:"appUrl"`
	ShortURL           *string           `json:"shortUrl"` // see Card.ShortURL
}

// TrashEntry is one thing that was taken out of a project, of whichever kind.
// A card and a folder, document or upload of Docs & Files can be put back
// through the API, and only while nothing they hung in is out of use.
type TrashEntry struct {
	// Kind is what the thing is: "tool", "column", "card", "post", "todoList",
	// "todo", "todoGroup", "checkinQuestion", "checkinAnswer", "forward",
	// "folder", "document" or "upload". Minka may name further kinds.
	Kind     string  `json:"kind"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	ToolType *string `json:"toolType"` // "board" or "chat" for a tool, null for the rest
	// Board is the board a card or a column lay on. For every other kind it is
	// the tool the thing belonged to, which is no board.
	Board      *BoardReference  `json:"board"`
	Column     *ColumnReference `json:"column"` // only a card has one
	TrashedAt  *string          `json:"trashedAt"`
	TrashedBy  *PersonReference `json:"trashedBy"`
	Restorable bool             `json:"restorable"`
	// BlockedBy is what has to come back first: "board", "column", "tool",
	// "todoList", "todoGroup", "question", "folder" or "archivedFolder", and
	// null where nothing blocks. Minka may name further ones.
	BlockedBy *string `json:"blockedBy"`
	AppURL    string  `json:"appUrl"`
}

// Kinds of a trash entry that lie on a board. Everything else in the trash
// belongs to a tool of another sort, which the same field carries.
const (
	TrashCard   = "card"
	TrashColumn = "column"
)

// ArchivedCard is one card in the archive of a board, as the archive lists it:
// what it is, and when and by whom it was put away. The card itself is read at
// its own address and carries everything else.
type ArchivedCard struct {
	ID         string           `json:"id"`
	ProjectID  string           `json:"projectId"`
	ShortID    *string          `json:"shortId"` // such as "MNK-1A", nil where the board shows no short identifiers
	Title      string           `json:"title"`
	ArchivedAt *string          `json:"archivedAt"` // nil for a card put away before the moment was noted
	ArchivedBy *PersonReference `json:"archivedBy"` // nil where nobody is known
	AppURL     string           `json:"appUrl"`
	ShortURL   *string          `json:"shortUrl"` // see Card.ShortURL
}

// LeftOutCard is a card the host did not take into the set an agent says it is
// at work on, together with the word it gives for why. The host may learn
// further words, so a client acts on the ones it knows and passes the rest on.
type LeftOutCard struct {
	CardID string `json:"cardId"`
	Reason string `json:"reason"`
}

// ReasonCardArchived says the card lies in the archive of its board, where
// nobody works on it.
const ReasonCardArchived = "cardArchived"

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
	Labels       []Label           `json:"labels"` // the board's labels first, then the project's; nil from a Minka without labels
	AppURL       string            `json:"appUrl"`
	// ShortURL is the card's short address, such as
	// "https://<host>/go/card/MNK-1A", which a renamed prefix keeps leading to.
	// It is nil where ShortID is, and nil from a Minka that does not hand it
	// out yet.
	ShortURL *string `json:"shortUrl"`
	// StepOf are the cards this card is a step of, those the reader sees.
	StepOf []LinkedCard `json:"stepOf"`
}

type Step struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`     // of a card step, the title of its card, empty where the reader cannot see it
	Completed bool              `json:"completed"` // of a card step, whether its card is in Done
	Assignees []PersonReference `json:"assignees"` // in the order they were assigned, somebody who left the project included
	DueOn     *string           `json:"dueOn"`     // the day it is due as YYYY-MM-DD, nil for a step without one
	// Card is the card this step is. It is nil for a step in words and from a
	// Minka that knows no card steps; a card step has no people and no day of
	// its own.
	Card *LinkedCard `json:"card"`
}

// LinkedCard is a card at either end of a card step. One the reader may not
// see carries its kind alone, and its ID is empty.
type LinkedCard struct {
	Kind        string            `json:"kind"`
	ID          string            `json:"id"`
	ShortID     *string           `json:"shortId"` // nil where the board shows no short identifiers
	Title       string            `json:"title"`
	ProjectID   string            `json:"projectId"`
	BoardID     string            `json:"boardId"`
	Column      ColumnReference   `json:"column"`
	ColumnColor *string           `json:"columnColor"`
	Assignees   []PersonReference `json:"assignees"` // empty for a card in the archive
	AppURL      string            `json:"appUrl"`
}

// Seen says whether the reader may open the card.
func (c LinkedCard) Seen() bool {
	return c.ID != ""
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
	// AppURL opens the page of the comment's item scrolled to the comment; a
	// Minka that does not hand it out leaves it empty.
	AppURL string `json:"appUrl"`
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

// ChatOfAProject is a chat read by its identifier alone, with the project it
// stands in. It is named like a line of the chats of a project, not titled
// like a tool.
type ChatOfAProject struct {
	ID        string           `json:"id"`
	ProjectID string           `json:"projectId"`
	Project   ProjectReference `json:"project"`
	Name      string           `json:"name"`
	AppURL    string           `json:"appUrl"`
}

// Tool is a tool read by its identifier alone, so that whoever holds only the
// tool learns the project it stands in.
type Tool struct {
	ID        string           `json:"id"`
	ProjectID string           `json:"projectId"`
	Project   ProjectReference `json:"project"`
	Title     string           `json:"title"`
	AppURL    string           `json:"appUrl"`
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
	ReplyTo     *string         `json:"replyTo"` // the message of the same chat this one answers, nil for most
}

// MessagesQuery chooses the messages of a chat. Without Before and After it is
// the end of the transcript.
type MessagesQuery struct {
	Before *int // messages said in front of the one with this seq
	After  *int // messages that changed after this changeSeq
	Limit  *int // at most this many, the server allows up to 200
}

// CreateCardInput creates a card with its description, due date, people and
// labels in one transaction.
type CreateCardInput struct {
	Title       string   `json:"title"`
	ColumnID    *string  `json:"columnId,omitempty"`    // nil lands the card in Triage
	Description *string  `json:"description,omitempty"` // HTML
	DueOn       *string  `json:"dueOn,omitempty"`       // YYYY-MM-DD
	AssigneeIDs []string `json:"assigneeIds,omitempty"` // members of the project
	LabelIDs    []string `json:"labelIds,omitempty"`    // labels of the board or its project, one per scope
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
