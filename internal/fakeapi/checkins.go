package fakeapi

import (
	"cmp"
	"encoding/json"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/richtext"
)

// What the check-ins of a project take, as the server holds them.
const (
	maxAnswerLength         = 100000
	answersOnAPage          = 50
	mostAnswersOnAPage      = 100
	mostOpenCheckinReminder = 100

	// daysToLookBack is how far the server looks for the latest asking of a
	// question: a monthly one may lie 37 days back.
	daysToLookBack = 38
)

// The four rhythms a question is asked in.
const (
	RhythmDaily          = "daily"
	RhythmWeekly         = "weekly"
	RhythmEveryOtherWeek = "everyOtherWeek"
	RhythmMonthly        = "monthly"
)

// timeOfDayPattern is what the server reads as a time of day: one or two
// digits for the hour, two for the minute, and seconds only when they are
// none.
var timeOfDayPattern = regexp.MustCompile(`^(\d{1,2}):(\d{2})(?::00)?$`)

// CheckinsTool is a Check-ins tool of a project, with its questions in the
// order they were set up.
type CheckinsTool struct {
	ID        string
	ProjectID string
	Title     string

	// Archived is a tool whose status is no longer active: the project names
	// it among its tools no more, and at its own address it is read and
	// written as before.
	Archived bool

	// Trashed is a tool in the trash of its project: it and its questions are
	// at no address, and nobody is reminded of them.
	Trashed bool

	Questions []*CheckinQuestion
}

// CheckinQuestion is a question asked on a schedule. The time of day holds in
// the zone of each person asked; the fake knows no zone but UTC.
type CheckinQuestion struct {
	ID        string
	Title     string
	Rhythm    string
	Weekdays  []int  // ISO numbers, Monday 1 to Sunday 7, each once and in order
	TimeOfDay string // HH:MM
	StartsOn  string // YYYY-MM-DD, what every other week is counted from
	Paused    bool
	AskedIDs  []string // whoever was asked, members or not any more
	CreatorID string
	CreatedAt time.Time
	Trashed   bool

	// ShownToClients says whether the clients of the project see the question;
	// a client it is not shown to cannot be named to be asked, and one asked
	// before is named nowhere.
	ShownToClients bool

	Answers   []*CheckinAnswer
	Reminders []*CheckinReminder
}

// CheckinAnswer is what somebody said to a question for one day.
type CheckinAnswer struct {
	ID        string
	AuthorID  string
	GroupOn   string // the day it belongs to, YYYY-MM-DD
	Body      string // the HTML as stored
	CreatedAt time.Time
	EditedAt  *time.Time
	Comments  []*Comment
	Trashed   bool
}

// CheckinReminder is a question put to one person for one day. It stays when
// the day is answered; whether it is open is worked out when it is read.
type CheckinReminder struct {
	PersonID string
	AskedFor string // YYYY-MM-DD
	AskedAt  time.Time
}

func (s *Server) registerCheckinsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectId}/checkins", s.authenticated(s.checkinsTools))
	mux.HandleFunc("GET /api/v1/checkins/{toolId}", s.authenticated(s.checkinsTool))
	mux.HandleFunc("GET /api/v1/checkins/{toolId}/questions", s.authenticated(s.checkinQuestions))
	mux.HandleFunc("POST /api/v1/checkins/{toolId}/questions", s.authenticated(s.addCheckinQuestion))
	mux.HandleFunc("GET /api/v1/questions/{questionId}", s.authenticated(s.checkinQuestion))
	mux.HandleFunc("PATCH /api/v1/questions/{questionId}", s.authenticated(s.updateCheckinQuestion))
	mux.HandleFunc("POST /api/v1/questions/{questionId}/pause", s.authenticated(s.pauseCheckinQuestion))
	mux.HandleFunc("POST /api/v1/questions/{questionId}/resume", s.authenticated(s.resumeCheckinQuestion))
	mux.HandleFunc("GET /api/v1/questions/{questionId}/answers", s.authenticated(s.checkinAnswers))
	mux.HandleFunc("POST /api/v1/questions/{questionId}/answers", s.authenticated(s.addCheckinAnswer))
	mux.HandleFunc("GET /api/v1/answers/{answerId}", s.authenticated(s.checkinAnswer))
	mux.HandleFunc("PATCH /api/v1/answers/{answerId}", s.authenticated(s.updateCheckinAnswer))
	mux.HandleFunc("GET /api/v1/me/checkin-reminders", s.authenticated(s.myCheckinReminders))
}

// checkinsTools lists the Check-ins tools of a project that are in use, in the
// order of the project page.
func (s *Server) checkinsTools(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	tools := []any{}
	for _, tool := range s.CheckinsTools {
		if tool.ProjectID == project.ID && !tool.Archived && !tool.Trashed {
			tools = append(tools, map[string]any{"id": tool.ID, "title": tool.Title, "appUrl": s.checkinsToolURL(tool)})
		}
	}
	writeJSON(w, http.StatusOK, tools)
}

// checkinsTool reads a tool by its identifier alone, with the project it
// stands in. A tool no longer in use is read as well.
func (s *Server) checkinsTool(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleCheckinsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.toolResource(tool.ID, tool.ProjectID, tool.Title, s.checkinsToolURL(tool)))
}

// checkinQuestions lists the questions of a tool out of the trash, the oldest
// first.
func (s *Server) checkinQuestions(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleCheckinsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	questions := []any{}
	for _, question := range tool.Questions {
		if !question.Trashed {
			questions = append(questions, s.checkinQuestionResource(tool, question))
		}
	}
	writeJSON(w, http.StatusOK, questions)
}

// addCheckinQuestion sets up a question that starts on the writer's today. The
// body is read field by field, the wording and the schedule are held before
// the tool is looked up, and nobody asked, the archived project and a stranger
// among the people only after it, in that order.
func (s *Server) addCheckinQuestion(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBodyWith(w, r, true)
	if !ok {
		return
	}
	body, ok := readQuestionFields(w, fields)
	if !ok {
		return
	}
	if !validTodoTitle(w, "question", "a question", deref(body.title)) {
		return
	}
	weekdays := body.weekdays
	if weekdays == nil {
		weekdays = []int{}
	}
	today := s.now.UTC().Format("2006-01-02")
	rhythm, days, timeOfDay, ok := checkinSchedule(w, deref(body.rhythm), weekdays, deref(body.timeOfDay))
	if !ok {
		return
	}
	tool := s.visibleCheckinsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	if len(body.askedIDs) == 0 {
		nobodyAsked(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	// A question set up through the API is shown to no client.
	if !s.allAsked(w, tool, &CheckinQuestion{}, body.askedIDs) {
		return
	}

	question := &CheckinQuestion{
		ID: newID(), Title: phpTrim(deref(body.title)), Rhythm: rhythm, Weekdays: days, TimeOfDay: timeOfDay,
		StartsOn: today, AskedIDs: withoutRepeats(body.askedIDs), CreatorID: person.ID, CreatedAt: s.now,
	}
	tool.Questions = append(tool.Questions, question)
	w.Header().Set("Location", s.URL+"/api/v1/questions/"+question.ID)
	writeJSON(w, http.StatusCreated, s.checkinQuestionResource(tool, question))
}

func (s *Server) checkinQuestion(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, question := s.visibleCheckinQuestion(r.PathValue("questionId"), person)
	if question == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.checkinQuestionResource(tool, question))
}

// updateCheckinQuestion changes what the body names and keeps the rest, the
// start day always, and the people asked where the body names none. Named
// people are the whole set only over whom the question shows as asked: a
// member who left and a client it is not shown to stay as they are.
func (s *Server) updateCheckinQuestion(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBodyWith(w, r, true)
	if !ok {
		return
	}
	body, ok := readQuestionFields(w, fields)
	if !ok {
		return
	}
	if body.title == nil && body.rhythm == nil && body.weekdays == nil && body.timeOfDay == nil && body.askedIDs == nil {
		unprocessable(w, "nothingToChange", "The body names neither a title nor a schedule nor the people asked.")
		return
	}
	if body.title != nil && !validTodoTitle(w, "question", "a question", *body.title) {
		return
	}
	tool, question := s.visibleCheckinQuestion(r.PathValue("questionId"), person)
	if question == nil {
		notFound(w)
		return
	}
	rhythm, weekdays, timeOfDay := question.Rhythm, question.Weekdays, question.TimeOfDay
	if body.rhythm != nil {
		rhythm = *body.rhythm
	}
	if body.weekdays != nil {
		weekdays = body.weekdays
	}
	if body.timeOfDay != nil {
		timeOfDay = *body.timeOfDay
	}
	rhythm, days, timeOfDay, ok := checkinSchedule(w, rhythm, weekdays, timeOfDay)
	if !ok {
		return
	}
	asked := body.askedIDs
	if asked != nil && len(asked) == 0 {
		nobodyAsked(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if !s.allAsked(w, tool, question, asked) {
		return
	}
	offered := s.shownAsAsked(tool, question)

	if body.title != nil {
		question.Title = phpTrim(*body.title)
	}
	question.Rhythm, question.Weekdays, question.TimeOfDay = rhythm, days, timeOfDay
	if asked != nil {
		question.AskedIDs = askedOnly(question.AskedIDs, asked, offered)
	}
	writeJSON(w, http.StatusOK, s.checkinQuestionResource(tool, question))
}

// pauseCheckinQuestion pauses a question; pausing a paused one is no error.
func (s *Server) pauseCheckinQuestion(w http.ResponseWriter, r *http.Request, person *Person) {
	s.setCheckinQuestionPaused(w, r, person, true)
}

// resumeCheckinQuestion asks a paused question again; resuming a running one
// is no error.
func (s *Server) resumeCheckinQuestion(w http.ResponseWriter, r *http.Request, person *Person) {
	s.setCheckinQuestionPaused(w, r, person, false)
}

// setCheckinQuestionPaused reads no body. In an archived project it refuses
// whatever state the question is in, because the statement that finds a
// question in that state asks for a project in use as well.
func (s *Server) setCheckinQuestionPaused(w http.ResponseWriter, r *http.Request, person *Person, paused bool) {
	tool, question := s.visibleCheckinQuestion(r.PathValue("questionId"), person)
	if question == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	question.Paused = paused
	writeJSON(w, http.StatusOK, s.checkinQuestionResource(tool, question))
}

// checkinAnswers lists a page of the answers to a question, the newest day
// first and within a day the one written last first. The page carries no sign
// of its end, and a size above the largest is cut down to it without a word.
func (s *Server) checkinAnswers(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, question := s.visibleCheckinQuestion(r.PathValue("questionId"), person)
	if question == nil {
		notFound(w)
		return
	}
	query := r.URL.Query()
	on, limitValue := query.Get("on"), query.Get("limit")
	personID, before := strings.ToLower(query.Get("personId")), strings.ToLower(query.Get("before"))
	if on != "" && !isAnswerDay(on) {
		unprocessable(w, "answerDayNotADate", "A day is written as YYYY-MM-DD.")
		return
	}
	limit := answersOnAPage
	if limitValue != "" {
		n, err := strconv.Atoi(limitValue)
		if !aWholeNumber.MatchString(limitValue) || err != nil || n < 1 {
			unprocessable(w, "answersLimitNotANumber", "The number of answers asked for has to be a whole number of at least one.")
			return
		}
		limit = min(n, mostAnswersOnAPage)
	}
	if personID != "" && !uuidPattern.MatchString(personID) {
		notFound(w)
		return
	}
	var boundary *CheckinAnswer
	if before != "" {
		// Any answer to the question marks a boundary, one in the trash too.
		boundary = answerOf(question, before)
		if boundary == nil {
			unprocessable(w, "answersBeforeUnknown", "The page starts behind an answer to this question, named by its id.")
			return
		}
	}

	answers := s.shownAnswers(question)
	answers = slices.DeleteFunc(answers, func(answer *CheckinAnswer) bool {
		return personID != "" && answer.AuthorID != personID ||
			on != "" && answer.GroupOn != on ||
			boundary != nil && compareAnswers(answer, boundary) <= 0
	})
	page := []any{}
	for _, answer := range answers[:min(limit, len(answers))] {
		page = append(page, s.checkinAnswerLineResource(tool, question, answer))
	}
	writeJSON(w, http.StatusOK, page)
}

// addCheckinAnswer answers a question for the day named in "askedFor", which
// has to be one the question was put to the writer for, or else for the day of
// its latest asking. What the day looks like is held before the question is
// looked up, the body after it, and whether the day was asked only then.
func (s *Server) addCheckinAnswer(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	bodyValue, ok1 := stringField(w, fields, "body")
	askedFor, ok2 := stringField(w, fields, "askedFor")
	if !ok1 || !ok2 {
		return
	}
	if askedFor != nil && !isAnswerDay(*askedFor) {
		unprocessable(w, "answerDayNotADate", "The day asked for is written as YYYY-MM-DD.")
		return
	}
	tool, question := s.visibleCheckinQuestion(r.PathValue("questionId"), person)
	if question == nil {
		notFound(w)
		return
	}
	body, ok := storedAnswer(w, deref(bodyValue))
	if !ok {
		return
	}
	groupOn := s.dayAnAnswerBelongsTo(question)
	if askedFor != nil {
		if !wasAskedFor(question, person.ID, *askedFor) {
			unprocessable(w, "answerDayNotAsked", "This question was not put to you for that day.")
			return
		}
		groupOn = *askedFor
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}

	answer := &CheckinAnswer{ID: newID(), AuthorID: person.ID, GroupOn: groupOn, Body: body, CreatedAt: s.now}
	question.Answers = append(question.Answers, answer)
	w.Header().Set("Location", s.URL+"/api/v1/answers/"+answer.ID)
	writeJSON(w, http.StatusCreated, s.checkinAnswerResource(tool, question, answer))
}

func (s *Server) checkinAnswer(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, question, answer := s.visibleCheckinAnswer(r.PathValue("answerId"), person)
	if answer == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.checkinAnswerResource(tool, question, answer))
}

// updateCheckinAnswer gives an answer a new body. Only its writer may, and
// anybody else is answered as if it were not there; the same body again is no
// error and changes nothing. The day an answer belongs to never changes.
func (s *Server) updateCheckinAnswer(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	bodyValue, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	if bodyValue == nil {
		unprocessable(w, "nothingToChange", "The body names no new text for the answer.")
		return
	}
	tool, question, answer := s.visibleCheckinAnswer(r.PathValue("answerId"), person)
	if answer == nil || answer.AuthorID != person.ID {
		notFound(w)
		return
	}
	body, ok := storedAnswer(w, *bodyValue)
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if body != answer.Body {
		answer.Body = body
		edited := s.now.Add(time.Minute)
		answer.EditedAt = &edited
	}
	writeJSON(w, http.StatusOK, s.checkinAnswerResource(tool, question, answer))
}

// myCheckinReminders lists what the person of the token was asked and has not
// answered yet, the one asked last first, at most a hundred and with no way
// to the rest. Per question only the latest day asked counts, and it is open
// while no answer of theirs belongs to that day or a later one.
func (s *Server) myCheckinReminders(w http.ResponseWriter, _ *http.Request, person *Person) {
	type open struct {
		tool     *CheckinsTool
		question *CheckinQuestion
		reminder *CheckinReminder
	}
	var reminders []open
	for _, tool := range s.CheckinsTools {
		project := s.projectWithID(tool.ProjectID)
		if tool.Trashed || project == nil || project.Archived || !slices.Contains(project.MemberIDs, person.ID) {
			continue
		}
		for _, question := range tool.Questions {
			if question.Trashed || question.Paused || !slices.Contains(question.AskedIDs, person.ID) || !s.seesQuestion(person, question) {
				continue
			}
			latest := latestReminderOf(question, person.ID)
			if latest == nil || answeredOnOrAfter(question, person.ID, latest.AskedFor) {
				continue
			}
			reminders = append(reminders, open{tool, question, latest})
		}
	}
	slices.SortFunc(reminders, func(a, b open) int {
		return cmp.Or(b.reminder.AskedAt.Compare(a.reminder.AskedAt), strings.Compare(b.reminder.AskedFor, a.reminder.AskedFor), strings.Compare(a.question.ID, b.question.ID))
	})

	list := []any{}
	for _, one := range reminders[:min(len(reminders), mostOpenCheckinReminder)] {
		list = append(list, map[string]any{
			"question": map[string]any{"id": one.question.ID, "title": one.question.Title},
			"project":  s.projectReference(one.tool.ProjectID),
			"tool":     map[string]any{"id": one.tool.ID, "title": one.tool.Title},
			"askedFor": one.reminder.AskedFor, "askedAt": timestamp(one.reminder.AskedAt),
			"appUrl": s.checkinQuestionURL(one.tool, one.question) + "?askedFor=" + one.reminder.AskedFor,
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// questionFields is what the body of a question names; nil for what it leaves
// out or sets to null.
type questionFields struct {
	title, rhythm, timeOfDay *string
	weekdays                 []int
	askedIDs                 []string
}

// readQuestionFields reads the body of a question in the order the server
// does: a field of the wrong type is malformed, and an identifier of a person
// that is none is refused like a stranger, before anything else is held.
func readQuestionFields(w http.ResponseWriter, fields map[string]any) (questionFields, bool) {
	var body questionFields
	var ok bool
	if body.title, ok = stringField(w, fields, "title"); !ok {
		return body, false
	}
	if body.rhythm, ok = stringField(w, fields, "rhythm"); !ok {
		return body, false
	}
	if value, present := fields["weekdays"]; present && value != nil {
		list, isList := value.([]any)
		if !isList {
			malformed(w, `The field "weekdays" has to be a list of whole numbers.`)
			return body, false
		}
		body.weekdays = make([]int, 0, len(list))
		for _, entry := range list {
			// A number written with a fraction or an exponent is no whole
			// number to the server, whatever its value.
			written, isNumber := entry.(json.Number)
			number, err := strconv.Atoi(string(written))
			if !isNumber || err != nil {
				malformed(w, `The field "weekdays" has to be a list of whole numbers.`)
				return body, false
			}
			body.weekdays = append(body.weekdays, number)
		}
	}
	if body.timeOfDay, ok = stringField(w, fields, "timeOfDay"); !ok {
		return body, false
	}
	if value, present := fields["askedIds"]; present && value != nil {
		list, isList := value.([]any)
		if !isList {
			malformed(w, `The field "askedIds" has to be a list of strings.`)
			return body, false
		}
		body.askedIDs = make([]string, 0, len(list))
		for _, entry := range list {
			id, isText := entry.(string)
			if !isText {
				malformed(w, `The field "askedIds" has to be a list of strings.`)
				return body, false
			}
			body.askedIDs = append(body.askedIDs, id)
		}
		for _, id := range body.askedIDs {
			if !uuidPattern.MatchString(id) {
				notAskable(w)
				return body, false
			}
		}
	}
	return body, true
}

// checkinSchedule holds a schedule the way the server builds one: weekdays at
// all, then the time, then the rhythm and the weekdays together. What it
// keeps is the weekdays each once and in the order of the week, and the time
// written with two digits each.
func checkinSchedule(w http.ResponseWriter, rhythm string, weekdays []int, timeOfDay string) (string, []int, string, bool) {
	if len(weekdays) == 0 {
		unprocessable(w, "questionWeekdaysMissing", "A question is asked on at least one weekday.")
		return "", nil, "", false
	}
	match := timeOfDayPattern.FindStringSubmatch(phpTrim(timeOfDay))
	hour, minute := -1, -1
	if match != nil {
		hour, _ = strconv.Atoi(match[1])
		minute, _ = strconv.Atoi(match[2])
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		unprocessable(w, "questionTimeInvalid", "The time of day is written as HH:MM on the 24-hour clock.")
		return "", nil, "", false
	}
	days := slices.Clone(weekdays)
	slices.Sort(days)
	days = slices.Compact(days)
	known := rhythm == RhythmDaily || rhythm == RhythmWeekly || rhythm == RhythmEveryOtherWeek || rhythm == RhythmMonthly
	outOfTheWeek := slices.ContainsFunc(days, func(day int) bool { return day < 1 || day > 7 })
	if !known || outOfTheWeek || rhythm != RhythmDaily && len(days) != 1 {
		unprocessable(w, "questionScheduleInvalid", "The rhythm is daily, weekly, everyOtherWeek or monthly, the weekdays are numbers from 1 (Monday) to 7, and every rhythm but daily takes exactly one.")
		return "", nil, "", false
	}
	return rhythm, days, time.Date(0, 1, 1, hour, minute, 0, 0, time.UTC).Format("15:04"), true
}

// storedAnswer is an answer as the server keeps it: held against its length
// as sent and again as it would be stored, and refused where it says nothing
// and carries no file either.
func storedAnswer(w http.ResponseWriter, value string) (string, bool) {
	const tooLong, message = "answerTooLong", "The answer is too long."
	stored, ok := storedRichText(w, tooLong, message, maxAnswerLength, value)
	if !ok {
		return "", false
	}
	if utf8.RuneCountInString(stored) > maxAnswerLength {
		unprocessable(w, tooLong, message)
		return "", false
	}
	text := strings.TrimSpace(html.UnescapeString(tagForm.ReplaceAllString(stored, "")))
	if text == "" && !richtext.HasAttachments(stored) {
		unprocessable(w, "answerMissing", "An answer needs a body.")
		return "", false
	}
	return stored, true
}

// allAsked refuses the write where somebody named is no member of the project
// or may not see the question, which a client of the project may not where the
// question is not shown to clients. Minka answers both alike.
func (s *Server) allAsked(w http.ResponseWriter, tool *CheckinsTool, question *CheckinQuestion, ids []string) bool {
	project := s.projectWithID(tool.ProjectID)
	for _, id := range ids {
		if !slices.Contains(project.MemberIDs, id) || !s.seesQuestion(s.People[id], question) {
			notAskable(w)
			return false
		}
	}
	return true
}

// askedOnly takes off of the people asked whoever was offered and is not named
// again, and adds whoever is named and not asked yet.
func askedOnly(carried, named, offered []string) []string {
	kept := slices.DeleteFunc(slices.Clone(carried), func(id string) bool {
		return slices.Contains(offered, id) && !slices.Contains(named, id)
	})
	for _, id := range named {
		if !slices.Contains(kept, id) {
			kept = append(kept, id)
		}
	}
	return kept
}

func notAskable(w http.ResponseWriter) {
	unprocessable(w, "personNotAProjectMember", "Only a member of the project can be asked.")
}

func nobodyAsked(w http.ResponseWriter) {
	unprocessable(w, "questionNobodyAsked", "A question asks at least one person.")
}

// shownAsAsked are the people asked who are still members and who may see the
// question, in the order of the members' names. An agent that stepped down is
// a member only where its project was archived then, and is listed there.
func (s *Server) shownAsAsked(tool *CheckinsTool, question *CheckinQuestion) []string {
	project := s.projectWithID(tool.ProjectID)
	var asked []string
	for _, id := range project.MemberIDs {
		member := s.People[id]
		if !slices.Contains(question.AskedIDs, id) || !s.seesQuestion(member, question) {
			continue
		}
		asked = append(asked, id)
	}
	slices.SortStableFunc(asked, func(a, b string) int {
		return strings.Compare(strings.ToLower(s.People[a].Name), strings.ToLower(s.People[b].Name))
	})
	return asked
}

// seesQuestion is false for a client the question is not shown to, and for an
// agent working for one.
func (s *Server) seesQuestion(person *Person, question *CheckinQuestion) bool {
	return !s.isAClient(person.ID) || question.ShownToClients
}

// dayAnAnswerBelongsTo is the day of the latest asking of a question at or
// before the fake's now, or today where it was never asked yet.
func (s *Server) dayAnAnswerBelongsTo(question *CheckinQuestion) string {
	now := s.now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for back := 0; back <= daysToLookBack; back++ {
		day := today.AddDate(0, 0, -back)
		if day.Format("2006-01-02") < question.StartsOn {
			break
		}
		if !question.fallsOn(day) {
			continue
		}
		asksAt, _ := time.Parse("2006-01-02 15:04", day.Format("2006-01-02")+" "+question.TimeOfDay)
		if !asksAt.After(now) {
			return day.Format("2006-01-02")
		}
	}
	return today.Format("2006-01-02")
}

// fallsOn says whether a question is asked on a day: one of its weekdays, not
// before its start, every other week counted from the first matching day on
// or after the start, and monthly within the first seven days.
func (q *CheckinQuestion) fallsOn(day time.Time) bool {
	if day.Format("2006-01-02") < q.StartsOn || !slices.Contains(q.Weekdays, isoWeekday(day)) {
		return false
	}
	switch q.Rhythm {
	case RhythmEveryOtherWeek:
		start, _ := time.Parse("2006-01-02", q.StartsOn)
		first := start.AddDate(0, 0, (q.Weekdays[0]-isoWeekday(start)+7)%7)
		return int(day.Sub(first).Hours()/24)%14 == 0
	case RhythmMonthly:
		return day.Day() <= 7
	}
	return true
}

func isoWeekday(day time.Time) int {
	if day.Weekday() == time.Sunday {
		return 7
	}
	return int(day.Weekday())
}

func isAnswerDay(value string) bool {
	day, err := time.Parse("2006-01-02", value)
	return err == nil && day.Format("2006-01-02") == value
}

func wasAskedFor(question *CheckinQuestion, personID, day string) bool {
	return slices.ContainsFunc(question.Reminders, func(reminder *CheckinReminder) bool {
		return reminder.PersonID == personID && reminder.AskedFor == day
	})
}

func latestReminderOf(question *CheckinQuestion, personID string) *CheckinReminder {
	var latest *CheckinReminder
	for _, reminder := range question.Reminders {
		if reminder.PersonID == personID && (latest == nil || reminder.AskedFor > latest.AskedFor) {
			latest = reminder
		}
	}
	return latest
}

func answeredOnOrAfter(question *CheckinQuestion, personID, day string) bool {
	return slices.ContainsFunc(question.Answers, func(answer *CheckinAnswer) bool {
		return !answer.Trashed && answer.AuthorID == personID && answer.GroupOn >= day
	})
}

func answerOf(question *CheckinQuestion, answerID string) *CheckinAnswer {
	for _, answer := range question.Answers {
		if answer.ID == answerID {
			return answer
		}
	}
	return nil
}

// compareAnswers orders answers the way a page lists them: the newest day
// first, and within a day the one written last first.
func compareAnswers(a, b *CheckinAnswer) int {
	return cmp.Or(strings.Compare(b.GroupOn, a.GroupOn), b.CreatedAt.Compare(a.CreatedAt), strings.Compare(b.ID, a.ID))
}

// shownAnswers are the answers to a question out of the trash, in the order of
// a page.
func (s *Server) shownAnswers(question *CheckinQuestion) []*CheckinAnswer {
	answers := slices.DeleteFunc(slices.Clone(question.Answers), func(answer *CheckinAnswer) bool { return answer.Trashed })
	slices.SortStableFunc(answers, compareAnswers)
	return answers
}

// visibleCheckinsTool finds a tool that is not in the trash, in a project the
// person is a member of. A tool that is no longer in use is found as well.
func (s *Server) visibleCheckinsTool(toolID string, person *Person) *CheckinsTool {
	if !uuidPattern.MatchString(toolID) {
		return nil
	}
	for _, tool := range s.CheckinsTools {
		if tool.ID == toolID && !tool.Trashed && s.visibleProject(tool.ProjectID, person) != nil {
			return tool
		}
	}
	return nil
}

// visibleCheckinQuestion finds a question out of the trash by its identifier
// alone.
func (s *Server) visibleCheckinQuestion(questionID string, person *Person) (*CheckinsTool, *CheckinQuestion) {
	if !uuidPattern.MatchString(questionID) {
		return nil, nil
	}
	for _, tool := range s.CheckinsTools {
		for _, question := range tool.Questions {
			if question.ID != questionID || question.Trashed || tool.Trashed {
				continue
			}
			if s.visibleProject(tool.ProjectID, person) == nil || !s.seesQuestion(person, question) {
				return nil, nil
			}
			return tool, question
		}
	}
	return nil, nil
}

// visibleCheckinAnswer finds an answer out of the trash whose question is out
// of the trash as well.
func (s *Server) visibleCheckinAnswer(answerID string, person *Person) (*CheckinsTool, *CheckinQuestion, *CheckinAnswer) {
	if !uuidPattern.MatchString(answerID) {
		return nil, nil, nil
	}
	for _, tool := range s.CheckinsTools {
		for _, question := range tool.Questions {
			answer := answerOf(question, answerID)
			if answer == nil || answer.Trashed {
				continue
			}
			if _, visible := s.visibleCheckinQuestion(question.ID, person); visible == nil {
				return nil, nil, nil
			}
			return tool, question, answer
		}
	}
	return nil, nil, nil
}

func (s *Server) checkinQuestionResource(tool *CheckinsTool, question *CheckinQuestion) map[string]any {
	asked := []any{}
	for _, id := range s.shownAsAsked(tool, question) {
		asked = append(asked, s.personReference(id))
	}
	return map[string]any{
		"id": question.ID, "projectId": tool.ProjectID, "toolId": tool.ID,
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"title": question.Title,
		"schedule": map[string]any{
			"rhythm": question.Rhythm, "weekdays": question.Weekdays, "timeOfDay": question.TimeOfDay, "startsOn": question.StartsOn,
		},
		"paused": question.Paused, "asked": asked, "appUrl": s.checkinQuestionURL(tool, question),
	}
}

func (s *Server) checkinAnswerLineResource(tool *CheckinsTool, question *CheckinQuestion, answer *CheckinAnswer) map[string]any {
	return map[string]any{
		"id": answer.ID, "questionId": question.ID, "groupOn": answer.GroupOn, "body": answer.Body,
		"commentCount": len(answer.Comments), "createdAt": timestamp(answer.CreatedAt), "editedAt": timestampOrNil(answer.EditedAt),
		"author": s.personReference(answer.AuthorID), "appUrl": s.checkinAnswerURL(tool, answer),
	}
}

func (s *Server) checkinAnswerResource(tool *CheckinsTool, question *CheckinQuestion, answer *CheckinAnswer) map[string]any {
	comments := []any{}
	for _, comment := range answer.Comments {
		comments = append(comments, s.commentResource(tool.ProjectID, comment))
	}
	return map[string]any{
		"id": answer.ID, "projectId": tool.ProjectID, "toolId": tool.ID, "questionId": question.ID,
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"question": map[string]any{"id": question.ID, "title": question.Title},
		"groupOn":  answer.GroupOn, "body": answer.Body, "bodySource": answer.Body,
		"attachments":  s.attachmentsIn(tool.ProjectID, answer.Body),
		"commentCount": len(answer.Comments), "createdAt": timestamp(answer.CreatedAt), "editedAt": timestampOrNil(answer.EditedAt),
		"author": s.personReference(answer.AuthorID), "comments": comments, "appUrl": s.checkinAnswerURL(tool, answer),
	}
}

func (s *Server) checkinsToolURL(tool *CheckinsTool) string {
	return s.URL + "/projects/" + tool.ProjectID + "/checkins/" + tool.ID
}

func (s *Server) checkinQuestionURL(tool *CheckinsTool, question *CheckinQuestion) string {
	return s.checkinsToolURL(tool) + "/questions/" + question.ID
}

func (s *Server) checkinAnswerURL(tool *CheckinsTool, answer *CheckinAnswer) string {
	return s.checkinsToolURL(tool) + "/answers/" + answer.ID
}
