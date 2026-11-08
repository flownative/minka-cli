// Package fakeapi is an in-memory Minka API for tests.
//
// It follows the contract of /api/v1 as the Minka server implements it: the
// same paths, status codes and error codes, checked in the same order, and the
// byte-identical 401 and 404 bodies. It does not sanitize descriptions, so a
// test sees exactly the HTML the CLI sent.
package fakeapi

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/gif"  // so that a picture of this kind says how wide it is
	_ "image/jpeg" // so that a picture of this kind says how wide it is
	_ "image/png"  // so that a picture of this kind says how wide it is
	"io"
	"math"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	UnauthenticatedBody = `{"error":{"code":"unauthenticated","message":"This address needs a valid access token in the Authorization header."}}`
	NotFoundBody        = `{"error":{"code":"notFound","message":"There is nothing at this address."}}`

	maxTitleLength       = 200
	maxDescriptionLength = 10000
	maxCommentLength     = 10000
	maxStepLength        = 500
	maxNotesLength       = 100000
	maxChatMessageLength = 10000
)

// uuidPattern is the shape of an identifier as Minka keeps one: lowercase.
// Where Minka reads one from a request, it writes it in lowercase first, so
// the fake does too before it holds a value against this pattern.
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// identifierWildcard finds the parts of a route that carry an identifier.
var identifierWildcard = regexp.MustCompile(`\{([A-Za-z]+Id)\}`)

// identifierFields are the fields of a body Minka reads an identifier from in
// any case. The name a client gives a draft of a chat message is none of
// them: Minka refuses it in capitals.
var identifierFields = []string{
	"assigneeId", "assigneeIds", "askedIds", "blobId", "cardIds", "categoryId", "columnId",
	"folderId", "groupId", "listId", "nextCardId", "nextGroupId", "nextSubtaskId", "nextTodoId",
	"nextColumnId", "notifiedIds", "notifyIds", "personId", "previousCardId", "previousColumnId", "previousGroupId",
	"participantIds", "previousSubtaskId", "previousTodoId", "replyTo", "todoId",
	"previousStepId", "nextStepId",
}

// identifiersOfTheAddressInAnyCase writes every identifier of the address in
// lowercase, as Minka does before it looks for what one names.
func identifiersOfTheAddressInAnyCase(r *http.Request) {
	for _, match := range identifierWildcard.FindAllStringSubmatch(r.Pattern, -1) {
		r.SetPathValue(match[1], strings.ToLower(r.PathValue(match[1])))
	}
}

// identifiersOfTheBodyInAnyCase writes the identifiers of a body in lowercase,
// alone or in a list; a value of another type is left for its field to refuse.
func identifiersOfTheBodyInAnyCase(fields map[string]any) {
	for _, name := range identifierFields {
		switch value := fields[name].(type) {
		case string:
			fields[name] = strings.ToLower(value)
		case []any:
			for i, entry := range value {
				if id, ok := entry.(string); ok {
					value[i] = strings.ToLower(id)
				}
			}
		}
	}
}

// maxClientIDLength and clientIDPattern are the shape of the name one client
// of an agent goes by.
const maxClientIDLength = 64

var clientIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,` + strconv.Itoa(maxClientIDLength) + `}$`)

// AtWork is one card one client of an agent says it is working on. The server
// keeps every signal it was sent; it does not forget one after the three
// minutes a real Minka counts it for.
type AtWork struct {
	AgentID    string
	ClientID   string
	CardID     string
	LastSeenAt time.Time
}

type Person struct {
	ID          string
	Name        string
	Email       string
	PrincipalID string
	Retired     bool // an agent that stepped down: gone from every circle, and its tokens with it

	// Client says that this person works with the organization as a client and
	// not on its team. No resource of the API carries the word, so nothing the
	// fake answers with shows it: it decides what a client may be told about,
	// and nothing else.
	Client bool

	// TimeZone is the zone of the preferences, empty for none: the
	// installation's is taken then. No resource carries it either.
	TimeZone string

	// OrganizationRole is owner, admin or member; empty counts as member. No
	// resource carries it: it decides who writes the conventions of a board.
	OrganizationRole string
}

type Project struct {
	ID          string
	Name        string
	Description string
	MemberIDs   []string
	Archived    bool // readable, and closed to every write but the read mark of a chat

	// TemplateLibrary is the organization's hidden project of templates,
	// whose members are the team: no list of projects names it, and its
	// To-dos tool holds the templates.
	TemplateLibrary bool
}

type Board struct {
	ID        string
	ProjectID string
	Title     string
	Columns   []*Column
	// Coordinators are the entries of the people who coordinate the board, by
	// person; one is kept after it expired, and read as none from then on.
	Coordinators map[string]*Coordinator
	// Conventions are the versions of how the team works on the board, the
	// oldest first; the last is in force.
	Conventions []*ConventionsVersion
	// Trashed stands for a board in the trash: the board, the archive of its
	// cards and the next card answer 404, and neither the list of the
	// project's boards nor anybody's cards name it. Nothing else knows it.
	Trashed bool
	// FormerPrefixes are prefixes the board's short identifiers had before:
	// a short identifier under one of them still leads to its card.
	FormerPrefixes []string
}

type Column struct {
	ID          string
	Type        string
	CustomTitle *string
	// Cards holds both areas of the column. The cards on hold keep their
	// order among themselves, the others theirs; how the two interleave here
	// means nothing.
	Cards         []*Card
	OnHoldEnabled bool    // only a working column can have an area for cards on hold
	Color         *string // only a working column keeps one; nil for none
	Role          *string // only a working column keeps one; Triage and Done carry theirs by type
}

type Card struct {
	ID string
	// ShortID is such as "MNK-1A", empty where the board shows no short
	// identifiers. A test sets it on a card; the server gives one to every
	// card of a board with a prefix, which this does not mirror: a card
	// created here has none.
	ShortID       string
	Title         string
	Description   string
	CreatedAt     time.Time
	CreatorID     string
	CompletedAt   *time.Time
	CompletedByID string
	AssigneeIDs   []string // in the order they were assigned
	SubscriberIDs []string
	OffIDs        []string // people who took themselves off and are not put back by Minka
	DueOn         *string  // YYYY-MM-DD, nil for none
	OnHold        bool     // waiting in the area on hold of its column
	Steps         []*Step
	Comments      []*Comment

	// Notes is the back of the card: one text, overwritten as a whole.
	Notes            string
	NotesUpdatedAt   *time.Time // nil while nobody has written notes
	NotesUpdatedByID string
	// BackFiles are the files on the back beside the notes, in the order they
	// were first put there.
	BackFiles []*CardBackFile
}

type Step struct {
	ID string
	// CardID names the card this step is, empty for a step in words. A card
	// step carries nothing of its own: its card gives title and tick.
	CardID      string
	Title       string
	Completed   bool
	AssigneeIDs []string // in the order they were assigned; somebody who left the project stays on
	DueOn       *string  // YYYY-MM-DD, nil for none
}

type Comment struct {
	ID        string
	AuthorID  string
	Body      string
	CreatedAt time.Time
	EditedAt  *time.Time
	Reactions []*Reaction
}

// Reaction is what people said about a comment or a message, the same content
// counted together.
type Reaction struct {
	Content   string
	PersonIDs []string // in the order they reacted in
}

type RecordedRequest struct {
	Method string
	Path   string // escaped, as it arrived
	Query  string // the raw query string
	Header http.Header
	Body   []byte
}

type Chat struct {
	ID        string
	ProjectID string   // empty for a private conversation
	Partners  []string // the two people of a private conversation, nobody for a chat of a project
	Name      string
	Messages  []*ChatMessage // in the order they were written, which is not the order of the transcript
	ReadMarks map[string]int // person ID → the seq read up to

	// One counter numbers the messages and their changes alike, so the numbers
	// of two messages next to each other may lie apart.
	lastSeq int
}

// isConversation says whether this is a private talk, which has people instead
// of a project and is addressed by the same endpoints.
func (c *Chat) isConversation() bool {
	return len(c.Partners) > 0
}

// TrashedCard is a card that was taken off a board, together with the column
// it hung in, which is where a restore puts it back.
type TrashedCard struct {
	Card      *Card
	Column    *Column
	TrashedAt time.Time
	TrashedBy string // person ID

	seq int
}

// ArchivedCard is a card that was put into the archive of its board. It keeps
// the column it was taken from, which is the column it goes on naming although
// it stands in none, and the moment and the person it was put away by, which is
// what the archive of a board is ordered and listed by.
type ArchivedCard struct {
	Card   *Card
	Column *Column
	At     time.Time // the zero moment is a card put away before Minka noted one
	ByID   string    // empty where nobody is known
}

// ArchivedCardsOnAPage is the page the archive of a board gives without a
// limit, MostArchivedCardsOnAPage the largest page it serves. A limit above the
// largest is not refused but cut down to it without a word.
const (
	ArchivedCardsOnAPage     = 50
	MostArchivedCardsOnAPage = 100
)

// TrashEntries is how many entries an answer carries at most, as the server
// caps them.
const TrashEntries = 200

// MaxUploadBytes is what Minka takes for one file.
const MaxUploadBytes = 100 << 20

// DisplayEdge is how long the longer side of a picture has to be before Minka
// renders a smaller one to show beside it. Anything below it has the original
// and nothing else, however the two sides are shared out.
const DisplayEdge = 1600

// Variant names as the API names them.
const (
	VariantOriginal = "original"
	VariantDisplay  = "display"
)

// Upload is a file somebody put into a project. The answer carries an
// attachment whose handle only this project has, and the bytes are kept so
// that the file can be asked for again.
type Upload struct {
	ID          string
	Circle      string // the project, or "conversation:" and the conversation
	Filename    string
	ContentType string
	Size        int
	Content     []byte
	// Display is the smaller rendering a wide picture got, and is empty for
	// every other file. Nothing here reads a picture, so it is a shorter run
	// of bytes of its own rather than a rendering of the original.
	Display []byte

	// UploaderID is who stored the file. Only they can put it into an upload
	// of a Docs & Files tool, and only while it stands nowhere yet.
	UploaderID string
	// StandsInAText says that a saved text carries the file. The fake does
	// not look through its texts for it, so a test says so here.
	StandsInAText bool
	// Swept says that the file stood in nothing for longer than Minka keeps
	// such a file and was taken away: it is then at no address at all.
	Swept bool
}

// renderings are the names this file can be asked for, the original first and
// the rest by name, as the server orders them.
func (u *Upload) renderings() []string {
	names := []string{VariantOriginal}
	if len(u.Display) > 0 {
		names = append(names, VariantDisplay)
	}
	return names
}

// bytesOf is one rendering of a file, or nil where the file has no such
// rendering and where the name is none at all.
func (u *Upload) bytesOf(variant string) []byte {
	switch variant {
	case VariantOriginal:
		return u.Content
	case VariantDisplay:
		return u.Display
	}
	return nil
}

// Handle stands for the signature the server puts on the pair of circle and
// file, which is what makes an attachment belong to a text of that circle and
// of no other. It has Minka's shape: the file's identifier, two dashes and
// the signature.
func (u *Upload) Handle() string {
	signature := sha256.Sum256([]byte(u.Circle))
	return u.ID + "--" + hex.EncodeToString(signature[:16])
}

type ChatMessage struct {
	ID              string
	Seq             int
	ChangeSeq       int
	AuthorID        string
	BodyText        string
	BodyHTML        string
	ClientMessageID string
	CreatedAt       time.Time
	EditedAt        *time.Time
	Removed         bool
	Imported        bool // brought in from another tool; no field of the answer says so
	Reactions       []*Reaction
	ReplyTo         string // the message of the same chat this one answers, empty for none
}

func (c *Chat) post(authorID, text, clientMessageID string, now time.Time) *ChatMessage {
	c.lastSeq++
	message := &ChatMessage{
		ID: newID(), Seq: c.lastSeq, ChangeSeq: c.lastSeq, AuthorID: authorID,
		BodyText: chatText(text), BodyHTML: chatHTML(text), ClientMessageID: clientMessageID, CreatedAt: now,
	}
	c.Messages = append(c.Messages, message)
	return message
}

// noteChange gives a message the next number of the chat as its change number.
// An edit, a removal and a reaction given or taken away all do, which is how
// whoever catches up learns of them.
func (c *Chat) noteChange(message *ChatMessage) {
	c.lastSeq++
	message.ChangeSeq = c.lastSeq
}

// Import writes a line the way an import brings it: it takes the next number
// of the chat and stands at the moment it was said, which may lie in front of
// messages with lower numbers.
func (c *Chat) Import(authorID, text string, saidAt time.Time) *ChatMessage {
	message := c.post(authorID, text, "", saidAt)
	message.Imported = true
	return message
}

// saidBefore is the order of a transcript: by moment, and by number among
// messages of the same moment.
func saidBefore(one, other *ChatMessage) int {
	if byMoment := one.CreatedAt.Compare(other.CreatedAt); byMoment != 0 {
		return byMoment
	}
	return one.Seq - other.Seq
}

// transcript is what a reader sees of a chat: every message that was not taken
// back, in the order things were said.
func (c *Chat) transcript() []*ChatMessage {
	var transcript []*ChatMessage
	for _, message := range c.Messages {
		if !message.Removed {
			transcript = append(transcript, message)
		}
	}
	slices.SortFunc(transcript, saidBefore)
	return transcript
}

// lastWord is the newest message of the transcript, or nil while nothing
// stands in it.
func (c *Chat) lastWord() *ChatMessage {
	transcript := c.transcript()
	if len(transcript) == 0 {
		return nil
	}
	return transcript[len(transcript)-1]
}

// unread counts by number, wherever a message stands in the transcript. What
// was taken back, what an import brought and what the person wrote themselves
// is nothing to come and read.
func (c *Chat) unread(personID string) int {
	unread := 0
	for _, message := range c.Messages {
		if message.Seq > c.ReadMarks[personID] && !message.Removed && !message.Imported && message.AuthorID != personID {
			unread++
		}
	}
	return unread
}

var tagForm = regexp.MustCompile(`<[a-zA-Z/][^<>]*>`)

// chatHTML is a simple stand-in for the markup the server makes of chat text:
// markup is kept as sent, plain text is escaped into a paragraph.
func chatHTML(text string) string {
	if tagForm.MatchString(text) {
		return text
	}
	return "<p>" + strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + "</p>"
}

// chatText is a simple stand-in for the plain text the server derives from the
// markup of a message.
func chatText(text string) string {
	if !tagForm.MatchString(text) {
		return text
	}
	return html.UnescapeString(tagForm.ReplaceAllString(strings.ReplaceAll(text, "<br>", "\n"), ""))
}

// Server is a running fake. The fields are its data; tests may change them
// between requests while holding no lock, as long as no request is running.
type Server struct {
	*httptest.Server

	People        map[string]*Person
	Tokens        map[string]string // token → person ID
	Projects      []*Project
	Boards        []*Board
	Chats         []*Chat
	FilesTools    []*FilesTool
	TodosTools    []*TodosTool
	CheckinsTools []*CheckinsTool
	PostsTools    []*PostsTool
	Calendars     []*CalendarTool
	Uploads       []*Upload

	// Labels are the labels of every project and tool, each place's in its
	// order; ItemLabels are the IDs of the labels every card, document and
	// upload carries, by the ID of the item.
	Labels     []*Label
	ItemLabels map[string][]string

	// LaterBookEntries stand at the end of every book's tree, for a test of an
	// entry of a kind a later Minka sends.
	LaterBookEntries []any

	Trash          []*TrashedCard
	TrashedColumns []*TrashedColumn
	Archive        []*ArchivedCard

	// Reminders are the reminders of every person, RemovedReminders those
	// taken away, with the outcome they were taken away with.
	Reminders        []*Reminder
	RemovedReminders []RemovedReminder

	// trashSeq counts what went into the trash, so that the trash can list
	// what came last first among things taken out in the same moment.
	trashSeq int

	// PostCategories are the kinds of post of every project, each in the
	// order its project lists them.
	PostCategories []*PostCategory

	// TimeZone is the installation's zone, which reads the days of whoever
	// set none of their own. It may be a name of the backward table, such as
	// CET.
	TimeZone string

	// IgnoresProjectStatus makes the server answer like a Minka from before
	// projects could be archived: no status, and the filter goes unnoticed.
	IgnoresProjectStatus bool

	// KeepsColumnColorOnNull makes the server answer like a Minka from before
	// a column's colour could be taken away: a null colour names nothing, so
	// it alone is nothingToChange and beside a title the colour stays.
	KeepsColumnColorOnNull bool

	// IgnoresColumnRoles makes the server answer like a Minka from before
	// column roles: a role in a body names nothing, and no column carries the
	// field, Triage and Done included.
	IgnoresColumnRoles bool

	// KnowsNoNextCard makes the server answer like a Minka from before it
	// named the card somebody takes up next on a board: the address is none.
	KnowsNoNextCard bool

	// MovesWithinToolsOnly makes the server answer like a Minka from before
	// moves into another tool: it reads no toolId and moves the entry within
	// its own tool.
	MovesWithinToolsOnly bool

	// KnowsNoBooks makes the server answer like a Minka from before books and
	// the order set by hand: it reads neither isBook nor inNavigation, writes
	// what else was sent with 200, and its answers carry none of their fields.
	KnowsNoBooks bool

	// KnowsNoTree makes the server answer like a Minka from before a folder
	// could be read by itself and as a tree: both addresses answer with a
	// page of its web, a folder's answer names neither its project, its
	// parent nor when it changed, and a page of a book neither when it
	// changed nor its version.
	KnowsNoTree bool

	// NeitherRenamesNorMovesSteps makes the server answer like a Minka from
	// before a step could be renamed or moved: a title names nothing, so it
	// alone is nothingToChange and beside the rest it is passed over, and the
	// address of a move is none.
	NeitherRenamesNorMovesSteps bool

	// AnswersANewStepWithItsCard makes the server answer like a Minka from
	// before a new step answered as itself: the answer to a new step, a
	// card step that stands already included, is the whole card. A Minka
	// that knows no card steps or cannot rename a step answers so as well.
	AnswersANewStepWithItsCard bool

	// KnowsNoCardSteps makes the server answer like a Minka from before a
	// card could be the step of another: a step carries no card and a card
	// names no parents, a cardId in the body of a new step is passed over, so
	// that only the missing title is refused, and making a step into a card
	// has no address.
	KnowsNoCardSteps bool

	// SearchesOnlyWithinFiles makes the server answer like a Minka from before
	// it searched across everything: a search without a tool or a folder is
	// searchScopeMissing, kinds and projectId are passed over, an offset has
	// no ceiling but its six digits, and a hit carries neither targetKind,
	// targetId, shortId, shortUrl nor apiUrl.
	SearchesOnlyWithinFiles bool

	// NamesPagesBeyondTheDeepestOffset makes a search name a next page beyond
	// the offset of 500 it serves at most, which the server then refuses, as
	// Minka did when it first searched across everything.
	NamesPagesBeyondTheDeepestOffset bool

	// MessagesPerPage, where set, caps a page of a transcript below the 200
	// Minka allows, so that a client cannot take the page size for granted.
	MessagesPerPage int

	// CLILatest and CLIMinimum, where set, are sent on every answer as
	// Minka-CLI-Latest and Minka-CLI-Minimum, the way a Minka does that names
	// the versions of minka. They are sent as they stand, so that a test can
	// hand out a value that is no version.
	CLILatest  string
	CLIMinimum string

	// HandsOutNoShortURL makes the server answer like a Minka from before a
	// card and the subject of a notification carried shortUrl: the field is
	// missing, not null.
	HandsOutNoShortURL bool

	// HandsOutNoCommentURL makes the server answer like a Minka from before a
	// comment carried appUrl: the field is missing.
	HandsOutNoCommentURL bool

	// AnswersCardsWithoutID makes the lookup of a short identifier answer with
	// a card that has no ID, which no Minka does: what a client does with an
	// answer it cannot use must not depend on that.
	AnswersCardsWithoutID bool

	// AfterStoring is called with every file just stored, before the answer
	// goes out. It lets a test change what the file meets next, such as a
	// sweep between storing a file and putting it into a tool.
	AfterStoring func(*Upload)

	// BeforeMovingStep is called with the card of every move of a step before
	// anything of it is looked at. It lets a test change the steps between
	// the read of the card and the move, as somebody else on the web would.
	BeforeMovingStep func(*Card)

	// StoreURL stands for a file store that serves the bytes itself. Where it
	// is set, a picture is answered with a redirect there, the way an
	// installation with such a store answers it.
	StoreURL string

	// LeavesOutWithReason is the word the fake gives for a card it leaves out
	// of a set at work. Empty is `cardArchived`, which is the one Minka gives;
	// another one stands for a word Minka may learn, and what a client does
	// with a word it does not know must not depend on the server's choice.
	LeavesOutWithReason string

	// ForgetsTheArchiveBoundariesOf are the cards a page of an archive may not
	// start behind although they lie in the archive: naming one refuses the page
	// with archivedCardsBeforeUnknown, the way the server does once somebody has
	// taken that card out of the archive between two pages of one walk. One walk
	// can lose one boundary after another, which is why this is a set.
	ForgetsTheArchiveBoundariesOf []string

	// RacesOnTheArchiveBoundariesOf is the same loss in its narrow form: the
	// check that the card lies in the archive passes, and by the time the page is
	// read the card is gone, so the answer is an empty page that reads like the
	// end of the list. One page answers that way for one entry, which is spent
	// with it, and the same card named twice is that window met twice.
	RacesOnTheArchiveBoundariesOf []string

	// SendsAFileShort hands a file out one byte shorter than it was said to
	// be, which no Minka does: what a client makes of bytes that are not the
	// ones announced must not depend on the server being right about them.
	SendsAFileShort bool

	// Notifications are the lines of the bells of everybody, in the order
	// they were written.
	Notifications []*Notification

	// ForgottenCursors are positions of the bell the server no longer knows
	// although it handed them out, which it answers with cursorUnknown.
	ForgottenCursors []string

	// WaitsOut stands for the clock a held request of the bell waits by. It is
	// given the wait and answers when that has passed; nil is the real clock.
	// A test makes the wait pass at once with it.
	WaitsOut func(time.Duration) <-chan time.Time

	// WhileWaiting is called once a request of the bell is held, without the
	// lock of the server, so that a test can let a line arrive in the wait.
	WhileWaiting func(*Person)

	// HoldsAtMost, where set, is the longest the server holds a request of
	// the bell instead of WaitAtMost, as an installation configured otherwise.
	HoldsAtMost time.Duration

	notificationSeqs map[string]int // person ID → the number of their last line
	arrival          chan struct{}  // closed when a line arrives

	mu           sync.Mutex
	requests     []RecordedRequest
	failWith     int
	failReadWith int
	// failWithServerError takes precedence over a pending failWith.
	failWithServerError string
	now                 time.Time
	atWork              []AtWork
}

// Fixture names the data New seeds, for tests to refer to.
type Fixture struct {
	Token         string
	StrangerToken string
	AgentToken    string
	Bruno         *Person
	Anke          *Person // a member of the project besides Bruno
	Stranger      *Person // no member of the project
	Client        *Person // a member of the project who works with it as a client, and signs in nowhere
	ForeignClient *Person // a client of no project of this server, so no place can tell them anything
	Agent         *Person
	RetiredAgent  *Person // an agent that stepped down: still a member, in no list
	Project       *Project
	Board         *Board
	Triage        *Column
	Doing         *Column
	NotNow        *Column
	Done          *Column
	CardInTriage  *Card
	CardInDoing   *Card
	OtherProject  *Project
	OtherBoard    *Board
	OtherCard     *Card
	Chat          *Chat // three messages by Bruno, Anke and Bruno; Bruno has read the first
	OtherChat     *Chat
	Conversation  *Chat // Bruno and Anke privately; her one word is unread

	// The Docs & Files tool of the project: a folder with a document and an
	// upload in it, a folder in the archive below that one with a document of
	// its own, a folder whose contents the clients of the project are shown,
	// and at the top a published document, a draft and one in the archive.
	Files                          *FilesTool
	Folder                         *Folder
	ArchivedFolder                 *Folder
	FolderShownToClients           *Folder
	Document                       *Document // at the top of the tool, with one comment on it
	DocumentInFolder               *Document
	DocumentBelowTheArchivedFolder *Document
	DraftDocument                  *Document
	ArchivedDocument               *Document
	FilesUpload                    *FilesUpload // in the folder, with one comment on it
	OtherFiles                     *FilesTool   // of the project that belongs to somebody else
	OtherDocument                  *Document

	// The To-dos tool of the project: on top a list with two groups, a to-do
	// standing in the list, one in the first group and one that is done, below
	// it an empty list, and in the archive a list with a group of its own and a
	// to-do in that group. A second tool of the project is no longer in use, and
	// the project that belongs to somebody else has a tool with a list and a
	// to-do. The grouped to-do is Bruno's, due on a day before the fake's today,
	// with an open subtask of Anke's and a done one of nobody's. It is the first
	// occurrence of a series Anke made of it, weekly on Thursdays with a lead of
	// two days, whose next one shows after today.
	Todos            *TodosTool
	TodoList         *TodoList
	StageGroup       *TodoGroup // the first group of the list, red, with an open to-do in it
	KitchenGroup     *TodoGroup // the second group, empty and without a colour
	LooseTodo        *Todo      // open, standing in the list itself
	GroupedTodo      *Todo      // open, in the stage group
	DoneTodo         *Todo
	EmptyTodoList    *TodoList
	ArchivedList     *TodoList
	ArchivedGroup    *TodoGroup // a group of the list in the archive
	TodosOutOfUse    *TodosTool
	OtherTodos       *TodosTool
	OtherTodoList    *TodoList
	ArchivedListTodo *Todo // in the group of the list in the archive, with one subtask
	OtherTodo        *Todo
	Series           *TodoSeries // of the list, made of the grouped to-do

	// The template library: a hidden project of Bruno, Anke, the agent,
	// Stranger, Theo, Ada and Zora, the last three in no other project. Its
	// To-dos tool holds a template with a blue group and a group in the trash,
	// an empty template, one in the archive and one in the trash. The first
	// to-do of the template is Stranger's, Bruno's and Olga's, who is on no
	// team; it tells Anke and Ada when done and has a subtask of Zora's and one
	// of Anke's. The to-do of the trashed group and one in the trash are
	// Theo's, and Anke has the to-do of the blue group. Ada's ID sorts after
	// Zora's, so an order by ID is not the one by name.
	Theo             *Person
	Ada              *Person // in the library, on the template only as one told when done
	Zora             *Person // in the library, on the template only with a subtask
	Olga             *Person // in no project at all, so on no team
	TemplateLibrary  *Project
	Templates        *TodosTool
	Template         *TodoList
	EmptyTemplate    *TodoList
	ArchivedTemplate *TodoList
	TrashedTemplate  *TodoList
	TemplateTodo     *Todo

	// The Check-ins tool of the project, on the fake's today, a Sunday. A
	// daily question asks Anke, Bruno and the client, who is not shown it;
	// Bruno was asked it for Thursday and Friday and answered Friday, Anke
	// answered Thursday and Friday. A weekly question asks Bruno, who has not
	// answered Monday's asking yet, and a monthly one asks him and is paused.
	// The project that belongs to somebody else has a question of its own.
	CheckinsTool       *CheckinsTool
	DailyQuestion      *CheckinQuestion
	WeeklyQuestion     *CheckinQuestion
	PausedQuestion     *CheckinQuestion
	BrunosAnswer       *CheckinAnswer // for Friday, with a comment of Anke's
	AnkesAnswer        *CheckinAnswer // for Friday, written after Bruno's
	AnkesEarlierAnswer *CheckinAnswer // for Thursday
	OtherCheckinsTool  *CheckinsTool
	OtherQuestion      *CheckinQuestion

	// The Posts tool of the project, which lets its clients in: a post pinned
	// on top although it is the oldest, the newest post filed under a kind with
	// a comment of Anke's, an older one, Bruno's draft, one in the archive that
	// is pinned and carries a comment, and one in the trash. The project offers
	// two kinds; a second tool is no longer in use, and the project that belongs
	// to somebody else has a tool with a post and a kind of its own.
	Posts         *PostsTool
	PinnedPost    *Post
	Post          *Post // with one comment of Anke's
	OlderPost     *Post
	DraftPost     *Post
	ArchivedPost  *Post // pinned, with one comment of Bruno's
	TrashedPost   *Post
	Announcement  *PostCategory
	Heartbeat     *PostCategory
	PostsOutOfUse *PostsTool
	OtherPosts    *PostsTool
	OtherPost     *Post
	OtherCategory *PostCategory

	// The calendars, in Berlin.
	Calendar         *CalendarTool
	CalendarOutOfUse *CalendarTool  // no longer in use: not listed, read and written at its address
	OtherCalendar    *CalendarTool  // of the project that belongs to somebody else
	WeeklyEvent      *CalendarEvent // Mondays at 10:00 in Berlin from 19 October 2026, with Anke taking part
	OnceEvent        *CalendarEvent // on 21 October 2026, by Anke, with a comment of Anke's and then one of Bruno's
	AllDayEvent      *CalendarEvent // 20 to 22 October 2026, highlighted
	OtherEvent       *CalendarEvent
}

// New starts a fake with a small fixture: Bruno is a member of one project
// with one board; a second project with a board and a card belongs to
// somebody else. The server stops when the test ends.
func New(t testing.TB) (*Server, *Fixture) {
	s := &Server{
		People:     map[string]*Person{},
		Tokens:     map[string]string{},
		ItemLabels: map[string][]string{},
		now:        time.Date(2026, 9, 13, 14, 2, 11, 123456000, time.UTC),
	}

	bruno := &Person{ID: newID(), Name: "Bruno", Email: "bruno@example.com", OrganizationRole: "owner"}
	stranger := &Person{ID: newID(), Name: "Stranger", Email: "stranger@example.com"}
	agent := &Person{ID: newID(), Name: "Claude (Bruno)", Email: "claude-bruno@agents.invalid", PrincipalID: bruno.ID}
	anke := &Person{ID: newID(), Name: "Anke", Email: "anke@example.com"}
	retired := &Person{ID: newID(), Name: "Claude (Anke)", Email: "claude-anke@agents.invalid", PrincipalID: anke.ID, Retired: true}
	client := &Person{ID: newID(), Name: "Nele", Email: "nele@example.com", Client: true}
	foreignClient := &Person{ID: newID(), Name: "Ilka", Email: "ilka@example.com", Client: true}
	for _, p := range []*Person{bruno, anke, stranger, agent, retired, client, foreignClient} {
		s.People[p.ID] = p
	}
	s.Tokens["minka_pat_bruno"] = bruno.ID
	s.Tokens["minka_pat_stranger"] = stranger.ID
	s.Tokens["minka_pat_agent"] = agent.ID

	project := &Project{ID: newID(), Name: "Test-Projekt", Description: "For trying things", MemberIDs: []string{bruno.ID, anke.ID, agent.ID, retired.ID, client.ID}}
	triage := &Column{ID: newID(), Type: "triage"}
	doingTitle := "Doing"
	doing := &Column{ID: newID(), Type: "column", CustomTitle: &doingTitle}
	notNow := &Column{ID: newID(), Type: "not_now"}
	done := &Column{ID: newID(), Type: "done"}
	board := &Board{ID: newID(), ProjectID: project.ID, Title: "API-3 Abnahme", Columns: []*Column{triage, doing, notNow, done}}

	cardInTriage := &Card{ID: newID(), Title: "Write the CLI", Description: "<p>With <strong>Go</strong></p>", CreatedAt: s.now, CreatorID: bruno.ID,
		Comments: []*Comment{{ID: newID(), AuthorID: bruno.ID, Body: "First comment", CreatedAt: s.now,
			Reactions: []*Reaction{{Content: "👍", PersonIDs: []string{anke.ID, bruno.ID}}, {Content: "🎉", PersonIDs: []string{anke.ID}}}}}}
	cardInDoing := &Card{ID: newID(), Title: "Review the API", CreatedAt: s.now, CreatorID: bruno.ID}
	secondInDoing := &Card{ID: newID(), Title: "Second in Doing", CreatedAt: s.now, CreatorID: bruno.ID}
	triage.Cards = []*Card{cardInTriage}
	doing.Cards = []*Card{cardInDoing, secondInDoing}

	otherProject := &Project{ID: newID(), Name: "Secret", MemberIDs: []string{stranger.ID}}
	otherCard := &Card{ID: newID(), Title: "Not yours", CreatedAt: s.now, CreatorID: stranger.ID}
	otherBoard := &Board{ID: newID(), ProjectID: otherProject.ID, Title: "Secret board", Columns: []*Column{
		{ID: newID(), Type: "triage", Cards: []*Card{otherCard}},
		{ID: newID(), Type: "not_now"},
		{ID: newID(), Type: "done"},
	}}

	s.Projects = []*Project{project, otherProject}
	s.Boards = []*Board{board, otherBoard}

	chat := &Chat{ID: newID(), ProjectID: project.ID, Name: "Planung", ReadMarks: map[string]int{bruno.ID: 1}}
	chat.post(bruno.ID, "Hallo", "", s.now)
	chat.post(anke.ID, "Zelt ist bestellt", "", s.now).Reactions = []*Reaction{{Content: "👍", PersonIDs: []string{bruno.ID}}}
	chat.post(bruno.ID, "Danke", "", s.now)
	otherChat := &Chat{ID: newID(), ProjectID: otherProject.ID, Name: "Secret chat", ReadMarks: map[string]int{}}
	otherChat.post(stranger.ID, "Psst", "", s.now)
	conversation := &Chat{ID: newID(), Partners: []string{bruno.ID, anke.ID}, ReadMarks: map[string]int{}}
	conversation.post(anke.ID, "Hast du kurz Zeit?", "", s.now)
	s.Chats = []*Chat{chat, otherChat, conversation}

	aWholeSecond := s.now.Truncate(time.Second)
	folder := &Folder{ID: newID(), Title: "Plans", CreatedAt: aWholeSecond, CreatorID: bruno.ID}
	archivedFolder := &Folder{ID: newID(), Title: "Last year", ParentID: folder.ID, Archived: true, CreatedAt: aWholeSecond, CreatorID: bruno.ID,
		EntryState: EntryState{ArchivedAt: aWholeSecond.Add(-2 * time.Hour), ArchivedBy: anke.ID}}
	folderShownToClients := &Folder{ID: newID(), Title: "For the client", ShownToClients: true, CreatedAt: aWholeSecond, CreatorID: bruno.ID}
	document := &Document{ID: newID(), Title: "How the stage goes up", Body: "<p>Eight people on <strong>Friday</strong>.</p>",
		CreatedAt: aWholeSecond, UpdatedAt: aWholeSecond, CreatorID: bruno.ID,
		Comments: []*Comment{{ID: newID(), AuthorID: anke.ID, Body: "<p>I can help</p>", CreatedAt: s.now}}}
	documentInFolder := &Document{ID: newID(), Title: "What the tent costs", Body: "<p>Four hundred.</p>",
		FolderID: folder.ID, CreatedAt: aWholeSecond, UpdatedAt: aWholeSecond, CreatorID: anke.ID}
	documentBelowTheArchivedFolder := &Document{ID: newID(), Title: "What the tent cost last year", Body: "<p>Three hundred.</p>",
		FolderID: archivedFolder.ID, CreatedAt: aWholeSecond, UpdatedAt: aWholeSecond, CreatorID: anke.ID}
	draftDocument := &Document{ID: newID(), Title: "Not finished", Body: "<p>Later.</p>", Draft: true,
		CreatedAt: aWholeSecond, UpdatedAt: aWholeSecond, CreatorID: bruno.ID}
	archivedDocument := &Document{ID: newID(), Title: "Put away", Body: "<p>Done with.</p>", Archived: true,
		CreatedAt: aWholeSecond, UpdatedAt: aWholeSecond, CreatorID: bruno.ID,
		EntryState: EntryState{ArchivedAt: aWholeSecond.Add(-time.Hour)}}
	sitePlan := &Upload{ID: newID(), Circle: project.ID, Filename: "site-plan.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.4 a plan"), UploaderID: anke.ID}
	sitePlan.Size = len(sitePlan.Content)
	filesUpload := &FilesUpload{ID: newID(), Title: "site-plan.pdf", BlobID: sitePlan.ID, FolderID: folder.ID,
		Description: "<p>Where everything stands</p>", CreatedAt: aWholeSecond, UpdatedAt: aWholeSecond, CreatorID: anke.ID,
		Comments: []*Comment{{ID: newID(), AuthorID: bruno.ID, Body: "<p>Printed</p>", CreatedAt: s.now}}}
	s.Uploads = append(s.Uploads, sitePlan)
	files := &FilesTool{
		ID: newID(), ProjectID: project.ID, Title: "Documents", ShownToClients: true,
		Folders:   []*Folder{folder, archivedFolder, folderShownToClients},
		Documents: []*Document{document, draftDocument, archivedDocument, documentInFolder, documentBelowTheArchivedFolder},
		Uploads:   []*FilesUpload{filesUpload},
	}
	otherDocument := &Document{ID: newID(), Title: "Not yours either", Body: "<p>Secret.</p>",
		CreatedAt: aWholeSecond, UpdatedAt: aWholeSecond, CreatorID: stranger.ID}
	otherFiles := &FilesTool{ID: newID(), ProjectID: otherProject.ID, Title: "Secret documents", Documents: []*Document{otherDocument}}
	s.FilesTools = []*FilesTool{files, otherFiles}

	stageGroup := &TodoGroup{ID: newID(), Title: "Stage", Color: "red"}
	kitchenGroup := &TodoGroup{ID: newID(), Title: "Kitchen"}
	looseTodo := &Todo{ID: newID(), Title: "Count the tents", CreatedAt: s.now, CreatorID: bruno.ID}
	speakersDue := "2026-09-10"
	groupedTodo := &Todo{ID: newID(), Title: "Fetch the speakers", GroupID: stageGroup.ID, DueOn: &speakersDue,
		AssigneeIDs: []string{bruno.ID}, CreatedAt: s.now, CreatorID: anke.ID,
		Subtasks: []*Subtask{{ID: newID(), Title: "Find the cables", AssigneeID: anke.ID}, {ID: newID(), Title: "Book the van", Completed: true}}}
	doneAt := s.now
	doneTodo := &Todo{ID: newID(), Title: "Order the beer", CompletedAt: &doneAt, CompletedByID: anke.ID, CreatedAt: s.now, CreatorID: bruno.ID}
	series := &TodoSeries{ID: newID(), Title: groupedTodo.Title, GroupID: stageGroup.ID, LeadDays: 2, LastOccursOn: speakersDue,
		Schedule:    RepetitionSchedule{Rhythm: RepeatsWeekly, Weekdays: []int{4}, Interval: 1, StartsOn: speakersDue},
		AssigneeIDs: []string{bruno.ID}, CreatedAt: s.now, UpdatedAt: s.now, CreatorID: anke.ID}
	groupedTodo.SeriesID, groupedTodo.OccursOn = series.ID, speakersDue
	todoList := &TodoList{ID: newID(), Title: "Setting up", Series: []*TodoSeries{series}, Description: "<p>Everything for <strong>Saturday</strong>.</p>", CreatorID: bruno.ID,
		Groups: []*TodoGroup{stageGroup, kitchenGroup}, Todos: []*Todo{looseTodo, groupedTodo, doneTodo},
		Comments: []*Comment{{ID: newID(), AuthorID: anke.ID, Body: "<p>I bring the van</p>", CreatedAt: s.now}}}
	emptyTodoList := &TodoList{ID: newID(), Title: "Tearing down", CreatorID: anke.ID}
	archivedGroup := &TodoGroup{ID: newID(), Title: "Leftovers"}
	archivedListTodo := &Todo{ID: newID(), Title: "Return the chairs", GroupID: archivedGroup.ID, CreatedAt: s.now, CreatorID: bruno.ID,
		Subtasks: []*Subtask{{ID: newID(), Title: "Count them"}}}
	archivedList := &TodoList{ID: newID(), Title: "Last year", Archived: true, ArchivedAt: s.now.Add(-time.Hour), CreatorID: bruno.ID,
		Groups: []*TodoGroup{archivedGroup}, Todos: []*Todo{archivedListTodo}}
	todos := &TodosTool{ID: newID(), ProjectID: project.ID, Title: "To-dos", Lists: []*TodoList{todoList, emptyTodoList, archivedList}}
	todosOutOfUse := &TodosTool{ID: newID(), ProjectID: project.ID, Title: "Old to-dos", Archived: true}
	otherTodo := &Todo{ID: newID(), Title: "Not yours either", CreatedAt: s.now, CreatorID: stranger.ID}
	otherTodoList := &TodoList{ID: newID(), Title: "Not yours", CreatorID: stranger.ID, Todos: []*Todo{otherTodo}}
	otherTodos := &TodosTool{ID: newID(), ProjectID: otherProject.ID, Title: "Secret to-dos", Lists: []*TodoList{otherTodoList}}
	theo := &Person{ID: newID(), Name: "Theo", Email: "theo@example.com"}
	ada := &Person{ID: "ffffffff-0000-4000-8000-00000000000a", Name: "Ada", Email: "ada@example.com"}
	zora := &Person{ID: "00000000-0000-4000-8000-00000000000a", Name: "Zora", Email: "zora@example.com"}
	olga := &Person{ID: newID(), Name: "Olga", Email: "olga@example.com"}
	for _, p := range []*Person{theo, ada, zora, olga} {
		s.People[p.ID] = p
	}
	library := &Project{ID: newID(), Name: "Templates", TemplateLibrary: true, MemberIDs: []string{bruno.ID, anke.ID, agent.ID, stranger.ID, theo.ID, ada.ID, zora.ID}}
	s.Projects = append(s.Projects, library)
	bankGroup := &TodoGroup{ID: newID(), Title: "Bank", Color: "blue"}
	oldGroup := &TodoGroup{ID: newID(), Title: "Old", Trashed: true}
	receiptsTodo := &Todo{ID: newID(), Title: "Collect the receipts", Notes: "<p>All of them.</p>", AssigneeIDs: []string{stranger.ID, bruno.ID, olga.ID},
		NotifiedIDs: []string{anke.ID, ada.ID}, CreatedAt: s.now, CreatorID: bruno.ID,
		Subtasks: []*Subtask{{ID: newID(), Title: "Coins", AssigneeID: zora.ID}, {ID: newID(), Title: "Notes", AssigneeID: anke.ID}}}
	reconcileTodo := &Todo{ID: newID(), Title: "Reconcile the account", GroupID: bankGroup.ID, AssigneeIDs: []string{anke.ID}, CreatedAt: s.now, CreatorID: bruno.ID}
	forgottenTodo := &Todo{ID: newID(), Title: "File the old way", GroupID: oldGroup.ID, AssigneeIDs: []string{theo.ID}, CreatedAt: s.now, CreatorID: bruno.ID}
	trashedTodo := &Todo{ID: newID(), Title: "Thrown out", AssigneeIDs: []string{theo.ID}, Trashed: true, CreatedAt: s.now, CreatorID: bruno.ID}
	template := &TodoList{ID: newID(), Title: "Monthly closing (mm-yyyy)", Description: "<p>Every month.</p>", CreatorID: bruno.ID,
		Groups: []*TodoGroup{bankGroup, oldGroup}, Todos: []*Todo{receiptsTodo, reconcileTodo, forgottenTodo, trashedTodo},
		Comments: []*Comment{{ID: newID(), AuthorID: anke.ID, Body: "<p>Stays with the template</p>", CreatedAt: s.now}}}
	emptyTemplate := &TodoList{ID: newID(), Title: "Onboarding", CreatorID: anke.ID}
	archivedTemplate := &TodoList{ID: newID(), Title: "Yearly closing", Archived: true, ArchivedAt: s.now.Add(-time.Hour), CreatorID: bruno.ID}
	trashedTemplate := &TodoList{ID: newID(), Title: "Weekly closing", Trashed: true, CreatorID: bruno.ID}
	templates := &TodosTool{ID: newID(), ProjectID: library.ID, Title: "To-do list templates",
		Lists: []*TodoList{template, emptyTemplate, archivedTemplate, trashedTemplate}}
	s.TodosTools = []*TodosTool{todos, todosOutOfUse, otherTodos, templates}

	at := func(day, clock string) time.Time {
		moment, _ := time.Parse("2006-01-02 15:04", day+" "+clock)
		return moment
	}
	ankesEarlierAnswer := &CheckinAnswer{ID: newID(), AuthorID: anke.ID, GroupOn: "2026-09-10", Body: "<p>Built the stage.</p>", CreatedAt: at("2026-09-10", "17:30")}
	brunosAnswer := &CheckinAnswer{ID: newID(), AuthorID: bruno.ID, GroupOn: "2026-09-11", Body: "<p>Counted the tents.</p>", CreatedAt: at("2026-09-11", "17:10"),
		Comments: []*Comment{{ID: newID(), AuthorID: anke.ID, Body: "<p>Thank you!</p>", CreatedAt: at("2026-09-11", "18:30")}}}
	ankesAnswer := &CheckinAnswer{ID: newID(), AuthorID: anke.ID, GroupOn: "2026-09-11", Body: "<p>Fetched the speakers.</p>", CreatedAt: at("2026-09-11", "18:00")}
	dailyQuestion := &CheckinQuestion{ID: newID(), Title: "What did you do today?", Rhythm: RhythmDaily, Weekdays: []int{1, 2, 3, 4, 5}, TimeOfDay: "17:00",
		StartsOn: "2026-09-01", AskedIDs: []string{anke.ID, bruno.ID, client.ID}, CreatorID: anke.ID, CreatedAt: at("2026-09-01", "08:00"),
		Answers: []*CheckinAnswer{ankesEarlierAnswer, brunosAnswer, ankesAnswer},
		Reminders: []*CheckinReminder{
			{PersonID: bruno.ID, AskedFor: "2026-09-10", AskedAt: at("2026-09-10", "17:00")},
			{PersonID: bruno.ID, AskedFor: "2026-09-11", AskedAt: at("2026-09-11", "17:00")},
		}}
	weeklyQuestion := &CheckinQuestion{ID: newID(), Title: "What are you working on this week?", Rhythm: RhythmWeekly, Weekdays: []int{1}, TimeOfDay: "09:00",
		StartsOn: "2026-09-01", AskedIDs: []string{bruno.ID}, CreatorID: bruno.ID, CreatedAt: at("2026-09-01", "08:10"),
		Reminders: []*CheckinReminder{{PersonID: bruno.ID, AskedFor: "2026-09-07", AskedAt: at("2026-09-07", "09:00")}}}
	pausedQuestion := &CheckinQuestion{ID: newID(), Title: "How was the month?", Rhythm: RhythmMonthly, Weekdays: []int{5}, TimeOfDay: "16:00",
		StartsOn: "2026-09-01", Paused: true, AskedIDs: []string{bruno.ID}, CreatorID: bruno.ID, CreatedAt: at("2026-09-01", "08:20"),
		Reminders: []*CheckinReminder{{PersonID: bruno.ID, AskedFor: "2026-09-04", AskedAt: at("2026-09-04", "16:00")}}}
	checkinsTool := &CheckinsTool{ID: newID(), ProjectID: project.ID, Title: "Check-ins", Questions: []*CheckinQuestion{dailyQuestion, weeklyQuestion, pausedQuestion}}
	otherQuestion := &CheckinQuestion{ID: newID(), Title: "Not yours to answer", Rhythm: RhythmDaily, Weekdays: []int{1}, TimeOfDay: "09:00",
		StartsOn: "2026-09-01", AskedIDs: []string{stranger.ID}, CreatorID: stranger.ID, CreatedAt: at("2026-09-01", "08:00"),
		Answers: []*CheckinAnswer{{ID: newID(), AuthorID: stranger.ID, GroupOn: "2026-09-07", Body: "<p>Secret.</p>", CreatedAt: at("2026-09-07", "10:00")}}}
	otherCheckinsTool := &CheckinsTool{ID: newID(), ProjectID: otherProject.ID, Title: "Secret check-ins", Questions: []*CheckinQuestion{otherQuestion}}
	s.CheckinsTools = []*CheckinsTool{checkinsTool, otherCheckinsTool}

	announcement := &PostCategory{ID: newID(), ProjectID: project.ID, Name: "📢 Announcement"}
	heartbeat := &PostCategory{ID: newID(), ProjectID: project.ID, Name: "❤️ Heartbeat"}
	pinnedAt := at("2026-09-02", "09:00")
	pinnedPost := &Post{ID: newID(), Title: "Where the tent stands", Body: "<div>Behind the barn.</div>",
		PinnedAt: &pinnedAt, CreatedAt: at("2026-09-01", "09:00"), UpdatedAt: at("2026-09-01", "09:00"), CreatorID: anke.ID,
		SubscriberIDs: []string{anke.ID}}
	post := &Post{ID: newID(), Title: "The tent is ordered", Body: "<div>It comes on <strong>Friday</strong>.</div>", CategoryID: announcement.ID,
		CreatedAt: at("2026-09-12", "10:00"), UpdatedAt: at("2026-09-12", "10:00"), CreatorID: bruno.ID,
		SubscriberIDs: []string{bruno.ID, anke.ID},
		Comments:      []*Comment{{ID: newID(), AuthorID: anke.ID, Body: "<div>Great</div>", CreatedAt: at("2026-09-12", "11:00")}}}
	olderPost := &Post{ID: newID(), Title: "Who brings benches?", Body: "<div>We need twenty.</div>",
		CreatedAt: at("2026-09-10", "09:00"), UpdatedAt: at("2026-09-10", "09:00"), CreatorID: anke.ID, SubscriberIDs: []string{anke.ID}}
	draftPost := &Post{ID: newID(), Title: "Not sent yet", Body: "<div>Later.</div>", Draft: true,
		CreatedAt: at("2026-09-11", "09:00"), UpdatedAt: at("2026-09-11", "09:00"), CreatorID: bruno.ID, SubscriberIDs: []string{bruno.ID}}
	archivedPinnedAt := at("2026-09-03", "09:00")
	archivedPost := &Post{ID: newID(), Title: "Last year's plan", Body: "<div>Done with.</div>", CategoryID: heartbeat.ID,
		PinnedAt: &archivedPinnedAt, Archived: true, ArchivedAt: at("2026-09-11", "16:00"), ArchivedByID: anke.ID,
		CreatedAt: at("2026-09-05", "09:00"), UpdatedAt: at("2026-09-05", "09:00"), CreatorID: bruno.ID, SubscriberIDs: []string{bruno.ID},
		Comments: []*Comment{{ID: newID(), AuthorID: bruno.ID, Body: "<div>Keep it for later</div>", CreatedAt: at("2026-09-05", "10:00")}}}
	trashedPost := &Post{ID: newID(), Title: "Wrong date", Body: "<div>Sorry.</div>", Trashed: true,
		CreatedAt: at("2026-09-08", "09:00"), UpdatedAt: at("2026-09-08", "09:00"), CreatorID: bruno.ID, SubscriberIDs: []string{bruno.ID}}
	posts := &PostsTool{ID: newID(), ProjectID: project.ID, Title: "Message Board", ShownToClients: true,
		Posts: []*Post{olderPost, pinnedPost, archivedPost, post, draftPost, trashedPost}}
	postsOutOfUse := &PostsTool{ID: newID(), ProjectID: project.ID, Title: "Old announcements", Archived: true}
	otherCategory := &PostCategory{ID: newID(), ProjectID: otherProject.ID, Name: "🔒 Secret"}
	otherPost := &Post{ID: newID(), Title: "Not yours to read", Body: "<div>Secret.</div>",
		CreatedAt: at("2026-09-07", "09:00"), UpdatedAt: at("2026-09-07", "09:00"), CreatorID: stranger.ID, SubscriberIDs: []string{stranger.ID}}
	otherPosts := &PostsTool{ID: newID(), ProjectID: otherProject.ID, Title: "Secret board", Posts: []*Post{otherPost}}
	s.PostsTools = []*PostsTool{posts, postsOutOfUse, otherPosts}
	s.PostCategories = []*PostCategory{announcement, heartbeat, otherCategory}

	// The calendar of the project, in Berlin, where summer time ends on 25
	// October 2026: between the first and the second Monday of the series.
	s.TimeZone = "Europe/Berlin"
	moment := func(text string) *time.Time {
		at, err := time.Parse(time.RFC3339, text)
		if err != nil {
			panic(err)
		}
		return &at
	}
	weeklyEvent := &CalendarEvent{
		ID: newID(), Title: "Weekly planning", StartsAt: moment("2026-10-19T08:00:00Z"), EndsAt: moment("2026-10-19T08:30:00Z"),
		Repetition: &EventRepetition{
			Schedule: RepetitionSchedule{Rhythm: RepeatsWeekly, Weekdays: []int{1}, Interval: 1, StartsOn: "2026-10-19"},
			TimeZone: "Europe/Berlin",
		},
		Description: "<p>Plan the <strong>week</strong>.</p>", JoinURL: "https://meet.example.com/planning",
		ParticipantIDs: []string{anke.ID}, CreatedAt: s.now, CreatorID: bruno.ID,
	}
	onceEvent := &CalendarEvent{
		ID: newID(), Title: "Acceptance", StartsAt: moment("2026-10-21T12:00:00Z"), EndsAt: moment("2026-10-21T13:00:00Z"),
		Comments: []*Comment{
			{ID: newID(), AuthorID: anke.ID, Body: "<p>The room is <strong>booked</strong>.</p>", CreatedAt: at("2026-10-01", "09:00")},
			{ID: newID(), AuthorID: bruno.ID, Body: "<p>I bring the projector.</p>", CreatedAt: at("2026-10-01", "10:00")},
		},
		CreatedAt: s.now, CreatorID: anke.ID,
	}
	allDayEvent := &CalendarEvent{
		ID: newID(), Title: "Trade fair", FirstDay: "2026-10-20", LastDay: "2026-10-22", Highlighted: true,
		CreatedAt: s.now, CreatorID: bruno.ID,
	}
	calendar := &CalendarTool{ID: newID(), ProjectID: project.ID, Title: "Termine", Events: []*CalendarEvent{weeklyEvent, onceEvent, allDayEvent}}
	calendarOutOfUse := &CalendarTool{ID: newID(), ProjectID: project.ID, Title: "Old dates", Archived: true}
	otherEvent := &CalendarEvent{ID: newID(), Title: "Not yours to see", FirstDay: "2026-10-20", LastDay: "2026-10-20", CreatedAt: s.now, CreatorID: stranger.ID}
	otherCalendar := &CalendarTool{ID: newID(), ProjectID: otherProject.ID, Title: "Secret dates", Events: []*CalendarEvent{otherEvent}}
	s.Calendars = []*CalendarTool{calendar, calendarOutOfUse, otherCalendar}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", s.authenticated(s.me))
	mux.HandleFunc("DELETE /api/v1/me/token", s.authenticated(s.revokeOwnToken))
	mux.HandleFunc("PUT /api/v1/me/work", s.authenticated(s.noteAtWork))
	mux.HandleFunc("GET /api/v1/projects", s.authenticated(s.projects))
	mux.HandleFunc("PATCH /api/v1/projects/{projectId}", s.authenticated(s.editProject))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/boards", s.authenticated(s.boards))
	mux.HandleFunc("GET /api/v1/boards/{boardId}", s.authenticated(s.board))
	mux.HandleFunc("GET /api/v1/boards/{boardId}/archive", s.authenticated(s.archivedCards))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/archive", s.authenticated(s.archiveCard))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/unarchive", s.authenticated(s.unarchiveCard))
	mux.HandleFunc("GET /api/v1/me/cards", s.authenticated(s.myCards))
	mux.HandleFunc("GET /api/v1/cards/{cardId}", s.authenticated(s.card))
	mux.HandleFunc("GET /api/v1/cards/{first}/{second}", s.authenticated(s.shortIdentifierOrNotes))
	mux.HandleFunc("POST /api/v1/boards/{boardId}/cards", s.authenticated(s.addCard))
	mux.HandleFunc("PATCH /api/v1/cards/{cardId}", s.authenticated(s.updateCard))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/moves", s.authenticated(s.moveCard))
	mux.HandleFunc("PUT /api/v1/columns/{columnId}/on-hold", s.authenticated(s.setColumnOnHold))
	s.registerColumnsRoutes(mux)
	mux.HandleFunc("POST /api/v1/cards/{cardId}/comments", s.authenticated(s.writeComment))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/comments/{commentId}/reactions", s.authenticated(s.reactToComment))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/comments/{commentId}/reactions", s.authenticated(s.unreactFromComment))
	mux.HandleFunc("POST /api/v1/chats/{chatId}/messages/{messageId}/reactions", s.authenticated(s.reactToMessage))
	mux.HandleFunc("DELETE /api/v1/chats/{chatId}/messages/{messageId}/reactions", s.authenticated(s.unreactFromMessage))
	mux.HandleFunc("PATCH /api/v1/cards/{cardId}/comments/{commentId}", s.authenticated(s.editComment))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/comments/{commentId}", s.authenticated(s.removeComment))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}", s.authenticated(s.removeCard))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/restore", s.authenticated(s.restoreCard))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/trash", s.authenticated(s.trash))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/people", s.authenticated(s.people))
	mux.HandleFunc("GET /api/v1/messages/{conversationId}/people", s.authenticated(s.conversationPeople))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/assignees", s.authenticated(s.assignCard))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/assignees/{personId}", s.authenticated(s.unassignCard))
	mux.HandleFunc("PUT /api/v1/cards/{cardId}/notes", s.authenticated(s.writeCardNotes))
	s.registerCardBackRoutes(mux)
	mux.HandleFunc("POST /api/v1/cards/{cardId}/steps", s.authenticated(s.addStep))
	mux.HandleFunc("PATCH /api/v1/cards/{cardId}/steps/{stepId}", s.authenticated(s.updateStep))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/steps/{stepId}/moves", s.authenticated(s.moveStep))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/steps/{stepId}", s.authenticated(s.removeStep))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/steps/{stepId}/card", s.authenticated(s.makeStepIntoCard))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/subscribers/{personId}", s.authenticated(s.subscribeToCard))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/subscribers/{personId}", s.authenticated(s.unsubscribeFromCard))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/chats", s.authenticated(s.chats))
	mux.HandleFunc("GET /api/v1/messages", s.authenticated(s.conversations))
	mux.HandleFunc("POST /api/v1/messages/{first}/{second}", s.authenticated(s.beginOrUpload))
	mux.HandleFunc("GET /api/v1/chats/{chatId}", s.authenticated(s.chat))
	mux.HandleFunc("GET /api/v1/chats/{chatId}/messages", s.authenticated(s.messages))
	mux.HandleFunc("POST /api/v1/chats/{chatId}/messages", s.authenticated(s.postMessage))
	mux.HandleFunc("PATCH /api/v1/chats/{chatId}/messages/{messageId}", s.authenticated(s.editMessage))
	mux.HandleFunc("DELETE /api/v1/chats/{chatId}/messages/{messageId}", s.authenticated(s.removeMessage))
	mux.HandleFunc("POST /api/v1/chats/{chatId}/read", s.authenticated(s.markChatRead))
	mux.HandleFunc("POST /api/v1/projects/{projectId}/uploads", s.authenticated(s.upload))
	mux.HandleFunc("GET /api/v1/uploads/{blobId}", s.authenticated(s.file))
	mux.HandleFunc("GET /api/v1/uploads/{blobId}/{variant}", s.answeringInBytes(s.fileBytes))
	s.registerFilesRoutes(mux)
	s.registerSearchRoutes(mux)
	s.registerTodosRoutes(mux)
	s.registerCheckinsRoutes(mux)
	s.registerPostsRoutes(mux)
	s.registerCalendarRoutes(mux)
	s.registerReminderRoutes(mux)
	s.registerNotificationsRoutes(mux)
	s.registerConventionsRoutes(mux)
	s.registerLabelRoutes(mux)
	s.registerNextCardRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { unknownAddress(w) })

	s.Server = httptest.NewServer(s.recording(mux))
	t.Cleanup(s.Close)

	return s, &Fixture{
		Token: "minka_pat_bruno", StrangerToken: "minka_pat_stranger", AgentToken: "minka_pat_agent",
		Bruno: bruno, Anke: anke, Stranger: stranger, Client: client, ForeignClient: foreignClient, Agent: agent, RetiredAgent: retired, Project: project, Board: board,
		Triage: triage, Doing: doing, NotNow: notNow, Done: done,
		CardInTriage: cardInTriage, CardInDoing: cardInDoing,
		OtherProject: otherProject, OtherBoard: otherBoard, OtherCard: otherCard,
		Chat: chat, OtherChat: otherChat, Conversation: conversation,
		Files: files, Folder: folder, ArchivedFolder: archivedFolder, FolderShownToClients: folderShownToClients,
		Document: document, DocumentInFolder: documentInFolder,
		DocumentBelowTheArchivedFolder: documentBelowTheArchivedFolder,
		DraftDocument:                  draftDocument, ArchivedDocument: archivedDocument,
		FilesUpload: filesUpload, OtherFiles: otherFiles, OtherDocument: otherDocument,
		Todos: todos, TodoList: todoList, StageGroup: stageGroup, KitchenGroup: kitchenGroup,
		LooseTodo: looseTodo, GroupedTodo: groupedTodo, DoneTodo: doneTodo,
		EmptyTodoList: emptyTodoList, ArchivedList: archivedList, ArchivedGroup: archivedGroup,
		TodosOutOfUse: todosOutOfUse, OtherTodos: otherTodos, OtherTodoList: otherTodoList,
		ArchivedListTodo: archivedListTodo, OtherTodo: otherTodo, Series: series,
		Theo: theo, Ada: ada, Zora: zora, Olga: olga, TemplateLibrary: library, Templates: templates, Template: template, EmptyTemplate: emptyTemplate,
		ArchivedTemplate: archivedTemplate, TrashedTemplate: trashedTemplate, TemplateTodo: receiptsTodo,
		CheckinsTool: checkinsTool, DailyQuestion: dailyQuestion, WeeklyQuestion: weeklyQuestion, PausedQuestion: pausedQuestion,
		BrunosAnswer: brunosAnswer, AnkesAnswer: ankesAnswer, AnkesEarlierAnswer: ankesEarlierAnswer,
		OtherCheckinsTool: otherCheckinsTool, OtherQuestion: otherQuestion,
		Posts: posts, PinnedPost: pinnedPost, Post: post, OlderPost: olderPost, DraftPost: draftPost,
		ArchivedPost: archivedPost, TrashedPost: trashedPost, Announcement: announcement, Heartbeat: heartbeat,
		PostsOutOfUse: postsOutOfUse, OtherPosts: otherPosts, OtherPost: otherPost, OtherCategory: otherCategory,
		Calendar: calendar, CalendarOutOfUse: calendarOutOfUse, OtherCalendar: otherCalendar,
		WeeklyEvent: weeklyEvent, OnceEvent: onceEvent, AllDayEvent: allDayEvent, OtherEvent: otherEvent,
	}
}

// Requests returns the requests the server received so far.
func (s *Server) Requests() []RecordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

// WritingRequests returns the requests that were not GET.
func (s *Server) WritingRequests() []RecordedRequest {
	var writing []RecordedRequest
	for _, r := range s.Requests() {
		if r.Method != http.MethodGet {
			writing = append(writing, r)
		}
	}
	return writing
}

// PutInTheArchive takes a card off its board into the archive of the board, as
// archiving it does. It leaves its column, and the column stays with it: that
// is the column an archived card names, on a board where it may have been
// deleted since.
//
// The entry comes back, so that a test can say when and by whom the card was
// put away, which is what the archive is ordered and listed by.
func (s *Server) PutInTheArchive(card *Card) *ArchivedCard {
	for _, board := range s.Boards {
		for _, column := range board.Columns {
			if !slices.Contains(column.Cards, card) {
				continue
			}
			column.Cards = slices.DeleteFunc(column.Cards, func(c *Card) bool { return c == card })
			entry := &ArchivedCard{Card: card, Column: column, At: s.now}
			s.Archive = append(s.Archive, entry)
			return entry
		}
	}
	return nil
}

// TakeOutOfTheArchive brings a card back to the top of the column it was taken
// from and off hold, as unarchiving it does. The moment and the person go with
// the entry: nothing left on the card says it was ever in an archive.
func (s *Server) TakeOutOfTheArchive(card *Card) {
	archived := s.inTheArchive(card.ID)
	if archived == nil {
		return
	}
	s.Archive = slices.DeleteFunc(s.Archive, func(a *ArchivedCard) bool { return a.Card == card })
	card.OnHold = false
	column := s.columnToUnarchiveInto(archived)
	column.Cards = append([]*Card{card}, column.Cards...)
}

// columnToUnarchiveInto is the column a card comes back to out of the archive:
// the one it was taken from while that stands on the board, else the first
// working column, else Triage.
func (s *Server) columnToUnarchiveInto(archived *ArchivedCard) *Column {
	board := s.boardOfColumn(archived.Column)
	if board == nil || slices.Contains(board.Columns, archived.Column) {
		return archived.Column
	}
	for _, kind := range []string{"column", "triage"} {
		if i := slices.IndexFunc(board.Columns, func(c *Column) bool { return c.Type == kind }); i >= 0 {
			return board.Columns[i]
		}
	}
	return archived.Column
}

// FailNextReadWith makes the next GET answer as FailNextWith does, so that a
// test can let a read fail after the writes before it went through.
func (s *Server) FailNextReadWith(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failReadWith = status
}

// FailNextWith makes the next request answer with this status and a body that
// is not JSON, like a proxy in front of the server would.
func (s *Server) FailNextWith(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failWith = status
}

// FailNextWithServerError makes the next request answer as Minka answers an
// exception under the API: 500 with the error code serverError and the
// reference of the entry in its log. It takes precedence over a pending
// FailNextWith.
func (s *Server) FailNextWithServerError(reference string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failWithServerError = reference
}

func (s *Server) recording(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		s.mu.Lock()
		s.requests = append(s.requests, RecordedRequest{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body})
		failWith := s.failWith
		s.failWith = 0
		if r.Method == http.MethodGet && s.failReadWith != 0 {
			failWith, s.failReadWith = s.failReadWith, 0
		}
		reference := s.failWithServerError
		s.failWithServerError = ""
		s.mu.Unlock()

		if reference != "" {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": map[string]any{"code": "serverError", "message": "Something went wrong on the server. The reference names the entry in its log."},
				"meta":  map[string]any{"reference": reference},
			})
			return
		}

		if failWith != 0 {
			w.Header().Set("Content-Type", "text/html")
			if failWith >= 300 && failWith < 400 {
				w.Header().Set("Location", "https://elsewhere.example/")
			}
			w.WriteHeader(failWith)
			_, _ = io.WriteString(w, "<html><body>Something in between</body></html>")
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		if s.CLILatest != "" {
			w.Header().Set("Minka-CLI-Latest", s.CLILatest)
		}
		if s.CLIMinimum != "" {
			w.Header().Set("Minka-CLI-Minimum", s.CLIMinimum)
		}
		next.ServeHTTP(w, r)
	})
}

type handler func(w http.ResponseWriter, r *http.Request, person *Person)

func (s *Server) authenticated(next handler) http.HandlerFunc {
	return s.signedIn(true, next)
}

// answeringInBytes is the one address that answers a file instead of JSON.
// What the request accepts is therefore no reason to turn it away: a client
// asking for the very picture it is about to be handed would be refused.
func (s *Server) answeringInBytes(next handler) http.HandlerFunc {
	return s.signedIn(false, next)
}

func (s *Server) signedIn(answersInJSON bool, next handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		if answersInJSON && accept != "" && !strings.Contains(accept, "application/json") && !strings.Contains(accept, "*/*") {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}

		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		personID, known := s.Tokens[token]
		if !found || !known {
			w.Header().Set("WWW-Authenticate", `Bearer realm="Minka"`)
			writeRaw(w, http.StatusUnauthorized, UnauthenticatedBody)
			return
		}

		if carriesArgumentsOfWebForms(r) {
			malformed(w, "The body carries form arguments of the web.")
			return
		}
		identifiersOfTheAddressInAnyCase(r)
		next(w, r, s.People[personID])
	}
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request, person *Person) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id": person.ID, "name": person.Name, "emailAddress": person.Email,
		"kind": kindOf(person), "retired": person.Retired, "principal": s.principalOf(person),
		"organizationRole": "member",
	})
}

// revokeOwnToken revokes the token the request carries and no other. A token
// that is gone already never reaches it: the sign-in answers 401 first.
func (s *Server) revokeOwnToken(w http.ResponseWriter, r *http.Request, _ *Person) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	delete(s.Tokens, token)
	w.WriteHeader(http.StatusNoContent)
}

// noteAtWork takes the whole set of cards one client of an agent is at work
// on. The set replaces what that client said before, and an empty one says
// that it has stopped.
//
// A person's token is turned away before anything of the body is read, so that
// nobody learns from the refusal what the body would have had to be. A card
// the agent may not see, and an identifier that is no UUID, are passed over
// without a word: the answer must not say whether a card somebody made up
// exists.
func (s *Server) noteAtWork(w http.ResponseWriter, r *http.Request, person *Person) {
	if kindOf(person) != "agent" {
		writeJSON(w, http.StatusForbidden, errorBody("agentsOnly", "Only an agent says which cards it is working on."))
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	clientID, ok := agentClientID(w, fields)
	if !ok {
		return
	}
	named, ok := stringListField(w, fields, "cardIds")
	if !ok {
		return
	}

	cards := []string{}
	var leftOut []any
	var leftOutIDs []string
	for _, id := range named {
		if !uuidPattern.MatchString(id) {
			continue
		}
		if _, _, card := s.visibleCard(id, person); card == nil {
			// A card in the archive is named in the answer, where a card that
			// is nothing to this agent is passed over: nobody works on a card
			// in the archive, and the rest of the set has to go on glowing.
			if _, _, archived := s.visibleArchivedCard(id, person); archived != nil && !slices.Contains(leftOutIDs, id) {
				leftOutIDs = append(leftOutIDs, id)
				leftOut = append(leftOut, map[string]any{"cardId": id, "reason": s.leftOutReason()})
			}
			continue
		}
		cards = append(cards, id)
	}

	s.atWork = slices.DeleteFunc(s.atWork, func(row AtWork) bool {
		return row.AgentID == person.ID && row.ClientID == clientID
	})
	for _, card := range cards {
		s.atWork = append(s.atWork, AtWork{AgentID: person.ID, ClientID: clientID, CardID: card, LastSeenAt: s.now})
	}

	if len(leftOut) == 0 {
		// 204 and no body, not even a content type.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// The cards it took are noted; the answer says which it did not take.
	writeJSON(w, http.StatusOK, map[string]any{"leftOut": leftOut})
}

// leftOutReason is the word the fake gives for a card it leaves out of a set at
// work.
func (s *Server) leftOutReason() string {
	if s.LeavesOutWithReason != "" {
		return s.LeavesOutWithReason
	}
	return "cardArchived"
}

// agentClientID reads which client of an agent is speaking. Only a field left
// out or null names none. An empty name, one that is too long or one that
// carries anything else is refused rather than read as the client that names
// none: two clients quietly sharing one set of cards is the mix-up the field is
// there to prevent.
func agentClientID(w http.ResponseWriter, fields map[string]any) (string, bool) {
	named, ok := stringField(w, fields, "clientId")
	if !ok {
		return "", false
	}
	if named == nil {
		return "", true
	}
	if !clientIDPattern.MatchString(*named) {
		malformed(w, fmt.Sprintf("The field \"clientId\" takes 1 to %d letters, digits, \".\", \"-\" and \"_\".", maxClientIDLength))
		return "", false
	}
	return *named, true
}

// AtWorkOf is the set one client of an agent last said it is working on.
func (s *Server) AtWorkOf(agentID, clientID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cards := []string{}
	for _, row := range s.atWork {
		if row.AgentID == agentID && row.ClientID == clientID {
			cards = append(cards, row.CardID)
		}
	}
	slices.Sort(cards)
	return cards
}

// CardsAtWork is every card somebody is at work on, whoever said so.
func (s *Server) CardsAtWork() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cards := []string{}
	for _, row := range s.atWork {
		if !slices.Contains(cards, row.CardID) {
			cards = append(cards, row.CardID)
		}
	}
	slices.Sort(cards)
	return cards
}

// kindOf says what sort of being somebody is: an account that acts for a human
// is an agent, everybody else is a human.
func kindOf(person *Person) string {
	if person.PrincipalID != "" {
		return "agent"
	}
	return "human"
}

// principalOf names the human an agent acts for, and nothing for anybody else.
func (s *Server) principalOf(person *Person) any {
	if person.PrincipalID == "" {
		return nil
	}
	principal := s.People[person.PrincipalID]
	return map[string]any{"id": principal.ID, "name": principal.Name}
}

// projects lists the projects in use, or the archived ones when asked for
// them. An empty status counts as none given; anything else but the two is
// refused, a project in the trash included.
func (s *Server) projects(w http.ResponseWriter, r *http.Request, person *Person) {
	if s.IgnoresProjectStatus {
		list := []any{}
		for _, p := range s.Projects {
			if slices.Contains(p.MemberIDs, person.ID) && !p.TemplateLibrary {
				list = append(list, map[string]any{"id": p.ID, "name": p.Name, "description": p.Description, "appUrl": s.URL + "/projects/" + p.ID})
			}
		}
		writeJSON(w, http.StatusOK, list)
		return
	}

	archived := false
	switch r.URL.Query().Get("status") {
	case "", "active":
	case "archived":
		archived = true
	default:
		writeJSON(w, http.StatusBadRequest, errorBody("unknownProjectStatus", `The status a list is asked for is either "active" or "archived".`))
		return
	}

	list := []any{}
	for _, p := range s.Projects {
		if slices.Contains(p.MemberIDs, person.ID) && p.Archived == archived && !p.TemplateLibrary {
			list = append(list, s.projectResource(p))
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) projectResource(p *Project) map[string]any {
	return map[string]any{"id": p.ID, "name": p.Name, "description": p.Description, "status": projectStatus(p), "appUrl": s.URL + "/projects/" + p.ID}
}

// editProject renames or describes a project. Only the team may write, so a
// client is told the same 404 as somebody outside the project, before anything
// about the body; an archived project answers last.
func (s *Server) editProject(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	name, ok := stringField(w, fields, "name")
	if !ok {
		return
	}
	description, ok := stringField(w, fields, "description")
	if !ok {
		return
	}
	if name == nil && description == nil {
		unprocessable(w, "nothingToChange", "The body names neither a name nor a description.")
		return
	}
	if name != nil && !validProjectName(w, *name) {
		return
	}
	if description != nil && utf8.RuneCountInString(phpTrim(*description)) > maxProjectDescriptionLength {
		unprocessable(w, "projectDescriptionTooLong", "The description of a project is too long.")
		return
	}
	if s.refusedAsArchived(w, project) {
		return
	}
	if name != nil {
		project.Name = phpTrim(*name)
	}
	if description != nil {
		project.Description = phpTrim(*description)
	}
	writeJSON(w, http.StatusOK, s.projectResource(project))
}

// maxProjectNameLength and maxProjectDescriptionLength count characters, as
// Minka does.
const (
	maxProjectNameLength        = 200
	maxProjectDescriptionLength = 2000
)

func validProjectName(w http.ResponseWriter, name string) bool {
	name = phpTrim(name)
	switch {
	case utf8.RuneCountInString(name) > maxProjectNameLength:
		unprocessable(w, "projectNameTooLong", "The name of a project is too long.")
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		unprocessable(w, "projectNameHasControlCharacters", "The name of a project must not contain control characters.")
	case name == "":
		unprocessable(w, "projectNameMissing", "A project needs a name.")
	default:
		return true
	}
	return false
}

func projectStatus(p *Project) string {
	if p.Archived {
		return "archived"
	}
	return "active"
}

// refusedAsArchived answers a write into an archived project. It is asked
// after the body was found valid and the thing was found, as the server does:
// a member learns why nothing was written, and anybody else got a 404 before.
func (s *Server) refusedAsArchived(w http.ResponseWriter, project *Project) bool {
	if project == nil || !project.Archived {
		return false
	}
	unprocessable(w, "projectArchived", "This project is archived. Nothing is written in it until it is put back in use.")
	return true
}

func (s *Server) projectOfCard(cardID string) *Project {
	for _, board := range s.Boards {
		for _, column := range board.Columns {
			for _, card := range column.Cards {
				if card.ID == cardID {
					return s.projectOf(board)
				}
			}
		}
	}
	if archived := s.inTheArchive(cardID); archived != nil {
		if board := s.boardOfColumn(archived.Column); board != nil {
			return s.projectOf(board)
		}
	}
	return nil
}

// projectOfChat is nil for a private conversation, which hangs on no project.
func (s *Server) projectOfChat(chat *Chat) *Project {
	for _, p := range s.Projects {
		if p.ID == chat.ProjectID {
			return p
		}
	}
	return nil
}

func (s *Server) boards(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	list := []any{}
	for _, b := range s.Boards {
		if b.ProjectID == project.ID && !b.Trashed {
			list = append(list, map[string]any{"id": b.ID, "title": b.Title, "appUrl": s.boardURL(b)})
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) board(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil {
		notFound(w)
		return
	}

	columns := []any{}
	for _, column := range board.Columns {
		columns = append(columns, s.columnResource(board, column, person))
	}
	fields := map[string]any{
		"id": board.ID, "projectId": board.ProjectID, "title": board.Title, "appUrl": s.boardURL(board), "columns": columns,
	}
	// The board names the reader's own entry and nobody else's; a client, who
	// may not coordinate, gets no field at all.
	if !s.isAClient(person.ID) {
		fields["coordinator"] = s.coordinatorResource(board, person)
		fields["conventions"] = conventionsResource(board, s)
	}
	writeJSON(w, http.StatusOK, fields)
}

// archivedCardsLimit is the shape a limit has to have, as the controller reads
// it: a whole number, and nothing that could overflow on the way in.
var archivedCardsLimit = regexp.MustCompile(`^[0-9]{1,18}$`)

// archivedCards answers a page of the archive of a board, the card put away
// last first.
//
// The order of the refusals is the controller's: a board that is not there, or
// belongs to a project this person is not in, beats both refusals of the query.
// A limit that is no whole number is refused; one above the largest page is not,
// it is cut down to it without a word.
func (s *Server) archivedCards(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil {
		notFound(w)
		return
	}

	limit := ArchivedCardsOnAPage
	if given := r.URL.Query().Get("limit"); given != "" {
		asked, err := strconv.ParseInt(given, 10, 64)
		if !archivedCardsLimit.MatchString(given) || err != nil || asked < 1 {
			unprocessable(w, "archivedCardsLimitNotANumber", "The number of cards asked for has to be a whole number of at least one.")
			return
		}
		limit = min(int(asked), MostArchivedCardsOnAPage)
	}

	var boundary *ArchivedCard
	if before := strings.ToLower(r.URL.Query().Get("before")); before != "" {
		if uuidPattern.MatchString(before) {
			boundary = s.inTheArchiveOf(board, before)
		}
		if boundary == nil || slices.Contains(s.ForgetsTheArchiveBoundariesOf, before) {
			// One status decides both the cursor and the listing, so a card this
			// page may not start behind is a card out of the archive: it is gone
			// from every page from here on, as it is gone for the cursor.
			if boundary != nil {
				s.TakeOutOfTheArchive(boundary.Card)
			}
			unprocessable(w, "archivedCardsBeforeUnknown", "The page starts behind a card in the archive of this board, named by its id.")
			return
		}
		if raced := slices.Index(s.RacesOnTheArchiveBoundariesOf, before); raced >= 0 {
			// The card went between the check above and the read below, so the
			// page comes back empty although the list goes on.
			s.RacesOnTheArchiveBoundariesOf = slices.Delete(s.RacesOnTheArchiveBoundariesOf, raced, raced+1)
			writeJSON(w, http.StatusOK, []any{})
			return
		}
	}

	var archive []*ArchivedCard
	for _, archived := range s.Archive {
		if s.boardOfColumn(archived.Column) != board {
			continue
		}
		if boundary != nil && archiveOrder(archived, boundary) <= 0 {
			continue
		}
		archive = append(archive, archived)
	}
	slices.SortStableFunc(archive, archiveOrder)

	list := []any{}
	for _, archived := range archive[:min(len(archive), limit)] {
		list = append(list, s.archivedCardResource(board, archived))
	}
	writeJSON(w, http.StatusOK, list)
}

// archiveOrder is the order the archive of a board is listed in: the card put
// away last first, one whose moment nobody noted as the oldest, and the id
// deciding between two cards put away in the same moment.
func archiveOrder(a, b *ArchivedCard) int {
	if !a.At.Equal(b.At) {
		return b.At.Compare(a.At)
	}
	return strings.Compare(a.Card.ID, b.Card.ID)
}

// inTheArchiveOf is the entry of this board's archive for a card, and nil for
// anything that does not lie in it.
func (s *Server) inTheArchiveOf(board *Board, cardID string) *ArchivedCard {
	archived := s.inTheArchive(cardID)
	if archived == nil || s.boardOfColumn(archived.Column) != board {
		return nil
	}
	return archived
}

func (s *Server) archivedCardResource(board *Board, archived *ArchivedCard) map[string]any {
	return s.withShortURL(archived.Card.ShortID, map[string]any{
		"id": archived.Card.ID, "projectId": board.ProjectID, "shortId": nullable(archived.Card.ShortID), "title": archived.Card.Title,
		"archivedAt": timestampOrNil(nilIfZero(archived.At)), "archivedBy": s.personReferenceOrNil(archived.ByID),
		"appUrl": s.cardURL(board, archived.Card),
	})
}

// archiveCard takes a card off its board into the archive of the board. A card
// that lies there already is refused with its own code rather than answered as
// though it had just been put away, and an archived project comes before that.
func (s *Server) archiveCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	onItsBoard := card != nil
	if card == nil {
		board, column, card = s.visibleArchivedCard(r.PathValue("cardId"), person)
	}
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return
	}
	if !onItsBoard {
		unprocessable(w, "cardArchived", "This card is archived. Nothing is written on it until it is back on its board.")
		return
	}

	entry := s.PutInTheArchive(card)
	entry.ByID = person.ID
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card, person))
}

// unarchiveCard brings a card back out of the archive onto its board. A card
// that stands on its board is refused with its own code, and an archived
// project comes before that.
func (s *Server) unarchiveCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	onItsBoard := card != nil
	if card == nil {
		board, column, card = s.visibleArchivedCard(r.PathValue("cardId"), person)
	}
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return
	}
	if onItsBoard {
		unprocessable(w, "cardNotArchived", "This card is not archived.")
		return
	}

	column = s.columnToUnarchiveInto(s.inTheArchive(card.ID))
	s.TakeOutOfTheArchive(card)
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card, person))
}

// columnResource lists the cards of a column in its two areas, and counts the
// ones on hold apart from the rest.
func (s *Server) columnResource(board *Board, column *Column, reader *Person) map[string]any {
	var listed, onHold []*Card
	for _, card := range column.Cards {
		if card.OnHold {
			onHold = append(onHold, card)
		} else {
			listed = append(listed, card)
		}
	}
	count := len(listed)
	if column.Type == "done" {
		// Done is ordered by completion, the card completed last first.
		slices.SortStableFunc(listed, func(a, b *Card) int { return completion(b).Compare(completion(a)) })
		if len(listed) > 25 {
			listed = listed[:25]
		}
	}
	resource := map[string]any{
		"id": column.ID, "type": column.Type, "title": columnTitle(column), "customTitle": column.CustomTitle,
		"color": shownColor(column), "cardCount": count, "cards": s.boardCardResources(board, listed, reader),
		"onHoldEnabled": column.OnHoldEnabled, "onHoldCardCount": len(onHold), "onHoldCards": s.boardCardResources(board, onHold, reader),
	}
	if !s.IgnoresColumnRoles {
		resource["role"] = roleOf(column)
	}
	return resource
}

func (s *Server) boardCardResources(board *Board, cards []*Card, reader *Person) []any {
	list := []any{}
	for _, card := range cards {
		list = append(list, s.withShortURL(card.ShortID, map[string]any{
			"id": card.ID, "shortId": nullable(card.ShortID), "title": card.Title, "hasDescription": card.Description != "",
			"stepCount": len(card.Steps), "completedStepCount": s.completedStepCount(card, reader),
			"commentCount": len(card.Comments), "createdAt": timestamp(card.CreatedAt),
			"creator": s.personReference(card.CreatorID), "completedAt": timestampOrNil(card.CompletedAt),
			"completedBy": s.personReferenceOrNil(card.CompletedByID), "assignees": s.assigneeReferences(card),
			"dueOn": card.DueOn, "labels": s.labelResources(s.LabelsOn(card.ID)), "appUrl": s.cardURL(board, card),
		}))
	}
	return list
}

// myCards lists the cards the person is assigned to, in the order the server
// gives them: projects by name, then by due date with the ones without a date
// last, then by title. Done is left out, as are cards of a project the person
// is not a member of. boardId and columnId narrow them, compared in lowercase
// as given: an ID that matches nothing, malformed or not, leaves none.
func (s *Server) myCards(w http.ResponseWriter, r *http.Request, person *Person) {
	boardID := strings.ToLower(r.URL.Query().Get("boardId"))
	columnID := strings.ToLower(r.URL.Query().Get("columnId"))
	type assigned struct {
		board  *Board
		column *Column
		card   *Card
	}
	var mine []assigned
	for _, board := range s.Boards {
		if s.visibleProject(board.ProjectID, person) == nil || !s.openForWork(board) {
			continue
		}
		for _, column := range board.Columns {
			for _, card := range column.Cards {
				if openCard(column, card) && slices.Contains(card.AssigneeIDs, person.ID) &&
					(boardID == "" || board.ID == boardID) && (columnID == "" || column.ID == columnID) {
					mine = append(mine, assigned{board, column, card})
				}
			}
		}
	}

	slices.SortStableFunc(mine, func(one, other assigned) int {
		if order := strings.Compare(s.projectOf(one.board).Name, s.projectOf(other.board).Name); order != 0 {
			return order
		}
		if order := strings.Compare(dayOrLast(one.card.DueOn), dayOrLast(other.card.DueOn)); order != 0 {
			return order
		}
		return strings.Compare(one.card.Title, other.card.Title)
	})

	list := []any{}
	for _, item := range mine {
		list = append(list, s.cardResource(item.board, item.column, item.card, person))
	}
	writeJSON(w, http.StatusOK, list)
}

// dayOrLast sorts a card without a due date behind every card that has one.
func dayOrLast(dueOn *string) string {
	if dueOn == nil {
		return "9999-99-99"
	}
	return *dueOn
}

// openForWork says whether a card on a board can be somebody's to do. A board
// in the trash and an archived project have nothing left to do, so their
// cards are neither among anybody's nor released, although each card can
// still be read by itself in an archived project.
func (s *Server) openForWork(board *Board) bool {
	return !board.Trashed && !s.projectOf(board).Archived
}

func (s *Server) projectOf(board *Board) *Project {
	for _, project := range s.Projects {
		if project.ID == board.ProjectID {
			return project
		}
	}
	return &Project{}
}

// card answers one card, from its board or from the archive of its board: a
// card in the archive is read at its own address like any other.
func (s *Server) card(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		board, column, card = s.visibleArchivedCard(r.PathValue("cardId"), person)
	}
	if card == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card, person))
}

// cardByShortIdentifier finds a card under the name people call it by, read
// in any case. Whatever names no card the person can see answers like an
// address that was never there.
func (s *Server) cardByShortIdentifier(w http.ResponseWriter, r *http.Request, person *Person) {
	identifier := strings.ToUpper(strings.TrimSpace(r.PathValue("identifier")))
	for _, board := range s.Boards {
		if s.visibleProject(board.ProjectID, person) == nil {
			continue
		}
		for _, column := range board.Columns {
			for _, card := range column.Cards {
				if goesBy(board, card, identifier) {
					resource := s.cardResource(board, column, card, person)
					if s.AnswersCardsWithoutID {
						resource["id"] = ""
					}
					writeJSON(w, http.StatusOK, resource)
					return
				}
			}
		}
	}
	notFound(w)
}

func (s *Server) addCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok1 := stringField(w, fields, "title")
	columnID, ok2 := stringField(w, fields, "columnId")
	description, ok3 := stringField(w, fields, "description")
	if !ok1 || !ok2 || !ok3 {
		return
	}
	storedTitle, ok := cardTitle(w, deref(title))
	if !ok {
		return
	}
	dueOn, ok := stringField(w, fields, "dueOn")
	if !ok || !validDueDate(w, dueOn, "cardDueDateNotADate") {
		return
	}
	assigneeIDs, ok := stringListField(w, fields, "assigneeIds")
	if !ok {
		return
	}
	for _, id := range assigneeIDs {
		if !uuidPattern.MatchString(id) {
			notAProjectMember(w)
			return
		}
	}
	labelIDs, ok := stringListField(w, fields, "labelIds")
	if !ok {
		return
	}
	labels, ok := s.labelsForANewCard(w, board, person, labelIDs)
	if !ok {
		return
	}
	if description != nil && !validDescription(w, *description) {
		return
	}
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return
	}
	var column *Column
	for _, c := range board.Columns {
		if (columnID == nil || *columnID == "") && c.Type == "triage" || columnID != nil && c.ID == *columnID {
			column = c
		}
	}
	if column == nil || column.Type == "not_now" || column.Type == "done" {
		notFound(w)
		return
	}

	// Card, people and labels are one transaction: somebody outside the
	// project or two labels of one scope leave no card behind.
	for _, id := range assigneeIDs {
		if !slices.Contains(s.visibleProject(board.ProjectID, person).MemberIDs, id) {
			notAProjectMember(w)
			return
		}
	}
	if labelsShareAScope(labels) {
		unprocessable(w, "labelsShareAScope", "A card takes only one label of each scope.")
		return
	}

	card := &Card{ID: newID(), Title: storedTitle, Description: phpTrim(harmlessStripped(deref(description))), CreatedAt: s.now, CreatorID: person.ID, DueOn: dueDate(dueOn)}
	card.subscribe(person.ID)
	for _, id := range assigneeIDs {
		if !slices.Contains(card.AssigneeIDs, id) {
			card.AssigneeIDs = append(card.AssigneeIDs, id)
			card.subscribe(id)
		}
	}
	for _, label := range labels {
		if !slices.Contains(s.ItemLabels[card.ID], label.ID) {
			s.PutLabelOn(card.ID, label)
		}
	}
	column.Cards = append([]*Card{card}, column.Cards...)
	w.Header().Set("Location", s.URL+"/api/v1/cards/"+card.ID)
	writeJSON(w, http.StatusCreated, s.cardResource(board, column, card, person))
}

// updateCard finds a card of the archive as well: why nothing was written on
// it is asked after the body, as the server does.
func (s *Server) updateCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok1 := stringField(w, fields, "title")
	description, ok2 := stringField(w, fields, "description")
	if !ok1 || !ok2 {
		return
	}
	// A due date of null removes it, so the field counts as named even then.
	_, dueDateNamed := fields["dueOn"]
	if title == nil && description == nil && !dueDateNamed {
		unprocessable(w, "nothingToChange", "The body names neither a title nor a description nor a due date.")
		return
	}
	var storedTitle string
	if title != nil {
		if storedTitle, ok = cardTitle(w, *title); !ok {
			return
		}
	}
	dueOn, ok := stringField(w, fields, "dueOn")
	if !ok || !validDueDate(w, dueOn, "cardDueDateNotADate") {
		return
	}
	if description != nil && !validDescription(w, *description) {
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	if title != nil {
		card.Title = storedTitle
	}
	if description != nil {
		card.Description = phpTrim(harmlessStripped(*description))
	}
	if dueDateNamed {
		card.DueOn = dueDate(dueOn)
	}
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card, person))
}

func (s *Server) moveCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, from, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	columnID, ok1 := stringField(w, fields, "columnId")
	position, ok2 := stringField(w, fields, "position")
	previousID, ok3 := stringField(w, fields, "previousCardId")
	nextID, ok4 := stringField(w, fields, "nextCardId")
	onHold, ok5 := booleanField(w, fields, "onHold")
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return
	}
	if columnID == nil || strings.TrimSpace(*columnID) == "" {
		unprocessable(w, "columnIdMissing", "A move needs the column to move the card to.")
		return
	}
	if position != nil && *position != "top" && *position != "bottom" {
		malformed(w, `The position is either "top" or "bottom".`)
		return
	}
	if position != nil && (previousID != nil || nextID != nil) {
		malformed(w, "A move names either a position or neighbours, not both.")
		return
	}

	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	var to *Column
	for _, c := range board.Columns {
		if c.ID == *columnID {
			to = c
		}
	}
	// A column of the board that carries no area on hold names its refusal,
	// and it names it before anything else the move got wrong. A column on
	// another board or on none stays an address that names nothing.
	toHold := onHold != nil && *onHold
	if toHold && to != nil && !to.OnHoldEnabled {
		unprocessable(w, "columnHasNoOnHoldArea", "This column has no area for the cards on hold.")
		return
	}
	for _, id := range []*string{previousID, nextID} {
		if id != nil && *id != "" && !uuidPattern.MatchString(*id) {
			notFound(w)
			return
		}
	}
	if to == nil {
		notFound(w)
		return
	}

	from.Cards = slices.DeleteFunc(from.Cards, func(c *Card) bool { return c == card })
	card.OnHold = toHold
	to.Cards = slices.Insert(to.Cards, placeInArea(to.Cards, toHold, position, previousID, nextID), card)

	if to.Type == "done" && from.Type != "done" {
		now := s.now
		card.CompletedAt = &now
		card.CompletedByID = person.ID
	}
	if to.Type != "done" {
		card.CompletedAt = nil
		card.CompletedByID = ""
	}
	if to != from {
		s.noteTheMove(board, card, to, person)
	}
	writeJSON(w, http.StatusOK, s.cardResource(board, to, card, person))
}

// placeInArea is where a card lands among the cards of one area of a column.
// A neighbour from the other area, or from no area at all, counts as none,
// and the card goes to the top of its area.
func placeInArea(cards []*Card, onHold bool, position, previousID, nextID *string) int {
	inArea := func(id *string) int {
		if id == nil {
			return -1
		}
		i := indexOf(cards, *id)
		if i < 0 || cards[i].OnHold != onHold {
			return -1
		}
		return i
	}
	switch {
	case position != nil && *position == "bottom":
		return len(cards)
	case inArea(previousID) >= 0:
		return inArea(previousID) + 1
	case inArea(nextID) >= 0:
		return inArea(nextID)
	}
	return 0
}

// setColumnOnHold switches the area for cards on hold of a working column.
// Switching it off hands its cards back to the end of the column, in the
// order they stood on hold. Triage, Not now and Done name their refusal,
// on and off alike.
func (s *Server) setColumnOnHold(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column := s.visibleColumnOfTheTeam(r.PathValue("columnId"), person)
	if column == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	enabled, ok := booleanField(w, fields, "onHoldEnabled")
	if !ok {
		return
	}
	if enabled == nil {
		unprocessable(w, "onHoldEnabledMissing", "Say whether the column is to have an area for the cards on hold.")
		return
	}
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return
	}
	if column.Type != "column" {
		unprocessable(w, "systemColumnHasNoOnHoldArea", "Triage, Not now and Done carry no area for the cards on hold.")
		return
	}

	column.OnHoldEnabled = *enabled
	if !*enabled {
		var waiting []*Card
		column.Cards = slices.DeleteFunc(column.Cards, func(c *Card) bool {
			if c.OnHold {
				waiting = append(waiting, c)
			}
			return c.OnHold
		})
		for _, card := range waiting {
			card.OnHold = false
		}
		column.Cards = append(column.Cards, waiting...)
	}
	writeJSON(w, http.StatusOK, s.columnResource(board, column, person))
}

func (s *Server) people(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	s.writePeople(w, project.ID, project.MemberIDs)
}

// conversationPeople lists the two of a private conversation, with mentions
// that hold in that conversation. Somebody outside it finds nothing there.
func (s *Server) conversationPeople(w http.ResponseWriter, r *http.Request, person *Person) {
	conversation := s.visibleChat(r.PathValue("conversationId"), person)
	if conversation == nil || !conversation.isConversation() {
		notFound(w)
		return
	}
	s.writePeople(w, "conversation:"+conversation.ID, conversation.Partners)
}

// writePeople hands out the people of one circle, each with the mention that
// names them in a text of that circle and of no other.
func (s *Server) writePeople(w http.ResponseWriter, circle string, memberIDs []string) {
	list := []any{}
	for _, id := range memberIDs {
		member := s.People[id]
		// Somebody who stepped down is in no circle any more, however long
		// their name stands on what they once wrote.
		if member.Retired {
			continue
		}
		handle := "sgid-person-" + circle + "-" + member.ID
		list = append(list, map[string]any{
			"id": member.ID, "name": member.Name, "emailAddress": member.Email,
			"kind": kindOf(member), "principal": s.principalOf(member),
			"mention": map[string]any{
				"handle": handle,
				"markup": fmt.Sprintf(
					`<action-text-attachment sgid=%q content-type="application/vnd.minka.person" content="@%s"></action-text-attachment>`,
					handle, member.Name,
				),
			},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// assignCard refuses a person that is no ID like a person outside the
// project, before it asks why nothing may be written on the card.
func (s *Server) assignCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	personID, ok := stringField(w, fields, "personId")
	if !ok {
		return
	}
	if personID == nil || strings.TrimSpace(*personID) == "" {
		unprocessable(w, "personIdMissing", "An assignment needs the person to assign.")
		return
	}
	if !uuidPattern.MatchString(*personID) {
		notAProjectMember(w)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	if project := s.visibleProject(board.ProjectID, person); !slices.Contains(project.MemberIDs, *personID) {
		notAProjectMember(w)
		return
	}

	status := http.StatusOK
	if !slices.Contains(card.AssigneeIDs, *personID) {
		card.AssigneeIDs = append(card.AssigneeIDs, *personID)
		card.subscribe(*personID)
		status = http.StatusCreated
	}
	writeJSON(w, status, s.cardResource(board, column, card, person))
}

// unassignCard takes somebody off a card without asking about membership, so
// a person who left the project can still be taken off.
func (s *Server) unassignCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	personID := r.PathValue("personId")
	if card == nil || !uuidPattern.MatchString(personID) {
		notFound(w)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	card.AssigneeIDs = slices.DeleteFunc(card.AssigneeIDs, func(id string) bool { return id == personID })

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// addStep puts a step at the end of the list and answers with that step.
func (s *Server) addStep(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	if _, namesACard := fields["cardId"]; namesACard && !s.KnowsNoCardSteps {
		s.addCardStep(w, board, column, card, archived, fields, person)
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	text, ok := stepText(w, title)
	if !ok {
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	step := &Step{ID: newID(), Title: text}
	card.Steps = append(card.Steps, step)
	s.answerAStep(w, http.StatusCreated, board, column, card, step, person)
}

// answerAStep answers a new step, or a card step that stands already, with
// the step as the card lists it, or with the card as an older Minka does.
func (s *Server) answerAStep(w http.ResponseWriter, status int, board *Board, column *Column, card *Card, step *Step, person *Person) {
	if s.AnswersANewStepWithItsCard || s.KnowsNoCardSteps || s.NeitherRenamesNorMovesSteps {
		writeJSON(w, status, s.cardResource(board, column, card, person))
		return
	}
	resources := s.stepResources(card, person)
	writeJSON(w, status, resources[slices.Index(card.Steps, step)])
}

// stepText is the line of a step as Minka keeps it, trimmed, or the refusal
// of one: the length first, then control characters, then nothing at all.
func stepText(w http.ResponseWriter, title *string) (string, bool) {
	text := phpTrim(deref(title))
	switch {
	case utf8.RuneCountInString(text) > maxStepLength:
		unprocessable(w, "stepTooLong", "The line of a step is too long.")
		return "", false
	case strings.IndexFunc(text, unicode.IsControl) >= 0:
		unprocessable(w, "stepHasControlCharacters", "The line of a step must not contain control characters.")
		return "", false
	case text == "":
		unprocessable(w, "stepMissing", "A step needs a line of text.")
		return "", false
	}
	return text, true
}

// shortIdentifierOrNotes tells the two addresses of two parts below /cards
// apart the way the routes of the server do, which the patterns of one mux
// cannot: "by-short-id" looks a card up, anything else is a card's notes.
func (s *Server) shortIdentifierOrNotes(w http.ResponseWriter, r *http.Request, person *Person) {
	if r.PathValue("first") == "by-short-id" {
		r.SetPathValue("identifier", r.PathValue("second"))
		s.cardByShortIdentifier(w, r, person)
		return
	}
	if r.PathValue("second") != "notes" {
		notFound(w)
		return
	}
	r.SetPathValue("cardId", strings.ToLower(r.PathValue("first")))
	s.cardNotes(w, r, person)
}

func (s *Server) cardNotes(w http.ResponseWriter, r *http.Request, person *Person) {
	board, card, _ := s.cardOfTheBack(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.notesResource(board, card))
}

// writeCardNotes replaces the notes of a card. An expectedUpdatedAt that is
// given writes only over the notes it names, or over none for null; notes
// that say the sent text already are no conflict, and notes nobody has
// written yet are created whatever was expected.
func (s *Server) writeCardNotes(w http.ResponseWriter, r *http.Request, person *Person) {
	board, card, archived := s.cardOfTheBack(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	// The text is looked at before the guard, as the controller does it: a
	// body that is no text is the one thing wrong with the request itself, and
	// everything else about the text is a refusal of what it says.
	if body == nil {
		unprocessable(w, "cardNotesMissing", `Writing the notes of a card needs the field "body"; an empty string clears them.`)
		return
	}
	text := strings.ReplaceAll(strings.ReplaceAll(*body, "\r\n", "\n"), "\r", "\n")
	switch {
	case !utf8.ValidString(text) || strings.IndexFunc(text, func(c rune) bool { return unicode.IsControl(c) && c != '\n' && c != '\t' }) >= 0:
		unprocessable(w, "cardNotesHasControlCharacters", "The notes of a card must not contain control characters other than line breaks and tabs.")
		return
	case hasInvisibleCharacters(text):
		unprocessable(w, "cardNotesHasInvisibleCharacters", "The notes of a card must not contain invisible format characters such as direction marks, tags or zero-width spaces; the zero-width joiner and non-joiner are fine.")
		return
	case utf8.RuneCountInString(text) > maxNotesLength:
		unprocessable(w, "cardNotesTooLong", "The notes of a card must not be longer than 100000 characters.")
		return
	}

	expected, guarded := fields["expectedUpdatedAt"]
	var expectedAt *time.Time
	if guarded && expected != nil {
		value, isString := expected.(string)
		at, read := aPointInTime(value)
		if !isString || !read {
			malformed(w, `The field "expectedUpdatedAt" has to be a point in time as "updatedAt" gives it, or null.`)
			return
		}
		expectedAt = &at
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	// The text the notes hold already is no write and no conflict, and that
	// includes an empty text on a card nobody has written notes for.
	if card.Notes == text {
		writeJSON(w, http.StatusOK, s.notesResource(board, card))
		return
	}
	if guarded && card.NotesUpdatedAt != nil && (expectedAt == nil || !expectedAt.Equal(*card.NotesUpdatedAt)) {
		refusal := errorBody("notesChanged", `The notes of this card were changed since they were read. Nothing was written; "notes" holds them as they stand.`)
		refusal["notes"] = s.notesResource(board, card)
		writeJSON(w, http.StatusConflict, refusal)
		return
	}
	// Every write stands a second after the one before, so that each has a
	// time of its own for the guard to tell apart.
	writtenAt := s.now
	if card.NotesUpdatedAt != nil {
		writtenAt = card.NotesUpdatedAt.Add(time.Second)
	}
	card.Notes, card.NotesUpdatedAt, card.NotesUpdatedByID = text, &writtenAt, person.ID
	writeJSON(w, http.StatusOK, s.notesResource(board, card))
}

// aPointInTime reads a moment the way Minka reads the one a guard names: up to
// six digits of a second, under Z or an offset written either way. The shape
// is looked at first, because Go reads a fraction of any length whatever the
// layout says.
func aPointInTime(value string) (time.Time, bool) {
	if !guardTimePattern.MatchString(value) {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999999Z07:00", "2006-01-02T15:04:05.999999Z0700"} {
		if at, err := time.Parse(layout, value); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

var guardTimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,6})?(Z|[+-]\d{2}:?\d{2})$`)

func (s *Server) notesResource(board *Board, card *Card) map[string]any {
	return map[string]any{
		"cardId": card.ID, "body": card.Notes, "updatedAt": timestampOrNil(card.NotesUpdatedAt),
		"updatedBy": s.personReferenceOrNil(card.NotesUpdatedByID),
		"appUrl":    s.cardURL(board, card) + "#notes",
	}
}

// updateStep looks for the step on the card before it reads the body, so a
// step that is not there is 404 even on a card that takes no write, and a
// card step is refused before the body as well. Of the
// body, the shape of assigneeIds is read before anything is asked of it; the
// line is read after the tick and before the day; a person whose identifier
// is none is refused before the archive is, and a person who is no member
// after it. The due day and the people are written first, so a person refused
// leaves the line as it was, then the line, then the tick.
func (s *Server) updateStep(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	index := slices.IndexFunc(card.Steps, func(step *Step) bool { return step.ID == r.PathValue("stepId") })
	if index < 0 {
		notFound(w)
		return
	}
	step := card.Steps[index]
	if step.CardID != "" {
		unprocessable(w, "stepIsACard", "A card step has no title, tick, due date or assignees of its own; its card carries them.")
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	assigneeIDs, ok := stringListField(w, fields, "assigneeIds")
	if !ok {
		return
	}
	_, namesTitle := fields["title"]
	namesTitle = namesTitle && !s.NeitherRenamesNorMovesSteps
	completedValue, namesCompletion := fields["completed"]
	_, namesDue := fields["dueOn"]
	if !namesTitle && !namesCompletion && !namesDue && assigneeIDs == nil {
		unprocessable(w, "nothingToChange", `The body names neither a title nor "completed" nor a due date nor assignees.`)
		return
	}
	completed, isFlag := completedValue.(bool)
	if namesCompletion && !isFlag {
		unprocessable(w, "stepCompletedMissing", `Changing a step needs the field "completed" as true or false.`)
		return
	}
	var text string
	if namesTitle {
		title, ok := stringField(w, fields, "title")
		if !ok {
			return
		}
		if text, ok = stepText(w, title); !ok {
			return
		}
	}
	dueOn, ok := stringField(w, fields, "dueOn")
	if !ok || namesDue && !validDueDate(w, dueOn, "stepDueDateNotADate") {
		return
	}
	for _, id := range assigneeIDs {
		if !uuidPattern.MatchString(id) {
			notAProjectMember(w)
			return
		}
	}

	if namesDue || assigneeIDs != nil {
		if s.refusedAsAClosedCard(w, board, archived) {
			return
		}
		// Who is on the step already stays without a question, somebody who
		// left the project included; only who comes new has to be a member.
		project := s.projectOf(board)
		for _, id := range assigneeIDs {
			if !slices.Contains(step.AssigneeIDs, id) && !slices.Contains(project.MemberIDs, id) {
				notAProjectMember(w)
				return
			}
		}
		if namesDue {
			step.DueOn = dueDate(dueOn)
		}
		if assigneeIDs != nil {
			step.AssigneeIDs = changedPeople(step.AssigneeIDs, assigneeIDs)
		}
	}
	if namesTitle {
		if s.refusedAsAClosedCard(w, board, archived) {
			return
		}
		step.Title = text
	}
	if namesCompletion {
		if s.refusedAsAClosedCard(w, board, archived) {
			return
		}
		step.Completed = completed
	}
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card, person))
}

// moveStep puts a step between two others of its card, by the neighbours as
// they stand: a previous step it finds decides, the next one only without it,
// and with neither the step goes to the top. A step that is not on the card
// is 404 before the body is read. A neighbour that is no other step of the
// card is refused, but an archived project or card is told first; the
// previous neighbour is read before the next.
func (s *Server) moveStep(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if card == nil || s.NeitherRenamesNorMovesSteps {
		notFound(w)
		return
	}
	if s.BeforeMovingStep != nil {
		s.BeforeMovingStep(card)
	}
	stepID := r.PathValue("stepId")
	index := slices.IndexFunc(card.Steps, func(step *Step) bool { return step.ID == stepID })
	if index < 0 {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	var neighbours [2]string
	for i, name := range []string{"previousStepId", "nextStepId"} {
		value, ok := stringField(w, fields, name)
		if !ok {
			return
		}
		id := deref(value)
		if id == "" {
			continue
		}
		if id == stepID || !slices.ContainsFunc(card.Steps, func(step *Step) bool { return step.ID == id }) {
			if !s.refusedAsAClosedCard(w, board, archived) {
				unprocessable(w, "stepPlaceUnknown", "The neighbours named are no other steps of this card.")
			}
			return
		}
		neighbours[i] = id
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}

	step := card.Steps[index]
	others := slices.Delete(slices.Clone(card.Steps), index, index+1)
	at := 0
	if previous := slices.IndexFunc(others, func(other *Step) bool { return other.ID == neighbours[0] }); neighbours[0] != "" && previous >= 0 {
		at = previous + 1
	} else if next := slices.IndexFunc(others, func(other *Step) bool { return other.ID == neighbours[1] }); neighbours[1] != "" && next >= 0 {
		at = next
	}
	card.Steps = slices.Insert(others, at, step)
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card, person))
}

// removeStep answers 204 for a step that is not on the card as well, one of
// another card included, unless nothing may be written on the card at all;
// it is 404 for a card the person may not see and for an address that names
// no step at all.
func (s *Server) removeStep(w http.ResponseWriter, r *http.Request, person *Person) {
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if card == nil || !uuidPattern.MatchString(r.PathValue("stepId")) {
		notFound(w)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	card.Steps = slices.DeleteFunc(card.Steps, func(step *Step) bool { return step.ID == r.PathValue("stepId") })

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

var messageNumber = regexp.MustCompile(`^[0-9]{1,18}$`)

func (s *Server) chats(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	list := []any{}
	for _, chat := range s.Chats {
		if !chat.isConversation() && chat.ProjectID == project.ID {
			list = append(list, map[string]any{
				"id": chat.ID, "name": chat.Name, "unreadCount": chat.unread(person.ID),
				"appUrl": s.URL + "/projects/" + project.ID + "/chats/" + chat.ID,
			})
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// chat reads a chat of a project by its identifier alone, with the project it
// stands in. A private conversation belongs to no project and is not at this
// address, although it answers at the ones below it.
func (s *Server) chat(w http.ResponseWriter, r *http.Request, person *Person) {
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil || chat.isConversation() {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": chat.ID, "projectId": chat.ProjectID, "project": s.projectReference(chat.ProjectID),
		"name": chat.Name, "appUrl": s.URL + "/projects/" + chat.ProjectID + "/chats/" + chat.ID,
	})
}

// visibleChat finds a chat of a project the person is a member of, or a
// private conversation of theirs. Both answer at the same endpoints under the
// same identifier, with a different lock.
func (s *Server) visibleChat(id string, person *Person) *Chat {
	for _, chat := range s.Chats {
		if chat.ID != id {
			continue
		}
		if chat.isConversation() {
			if slices.Contains(chat.Partners, person.ID) {
				return chat
			}
			continue
		}
		if s.visibleProject(chat.ProjectID, person) != nil {
			return chat
		}
	}
	return nil
}

// conversations lists the private conversations of a person, the one with the
// newest word first.
func (s *Server) conversations(w http.ResponseWriter, _ *http.Request, person *Person) {
	var mine []*Chat
	for _, chat := range s.Chats {
		if chat.isConversation() && slices.Contains(chat.Partners, person.ID) {
			mine = append(mine, chat)
		}
	}
	slices.SortStableFunc(mine, func(one, other *Chat) int {
		return lastWordAt(other).Compare(lastWordAt(one))
	})

	list := []any{}
	for _, chat := range mine {
		list = append(list, s.conversationResource(chat, person))
	}
	writeJSON(w, http.StatusOK, list)
}

// beginOrUpload tells the two addresses below /messages apart the way the
// routes of the server do, which the patterns of one mux cannot: "with" names
// a person to talk to, everything else a conversation to put a file into.
func (s *Server) beginOrUpload(w http.ResponseWriter, r *http.Request, person *Person) {
	if r.PathValue("first") == "with" {
		r.SetPathValue("personId", strings.ToLower(r.PathValue("second")))
		s.startConversation(w, r, person)
		return
	}
	if r.PathValue("second") != "uploads" {
		notFound(w)
		return
	}
	r.SetPathValue("conversationId", strings.ToLower(r.PathValue("first")))
	s.upload(w, r, person)
}

// startConversation answers with the conversation of these two, and begins it
// where they have none. Somebody unknown, somebody of another organization and
// the person themselves all answer like a person who does not exist.
func (s *Server) startConversation(w http.ResponseWriter, r *http.Request, person *Person) {
	other, known := s.People[r.PathValue("personId")]
	if !known || other.ID == person.ID {
		notFound(w)
		return
	}

	for _, chat := range s.Chats {
		if chat.isConversation() && slices.Contains(chat.Partners, person.ID) && slices.Contains(chat.Partners, other.ID) {
			writeJSON(w, http.StatusOK, s.conversationResource(chat, person))
			return
		}
	}

	conversation := &Chat{ID: newID(), Partners: []string{person.ID, other.ID}, ReadMarks: map[string]int{}}
	s.Chats = append(s.Chats, conversation)
	writeJSON(w, http.StatusCreated, s.conversationResource(conversation, person))
}

func (s *Server) conversationResource(chat *Chat, person *Person) map[string]any {
	other := s.People[chat.Partners[0]]
	if other.ID == person.ID {
		other = s.People[chat.Partners[1]]
	}

	var lastText, lastAt any
	if last := chat.lastWord(); last != nil {
		lastText, lastAt = last.BodyText, timestamp(last.CreatedAt)
	}

	return map[string]any{
		"id": chat.ID, "name": other.Name,
		"counterpart":     map[string]any{"id": other.ID, "name": other.Name},
		"lastMessageText": lastText, "lastMessageAt": lastAt,
		"unreadCount": chat.unread(person.ID),
		"appUrl":      s.URL + "/messages/" + chat.ID,
	}
}

// lastWordAt is when a conversation was last spoken in, which is the beginning
// of time while nothing has been said.
func lastWordAt(chat *Chat) time.Time {
	if last := chat.lastWord(); last != nil {
		return last.CreatedAt
	}
	return time.Time{}
}

// messages pages the way the server does: before a seq, after a changeSeq,
// or the end of the transcript, always in the order things were said. A seq
// names a message and never its place, so the numbers down a page need not
// rise.
func (s *Server) messages(w http.ResponseWriter, r *http.Request, person *Person) {
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		notFound(w)
		return
	}
	query := r.URL.Query()
	wanted := 50
	if limit := query.Get("limit"); limit != "" {
		number, err := strconv.Atoi(limit)
		if !messageNumber.MatchString(limit) || err != nil || number < 1 {
			unprocessable(w, "chatMessagesLimitNotANumber", "The number of messages asked for has to be a whole number of at least one.")
			return
		}
		wanted = min(number, 200)
	}
	if s.MessagesPerPage > 0 {
		wanted = min(wanted, s.MessagesPerPage)
	}
	last := func(messages []*ChatMessage) []*ChatMessage {
		return messages[max(0, len(messages)-wanted):]
	}
	// A message that was taken back is out of the transcript and comes back
	// only among what changed above a change number.
	transcript := chat.transcript()

	var page []*ChatMessage
	switch {
	case query.Get("before") != "":
		before, err := strconv.Atoi(query.Get("before"))
		if !messageNumber.MatchString(query.Get("before")) || err != nil {
			notFound(w)
			return
		}
		// The cursor stands where the message of that number was said, taken
		// back or not. A number no message carries names the place of the
		// highest number below it, and that message belongs to the page.
		var boundary *ChatMessage
		for _, message := range chat.Messages {
			if message.Seq <= before && (boundary == nil || message.Seq > boundary.Seq) {
				boundary = message
			}
		}
		if boundary != nil {
			for _, message := range transcript {
				if saidBefore(message, boundary) < 0 || message == boundary && boundary.Seq != before {
					page = append(page, message)
				}
			}
		}
		page = last(page)
	case query.Get("after") != "":
		after, err := strconv.Atoi(query.Get("after"))
		if !messageNumber.MatchString(query.Get("after")) || err != nil {
			notFound(w)
			return
		}
		for _, message := range chat.Messages {
			if message.ChangeSeq > after {
				page = append(page, message)
			}
		}
		slices.SortFunc(page, func(a, b *ChatMessage) int { return a.ChangeSeq - b.ChangeSeq })
		page = page[:min(len(page), wanted)]
		slices.SortFunc(page, saidBefore)
	default:
		page = last(transcript)
	}

	list := []any{}
	for _, message := range page {
		list = append(list, s.chatMessageResource(chat, message))
	}
	writeJSON(w, http.StatusOK, list)
}

func validChatText(w http.ResponseWriter, text string) bool {
	switch {
	case utf8.RuneCountInString(text) > maxChatMessageLength:
		unprocessable(w, "chatMessageTooLong", "The message is too long.")
	case strings.IndexFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' }) >= 0:
		unprocessable(w, "chatMessageHasControlCharacters", "A chat message must not contain control characters other than line breaks and tabs.")
	case text == "":
		unprocessable(w, "chatMessageMissing", "A chat message needs a body.")
	default:
		return true
	}
	return false
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	clientMessageID, ok := stringField(w, fields, "clientMessageId")
	if !ok {
		return
	}
	if deref(clientMessageID) != "" && !uuidPattern.MatchString(*clientMessageID) {
		unprocessable(w, "chatMessageDraftNotAnIdentifier", "The name of the draft has to be a UUID.")
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	text := phpTrim(deref(body))
	if !validChatText(w, text) {
		return
	}
	replyTo, ok := stringField(w, fields, "replyTo")
	if !ok {
		return
	}
	answered := deref(replyTo)
	if answered != "" && !uuidPattern.MatchString(answered) {
		replyToUnknown(w)
		return
	}
	// Asked before a message sent earlier is looked for: in an archived
	// project a refused write must not pass for one that went through.
	if s.refusedAsArchived(w, s.projectOfChat(chat)) {
		return
	}
	// The message answered has to stand in this chat, asked after the
	// archive like the right to write, as Minka asks it inside the write.
	if answered != "" && !slices.ContainsFunc(chat.Messages, func(m *ChatMessage) bool { return m.ID == answered && !m.Removed }) {
		replyToUnknown(w)
		return
	}
	if deref(clientMessageID) != "" {
		for _, message := range chat.Messages {
			if message.ClientMessageID == *clientMessageID && message.AuthorID == person.ID {
				writeJSON(w, http.StatusOK, s.chatMessageResource(chat, message))
				return
			}
		}
	}
	message := chat.post(person.ID, text, deref(clientMessageID), s.now)
	message.ReplyTo = answered
	writeJSON(w, http.StatusCreated, s.chatMessageResource(chat, message))
}

// orNull is a value the answer names as null where there is none.
func orNull(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func replyToUnknown(w http.ResponseWriter) {
	unprocessable(w, "chatMessageReplyToUnknown", "A reply has to answer a message of the same chat that still stands.")
}

// authoredMessage is the message of this chat that the person wrote and has
// not removed, as the server's statements find it.
func authoredMessage(chat *Chat, messageID string, person *Person) *ChatMessage {
	for _, message := range chat.Messages {
		if message.ID == messageID && message.AuthorID == person.ID {
			return message
		}
	}
	return nil
}

// editMessage looks for the message in the chat before it reads the body: a
// message of another chat is 404 also in an archived project, and so is
// somebody else's or a removed one once the archive has been asked.
func (s *Server) editMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	chat, _ := s.messageOfChat(r, person)
	if chat == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	text := phpTrim(deref(body))
	if !validChatText(w, text) {
		return
	}
	if s.refusedAsArchived(w, s.projectOfChat(chat)) {
		return
	}
	message := authoredMessage(chat, r.PathValue("messageId"), person)
	if message == nil || message.Removed {
		notFound(w)
		return
	}
	if message.BodyHTML != chatHTML(text) {
		now := s.now
		message.BodyText, message.BodyHTML, message.EditedAt = chatText(text), chatHTML(text), &now
		chat.noteChange(message)
	}
	writeJSON(w, http.StatusOK, s.chatMessageResource(chat, message))
}

// removeMessage answers 404 for a message that is not in the chat before it
// asks about the archive, as the server does once its write found nothing.
func (s *Server) removeMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	chat, _ := s.messageOfChat(r, person)
	if chat == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfChat(chat)) {
		return
	}
	message := authoredMessage(chat, r.PathValue("messageId"), person)
	if message == nil {
		notFound(w)
		return
	}
	if !message.Removed {
		message.Removed, message.BodyText, message.BodyHTML = true, "", ""
		chat.noteChange(message)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// markChatRead keeps the highest mark and never one beyond the counter of the
// chat, so that a number from elsewhere cannot silence the chat for good.
func (s *Server) markChatRead(w http.ResponseWriter, r *http.Request, person *Person) {
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	value, isNumber := fields["seq"].(float64)
	if !isNumber || value < 0 || value != math.Trunc(value) {
		unprocessable(w, "chatReadSeqMissing", "The number a chat has been read up to has to be a whole number of at least zero.")
		return
	}
	seq := min(int(value), chat.lastSeq)
	if seq > chat.ReadMarks[person.ID] {
		chat.ReadMarks[person.ID] = seq
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

const maxReactionLength = 20

// reactToComment and the three that follow it are the two ways to react and
// the two ways to take a reaction away. Both texts behave the same, so the
// work is in reactTo and unreactFrom. The comment or the message is looked
// for before the body, and the reaction is read before the archive is asked.
func (s *Server) reactToComment(w http.ResponseWriter, r *http.Request, person *Person) {
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	comment := commentOf(card, r.PathValue("commentId"))
	if comment == nil {
		notFound(w)
		return
	}
	content, ok := reactionContent(w, r)
	if !ok {
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	status := reactTo(&comment.Reactions, content, person.ID)
	writeJSON(w, status, s.commentResource(board.ProjectID, s.cardURL(board, card), comment))
}

func (s *Server) unreactFromComment(w http.ResponseWriter, r *http.Request, person *Person) {
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	comment := commentOf(card, r.PathValue("commentId"))
	if comment == nil {
		notFound(w)
		return
	}
	content, ok := validReaction(w, r.URL.Query().Get("content"))
	if !ok {
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	unreactFrom(&comment.Reactions, content, person.ID)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reactToMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	chat, message := s.messageOfChat(r, person)
	if chat == nil {
		notFound(w)
		return
	}
	content, ok := reactionContent(w, r)
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectOfChat(chat)) {
		return
	}
	if message.Removed {
		notFound(w)
		return
	}
	status := reactTo(&message.Reactions, content, person.ID)
	if status == http.StatusCreated {
		chat.noteChange(message)
	}
	writeJSON(w, status, s.chatMessageResource(chat, message))
}

func (s *Server) unreactFromMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	chat, message := s.messageOfChat(r, person)
	if chat == nil {
		notFound(w)
		return
	}
	content, ok := validReaction(w, r.URL.Query().Get("content"))
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectOfChat(chat)) {
		return
	}
	if message.Removed {
		notFound(w)
		return
	}
	if unreactFrom(&message.Reactions, content, person.ID) {
		chat.noteChange(message)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// messageOfChat is the chat of the address with the message it names, a
// removed one included; both are nil where the chat holds no such message. A
// message that belongs to another chat is as unreachable as one that is not
// there.
func (s *Server) messageOfChat(r *http.Request, person *Person) (*Chat, *ChatMessage) {
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		return nil, nil
	}
	for _, message := range chat.Messages {
		if message.ID == r.PathValue("messageId") {
			return chat, message
		}
	}
	return nil, nil
}

func commentOf(card *Card, commentID string) *Comment {
	if card == nil {
		return nil
	}
	for _, comment := range card.Comments {
		if comment.ID == commentID {
			return comment
		}
	}
	return nil
}

// reactionContent reads what the body says to react with, and refuses what the
// server refuses.
func reactionContent(w http.ResponseWriter, r *http.Request) (string, bool) {
	fields, ok := readBody(w, r)
	if !ok {
		return "", false
	}
	value, ok := stringField(w, fields, "content")
	if !ok {
		return "", false
	}
	return validReaction(w, deref(value))
}

// validReaction is a reaction as the server keeps it, or the refusal it draws.
func validReaction(w http.ResponseWriter, value string) (string, bool) {
	content := phpTrim(value)
	switch {
	case content == "":
		unprocessable(w, "reactionMissing", "A reaction needs a content.")
	case utf8.RuneCountInString(content) > maxReactionLength:
		unprocessable(w, "reactionTooLong", "A reaction must not be longer than 20 characters.")
	case strings.IndexFunc(content, unicode.IsControl) >= 0:
		unprocessable(w, "reactionHasControlCharacters", "A reaction must not contain control characters.")
	default:
		return content, true
	}
	return "", false
}

// reactTo adds a reaction, and answers 200 where the same one of this person
// stood already.
func reactTo(reactions *[]*Reaction, content, personID string) int {
	for _, reaction := range *reactions {
		if reaction.Content != content {
			continue
		}
		if slices.Contains(reaction.PersonIDs, personID) {
			return http.StatusOK
		}
		reaction.PersonIDs = append(reaction.PersonIDs, personID)
		return http.StatusCreated
	}
	*reactions = append(*reactions, &Reaction{Content: content, PersonIDs: []string{personID}})
	return http.StatusCreated
}

// unreactFrom takes away the reaction of this person, and where they left
// none, the one somebody else left last: taking a reaction away is not bound
// to whoever left it. It says whether there was one to take.
func unreactFrom(reactions *[]*Reaction, content, personID string) bool {
	for i, reaction := range *reactions {
		if reaction.Content != content {
			continue
		}
		whose := slices.Index(reaction.PersonIDs, personID)
		if whose < 0 {
			whose = len(reaction.PersonIDs) - 1
		}
		reaction.PersonIDs = slices.Delete(reaction.PersonIDs, whose, whose+1)
		if len(reaction.PersonIDs) == 0 {
			*reactions = slices.Delete(*reactions, i, i+1)
		}
		return true
	}
	return false
}

// upload takes the bytes of the body as a file of the project. The order of
// the checks is the order of the server: the project first, then what the
// request says about the file, then the bytes themselves.
func (s *Server) upload(w http.ResponseWriter, r *http.Request, person *Person) {
	// Which circle the file goes into is what the address names, and a person
	// outside it finds nothing there.
	circle := ""
	if projectID := r.PathValue("projectId"); projectID != "" {
		if project := s.visibleProject(projectID, person); project != nil {
			circle = project.ID
		}
	} else if conversation := s.visibleChat(r.PathValue("conversationId"), person); conversation != nil && conversation.isConversation() {
		circle = "conversation:" + conversation.ID
	}
	if circle == "" {
		notFound(w)
		return
	}
	filename := phpTrim(r.URL.Query().Get("filename"))
	if filename == "" {
		unprocessable(w, "filenameMissing", `The query argument "filename" has to name the file.`)
		return
	}
	contentType := mediaType(r.Header.Get("Content-Type"))
	if contentType == "" {
		unprocessable(w, "contentTypeMissing", `The header "Content-Type" has to name the media type of the file.`)
		return
	}
	if r.ContentLength > MaxUploadBytes {
		tooLarge(w)
		return
	}

	content, err := io.ReadAll(io.LimitReader(r.Body, MaxUploadBytes+1))
	if err != nil {
		malformed(w, "The bytes of the file did not arrive whole.")
		return
	}
	if len(content) > MaxUploadBytes {
		tooLarge(w)
		return
	}
	// What the bytes are is read out of them, and the header decides nothing.
	// It may contradict them, though, where one of the two is a picture: that
	// is what decides whether a file is later shown or handed over.
	looksLike := mediaType(http.DetectContentType(content))
	if looksLike != contentType && (isPicture(looksLike) || isPicture(contentType)) {
		unprocessable(w, "contentTypeMismatch", "The file is a "+looksLike+", and not the "+contentType+" it says it is.")
		return
	}
	stored := contentType
	if looksLike != "application/octet-stream" {
		stored = looksLike
	}

	// The server learns that the project is archived when it stores the file,
	// which is after it read and checked it.
	if project := s.visibleProject(r.PathValue("projectId"), person); s.refusedAsArchived(w, project) {
		return
	}

	upload := &Upload{
		ID: newID(), Circle: circle, Filename: filename, ContentType: stored, Size: len(content),
		Content: content, Display: displayRenderingOf(content, stored), UploaderID: person.ID,
	}
	s.Uploads = append(s.Uploads, upload)
	if s.AfterStoring != nil {
		s.AfterStoring(upload)
	}
	writeJSON(w, http.StatusCreated, s.uploadResource(upload))
}

// rasterTypes are the pictures a browser draws without help, and they are the
// only files Minka ever shows where they stand. An SVG is a document here,
// however its type reads: it can carry scripts.
var rasterTypes = []string{"image/avif", "image/gif", "image/jpeg", "image/png", "image/webp"}

func isPicture(contentType string) bool {
	return slices.Contains(rasterTypes, contentType)
}

// displayRenderingOf is the smaller rendering a picture gets when it is large
// enough to need one, and nothing for every other file. The longer of the two
// sides decides, as it does on the server, so a tall picture gets one as
// readily as a wide one. What the rendering weighs is of no interest beyond
// weighing less than the file it was made from.
func displayRenderingOf(content []byte, contentType string) []byte {
	if !isPicture(contentType) {
		return nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || max(config.Width, config.Height) <= DisplayEdge {
		return nil
	}
	return content[:len(content)/2]
}

func (s *Server) uploadResource(upload *Upload) map[string]any {
	kind := "file"
	if isPicture(upload.ContentType) {
		kind = "image"
	}
	address := s.URL + "/projects/" + upload.Circle + "/uploads/" + upload.ID + "/original/" + upload.Filename
	if conversationID, private := strings.CutPrefix(upload.Circle, "conversation:"); private {
		address = s.URL + "/messages/" + conversationID + "/uploads/" + upload.ID + "/original/" + upload.Filename
	}
	return map[string]any{
		"id": upload.ID, "filename": upload.Filename, "contentType": upload.ContentType, "size": upload.Size,
		"kind": kind, "appUrl": address,
		"attachmentSource": fmt.Sprintf(
			`<action-text-attachment sgid=%q content-type=%q url=%q filename=%q filesize="%d" presentation="gallery"></action-text-attachment>`,
			upload.Handle(), upload.ContentType, address, upload.Filename, upload.Size,
		),
	}
}

// file says what an uploaded file is and where each of its renderings can be
// had. The address names the file alone; which circle it went into is looked
// up here, and being in that circle is what decides.
func (s *Server) file(w http.ResponseWriter, r *http.Request, person *Person) {
	upload := s.visibleUpload(r.PathValue("blobId"), person)
	if upload == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.fileResource(upload))
}

// fileBytes hands one rendering over. A rendering the file has not got is
// named as such rather than answered with the file in its place, and a name
// that is no rendering at all reads the same way.
func (s *Server) fileBytes(w http.ResponseWriter, r *http.Request, person *Person) {
	upload := s.visibleUpload(r.PathValue("blobId"), person)
	if upload == nil {
		notFound(w)
		return
	}
	variant := r.PathValue("variant")
	content := upload.bytesOf(variant)
	if content == nil {
		writeJSON(w, http.StatusNotFound, errorBody("variantNotAvailable", fmt.Sprintf(`This file has no rendering called "%s".`, variant)))
		return
	}

	download := r.URL.Query().Get("download") == "1"
	// A picture that is shown where it stands may come from the store itself,
	// and that answer carries none of the headers below. The name of the file
	// has the last word, because it is what the store derives the type of its
	// object from: a picture named after something else leaves from here.
	if s.StoreURL != "" && !download && isPicture(upload.ContentType) && namesItsOwnType(upload) {
		w.Header().Set("Location", s.StoreURL+"/"+upload.ID+"/"+variant)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Vary", "Authorization")
		w.WriteHeader(http.StatusFound)
		return
	}

	if s.SendsAFileShort && len(content) > 0 {
		content = content[:len(content)-1]
	}

	disposition := "attachment"
	mediaType := "application/octet-stream"
	if isPicture(upload.ContentType) && !download {
		disposition, mediaType = "inline", upload.ContentType
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, upload.Filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Vary", "Authorization")
	// Nothing a member uploaded is ever rendered inside Minka's own origin.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(content)
	}
}

// visibleUpload is the file behind an identifier, and nothing at all for
// somebody outside the circle it went into: a file of somebody else has to
// read like an address that names nothing.
func (s *Server) visibleUpload(blobID string, person *Person) *Upload {
	if !uuidPattern.MatchString(blobID) {
		return nil
	}
	for _, upload := range s.Uploads {
		if upload.ID != blobID || upload.Swept {
			continue
		}
		if conversationID, private := strings.CutPrefix(upload.Circle, "conversation:"); private {
			if chat := s.visibleChat(conversationID, person); chat != nil && chat.isConversation() {
				return upload
			}
			return nil
		}
		// An earlier file of an upload is the team's alone, as the versions
		// that name it are, but whoever stored a file may always fetch it.
		if s.visibleProject(upload.Circle, person) != nil && (!s.isAClient(person.ID) || upload.UploaderID == person.ID || !s.isAnEarlierFile(upload.ID)) {
			return upload
		}
		return nil
	}
	return nil
}

func (s *Server) fileResource(upload *Upload) map[string]any {
	variants := []any{}
	for _, name := range upload.renderings() {
		variants = append(variants, map[string]any{
			"name": name, "contentType": upload.ContentType, "byteSize": len(upload.bytesOf(name)),
			"url": s.URL + "/api/v1/uploads/" + upload.ID + "/" + name,
		})
	}
	return map[string]any{
		"id": upload.ID, "filename": upload.Filename, "contentType": upload.ContentType,
		"byteSize": len(upload.Content), "isImage": isPicture(upload.ContentType), "variants": variants,
	}
}

// attachmentsIn names the files hanging in one text, in the order they stand
// in it. The question is put to the files of this circle, so a handle issued
// for another one names nothing here.
func (s *Server) attachmentsIn(circle, text string) []any {
	type hanging struct {
		at     int
		upload *Upload
	}
	var found []hanging
	for _, upload := range s.Uploads {
		if upload.Circle != circle {
			continue
		}
		if at := strings.Index(text, `sgid="`+upload.Handle()+`"`); at >= 0 {
			found = append(found, hanging{at, upload})
		}
	}
	slices.SortFunc(found, func(a, b hanging) int { return a.at - b.at })

	list := []any{}
	for _, one := range found {
		list = append(list, map[string]any{
			"id": one.upload.ID, "filename": one.upload.Filename, "contentType": one.upload.ContentType,
			// A file is described beside a text as it was uploaded, whatever
			// the smaller rendering of it weighs.
			"byteSize": len(one.upload.Content), "isImage": isPicture(one.upload.ContentType),
			"url": s.URL + "/api/v1/uploads/" + one.upload.ID,
		})
	}
	return list
}

// attachmentElement is a file or a mention as a stored text carries it.
var attachmentElement = regexp.MustCompile(`(?s)<action-text-attachment\b[^>]*>.*?</action-text-attachment>`)

// sourceOf is a stored text as the editor opens it. A file of the text that
// is not among the files of its circle, gone or never one of them, has no
// handle to be given again, so the source no longer carries it; mentions
// stay as they are.
func (s *Server) sourceOf(circle, text string) string {
	return attachmentElement.ReplaceAllStringFunc(text, func(element string) string {
		if strings.Contains(element, `content-type="application/vnd.minka.person"`) {
			return element
		}
		for _, upload := range s.Uploads {
			if upload.Circle == circle && strings.Contains(element, `sgid="`+upload.Handle()+`"`) {
				return element
			}
		}
		return ""
	})
}

// namesItsOwnType says whether the name of a file stands for the type the
// file turned out to be, which is what a store would go by.
func namesItsOwnType(upload *Upload) bool {
	return mediaType(mime.TypeByExtension(strings.ToLower(filepath.Ext(upload.Filename)))) == upload.ContentType
}

// circleOf is the circle a chat belongs to: its project, or the conversation
// itself where two people talk privately.
func circleOf(chat *Chat) string {
	if chat.isConversation() {
		return "conversation:" + chat.ID
	}
	return chat.ProjectID
}

func tooLarge(w http.ResponseWriter) {
	writeJSON(w, http.StatusRequestEntityTooLarge, errorBody("tooLarge", fmt.Sprintf("A file must not be larger than %d bytes.", MaxUploadBytes)))
}

// mediaType is the type itself, without what a header says about its encoding.
func mediaType(header string) string {
	return strings.TrimSpace(strings.Split(header, ";")[0])
}

func (s *Server) chatMessageResource(chat *Chat, message *ChatMessage) map[string]any {
	return map[string]any{
		"id": message.ID, "seq": message.Seq, "changeSeq": message.ChangeSeq,
		"author": s.personReference(message.AuthorID), "bodyHtml": message.BodyHTML, "bodyText": message.BodyText, "bodySource": message.BodyHTML,
		"attachments": s.attachmentsIn(circleOf(chat), message.BodyHTML),
		"createdAt":   timestamp(message.CreatedAt), "editedAt": timestampOrNil(message.EditedAt), "removed": message.Removed,
		"reactions": s.reactionResources(message.Reactions), "replyTo": orNull(message.ReplyTo),
	}
}

// subscribe puts a person on the list the way Minka does on its own: not
// somebody who took themselves off it.
func (c *Card) subscribe(personID string) {
	if !slices.Contains(c.OffIDs, personID) && !slices.Contains(c.SubscriberIDs, personID) {
		c.SubscriberIDs = append(c.SubscriberIDs, personID)
	}
}

func (s *Server) subscriberReferences(card *Card) []any {
	ids := slices.Clone(card.SubscriberIDs)
	slices.SortFunc(ids, func(a, b string) int { return strings.Compare(s.People[a].Name, s.People[b].Name) })
	list := []any{}
	for _, id := range ids {
		list = append(list, s.personReference(id))
	}
	return list
}

// namedMember is the person of the path if they are a member of the project
// of the card, which the server asks both on and off the list.
func (s *Server) namedMember(board *Board, personID string, person *Person) bool {
	return uuidPattern.MatchString(personID) && slices.Contains(s.visibleProject(board.ProjectID, person).MemberIDs, personID)
}

func (s *Server) subscribeToCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	personID := r.PathValue("personId")
	if card == nil || !uuidPattern.MatchString(personID) {
		notFound(w)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	if !s.namedMember(board, personID, person) {
		notAProjectMember(w)
		return
	}
	card.OffIDs = slices.DeleteFunc(card.OffIDs, func(id string) bool { return id == personID })
	card.subscribe(personID)
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card, person))
}

func (s *Server) unsubscribeFromCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	personID := r.PathValue("personId")
	if card == nil || !uuidPattern.MatchString(personID) {
		notFound(w)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	if !s.namedMember(board, personID, person) {
		notAProjectMember(w)
		return
	}
	card.SubscriberIDs = slices.DeleteFunc(card.SubscriberIDs, func(id string) bool { return id == personID })
	if !slices.Contains(card.OffIDs, personID) {
		card.OffIDs = append(card.OffIDs, personID)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// ownComment is the comment of the card the person wrote, as the server finds
// it: anything else is not there.
func ownComment(card *Card, commentID string, person *Person) *Comment {
	for _, comment := range card.Comments {
		if comment.ID == commentID && comment.AuthorID == person.ID {
			return comment
		}
	}
	return nil
}

// editComment looks for the comment under the card before it reads the body,
// and asks why nothing was written only after the text, in the order the
// server does: the project, the archive of the board, and last whose comment
// it is, which only its author changes.
func (s *Server) editComment(w http.ResponseWriter, r *http.Request, person *Person) {
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if commentOf(card, r.PathValue("commentId")) == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	text, ok := commentText(w, deref(body))
	if !ok {
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	comment := ownComment(card, r.PathValue("commentId"), person)
	if comment == nil {
		notFound(w)
		return
	}
	// The same text again is a write as well: the moment moves on.
	now := s.now
	comment.Body, comment.EditedAt = text, &now
	writeJSON(w, http.StatusOK, s.commentResource(board.ProjectID, s.cardURL(board, card), comment))
}

// isCommentControl is a control character a comment may not hold: all of
// them but line breaks and tabs.
func isCommentControl(r rune) bool {
	return unicode.IsControl(r) && r != 9 && r != 10 && r != 13
}

func (s *Server) removeComment(w http.ResponseWriter, r *http.Request, person *Person) {
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if commentOf(card, r.PathValue("commentId")) == nil {
		notFound(w)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	comment := ownComment(card, r.PathValue("commentId"), person)
	if comment == nil {
		notFound(w)
		return
	}
	card.Comments = slices.DeleteFunc(card.Comments, func(c *Comment) bool { return c == comment })
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func notAProjectMember(w http.ResponseWriter) {
	unprocessable(w, "personNotAProjectMember", "Only a member of the project of the card can be named on it.")
}

func (s *Server) assigneeReferences(card *Card) []any {
	list := []any{}
	for _, id := range card.AssigneeIDs {
		list = append(list, s.personReference(id))
	}
	return list
}

// removeCard moves a card to the trash, from its board or from the archive of
// its board. The fake forgets where it came from, which reads the same: every
// later request for it is 404, and what a restore gives back is a card on the
// board either way. The entry of the archive goes with it, and the moment and
// the person it was put away by go with the entry.
func (s *Server) removeCard(w http.ResponseWriter, r *http.Request, person *Person) {
	_, column, card := s.visibleCard(r.PathValue("cardId"), person)
	fromTheArchive := false
	if card == nil {
		_, column, card = s.visibleArchivedCard(r.PathValue("cardId"), person)
		fromTheArchive = card != nil
	}
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	if fromTheArchive {
		s.Archive = slices.DeleteFunc(s.Archive, func(a *ArchivedCard) bool { return a.Card == card })
	} else {
		column.Cards = slices.DeleteFunc(column.Cards, func(c *Card) bool { return c == card })
	}
	s.trashSeq++
	s.Trash = append(s.Trash, &TrashedCard{Card: card, Column: column, TrashedAt: s.now, TrashedBy: person.ID, seq: s.trashSeq})

	// The server sends its JSON Content-Type along with the empty 204.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// trash lists what was taken out of a project, the newest first, and says in
// a header when it hands out less than there is.
func (s *Server) trash(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}

	// Every statement of the trash demands the team, so a client reads an
	// empty one.
	if s.isAClient(person.ID) {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	appURL := s.URL + "/projects/" + project.ID + "/trash"
	var entries []trashRow
	for _, trashed := range slices.Backward(s.Trash) {
		board := s.boardOfColumn(trashed.Column)
		if board == nil || board.ProjectID != project.ID {
			continue
		}
		// A card whose column lies in the trash as well comes back only after
		// the column, which only the web brings back.
		var blockedBy any
		if s.trashedColumn(trashed.Column) != nil {
			blockedBy = "column"
		}
		entries = append(entries, trashRow{trashed.TrashedAt, trashed.seq, map[string]any{
			"kind": "card", "id": trashed.Card.ID, "title": trashed.Card.Title, "toolType": nil,
			"board":     map[string]any{"id": board.ID, "title": board.Title},
			"column":    map[string]any{"id": trashed.Column.ID, "type": trashed.Column.Type, "title": columnTitle(trashed.Column), "customTitle": trashed.Column.CustomTitle},
			"trashedAt": timestampOrNil(nilIfZero(trashed.TrashedAt)), "trashedBy": s.personReferenceOrNil(trashed.TrashedBy),
			"restorable": blockedBy == nil, "blockedBy": blockedBy,
			"appUrl": appURL,
		}})
	}
	for _, trashed := range slices.Backward(s.TrashedColumns) {
		if trashed.Board.ProjectID != project.ID {
			continue
		}
		entries = append(entries, trashRow{trashed.TrashedAt, trashed.seq, map[string]any{
			"kind": "column", "id": trashed.Column.ID, "title": trashed.Column.CustomTitle, "toolType": nil,
			"board":     map[string]any{"id": trashed.Board.ID, "title": trashed.Board.Title},
			"column":    nil,
			"trashedAt": timestampOrNil(nilIfZero(trashed.TrashedAt)), "trashedBy": s.personReferenceOrNil(trashed.TrashedBy),
			"restorable": false, "blockedBy": nil,
			"appUrl": appURL,
		}})
	}
	entries = append(entries, s.trashedFilesEntries(project, person)...)
	slices.SortStableFunc(entries, func(a, b trashRow) int {
		if !a.at.Equal(b.at) {
			return b.at.Compare(a.at)
		}
		return b.seq - a.seq
	})
	if len(entries) > TrashEntries {
		entries = entries[:TrashEntries]
		w.Header().Set("X-Minka-Truncated", "true")
	}

	list := []any{}
	for _, e := range entries {
		list = append(list, e.resource)
	}
	writeJSON(w, http.StatusOK, list)
}

// trashRow is one line of the trash of a project, with what orders it: the
// moment it went in, and the count of what went in before for one moment.
type trashRow struct {
	at       time.Time
	seq      int
	resource map[string]any
}

// nilIfZero is how a row from before Minka noted the time says nothing.
func nilIfZero(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	return &at
}

// restoreCard puts a card of the trash back on top of the column it was taken
// from. A card that is not in the trash is a card at an address that has
// nothing, which a second restore is as well.
func (s *Server) restoreCard(w http.ResponseWriter, r *http.Request, person *Person) {
	index := slices.IndexFunc(s.Trash, func(t *TrashedCard) bool { return t.Card.ID == r.PathValue("cardId") })
	if index < 0 {
		notFound(w)
		return
	}
	trashed := s.Trash[index]
	board := s.boardOfColumn(trashed.Column)
	if board == nil || s.visibleProject(board.ProjectID, person) == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return
	}
	if s.trashedColumn(trashed.Column) != nil {
		notFound(w)
		return
	}

	s.Trash = slices.Delete(s.Trash, index, index+1)
	trashed.Card.OnHold = false
	trashed.Column.Cards = append([]*Card{trashed.Card}, trashed.Column.Cards...)
	writeJSON(w, http.StatusOK, s.cardResource(board, trashed.Column, trashed.Card, person))
}

// boardOfColumn is the board a column stands on, or stood on before it went
// into the trash.
func (s *Server) boardOfColumn(column *Column) *Board {
	for _, board := range s.Boards {
		if slices.Contains(board.Columns, column) {
			return board
		}
	}
	if trashed := s.trashedColumn(column); trashed != nil {
		return trashed.Board
	}
	return nil
}

// writeComment reads the body before it looks the card up, as the server
// does, but holds the text only against a card it found.
func (s *Server) writeComment(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	// The server stores the comment as rich text; the fake keeps what was sent.
	text, ok := commentText(w, deref(body))
	if !ok {
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	comment := &Comment{ID: newID(), AuthorID: person.ID, Body: text, CreatedAt: s.now}
	card.subscribe(person.ID)
	card.Comments = append(card.Comments, comment)
	writeJSON(w, http.StatusCreated, s.commentResource(board.ProjectID, s.cardURL(board, card), comment))
}

func (s *Server) visibleProject(id string, person *Person) *Project {
	for _, p := range s.Projects {
		if p.ID == id && slices.Contains(p.MemberIDs, person.ID) {
			return p
		}
	}
	return nil
}

func (s *Server) visibleBoard(id string, person *Person) *Board {
	for _, b := range s.Boards {
		if b.ID == id && !b.Trashed && s.visibleProject(b.ProjectID, person) != nil {
			return b
		}
	}
	return nil
}

func (s *Server) visibleColumn(id string, person *Person) (*Board, *Column) {
	for _, b := range s.Boards {
		if s.visibleProject(b.ProjectID, person) == nil {
			continue
		}
		for _, c := range b.Columns {
			if c.ID == id {
				return b, c
			}
		}
	}
	return nil, nil
}

// visibleArchivedCard finds a card in the archive of a board of a project this
// person is in, together with the board and the column it was taken from.
func (s *Server) visibleArchivedCard(id string, person *Person) (*Board, *Column, *Card) {
	archived := s.inTheArchive(id)
	if archived == nil {
		return nil, nil, nil
	}
	board := s.boardOfColumn(archived.Column)
	if board == nil || s.visibleProject(board.ProjectID, person) == nil {
		return nil, nil, nil
	}
	return board, archived.Column, archived.Card
}

// inTheArchive is the entry of a board's archive for this card, and nil for a
// card that stands on a board.
func (s *Server) inTheArchive(id string) *ArchivedCard {
	for _, archived := range s.Archive {
		if archived.Card.ID == id {
			return archived
		}
	}
	return nil
}

// cardToWrite finds a card the way a write looks it up: on its board or in
// the archive of its board, which a write learns of only after the project.
func (s *Server) cardToWrite(id string, person *Person) (board *Board, column *Column, card *Card, archived bool) {
	if board, column, card = s.visibleCard(id, person); card != nil {
		return board, column, card, false
	}
	board, column, card = s.visibleArchivedCard(id, person)
	return board, column, card, card != nil
}

// refusedAsAClosedCard says why a write on a card wrote nothing: the project
// is archived, or the card lies in the archive of its board.
func (s *Server) refusedAsAClosedCard(w http.ResponseWriter, board *Board, archived bool) bool {
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return true
	}
	if archived {
		unprocessable(w, "cardArchived", "This card is archived. Nothing is written on it until it is back on its board.")
		return true
	}
	return false
}

func (s *Server) visibleCard(id string, person *Person) (*Board, *Column, *Card) {
	for _, b := range s.Boards {
		if s.visibleProject(b.ProjectID, person) == nil {
			continue
		}
		for _, c := range b.Columns {
			for _, card := range c.Cards {
				if card.ID == id {
					return b, c, card
				}
			}
		}
	}
	return nil, nil, nil
}

func (s *Server) cardResource(board *Board, column *Column, card *Card, reader *Person) map[string]any {
	// Minka orders the comments by the moment they were written and then by
	// their ID, which it makes from the clock as well, so two of the same
	// moment stand in the order they were written in.
	ordered := slices.Clone(card.Comments)
	slices.SortStableFunc(ordered, func(a, b *Comment) int { return a.CreatedAt.Compare(b.CreatedAt) })
	comments := []any{}
	for _, comment := range ordered {
		comments = append(comments, s.commentResource(board.ProjectID, s.cardURL(board, card), comment))
	}
	project := s.projectOf(board)
	resource := s.withShortURL(card.ShortID, map[string]any{
		"id": card.ID, "shortId": nullable(card.ShortID), "projectId": board.ProjectID, "boardId": board.ID,
		"project":     map[string]any{"id": project.ID, "name": project.Name},
		"board":       map[string]any{"id": board.ID, "title": board.Title},
		"title":       card.Title,
		"description": card.Description, "descriptionSource": card.Description,
		"attachments":  s.attachmentsIn(board.ProjectID, card.Description),
		"column":       map[string]any{"id": column.ID, "type": column.Type, "title": columnTitle(column), "customTitle": column.CustomTitle},
		"archived":     s.inTheArchive(card.ID) != nil,
		"commentCount": len(card.Comments), "createdAt": timestamp(card.CreatedAt),
		"creator": s.personReference(card.CreatorID), "completedAt": timestampOrNil(card.CompletedAt),
		"completedBy": s.personReferenceOrNil(card.CompletedByID), "assignees": s.assigneeReferences(card),
		"dueOn": card.DueOn, "onHold": card.OnHold, "steps": s.stepResources(card, reader), "subscribers": s.subscriberReferences(card),
		"comments": comments, "labels": s.labelResources(s.LabelsOn(card.ID)),
		"appUrl": s.cardURL(board, card),
	})
	if !s.KnowsNoCardSteps {
		resource["stepOf"] = s.parentsOf(card, reader)
	}
	return resource
}

func (s *Server) reactionResources(reactions []*Reaction) []any {
	list := []any{}
	for _, reaction := range reactions {
		people := []any{}
		for _, personID := range reaction.PersonIDs {
			people = append(people, s.personReference(personID))
		}
		list = append(list, map[string]any{"content": reaction.Content, "people": people})
	}
	return list
}

// commentResource answers a comment with the page of its item opened at it,
// the address its notification leads to.
func (s *Server) commentResource(circle, itemURL string, comment *Comment) map[string]any {
	resource := map[string]any{
		"id": comment.ID, "author": s.personReference(comment.AuthorID), "body": comment.Body, "bodySource": comment.Body,
		"attachments": s.attachmentsIn(circle, comment.Body),
		"reactions":   s.reactionResources(comment.Reactions),
		"createdAt":   timestamp(comment.CreatedAt), "editedAt": timestampOrNil(comment.EditedAt),
	}
	if !s.HandsOutNoCommentURL {
		resource["appUrl"] = itemURL + "#comment-" + comment.ID
	}
	return resource
}

func (s *Server) personReference(id string) map[string]any {
	return map[string]any{"id": id, "name": s.People[id].Name}
}

func (s *Server) personReferenceOrNil(id string) any {
	if id == "" {
		return nil
	}
	return s.personReference(id)
}

// withShortURL adds the address of a card by its short identifier, which keeps
// leading to it after the board's prefix is renamed, and null for a card of a
// board without a prefix.
func (s *Server) withShortURL(shortID string, resource map[string]any) map[string]any {
	if s.HandsOutNoShortURL {
		return resource
	}
	resource["shortUrl"] = nil
	if shortID != "" {
		resource["shortUrl"] = s.URL + "/go/card/" + shortID
	}
	return resource
}

func (s *Server) boardURL(b *Board) string {
	return s.URL + "/projects/" + b.ProjectID + "/boards/" + b.ID
}

func (s *Server) cardURL(b *Board, c *Card) string {
	return s.boardURL(b) + "/cards/" + c.ID
}

func columnTitle(c *Column) string {
	if c.CustomTitle != nil {
		return *c.CustomTitle
	}
	return map[string]string{"triage": "Triage", "not_now": "Not now", "done": "Done", "column": "Column"}[c.Type]
}

func readBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	return readBodyWith(w, r, false)
}

// endsAfterOneValue says whether nothing but white space follows the value a
// decoder read, the way the server refuses anything behind a body.
func endsAfterOneValue(decoder *json.Decoder) bool {
	_, err := decoder.Token()
	return err == io.EOF
}

// readBodyWith reads a body, keeping every number as it was written where
// asked to, so that 1 and 1.0 can be told apart as the server tells them.
func readBodyWith(w http.ResponseWriter, r *http.Request, keepNumbers bool) (map[string]any, bool) {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]))
	if mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, errorBody("unsupportedMediaType", "This address takes a body of the type application/json."))
		return nil, false
	}
	content, err := io.ReadAll(r.Body)
	var value any
	decoder := json.NewDecoder(bytes.NewReader(content))
	if keepNumbers {
		decoder.UseNumber()
	}
	if err != nil || decoder.Decode(&value) != nil || !endsAfterOneValue(decoder) {
		malformed(w, "The body is not valid JSON.")
		return nil, false
	}
	fields, ok := value.(map[string]any)
	if !ok {
		malformed(w, "The body is not a JSON object.")
		return nil, false
	}
	identifiersOfTheBodyInAnyCase(fields)
	return fields, true
}

// carriesArgumentsOfWebForms says whether a request names an argument the web
// reserves for its forms, one whose name starts with two underscores. The
// server refuses such a request before it looks at the address, for any
// method. It takes its arguments from the query and from the body as Flow
// parses it: a body of a JSON media type as JSON, one of any other subtype
// as a form, one without a media type not at all, and the bytes of an upload
// not at all. A body is peeked at here and left to be read again. An XML
// body, which Flow reads as well, is not looked into: no address takes one.
func carriesArgumentsOfWebForms(r *http.Request) bool {
	for name := range r.URL.Query() {
		if strings.HasPrefix(name, "__") {
			return true
		}
	}
	if r.Body == nil || takesTheBytesOfAFile(r) {
		return false
	}
	content, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(content))
	_, subtype, _ := strings.Cut(strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0])), "/")
	var names []string
	switch subtype {
	case "":
		return false
	case "json", "x-json", "javascript", "x-javascript":
		var fields map[string]json.RawMessage
		if json.Unmarshal(content, &fields) != nil {
			return false
		}
		for name := range fields {
			names = append(names, name)
		}
	case "xml":
		return false
	default:
		fields, _ := url.ParseQuery(string(content))
		for name := range fields {
			names = append(names, name)
		}
	}
	for _, name := range names {
		if strings.HasPrefix(name, "__") {
			return true
		}
	}
	return false
}

// takesTheBytesOfAFile says whether a request carries the bytes of an upload,
// whose body the server keeps out of its parsing.
func takesTheBytesOfAFile(r *http.Request) bool {
	return r.Method == http.MethodPost && uploadPath.MatchString(r.URL.Path)
}

var uploadPath = regexp.MustCompile(`^/api/v1/(projects|messages)/[^/]+/uploads$`)

func stringField(w http.ResponseWriter, fields map[string]any, name string) (*string, bool) {
	value, present := fields[name]
	if !present || value == nil {
		return nil, true
	}
	s, ok := value.(string)
	if !ok {
		malformed(w, fmt.Sprintf("The field %q has to be a string.", name))
		return nil, false
	}
	return &s, true
}

func booleanField(w http.ResponseWriter, fields map[string]any, name string) (*bool, bool) {
	value, present := fields[name]
	if !present || value == nil {
		return nil, true
	}
	b, ok := value.(bool)
	if !ok {
		malformed(w, fmt.Sprintf("The field %q has to be true or false.", name))
		return nil, false
	}
	return &b, true
}

// validDescription checks the length of what was sent, then invisible
// characters once the harmless ones are stripped. The server counts the
// sanitized HTML instead, which only differs for markup it removes.
func validDescription(w http.ResponseWriter, description string) bool {
	if utf8.RuneCountInString(phpTrim(description)) > maxDescriptionLength {
		unprocessable(w, "cardDescriptionTooLong", "The description of a card is too long.")
		return false
	}
	if hasInvisibleCharactersInMarkup(description) {
		unprocessable(w, "cardDescriptionHasInvisibleCharacters", "The description of a card must not contain invisible format characters such as direction overrides, isolates or tag characters; the zero-width joiner and non-joiner are fine.")
		return false
	}
	return true
}

func indexOf(cards []*Card, id string) int {
	return slices.IndexFunc(cards, func(c *Card) bool { return c.ID == id })
}

func notFound(w http.ResponseWriter) {
	writeRaw(w, http.StatusNotFound, NotFoundBody)
}

// unknownAddress is what Flow answers an unknown path or a wrong method with:
// an HTML page, not the API's error.
func unknownAddress(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, "<html><body>Not found</body></html>")
}

func malformed(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, errorBody("malformedBody", message))
}

func unprocessable(w http.ResponseWriter, code, message string) {
	writeJSON(w, http.StatusUnprocessableEntity, errorBody(code, message))
}

// nullable is null for an empty text, the way the API answers for what is not
// there.
func nullable(text string) any {
	if text == "" {
		return nil
	}
	return text
}

func errorBody(code, message string) map[string]any {
	return map[string]any{"error": map[string]string{"code": code, "message": message}}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	writeRaw(w, status, string(encoded))
}

func writeRaw(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// timestampLayout is the one form the API gives points in time in: UTC with
// microseconds.
const timestampLayout = "2006-01-02T15:04:05.000000Z"

func timestamp(t time.Time) string {
	return t.UTC().Format(timestampLayout)
}

func timestampOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return timestamp(*t)
}

// validDueDate accepts no date at all, an empty one, or a day that exists,
// written as YYYY-MM-DD. 2026-02-30 and 2026-9-8 are refused with the code
// of what the day is to stand on.
func validDueDate(w http.ResponseWriter, value *string, notADateCode string) bool {
	date := phpTrim(deref(value))
	if date == "" {
		return true
	}
	if strings.IndexFunc(date, unicode.IsControl) < 0 {
		if day, err := time.Parse("2006-01-02", date); err == nil && day.Format("2006-01-02") == date {
			return true
		}
	}
	unprocessable(w, notADateCode, "A due date is a day written as YYYY-MM-DD.")
	return false
}

// dueDate is how a checked due date is stored: nil where the text names none.
func dueDate(value *string) *string {
	date := phpTrim(deref(value))
	if date == "" {
		return nil
	}
	return &date
}

func stringListField(w http.ResponseWriter, fields map[string]any, name string) ([]string, bool) {
	value, present := fields[name]
	if !present || value == nil {
		return nil, true
	}
	list, ok := value.([]any)
	if !ok {
		malformed(w, fmt.Sprintf("The field %q has to be a list.", name))
		return nil, false
	}
	texts := make([]string, 0, len(list))
	for _, entry := range list {
		text, ok := entry.(string)
		if !ok {
			malformed(w, fmt.Sprintf("The field %q has to be a list of strings.", name))
			return nil, false
		}
		texts = append(texts, text)
	}
	return texts, true
}

// phpTrim trims like PHP's trim(), which the server uses: it removes NUL and
// vertical tab, and no Unicode spaces.
func phpTrim(s string) string {
	return strings.Trim(s, " \t\n\r\x00\x0B")
}

func completion(c *Card) time.Time {
	if c.CompletedAt == nil {
		return time.Time{}
	}
	return *c.CompletedAt
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
