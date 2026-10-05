package fakeapi

import (
	"cmp"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Reminder is one person's "Remind me" of an item, a comment or a line of a
// project chat. It names its subject by identifier alone: what kind of thing
// that is, its title and whether the person still sees it are worked out on
// every reading, as the server does.
type Reminder struct {
	PersonID  string
	SubjectID string
	RemindAt  time.Time
}

// RemovedReminder is a reminder taken away, with the outcome it was taken away
// with. The server records the outcome and shows it nowhere, so only a test
// can read it here.
type RemovedReminder struct {
	PersonID  string
	SubjectID string
	Outcome   string
}

// mostRemindersOnASide is how many reminders the server reads at most on
// either side of now, before it leaves out those whose subject is out of
// sight.
const mostRemindersOnASide = 500

// reminderExcerptLength is how many characters of a comment, a chat line or
// an answer stand in the title of its reminder, behind the writer's name.
const reminderExcerptLength = 60

// reminderMorningHour is the hour "tomorrow" and "nextMonday" point at.
const reminderMorningHour = 9

// reminderKinds are the words the address of a reminder takes for its kind,
// in the case the server compares them in.
var reminderKinds = []string{
	"card", "post", "todo", "todoList", "document", "upload", "question", "answer", "event", "forward",
	"comment", "chatMessage",
}

func (s *Server) registerReminderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/reminders", s.authenticated(s.myReminders))
	mux.HandleFunc("PUT /api/v1/me/reminders/{kind}/{subjectId}", s.authenticated(s.setReminder))
	mux.HandleFunc("DELETE /api/v1/me/reminders/{kind}/{subjectId}", s.authenticated(s.removeReminder))
}

// Now is the moment the fake takes for now, which a reminder is due by.
func (s *Server) Now() time.Time {
	return s.now
}

// reminderTarget is what a reminder is of, as far as the person may see it.
type reminderTarget struct {
	kind      string
	title     string
	projectID string
	appURL    string
}

// myReminders lists the person's reminders: the due ones first, the most
// recently due first, then those ahead, the soonest first. Each side is cut
// before the ones out of sight are left out, so a side can be cut and still
// be shorter than the cut.
func (s *Server) myReminders(w http.ResponseWriter, _ *http.Request, person *Person) {
	var due, ahead []*Reminder
	for _, reminder := range s.Reminders {
		if reminder.PersonID != person.ID {
			continue
		}
		if reminder.RemindAt.After(s.now) {
			ahead = append(ahead, reminder)
		} else {
			due = append(due, reminder)
		}
	}
	slices.SortFunc(due, func(a, b *Reminder) int {
		return cmp.Or(b.RemindAt.Compare(a.RemindAt), strings.Compare(a.SubjectID, b.SubjectID))
	})
	slices.SortFunc(ahead, func(a, b *Reminder) int {
		return cmp.Or(a.RemindAt.Compare(b.RemindAt), strings.Compare(a.SubjectID, b.SubjectID))
	})

	list := []any{}
	for _, side := range []struct {
		reminders []*Reminder
		isDue     bool
	}{{due, true}, {ahead, false}} {
		for _, reminder := range side.reminders[:min(len(side.reminders), mostRemindersOnASide)] {
			if target := s.reminderTargetOf(reminder.SubjectID, person); target != nil {
				list = append(list, s.reminderResource(target, reminder.SubjectID, reminder.RemindAt, side.isDue))
			}
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// setReminder sets a reminder or moves the one already set, due or not. The
// subject is looked up before the body is read, and a kind other than the
// subject's is the same 404 as a subject that is not there.
func (s *Server) setReminder(w http.ResponseWriter, r *http.Request, person *Person) {
	id := r.PathValue("subjectId")
	target := s.reminderTargetOf(id, person)
	if target == nil || r.PathValue("kind") != target.kind {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	remindAt, ok := s.remindAtIn(w, fields, person)
	if !ok {
		return
	}

	set := false
	for _, reminder := range s.Reminders {
		if reminder.PersonID == person.ID && reminder.SubjectID == id {
			reminder.RemindAt = remindAt
			set = true
		}
	}
	if !set {
		s.Reminders = append(s.Reminders, &Reminder{PersonID: person.ID, SubjectID: id, RemindAt: remindAt})
	}
	writeJSON(w, http.StatusOK, s.reminderResource(target, id, remindAt, false))
}

// removeReminder takes a reminder away whether or not the person still sees
// its subject. The kind has to be one of the words but is not compared; the
// outcome is checked before the reminder is looked for.
func (s *Server) removeReminder(w http.ResponseWriter, r *http.Request, person *Person) {
	id := r.PathValue("subjectId")
	if !slices.Contains(reminderKinds, r.PathValue("kind")) || !uuidPattern.MatchString(id) {
		notFound(w)
		return
	}
	outcome := r.URL.Query().Get("outcome")
	if outcome == "" {
		outcome = "dismissed"
	}
	if outcome != "done" && outcome != "dismissed" {
		unprocessable(w, "reminderOutcomeUnknown", `The outcome of a reminder is "done" or "dismissed".`)
		return
	}

	index := slices.IndexFunc(s.Reminders, func(reminder *Reminder) bool {
		return reminder.PersonID == person.ID && reminder.SubjectID == id
	})
	if index < 0 {
		notFound(w)
		return
	}
	s.Reminders = slices.Delete(s.Reminders, index, index+1)
	s.RemovedReminders = append(s.RemovedReminders, RemovedReminder{PersonID: person.ID, SubjectID: id, Outcome: outcome})
	w.WriteHeader(http.StatusNoContent)
}

// remindAtIn reads the moment of a reminder from a preset or from remindAt.
// A field set to null counts as left out.
func (s *Server) remindAtIn(w http.ResponseWriter, fields map[string]any, person *Person) (time.Time, bool) {
	preset, ok := optionalString(w, fields, "preset")
	if !ok {
		return time.Time{}, false
	}
	moment, ok := optionalString(w, fields, "remindAt")
	if !ok {
		return time.Time{}, false
	}
	if preset != nil && moment != nil {
		malformed(w, `The body names either "preset" or "remindAt", not both.`)
		return time.Time{}, false
	}
	if preset != nil {
		remindAt, known := s.presetMoment(*preset, person)
		if !known {
			unprocessable(w, "reminderPresetUnknown", "The preset is one of in30Minutes, in1Hour, in3Hours, tomorrow and nextMonday.")
		}
		return remindAt, known
	}
	if moment == nil {
		unprocessable(w, "reminderTimeMissing", `A reminder needs "preset" or "remindAt".`)
		return time.Time{}, false
	}
	remindAt, valid := parseReminderMoment(*moment)
	if !valid {
		unprocessable(w, "reminderTimeNotAMoment", "A moment is written in ISO 8601 with its offset, such as 2026-10-06T09:00:00+02:00.")
		return time.Time{}, false
	}
	if !remindAt.After(s.now) {
		unprocessable(w, "reminderTimeGone", "The moment of a reminder has to lie ahead.")
		return time.Time{}, false
	}
	return remindAt, true
}

// optionalString is a field that is left out, null or a string; any other
// value is a malformed body.
func optionalString(w http.ResponseWriter, fields map[string]any, name string) (*string, bool) {
	value, present := fields[name]
	if !present || value == nil {
		return nil, true
	}
	text, ok := value.(string)
	if !ok {
		malformed(w, "The field \""+name+"\" has to be a string.")
		return nil, false
	}
	return &text, true
}

// presetMoment is the moment a preset of the web menu points at, counted in
// the person's zone: the morning hour of the next day, or of the next Monday,
// which on a Monday is the Monday a week on.
func (s *Server) presetMoment(preset string, person *Person) (time.Time, bool) {
	_, zone := s.zoneOf(person)
	local := s.now.In(zone)
	morning := func(days int) time.Time {
		return time.Date(local.Year(), local.Month(), local.Day()+days, reminderMorningHour, 0, 0, 0, zone)
	}
	switch preset {
	case "in30Minutes":
		return s.now.Add(30 * time.Minute), true
	case "in1Hour":
		return s.now.Add(time.Hour), true
	case "in3Hours":
		return s.now.Add(3 * time.Hour), true
	case "tomorrow":
		return morning(1), true
	case "nextMonday":
		days := (int(time.Monday) - int(local.Weekday()) + 7) % 7
		if days == 0 {
			days = 7
		}
		return morning(days), true
	}
	return time.Time{}, false
}

// rfc3339Moment is a moment as the server reads one: RFC 3339 with a capital
// T, seconds, a fraction of any length and Z or an offset of ±HH:MM up to 14
// hours.
var rfc3339Moment = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d)(?:\.(\d+))?(Z|[+-](?:(?:0\d|1[0-3]):[0-5]\d|14:00))$`)

// parseReminderMoment reads a moment the way the server does: in the form
// above, on a day that exists, its fraction cut to microseconds.
func parseReminderMoment(value string) (time.Time, bool) {
	parts := rfc3339Moment.FindStringSubmatch(value)
	if parts == nil {
		return time.Time{}, false
	}
	fraction := (parts[2] + "000000")[:6]
	moment, err := time.Parse(time.RFC3339Nano, parts[1]+"."+fraction+parts[3])
	return moment, err == nil
}

func (s *Server) reminderResource(target *reminderTarget, id string, remindAt time.Time, isDue bool) map[string]any {
	return map[string]any{
		"kind": target.kind, "id": id, "title": target.title, "project": s.projectReference(target.projectID),
		"remindAt": timestamp(remindAt), "isDue": isDue, "appUrl": target.appURL,
	}
}

// reminderTargetOf finds what an identifier names among the things a reminder
// can be of, as their pages find them: a card on its board or in its archive,
// a post or a document that went out or is the person's own draft, a to-do, a
// list, an upload, a question, an answer, an event, a comment on any of them,
// and a line of a project chat. Nothing else, and nothing the person does not
// see. The fake has no forwards, and a client sees a document or an upload
// only below a folder shown to clients: the switch of an entry at the top of
// a tool is not kept.
func (s *Server) reminderTargetOf(id string, person *Person) *reminderTarget {
	if !uuidPattern.MatchString(id) {
		return nil
	}
	if target := s.reminderItemOf(id, person); target != nil {
		return target
	}
	if target := s.reminderCommentOf(id, person); target != nil {
		return target
	}
	return s.reminderChatLineOf(id, person)
}

// reminderItem is an item a reminder can be of, with the comments under it.
type reminderItem struct {
	id       string
	target   reminderTarget
	comments []*Comment
}

func (s *Server) reminderItemOf(id string, person *Person) *reminderTarget {
	for _, item := range s.reminderItems(person) {
		if item.id == id {
			target := item.target
			return &target
		}
	}
	return nil
}

func (s *Server) reminderCommentOf(id string, person *Person) *reminderTarget {
	for _, item := range s.reminderItems(person) {
		for _, comment := range item.comments {
			if comment.ID == id {
				return &reminderTarget{
					kind: "comment", title: s.writerAndStart(comment.AuthorID, excerptOfMarkup(comment.Body)),
					projectID: item.target.projectID, appURL: item.target.appURL + "#comment-" + comment.ID,
				}
			}
		}
	}
	return nil
}

func (s *Server) reminderChatLineOf(id string, person *Person) *reminderTarget {
	for _, chat := range s.Chats {
		if chat.isConversation() || s.visibleChat(chat.ID, person) == nil {
			continue
		}
		for _, line := range chat.Messages {
			if line.ID == id && !line.Removed {
				return &reminderTarget{
					kind: "chatMessage", title: s.writerAndStart(line.AuthorID, excerptOf(line.BodyText)), projectID: chat.ProjectID,
					appURL: s.URL + "/projects/" + chat.ProjectID + "/chats/" + chat.ID + "#chat-message-" + line.ID,
				}
			}
		}
	}
	return nil
}

// reminderItems are all items the person sees as a reminder sees them.
func (s *Server) reminderItems(person *Person) []reminderItem {
	var items []reminderItem
	add := func(id, kind, title, projectID, appURL string, comments []*Comment) {
		items = append(items, reminderItem{id, reminderTarget{kind: kind, title: title, projectID: projectID, appURL: appURL}, comments})
	}
	for _, board := range s.Boards {
		if s.visibleProject(board.ProjectID, person) == nil {
			continue
		}
		for _, column := range board.Columns {
			for _, card := range column.Cards {
				add(card.ID, "card", card.Title, board.ProjectID, s.cardURL(board, card), card.Comments)
			}
		}
	}
	for _, archived := range s.Archive {
		if board := s.boardOfColumn(archived.Column); board != nil && s.visibleProject(board.ProjectID, person) != nil {
			add(archived.Card.ID, "card", archived.Card.Title, board.ProjectID, s.cardURL(board, archived.Card), archived.Card.Comments)
		}
	}
	for _, tool := range s.PostsTools {
		if s.visiblePostsTool(tool.ID, person) == nil {
			continue
		}
		for _, post := range tool.Posts {
			if !post.Trashed && (!post.Draft || post.CreatorID == person.ID) && (post.ShownToClients || !s.isAClient(person.ID)) {
				add(post.ID, "post", post.Title, tool.ProjectID, s.postURL(tool, post), post.Comments)
			}
		}
	}
	for _, tool := range s.readableTodosTools() {
		for _, list := range tool.Lists {
			if _, visible := s.visibleTodoList(list.ID, person); visible != nil {
				add(list.ID, "todoList", list.Title, tool.ProjectID, s.todoListURL(tool, list), list.Comments)
			}
			for _, todo := range list.Todos {
				if _, _, visible := s.visibleTodo(todo.ID, person); visible != nil {
					add(todo.ID, "todo", todo.Title, tool.ProjectID, s.todoURL(tool, todo), todo.Comments)
				}
			}
		}
	}
	for _, tool := range s.FilesTools {
		if s.visibleFilesTool(tool.ID, person) == nil {
			continue
		}
		client := s.isAClient(person.ID)
		for _, document := range tool.Documents {
			if (!document.Draft || document.CreatorID == person.ID) && (!client || shownToClients(tool, document.FolderID)) {
				add(document.ID, "document", document.Title, tool.ProjectID, s.documentURL(tool, document), document.Comments)
			}
		}
		for _, upload := range tool.Uploads {
			if !client || shownToClients(tool, upload.FolderID) {
				add(upload.ID, "upload", upload.Title, tool.ProjectID, s.filesUploadURL(tool, upload), upload.Comments)
			}
		}
	}
	for _, tool := range s.CheckinsTools {
		for _, question := range tool.Questions {
			if _, visible := s.visibleCheckinQuestion(question.ID, person); visible == nil {
				continue
			}
			add(question.ID, "question", question.Title, tool.ProjectID, s.checkinQuestionURL(tool, question), nil)
			for _, answer := range question.Answers {
				if !answer.Trashed {
					add(answer.ID, "answer", s.writerAndStart(answer.AuthorID, excerptOf(wordsOf(answer.Body))), tool.ProjectID, s.checkinAnswerURL(tool, answer), answer.Comments)
				}
			}
		}
	}
	for _, tool := range s.Calendars {
		for _, event := range tool.Events {
			if _, visible := s.visibleEvent(event.ID, person); visible != nil {
				add(event.ID, "event", event.Title, tool.ProjectID, s.eventURL(tool, event), event.Comments)
			}
		}
	}
	return items
}

// writerAndStart is the title of a reminder of what somebody wrote: their name
// and the start of the words.
func (s *Server) writerAndStart(authorID, excerpt string) string {
	name := s.People[authorID].Name
	if excerpt == "" {
		return name
	}
	return name + ": " + excerpt
}

var whiteSpace = regexp.MustCompile(`\s+`)

func excerptOfMarkup(body string) string {
	return excerptOf(html.UnescapeString(tagForm.ReplaceAllString(strings.ReplaceAll(body, "<", " <"), "")))
}

// lineBreakingTag is a tag of an element the server puts on a line of its
// own when it reads the words of a text, or a line break.
var lineBreakingTag = regexp.MustCompile(`(?i)</?(?:blockquote|div|figure|h[1-6]|hr|li|ol|p|pre|table|tbody|tr|ul|br)\b[^<>]*>`)

// wordsOf is the plain text the server keeps beside the HTML of an answer:
// a block on a line of its own, the other tags gone, the entities decoded.
// Inline elements next to each other run together, as they do there.
func wordsOf(body string) string {
	return html.UnescapeString(tagForm.ReplaceAllString(lineBreakingTag.ReplaceAllString(body, "\n"), ""))
}

// excerptOf is the start of a text in one line, cut behind a whole number of
// characters and marked as cut.
func excerptOf(text string) string {
	text = strings.TrimSpace(whiteSpace.ReplaceAllString(text, " "))
	if utf8.RuneCountInString(text) <= reminderExcerptLength {
		return text
	}
	return strings.TrimRight(string([]rune(text)[:reminderExcerptLength]), " ") + "…"
}
