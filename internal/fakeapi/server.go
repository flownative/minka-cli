// Package fakeapi is an in-memory Minka API for tests.
//
// It follows the contract of /api/v1 as the Minka server implements it: the
// same paths, status codes and error codes, checked in the same order, and the
// byte-identical 401 and 404 bodies. It does not sanitize descriptions, so a
// test sees exactly the HTML the CLI sent.
package fakeapi

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
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
	maxChatMessageLength = 10000
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Person struct {
	ID          string
	Name        string
	Email       string
	PrincipalID string
	Retired     bool // an agent that stepped down: gone from every circle, and its tokens with it
}

type Project struct {
	ID          string
	Name        string
	Description string
	MemberIDs   []string
	Archived    bool // readable, and closed to every write but the read mark of a chat
}

type Board struct {
	ID        string
	ProjectID string
	Title     string
	Columns   []*Column
}

type Column struct {
	ID          string
	Type        string
	CustomTitle *string
	// Cards holds both areas of the column. The cards on hold keep their
	// order among themselves, the others theirs; how the two interleave here
	// means nothing.
	Cards         []*Card
	OnHoldEnabled bool // only a working column can have an area for cards on hold
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
}

type Step struct {
	ID        string
	Title     string
	Completed bool
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
}

// TrashEntries is how many entries an answer carries at most, as the server
// caps them.
const TrashEntries = 200

// MaxUploadBytes is what Minka takes for one file.
const MaxUploadBytes = 50 << 20

// Upload is a file somebody put into a project. The bytes themselves are of no
// interest here; what matters is that the answer carries an attachment whose
// handle only this project has.
type Upload struct {
	ID          string
	Circle      string // the project, or "conversation:" and the conversation
	Filename    string
	ContentType string
	Size        int
}

// handle stands for the signature the server puts on the pair of circle and
// file, which is what makes an attachment belong to a text of that circle and
// of no other.
func (u *Upload) handle() string {
	return "sgid-" + u.Circle + "-" + u.ID
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

	People   map[string]*Person
	Tokens   map[string]string // token → person ID
	Projects []*Project
	Boards   []*Board
	Chats    []*Chat
	Uploads  []*Upload
	Trash    []*TrashedCard

	// IgnoresProjectStatus makes the server answer like a Minka from before
	// projects could be archived: no status, and the filter goes unnoticed.
	IgnoresProjectStatus bool

	// AnswersCardsWithoutID makes the lookup of a short identifier answer with
	// a card that has no ID, which no Minka does: what a client does with an
	// answer it cannot use must not depend on that.
	AnswersCardsWithoutID bool

	mu       sync.Mutex
	requests []RecordedRequest
	failWith int
	now      time.Time
}

// Fixture names the data New seeds, for tests to refer to.
type Fixture struct {
	Token         string
	StrangerToken string
	Bruno         *Person
	Anke          *Person // a member of the project besides Bruno
	Stranger      *Person // no member of the project
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
}

// New starts a fake with a small fixture: Bruno is a member of one project
// with one board; a second project with a board and a card belongs to
// somebody else. The server stops when the test ends.
func New(t testing.TB) (*Server, *Fixture) {
	s := &Server{
		People: map[string]*Person{},
		Tokens: map[string]string{},
		now:    time.Date(2026, 9, 13, 14, 2, 11, 123456000, time.UTC),
	}

	bruno := &Person{ID: newID(), Name: "Bruno", Email: "bruno@example.com"}
	stranger := &Person{ID: newID(), Name: "Stranger", Email: "stranger@example.com"}
	agent := &Person{ID: newID(), Name: "Claude (Bruno)", Email: "claude-bruno@agents.invalid", PrincipalID: bruno.ID}
	anke := &Person{ID: newID(), Name: "Anke", Email: "anke@example.com"}
	retired := &Person{ID: newID(), Name: "Claude (Anke)", Email: "claude-anke@agents.invalid", PrincipalID: anke.ID, Retired: true}
	for _, p := range []*Person{bruno, anke, stranger, agent, retired} {
		s.People[p.ID] = p
	}
	s.Tokens["minka_pat_bruno"] = bruno.ID
	s.Tokens["minka_pat_stranger"] = stranger.ID
	s.Tokens["minka_pat_agent"] = agent.ID

	project := &Project{ID: newID(), Name: "Test-Projekt", Description: "For trying things", MemberIDs: []string{bruno.ID, anke.ID, agent.ID, retired.ID}}
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", s.authenticated(s.me))
	mux.HandleFunc("GET /api/v1/projects", s.authenticated(s.projects))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/boards", s.authenticated(s.boards))
	mux.HandleFunc("GET /api/v1/boards/{boardId}", s.authenticated(s.board))
	mux.HandleFunc("GET /api/v1/me/cards", s.authenticated(s.myCards))
	mux.HandleFunc("GET /api/v1/cards/{cardId}", s.authenticated(s.card))
	mux.HandleFunc("GET /api/v1/cards/by-short-id/{identifier}", s.authenticated(s.cardByShortIdentifier))
	mux.HandleFunc("POST /api/v1/boards/{boardId}/cards", s.authenticated(s.addCard))
	mux.HandleFunc("PATCH /api/v1/cards/{cardId}", s.authenticated(s.updateCard))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/moves", s.authenticated(s.moveCard))
	mux.HandleFunc("PUT /api/v1/columns/{columnId}/on-hold", s.authenticated(s.setColumnOnHold))
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
	mux.HandleFunc("POST /api/v1/cards/{cardId}/steps", s.authenticated(s.addStep))
	mux.HandleFunc("PATCH /api/v1/cards/{cardId}/steps/{stepId}", s.authenticated(s.updateStep))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/steps/{stepId}", s.authenticated(s.removeStep))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/subscribers/{personId}", s.authenticated(s.subscribeToCard))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/subscribers/{personId}", s.authenticated(s.unsubscribeFromCard))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/chats", s.authenticated(s.chats))
	mux.HandleFunc("GET /api/v1/messages", s.authenticated(s.conversations))
	mux.HandleFunc("POST /api/v1/messages/{first}/{second}", s.authenticated(s.beginOrUpload))
	mux.HandleFunc("GET /api/v1/chats/{chatId}/messages", s.authenticated(s.messages))
	mux.HandleFunc("POST /api/v1/chats/{chatId}/messages", s.authenticated(s.postMessage))
	mux.HandleFunc("PATCH /api/v1/chats/{chatId}/messages/{messageId}", s.authenticated(s.editMessage))
	mux.HandleFunc("DELETE /api/v1/chats/{chatId}/messages/{messageId}", s.authenticated(s.removeMessage))
	mux.HandleFunc("POST /api/v1/chats/{chatId}/read", s.authenticated(s.markChatRead))
	mux.HandleFunc("POST /api/v1/projects/{projectId}/uploads", s.authenticated(s.upload))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Flow answers an unknown path or a wrong method with an HTML page.
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<html><body>Not found</body></html>")
	})

	s.Server = httptest.NewServer(s.recording(mux))
	t.Cleanup(s.Close)

	return s, &Fixture{
		Token: "minka_pat_bruno", StrangerToken: "minka_pat_stranger",
		Bruno: bruno, Anke: anke, Stranger: stranger, Agent: agent, RetiredAgent: retired, Project: project, Board: board,
		Triage: triage, Doing: doing, NotNow: notNow, Done: done,
		CardInTriage: cardInTriage, CardInDoing: cardInDoing,
		OtherProject: otherProject, OtherBoard: otherBoard, OtherCard: otherCard,
		Chat: chat, OtherChat: otherChat, Conversation: conversation,
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

// FailNextWith makes the next request answer with this status and a body that
// is not JSON, like a proxy in front of the server would.
func (s *Server) FailNextWith(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failWith = status
}

func (s *Server) recording(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		s.mu.Lock()
		s.requests = append(s.requests, RecordedRequest{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body})
		failWith := s.failWith
		s.failWith = 0
		s.mu.Unlock()

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
		next.ServeHTTP(w, r)
	})
}

type handler func(w http.ResponseWriter, r *http.Request, person *Person)

func (s *Server) authenticated(next handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		if accept != "" && !strings.Contains(accept, "application/json") && !strings.Contains(accept, "*/*") {
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
			if slices.Contains(p.MemberIDs, person.ID) {
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
		if slices.Contains(p.MemberIDs, person.ID) && p.Archived == archived {
			list = append(list, map[string]any{"id": p.ID, "name": p.Name, "description": p.Description, "status": projectStatus(p), "appUrl": s.URL + "/projects/" + p.ID})
		}
	}
	writeJSON(w, http.StatusOK, list)
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
		if b.ProjectID == project.ID {
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
		columns = append(columns, s.columnResource(board, column))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": board.ID, "projectId": board.ProjectID, "title": board.Title, "appUrl": s.boardURL(board), "columns": columns,
	})
}

// columnResource lists the cards of a column in its two areas, and counts the
// ones on hold apart from the rest.
func (s *Server) columnResource(board *Board, column *Column) map[string]any {
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
	return map[string]any{
		"id": column.ID, "type": column.Type, "title": columnTitle(column), "customTitle": column.CustomTitle,
		"cardCount": count, "cards": s.boardCardResources(board, listed),
		"onHoldEnabled": column.OnHoldEnabled, "onHoldCardCount": len(onHold), "onHoldCards": s.boardCardResources(board, onHold),
	}
}

func (s *Server) boardCardResources(board *Board, cards []*Card) []any {
	list := []any{}
	for _, card := range cards {
		list = append(list, map[string]any{
			"id": card.ID, "shortId": nullable(card.ShortID), "title": card.Title, "hasDescription": card.Description != "",
			"stepCount": len(card.Steps), "completedStepCount": completedStepCount(card),
			"commentCount": len(card.Comments), "createdAt": timestamp(card.CreatedAt),
			"creator": s.personReference(card.CreatorID), "completedAt": timestampOrNil(card.CompletedAt),
			"completedBy": s.personReferenceOrNil(card.CompletedByID), "assignees": s.assigneeReferences(card),
			"dueOn": card.DueOn, "appUrl": s.cardURL(board, card),
		})
	}
	return list
}

// myCards lists the cards the person is assigned to, in the order the server
// gives them: projects by name, then by due date with the ones without a date
// last, then by title. Done is left out, as are cards of a project the person
// is not a member of.
func (s *Server) myCards(w http.ResponseWriter, _ *http.Request, person *Person) {
	type assigned struct {
		board  *Board
		column *Column
		card   *Card
	}
	var mine []assigned
	for _, board := range s.Boards {
		// An archived project has nothing left to do, so its cards are not
		// among them, although each can still be read by itself.
		if project := s.visibleProject(board.ProjectID, person); project == nil || project.Archived {
			continue
		}
		for _, column := range board.Columns {
			if column.Type == "done" {
				continue
			}
			for _, card := range column.Cards {
				if slices.Contains(card.AssigneeIDs, person.ID) {
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
		list = append(list, s.cardResource(item.board, item.column, item.card))
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

func (s *Server) projectOf(board *Board) *Project {
	for _, project := range s.Projects {
		if project.ID == board.ProjectID {
			return project
		}
	}
	return &Project{}
}

func (s *Server) card(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card))
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
				if card.ShortID != "" && card.ShortID == identifier {
					resource := s.cardResource(board, column, card)
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
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok1 := stringField(w, fields, "title")
	columnID, ok2 := stringField(w, fields, "columnId")
	description, ok3 := stringField(w, fields, "description")
	if !ok1 || !ok2 || !ok3 || !validTitle(w, deref(title)) || (description != nil && !validDescription(w, *description)) {
		return
	}
	dueOn, ok := stringField(w, fields, "dueOn")
	if !ok || !validDueDate(w, dueOn) {
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

	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil {
		notFound(w)
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

	// Card and people are one transaction: somebody outside the project
	// leaves no card behind.
	for _, id := range assigneeIDs {
		if !slices.Contains(s.visibleProject(board.ProjectID, person).MemberIDs, id) {
			notAProjectMember(w)
			return
		}
	}

	card := &Card{ID: newID(), Title: phpTrim(*title), Description: phpTrim(deref(description)), CreatedAt: s.now, CreatorID: person.ID, DueOn: dueDate(dueOn)}
	card.subscribe(person.ID)
	for _, id := range assigneeIDs {
		if !slices.Contains(card.AssigneeIDs, id) {
			card.AssigneeIDs = append(card.AssigneeIDs, id)
			card.subscribe(id)
		}
	}
	column.Cards = append([]*Card{card}, column.Cards...)
	w.Header().Set("Location", s.URL+"/api/v1/cards/"+card.ID)
	writeJSON(w, http.StatusCreated, s.cardResource(board, column, card))
}

func (s *Server) updateCard(w http.ResponseWriter, r *http.Request, person *Person) {
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
	if title != nil && !validTitle(w, *title) || description != nil && !validDescription(w, *description) {
		return
	}
	dueOn, ok := stringField(w, fields, "dueOn")
	if !ok || !validDueDate(w, dueOn) {
		return
	}

	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	if title != nil {
		card.Title = phpTrim(*title)
	}
	if description != nil {
		card.Description = phpTrim(*description)
	}
	if dueDateNamed {
		card.DueOn = dueDate(dueOn)
	}
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card))
}

func (s *Server) moveCard(w http.ResponseWriter, r *http.Request, person *Person) {
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

	board, from, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
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
	writeJSON(w, http.StatusOK, s.cardResource(board, to, card))
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

	board, column := s.visibleColumn(r.PathValue("columnId"), person)
	if column == nil {
		notFound(w)
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
	writeJSON(w, http.StatusOK, s.columnResource(board, column))
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

// assignCard checks the body before it looks up the card, as the server does:
// a person that is no ID is refused like a person outside the project.
func (s *Server) assignCard(w http.ResponseWriter, r *http.Request, person *Person) {
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

	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
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
	writeJSON(w, status, s.cardResource(board, column, card))
}

// unassignCard takes somebody off a card without asking about membership, so
// a person who left the project can still be taken off.
func (s *Server) unassignCard(w http.ResponseWriter, r *http.Request, person *Person) {
	_, _, card := s.visibleCard(r.PathValue("cardId"), person)
	personID := r.PathValue("personId")
	if card == nil || !uuidPattern.MatchString(personID) {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	card.AssigneeIDs = slices.DeleteFunc(card.AssigneeIDs, func(id string) bool { return id == personID })

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// addStep puts a step at the end of the list. The line is checked before the
// card is looked up, in the order the server checks it.
func (s *Server) addStep(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	text := phpTrim(deref(title))
	switch {
	case utf8.RuneCountInString(text) > maxStepLength:
		unprocessable(w, "stepTooLong", "The line of a step is too long.")
		return
	case strings.IndexFunc(text, unicode.IsControl) >= 0:
		unprocessable(w, "stepHasControlCharacters", "The line of a step must not contain control characters.")
		return
	case text == "":
		unprocessable(w, "stepMissing", "A step needs a line of text.")
		return
	}

	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	card.Steps = append(card.Steps, &Step{ID: newID(), Title: text})
	writeJSON(w, http.StatusCreated, s.cardResource(board, column, card))
}

func (s *Server) updateStep(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	completed, isFlag := fields["completed"].(bool)
	if !isFlag {
		unprocessable(w, "stepCompletedMissing", `Changing a step needs the field "completed" as true or false.`)
		return
	}

	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	index := slices.IndexFunc(card.Steps, func(step *Step) bool { return step.ID == r.PathValue("stepId") })
	if index < 0 {
		notFound(w)
		return
	}
	card.Steps[index].Completed = completed
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card))
}

// removeStep answers 204 for a step that is not on the card as well, but 404
// for a card the person may not see.
func (s *Server) removeStep(w http.ResponseWriter, r *http.Request, person *Person) {
	_, _, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	card.Steps = slices.DeleteFunc(card.Steps, func(step *Step) bool { return step.ID == r.PathValue("stepId") })

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func completedStepCount(card *Card) int {
	completed := 0
	for _, step := range card.Steps {
		if step.Completed {
			completed++
		}
	}
	return completed
}

func stepResources(card *Card) []any {
	list := []any{}
	for _, step := range card.Steps {
		list = append(list, map[string]any{"id": step.ID, "title": step.Title, "completed": step.Completed})
	}
	return list
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
		r.SetPathValue("personId", r.PathValue("second"))
		s.startConversation(w, r, person)
		return
	}
	if r.PathValue("second") != "uploads" {
		notFound(w)
		return
	}
	r.SetPathValue("conversationId", r.PathValue("first"))
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
		list = append(list, s.chatMessageResource(message))
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
	clientMessageID, ok := stringField(w, fields, "clientMessageId")
	if !ok {
		return
	}
	if deref(clientMessageID) != "" && !uuidPattern.MatchString(*clientMessageID) {
		unprocessable(w, "chatMessageDraftNotAnIdentifier", "The name of the draft has to be a UUID.")
		return
	}

	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		notFound(w)
		return
	}
	// Asked before a message sent earlier is looked for: in an archived
	// project a refused write must not pass for one that went through.
	if s.refusedAsArchived(w, s.projectOfChat(chat)) {
		return
	}
	if deref(clientMessageID) != "" {
		for _, message := range chat.Messages {
			if message.ClientMessageID == *clientMessageID && message.AuthorID == person.ID {
				writeJSON(w, http.StatusOK, s.chatMessageResource(message))
				return
			}
		}
	}
	writeJSON(w, http.StatusCreated, s.chatMessageResource(chat.post(person.ID, text, deref(clientMessageID), s.now)))
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

func (s *Server) editMessage(w http.ResponseWriter, r *http.Request, person *Person) {
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
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		notFound(w)
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
	writeJSON(w, http.StatusOK, s.chatMessageResource(message))
}

func (s *Server) removeMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	chat := s.visibleChat(r.PathValue("chatId"), person)
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
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	value, isNumber := fields["seq"].(float64)
	if !isNumber || value < 0 || value != math.Trunc(value) {
		unprocessable(w, "chatReadSeqMissing", "The number a chat has been read up to has to be a whole number of at least zero.")
		return
	}
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		notFound(w)
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
// work is in reactTo and unreactFrom.
func (s *Server) reactToComment(w http.ResponseWriter, r *http.Request, person *Person) {
	_, _, card := s.visibleCard(r.PathValue("cardId"), person)
	comment := commentOf(card, r.PathValue("commentId"))
	if comment == nil {
		notFound(w)
		return
	}
	content, ok := reactionContent(w, r)
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	status := reactTo(&comment.Reactions, content, person.ID)
	writeJSON(w, status, s.commentResource(comment))
}

func (s *Server) unreactFromComment(w http.ResponseWriter, r *http.Request, person *Person) {
	_, _, card := s.visibleCard(r.PathValue("cardId"), person)
	comment := commentOf(card, r.PathValue("commentId"))
	if comment == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	unreactFrom(&comment.Reactions, r.URL.Query().Get("content"), person.ID)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reactToMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	message := s.messageOfChat(r, person)
	if message == nil {
		notFound(w)
		return
	}
	content, ok := reactionContent(w, r)
	if !ok {
		return
	}
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if s.refusedAsArchived(w, s.projectOfChat(chat)) {
		return
	}
	status := reactTo(&message.Reactions, content, person.ID)
	if status == http.StatusCreated {
		chat.noteChange(message)
	}
	writeJSON(w, status, s.chatMessageResource(message))
}

func (s *Server) unreactFromMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	message := s.messageOfChat(r, person)
	if message == nil {
		notFound(w)
		return
	}
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if s.refusedAsArchived(w, s.projectOfChat(chat)) {
		return
	}
	if unreactFrom(&message.Reactions, r.URL.Query().Get("content"), person.ID) {
		chat.noteChange(message)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// messageOfChat is the message of the chat the address names. A message that
// belongs to another chat is as unreachable as one that is not there.
func (s *Server) messageOfChat(r *http.Request, person *Person) *ChatMessage {
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		return nil
	}
	for _, message := range chat.Messages {
		if message.ID == r.PathValue("messageId") && !message.Removed {
			return message
		}
	}
	return nil
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
	content := phpTrim(deref(value))
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
	}
	s.Uploads = append(s.Uploads, upload)
	writeJSON(w, http.StatusCreated, s.uploadResource(upload))
}

func isPicture(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
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
			upload.handle(), upload.ContentType, address, upload.Filename, upload.Size,
		),
	}
}

func tooLarge(w http.ResponseWriter) {
	writeJSON(w, http.StatusRequestEntityTooLarge, errorBody("tooLarge", fmt.Sprintf("A file must not be larger than %d bytes.", MaxUploadBytes)))
}

// mediaType is the type itself, without what a header says about its encoding.
func mediaType(header string) string {
	return strings.TrimSpace(strings.Split(header, ";")[0])
}

func (s *Server) chatMessageResource(message *ChatMessage) map[string]any {
	return map[string]any{
		"id": message.ID, "seq": message.Seq, "changeSeq": message.ChangeSeq,
		"author": s.personReference(message.AuthorID), "bodyHtml": message.BodyHTML, "bodyText": message.BodyText, "bodySource": message.BodyHTML,
		"createdAt": timestamp(message.CreatedAt), "editedAt": timestampOrNil(message.EditedAt), "removed": message.Removed,
		"reactions": s.reactionResources(message.Reactions),
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
	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	personID := r.PathValue("personId")
	if !s.namedMember(board, personID, person) {
		notAProjectMember(w)
		return
	}
	card.OffIDs = slices.DeleteFunc(card.OffIDs, func(id string) bool { return id == personID })
	card.subscribe(personID)
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card))
}

func (s *Server) unsubscribeFromCard(w http.ResponseWriter, r *http.Request, person *Person) {
	board, _, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	personID := r.PathValue("personId")
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

// editComment looks up card and comment before it checks the text, in the
// order the server does.
func (s *Server) editComment(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	_, _, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	comment := ownComment(card, r.PathValue("commentId"), person)
	if comment == nil {
		notFound(w)
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	text := phpTrim(deref(body))
	switch {
	case strings.IndexFunc(text, isCommentControl) >= 0:
		unprocessable(w, "commentHasControlCharacters", "A comment must not contain control characters other than line breaks and tabs.")
		return
	case utf8.RuneCountInString(text) > maxCommentLength:
		unprocessable(w, "commentTooLong", "The comment is too long.")
		return
	case text == "":
		unprocessable(w, "commentMissing", "A comment needs a body.")
		return
	}
	if comment.Body != text {
		now := s.now
		comment.Body, comment.EditedAt = text, &now
	}
	writeJSON(w, http.StatusOK, s.commentResource(comment))
}

// isCommentControl is a control character a comment may not hold: all of
// them but line breaks and tabs.
func isCommentControl(r rune) bool {
	return unicode.IsControl(r) && r != 9 && r != 10 && r != 13
}

func (s *Server) removeComment(w http.ResponseWriter, r *http.Request, person *Person) {
	_, _, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
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

// removeCard moves a card to the trash. The fake forgets it, which reads the
// same: every later request for it is 404.
func (s *Server) removeCard(w http.ResponseWriter, r *http.Request, person *Person) {
	_, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	column.Cards = slices.DeleteFunc(column.Cards, func(c *Card) bool { return c == card })
	s.Trash = append(s.Trash, &TrashedCard{Card: card, Column: column, TrashedAt: s.now, TrashedBy: person.ID})

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

	var entries []*TrashedCard
	for _, trashed := range s.Trash {
		if board := s.boardOfColumn(trashed.Column); board != nil && board.ProjectID == project.ID {
			entries = append(entries, trashed)
		}
	}
	slices.Reverse(entries)
	if len(entries) > TrashEntries {
		entries = entries[:TrashEntries]
		w.Header().Set("X-Minka-Truncated", "true")
	}

	list := []any{}
	for _, trashed := range entries {
		board := s.boardOfColumn(trashed.Column)
		list = append(list, map[string]any{
			"kind": "card", "id": trashed.Card.ID, "title": trashed.Card.Title, "toolType": nil,
			"board":     map[string]any{"id": board.ID, "title": board.Title},
			"column":    map[string]any{"id": trashed.Column.ID, "type": trashed.Column.Type, "title": columnTitle(trashed.Column), "customTitle": trashed.Column.CustomTitle},
			"trashedAt": timestampOrNil(nilIfZero(trashed.TrashedAt)), "trashedBy": s.personReferenceOrNil(trashed.TrashedBy),
			"restorable": true, "blockedBy": nil,
			"appUrl": s.URL + "/projects/" + project.ID + "/trash",
		})
	}
	writeJSON(w, http.StatusOK, list)
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

	s.Trash = slices.Delete(s.Trash, index, index+1)
	trashed.Card.OnHold = false
	trashed.Column.Cards = append([]*Card{trashed.Card}, trashed.Column.Cards...)
	writeJSON(w, http.StatusOK, s.cardResource(board, trashed.Column, trashed.Card))
}

func (s *Server) boardOfColumn(column *Column) *Board {
	for _, board := range s.Boards {
		if slices.Contains(board.Columns, column) {
			return board
		}
	}
	return nil
}

func (s *Server) writeComment(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	// The server stores the comment as rich text; the fake keeps what was sent.
	text := phpTrim(deref(body))
	if strings.IndexFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' }) >= 0 {
		unprocessable(w, "commentHasControlCharacters", "A comment must not contain control characters other than line breaks and tabs.")
		return
	}
	if utf8.RuneCountInString(text) > maxCommentLength {
		unprocessable(w, "commentTooLong", "The comment is too long.")
		return
	}
	if text == "" {
		unprocessable(w, "commentMissing", "A comment needs a body.")
		return
	}

	_, _, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectOfCard(r.PathValue("cardId"))) {
		return
	}
	comment := &Comment{ID: newID(), AuthorID: person.ID, Body: text, CreatedAt: s.now}
	card.subscribe(person.ID)
	card.Comments = append(card.Comments, comment)
	writeJSON(w, http.StatusCreated, s.commentResource(comment))
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
		if b.ID == id && s.visibleProject(b.ProjectID, person) != nil {
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

func (s *Server) cardResource(board *Board, column *Column, card *Card) map[string]any {
	comments := []any{}
	for _, comment := range card.Comments {
		comments = append(comments, s.commentResource(comment))
	}
	project := s.projectOf(board)
	return map[string]any{
		"id": card.ID, "shortId": nullable(card.ShortID), "projectId": board.ProjectID, "boardId": board.ID,
		"project":     map[string]any{"id": project.ID, "name": project.Name},
		"board":       map[string]any{"id": board.ID, "title": board.Title},
		"title":       card.Title,
		"description": card.Description, "descriptionSource": card.Description,
		"column":       map[string]any{"id": column.ID, "type": column.Type, "title": columnTitle(column), "customTitle": column.CustomTitle},
		"commentCount": len(card.Comments), "createdAt": timestamp(card.CreatedAt),
		"creator": s.personReference(card.CreatorID), "completedAt": timestampOrNil(card.CompletedAt),
		"completedBy": s.personReferenceOrNil(card.CompletedByID), "assignees": s.assigneeReferences(card),
		"dueOn": card.DueOn, "onHold": card.OnHold, "steps": stepResources(card), "subscribers": s.subscriberReferences(card),
		"comments": comments,
		"appUrl":   s.cardURL(board, card),
	}
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

func (s *Server) commentResource(comment *Comment) map[string]any {
	return map[string]any{
		"id": comment.ID, "author": s.personReference(comment.AuthorID), "body": comment.Body, "bodySource": comment.Body,
		"reactions": s.reactionResources(comment.Reactions),
		"createdAt": timestamp(comment.CreatedAt), "editedAt": timestampOrNil(comment.EditedAt),
	}
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
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]))
	if mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, errorBody("unsupportedMediaType", "This address takes a body of the type application/json."))
		return nil, false
	}
	content, err := io.ReadAll(r.Body)
	var value any
	if err != nil || json.Unmarshal(content, &value) != nil {
		malformed(w, "The body is not valid JSON.")
		return nil, false
	}
	fields, ok := value.(map[string]any)
	if !ok {
		malformed(w, "The body is not a JSON object.")
		return nil, false
	}
	for name := range fields {
		if strings.HasPrefix(name, "__") {
			malformed(w, "The body carries form arguments of the web.")
			return nil, false
		}
	}
	return fields, true
}

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

func validTitle(w http.ResponseWriter, title string) bool {
	title = phpTrim(title)
	switch {
	case utf8.RuneCountInString(title) > maxTitleLength:
		unprocessable(w, "cardTitleTooLong", "The title of a card is too long.")
	case strings.IndexFunc(title, unicode.IsControl) >= 0:
		unprocessable(w, "cardTitleHasControlCharacters", "The title of a card must not contain control characters.")
	case title == "":
		unprocessable(w, "cardTitleMissing", "A card needs a title.")
	default:
		return true
	}
	return false
}

// validDescription checks the length of what was sent. The server counts the
// sanitized HTML instead, which only differs for markup it removes.
func validDescription(w http.ResponseWriter, description string) bool {
	if utf8.RuneCountInString(phpTrim(description)) > maxDescriptionLength {
		unprocessable(w, "cardDescriptionTooLong", "The description of a card is too long.")
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

func timestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000Z")
}

func timestampOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return timestamp(*t)
}

// validDueDate accepts no date at all, an empty one, or a day that exists,
// written as YYYY-MM-DD. 2026-02-30 and 2026-9-8 are refused.
func validDueDate(w http.ResponseWriter, value *string) bool {
	date := phpTrim(deref(value))
	if date == "" {
		return true
	}
	if strings.IndexFunc(date, unicode.IsControl) < 0 {
		if day, err := time.Parse("2006-01-02", date); err == nil && day.Format("2006-01-02") == date {
			return true
		}
	}
	unprocessable(w, "cardDueDateNotADate", "A due date is a day written as YYYY-MM-DD.")
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
