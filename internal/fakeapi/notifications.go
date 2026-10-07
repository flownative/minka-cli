package fakeapi

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// NotificationsOnAPage is how many lines an answer carries without a limit,
// MostNotificationsOnAPage the most it carries at all, and MostWaitSeconds the
// longest a request is held.
const (
	NotificationsOnAPage     = 50
	MostNotificationsOnAPage = 200
	MostWaitSeconds          = 300
)

// CoordinatorExpiresAfter is how long a coordinator holds a board after its
// last sign of life.
const CoordinatorExpiresAfter = 4 * time.Hour

// maxCoordinatorSession is the longest name of a session a board keeps.
const maxCoordinatorSession = 64

// Notification is one line of the bell of a person.
type Notification struct {
	ID        string
	PersonID  string // whose bell it stands in
	Occasion  string // such as card.moved
	ActorID   string // empty where nobody did it
	Summary   string
	CreatedAt time.Time
	Read      bool

	// CardID names a card the line is about, which it shows where the card
	// stands when the line is read. Anything else is named by its kind, ID and
	// title.
	CardID       string
	SubjectType  string
	SubjectID    string
	SubjectTitle string
	ProjectID    string

	seq int
}

// Coordinator is the session of a person that coordinates a board.
type Coordinator struct {
	PersonID   string
	Session    string
	Since      time.Time
	LastSeenAt time.Time
}

func (c *Coordinator) expiresAt() time.Time {
	return c.LastSeenAt.Add(CoordinatorExpiresAfter)
}

func (s *Server) registerNotificationsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/notifications", s.authenticated(s.notifications))
	mux.HandleFunc("PUT /api/v1/boards/{boardId}/coordinator", s.authenticated(s.claimCoordinator))
	mux.HandleFunc("DELETE /api/v1/boards/{boardId}/coordinator", s.authenticated(s.releaseCoordinator))
}

// Notify puts a line into the bell of its person, as Minka does on an
// occasion, and wakes a request that waits for it. It takes the lock itself,
// so a test may call it while a request is held.
func (s *Server) Notify(n *Notification) *Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notify(n)
}

func (s *Server) notify(n *Notification) *Notification {
	s.notificationSeq++
	n.seq = s.notificationSeq
	if n.ID == "" {
		n.ID = newID()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = s.now
	}
	s.Notifications = append(s.Notifications, n)
	if s.arrival != nil {
		close(s.arrival)
		s.arrival = nil
	}
	return n
}

// CursorOf is the position of a line, the one an answer hands out for it.
func CursorOf(n *Notification) string {
	return cursorOfSeq(n.seq)
}

func cursorOfSeq(seq int) string {
	return "ntf" + strconv.Itoa(seq)
}

var cursorPattern = regexp.MustCompile(`^ntf(0|[1-9][0-9]{0,17})$`)

// knownCursor reads a position this person was handed: the start of a bell
// that never had a line, or the position of one of their own lines.
func (s *Server) knownCursor(cursor string, person *Person) (int, bool) {
	match := cursorPattern.FindStringSubmatch(cursor)
	if match == nil || slices.Contains(s.ForgottenCursors, cursor) {
		return 0, false
	}
	seq, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	if seq == 0 {
		return 0, true
	}
	for _, n := range s.Notifications {
		if n.seq == seq && n.PersonID == person.ID {
			return seq, true
		}
	}
	return 0, false
}

// positionOf is where the bell of a person stands now.
func (s *Server) positionOf(person *Person) string {
	seq := 0
	for _, n := range s.Notifications {
		if n.PersonID == person.ID {
			seq = n.seq
		}
	}
	return cursorOfSeq(seq)
}

func (s *Server) linesAfter(seq int, person *Person, limit int) []*Notification {
	var lines []*Notification
	for _, n := range s.Notifications {
		if n.PersonID == person.ID && n.seq > seq {
			lines = append(lines, n)
		}
	}
	slices.SortFunc(lines, func(a, b *Notification) int { return a.seq - b.seq })
	return lines[:min(len(lines), limit)]
}

var waitOrLimit = regexp.MustCompile(`^[0-9]{1,18}$`)

// notifications answers the lines of the bell behind a position. Without one
// it answers none and where the bell stands. With nothing behind the position
// it holds the request for up to wait seconds and answers as soon as a line
// arrives; a held request is a sign of life of every board its person
// coordinates.
//
// The handler runs with the lock of the server held; it lets go of it while it
// waits, so that a line can arrive.
func (s *Server) notifications(w http.ResponseWriter, r *http.Request, person *Person) {
	query := r.URL.Query()
	wait := 0
	if given := query.Get("wait"); given != "" {
		value, err := strconv.Atoi(given)
		if !waitOrLimit.MatchString(given) || err != nil || value > MostWaitSeconds {
			unprocessable(w, "notificationsWaitInvalid", "The wait is a whole number of seconds from 0 to 300.")
			return
		}
		wait = value
	}
	limit := NotificationsOnAPage
	if given := query.Get("limit"); given != "" {
		value, err := strconv.Atoi(given)
		if !waitOrLimit.MatchString(given) || err != nil || value < 1 || value > MostNotificationsOnAPage {
			unprocessable(w, "notificationsLimitInvalid", "The limit is a whole number from 1 to 200.")
			return
		}
		limit = value
	}

	if !query.Has("after") {
		writeJSON(w, http.StatusOK, s.notificationsPage(nil, s.positionOf(person), person))
		return
	}
	after := query.Get("after")
	seq, known := s.knownCursor(after, person)
	if !known {
		unprocessable(w, "cursorUnknown", "The position to read on from is not one this list handed out.")
		return
	}

	if wait > 0 {
		s.seenAlive(person)
	}

	lines := s.linesAfter(seq, person, limit)
	if len(lines) > 0 || wait == 0 {
		s.answerLines(w, lines, after, person)
		return
	}

	waitsOut := s.WaitsOut
	if waitsOut == nil {
		waitsOut = time.After
	}
	whileWaiting := s.WhileWaiting
	timer := waitsOut(time.Duration(wait) * time.Second)
	for first := true; ; first = false {
		if s.arrival == nil {
			s.arrival = make(chan struct{})
		}
		arrival := s.arrival
		s.mu.Unlock()
		if first && whileWaiting != nil {
			whileWaiting(person)
		}
		ended := false
		select {
		case <-arrival:
		case <-timer:
			ended = true
		case <-r.Context().Done():
			ended = true
		}
		s.mu.Lock()
		lines = s.linesAfter(seq, person, limit)
		if len(lines) > 0 || ended {
			s.answerLines(w, lines, after, person)
			return
		}
	}
}

func (s *Server) answerLines(w http.ResponseWriter, lines []*Notification, after string, person *Person) {
	cursor := after
	if len(lines) > 0 {
		cursor = CursorOf(lines[len(lines)-1])
	}
	writeJSON(w, http.StatusOK, s.notificationsPage(lines, cursor, person))
}

func (s *Server) notificationsPage(lines []*Notification, cursor string, person *Person) map[string]any {
	data := []any{}
	for _, n := range lines {
		data = append(data, s.notificationResource(n, person))
	}
	return map[string]any{"notifications": data, "cursor": cursor}
}

func (s *Server) notificationResource(n *Notification, person *Person) map[string]any {
	var actor any
	if a := s.People[n.ActorID]; a != nil {
		actor = map[string]any{"id": a.ID, "name": a.Name}
	}
	var project any
	if p := s.projectWithID(n.ProjectID); p != nil {
		project = map[string]any{"id": p.ID, "name": p.Name}
	}

	subject := map[string]any{"type": n.SubjectType, "id": n.SubjectID, "shortId": nil, "title": n.SubjectTitle, "url": "", "appUrl": ""}
	var card any
	if n.CardID != "" {
		subject["type"], subject["id"] = "card", n.CardID
		subject["url"] = s.URL + "/api/v1/cards/" + n.CardID
		if board, column, c := s.visibleCard(n.CardID, person); c != nil {
			subject["shortId"], subject["title"], subject["appUrl"] = nullable(c.ShortID), c.Title, s.cardURL(board, c)
			card = map[string]any{"boardId": board.ID, "column": map[string]any{"id": column.ID, "title": columnTitle(column)}}
			if project == nil {
				p := s.projectOf(board)
				project = map[string]any{"id": p.ID, "name": p.Name}
			}
		}
	}

	return map[string]any{
		"id": n.ID, "cursor": CursorOf(n), "occasion": n.Occasion, "createdAt": timestamp(n.CreatedAt), "read": n.Read,
		"actor": actor, "summary": n.Summary, "subject": subject, "card": card, "project": project,
	}
}

// noteTheMove tells every assignee of a card but the mover that it changed
// column.
func (s *Server) noteTheMove(board *Board, card *Card, to *Column, mover *Person) {
	name := card.ShortID
	if name == "" {
		name = card.Title
	}
	for _, assigneeID := range card.AssigneeIDs {
		if assigneeID == mover.ID {
			continue
		}
		s.notify(&Notification{
			PersonID: assigneeID, Occasion: "card.moved", ActorID: mover.ID, CardID: card.ID, ProjectID: board.ProjectID,
			Summary: fmt.Sprintf("%s moved %s to “%s”", mover.Name, name, columnTitle(to)),
		})
	}
}

// coordinatorOf is the coordinator of a board while it holds it.
func (s *Server) coordinatorOf(board *Board) *Coordinator {
	if board.Coordinator == nil || !s.now.Before(board.Coordinator.expiresAt()) {
		return nil
	}
	return board.Coordinator
}

func (s *Server) coordinatorResource(board *Board) any {
	c := s.coordinatorOf(board)
	if c == nil {
		return nil
	}
	holder := s.People[c.PersonID]
	return map[string]any{
		"person":  map[string]any{"id": holder.ID, "name": holder.Name, "kind": kindOf(holder)},
		"session": c.Session, "since": timestamp(c.Since), "lastSeenAt": timestamp(c.LastSeenAt), "expiresAt": timestamp(c.expiresAt()),
	}
}

// seenAlive renews every board a person coordinates.
func (s *Server) seenAlive(person *Person) {
	for _, board := range s.Boards {
		if c := s.coordinatorOf(board); c != nil && c.PersonID == person.ID {
			c.LastSeenAt = s.now
		}
	}
}

// boardToCoordinate is a board the person may edit: a member of its project
// who is no client. Anybody else is told that there is no such board.
func (s *Server) boardToCoordinate(r *http.Request, person *Person) *Board {
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil || s.isAClient(person.ID) {
		return nil
	}
	return board
}

// claimCoordinator makes a session of the person the coordinator of a board,
// or renews it. Another session of the same person takes it over; another
// person's that has not expired is refused with the holder.
func (s *Server) claimCoordinator(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.boardToCoordinate(r, person)
	if board == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	session, ok := stringField(w, fields, "session")
	if !ok {
		return
	}
	if session == nil || strings.TrimSpace(*session) == "" || utf8.RuneCountInString(*session) > maxCoordinatorSession {
		unprocessable(w, "coordinatorSessionInvalid", "The session is named by 1 to 64 characters.")
		return
	}

	holder := s.coordinatorOf(board)
	if holder != nil && holder.PersonID != person.ID {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":       map[string]string{"code": "coordinatorTaken", "message": "Somebody else coordinates this board."},
			"coordinator": s.coordinatorResource(board),
		})
		return
	}
	if holder == nil || holder.Session != *session {
		board.Coordinator = &Coordinator{PersonID: person.ID, Session: *session, Since: s.now, LastSeenAt: s.now}
	} else {
		holder.LastSeenAt = s.now
	}
	writeJSON(w, http.StatusOK, s.coordinatorResource(board))
}

// releaseCoordinator takes the coordinator off a board, whoever holds it. A
// board nobody coordinates answers the same.
func (s *Server) releaseCoordinator(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.boardToCoordinate(r, person)
	if board == nil {
		notFound(w)
		return
	}
	board.Coordinator = nil
	w.WriteHeader(http.StatusNoContent)
}

// SetNow moves the moment the fake takes for now.
func (s *Server) SetNow(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}
