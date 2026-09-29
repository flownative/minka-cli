// Package bundle writes an import bundle: the directory of JSONL files and
// attached files that Minka's applier reads foreign history from.
package bundle

import "encoding/json"

// Format is the version of the bundle format a manifest announces.
const Format = 1

// Kind names one sort of record. It is the name of the JSONL file and the key
// of the counts in the manifest.
type Kind string

const (
	People         Kind = "people"
	Projects       Kind = "projects"
	Tools          Kind = "tools"
	Categories     Kind = "categories"
	Columns        Kind = "columns"
	Cards          Kind = "cards"
	Steps          Kind = "steps"
	Posts          Kind = "posts"
	Questions      Kind = "questions"
	Answers        Kind = "answers"
	TodoLists      Kind = "todolists"
	TodoGroups     Kind = "todogroups"
	Todos          Kind = "todos"
	Forwards       Kind = "forwards"
	ForwardReplies Kind = "forwardreplies"
	Folders        Kind = "folders"
	Documents      Kind = "documents"
	Uploads        Kind = "uploads"
	Messages       Kind = "messages"
	Comments       Kind = "comments"
	Reactions      Kind = "reactions"
	Attachments    Kind = "attachments"
)

// Kinds lists every kind in the order of the format, which is the order a
// report shows them in.
var Kinds = []Kind{People, Projects, Tools, Categories, Columns, Cards, Steps, Posts, Questions, Answers, TodoLists, TodoGroups, Todos,
	Forwards, ForwardReplies, Folders, Documents, Uploads, Messages, Comments, Reactions, Attachments}

const (
	ToolBoard    = "board"
	ToolChat     = "chat"
	ToolPosts    = "posts"
	ToolCheckins = "checkins"
	ToolTodos    = "todos"
	ToolForwards = "forwards"
	ToolFiles    = "files"

	OwnerAnswer       = "answer"
	OwnerCard         = "card"
	OwnerComment      = "comment"
	OwnerDocument     = "document"
	OwnerForward      = "forward"
	OwnerForwardReply = "forwardreply"
	OwnerMessage      = "message"
	OwnerPerson       = "person"
	OwnerPost         = "post"
	OwnerTodo         = "todo"
	OwnerTodoList     = "todolist"
	OwnerUpload       = "upload"

	RoleTriage = "triage"
	RoleWork   = "work"
	RoleNotNow = "notnow"
	RoleDone   = "done"

	StatusActive   = "active"
	StatusArchived = "archived"
	StatusTrashed  = "trashed"

	ParentAnswer   = "answer"
	ParentCard     = "card"
	ParentDocument = "document"
	ParentForward  = "forward"
	ParentPost     = "post"
	ParentTodo     = "todo"
	ParentTodoList = "todolist"
	ParentUpload   = "upload"

	TargetAnswer   = "answer"
	TargetCard     = "card"
	TargetComment  = "comment"
	TargetDocument = "document"
	TargetForward  = "forward"
	TargetMessage  = "message"
	TargetPost     = "post"
	TargetTodo     = "todo"
	TargetTodoList = "todolist"
	TargetUpload   = "upload"

	// The four rhythms a question can be asked in. Daily means on each of the
	// chosen weekdays.
	RhythmDaily          = "daily"
	RhythmWeekly         = "weekly"
	RhythmEveryOtherWeek = "everyOtherWeek"
	RhythmMonthly        = "monthly"
)

// Company is whom a client belongs to at the source. Neither part is ever
// empty: Minka ends a run over a company without an identifier or without a
// name.
type Company struct {
	SourceID string `json:"sourceId"`
	Name     string `json:"name"`
}

// CompanyOrNone is what a record says about a company: which one it is, or that
// the source names none. Minka reads a field that is missing and one that says
// none the same way and drops the company it holds for the record, so both say
// that there is no company and neither leaves one standing.
type CompanyOrNone struct {
	Company *Company
}

// TheCompany is the company a record names, NoCompany the word that it names
// none.
func TheCompany(sourceID, name string) *CompanyOrNone {
	return &CompanyOrNone{Company: &Company{SourceID: sourceID, Name: name}}
}

func NoCompany() *CompanyOrNone {
	return &CompanyOrNone{}
}

func (c CompanyOrNone) MarshalJSON() ([]byte, error) {
	if c.Company == nil {
		return []byte("null"), nil
	}
	return json.Marshal(c.Company)
}

// Record is what every record carries. CreatedBy is nil when the source does
// not say who created it.
type Record struct {
	SourceID  string  `json:"sourceId"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	CreatedBy *string `json:"createdBy"`
}

// Person is somebody of the source. Client is said either way and never left
// out: the word false is the only way back from a client to somebody of the
// company. Company stands on a client alone, because the company of anybody
// else is nothing the import has a use for.
type Person struct {
	Record
	Name    string  `json:"name"`
	Email   *string `json:"email"`
	Avatar  *string `json:"avatar"`
	Retired bool    `json:"retired"`

	Client  bool           `json:"client"`
	Company *CompanyOrNone `json:"company,omitempty"`
}

// Project is a project of the source. ClientAccess tells that it works with
// clients and ClientCompany names the company it works with; the two stand
// together or neither does, because a company without the access would say that
// a project with a client company has none.
type Project struct {
	Record
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Members     []string `json:"members"`

	ClientAccess  *bool          `json:"clientAccess,omitempty"`
	ClientCompany *CompanyOrNone `json:"clientCompany,omitempty"`
}

// Tool is a tool of a project. VisibleToClients is whether the source shows the
// tool to clients and is left out where the source says nothing, which leaves
// what Minka holds alone.
type Tool struct {
	Record
	ProjectSourceID string `json:"projectSourceId"`
	Kind            string `json:"kind"`
	Name            string `json:"name"`
	Position        int    `json:"position"`

	VisibleToClients *bool `json:"visibleToClients,omitempty"`
}

// Category is a kind of post a project offers. It belongs to the project, not
// to one of its tools.
type Category struct {
	Record
	ProjectSourceID string `json:"projectSourceId"`
	Emoji           string `json:"emoji"`
	Name            string `json:"name"`
	Position        int    `json:"position"`
}

// Column is a column of a board. A board has at most one column of each role
// other than work.
type Column struct {
	Record
	ProjectSourceID string `json:"projectSourceId"`
	ToolSourceID    string `json:"toolSourceId"`
	Name            string `json:"name"`
	Position        int    `json:"position"`
	Role            string `json:"role"`
}

// Card is a card of a board. Position counts from 1 within the column, without
// gaps, whatever the status of the card. OnHold marks a card the source had
// set aside within its column.
type Card struct {
	Record
	ProjectSourceID string   `json:"projectSourceId"`
	ToolSourceID    string   `json:"toolSourceId"`
	ColumnSourceID  string   `json:"columnSourceId"`
	Title           string   `json:"title"`
	Description     *string  `json:"description"`
	Position        int      `json:"position"`
	DueOn           *string  `json:"dueOn"`
	CompletedAt     *string  `json:"completedAt"`
	CompletedBy     *string  `json:"completedBy"`
	Assignees       []string `json:"assignees"`
	Status          string   `json:"status"`
	OnHold          bool     `json:"onHold,omitempty"`

	// Subscribers are whom the source says follow the item. Minka takes them as
	// the whole truth, so nil leaves the field out and Minka follows the item
	// as it always did, while an empty list says nobody follows it.
	Subscribers *[]string `json:"subscribers,omitempty"`
}

type Step struct {
	Record
	ProjectSourceID string  `json:"projectSourceId"`
	ToolSourceID    string  `json:"toolSourceId"`
	CardSourceID    string  `json:"cardSourceId"`
	Title           string  `json:"title"`
	Position        int     `json:"position"`
	CompletedAt     *string `json:"completedAt"`
	CompletedBy     *string `json:"completedBy"`
}

// Post is a post of a Posts tool. Status is never a draft: a draft is its
// writer's own and no bundle carries one. VisibleToClients is whether the source
// shows this post to clients, whatever it shows of the tool it stands in.
type Post struct {
	Record
	ProjectSourceID  string  `json:"projectSourceId"`
	ToolSourceID     string  `json:"toolSourceId"`
	CategorySourceID *string `json:"categorySourceId"`
	Title            string  `json:"title"`
	Body             string  `json:"body"`
	PinnedAt         *string `json:"pinnedAt"`
	Status           string  `json:"status"`

	VisibleToClients *bool `json:"visibleToClients,omitempty"`

	// Subscribers are what they are on a Card.
	Subscribers *[]string `json:"subscribers,omitempty"`
}

// Question is a question a Check-ins tool asks on a schedule. Weekdays count
// from 1 for Monday to 7 for Sunday, one or more of them for the rhythm daily
// and exactly one for every other rhythm; TimeOfDay is HH:MM. StartsOn is the
// day the rhythm is counted from, which decides which week an everyOtherWeek
// question falls in. Paused is a question that asks nobody any more.
//
// Nothing hangs on a question: it carries no comments and no reactions, and
// the format has no word for either of a question.
//
// VisibleToClients is whether the source shows this question to clients,
// whatever it shows of the tool it stands in.
type Question struct {
	Record
	ProjectSourceID string   `json:"projectSourceId"`
	ToolSourceID    string   `json:"toolSourceId"`
	Title           string   `json:"title"`
	Rhythm          string   `json:"rhythm"`
	Weekdays        []int    `json:"weekdays"`
	TimeOfDay       string   `json:"timeOfDay"`
	StartsOn        string   `json:"startsOn"`
	Asked           []string `json:"asked"`
	Paused          bool     `json:"paused"`
	Status          string   `json:"status"`

	VisibleToClients *bool `json:"visibleToClients,omitempty"`
}

// Answer is what somebody answered a question on one day. GroupOn is that day,
// as the source has it: it is never held against the schedule, so an answer may
// sit on a day the question never asked on.
type Answer struct {
	Record
	ProjectSourceID  string `json:"projectSourceId"`
	ToolSourceID     string `json:"toolSourceId"`
	QuestionSourceID string `json:"questionSourceId"`
	GroupOn          string `json:"groupOn"`
	Body             string `json:"body"`
}

// TodoList is a list of a To-dos tool. Minka puts every new list on top of its
// tool, so the lists that are in use stand in the file from the bottom up: the
// last line of a tool is the list on top. The ones the source has put away stand
// before them in the order it lists them, which decides nothing there.
// Status is active, archived or trashed.
// VisibleToClients is whether the source shows this list to clients, whatever it
// shows of the tool it stands in.
type TodoList struct {
	Record
	ProjectSourceID string  `json:"projectSourceId"`
	ToolSourceID    string  `json:"toolSourceId"`
	Title           string  `json:"title"`
	Description     *string `json:"description"`
	Status          string  `json:"status"`

	VisibleToClients *bool `json:"visibleToClients,omitempty"`
}

// TodoGroup is a named heading inside a list. A group goes to the end of its
// list, so the order of the file is the order of the list. Status is active or
// trashed: the shelf is the list's.
//
// Nothing hangs on a group: it carries no comment, no reaction and no file, and
// the format has no word for either of one.
type TodoGroup struct {
	Record
	ProjectSourceID string `json:"projectSourceId"`
	ToolSourceID    string `json:"toolSourceId"`
	ListSourceID    string `json:"listSourceId"`
	Title           string `json:"title"`
	Status          string `json:"status"`
}

// Todo is a to-do of a list, standing directly in it or in one of its groups.
// It goes to the end of its list or its group, so the order of the file is the
// order of the list. GroupSourceID names a group of the same list and is nil
// for a to-do that stands in the list itself.
//
// StartsOn lies before DueOn, and one day alone is the day the to-do is due.
// NotifyWhenDone are the people the source tells when it is done. Status is
// active or trashed, as a group's is.
type Todo struct {
	Record
	ProjectSourceID string   `json:"projectSourceId"`
	ToolSourceID    string   `json:"toolSourceId"`
	ListSourceID    string   `json:"listSourceId"`
	GroupSourceID   *string  `json:"groupSourceId"`
	Title           string   `json:"title"`
	Notes           *string  `json:"notes"`
	StartsOn        *string  `json:"startsOn"`
	DueOn           *string  `json:"dueOn"`
	Assignees       []string `json:"assignees"`
	NotifyWhenDone  []string `json:"notifyWhenDone"`
	CompletedAt     *string  `json:"completedAt"`
	CompletedBy     *string  `json:"completedBy"`
	Status          string   `json:"status"`

	// Subscribers are what they are on a Card.
	Subscribers *[]string `json:"subscribers,omitempty"`
}

// Forward is a mail somebody forwarded into a project. Title is the subject,
// CreatedBy whoever forwarded it and CreatedAt the moment they did. The sender
// stays an address and never becomes a person; FromName is what the source
// calls them and is nil where it says nothing.
//
// Status is active or trashed: forwards have no shelf in Minka, and a bundle
// that said one would stop the run. Whether clients see a forward is the word
// of its tool, so a forward carries none of its own.
type Forward struct {
	Record
	ProjectSourceID string  `json:"projectSourceId"`
	ToolSourceID    string  `json:"toolSourceId"`
	Title           string  `json:"title"`
	FromAddress     string  `json:"fromAddress"`
	FromName        *string `json:"fromName"`
	Body            string  `json:"body"`
	Status          string  `json:"status"`
}

// ForwardReply is an answer by mail to a forward: a record of its own and no
// comment, so nothing hangs on it. CreatedBy is the person who wrote it where
// the bundle carries them, and then FromAddress is nil; for somebody from
// outside it is the other way round, because an outsider never becomes a
// person. One of the two is always there.
type ForwardReply struct {
	Record
	ProjectSourceID string  `json:"projectSourceId"`
	ToolSourceID    string  `json:"toolSourceId"`
	ForwardSourceID string  `json:"forwardSourceId"`
	FromAddress     *string `json:"fromAddress"`
	Body            string  `json:"body"`
}

// Folder is a folder of a Docs & Files tool. ParentSourceID names the folder it
// stands in and is nil at the top of the tool. Status is active, archived or
// trashed and is the folder's own: what stands in it carries its own status and
// goes along through the folder.
//
// A folder stands in the file after the folder it is in. For an entry in use the
// order of the file decides, bottom up, because Minka puts each new one on top of
// its kind; for one that has left its place it decides nothing, Minka ordering an
// archive by the moment a record was put away, so those stand first and in the
// order the source lists them.
//
// VisibleToClients is what the source says of this folder, written at every
// depth: Minka reads it below the top as well, where it is the one thing that
// can hide a top folder the source shows. A folder takes no comments and no
// reactions.
type Folder struct {
	Record
	ProjectSourceID string  `json:"projectSourceId"`
	ToolSourceID    string  `json:"toolSourceId"`
	ParentSourceID  *string `json:"parentSourceId"`
	Title           string  `json:"title"`
	Status          string  `json:"status"`

	VisibleToClients *bool `json:"visibleToClients,omitempty"`
}

// Document is a document of a Docs & Files tool. FolderSourceID names the
// folder it stands in and is nil at the top of the tool. Status and
// VisibleToClients are a folder's.
type Document struct {
	Record
	ProjectSourceID string  `json:"projectSourceId"`
	ToolSourceID    string  `json:"toolSourceId"`
	FolderSourceID  *string `json:"folderSourceId"`
	Title           string  `json:"title"`
	Body            *string `json:"body"`
	Status          string  `json:"status"`

	VisibleToClients *bool `json:"visibleToClients,omitempty"`

	// Subscribers are what they are on a Card.
	Subscribers *[]string `json:"subscribers,omitempty"`
}

// Upload is one stored file of a Docs & Files tool standing beside a text
// rather than inside one. Title is the name of the file, because Minka derives
// nothing from it, and File names the record of the file in attachments.jsonl.
// Status and VisibleToClients are a folder's.
//
// Only the file the source shows now comes; the format keeps `versions` free
// for the older ones and reads it nowhere, so a bundle writes none.
type Upload struct {
	Record
	ProjectSourceID string  `json:"projectSourceId"`
	ToolSourceID    string  `json:"toolSourceId"`
	FolderSourceID  *string `json:"folderSourceId"`
	Title           string  `json:"title"`
	File            string  `json:"file"`
	Description     *string `json:"description"`
	Status          string  `json:"status"`

	VisibleToClients *bool `json:"visibleToClients,omitempty"`

	// Subscribers are what they are on a Card.
	Subscribers *[]string `json:"subscribers,omitempty"`
}

// Message is one line of a chat.
type Message struct {
	Record
	ProjectSourceID string `json:"projectSourceId"`
	ToolSourceID    string `json:"toolSourceId"`
	Body            string `json:"body"`
}

type Comment struct {
	Record
	ProjectSourceID string `json:"projectSourceId"`
	ParentKind      string `json:"parentKind"`
	ParentSourceID  string `json:"parentSourceId"`
	Body            string `json:"body"`
}

// Reaction is what somebody answered a record with, usually one emoji.
// CreatedBy is who reacted and is never nil.
type Reaction struct {
	Record
	TargetKind     string `json:"targetKind"`
	TargetSourceID string `json:"targetSourceId"`
	Content        string `json:"content"`
}

// Attachment describes a file of the bundle. File is relative to the
// directory files/ of the bundle. ContentType is what the source says and nil
// where it says nothing. CreatedBy is nil for a file of a text, which reads as
// whoever wrote the text.
//
// Gone marks a file the source no longer has: such a record says what the
// file was and whose, and File, Bytes and SHA256 are nil.
type Attachment struct {
	Record
	Filename      string  `json:"filename"`
	ContentType   *string `json:"contentType"`
	Bytes         *int64  `json:"bytes"`
	SHA256        *string `json:"sha256"`
	File          *string `json:"file"`
	Gone          bool    `json:"gone,omitempty"`
	OwnerKind     string  `json:"ownerKind"`
	OwnerSourceID string  `json:"ownerSourceId"`
}

// Holds says of a record what the bundle stores for it.
func (a *Attachment) Holds(stored StoredFile) {
	a.File, a.Bytes, a.SHA256 = &stored.File, &stored.Bytes, &stored.SHA256
}

// Manifest says where a bundle came from and how much it holds. Parts names
// the files of a kind that was written in several, in their order; a kind that
// stands in its one file is not in it.
type Manifest struct {
	Format         int               `json:"format"`
	Source         string            `json:"source"`
	SourceInstance string            `json:"sourceInstance"`
	ExtractedAt    string            `json:"extractedAt"`
	Extractor      string            `json:"extractor"`
	Counts         map[Kind]int      `json:"counts"`
	Parts          map[Kind][]string `json:"parts,omitempty"`
}
