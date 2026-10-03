package fakeapi

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // the zones of a series, wherever the tests run
	"unicode/utf8"
)

// What the server keeps of an event, and the longest span it lists.
const (
	maxEventDescriptionLength = 100000
	maxJoinURLLength          = 2000
	longestSpanInDays         = 366
)

// eventTimeFields are the four fields that name the time of an event; a
// change that names one of them names the whole time anew.
var eventTimeFields = []string{"startsAt", "endsAt", "firstDay", "lastDay"}

// eventMoment is a moment as the server reads one: ISO 8601 with seconds, a
// fraction of up to six digits and an offset of at most fourteen hours.
var eventMoment = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,6}))?(Z|([+-])((?:0\d|1[0-3])):?([0-5]\d)|([+-])14:?00)$`)

// zoneRegions are the regions whose zones PHP lists, beside UTC. A name of
// the backward table, such as CET or US/Eastern, is not listed there.
var zoneRegions = []string{"Africa/", "America/", "Antarctica/", "Arctic/", "Asia/", "Atlantic/", "Australia/", "Europe/", "Indian/", "Pacific/"}

// eventRefusals are the codes of a schedule of an event.
var eventRefusals = scheduleRefusals{prefix: "eventRepetition", outOfOrder: "A series ends on or after the first day of its event."}

// CalendarTool is a calendar of a project with its events, in the order
// they were added.
type CalendarTool struct {
	ID        string
	ProjectID string
	Title     string

	// Archived is a calendar whose status is no longer active: the project
	// names it among its tools no more, and at its own address it is read
	// and written as before.
	Archived bool

	// ShownToClients lets the clients of the project into the calendar.
	ShownToClients bool

	Events []*CalendarEvent
}

// CalendarEvent is one event. A timed one has two moments, an all-day one a
// first and a last day; a series is one event with a repetition.
type CalendarEvent struct {
	ID       string
	Title    string
	StartsAt *time.Time // in UTC, nil for an all-day event
	EndsAt   *time.Time
	FirstDay string // YYYY-MM-DD, empty for a timed event
	LastDay  string

	Repetition  *EventRepetition // nil for an event that does not repeat
	Description string           // the HTML as stored
	JoinURL     string           // empty for none
	Highlighted bool

	// ParticipantIDs are in the order they were put on.
	ParticipantIDs []string

	// ShownToClients is true for an event a client added, and false for one
	// of the team; the API changes it for neither.
	ShownToClients bool
	Trashed        bool

	// CommentCount stands for comments the API neither reads nor writes on
	// an event.
	CommentCount int
	CreatedAt    time.Time
	CreatorID    string
}

// EventRepetition is how an event repeats: its schedule, and for a timed
// event the zone it keeps its clock time in.
type EventRepetition struct {
	Schedule RepetitionSchedule
	TimeZone string // empty for an all-day series
}

// eventTime is the time of an event or of one occurrence of it.
type eventTime struct {
	startsAt, endsAt  *time.Time
	firstDay, lastDay string
}

func (t eventTime) allDay() bool {
	return t.firstDay != ""
}

func (e *CalendarEvent) time() eventTime {
	return eventTime{startsAt: e.StartsAt, endsAt: e.EndsAt, firstDay: e.FirstDay, lastDay: e.LastDay}
}

func (s *Server) registerCalendarRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectId}/calendars", s.authenticated(s.calendars))
	mux.HandleFunc("GET /api/v1/calendars/{toolId}", s.authenticated(s.calendar))
	mux.HandleFunc("GET /api/v1/calendars/{toolId}/events", s.authenticated(s.eventsInSpan))
	mux.HandleFunc("POST /api/v1/calendars/{toolId}/events", s.authenticated(s.addEvent))
	mux.HandleFunc("GET /api/v1/events/{eventId}", s.authenticated(s.event))
	mux.HandleFunc("PATCH /api/v1/events/{eventId}", s.authenticated(s.updateEvent))
	mux.HandleFunc("DELETE /api/v1/events/{eventId}", s.authenticated(s.removeEvent))
}

// calendars lists the calendars of a project that are in use, in the order
// of the project page.
func (s *Server) calendars(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	calendars := []any{}
	for _, tool := range s.Calendars {
		if tool.ProjectID == project.ID && !tool.Archived && (tool.ShownToClients || !s.isAClient(person.ID)) {
			calendars = append(calendars, map[string]any{"id": tool.ID, "title": tool.Title, "appUrl": s.calendarURL(tool)})
		}
	}
	writeJSON(w, http.StatusOK, calendars)
}

func (s *Server) calendar(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleCalendar(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.toolResource(tool.ID, tool.ProjectID, tool.Title, s.calendarURL(tool)))
}

// eventsInSpan lists every event with an occurrence between two days of the
// reader's zone, both included: each once, with the times it takes place at
// in the span, ordered by the first of them and then by when it was added.
func (s *Server) eventsInSpan(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleCalendar(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	from, fromIsADay := dayOf(r.URL.Query().Get("from"))
	to, toIsADay := dayOf(r.URL.Query().Get("to"))
	if !fromIsADay || !toIsADay {
		unprocessable(w, "eventsSpanNotADay", `A span is named by "from" and "to", each a day written as YYYY-MM-DD.`)
		return
	}
	if to < from {
		unprocessable(w, "eventsSpanOutOfOrder", `The span ends on or after the day it starts: "to" must not lie before "from".`)
		return
	}
	if daysBetween(from, to) >= longestSpanInDays {
		unprocessable(w, "eventsSpanTooLong", "A span holds at most 366 days.")
		return
	}

	zoneName, zone := s.zoneOf(person)
	type inSpan struct {
		event       *CalendarEvent
		occurrences []eventTime
	}
	var listed []inSpan
	for _, event := range tool.Events {
		if !s.seesEvent(tool, event, person) {
			continue
		}
		if occurrences := occurrencesBetween(event, from, to, zone); len(occurrences) > 0 {
			listed = append(listed, inSpan{event, occurrences})
		}
	}
	slices.SortStableFunc(listed, func(a, b inSpan) int {
		return startOf(a.occurrences[0]).Compare(startOf(b.occurrences[0]))
	})
	events := []any{}
	for _, one := range listed {
		events = append(events, s.eventLineResource(tool, one.event, one.occurrences))
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "timeZone": zoneName, "events": events})
}

func (s *Server) event(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, event := s.visibleEvent(r.PathValue("eventId"), person)
	if event == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.eventResource(tool, event))
}

// addEvent adds an event to a calendar, refusing in the order the controller
// does: the calendar, the body and the type of every field, each value, and
// what the write meets last.
func (s *Server) addEvent(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleCalendar(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	body, ok := readBodyWith(w, r, true)
	if !ok {
		return
	}
	fields, ok := readEventFields(w, body)
	if !ok {
		return
	}

	if !validTodoTitle(w, "event", "an event", deref(fields.title)) {
		return
	}
	when, ok := readEventTime(w, fields)
	if !ok {
		return
	}
	joinURL, ok := readJoinURL(w, deref(fields.joinURL))
	if !ok || !validParticipantIDs(w, fields.participantIDs) {
		return
	}
	var repetition *EventRepetition
	if fields.schedule != nil {
		if !validScheduleValues(w, eventRefusals, *fields.schedule) {
			return
		}
		if repetition, ok = s.readRepetition(w, person, *fields.schedule, fields.timeZone, when, nil); !ok {
			return
		}
	}
	description, ok := storedRichText(w, "eventDescriptionTooLong", "The description of an event is too long.", maxEventDescriptionLength, deref(fields.description))
	if !ok || s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}

	event := &CalendarEvent{
		ID: newID(), Title: phpTrim(deref(fields.title)), StartsAt: when.startsAt, EndsAt: when.endsAt, FirstDay: when.firstDay, LastDay: when.lastDay,
		Repetition: repetition, Description: description, JoinURL: joinURL, Highlighted: fields.highlighted != nil && *fields.highlighted,
		ShownToClients: s.isAClient(person.ID), CreatedAt: s.now, CreatorID: person.ID,
	}
	participants, ok := s.participantsAfter(w, tool, event, fields.participantIDs)
	if !ok {
		return
	}
	event.ParticipantIDs = participants
	tool.Events = append(tool.Events, event)
	w.Header().Set("Location", s.URL+"/api/v1/events/"+event.ID)
	writeJSON(w, http.StatusCreated, s.eventResource(tool, event))
}

// updateEvent changes what the body names; the rest stays. The four fields
// of the time are read together, and the repetition follows what the body
// says of "schedule": nothing keeps it, null stops it, an object sets it.
// There is no guard: the last write wins.
func (s *Server) updateEvent(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, event := s.visibleEvent(r.PathValue("eventId"), person)
	if event == nil {
		notFound(w)
		return
	}
	body, ok := readBodyWith(w, r, true)
	if !ok {
		return
	}
	fields, ok := readEventFields(w, body)
	if !ok {
		return
	}
	if fields.title == nil && fields.description == nil && !fields.namesJoinURL && fields.highlighted == nil &&
		fields.participantIDs == nil && !fields.namesTime && !fields.namesSchedule {
		unprocessable(w, "nothingToChange", "The body names neither a title nor a time nor a schedule nor a description nor a link nor people nor the highlight.")
		return
	}

	if fields.title != nil && !validTodoTitle(w, "event", "an event", *fields.title) {
		return
	}
	when := event.time()
	if fields.namesTime {
		if when, ok = readEventTime(w, fields); !ok {
			return
		}
	}
	joinURL, ok := readJoinURL(w, event.JoinURL)
	if fields.namesJoinURL {
		joinURL, ok = readJoinURL(w, deref(fields.joinURL))
	}
	if !ok || !validParticipantIDs(w, fields.participantIDs) {
		return
	}
	repetition := event.Repetition
	switch {
	case !fields.namesSchedule:
		if repetition != nil && !repetition.fits(when) {
			unprocessable(w, "eventRepetitionKeepsItsFirstDay", `A series keeps its first day and whether it is all-day; to move it, send its "schedule" along.`)
			return
		}
	case fields.schedule == nil:
		repetition = nil
	default:
		if !validScheduleValues(w, eventRefusals, *fields.schedule) {
			return
		}
		if repetition, ok = s.readRepetition(w, person, *fields.schedule, fields.timeZone, when, event.Repetition); !ok {
			return
		}
	}
	description := event.Description
	if fields.description != nil {
		if description, ok = storedRichText(w, "eventDescriptionTooLong", "The description of an event is too long.", maxEventDescriptionLength, *fields.description); !ok {
			return
		}
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	participants := event.ParticipantIDs
	if fields.participantIDs != nil {
		if participants, ok = s.participantsAfter(w, tool, event, fields.participantIDs); !ok {
			return
		}
	}

	if fields.title != nil {
		event.Title = phpTrim(*fields.title)
	}
	event.StartsAt, event.EndsAt, event.FirstDay, event.LastDay = when.startsAt, when.endsAt, when.firstDay, when.lastDay
	event.Repetition, event.Description, event.JoinURL, event.ParticipantIDs = repetition, description, joinURL, participants
	if fields.highlighted != nil {
		event.Highlighted = *fields.highlighted
	}
	writeJSON(w, http.StatusOK, s.eventResource(tool, event))
}

// removeEvent puts an event into the trash, a series with all its days. A
// client puts away only an event of their own; anybody else's answers like
// one that is not there, once the archived project has been asked.
func (s *Server) removeEvent(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, event := s.visibleEvent(r.PathValue("eventId"), person)
	if event == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if s.isAClient(person.ID) && event.CreatorID != person.ID {
		notFound(w)
		return
	}
	event.Trashed = true
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// eventFields are the fields of a body about an event, each of the type it
// has to be. Null counts as left out, except where noted.
type eventFields struct {
	title, description, joinURL, timeZone *string
	namesJoinURL                          bool // "joinUrl" is there, null included, which takes the link off
	highlighted                           *bool
	participantIDs                        []string // nil where the body names nobody, empty for nobody
	startsAt, endsAt, firstDay, lastDay   *string
	namesTime                             bool // one of the four fields is there, null included
	namesSchedule                         bool // "schedule" is there, null included, which stops the repetition
	schedule                              *scheduleFields
}

// readEventFields reads the type of every field, in the order the server
// does, before any value is looked at.
func readEventFields(w http.ResponseWriter, body map[string]any) (eventFields, bool) {
	var f eventFields
	var ok bool
	for _, field := range []struct {
		name  string
		value **string
	}{{"title", &f.title}, {"description", &f.description}, {"joinUrl", &f.joinURL}} {
		if *field.value, ok = stringField(w, body, field.name); !ok {
			return f, false
		}
	}
	_, f.namesJoinURL = body["joinUrl"]
	if f.highlighted, ok = booleanField(w, body, "highlighted"); !ok {
		return f, false
	}
	if f.participantIDs, ok = participantIDsField(w, body); !ok {
		return f, false
	}
	if f.timeZone, ok = stringField(w, body, "timeZone"); !ok {
		return f, false
	}
	for _, field := range []struct {
		name  string
		value **string
	}{{"startsAt", &f.startsAt}, {"endsAt", &f.endsAt}, {"firstDay", &f.firstDay}, {"lastDay", &f.lastDay}} {
		if *field.value, ok = stringField(w, body, field.name); !ok {
			return f, false
		}
		if _, named := body[field.name]; named {
			f.namesTime = true
		}
	}
	_, f.namesSchedule = body["schedule"]
	if body["schedule"] != nil {
		if f.schedule, ok = scheduleShape(w, body); !ok {
			return f, false
		}
	}
	return f, true
}

func participantIDsField(w http.ResponseWriter, body map[string]any) ([]string, bool) {
	value, present := body["participantIds"]
	if !present || value == nil {
		return nil, true
	}
	list, isList := value.([]any)
	if !isList {
		malformed(w, `The field "participantIds" has to be a list of strings.`)
		return nil, false
	}
	ids := make([]string, 0, len(list))
	for _, entry := range list {
		id, isText := entry.(string)
		if !isText {
			malformed(w, `The field "participantIds" has to be a list of strings.`)
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

// readEventTime reads two moments as a timed event and a first day as an
// all-day one, which lasts that one day unless a last day is named.
func readEventTime(w http.ResponseWriter, f eventFields) (eventTime, bool) {
	namesMoments := f.startsAt != nil || f.endsAt != nil
	namesDays := f.firstDay != nil || f.lastDay != nil
	if namesMoments && namesDays {
		unprocessable(w, "eventTimeAmbiguous", `An event is either timed, with "startsAt" and "endsAt", or all-day, with "firstDay" and "lastDay", not both.`)
		return eventTime{}, false
	}
	if namesMoments && (f.startsAt == nil || f.endsAt == nil) || !namesMoments && f.firstDay == nil {
		unprocessable(w, "eventTimeMissing", `An event needs "startsAt" and "endsAt", or "firstDay" for an all-day one.`)
		return eventTime{}, false
	}
	if f.firstDay != nil {
		first, isADay := dayOf(*f.firstDay)
		last, lastIsADay := dayOf(deref(f.lastDay))
		if f.lastDay == nil {
			last, lastIsADay = first, isADay
		}
		if !isADay || !lastIsADay {
			eventTimeNotReadable(w)
			return eventTime{}, false
		}
		if last < first {
			eventEndsBeforeItStarts(w)
			return eventTime{}, false
		}
		return eventTime{firstDay: first, lastDay: last}, true
	}
	startsAt, startIsAMoment := momentOf(*f.startsAt)
	endsAt, endIsAMoment := momentOf(*f.endsAt)
	if !startIsAMoment || !endIsAMoment {
		eventTimeNotReadable(w)
		return eventTime{}, false
	}
	if endsAt.Before(startsAt) {
		eventEndsBeforeItStarts(w)
		return eventTime{}, false
	}
	if startsAt.Year() < 1 {
		eventTimeNotReadable(w)
		return eventTime{}, false
	}
	return eventTime{startsAt: &startsAt, endsAt: &endsAt}, true
}

// momentOf reads a moment that says its offset, as there is no zone to
// assume for it, and gives it in UTC. A day a month lacks is none.
func momentOf(value string) (time.Time, bool) {
	parts := eventMoment.FindStringSubmatch(value)
	if parts == nil {
		return time.Time{}, false
	}
	number := func(i int) int {
		n, _ := strconv.Atoi(parts[i])
		return n
	}
	offset := 0
	switch {
	case parts[12] != "":
		offset = 14 * 3600
		if parts[12] == "-" {
			offset = -offset
		}
	case parts[9] != "":
		offset = number(10)*3600 + number(11)*60
		if parts[9] == "-" {
			offset = -offset
		}
	}
	fraction := parts[7] + strings.Repeat("0", 9-len(parts[7]))
	nanoseconds, _ := strconv.Atoi(fraction)
	moment := time.Date(number(1), time.Month(number(2)), number(3), number(4), number(5), number(6), nanoseconds, time.FixedZone("", offset))
	if moment.Format("2006-01-02T15:04:05") != value[:19] {
		return time.Time{}, false
	}
	return moment.UTC(), true
}

// dayOf reads a day the way the server does: trimmed, written as
// YYYY-MM-DD, existing, and in the year 1 or later.
func dayOf(value string) (string, bool) {
	value = phpTrim(value)
	date, ok := midnightOf(value)
	return value, ok && date.Year() >= 1
}

// readJoinURL reads the link of an event: empty for none, else an address of
// the web. The server holds it against PHP's check of a URL, which this
// approaches by a host it demands.
func readJoinURL(w http.ResponseWriter, value string) (string, bool) {
	value = phpTrim(value)
	if value == "" {
		return "", true
	}
	address, err := url.Parse(value)
	if utf8.RuneCountInString(value) > maxJoinURLLength || strings.ContainsFunc(value, func(r rune) bool { return r <= ' ' || r == 0x7f }) ||
		err != nil || address.Host == "" || !slices.Contains([]string{"http", "https"}, strings.ToLower(address.Scheme)) {
		unprocessable(w, "eventJoinUrlNotAnAddress", "The link of an event is a web address starting with http:// or https://.")
		return "", false
	}
	return value, true
}

// validParticipantIDs refuses an identifier that is none the way a stranger
// is, before anything else of the people is asked.
func validParticipantIDs(w http.ResponseWriter, ids []string) bool {
	for _, id := range ids {
		if !uuidPattern.MatchString(id) {
			participantNotAMember(w)
			return false
		}
	}
	return true
}

// readRepetition makes the repetition a schedule gives an event from its
// first day on: for a timed event that day in the zone the series keeps its
// clock time in, which the body names, or the series has, or is the
// writer's own.
func (s *Server) readRepetition(w http.ResponseWriter, writer *Person, fields scheduleFields, timeZone *string, when eventTime, saved *EventRepetition) (*EventRepetition, bool) {
	zone := ""
	if !when.allDay() {
		var ok bool
		if zone, ok = s.zoneOfARepetition(w, writer, timeZone, saved); !ok {
			return nil, false
		}
	}
	var savedSchedule *RepetitionSchedule
	if saved != nil {
		savedSchedule = &saved.Schedule
	}
	schedule, ok := composedSchedule(w, eventRefusals, fields, firstDayIn(when, zone), savedSchedule)
	if !ok {
		return nil, false
	}
	repetition := &EventRepetition{Schedule: schedule, TimeZone: zone}
	if !repetition.fits(when) {
		unprocessable(w, "eventRepetitionDoesNotFit", "The schedule does not fall on the first day of the event, such as a weekly one on other weekdays.")
		return nil, false
	}
	return repetition, true
}

func (s *Server) zoneOfARepetition(w http.ResponseWriter, writer *Person, named *string, saved *EventRepetition) (string, bool) {
	if named == nil && saved != nil && saved.TimeZone != "" {
		return saved.TimeZone, true
	}
	name := phpTrim(deref(named))
	if named == nil {
		name, _ = s.zoneOf(writer)
	}
	if !isAListedZone(name) {
		unprocessable(w, "eventRepetitionTimeZoneUnknown", `A timed series keeps its clock time in an IANA time zone such as "Europe/Berlin", named by "timeZone".`)
		return "", false
	}
	return name, true
}

// isAListedZone says whether PHP lists a zone of this name. What the fake
// knows of the list is its regions; a name of the backward table inside one,
// such as Europe/Kiev, passes here and is refused there.
func isAListedZone(name string) bool {
	if _, err := time.LoadLocation(name); err != nil || name == "" || name == "Local" || !spelledAsStored(name) {
		return false
	}
	return name == "UTC" || slices.ContainsFunc(zoneRegions, func(region string) bool { return strings.HasPrefix(name, region) })
}

// spelledAsStored says whether a zone is written in the capitals of its file
// in the zone database of the system. PHP holds a name to its list exactly,
// and a file system that ignores case loads Europe/berlin all the same. A
// system without the database loads from the copy built in, which is exact.
func spelledAsStored(name string) bool {
	dir := "/usr/share/zoneinfo"
	if _, err := os.Stat(dir); err != nil {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		entries, err := os.ReadDir(dir)
		if err != nil || !slices.ContainsFunc(entries, func(entry os.DirEntry) bool { return entry.Name() == part }) {
			return false
		}
		dir = filepath.Join(dir, part)
	}
	return true
}

// fits says whether the repetition starts on the first day of the event and
// is of its kind: a timed event repeats in a zone, an all-day one in none.
func (r *EventRepetition) fits(when eventTime) bool {
	if when.allDay() != (r.TimeZone == "") {
		return false
	}
	firstDay := firstDayIn(when, r.TimeZone)
	return r.Schedule.StartsOn == firstDay && r.Schedule.fallsOn(firstDay)
}

func (r RepetitionSchedule) fallsOn(day string) bool {
	next, ok := r.nextOccurrenceAfter(addDays(day, -1))
	return ok && next == day
}

// daysBetween is how many days the second day lies after the first.
func daysBetween(first, last string) int {
	a, _ := midnightOf(first)
	b, _ := midnightOf(last)
	return int((b.Unix() - a.Unix()) / 86400)
}

// firstDayIn is the day an event starts on: its first day, or the day of its
// start in the zone given.
func firstDayIn(when eventTime, zone string) string {
	if when.allDay() {
		return when.firstDay
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		location = time.UTC
	}
	return when.startsAt.In(location).Format(dayLayout)
}

// participantsAfter is who takes part once the people named are the whole
// set: those on it already keep their place, the others follow in the order
// named. Each newcomer is asked whether they belong to the project and then
// whether the event is shown to them.
func (s *Server) participantsAfter(w http.ResponseWriter, tool *CalendarTool, event *CalendarEvent, named []string) ([]string, bool) {
	project := s.projectWithID(tool.ProjectID)
	var participants []string
	for _, id := range event.ParticipantIDs {
		if slices.Contains(named, id) {
			participants = append(participants, id)
		}
	}
	for _, id := range named {
		if slices.Contains(participants, id) {
			continue
		}
		if !slices.Contains(project.MemberIDs, id) {
			participantNotAMember(w)
			return nil, false
		}
		if s.isAClient(id) && !(tool.ShownToClients && event.ShownToClients) {
			unprocessable(w, "personCannotSeeIt", "Somebody chosen is a client of the project, and this event is not shown to clients.")
			return nil, false
		}
		participants = append(participants, id)
	}
	if participants == nil {
		participants = []string{}
	}
	return participants, true
}

// occurrencesBetween are the times an event takes place at between two days,
// in the order they start. The days are plain days for an all-day event and
// the reader's for a timed one, so one that started the evening before
// reaches in.
func occurrencesBetween(event *CalendarEvent, from, to string, readerZone *time.Location) []eventTime {
	when := event.time()
	if when.allDay() {
		if event.Repetition == nil {
			if when.firstDay <= to && when.lastDay >= from {
				return []eventTime{when}
			}
			return nil
		}
		further := daysBetween(when.firstDay, when.lastDay)
		var occurrences []eventTime
		for _, day := range event.Repetition.Schedule.daysBetween(addDays(from, -further), to) {
			occurrences = append(occurrences, eventTime{firstDay: day, lastDay: addDays(day, further)})
		}
		return occurrences
	}

	windowStart, _ := time.ParseInLocation(dayLayout, from, readerZone)
	windowEnd, _ := time.ParseInLocation(dayLayout, addDays(to, 1), readerZone)
	inWindow := func(start, end time.Time) bool {
		return start.Before(windowEnd) && (end.After(windowStart) || !start.Before(windowStart))
	}
	if event.Repetition == nil {
		if inWindow(*when.startsAt, *when.endsAt) {
			return []eventTime{when}
		}
		return nil
	}

	zone, _ := time.LoadLocation(event.Repetition.TimeZone)
	localStart, localEnd := when.startsAt.In(zone), when.endsAt.In(zone)
	further := daysBetween(localStart.Format(dayLayout), localEnd.Format(dayLayout))
	// A day earlier and later than the window, because the reader's days are
	// not the days of the event's zone.
	days := event.Repetition.Schedule.daysBetween(
		addDays(windowStart.In(zone).Format(dayLayout), -further-1),
		addDays(windowEnd.In(zone).Format(dayLayout), 1),
	)
	var occurrences []eventTime
	for _, day := range days {
		// The first occurrence is the event itself, which says which of two
		// equal clock times it meant.
		if day == event.Repetition.Schedule.StartsOn {
			if inWindow(*when.startsAt, *when.endsAt) {
				occurrences = append(occurrences, when)
			}
			continue
		}
		start := atTheClockTimeOf(localStart, day, zone)
		end := later(start, atTheClockTimeOf(localEnd, addDays(day, further), zone))
		if inWindow(start, end) {
			start, end = start.UTC(), end.UTC()
			occurrences = append(occurrences, eventTime{startsAt: &start, endsAt: &end})
		}
	}
	return occurrences
}

// atTheClockTimeOf is the moment a day of the zone shows the clock time of
// another moment, placed where PHP's setTime() places it: an hour that comes
// twice is taken the first time, and one that a change of summer time skips
// is moved forward by the length of the change.
func atTheClockTimeOf(local time.Time, day string, zone *time.Location) time.Time {
	date, _ := midnightOf(day)
	wall := time.Date(date.Year(), date.Month(), date.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC)
	offsetAt := func(moment time.Time) time.Duration {
		_, seconds := moment.In(zone).Zone()
		return time.Duration(seconds) * time.Second
	}
	// A zone changes its offset at most once within a day either way.
	before, after := offsetAt(wall.Add(-24*time.Hour)), offsetAt(wall.Add(24*time.Hour))
	var found []time.Time
	for _, offset := range []time.Duration{before, after} {
		if moment := wall.Add(-offset); offsetAt(moment) == offset && !slices.ContainsFunc(found, moment.Equal) {
			found = append(found, moment)
		}
	}
	if len(found) == 0 {
		return wall.Add(-before).In(zone)
	}
	return slices.MinFunc(found, func(a, b time.Time) int { return a.Compare(b) }).In(zone)
}

// daysBetween are the days of the schedule from one day to another, both
// included.
func (r RepetitionSchedule) daysBetween(from, to string) []string {
	var days []string
	for day, ok := r.nextOccurrenceAfter(addDays(from, -1)); ok && day <= to; day, ok = r.nextOccurrenceAfter(day) {
		days = append(days, day)
	}
	return days
}

// startOf is when an occurrence starts for the order of a span: an all-day
// one at midnight UTC of its first day.
func startOf(occurrence eventTime) time.Time {
	if occurrence.startsAt != nil {
		return *occurrence.startsAt
	}
	day, _ := midnightOf(occurrence.firstDay)
	return day
}

// zoneOf is the zone a person's days are read in: their own, else the
// installation's.
func (s *Server) zoneOf(person *Person) (string, *time.Location) {
	name := person.TimeZone
	if name == "" {
		name = s.TimeZone
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return name, time.UTC
	}
	return name, location
}

// visibleCalendar finds a calendar in a project the person is a member of,
// and for a client only one the clients are let into.
func (s *Server) visibleCalendar(toolID string, person *Person) *CalendarTool {
	for _, tool := range s.Calendars {
		if tool.ID == toolID && s.visibleProject(tool.ProjectID, person) != nil && (tool.ShownToClients || !s.isAClient(person.ID)) {
			return tool
		}
	}
	return nil
}

// visibleEvent finds an event out of the trash by its identifier alone; a
// client finds only one shown to clients.
func (s *Server) visibleEvent(eventID string, person *Person) (*CalendarTool, *CalendarEvent) {
	if !uuidPattern.MatchString(eventID) {
		return nil, nil
	}
	for _, tool := range s.Calendars {
		for _, event := range tool.Events {
			if event.ID == eventID {
				if s.visibleCalendar(tool.ID, person) == nil || !s.seesEvent(tool, event, person) {
					return nil, nil
				}
				return tool, event
			}
		}
	}
	return nil, nil
}

func (s *Server) seesEvent(tool *CalendarTool, event *CalendarEvent, person *Person) bool {
	return !event.Trashed && (!s.isAClient(person.ID) || tool.ShownToClients && event.ShownToClients)
}

func occurrenceResource(occurrence eventTime) map[string]any {
	return map[string]any{
		"startsAt": timestampOrNil(occurrence.startsAt), "endsAt": timestampOrNil(occurrence.endsAt),
		"firstDay": nullable(occurrence.firstDay), "lastDay": nullable(occurrence.lastDay),
	}
}

// eventFieldsOfBothForms are what a line of a span and an event read by
// itself share.
func (s *Server) eventFieldsOfBothForms(tool *CalendarTool, event *CalendarEvent) map[string]any {
	fields := occurrenceResource(event.time())
	var schedule, zone any
	if event.Repetition != nil {
		schedule, zone = scheduleResource(event.Repetition.Schedule), nullable(event.Repetition.TimeZone)
	}
	participants := []any{}
	for _, id := range event.ParticipantIDs {
		participants = append(participants, s.personReference(id))
	}
	for name, value := range map[string]any{
		"id": event.ID, "projectId": tool.ProjectID, "title": event.Title, "allDay": event.FirstDay != "",
		"schedule": schedule, "timeZone": zone, "joinUrl": nullable(event.JoinURL), "highlighted": event.Highlighted,
		"participants": participants, "commentCount": event.CommentCount, "appUrl": s.eventURL(tool, event),
	} {
		fields[name] = value
	}
	return fields
}

func (s *Server) eventLineResource(tool *CalendarTool, event *CalendarEvent, occurrences []eventTime) map[string]any {
	line := s.eventFieldsOfBothForms(tool, event)
	times := []any{}
	for _, occurrence := range occurrences {
		times = append(times, occurrenceResource(occurrence))
	}
	line["occurrences"] = times
	return line
}

func (s *Server) eventResource(tool *CalendarTool, event *CalendarEvent) map[string]any {
	resource := s.eventFieldsOfBothForms(tool, event)
	for name, value := range map[string]any{
		"toolId": tool.ID, "project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"description": event.Description, "descriptionSource": event.Description, "attachments": s.attachmentsIn(tool.ProjectID, event.Description),
		"createdAt": timestamp(event.CreatedAt), "creator": s.personReference(event.CreatorID),
	} {
		resource[name] = value
	}
	return resource
}

func (s *Server) calendarURL(tool *CalendarTool) string {
	return s.URL + "/projects/" + tool.ProjectID + "/calendars/" + tool.ID
}

func (s *Server) eventURL(tool *CalendarTool, event *CalendarEvent) string {
	return s.calendarURL(tool) + "/events/" + event.ID
}

func eventTimeNotReadable(w http.ResponseWriter) {
	unprocessable(w, "eventTimeNotReadable", "A moment is written as ISO 8601 with an offset, such as 2026-10-05T09:00:00+02:00, a day as YYYY-MM-DD.")
}

func eventEndsBeforeItStarts(w http.ResponseWriter) {
	unprocessable(w, "eventEndsBeforeItStarts", "An event ends at or after the moment it starts, and on or after its first day.")
}

func participantNotAMember(w http.ResponseWriter) {
	unprocessable(w, "personNotAProjectMember", "Only a member of the project can take part in an event of it.")
}
