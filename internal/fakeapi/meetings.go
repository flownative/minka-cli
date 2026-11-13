package fakeapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Every event the reader sees has a meeting: the agenda of the decisions it
// is to take, the outcome of each, and whether it is closed. A series meets
// on each of its days, so each day has an agenda of its own.

// maxVoteCount is the highest count of votes a meeting records.
const maxVoteCount = 99999

// meetingOutcomes are the words a meeting makes of an item, each with the
// role of the status it puts the decision in.
var meetingOutcomes = map[string]string{
	"accepted": "holds", "acceptedWithChanges": "holds", "rejected": "rejected", "deferred": "deferred",
}

// Meeting is what an event keeps of one day it meets on.
type Meeting struct {
	Items      []*AgendaItem // in the order of the agenda
	ClosedAt   *time.Time    // nil while open
	ClosedByID string        // empty while open, and where the person was purged
}

// AgendaItem is a decision on the agenda of a meeting, with what the meeting
// made of it.
type AgendaItem struct {
	ID         string
	DecisionID string
	Outcome    *MeetingOutcome // nil until recorded
}

// MeetingOutcome is what a meeting made of an item, and the votes counted
// there, whatever the decision says since. An empty Outcome stands for a
// word only a newer Minka knows, which the API hands out as null.
type MeetingOutcome struct {
	Outcome      string
	StatusID     string // the status it put the decision in
	Votes        string // the note, empty for none
	VotesFor     *int
	VotesAgainst *int
	Abstentions  *int
	RecordedByID string
	RecordedAt   time.Time
}

// EventMove is the time one occurrence of a series takes place at instead of
// the one its schedule gives.
type EventMove struct {
	StartsAt, EndsAt  *time.Time
	FirstDay, LastDay string
}

// MeetingOn is the meeting of an event on a day, made where there is none
// yet; the day is empty for an event that does not repeat.
func (e *CalendarEvent) MeetingOn(day string) *Meeting {
	if e.Meetings == nil {
		e.Meetings = map[string]*Meeting{}
	}
	if e.Meetings[day] == nil {
		e.Meetings[day] = &Meeting{}
	}
	return e.Meetings[day]
}

// PutOnAgenda puts decisions at the end of an agenda, as a test prepares one.
func (m *Meeting) PutOnAgenda(decisions ...*Decision) []*AgendaItem {
	var items []*AgendaItem
	for _, decision := range decisions {
		items = append(items, &AgendaItem{ID: newID(), DecisionID: decision.ID})
	}
	m.Items = append(m.Items, items...)
	return items
}

func (s *Server) registerMeetingRoutes(mux *http.ServeMux) {
	for pattern, handler := range map[string]func(http.ResponseWriter, *http.Request, *Person){
		"GET /api/v1/events/{eventId}/meeting":                               s.meeting,
		"POST /api/v1/events/{eventId}/meeting/agenda":                       s.putOnAgenda,
		"DELETE /api/v1/events/{eventId}/meeting/agenda/{agendaItemId}":      s.takeOffAgenda,
		"POST /api/v1/events/{eventId}/meeting/agenda/{agendaItemId}/moves":  s.moveAgendaItem,
		"PUT /api/v1/events/{eventId}/meeting/agenda/{agendaItemId}/outcome": s.recordOutcome,
		"POST /api/v1/events/{eventId}/meeting/close":                        s.closeMeeting,
	} {
		mux.HandleFunc(pattern, s.authenticated(func(w http.ResponseWriter, r *http.Request, person *Person) {
			if s.KnowsNoMeetings {
				unknownAddress(w)
				return
			}
			handler(w, r, person)
		}))
	}
}

// heldMeeting is a meeting a request names, and what its day says of it.
type heldMeeting struct {
	tool      *CalendarTool
	event     *CalendarEvent
	meeting   *Meeting
	day       string // empty for an event that does not repeat
	cancelled bool
}

func (h heldMeeting) closed() bool {
	return h.meeting.ClosedAt != nil
}

// meetingToRead finds the meeting of a request: the event the reader sees,
// then for a series the day, which has to be readable and one the schedule
// gives or a cancelled one. An event that does not repeat reads no day.
func (s *Server) meetingToRead(w http.ResponseWriter, r *http.Request, person *Person) (heldMeeting, bool) {
	tool, event := s.visibleEvent(strings.ToLower(r.PathValue("eventId")), person)
	if event == nil {
		notFound(w)
		return heldMeeting{}, false
	}
	held := heldMeeting{tool: tool, event: event}
	if event.Repetition != nil {
		day := r.URL.Query().Get("day")
		if _, isADay := midnightOf(day); !isADay {
			unprocessable(w, "meetingDayMissing", `A series meets on each of its days; "day" names one as YYYY-MM-DD.`)
			return heldMeeting{}, false
		}
		// A cancelled day counts only while the schedule still gives it.
		if !event.Repetition.Schedule.fallsOn(day) {
			notFound(w)
			return heldMeeting{}, false
		}
		held.day, held.cancelled = day, slices.Contains(event.CancelledDays, day)
	}
	held.meeting = event.MeetingOn(held.day)
	return held, true
}

// meetingToChange is the meeting of a write: an archived project is refused
// after the day is read, and a cancelled day is not there for any write.
func (s *Server) meetingToChange(w http.ResponseWriter, r *http.Request, person *Person) (heldMeeting, bool) {
	held, ok := s.meetingToRead(w, r, person)
	if !ok || s.refusedAsArchived(w, s.projectWithID(held.tool.ProjectID)) {
		return heldMeeting{}, false
	}
	if held.cancelled {
		notFound(w)
		return heldMeeting{}, false
	}
	return held, true
}

// decisionOnAgenda is the decision of an item where it still counts: one out
// of the trash, with its tool.
func (s *Server) decisionOnAgenda(item *AgendaItem) (*DecisionsTool, *Decision) {
	for _, tool := range s.DecisionsTools {
		if decision := s.decisionOfTool(tool, item.DecisionID); decision != nil {
			return tool, decision
		}
	}
	return nil, nil
}

// showsAgendaItem says whether the reader is shown an item: its decision is
// out of the trash and one the reader sees.
func (s *Server) showsAgendaItem(item *AgendaItem, reader *Person) bool {
	tool, decision := s.decisionOnAgenda(item)
	return decision != nil && s.visibleDecisionsTool(tool.ID, reader) != nil && s.seesDecision(tool, decision, reader)
}

// mayChangeAgendaItem says whether the reader may change an item: they are
// shown it, and a decision in the archive counts only for taking it off and
// for closing.
func (s *Server) mayChangeAgendaItem(item *AgendaItem, reader *Person, archivedToo bool) bool {
	_, decision := s.decisionOnAgenda(item)
	return s.showsAgendaItem(item, reader) && (archivedToo || !decision.Archived)
}

// mayChangeEveryItem says whether the reader may change every item whose
// decision still counts, those hidden from them too.
func (s *Server) mayChangeEveryItem(meeting *Meeting, reader *Person) bool {
	for _, item := range meeting.Items {
		if _, decision := s.decisionOnAgenda(item); decision != nil && !s.mayChangeAgendaItem(item, reader, true) {
			return false
		}
	}
	return true
}

// shownAgenda are the items the reader is shown, in their order.
func (s *Server) shownAgenda(meeting *Meeting, reader *Person) []*AgendaItem {
	var shown []*AgendaItem
	for _, item := range meeting.Items {
		if s.showsAgendaItem(item, reader) {
			shown = append(shown, item)
		}
	}
	return shown
}

// agendaItemNamed finds the item of an address among those the reader is
// shown.
func (s *Server) agendaItemNamed(r *http.Request, meeting *Meeting, reader *Person) *AgendaItem {
	id := strings.ToLower(r.PathValue("agendaItemId"))
	for _, item := range s.shownAgenda(meeting, reader) {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func meetingClosed(w http.ResponseWriter) {
	unprocessable(w, "meetingClosed", "This meeting is closed.")
}

func (s *Server) meeting(w http.ResponseWriter, r *http.Request, person *Person) {
	held, ok := s.meetingToRead(w, r, person)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.meetingResource(held, person))
}

// putOnAgenda puts decisions at the end of the agenda, all or none. Every
// decision is looked at before any is put on, so one that is unknown or not
// open is refused before one that stands on the agenda already.
func (s *Server) putOnAgenda(w http.ResponseWriter, r *http.Request, person *Person) {
	held, ok := s.meetingToChange(w, r, person)
	if !ok {
		return
	}
	if held.closed() {
		meetingClosed(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	if fields["decisionIds"] == nil {
		malformed(w, `The field "decisionIds" has to be a list of strings.`)
		return
	}
	named, ok := stringListField(w, fields, "decisionIds")
	if !ok {
		return
	}
	if len(named) == 0 {
		unprocessable(w, "decisionsMissing", "At least one decision goes on the agenda.")
		return
	}
	ids := make([]string, len(named))
	for i, id := range named {
		if ids[i] = strings.ToLower(id); !uuidPattern.MatchString(ids[i]) {
			unprocessable(w, "decisionUnknown", "No decision of this project you may change goes by that identifier.")
			return
		}
	}
	for _, id := range ids {
		tool, decision := s.visibleDecision(id, person)
		if decision == nil || tool.ProjectID != held.tool.ProjectID || decision.Archived {
			unprocessable(w, "decisionUnknown", "No decision of this project you may change goes by that identifier.")
			return
		}
		role := s.statusRole(tool, decision.StatusID)
		if role != "proposed" && role != "deferred" && role != "" {
			unprocessable(w, "decisionNotForAMeetingAgenda", "Only a decision still to be taken goes on the agenda of a meeting.")
			return
		}
	}
	for i, id := range ids {
		if slices.Contains(ids[:i], id) || slices.ContainsFunc(held.meeting.Items, func(item *AgendaItem) bool { return item.DecisionID == id }) {
			unprocessable(w, "decisionAlreadyOnMeetingAgenda", "This decision is on the agenda of the meeting already.")
			return
		}
	}
	for _, id := range ids {
		held.meeting.Items = append(held.meeting.Items, &AgendaItem{ID: newID(), DecisionID: id})
	}
	writeJSON(w, http.StatusCreated, s.meetingResource(held, person))
}

// takeOffAgenda takes an item off and leaves its decision as it is. An item
// that is gone is 404, a second time too.
func (s *Server) takeOffAgenda(w http.ResponseWriter, r *http.Request, person *Person) {
	held, ok := s.meetingToChange(w, r, person)
	if !ok {
		return
	}
	item := s.agendaItemNamed(r, held.meeting, person)
	if item == nil || !s.mayChangeAgendaItem(item, person, true) {
		notFound(w)
		return
	}
	if held.closed() {
		meetingClosed(w)
		return
	}
	held.meeting.Items = slices.DeleteFunc(held.meeting.Items, func(other *AgendaItem) bool { return other == item })
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// moveAgendaItem puts an item behind one neighbour or in front of the other,
// and first with neither. A neighbour is one of the items the reader is
// shown and not the item itself, named as the agenda hands its ID out, in
// lowercase; the previous one is judged before the next one is read.
func (s *Server) moveAgendaItem(w http.ResponseWriter, r *http.Request, person *Person) {
	held, ok := s.meetingToChange(w, r, person)
	if !ok {
		return
	}
	item := s.agendaItemNamed(r, held.meeting, person)
	if item == nil || !s.mayChangeAgendaItem(item, person, false) {
		notFound(w)
		return
	}
	if held.closed() {
		meetingClosed(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	if s.BeforeMovingAgendaItem != nil {
		s.BeforeMovingAgendaItem(held.meeting)
	}
	shown := s.shownAgenda(held.meeting, person)
	var neighbours [2]string
	for i, name := range []string{"previousItemId", "nextItemId"} {
		value, ok := stringField(w, fields, name)
		if !ok {
			return
		}
		id := deref(value)
		if id != "" && (id == item.ID || !slices.ContainsFunc(shown, func(other *AgendaItem) bool { return other.ID == id })) {
			unprocessable(w, "agendaPlaceUnknown", "The neighbours named are no other items of this agenda.")
			return
		}
		neighbours[i] = id
	}

	others := slices.DeleteFunc(slices.Clone(held.meeting.Items), func(other *AgendaItem) bool { return other == item })
	at := 0
	if previous := slices.IndexFunc(others, func(other *AgendaItem) bool { return other.ID == neighbours[0] }); neighbours[0] != "" {
		at = previous + 1
	} else if next := slices.IndexFunc(others, func(other *AgendaItem) bool { return other.ID == neighbours[1] }); neighbours[1] != "" {
		at = next
	}
	held.meeting.Items = slices.Insert(others, at, item)
	writeJSON(w, http.StatusOK, s.meetingResource(held, person))
}

// recordOutcome records what the meeting made of an item, or corrects it;
// the last one written stands. A closed meeting takes a correction and no
// first outcome, which is refused before the body is read. A vote field left
// out keeps its value, on the item and on the decision, and null clears it.
func (s *Server) recordOutcome(w http.ResponseWriter, r *http.Request, person *Person) {
	held, ok := s.meetingToChange(w, r, person)
	if !ok {
		return
	}
	item := s.agendaItemNamed(r, held.meeting, person)
	if item == nil || !s.mayChangeAgendaItem(item, person, false) {
		notFound(w)
		return
	}
	if item.Outcome == nil && held.closed() {
		meetingClosed(w)
		return
	}
	fields, ok := readBodyWith(w, r, true)
	if !ok {
		return
	}
	word, ok := stringField(w, fields, "outcome")
	if !ok {
		return
	}
	role, known := meetingOutcomes[deref(word)]
	if !known {
		unprocessable(w, "meetingOutcomeUnknown", `The outcome is "accepted", "acceptedWithChanges", "rejected" or "deferred".`)
		return
	}
	withChanges := *word == "acceptedWithChanges"
	sentence, ok := stringField(w, fields, "sentence")
	if !ok {
		return
	}
	if sentence != nil && !withChanges {
		unprocessable(w, "sentenceWithoutChanges", `Only an outcome of "acceptedWithChanges" takes a "sentence".`)
		return
	}
	if withChanges && !validTitle(w, "decision", "a decision", deref(sentence)) {
		return
	}
	tool, decision := s.decisionOnAgenda(item)
	recorded := votes{}
	if item.Outcome != nil {
		recorded = votes{item.Outcome.Votes, item.Outcome.VotesFor, item.Outcome.VotesAgainst, item.Outcome.Abstentions}
	}
	recorded, ok = readVotes(w, fields, recorded)
	if !ok {
		return
	}
	ofTheDecision, _ := readVotes(nil, fields, votes{decision.Votes, decision.VotesFor, decision.VotesAgainst, decision.Abstentions})
	// What is seeded stays only where the outcome is recorded: a refusal
	// leaves the tool as it was.
	asItWas := slices.Clone(tool.Statuses)
	tool.seedStatuses()
	status := tool.StatusOfRole(role)
	if status == nil {
		tool.Statuses = asItWas
		unprocessable(w, "meetingOutcomeHasNoStatus", "The Decisions tool of this decision has no status for this outcome.")
		return
	}

	item.Outcome = &MeetingOutcome{
		Outcome: *word, StatusID: status.ID, Votes: recorded.note, VotesFor: recorded.votesFor, VotesAgainst: recorded.votesAgainst,
		Abstentions: recorded.abstentions, RecordedByID: person.ID, RecordedAt: s.now,
	}
	before := decision.snapshot("", time.Time{})
	if withChanges {
		decision.Title = phpTrim(*sentence)
	}
	decision.Votes, decision.VotesFor, decision.VotesAgainst, decision.Abstentions = ofTheDecision.note, ofTheDecision.votesFor, ofTheDecision.votesAgainst, ofTheDecision.abstentions
	// An outcome that decides makes the meeting the day and the event the
	// decision was taken on; one that defers takes back only what names this
	// very meeting.
	switch day := held.meetingDay(); {
	case role != "deferred":
		decision.DecidedOn, decision.EventID = day, held.event.ID
	case decision.EventID == held.event.ID && decision.DecidedOn == day:
		decision.DecidedOn, decision.EventID = "", ""
	}
	if decision.StatusID != status.ID {
		decision.StatusID = status.ID
		decision.StatusSetBy = &DecisionStatusSetBy{PersonID: person.ID, At: s.now}
		if role == "holds" {
			s.moveTheEarlier(tool, decision, person)
		}
	}
	if after := decision.snapshot(person.ID, s.aMomentAfter(decision.UpdatedAt)); !before.sameText(after) {
		decision.Versions = append(decision.Versions, after)
		decision.UpdatedAt = after.WrittenAt
	}
	writeJSON(w, http.StatusOK, s.meetingResource(held, person))
}

// votes are the note and the three counts of a vote.
type votes struct {
	note                                string
	votesFor, votesAgainst, abstentions *int
}

// readVotes reads the votes of a body over what stands: a field left out
// keeps its value, null clears it. The note is one line of at most 100
// characters, a count a whole number as JSON writes one. Without a writer
// nothing is refused, for a body that was read once already.
func readVotes(w http.ResponseWriter, fields map[string]any, current votes) (votes, bool) {
	refuse := func(code, message string) (votes, bool) {
		if w != nil {
			unprocessable(w, code, message)
		}
		return current, false
	}
	if value, named := fields["votes"]; named {
		note, isText := value.(string)
		if value != nil && !isText {
			if w != nil {
				malformed(w, `The field "votes" has to be a string.`)
			}
			return current, false
		}
		current.note = phpTrim(note)
	}
	switch {
	case utf8.RuneCountInString(current.note) > maxDecisionVotes:
		return refuse("decisionVotesTooLong", "The note on the votes of a decision is one line of at most 100 characters.")
	case strings.IndexFunc(current.note, unicode.IsControl) >= 0:
		return refuse("decisionVotesHasControlCharacters", "The note on the votes of a decision is one line of at most 100 characters.")
	}
	for _, read := range []struct {
		name  string
		count **int
	}{{"votesFor", &current.votesFor}, {"votesAgainst", &current.votesAgainst}, {"abstentions", &current.abstentions}} {
		name, count := read.name, read.count
		value, named := fields[name]
		if !named {
			continue
		}
		if value == nil {
			*count = nil
			continue
		}
		number, _ := value.(json.Number)
		counted, err := strconv.Atoi(number.String())
		if err != nil || counted < 0 || counted > maxVoteCount {
			return refuse("decisionVoteCountMalformed", `The field "`+name+`" is a whole number from 0 to 99999, or null.`)
		}
		*count = &counted
	}
	return current, true
}

// closeMeeting closes a meeting for good and tells the audience. An empty
// agenda and items the writer may not change are both the 404 of a meeting
// that is not there to close; the people chosen are asked about last, and
// whoever cannot be told leaves the meeting open.
func (s *Server) closeMeeting(w http.ResponseWriter, r *http.Request, person *Person) {
	held, ok := s.meetingToChange(w, r, person)
	if !ok {
		return
	}
	if held.closed() {
		meetingClosed(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	named, ok := stringField(w, fields, "audience")
	if !ok {
		return
	}
	audience := "everybody"
	if named != nil {
		if audience = *named; !slices.Contains(decisionAudiences, audience) {
			unprocessable(w, "meetingAudienceUnknown", `A closed meeting reaches "everybody", "chosen" with "notifyIds", or "nobody".`)
			return
		}
	}
	listed, ok := stringListField(w, fields, "notifyIds")
	if !ok {
		return
	}
	var notifyIDs []string
	for _, personID := range listed {
		if !uuidPattern.MatchString(personID) {
			unprocessable(w, "personNotAProjectMember", "Only a member of the project hears of its meeting.")
			return
		}
		notifyIDs = append(notifyIDs, strings.ToLower(personID))
	}
	if audience == "chosen" && len(notifyIDs) == 0 {
		unprocessable(w, "notifyIdsMissing", `An audience of "chosen" names the people in "notifyIds".`)
		return
	}
	if audience != "chosen" && len(notifyIDs) > 0 {
		unprocessable(w, "notifyIdsWithoutChosenAudience", `Only an audience of "chosen" takes "notifyIds".`)
		return
	}
	counts := slices.ContainsFunc(held.meeting.Items, func(item *AgendaItem) bool {
		_, decision := s.decisionOnAgenda(item)
		return decision != nil
	})
	if !counts || !s.mayChangeEveryItem(held.meeting, person) {
		notFound(w)
		return
	}
	project := s.projectWithID(held.tool.ProjectID)
	shownToClients := held.tool.ShownToClients && held.event.ShownToClients
	var told []string
	switch audience {
	case "everybody":
		for _, id := range project.MemberIDs {
			// Everybody is the humans of the project: no agent hears of it.
			if id != person.ID && s.People[id].PrincipalID == "" && (shownToClients || !s.isAClient(id)) {
				told = append(told, id)
			}
		}
	case "chosen":
		for _, id := range notifyIDs {
			if id == person.ID || slices.Contains(told, id) {
				continue
			}
			if !slices.Contains(project.MemberIDs, id) {
				unprocessable(w, "personNotAProjectMember", "Only a member of the project hears of its meeting.")
				return
			}
			if s.isAClient(id) && !shownToClients {
				unprocessable(w, "personCannotSeeIt", "Somebody chosen is a client of the project, and this event is not shown to clients.")
				return
			}
			told = append(told, id)
		}
	}

	now := s.now
	held.meeting.ClosedAt, held.meeting.ClosedByID = &now, person.ID
	for _, id := range told {
		s.notify(&Notification{
			PersonID: id, Occasion: "meeting.closed", ActorID: person.ID, Summary: person.Name + " closed the meeting " + held.event.Title,
			SubjectType: "event", SubjectID: held.event.ID, SubjectTitle: held.event.Title, ProjectID: project.ID,
		})
	}
	writeJSON(w, http.StatusOK, s.meetingResource(held, person))
}

// meetingResource is a meeting as its reader is shown it: the items they see
// numbered without a gap, the first of them without an outcome marked, and
// what they may do with each.
func (s *Server) meetingResource(held heldMeeting, reader *Person) map[string]any {
	mayRecord := !s.projectWithID(held.tool.ProjectID).Archived && !held.cancelled
	items := []any{}
	current, mayChangeOne := true, false
	for _, item := range s.shownAgenda(held.meeting, reader) {
		tool, decision := s.decisionOnAgenda(item)
		mayChange := s.mayChangeAgendaItem(item, reader, false)
		mayTakeOff := s.mayChangeAgendaItem(item, reader, true)
		mayChangeOne = mayChangeOne || mayTakeOff
		var outcome any
		if recorded := item.Outcome; recorded != nil {
			outcome = map[string]any{
				"outcome": nullable(recorded.Outcome), "status": outcomeStatusResource(tool, recorded.StatusID),
				"withChanges": recorded.Outcome == "acceptedWithChanges", "votes": nullable(recorded.Votes),
				"votesFor": countOrNil(recorded.VotesFor), "votesAgainst": countOrNil(recorded.VotesAgainst), "abstentions": countOrNil(recorded.Abstentions),
				"recordedBy": s.personReferenceOrNil(recorded.RecordedByID), "recordedAt": timestamp(recorded.RecordedAt),
			}
		}
		items = append(items, map[string]any{
			"id": item.ID, "number": len(items) + 1, "current": current && item.Outcome == nil,
			"decision": s.linkResource(tool, decision), "outcome": outcome,
			"mayMove":          mayChange && mayRecord && !held.closed(),
			"mayTakeOff":       mayTakeOff && mayRecord && !held.closed(),
			"mayRecordOutcome": mayChange && mayRecord && (!held.closed() || item.Outcome != nil),
		})
		current = current && item.Outcome != nil
	}
	address := s.eventURL(held.tool, held.event) + "/meeting"
	if held.day != "" {
		address += "?day=" + held.day
	}
	resource := map[string]any{}
	for name, value := range s.NewerMeetingFields {
		resource[name] = value
	}
	for name, value := range map[string]any{
		"eventId": held.event.ID, "projectId": held.tool.ProjectID, "toolId": held.tool.ID, "title": held.event.Title,
		"day": nullable(held.day), "cancelled": held.cancelled, "closed": held.closed(),
		"closedAt": timestampOrNil(held.meeting.ClosedAt), "closedBy": s.personReferenceOrNil(held.meeting.ClosedByID),
		"mayClose": !held.closed() && mayRecord && mayChangeOne && s.mayChangeEveryItem(held.meeting, reader),
		"items":    items, "appUrl": address,
	} {
		resource[name] = value
	}
	return resource
}

// meetingDay is the day a meeting is kept under: the day of a series, and
// for an event that does not repeat its first day, or the day its start
// falls on in UTC, since its readers' zones differ.
func (h heldMeeting) meetingDay() string {
	switch {
	case h.day != "":
		return h.day
	case h.event.FirstDay != "":
		return h.event.FirstDay
	}
	return h.event.StartsAt.UTC().Format(dayLayout)
}

// outcomeStatusResource is the status an outcome put its decision in; one
// the team removed since has neither ID nor role nor name.
func outcomeStatusResource(tool *DecisionsTool, statusID string) map[string]any {
	for _, status := range tool.Statuses {
		if status.ID == statusID {
			return statusResource(status)
		}
	}
	return map[string]any{"id": nil, "name": "", "customName": nil, "role": nil}
}

// seedStatuses gives a tool without statuses the set a new tool starts
// with, and one that has a proposed status and no deferred one the deferred
// status right behind it, as Minka does before it looks for a role.
func (t *DecisionsTool) seedStatuses() {
	if len(t.Statuses) == 0 {
		for _, role := range decisionRoles {
			t.Statuses = append(t.Statuses, &DecisionStatus{ID: newID(), Role: role.role})
		}
	}
	if t.StatusOfRole("deferred") != nil {
		return
	}
	if proposed := t.StatusOfRole("proposed"); proposed != nil {
		t.Statuses = slices.Insert(t.Statuses, slices.Index(t.Statuses, proposed)+1, &DecisionStatus{ID: newID(), Role: "deferred"})
	}
}

func countOrNil(count *int) any {
	if count == nil {
		return nil
	}
	return *count
}
