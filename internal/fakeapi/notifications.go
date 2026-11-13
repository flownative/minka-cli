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

	// MovedToColumnID is the column a card.moved line tells of, which its
	// summary names as it is called when the line is read. Where the column
	// is gone the summary is the bell's own sentence; where it is empty the
	// line says Summary.
	MovedToColumnID string

	seq int
}

// notificationKinds are the occasions Minka knows. A row of any other, which a
// version rolled back may leave behind, makes no line.
var notificationKinds = []string{
	"card.assigned", "card.completed", "card.dueDateSet", "card.dueDateCleared", "card.moved", "card.due",
	"step.assigned", "steps.due", "todo.assigned", "todo.completed", "todo.added", "todos.due",
	"comment.written", "post.added", "checkinAnswer.added", "checkinQuestion.asked",
	"calendarEvent.participantAdded", "calendarEvent.moved", "calendarEvent.dayMoved", "calendarEvent.dayCancelled",
	"calendarEvent.startsSoon", "reminder.due", "person.mentioned", "person.mentionedPrivately",
	"board.conventionsChanged", "meeting.closed",
}

// reactionKind is the occasion of a reaction, which the bell rolls up into one
// line of its own and neither list nor lookup shows.
const reactionKind = "reaction.added"

// timeSensitiveKinds name the person or remind them: a push of one of them
// rings loudly.
var timeSensitiveKinds = []string{
	"person.mentioned", "person.mentionedPrivately", "card.assigned", "step.assigned", "todo.assigned",
	"calendarEvent.participantAdded", "calendarEvent.moved", "calendarEvent.dayMoved", "calendarEvent.dayCancelled",
	"calendarEvent.startsSoon", "todos.due", "card.due", "steps.due", "checkinQuestion.asked", "reminder.due",
}

// Coordinator is the session of a person that coordinates a board.
type Coordinator struct {
	Session    string
	Since      time.Time
	LastSeenAt time.Time
}

func (c *Coordinator) expiresAt() time.Time {
	return c.LastSeenAt.Add(CoordinatorExpiresAfter)
}

func (s *Server) registerNotificationsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/notifications", s.authenticated(s.notifications))
	mux.HandleFunc("GET /api/v1/me/notifications/count", s.authenticated(s.notificationsCount))
	mux.HandleFunc("GET /api/v1/notifications/{notificationId}", s.authenticated(s.oneNotification))
	mux.HandleFunc("POST /api/v1/notifications/{notificationId}/read", s.authenticated(s.markNotificationRead))
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

// isShown says whether a row of the bell is one the person may still see:
// in a project the person is a member of and that is in use, about a card
// that still stands on a board or in a column in the trash. What it hides is
// left out before a page is counted.
func (s *Server) isShown(n *Notification, person *Person) bool {
	if n.ProjectID != "" {
		project := s.projectWithID(n.ProjectID)
		if project == nil || project.Archived || !slices.Contains(project.MemberIDs, person.ID) {
			return false
		}
	}
	if n.CardID != "" {
		board, _, card := s.cardOfTheBell(n.CardID, person)
		if card == nil || s.projectOf(board).Archived {
			return false
		}
	}
	return true
}

// cardOfTheBell finds a card the way the bell does: on its board, or in a
// column in the trash, which keeps its cards on it and whose status the bell
// does not ask. A card in the trash by itself or in the archive is not found.
func (s *Server) cardOfTheBell(id string, person *Person) (*Board, *Column, *Card) {
	if board, column, card := s.visibleCard(id, person); card != nil {
		return board, column, card
	}
	for _, trashed := range s.TrashedColumns {
		if s.visibleProject(trashed.Board.ProjectID, person) == nil {
			continue
		}
		for _, card := range trashed.Column.Cards {
			if card.ID == id {
				return trashed.Board, trashed.Column, card
			}
		}
	}
	return nil, nil, nil
}

// makesALine says whether a row is shown as a line of its own: one of an
// occasion Minka knows.
func makesALine(n *Notification) bool {
	return slices.Contains(notificationKinds, n.Occasion)
}

// linesAfter reads up to limit rows of the person behind a position, the
// oldest first, as Minka does: rows the person may no longer see, and
// reactions, are left out before the limit counts; a row of an occasion Minka
// does not know counts toward it and makes no line. readUpTo is the number of
// the last row read, the position asked from where there was none.
func (s *Server) linesAfter(seq int, person *Person, limit int) (lines []*Notification, readUpTo int) {
	var rows []*Notification
	for _, n := range s.Notifications {
		if n.PersonID == person.ID && n.seq > seq && n.Occasion != reactionKind && s.isShown(n, person) {
			rows = append(rows, n)
		}
	}
	slices.SortFunc(rows, func(a, b *Notification) int { return a.seq - b.seq })
	readUpTo = seq
	for _, n := range rows[:min(len(rows), limit)] {
		readUpTo = n.seq
		if makesALine(n) {
			lines = append(lines, n)
		}
	}
	return lines, readUpTo
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

	lines, readUpTo := s.linesAfter(seq, person, limit)
	if len(lines) > 0 || wait == 0 {
		s.answerLines(w, lines, readUpTo, person)
		return
	}
	// Rows that made no line are read: the wait goes on behind them.
	seq = readUpTo

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
		lines, seq = s.linesAfter(seq, person, limit)
		if len(lines) > 0 || ended {
			s.answerLines(w, lines, seq, person)
			return
		}
	}
}

// answerLines answers the lines read and the position of the last row read,
// which lies behind the last line where rows that made none followed it.
func (s *Server) answerLines(w http.ResponseWriter, lines []*Notification, readUpTo int, person *Person) {
	writeJSON(w, http.StatusOK, s.notificationsPage(lines, strconv.Itoa(readUpTo), person))
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
	// A line about the conventions leads to their page, in both addresses.
	if n.Occasion == "board.conventionsChanged" {
		if board := s.visibleBoard(n.SubjectID, person); board != nil {
			subject["url"], subject["appUrl"] = s.boardURL(board)+"/conventions", s.boardURL(board)+"/conventions"
		}
	}
	shortID := ""
	var card any
	if n.CardID != "" {
		subject["type"], subject["id"] = "card", n.CardID
		subject["url"] = s.URL + "/api/v1/cards/" + n.CardID
		if board, column, c := s.cardOfTheBell(n.CardID, person); c != nil {
			subject["shortId"], subject["title"], subject["appUrl"] = nullable(c.ShortID), c.Title, s.cardURL(board, c)
			shortID = c.ShortID
			card = map[string]any{"boardId": board.ID, "column": map[string]any{"id": column.ID, "title": columnTitle(column)}}
			if project == nil {
				p := s.projectOf(board)
				project = map[string]any{"id": p.ID, "name": p.Name}
			}
		}
	}
	s.withShortURL(shortID, subject)

	return map[string]any{
		"id": n.ID, "cursor": CursorOf(n), "occasion": n.Occasion, "createdAt": timestamp(n.CreatedAt), "read": n.Read,
		"actor": actor, "summary": s.summaryOf(n, person), "subject": subject, "card": card, "project": project,
	}
}

// summaryOf is the summary of a line as it is read: a card.moved line that
// names its column says who moved the card where, with the names they have
// now. Where it names none, or that column is gone for good and not only in
// the trash, it says the bell's own sentence, as the push does.
func (s *Server) summaryOf(n *Notification, person *Person) string {
	if n.Occasion != "card.moved" {
		return n.Summary
	}
	_, _, card := s.cardOfTheBell(n.CardID, person)
	column := s.columnEvenInTheTrash(n.MovedToColumnID)
	if card == nil || column == nil {
		return s.pushTitleOf(n, person)
	}
	name := card.ShortID
	if name == "" {
		name = "“" + card.Title + "”"
	}
	mover := ""
	if actor := s.People[n.ActorID]; actor != nil {
		mover = actor.Name
	}
	return fmt.Sprintf("%s moved %s to “%s”", mover, name, columnTitle(column))
}

// columnEvenInTheTrash finds a column on a board or in the trash, where Minka
// keeps it until its board is purged.
func (s *Server) columnEvenInTheTrash(id string) *Column {
	for _, board := range s.Boards {
		for _, column := range board.Columns {
			if column.ID == id {
				return column
			}
		}
	}
	for _, trashed := range s.TrashedColumns {
		if trashed.Column.ID == id {
			return trashed.Column
		}
	}
	return nil
}

// pushTitleOf is the sentence a push of the line says: the bell's own for a
// move and a completion, about the card as it is called now, and the summary
// for everything else.
func (s *Server) pushTitleOf(n *Notification, person *Person) string {
	title := n.SubjectTitle
	if _, _, card := s.cardOfTheBell(n.CardID, person); card != nil {
		title = card.Title
	}
	switch n.Occasion {
	case "card.moved":
		return "Moved to another column: " + title
	case "card.completed":
		return "Completed: " + title
	}
	return n.Summary
}

// unreadCount is the number on the bell: every unread row the person may
// still see, those of an occasion that makes no line and reactions included.
func (s *Server) unreadCount(person *Person) int {
	count := 0
	for _, n := range s.Notifications {
		if n.PersonID == person.ID && !n.Read && s.isShown(n, person) {
			count++
		}
	}
	return count
}

// notificationOf finds a line of the person's bell by its ID, as the bell
// shows it: one of somebody else, one the person may no longer see, one that
// makes no line and a reaction are all not there.
func (s *Server) notificationOf(r *http.Request, person *Person) *Notification {
	id := r.PathValue("notificationId")
	for _, n := range s.Notifications {
		if n.ID == id && n.PersonID == person.ID && makesALine(n) && s.isShown(n, person) {
			return n
		}
	}
	return nil
}

func (s *Server) notificationsCount(w http.ResponseWriter, _ *http.Request, person *Person) {
	writeJSON(w, http.StatusOK, map[string]any{"unreadCount": s.unreadCount(person)})
}

// oneNotification answers a line as its push named it, for the tap on that
// push.
func (s *Server) oneNotification(w http.ResponseWriter, r *http.Request, person *Person) {
	n := s.notificationOf(r, person)
	if n == nil {
		notFound(w)
		return
	}
	// The body names the project of the row, which for a card is the card's.
	project := s.projectWithID(n.ProjectID)
	appURL, thread := "", n.SubjectID
	if n.CardID != "" {
		thread = n.CardID
		if board, _, card := s.cardOfTheBell(n.CardID, person); card != nil {
			appURL = s.cardURL(board, card)
			if project == nil {
				project = s.projectOf(board)
			}
		}
	}
	body := ""
	if project != nil {
		body = project.Name
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": n.ID, "title": s.pushTitleOf(n, person), "body": body, "appUrl": appURL, "createdAt": timestamp(n.CreatedAt),
		"read": n.Read, "threadId": thread, "timeSensitive": slices.Contains(timeSensitiveKinds, n.Occasion),
		"unreadCount": s.unreadCount(person),
	})
}

// markNotificationRead marks a line read; a second time changes nothing. It
// answers the number on the bell.
func (s *Server) markNotificationRead(w http.ResponseWriter, r *http.Request, person *Person) {
	n := s.notificationOf(r, person)
	if n == nil {
		notFound(w)
		return
	}
	n.Read = true
	writeJSON(w, http.StatusOK, map[string]any{"unreadCount": s.unreadCount(person)})
}

// noteTheMove tells every assignee of a card but the mover that it changed
// column. A move into Done is told as the completion alone, to the
// subscribers of the card but the mover.
func (s *Server) noteTheMove(board *Board, card *Card, to *Column, mover *Person) {
	name := card.ShortID
	if name == "" {
		name = "“" + card.Title + "”"
	}
	occasion, recipients, summary, movedTo := "card.moved", card.AssigneeIDs, fmt.Sprintf("%s moved %s to “%s”", mover.Name, name, columnTitle(to)), to.ID
	if to.Type == "done" {
		occasion, recipients, summary, movedTo = "card.completed", card.SubscriberIDs, "Completed: "+card.Title, ""
	}
	for _, personID := range recipients {
		if personID == mover.ID {
			continue
		}
		s.notify(&Notification{
			PersonID: personID, Occasion: occasion, ActorID: mover.ID, CardID: card.ID, ProjectID: board.ProjectID, Summary: summary,
			MovedToColumnID: movedTo,
		})
	}
}

// coordinatorOf is the person's own entry on a board while it holds.
func (s *Server) coordinatorOf(board *Board, person *Person) *Coordinator {
	c := board.Coordinators[person.ID]
	if c == nil || !s.now.Before(c.expiresAt()) {
		return nil
	}
	return c
}

func (s *Server) coordinatorResource(board *Board, person *Person) any {
	c := s.coordinatorOf(board, person)
	if c == nil {
		return nil
	}
	return map[string]any{
		"person":  map[string]any{"id": person.ID, "name": person.Name, "kind": kindOf(person)},
		"session": c.Session, "since": timestamp(c.Since), "lastSeenAt": timestamp(c.LastSeenAt), "expiresAt": timestamp(c.expiresAt()),
	}
}

// seenAlive renews every entry a person still holds on a board they still
// see as one of the team.
func (s *Server) seenAlive(person *Person) {
	for _, board := range s.Boards {
		if c := s.coordinatorOf(board, person); c != nil && s.seesOnTheTeam(board, person) {
			c.LastSeenAt = s.now
		}
	}
}

// seesOnTheTeam says whether a person sees a board as one of the team: a
// member of its project, archived or not, who is no client.
func (s *Server) seesOnTheTeam(board *Board, person *Person) bool {
	return s.visibleBoard(board.ID, person) != nil && !s.isAClient(person.ID)
}

// boardToCoordinate is a board the person may edit: a member of its project
// who is no client. Anybody else is told that there is no such board.
func (s *Server) boardToCoordinate(r *http.Request, person *Person) *Board {
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil || !s.seesOnTheTeam(board, person) {
		return nil
	}
	return board
}

// claimCoordinator claims, renews or relays the person's own entry on a
// board, in the order of the controller: the board, a client, the session,
// then the claim. Another session of the same person takes the entry over and
// since starts anew; other people's entries are neither read nor touched, so
// nothing is ever refused for them. Only then is an archived project refused,
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
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return
	}

	own := s.coordinatorOf(board, person)
	if own == nil || own.Session != *session {
		if board.Coordinators == nil {
			board.Coordinators = map[string]*Coordinator{}
		}
		board.Coordinators[person.ID] = &Coordinator{Session: *session, Since: s.now, LastSeenAt: s.now}
	} else {
		own.LastSeenAt = s.now
	}
	writeJSON(w, http.StatusOK, s.claimResource(board, person))
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

// releaseCoordinator removes the person's own entry on a board and nobody
// else's. Without one it answers the same.
func (s *Server) releaseCoordinator(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.boardToCoordinate(r, person)
	if board == nil {
		notFound(w)
		return
	}
	delete(board.Coordinators, person.ID)
	w.WriteHeader(http.StatusNoContent)
}

// SetNow moves the moment the fake takes for now.
func (s *Server) SetNow(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}
