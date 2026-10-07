package fakeapi

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// NotificationsOnAPage is how many lines an answer carries without a limit,
// MostNotificationsOnAPage the most it carries at all, MostWaitSeconds the
// longest wait it takes and WaitAtMost the longest it holds a request, which
// is Minka's setting notifications.waitAtMost.
const (
	NotificationsOnAPage     = 50
	MostNotificationsOnAPage = 200
	MostWaitSeconds          = 300
	WaitAtMost               = 50 * time.Second
)

// waitAtMost is how long this server holds a request.
func (s *Server) waitAtMost() time.Duration {
	if s.HoldsAtMost > 0 {
		return s.HoldsAtMost
	}
	return WaitAtMost
}

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
	if s.notificationSeqs == nil {
		s.notificationSeqs = map[string]int{}
	}
	s.notificationSeqs[n.PersonID]++
	n.seq = s.notificationSeqs[n.PersonID]
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

// CursorOf is the position of a line, the one an answer hands out for it:
// its number in the sequence of its person.
func CursorOf(n *Notification) string {
	return strconv.Itoa(n.seq)
}

// listNumber is a number as the list reads one, without a sign or a leading
// zero.
var listNumber = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})$`)

// wholeNumberIn reads a whole number between two bounds, or says it is none.
func wholeNumberIn(value string, lowest, highest int) (int, bool) {
	if !listNumber.MatchString(value) {
		return 0, false
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < lowest || number > highest {
		return 0, false
	}
	return number, true
}

// positionOf is where the bell of a person stands now: the number of their
// last line, 0 before the first.
func (s *Server) positionOf(person *Person) int {
	return s.notificationSeqs[person.ID]
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

// notifications answers the lines of the bell behind a position, in the
// order of the controller: the wait, the limit, the sign of life of a wait of
// more than nothing, then the position. Without one it answers none and where
// the bell stands. A position is any whole number up to where the bell stands.
// With nothing behind it the request is held for the wait, at most WaitAtMost,
// and answered as soon as a line arrives.
//
// The handler runs with the lock of the server held; it lets go of it while it
// waits, so that a line can arrive.
func (s *Server) notifications(w http.ResponseWriter, r *http.Request, person *Person) {
	query := r.URL.Query()
	wait := 0
	if query.Has("wait") {
		value, ok := wholeNumberIn(query.Get("wait"), 0, MostWaitSeconds)
		if !ok {
			unprocessable(w, "notificationsWaitInvalid", `"wait" is a whole number of seconds from 0 to 300.`)
			return
		}
		wait = value
	}
	limit := NotificationsOnAPage
	if given := query.Get("limit"); given != "" {
		value, ok := wholeNumberIn(given, 1, MostNotificationsOnAPage)
		if !ok {
			unprocessable(w, "notificationsLimitInvalid", `"limit" is a whole number from 1 to 200.`)
			return
		}
		limit = value
	}
	if wait > 0 {
		s.seenAlive(person)
	}

	position := s.positionOf(person)
	after := query.Get("after")
	if after == "" {
		writeJSON(w, http.StatusOK, s.notificationsPage(nil, strconv.Itoa(position), person))
		return
	}
	seq, known := wholeNumberIn(after, 0, position)
	if !known || slices.Contains(s.ForgottenCursors, after) {
		unprocessable(w, "cursorUnknown", `The cursor is not one this list handed out. Start again without "after".`)
		return
	}

	lines := s.linesAfter(seq, person, limit)
	if len(lines) > 0 || wait == 0 {
		s.answerLines(w, lines, after, person)
		return
	}

	held := time.Duration(wait) * time.Second
	if most := s.waitAtMost(); held > most {
		held = most
	}
	waitsOut := s.WaitsOut
	if waitsOut == nil {
		waitsOut = time.After
	}
	whileWaiting := s.WhileWaiting
	timer := waitsOut(held)
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
// column. A move into Done is told as the completion alone, to the
// subscribers of the card but the mover.
func (s *Server) noteTheMove(board *Board, card *Card, to *Column, mover *Person) {
	name := card.ShortID
	if name == "" {
		name = "“" + card.Title + "”"
	}
	occasion, recipients, summary := "card.moved", card.AssigneeIDs, fmt.Sprintf("%s moved %s to “%s”", mover.Name, name, columnTitle(to))
	if to.Type == "done" {
		occasion, recipients, summary = "card.completed", card.SubscriberIDs, "Completed: "+card.Title
	}
	for _, personID := range recipients {
		if personID == mover.ID {
			continue
		}
		s.notify(&Notification{
			PersonID: personID, Occasion: occasion, ActorID: mover.ID, CardID: card.ID, ProjectID: board.ProjectID, Summary: summary,
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
// or renews it, in the order of the controller: the board, a client, the
// session, then the claim. Another session of the same person takes it over
// and since starts anew; another person's entry that has not expired is
// refused with the holder; and only then is an archived project refused,
// where nothing can be written.
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
	if session == nil || !isASessionName(*session) {
		unprocessable(w, "coordinatorSessionInvalid", "A session is a readable name of 1 to 64 characters without control characters.")
		return
	}

	holder := s.coordinatorOf(board)
	if holder != nil && holder.PersonID != person.ID {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":       map[string]string{"code": "coordinatorTaken", "message": "Somebody else coordinates this board. Nothing was written; \"coordinator\" names them."},
			"coordinator": s.coordinatorResource(board),
		})
		return
	}
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return
	}
	if holder == nil || holder.Session != *session {
		board.Coordinator = &Coordinator{PersonID: person.ID, Session: *session, Since: s.now, LastSeenAt: s.now}
	} else {
		holder.LastSeenAt = s.now
	}
	writeJSON(w, http.StatusOK, s.coordinatorResource(board))
}

// isASessionName is what a board keeps as the name of a session: 1 to 64
// characters, none of them a control or a format character. Blanks are names
// as well.
func isASessionName(session string) bool {
	if session == "" || utf8.RuneCountInString(session) > maxCoordinatorSession {
		return false
	}
	return !strings.ContainsFunc(session, func(r rune) bool { return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) })
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
