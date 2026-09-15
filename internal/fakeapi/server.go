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
}

type Project struct {
	ID          string
	Name        string
	Description string
	MemberIDs   []string
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
	Cards       []*Card
}

type Card struct {
	ID            string
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
	ProjectID string
	Name      string
	Messages  []*ChatMessage // in the order of the transcript
	ReadMarks map[string]int // person ID → the seq read up to

	lastChangeSeq int
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
}

func (c *Chat) post(authorID, text, clientMessageID string, now time.Time) *ChatMessage {
	c.lastChangeSeq++
	message := &ChatMessage{
		ID: newID(), Seq: len(c.Messages) + 1, ChangeSeq: c.lastChangeSeq, AuthorID: authorID,
		BodyText: chatText(text), BodyHTML: chatHTML(text), ClientMessageID: clientMessageID, CreatedAt: now,
	}
	c.Messages = append(c.Messages, message)
	return message
}

func (c *Chat) unread(personID string) int {
	unread := 0
	for _, message := range c.Messages {
		if message.Seq > c.ReadMarks[personID] {
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
	for _, p := range []*Person{bruno, anke, stranger, agent} {
		s.People[p.ID] = p
	}
	s.Tokens["minka_pat_bruno"] = bruno.ID
	s.Tokens["minka_pat_stranger"] = stranger.ID
	s.Tokens["minka_pat_agent"] = agent.ID

	project := &Project{ID: newID(), Name: "Test-Projekt", Description: "For trying things", MemberIDs: []string{bruno.ID, anke.ID, agent.ID}}
	triage := &Column{ID: newID(), Type: "triage"}
	doingTitle := "Doing"
	doing := &Column{ID: newID(), Type: "column", CustomTitle: &doingTitle}
	notNow := &Column{ID: newID(), Type: "not_now"}
	done := &Column{ID: newID(), Type: "done"}
	board := &Board{ID: newID(), ProjectID: project.ID, Title: "API-3 Abnahme", Columns: []*Column{triage, doing, notNow, done}}

	cardInTriage := &Card{ID: newID(), Title: "Write the CLI", Description: "<p>With <strong>Go</strong></p>", CreatedAt: s.now, CreatorID: bruno.ID,
		Comments: []*Comment{{ID: newID(), AuthorID: bruno.ID, Body: "First comment", CreatedAt: s.now}}}
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
	chat.post(anke.ID, "Zelt ist bestellt", "", s.now)
	chat.post(bruno.ID, "Danke", "", s.now)
	otherChat := &Chat{ID: newID(), ProjectID: otherProject.ID, Name: "Secret chat", ReadMarks: map[string]int{}}
	otherChat.post(stranger.ID, "Psst", "", s.now)
	s.Chats = []*Chat{chat, otherChat}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", s.authenticated(s.me))
	mux.HandleFunc("GET /api/v1/projects", s.authenticated(s.projects))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/boards", s.authenticated(s.boards))
	mux.HandleFunc("GET /api/v1/boards/{boardId}", s.authenticated(s.board))
	mux.HandleFunc("GET /api/v1/cards/{cardId}", s.authenticated(s.card))
	mux.HandleFunc("POST /api/v1/boards/{boardId}/cards", s.authenticated(s.addCard))
	mux.HandleFunc("PATCH /api/v1/cards/{cardId}", s.authenticated(s.updateCard))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/moves", s.authenticated(s.moveCard))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/comments", s.authenticated(s.writeComment))
	mux.HandleFunc("PATCH /api/v1/cards/{cardId}/comments/{commentId}", s.authenticated(s.editComment))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/comments/{commentId}", s.authenticated(s.removeComment))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}", s.authenticated(s.removeCard))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/people", s.authenticated(s.people))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/assignees", s.authenticated(s.assignCard))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/assignees/{personId}", s.authenticated(s.unassignCard))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/steps", s.authenticated(s.addStep))
	mux.HandleFunc("PATCH /api/v1/cards/{cardId}/steps/{stepId}", s.authenticated(s.updateStep))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/steps/{stepId}", s.authenticated(s.removeStep))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/subscribers/{personId}", s.authenticated(s.subscribeToCard))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/subscribers/{personId}", s.authenticated(s.unsubscribeFromCard))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/chats", s.authenticated(s.chats))
	mux.HandleFunc("GET /api/v1/chats/{chatId}/messages", s.authenticated(s.messages))
	mux.HandleFunc("POST /api/v1/chats/{chatId}/messages", s.authenticated(s.postMessage))
	mux.HandleFunc("PATCH /api/v1/chats/{chatId}/messages/{messageId}", s.authenticated(s.editMessage))
	mux.HandleFunc("DELETE /api/v1/chats/{chatId}/messages/{messageId}", s.authenticated(s.removeMessage))
	mux.HandleFunc("POST /api/v1/chats/{chatId}/read", s.authenticated(s.markChatRead))
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
		Bruno: bruno, Anke: anke, Stranger: stranger, Agent: agent, Project: project, Board: board,
		Triage: triage, Doing: doing, NotNow: notNow, Done: done,
		CardInTriage: cardInTriage, CardInDoing: cardInDoing,
		OtherProject: otherProject, OtherBoard: otherBoard, OtherCard: otherCard,
		Chat: chat, OtherChat: otherChat,
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
	var principal any
	if person.PrincipalID != "" {
		p := s.People[person.PrincipalID]
		principal = map[string]any{"id": p.ID, "name": p.Name, "emailAddress": p.Email}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": person.ID, "name": person.Name, "emailAddress": person.Email,
		"organizationRole": "member", "principal": principal,
	})
}

func (s *Server) projects(w http.ResponseWriter, _ *http.Request, person *Person) {
	list := []any{}
	for _, p := range s.Projects {
		if slices.Contains(p.MemberIDs, person.ID) {
			list = append(list, map[string]any{"id": p.ID, "name": p.Name, "description": p.Description, "appUrl": s.URL + "/projects/" + p.ID})
		}
	}
	writeJSON(w, http.StatusOK, list)
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
		cards := []any{}
		listed := column.Cards
		if column.Type == "done" {
			// Done is ordered by completion, the card completed last first.
			listed = slices.Clone(listed)
			slices.SortStableFunc(listed, func(a, b *Card) int { return completion(b).Compare(completion(a)) })
			if len(listed) > 25 {
				listed = listed[:25]
			}
		}
		for _, card := range listed {
			cards = append(cards, map[string]any{
				"id": card.ID, "title": card.Title, "hasDescription": card.Description != "",
				"stepCount": len(card.Steps), "completedStepCount": completedStepCount(card),
				"commentCount": len(card.Comments), "createdAt": timestamp(card.CreatedAt),
				"creator": s.personReference(card.CreatorID), "completedAt": timestampOrNil(card.CompletedAt),
				"completedBy": s.personReferenceOrNil(card.CompletedByID), "assignees": s.assigneeReferences(card),
				"dueOn": card.DueOn, "appUrl": s.cardURL(board, card),
			})
		}
		columns = append(columns, map[string]any{
			"id": column.ID, "type": column.Type, "title": columnTitle(column), "customTitle": column.CustomTitle,
			"cardCount": len(column.Cards), "cards": cards,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": board.ID, "projectId": board.ProjectID, "title": board.Title, "appUrl": s.boardURL(board), "columns": columns,
	})
}

func (s *Server) card(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column, card := s.visibleCard(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card))
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
	if !ok1 || !ok2 || !ok3 || !ok4 {
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
	var to *Column
	for _, c := range board.Columns {
		if c.ID == *columnID {
			to = c
		}
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
	index := 0
	switch {
	case position != nil && *position == "bottom":
		index = len(to.Cards)
	case previousID != nil && indexOf(to.Cards, *previousID) >= 0:
		index = indexOf(to.Cards, *previousID) + 1
	case nextID != nil && indexOf(to.Cards, *nextID) >= 0:
		index = indexOf(to.Cards, *nextID)
	}
	to.Cards = slices.Insert(to.Cards, index, card)

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

func (s *Server) people(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	list := []any{}
	for _, id := range project.MemberIDs {
		member := s.People[id]
		list = append(list, map[string]any{"id": member.ID, "name": member.Name, "emailAddress": member.Email})
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
		if chat.ProjectID == project.ID {
			list = append(list, map[string]any{
				"id": chat.ID, "name": chat.Name, "unreadCount": chat.unread(person.ID),
				"appUrl": s.URL + "/projects/" + project.ID + "/chats/" + chat.ID,
			})
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) visibleChat(id string, person *Person) *Chat {
	for _, chat := range s.Chats {
		if chat.ID == id && s.visibleProject(chat.ProjectID, person) != nil {
			return chat
		}
	}
	return nil
}

// messages pages the way the server does: before a seq, after a changeSeq,
// or the end of the transcript, always oldest first.
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

	var page []*ChatMessage
	switch {
	case query.Get("before") != "":
		before, err := strconv.Atoi(query.Get("before"))
		if !messageNumber.MatchString(query.Get("before")) || err != nil {
			notFound(w)
			return
		}
		for _, message := range chat.Messages {
			if message.Seq < before {
				page = append(page, message)
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
		slices.SortFunc(page, func(a, b *ChatMessage) int { return a.Seq - b.Seq })
	default:
		page = last(chat.Messages)
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
	message := authoredMessage(chat, r.PathValue("messageId"), person)
	if message == nil || message.Removed {
		notFound(w)
		return
	}
	if message.BodyHTML != chatHTML(text) {
		now := s.now
		chat.lastChangeSeq++
		message.BodyText, message.BodyHTML, message.EditedAt, message.ChangeSeq = chatText(text), chatHTML(text), &now, chat.lastChangeSeq
	}
	writeJSON(w, http.StatusOK, s.chatMessageResource(message))
}

func (s *Server) removeMessage(w http.ResponseWriter, r *http.Request, person *Person) {
	chat := s.visibleChat(r.PathValue("chatId"), person)
	if chat == nil {
		notFound(w)
		return
	}
	message := authoredMessage(chat, r.PathValue("messageId"), person)
	if message == nil {
		notFound(w)
		return
	}
	if !message.Removed {
		chat.lastChangeSeq++
		message.Removed, message.BodyText, message.BodyHTML, message.ChangeSeq = true, "", "", chat.lastChangeSeq
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// markChatRead keeps the highest mark and never one beyond the last message.
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
	seq := min(int(value), len(chat.Messages))
	if seq > chat.ReadMarks[person.ID] {
		chat.ReadMarks[person.ID] = seq
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) chatMessageResource(message *ChatMessage) map[string]any {
	return map[string]any{
		"id": message.ID, "seq": message.Seq, "changeSeq": message.ChangeSeq,
		"author": s.personReference(message.AuthorID), "bodyHtml": message.BodyHTML, "bodyText": message.BodyText, "bodySource": message.BodyHTML,
		"createdAt": timestamp(message.CreatedAt), "editedAt": timestampOrNil(message.EditedAt), "removed": message.Removed,
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
	column.Cards = slices.DeleteFunc(column.Cards, func(c *Card) bool { return c == card })

	// The server sends its JSON Content-Type along with the empty 204.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
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
	return map[string]any{
		"id": card.ID, "projectId": board.ProjectID, "boardId": board.ID, "title": card.Title,
		"description": card.Description, "descriptionSource": card.Description,
		"column":       map[string]any{"id": column.ID, "type": column.Type, "title": columnTitle(column), "customTitle": column.CustomTitle},
		"commentCount": len(card.Comments), "createdAt": timestamp(card.CreatedAt),
		"creator": s.personReference(card.CreatorID), "completedAt": timestampOrNil(card.CompletedAt),
		"completedBy": s.personReferenceOrNil(card.CompletedByID), "assignees": s.assigneeReferences(card),
		"dueOn": card.DueOn, "steps": stepResources(card), "subscribers": s.subscriberReferences(card),
		"comments": comments,
		"appUrl":   s.cardURL(board, card),
	}
}

func (s *Server) commentResource(comment *Comment) map[string]any {
	return map[string]any{
		"id": comment.ID, "author": s.personReference(comment.AuthorID), "body": comment.Body, "bodySource": comment.Body,
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
