package fakeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"time"
)

// defaultLeadDays is how many days before its day an occurrence shows where
// a new series names no lead of its own; maxLeadDays is the longest lead.
const (
	defaultLeadDays = 7
	maxLeadDays     = 366
)

// TodoSeries makes a to-do again and again on the days of its schedule. It
// is never deleted, only stopped; the occurrences it made stay ordinary
// to-dos that name it.
type TodoSeries struct {
	ID           string
	Title        string
	Notes        string // the HTML the occurrences start from
	GroupID      string // empty for occurrences standing in the list directly
	Schedule     RepetitionSchedule
	LeadDays     int
	LastOccursOn string // the day of the occurrence made last, empty before the first
	ResumedOn    string // the day it was resumed last, empty if never
	EndedAt      *time.Time
	AssigneeIDs  []string // in the order they were put on
	NotifiedIDs  []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CreatorID    string
}

func (s *Server) registerTodoSeriesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/todo-lists/{listId}/series", s.authenticated(s.todoSeriesOfList))
	mux.HandleFunc("POST /api/v1/todo-lists/{listId}/series", s.authenticated(s.repeatTodo))
	mux.HandleFunc("GET /api/v1/todo-series/{seriesId}", s.authenticated(s.todoSeries))
	mux.HandleFunc("PATCH /api/v1/todo-series/{seriesId}", s.authenticated(s.updateTodoSeries))
	mux.HandleFunc("POST /api/v1/todo-series/{seriesId}/stop", s.authenticated(s.stopTodoSeries))
	mux.HandleFunc("POST /api/v1/todo-series/{seriesId}/resume", s.authenticated(s.resumeTodoSeries))
	mux.HandleFunc("POST /api/v1/todo-series/{seriesId}/show-now", s.authenticated(s.showTodoSeriesNow))
}

// todoSeriesOfList lists every series of a list, stopped ones too, the one
// made first first.
func (s *Server) todoSeriesOfList(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	series := []any{}
	for _, one := range list.Series {
		series = append(series, s.todoSeriesResource(tool, list, one))
	}
	writeJSON(w, http.StatusOK, series)
}

// repeatTodo makes a to-do of the list the first occurrence of a new series,
// starting on its due day. The body is read in full before the list is looked
// for, the type of every field before any value; what the to-do and its days
// allow is asked after, and the archived project and list only once the write
// refused.
func (s *Server) repeatTodo(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBodyWith(w, r, true)
	if !ok {
		return
	}
	todoID, ok := stringField(w, fields, "todoId")
	if !ok {
		return
	}
	if _, ok := wholeNumberField(w, fields, "leadDays"); !ok {
		return
	}
	schedule, ok := scheduleShape(w, fields)
	if !ok {
		return
	}

	if !uuidPattern.MatchString(deref(todoID)) {
		todoToRepeatUnknown(w)
		return
	}
	leadDays, ok := seriesLeadDays(w, fields)
	if !ok {
		return
	}
	if leadDays == nil {
		usual := defaultLeadDays
		leadDays = &usual
	}
	if schedule == nil {
		schedule = &scheduleFields{}
	} else if !validScheduleValues(w, *schedule) {
		return
	}
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	todo := activeTodoOf(list, *todoID)
	if todo == nil {
		todoToRepeatUnknown(w)
		return
	}
	if todo.SeriesID != "" {
		todoRepeatsAlready(w)
		return
	}
	if todo.DueOn == nil {
		unprocessable(w, "todoSeriesNeedsDueDay", "A to-do repeats from its due day; it has none.")
		return
	}
	// The series starts on the due day, whatever startsOn the body names.
	repetition, ok := composedSchedule(w, *schedule, *todo.DueOn, nil)
	if !ok || s.refusedAsNothingWritten(w, tool, list) {
		return
	}

	series := &TodoSeries{
		ID: newID(), Title: todo.Title, Notes: todo.Notes, GroupID: todo.GroupID, Schedule: repetition, LeadDays: *leadDays,
		LastOccursOn: *todo.DueOn, AssigneeIDs: slices.Clone(todo.AssigneeIDs), NotifiedIDs: slices.Clone(todo.NotifiedIDs),
		CreatedAt: s.now, UpdatedAt: s.now, CreatorID: person.ID,
	}
	list.Series = append(list.Series, series)
	todo.SeriesID, todo.OccursOn = series.ID, *todo.DueOn
	w.Header().Set("Location", s.URL+"/api/v1/todo-series/"+series.ID)
	writeJSON(w, http.StatusCreated, s.todoSeriesResource(tool, list, series))
}

func (s *Server) todoSeries(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, series := s.visibleTodoSeries(r.PathValue("seriesId"), person)
	if series == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.todoSeriesResource(tool, list, series))
}

// updateTodoSeries changes what the occurrences made from now on start from.
// What the body leaves out stays, the people it names none of included. With
// "expectedUpdatedAt" a series written since is answered 409 with the series
// as it stands; without it the last write wins. A body that changes nothing
// writes nothing and meets no conflict. The body is checked in full before
// the series is looked for, the type of every field before any value, and the
// archived project and list, the guard and the people only once the write
// refused, in that order.
func (s *Server) updateTodoSeries(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBodyWith(w, r, true)
	if !ok {
		return
	}
	_, namesTitle := fields["title"]
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	assigneeIDs, ok := seriesPeopleShape(w, fields, "assigneeIds")
	if !ok {
		return
	}
	notifiedIDs, ok := seriesPeopleShape(w, fields, "notifiedIds")
	if !ok {
		return
	}
	schedule, ok := scheduleShape(w, fields)
	if !ok {
		return
	}
	if _, ok := wholeNumberField(w, fields, "leadDays"); !ok {
		return
	}
	expected, guarded, ok := expectedUpdatedAt(w, fields)
	if !ok {
		return
	}

	if namesTitle && !validTodoTitle(w, "todo", "a to-do", deref(title)) {
		return
	}
	if !seriesPeopleIdentifiers(w, assigneeIDs) || !seriesPeopleIdentifiers(w, notifiedIDs) {
		return
	}
	if schedule != nil && !validScheduleValues(w, *schedule) {
		return
	}
	leadDays, ok := seriesLeadDays(w, fields)
	if !ok {
		return
	}
	if title == nil && assigneeIDs == nil && notifiedIDs == nil && (schedule == nil || schedule.isEmpty()) && leadDays == nil {
		unprocessable(w, "nothingToChange", "The body names neither a title nor people nor a schedule nor lead days.")
		return
	}
	tool, list, series := s.visibleTodoSeries(r.PathValue("seriesId"), person)
	if series == nil {
		notFound(w)
		return
	}
	repetition := series.Schedule
	if schedule != nil && !schedule.isEmpty() {
		startsOn := series.Schedule.StartsOn
		if schedule.startsOn != nil {
			startsOn = *schedule.startsOn
		}
		if repetition, ok = composedSchedule(w, *schedule, startsOn, &series.Schedule); !ok {
			return
		}
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	newTitle, newLeadDays := series.Title, series.LeadDays
	if title != nil {
		newTitle = phpTrim(*title)
	}
	if leadDays != nil {
		newLeadDays = *leadDays
	}
	peopleChange := s.peopleChange(tool, list, assigneeIDs, series.AssigneeIDs) || s.peopleChange(tool, list, notifiedIDs, series.NotifiedIDs)
	if !peopleChange && newTitle == series.Title && newLeadDays == series.LeadDays && reflect.DeepEqual(repetition, series.Schedule) {
		writeJSON(w, http.StatusOK, s.todoSeriesResource(tool, list, series))
		return
	}
	if guarded && !expected.Equal(series.UpdatedAt) {
		refusal := errorBody("todoSeriesChanged", `This series was written over since it was read. Nothing was written; "series" holds it as it stands.`)
		refusal["series"] = s.todoSeriesResource(tool, list, series)
		writeJSON(w, http.StatusConflict, refusal)
		return
	}
	for _, ids := range [][]string{assigneeIDs, notifiedIDs} {
		for _, id := range ids {
			if !s.seesTheList(tool, list, id) {
				seriesPersonNotAMember(w)
				return
			}
		}
	}

	series.Title, series.Schedule, series.LeadDays = newTitle, repetition, newLeadDays
	// Named people are the whole set: whoever is stored and no longer seen
	// goes with such a write.
	if assigneeIDs != nil {
		series.AssigneeIDs = withoutRepeats(assigneeIDs)
	}
	if notifiedIDs != nil {
		series.NotifiedIDs = withoutRepeats(notifiedIDs)
	}
	series.UpdatedAt = s.aMomentAfter(series.UpdatedAt)
	writeJSON(w, http.StatusOK, s.todoSeriesResource(tool, list, series))
}

// stopTodoSeries ends a series; what it made stays. Why a stop wrote nothing
// is asked only after it refused: the project, the list, then the series
// being stopped already.
func (s *Server) stopTodoSeries(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, series := s.visibleTodoSeries(r.PathValue("seriesId"), person)
	if series == nil {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) || refusedAsStopped(w, series) {
		return
	}
	at := s.aMomentAfter(series.UpdatedAt)
	series.EndedAt, series.UpdatedAt = &at, at
	writeJSON(w, http.StatusOK, s.todoSeriesResource(tool, list, series))
}

// resumeTodoSeries lets a stopped series repeat again from its first day
// after today; the days it missed are not made up. A finished series is not
// stopped, and is refused like a running one.
func (s *Server) resumeTodoSeries(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, series := s.visibleTodoSeries(r.PathValue("seriesId"), person)
	if series == nil {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	if series.EndedAt == nil {
		unprocessable(w, "todoSeriesNotStopped", "This series is running; only a stopped one is resumed.")
		return
	}
	series.EndedAt, series.ResumedOn = nil, s.today()
	series.UpdatedAt = s.aMomentAfter(series.UpdatedAt)
	writeJSON(w, http.StatusOK, s.todoSeriesResource(tool, list, series))
}

// showTodoSeriesNow makes the upcoming occurrence at once, at the end of the
// list. The new to-do is named by Location alone; the body is the series.
func (s *Server) showTodoSeriesNow(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, series := s.visibleTodoSeries(r.PathValue("seriesId"), person)
	if series == nil {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) || refusedAsStopped(w, series) {
		return
	}
	occursOn, ok := series.Schedule.occurrenceToShowNow(series.LastOccursOn, series.ResumedOn, s.today())
	if !ok {
		if _, running := series.Schedule.upcomingOccurrence(series.LastOccursOn, series.ResumedOn, s.today()); !running {
			unprocessable(w, "todoSeriesFinished", "This series has no further day.")
			return
		}
		unprocessable(w, "todoSeriesOccurrenceAhead", "This series holds an occurrence not yet due; the next one comes after it.")
		return
	}

	groupID := ""
	if series.GroupID != "" && groupOf(list, series.GroupID) != nil {
		groupID = series.GroupID
	}
	due := occursOn
	todo := &Todo{
		ID: newID(), Title: series.Title, GroupID: groupID, Notes: series.Notes, DueOn: &due,
		AssigneeIDs: s.seeingTheList(tool, list, series.AssigneeIDs), NotifiedIDs: s.seeingTheList(tool, list, series.NotifiedIDs),
		CreatedAt: s.now, CreatorID: series.CreatorID, SeriesID: series.ID, OccursOn: occursOn,
	}
	list.Todos = append(list.Todos, todo)
	series.LastOccursOn = max(series.LastOccursOn, occursOn)
	series.UpdatedAt = s.aMomentAfter(series.UpdatedAt)
	w.Header().Set("Location", s.URL+"/api/v1/todos/"+todo.ID)
	writeJSON(w, http.StatusCreated, s.todoSeriesResource(tool, list, series))
}

func refusedAsStopped(w http.ResponseWriter, series *TodoSeries) bool {
	if series.EndedAt == nil {
		return false
	}
	unprocessable(w, "todoSeriesStopped", "This series is stopped; resume it first.")
	return true
}

// visibleTodoSeries finds a series through its list, which the person has to
// be able to read: whoever may not read the list finds nothing.
func (s *Server) visibleTodoSeries(seriesID string, person *Person) (*TodosTool, *TodoList, *TodoSeries) {
	if !uuidPattern.MatchString(seriesID) {
		return nil, nil, nil
	}
	for _, tool := range s.TodosTools {
		for _, list := range tool.Lists {
			for _, series := range list.Series {
				if series.ID != seriesID {
					continue
				}
				tool, list := s.visibleTodoList(list.ID, person)
				if list == nil {
					return nil, nil, nil
				}
				return tool, list, series
			}
		}
	}
	return nil, nil, nil
}

// activeTodoOf is a to-do of the list out of the trash, open or done, and in
// no group in the trash.
func activeTodoOf(list *TodoList, todoID string) *Todo {
	for _, todo := range countedTodos(list) {
		if todo.ID == todoID {
			return todo
		}
	}
	return nil
}

// peopleChange says whether the people named differ from those the series
// shows, taken once each and in their order. Nobody named is no change.
func (s *Server) peopleChange(tool *TodosTool, list *TodoList, named, stored []string) bool {
	return named != nil && !slices.Equal(withoutRepeats(named), s.seeingTheList(tool, list, stored))
}

// seesTheList says whether somebody is a member of the project who is shown
// the list: a client is not shown one hidden from the clients.
func (s *Server) seesTheList(tool *TodosTool, list *TodoList, personID string) bool {
	return slices.Contains(s.projectWithID(tool.ProjectID).MemberIDs, personID) && !(list.HiddenFromClients && s.isAClient(personID))
}

// seeingTheList are the people of a series who still see its list, in their
// order; the others stay on the series and show nowhere.
func (s *Server) seeingTheList(tool *TodosTool, list *TodoList, ids []string) []string {
	seeing := []string{}
	for _, id := range ids {
		if s.seesTheList(tool, list, id) {
			seeing = append(seeing, id)
		}
	}
	return seeing
}

// today is the installation's day, which the fake keeps in UTC.
func (s *Server) today() string {
	return s.now.UTC().Format(dayLayout)
}

// todoSeriesResource is a series with when its next occurrence comes, as
// counted on the installation's today. A list in the archive makes nothing
// until it is back, so it names no day to show and nothing to show now.
func (s *Server) todoSeriesResource(tool *TodosTool, list *TodoList, series *TodoSeries) map[string]any {
	today := s.today()
	schedule := series.Schedule
	stopped := series.EndedAt != nil
	state := "stopped"
	var nextOccursOn, nextShowsOn any
	canShowNow := false
	if !stopped {
		state = "finished"
		if next, ok := schedule.upcomingOccurrence(series.LastOccursOn, series.ResumedOn, today); ok {
			state, nextOccursOn = "running", next
			if showsFrom := schedule.showsFrom(next, series.LeadDays, series.LastOccursOn); showsFrom > today && !list.Archived {
				nextShowsOn = showsFrom
			}
		}
		_, showable := schedule.occurrenceToShowNow(series.LastOccursOn, series.ResumedOn, today)
		canShowNow = showable && !list.Archived
	}
	var group any
	groupID := ""
	if g := groupOf(list, series.GroupID); series.GroupID != "" && g != nil {
		group, groupID = map[string]any{"id": g.ID, "title": g.Title}, g.ID
	}
	return map[string]any{
		"id": series.ID, "projectId": tool.ProjectID, "toolId": tool.ID, "listId": list.ID, "groupId": nullable(groupID),
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"list": map[string]any{"id": list.ID, "title": list.Title}, "group": group,
		"title": series.Title, "notes": series.Notes, "schedule": scheduleResource(schedule), "leadDays": series.LeadDays,
		"state": state, "endedAt": timestampOrNil(series.EndedAt),
		"nextOccursOn": nextOccursOn, "nextShowsOn": nextShowsOn, "canShowNow": canShowNow,
		"assignees": s.peopleOnSeries(tool, list, series.AssigneeIDs), "notified": s.peopleOnSeries(tool, list, series.NotifiedIDs),
		"createdAt": timestamp(series.CreatedAt), "creator": s.personReference(series.CreatorID),
		"updatedAt": timestamp(series.UpdatedAt), "appUrl": s.todosToolURL(tool) + "/series/" + series.ID,
	}
}

func (s *Server) peopleOnSeries(tool *TodosTool, list *TodoList, ids []string) []any {
	people := []any{}
	for _, id := range s.seeingTheList(tool, list, ids) {
		people = append(people, s.personReference(id))
	}
	return people
}

// scheduleResource names each field a rhythm uses, and null or nothing for
// the others.
func scheduleResource(schedule RepetitionSchedule) map[string]any {
	weekdays := []int{}
	if schedule.Rhythm == RepeatsDaily || schedule.Rhythm == RepeatsWeekly {
		weekdays = schedule.Weekdays
	}
	var day, weekday, instance, month any
	switch schedule.Rhythm {
	case RepeatsMonthlyOnDay:
		day = schedule.DayOfMonth
	case RepeatsMonthlyOnWeekday:
		weekday, instance = schedule.Weekdays[0], schedule.Instance
	case RepeatsYearly:
		day, month = schedule.DayOfMonth, schedule.Month
	}
	return map[string]any{
		"rhythm": schedule.Rhythm, "weekdays": weekdays, "interval": schedule.Interval, "day": day, "weekday": weekday,
		"instance": instance, "month": month, "startsOn": schedule.StartsOn, "endsOn": nullable(schedule.EndsOn),
	}
}

// scheduleFields are the fields of a schedule a request sends, each checked
// on its own. What it leaves out comes from the stored schedule while rhythm
// and first day stay, else from the first day, the way the web form takes
// it; a rhythm ignores the fields it does not use.
type scheduleFields struct {
	rhythm                                  *string
	weekdays                                []int // nil where the request sends none
	interval, day, weekday, instance, month *int
	startsOn                                *string
	namesEndsOn                             bool // the request sends "endsOn", null included
	endsOn                                  *string
}

func (f scheduleFields) isEmpty() bool {
	return f.rhythm == nil && f.weekdays == nil && f.interval == nil && f.day == nil && f.weekday == nil &&
		f.instance == nil && f.month == nil && f.startsOn == nil && !f.namesEndsOn
}

func (f scheduleFields) endsOnWith(saved *RepetitionSchedule) string {
	if f.namesEndsOn {
		return deref(f.endsOn)
	}
	if saved != nil {
		return saved.EndsOn
	}
	return ""
}

// scheduleFrom builds the schedule the fields make from a first day, and an
// error where they make none, such as 31 February.
func (f scheduleFields) scheduleFrom(startsOn string, saved *RepetitionSchedule) (RepetitionSchedule, error) {
	rhythm := RepeatsWeekly
	switch {
	case f.rhythm != nil:
		rhythm = *f.rhythm
	case saved != nil:
		rhythm = saved.Rhythm
	}
	firstDay, ok := midnightOf(startsOn)
	if !ok {
		return RepetitionSchedule{}, errNoSchedule
	}
	endsOn := f.endsOnWith(saved)
	sameRhythm := saved != nil && saved.Rhythm == rhythm
	keepsDetails := sameRhythm && saved.StartsOn == startsOn
	interval := 1
	if sameRhythm {
		interval = saved.Interval
	}
	if f.interval != nil {
		interval = *f.interval
	}
	dayOfMonth := firstDay.Day()
	weekdayOfFirstDay := isoWeekday(firstDay)
	pick := func(sent *int, kept bool, stored, derived int) int {
		switch {
		case sent != nil:
			return *sent
		case kept:
			return stored
		}
		return derived
	}

	switch rhythm {
	case RepeatsDaily:
		weekdays := []int{1, 2, 3, 4, 5}
		if sameRhythm {
			weekdays = saved.Weekdays
		}
		if f.weekdays != nil {
			weekdays = f.weekdays
		}
		return dailySchedule(weekdays, startsOn, endsOn)
	case RepeatsWeekly:
		weekdays := []int{weekdayOfFirstDay}
		if sameRhythm {
			weekdays = saved.Weekdays
		}
		if f.weekdays != nil {
			weekdays = f.weekdays
		}
		return weeklySchedule(weekdays, interval, startsOn, endsOn)
	case RepeatsMonthlyOnDay:
		return monthlyOnDaySchedule(pick(f.day, keepsDetails, saved.dayOfMonthOrZero(), dayOfMonth), interval, startsOn, endsOn)
	case RepeatsMonthlyOnWeekday:
		// A day after the 28th is the last of its weekday in the month.
		instance := LastWeekdayInstance
		if dayOfMonth <= 28 {
			instance = (dayOfMonth-1)/7 + 1
		}
		weekday := pick(f.weekday, keepsDetails, saved.firstWeekdayOrZero(), weekdayOfFirstDay)
		return monthlyOnWeekdaySchedule(weekday, pick(f.instance, keepsDetails, saved.instanceOrZero(), instance), interval, startsOn, endsOn)
	case RepeatsYearly:
		month := pick(f.month, keepsDetails, saved.monthOrZero(), int(firstDay.Month()))
		return yearlySchedule(month, pick(f.day, keepsDetails, saved.dayOfMonthOrZero(), dayOfMonth), startsOn, endsOn)
	}
	return RepetitionSchedule{}, errNoSchedule
}

func (r *RepetitionSchedule) dayOfMonthOrZero() int {
	if r == nil {
		return 0
	}
	return r.DayOfMonth
}

func (r *RepetitionSchedule) firstWeekdayOrZero() int {
	if r == nil || len(r.Weekdays) == 0 {
		return 0
	}
	return r.Weekdays[0]
}

func (r *RepetitionSchedule) instanceOrZero() int {
	if r == nil {
		return 0
	}
	return r.Instance
}

func (r *RepetitionSchedule) monthOrZero() int {
	if r == nil {
		return 0
	}
	return r.Month
}

// composedSchedule is the schedule the fields make from a first day: an end
// before that day is out of order, and fields that make no schedule are
// invalid.
func composedSchedule(w http.ResponseWriter, fields scheduleFields, startsOn string, saved *RepetitionSchedule) (RepetitionSchedule, bool) {
	if endsOn := fields.endsOnWith(saved); endsOn != "" && endsOn < startsOn {
		unprocessable(w, "todoSeriesDatesOutOfOrder", "A series ends on or after the day it starts.")
		return RepetitionSchedule{}, false
	}
	schedule, err := fields.scheduleFrom(startsOn, saved)
	if err != nil {
		todoSeriesScheduleInvalid(w)
		return RepetitionSchedule{}, false
	}
	return schedule, true
}

// scheduleShape reads the type of every field of "schedule", the way the
// server does before it looks at any value; nil is a body without a schedule.
// Null inside counts as left out, except in "endsOn", where it takes the end
// off.
func scheduleShape(w http.ResponseWriter, fields map[string]any) (*scheduleFields, bool) {
	value, present := fields["schedule"]
	if !present {
		return nil, true
	}
	schedule, isObject := value.(map[string]any)
	if !isObject {
		malformed(w, `The field "schedule" has to be an object.`)
		return nil, false
	}
	var f scheduleFields
	var ok bool
	if f.rhythm, ok = stringField(w, schedule, "rhythm"); !ok {
		return nil, false
	}
	if f.startsOn, ok = stringField(w, schedule, "startsOn"); !ok {
		return nil, false
	}
	if f.endsOn, ok = stringField(w, schedule, "endsOn"); !ok {
		return nil, false
	}
	for _, field := range []struct {
		name  string
		value **int
	}{{"interval", &f.interval}, {"day", &f.day}, {"month", &f.month}, {"weekday", &f.weekday}, {"instance", &f.instance}} {
		if *field.value, ok = wholeNumberField(w, schedule, field.name); !ok {
			return nil, false
		}
	}
	if f.weekdays, ok = wholeNumbersField(w, schedule, "weekdays"); !ok {
		return nil, false
	}
	_, f.namesEndsOn = schedule["endsOn"]
	if deref(f.endsOn) == "" {
		f.endsOn = nil
	}
	return &f, true
}

// validScheduleValues holds each field of a schedule on its own, in the order
// the server does: the days, then the ranges, then the names of rhythm,
// weekdays and instance.
func validScheduleValues(w http.ResponseWriter, f scheduleFields) bool {
	for _, day := range []*string{f.startsOn, f.endsOn} {
		if _, isADay := midnightOf(deref(day)); day != nil && !isADay {
			unprocessable(w, "todoSeriesDatesNotADate", "A day is written as YYYY-MM-DD.")
			return false
		}
	}
	if outside(f.interval, 1, maxRepetitionInterval) || outside(f.day, 1, 31) || outside(f.month, 1, 12) {
		todoSeriesScheduleInvalid(w)
		return false
	}
	if f.rhythm != nil && !slices.Contains([]string{RepeatsDaily, RepeatsWeekly, RepeatsMonthlyOnDay, RepeatsMonthlyOnWeekday, RepeatsYearly}, *f.rhythm) {
		todoSeriesScheduleInvalid(w)
		return false
	}
	for _, weekday := range f.weekdays {
		if weekday < 1 || weekday > 7 {
			todoSeriesScheduleInvalid(w)
			return false
		}
	}
	if outside(f.weekday, 1, 7) || f.instance != nil && !slices.Contains([]int{1, 2, 3, 4, LastWeekdayInstance}, *f.instance) {
		todoSeriesScheduleInvalid(w)
		return false
	}
	return true
}

func outside(value *int, lowest, highest int) bool {
	return value != nil && (*value < lowest || *value > highest)
}

// seriesLeadDays reads "leadDays": nil where the body leaves it out, and null
// in it no number of days at all.
func seriesLeadDays(w http.ResponseWriter, fields map[string]any) (*int, bool) {
	if _, present := fields["leadDays"]; !present {
		return nil, true
	}
	leadDays, ok := wholeNumberField(w, fields, "leadDays")
	if !ok {
		return nil, false
	}
	if leadDays == nil || *leadDays < 0 || *leadDays > maxLeadDays {
		unprocessable(w, "todoSeriesLeadDaysInvalid", "An occurrence shows 0 to 366 days before its day.")
		return nil, false
	}
	return leadDays, true
}

// wholeNumberField reads a number the server takes as a whole one: written
// without a fraction or an exponent. Null counts as left out.
func wholeNumberField(w http.ResponseWriter, fields map[string]any, name string) (*int, bool) {
	value, present := fields[name]
	if !present || value == nil {
		return nil, true
	}
	number, ok := wholeNumber(value)
	if !ok {
		malformed(w, fmt.Sprintf("The field %q has to be a whole number.", name))
		return nil, false
	}
	return &number, true
}

func wholeNumbersField(w http.ResponseWriter, fields map[string]any, name string) ([]int, bool) {
	value, present := fields[name]
	if !present || value == nil {
		return nil, true
	}
	list, isList := value.([]any)
	if !isList {
		malformed(w, fmt.Sprintf("The field %q has to be a list of whole numbers.", name))
		return nil, false
	}
	numbers := make([]int, 0, len(list))
	for _, entry := range list {
		number, ok := wholeNumber(entry)
		if !ok {
			malformed(w, fmt.Sprintf("The field %q has to be a list of whole numbers.", name))
			return nil, false
		}
		numbers = append(numbers, number)
	}
	return numbers, true
}

func wholeNumber(value any) (int, bool) {
	written, isNumber := value.(json.Number)
	if !isNumber {
		return 0, false
	}
	number, err := strconv.Atoi(string(written))
	return number, err == nil
}

// seriesPeopleShape reads a list of the people of a series: nil where the
// body leaves it out, and null in it no list at all.
func seriesPeopleShape(w http.ResponseWriter, fields map[string]any, name string) ([]string, bool) {
	value, present := fields[name]
	if !present {
		return nil, true
	}
	list, isList := value.([]any)
	if !isList {
		malformed(w, fmt.Sprintf("The field %q has to be a list of strings.", name))
		return nil, false
	}
	ids := make([]string, 0, len(list))
	for _, entry := range list {
		id, isText := entry.(string)
		if !isText {
			malformed(w, fmt.Sprintf("The field %q has to be a list of strings.", name))
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

// seriesPeopleIdentifiers refuses an identifier that is none the way a
// stranger is.
func seriesPeopleIdentifiers(w http.ResponseWriter, ids []string) bool {
	for _, id := range ids {
		if !uuidPattern.MatchString(id) {
			seriesPersonNotAMember(w)
			return false
		}
	}
	return true
}

func todoToRepeatUnknown(w http.ResponseWriter) {
	unprocessable(w, "todoToRepeatUnknown", `The to-do to repeat, named by "todoId", is not an active to-do of this list.`)
}

func todoRepeatsAlready(w http.ResponseWriter) {
	unprocessable(w, "todoRepeatsAlready", "This to-do is an occurrence of a series already.")
}

func todoSeriesScheduleInvalid(w http.ResponseWriter) {
	unprocessable(w, "todoSeriesScheduleInvalid", "The rhythm is daily, weekly, monthlyOnDay, monthlyOnWeekday or yearly; weekdays are numbers from 1 (Monday) to 7, the day of a month 1 to 31, the instance 1 to 4 or -1 for the last, the month 1 to 12 and the interval 1 to 99.")
}

func seriesPersonNotAMember(w http.ResponseWriter) {
	unprocessable(w, "personNotAProjectMember", "Only a member of the project who sees the list can be named on a series.")
}
