package fakeapi

import (
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func meetingOf(event *CalendarEvent) string {
	return "/api/v1/events/" + event.ID + "/meeting"
}

// itemsOf are the items of a meeting as answered.
func itemsOf(t *testing.T, meeting map[string]any) []map[string]any {
	t.Helper()
	listed, ok := meeting["items"].([]any)
	if !ok {
		t.Fatalf("the answer carries no items: %v", meeting)
	}
	items := make([]map[string]any, len(listed))
	for i, item := range listed {
		items[i] = item.(map[string]any)
	}
	return items
}

// aSecondProposal adds another decision still to be taken to the fixture.
func aSecondProposal(f *Fixture) *Decision {
	second := &Decision{ID: newID(), Title: "We move the party", StatusID: f.Decisions.StatusOfRole("proposed").ID,
		CreatedAt: f.ProposedDecision.CreatedAt.Add(time.Hour), UpdatedAt: f.ProposedDecision.CreatedAt.Add(time.Hour), CreatorID: f.Bruno.ID, ShortID: "DEC-9"}
	f.Decisions.Decisions = append(f.Decisions.Decisions, second)
	return second
}

// withAClient lets the client of the project into the calendar, the event
// and the first proposal, and gives them a token.
func withAClient(s *Server, f *Fixture, event *CalendarEvent) string {
	f.Calendar.ShownToClients, event.ShownToClients, f.ProposedDecision.ShownToClients = true, true, true
	s.Tokens["minka_pat_client"] = f.Client.ID
	return "minka_pat_client"
}

// Every event has a meeting: without an agenda it is open, empty and not to
// be closed, with exactly the fields Minka hands out. Decisions put on go to
// the end in their order, numbered, the first one current.
func TestAMeetingReadsAsItStands(t *testing.T) {
	s, f := New(t)
	second := aSecondProposal(f)
	path := meetingOf(f.OnceEvent)

	got, empty := answerTo(t, s, f.Token, "GET", path, "")
	want := map[string]any{
		"eventId": f.OnceEvent.ID, "projectId": f.Project.ID, "toolId": f.Calendar.ID, "title": f.OnceEvent.Title,
		"day": nil, "cancelled": false, "closed": false, "closedAt": nil, "closedBy": nil, "mayClose": false,
		"items": []any{}, "appUrl": s.eventURL(f.Calendar, f.OnceEvent) + "/meeting",
	}
	if got != "200" || !reflect.DeepEqual(empty, want) {
		t.Fatalf("an event without an agenda: %s %v", got, empty)
	}
	// An event that does not repeat reads no day at all.
	if got, _ := answerTo(t, s, f.Token, "GET", path+"?day=8.10.2035", ""); got != "200" {
		t.Errorf("a day on an event that does not repeat: %s", got)
	}

	got, meeting := answerTo(t, s, f.Token, "POST", path+"/agenda", `{"decisionIds":["`+strings.ToUpper(f.ProposedDecision.ID)+`","`+second.ID+`"]}`)
	items := itemsOf(t, meeting)
	if got != "201" || len(items) != 2 || meeting["mayClose"] != true {
		t.Fatalf("put on: %s %v", got, meeting)
	}
	first := map[string]any{
		"id": f.OnceEvent.Meetings[""].Items[0].ID, "number": 1.0, "current": true, "outcome": nil,
		"mayMove": true, "mayTakeOff": true, "mayRecordOutcome": true,
		"decision": map[string]any{
			"id": f.ProposedDecision.ID, "title": f.ProposedDecision.Title, "archived": false,
			"appUrl": s.decisionURL(f.Decisions, f.ProposedDecision), "shortId": nullable(f.ProposedDecision.ShortID),
			"status": statusResource(f.Decisions.StatusOfRole("proposed")),
		},
	}
	if !reflect.DeepEqual(items[0], first) {
		t.Errorf("the first item reads %v, want %v", items[0], first)
	}
	if items[1]["number"] != 2.0 || items[1]["current"] != false || items[1]["decision"].(map[string]any)["id"] != second.ID {
		t.Errorf("the second item reads %v", items[1])
	}
	if f.ProposedDecision.StatusID != f.Decisions.StatusOfRole("proposed").ID {
		t.Error("putting a decision on changed it")
	}
}

// A series meets on each of its days: without a readable day it is
// meetingDayMissing, a day the schedule does not give is not there, each day
// has its own agenda, and a cancelled day reads as cancelled and takes no
// write.
func TestASeriesMeetsOnEachOfItsDays(t *testing.T) {
	s, f := New(t)
	path := meetingOf(f.WeeklyEvent)
	// A day cancelled or moved that the schedule gives no more counts for nothing.
	f.WeeklyEvent.CancelledDays = []string{"2026-11-02", "2026-10-27"}
	body := `{"decisionIds":["` + f.ProposedDecision.ID + `"]}`

	for query, want := range map[string]string{
		"":                "422 meetingDayMissing",
		"?day=":           "422 meetingDayMissing",
		"?day=26.10.2026": "422 meetingDayMissing",
		"?day=2026-02-30": "422 meetingDayMissing",
		"?day=2026-10-27": "404",
		"?day=2026-10-12": "404",
		"?day=2026-10-26": "200",
		"?day=2026-11-02": "200",
	} {
		if got, _ := answerTo(t, s, f.Token, "GET", path+query, ""); got != want {
			t.Errorf("GET %q: %s, want %s", query, got, want)
		}
	}
	if got, _ := answerTo(t, s, f.Token, "GET", meetingOf(f.OtherEvent)+"?day=nonsense", ""); got != "404" {
		t.Errorf("an event nobody here sees: %s", got)
	}

	got, meeting := answerTo(t, s, f.Token, "POST", path+"/agenda?day=2026-10-26", body)
	if got != "201" || meeting["day"] != "2026-10-26" || !strings.HasSuffix(meeting["appUrl"].(string), "/meeting?day=2026-10-26") {
		t.Fatalf("put on a day: %s %v", got, meeting)
	}
	if _, other := answerTo(t, s, f.Token, "GET", path+"?day=2026-10-19", ""); len(itemsOf(t, other)) != 0 {
		t.Errorf("another day shares the agenda: %v", other)
	}
	// The same decision stands on another meeting as well.
	if got, _ := answerTo(t, s, f.Token, "POST", path+"/agenda?day=2026-10-19", body); got != "201" {
		t.Errorf("the same decision on another day: %s", got)
	}

	_, cancelled := answerTo(t, s, f.Token, "GET", path+"?day=2026-11-02", "")
	if cancelled["cancelled"] != true || cancelled["mayClose"] != false {
		t.Errorf("a cancelled day reads %v", cancelled)
	}
	for _, write := range [][2]string{
		{"POST", "/agenda"}, {"POST", "/close"}, {"DELETE", "/agenda/" + newID()},
		{"POST", "/agenda/" + newID() + "/moves"}, {"PUT", "/agenda/" + newID() + "/outcome"},
	} {
		if got, _ := answerTo(t, s, f.Token, write[0], path+write[1]+"?day=2026-11-02", `{}`); got != "404" {
			t.Errorf("%s %s on a cancelled day: %s", write[0], write[1], got)
		}
		if got, _ := answerTo(t, s, f.Token, write[0], path+write[1], `{}`); got != "422 meetingDayMissing" {
			t.Errorf("%s %s without a day: %s", write[0], write[1], got)
		}
	}

	// The day is asked before the archive, the archive before a cancelled day.
	f.Project.Archived = true
	if got, read := answerTo(t, s, f.Token, "GET", path+"?day=2026-10-26", ""); got != "200" || read["mayClose"] != false || itemsOf(t, read)[0]["mayMove"] != false {
		t.Errorf("read in an archived project: %s %v", got, read)
	}
	for query, want := range map[string]string{"": "422 meetingDayMissing", "?day=2026-10-27": "404", "?day=2026-10-26": "422 projectArchived", "?day=2026-11-02": "422 projectArchived"} {
		if got, _ := answerTo(t, s, f.Token, "POST", path+"/agenda"+query, body); got != want {
			t.Errorf("a write in an archived project %q: %s, want %s", query, got, want)
		}
	}
}

// Putting on refuses in Minka's order, all or none: the closed meeting, the
// body, no decisions, an identifier that is none, then for every decision in
// turn one that is unknown or not open, and only then one that stands on the
// agenda already.
func TestPuttingOnIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	second := aSecondProposal(f)
	path := meetingOf(f.OnceEvent) + "/agenda"
	meeting := f.OnceEvent.MeetingOn("")
	meeting.PutOnAgenda(second)
	withoutStatus := &Decision{ID: newID(), Title: "From before the statuses", CreatedAt: s.now, CreatorID: f.Bruno.ID}
	ownStatus := &DecisionStatus{ID: newID(), Name: "In committee"}
	f.Decisions.Statuses = append(f.Decisions.Statuses, ownStatus)
	inCommittee := &Decision{ID: newID(), Title: "In committee", StatusID: ownStatus.ID, CreatedAt: s.now, CreatorID: f.Bruno.ID}
	f.Decisions.Decisions = append(f.Decisions.Decisions, withoutStatus, inCommittee)
	list := func(ids ...string) string { return `{"decisionIds":["` + strings.Join(ids, `","`) + `"]}` }

	for _, c := range []struct{ name, body, want string }{
		{"no field", `{}`, "400 malformedBody"},
		{"null", `{"decisionIds":null}`, "400 malformedBody"},
		{"a string", `{"decisionIds":"` + f.ProposedDecision.ID + `"}`, "400 malformedBody"},
		{"a number in the list", `{"decisionIds":[1]}`, "400 malformedBody"},
		{"an empty list", `{"decisionIds":[]}`, "422 decisionsMissing"},
		{"a short identifier", list("DEC-2"), "422 decisionUnknown"},
		{"an empty identifier", list(""), "422 decisionUnknown"},
		{"an unknown decision", list(newID()), "422 decisionUnknown"},
		{"a decision of another project", list(f.OtherDecision.ID), "422 decisionUnknown"},
		{"a decision in the archive", list(f.ArchivedDecision.ID), "422 decisionUnknown"},
		{"a decision in force", list(f.Decision.ID), "422 decisionNotForAMeetingAgenda"},
		{"a draft", list(f.DraftDecision.ID), "422 decisionNotForAMeetingAgenda"},
		{"a decision without a status", list(withoutStatus.ID), "422 decisionNotForAMeetingAgenda"},
		{"one on the agenda before an unknown one", list(second.ID, newID()), "422 decisionUnknown"},
		{"one on the agenda before one in force", list(second.ID, f.Decision.ID), "422 decisionNotForAMeetingAgenda"},
		{"one on the agenda already", list(f.ProposedDecision.ID, second.ID), "422 decisionAlreadyOnMeetingAgenda"},
		{"the same one twice", list(f.ProposedDecision.ID, f.ProposedDecision.ID), "422 decisionAlreadyOnMeetingAgenda"},
	} {
		if got, _ := answerTo(t, s, f.Token, "POST", path, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	if len(meeting.Items) != 1 {
		t.Fatalf("a refusal wrote %d items", len(meeting.Items))
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path, list(inCommittee.ID)); got != "201" {
		t.Errorf("a status the team added: %s", got)
	}

	now := s.now
	meeting.ClosedAt = &now
	if got, _ := answerTo(t, s, f.Token, "POST", path, `not even JSON`); got != "422 meetingClosed" {
		t.Errorf("a closed meeting before the body: %s", got)
	}
}

// A move goes behind the previous neighbour, in front of the next, and first
// with neither; a neighbour is another item the reader is shown, the
// previous one asked first.
func TestAnAgendaItemMoves(t *testing.T) {
	s, f := New(t)
	second, third := aSecondProposal(f), aSecondProposal(f)
	meeting := f.OnceEvent.MeetingOn("")
	items := meeting.PutOnAgenda(f.ProposedDecision, second, third)
	a, b, c := items[0], items[1], items[2]
	path := meetingOf(f.OnceEvent) + "/agenda/"
	order := func() []*AgendaItem { return meeting.Items }

	for _, step := range []struct {
		item *AgendaItem
		body string
		want []*AgendaItem
	}{
		{c, `{}`, []*AgendaItem{c, a, b}},
		{c, `{"previousItemId":"` + b.ID + `"}`, []*AgendaItem{a, b, c}},
		{c, `{"nextItemId":"` + b.ID + `"}`, []*AgendaItem{a, c, b}},
		{a, `{"previousItemId":"` + b.ID + `","nextItemId":"` + c.ID + `"}`, []*AgendaItem{c, b, a}},
		{a, `{"previousItemId":"","nextItemId":null}`, []*AgendaItem{a, c, b}},
		{a, `{}`, []*AgendaItem{a, c, b}},
	} {
		got, answer := answerTo(t, s, f.Token, "POST", path+step.item.ID+"/moves", step.body)
		if got != "200" || !reflect.DeepEqual(order(), step.want) || len(itemsOf(t, answer)) != 3 {
			t.Fatalf("%s: %s, the agenda is not as wanted", step.body, got)
		}
	}

	for body, want := range map[string]string{
		`not even JSON`:                         "400 malformedBody",
		`[]`:                                    "400 malformedBody",
		`{"previousItemId":1}`:                  "400 malformedBody",
		`{"nextItemId":"x"}`:                    "422 agendaPlaceUnknown",
		`{"previousItemId":"` + a.ID + `"}`:     "422 agendaPlaceUnknown",
		`{"nextItemId":"` + newID() + `"}`:      "422 agendaPlaceUnknown",
		`{"previousItemId":"x","nextItemId":1}`: "422 agendaPlaceUnknown",

		// The previous neighbour is judged before the next one is read, and
		// an ID is read in lowercase only.
		`{"previousItemId":"` + c.ID + `","nextItemId":1}`:   "400 malformedBody",
		`{"previousItemId":"` + strings.ToUpper(c.ID) + `"}`: "422 agendaPlaceUnknown",
		`{"nextItemId":"` + strings.ToUpper(c.ID) + `"}`:     "422 agendaPlaceUnknown",
	} {
		if got, _ := answerTo(t, s, f.Token, "POST", path+a.ID+"/moves", body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	for _, unknown := range []string{newID(), "nonsense"} {
		if got, _ := answerTo(t, s, f.Token, "POST", path+unknown+"/moves", `{}`); got != "404" {
			t.Errorf("an item %q: %s", unknown, got)
		}
	}
	now := s.now
	meeting.ClosedAt = &now
	if got, _ := answerTo(t, s, f.Token, "POST", path+a.ID+"/moves", `not even JSON`); got != "422 meetingClosed" {
		t.Errorf("a closed meeting before the body: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path+newID()+"/moves", `{}`); got != "404" {
		t.Errorf("an unknown item on a closed meeting: %s", got)
	}
}

// Taking off answers 204 and leaves the decision; a second time is 404, as
// is an item that is none; a closed meeting keeps its items.
func TestAnAgendaItemIsTakenOffOnce(t *testing.T) {
	s, f := New(t)
	meeting := f.OnceEvent.MeetingOn("")
	item := meeting.PutOnAgenda(f.ProposedDecision, f.ArchivedDecision)[0]
	archived := meeting.Items[1]
	path := meetingOf(f.OnceEvent) + "/agenda/"

	request, _ := http.NewRequest("DELETE", s.URL+path+item.ID, nil)
	request.Header.Set("Authorization", "Bearer "+f.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 204 || len(body) != 0 || len(meeting.Items) != 1 {
		t.Fatalf("taking off: %d %q", response.StatusCode, body)
	}
	for _, gone := range []string{item.ID, "nonsense"} {
		if got, _ := answerTo(t, s, f.Token, "DELETE", path+gone, ""); got != "404" {
			t.Errorf("a second time, %q: %s", gone, got)
		}
	}
	if _, decision := s.visibleDecision(f.ProposedDecision.ID, f.Bruno); decision == nil {
		t.Error("the decision went with its item")
	}

	// An archived decision's item is neither moved nor recorded, and comes off.
	_, read := answerTo(t, s, f.Token, "GET", meetingOf(f.OnceEvent), "")
	if shown := itemsOf(t, read)[0]; shown["mayMove"] != false || shown["mayRecordOutcome"] != false || shown["mayTakeOff"] != true || read["mayClose"] != true {
		t.Errorf("the item of an archived decision reads %v, mayClose %v", shown, read["mayClose"])
	}
	for _, write := range [][3]string{{"POST", "/moves", `{}`}, {"PUT", "/outcome", `{"outcome":"accepted"}`}} {
		if got, _ := answerTo(t, s, f.Token, write[0], path+archived.ID+write[1], write[2]); got != "404" {
			t.Errorf("%s on the item of an archived decision: %s", write[1], got)
		}
	}
	now := s.now
	meeting.ClosedAt = &now
	if got, _ := answerTo(t, s, f.Token, "DELETE", path+archived.ID, ""); got != "422 meetingClosed" || len(meeting.Items) != 1 {
		t.Errorf("taking off a closed meeting: %s", got)
	}
}

// An outcome puts the decision into the status of its role, makes the
// meeting the day and event it was taken on, changes the sentence with
// changes, and moves the current item on. The last one written stands, and
// one that defers takes back the day and the event this meeting had set.
func TestAnOutcomeChangesTheDecision(t *testing.T) {
	s, f := New(t)
	second := aSecondProposal(f)
	meeting := f.WeeklyEvent.MeetingOn("2026-10-26")
	item := meeting.PutOnAgenda(f.ProposedDecision, second)[0]
	path := meetingOf(f.WeeklyEvent) + "/agenda/" + item.ID + "/outcome?day=2026-10-26"
	decision := f.ProposedDecision

	got, answer := answerTo(t, s, f.Token, "PUT", path, `{"outcome":"acceptedWithChanges","sentence":" We buy two tents ","votesFor":7,"votesAgainst":2,"votes":"Show of hands"}`)
	items := itemsOf(t, answer)
	want := map[string]any{
		"outcome": "acceptedWithChanges", "status": statusResource(f.Decisions.StatusOfRole("holds")), "withChanges": true,
		"votes": "Show of hands", "votesFor": 7.0, "votesAgainst": 2.0, "abstentions": nil,
		"recordedBy": map[string]any{"id": f.Bruno.ID, "name": f.Bruno.Name}, "recordedAt": timestamp(s.now),
	}
	if got != "200" || !reflect.DeepEqual(items[0]["outcome"], want) {
		t.Fatalf("the outcome reads %s %v, want %v", got, items[0]["outcome"], want)
	}
	if items[0]["current"] != false || items[1]["current"] != true || items[0]["decision"].(map[string]any)["title"] != "We buy two tents" {
		t.Errorf("the agenda after the outcome: %v", items)
	}
	_, read := answerTo(t, s, f.Token, "GET", "/api/v1/decisions/"+decision.ID, "")
	if read["title"] != "We buy two tents" || read["status"].(map[string]any)["role"] != "holds" || read["decidedOn"] != "2026-10-26" ||
		read["decidedIn"].(map[string]any)["id"] != f.WeeklyEvent.ID || read["votesFor"] != 7.0 || read["votes"] != "Show of hands" {
		t.Errorf("the decision reads %v", read)
	}

	for outcome, role := range map[string]string{"rejected": "rejected", "accepted": "holds"} {
		if got, _ := answerTo(t, s, f.Token, "PUT", path, `{"outcome":"`+outcome+`"}`); got != "200" || s.statusRole(f.Decisions, decision.StatusID) != role || decision.DecidedOn != "2026-10-26" {
			t.Errorf("%s: %s, role %s, decided on %q", outcome, got, s.statusRole(f.Decisions, decision.StatusID), decision.DecidedOn)
		}
	}
	if decision.Title != "We buy two tents" {
		t.Errorf("a correction took the sentence back: %q", decision.Title)
	}
	if got, _ := answerTo(t, s, f.Token, "PUT", path, `{"outcome":"deferred"}`); got != "200" || s.statusRole(f.Decisions, decision.StatusID) != "deferred" || decision.DecidedOn != "" || decision.EventID != "" {
		t.Errorf("deferred: %s, decided on %q in %q", got, decision.DecidedOn, decision.EventID)
	}

	// A day and an event the decision had from elsewhere stay when deferred.
	other := meeting.Items[1]
	second.DecidedOn, second.EventID = "2026-09-01", f.OnceEvent.ID
	if got, _ := answerTo(t, s, f.Token, "PUT", meetingOf(f.WeeklyEvent)+"/agenda/"+other.ID+"/outcome?day=2026-10-26", `{"outcome":"deferred"}`); got != "200" || second.DecidedOn != "2026-09-01" || second.EventID != f.OnceEvent.ID {
		t.Errorf("deferred elsewhere: %s, decided on %q in %q", got, second.DecidedOn, second.EventID)
	}

	// A day and an event that name this very meeting are taken back, whoever
	// wrote them.
	second.DecidedOn, second.EventID = "2026-10-26", f.WeeklyEvent.ID
	if got, _ := answerTo(t, s, f.Token, "PUT", meetingOf(f.WeeklyEvent)+"/agenda/"+other.ID+"/outcome?day=2026-10-26", `{"outcome":"deferred"}`); got != "200" || second.DecidedOn != "" || second.EventID != "" {
		t.Errorf("deferred here: %s, decided on %q in %q", got, second.DecidedOn, second.EventID)
	}

	// A timed event that does not repeat decides on the day its start falls
	// on in UTC, whatever the day of its reader, and a deferral there takes
	// that day back.
	late := time.Date(2026, 10, 21, 22, 30, 0, 0, time.UTC)
	f.OnceEvent.StartsAt = &late
	once := f.OnceEvent.MeetingOn("").PutOnAgenda(second)[0]
	path = meetingOf(f.OnceEvent) + "/agenda/" + once.ID + "/outcome"
	if got, _ := answerTo(t, s, f.Token, "PUT", path, `{"outcome":"accepted"}`); got != "200" || second.DecidedOn != "2026-10-21" {
		t.Errorf("an event that does not repeat: %s, decided on %q", got, second.DecidedOn)
	}
	if got, _ := answerTo(t, s, f.Token, "PUT", path, `{"outcome":"deferred"}`); got != "200" || second.DecidedOn != "" {
		t.Errorf("deferred on an event that does not repeat: %s, decided on %q", got, second.DecidedOn)
	}
	all := f.AllDayEvent.MeetingOn("").PutOnAgenda(second)[0]
	if got, _ := answerTo(t, s, f.Token, "PUT", meetingOf(f.AllDayEvent)+"/agenda/"+all.ID+"/outcome", `{"outcome":"rejected"}`); got != "200" || second.DecidedOn != "2026-10-20" {
		t.Errorf("an all-day event: %s, decided on %q", got, second.DecidedOn)
	}
}

// A decision that comes into force by an outcome moves the earlier decision
// it supersedes, as any other way into force does.
func TestAnAcceptedDecisionMovesTheEarlierOne(t *testing.T) {
	s, f := New(t)
	f.ProposedDecision.EarlierID, f.ProposedDecision.Relation = f.Decision.ID, "supersedes"
	item := f.OnceEvent.MeetingOn("").PutOnAgenda(f.ProposedDecision)[0]
	path := meetingOf(f.OnceEvent) + "/agenda/" + item.ID + "/outcome"

	if got, _ := answerTo(t, s, f.Token, "PUT", path, `{"outcome":"rejected"}`); got != "200" || s.statusRole(f.Decisions, f.Decision.StatusID) != "holds" {
		t.Fatalf("rejected: %s, the earlier one is %s", got, s.statusRole(f.Decisions, f.Decision.StatusID))
	}
	if got, _ := answerTo(t, s, f.Token, "PUT", path, `{"outcome":"accepted"}`); got != "200" || s.statusRole(f.Decisions, f.Decision.StatusID) != "superseded" {
		t.Errorf("accepted: %s, the earlier one is %s", got, s.statusRole(f.Decisions, f.Decision.StatusID))
	}
}

// An outcome that changes only a count of votes keeps a version of the
// decision, which carries the counts; one that changes nothing keeps none.
func TestACountOfVotesKeepsAVersion(t *testing.T) {
	s, f := New(t)
	decision := f.ProposedDecision
	item := f.OnceEvent.MeetingOn("").PutOnAgenda(decision)[0]
	path := meetingOf(f.OnceEvent) + "/agenda/" + item.ID + "/outcome"

	for _, step := range []struct {
		body string
		kept int
	}{
		{`{"outcome":"accepted","votesFor":7}`, 1},
		{`{"outcome":"accepted","votesFor":8}`, 1},
		{`{"outcome":"accepted","abstentions":0}`, 1},
		{`{"outcome":"accepted","votesFor":8}`, 0},
	} {
		before := len(decision.Versions)
		if got, _ := answerTo(t, s, f.Token, "PUT", path, step.body); got != "200" || len(decision.Versions)-before != step.kept {
			t.Fatalf("%s: %s, %d versions kept, want %d", step.body, got, len(decision.Versions)-before, step.kept)
		}
	}
	newest := decision.Versions[len(decision.Versions)-1]
	_, version := answerTo(t, s, f.Token, "GET", "/api/v1/decisions/"+decision.ID+"/versions/"+newest.ID, "")
	if version["votesFor"] != 8.0 || version["abstentions"] != 0.0 || version["votesAgainst"] != nil {
		t.Errorf("the version counts %v, %v and %v", version["votesFor"], version["votesAgainst"], version["abstentions"])
	}
	if _, there := version["votesAgainst"]; !there {
		t.Error("the version carries no votesAgainst")
	}
}

// Before it looks for the status of an outcome, Minka gives a tool without
// statuses the set it starts with, and one with a proposed status and no
// deferred one the deferred status behind it. Any other role that is missing
// stays missing. A status removed since reads without ID, role and name on
// the outcome that used it.
func TestAToolIsGivenTheStatusesAnOutcomeNeeds(t *testing.T) {
	s, f := New(t)
	second := aSecondProposal(f)
	items := f.OnceEvent.MeetingOn("").PutOnAgenda(f.ProposedDecision, second)
	path := meetingOf(f.OnceEvent) + "/agenda/"

	f.Decisions.Statuses = slicesWithout(f.Decisions.Statuses, f.Decisions.StatusOfRole("deferred"))
	if got, _ := answerTo(t, s, f.Token, "PUT", path+items[0].ID+"/outcome", `{"outcome":"deferred"}`); got != "200" {
		t.Fatalf("a tool without the deferred status: %s", got)
	}
	if roles := rolesOf(f.Decisions); len(roles) != 8 || roles[1] != "proposed" || roles[2] != "deferred" {
		t.Errorf("the statuses are %v", roles)
	}

	deferred := f.Decisions.StatusOfRole("deferred")
	f.Decisions.Statuses = slicesWithout(slicesWithout(f.Decisions.Statuses, deferred), f.Decisions.StatusOfRole("proposed"))
	_, read := answerTo(t, s, f.Token, "GET", meetingOf(f.OnceEvent), "")
	gone := map[string]any{"id": nil, "name": "", "customName": nil, "role": nil}
	if status := itemsOf(t, read)[0]["outcome"].(map[string]any)["status"]; !reflect.DeepEqual(status, gone) {
		t.Errorf("the status of the outcome reads %v, want %v", status, gone)
	}
	if got, _ := answerTo(t, s, f.Token, "PUT", path+items[1].ID+"/outcome", `{"outcome":"deferred"}`); got != "422 meetingOutcomeHasNoStatus" || items[1].Outcome != nil {
		t.Errorf("a tool with neither a proposed nor a deferred status: %s", got)
	}

	// A refusal leaves the tool without the status it would have been given.
	f.Decisions.Statuses = []*DecisionStatus{{ID: newID(), Role: "proposed"}, {ID: newID(), Role: "holds"}}
	if got, _ := answerTo(t, s, f.Token, "PUT", path+items[1].ID+"/outcome", `{"outcome":"rejected"}`); got != "422 meetingOutcomeHasNoStatus" || !slices.Equal(rolesOf(f.Decisions), []string{"proposed", "holds"}) {
		t.Errorf("a refused outcome: %s, the statuses are %v", got, rolesOf(f.Decisions))
	}

	f.Decisions.Statuses = nil
	if got, _ := answerTo(t, s, f.Token, "PUT", path+items[1].ID+"/outcome", `{"outcome":"rejected"}`); got != "200" || len(f.Decisions.Statuses) != 8 || s.statusRole(f.Decisions, second.StatusID) != "rejected" {
		t.Errorf("a tool without statuses: %s, %v", got, rolesOf(f.Decisions))
	}
}

func rolesOf(tool *DecisionsTool) []string {
	var roles []string
	for _, status := range tool.Statuses {
		roles = append(roles, status.Role)
	}
	return roles
}

// A Decisions tool in the archive is still in use, since only one in the
// trash is not: its decisions go on an agenda and are moved there.
func TestADecisionOfAnArchivedToolGoesOnAnAgenda(t *testing.T) {
	s, f := New(t)
	f.Decisions.Archived = true
	second := aSecondProposal(f)
	path := meetingOf(f.OnceEvent)

	got, meeting := answerTo(t, s, f.Token, "POST", path+"/agenda", `{"decisionIds":["`+f.ProposedDecision.ID+`","`+second.ID+`"]}`)
	if got != "201" || itemsOf(t, meeting)[0]["mayMove"] != true || meeting["mayClose"] != true {
		t.Fatalf("put on: %s %v", got, meeting)
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path+"/agenda/"+itemsOf(t, meeting)[1]["id"].(string)+"/moves", `{}`); got != "200" {
		t.Errorf("the move: %s", got)
	}
}

// Of two counts that are none, Minka names the first in its own order.
func TestTheFirstMalformedCountIsNamed(t *testing.T) {
	s, f := New(t)
	item := f.OnceEvent.MeetingOn("").PutOnAgenda(f.ProposedDecision)[0]
	for range 8 {
		got, answer := answerTo(t, s, f.Token, "PUT", meetingOf(f.OnceEvent)+"/agenda/"+item.ID+"/outcome", `{"outcome":"accepted","abstentions":"x","votesAgainst":"y","votesFor":"z"}`)
		if message, _ := answer["error"].(map[string]any)["message"].(string); got != "422 decisionVoteCountMalformed" || !strings.Contains(message, `"votesFor"`) {
			t.Fatalf("%s %q", got, message)
		}
	}
}

// A vote field left out keeps its value, on the item and on the decision,
// and null clears it.
func TestACorrectionKeepsTheVotesItLeavesOutAndClearsTheNullOnes(t *testing.T) {
	s, f := New(t)
	item := f.OnceEvent.MeetingOn("").PutOnAgenda(f.ProposedDecision)[0]
	path := meetingOf(f.OnceEvent) + "/agenda/" + item.ID + "/outcome"
	votesOf := func(fields map[string]any) map[string]any {
		return map[string]any{"votes": fields["votes"], "votesFor": fields["votesFor"], "votesAgainst": fields["votesAgainst"], "abstentions": fields["abstentions"]}
	}

	for _, step := range []struct {
		body string
		want map[string]any
	}{
		{`{"outcome":"accepted","votesFor":7,"votesAgainst":2,"votes":"Show of hands"}`, map[string]any{"votes": "Show of hands", "votesFor": 7.0, "votesAgainst": 2.0, "abstentions": nil}},
		{`{"outcome":"rejected"}`, map[string]any{"votes": "Show of hands", "votesFor": 7.0, "votesAgainst": 2.0, "abstentions": nil}},
		{`{"outcome":"rejected","abstentions":1}`, map[string]any{"votes": "Show of hands", "votesFor": 7.0, "votesAgainst": 2.0, "abstentions": 1.0}},
		{`{"outcome":"rejected","votesFor":null,"votes":null}`, map[string]any{"votes": nil, "votesFor": nil, "votesAgainst": 2.0, "abstentions": 1.0}},
		{`{"outcome":"rejected","votesAgainst":0,"abstentions":null}`, map[string]any{"votes": nil, "votesFor": nil, "votesAgainst": 0.0, "abstentions": nil}},
	} {
		got, answer := answerTo(t, s, f.Token, "PUT", path, step.body)
		if got != "200" {
			t.Fatalf("%s: %s", step.body, got)
		}
		if recorded := votesOf(itemsOf(t, answer)[0]["outcome"].(map[string]any)); !reflect.DeepEqual(recorded, step.want) {
			t.Errorf("%s: the item counts %v, want %v", step.body, recorded, step.want)
		}
		_, decision := answerTo(t, s, f.Token, "GET", "/api/v1/decisions/"+f.ProposedDecision.ID, "")
		if counted := votesOf(decision); !reflect.DeepEqual(counted, step.want) {
			t.Errorf("%s: the decision counts %v, want %v", step.body, counted, step.want)
		}
	}
}

// An outcome is refused in Minka's order: the item, a first outcome on a
// closed meeting before the body, the word, a sentence where none belongs,
// the sentence itself, the votes, and last a tool without a status for it.
func TestAnOutcomeIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	second := aSecondProposal(f)
	meeting := f.OnceEvent.MeetingOn("")
	items := meeting.PutOnAgenda(f.ProposedDecision, second)
	path := meetingOf(f.OnceEvent) + "/agenda/"
	long := strings.Repeat("x", 101)

	for _, c := range []struct{ body, want string }{
		{`not even JSON`, "400 malformedBody"},
		{`{"outcome":1}`, "400 malformedBody"},
		{`{}`, "422 meetingOutcomeUnknown"},
		{`{"outcome":null}`, "422 meetingOutcomeUnknown"},
		{`{"outcome":"Accepted"}`, "422 meetingOutcomeUnknown"},
		{`{"outcome":"postponed","sentence":1}`, "422 meetingOutcomeUnknown"},
		{`{"outcome":"accepted","sentence":1}`, "400 malformedBody"},
		{`{"outcome":"accepted","sentence":"","votesFor":"seven"}`, "422 sentenceWithoutChanges"},
		{`{"outcome":"rejected","sentence":"We buy two tents"}`, "422 sentenceWithoutChanges"},
		{`{"outcome":"accepted","sentence":null}`, "200"},
		{`{"outcome":"acceptedWithChanges","votesFor":"seven"}`, "422 decisionTitleMissing"},
		{`{"outcome":"acceptedWithChanges","sentence":" "}`, "422 decisionTitleMissing"},
		{`{"outcome":"acceptedWithChanges","sentence":"` + strings.Repeat("x", 201) + `"}`, "422 decisionTitleTooLong"},
		{`{"outcome":"acceptedWithChanges","sentence":"a\tb"}`, "422 decisionTitleHasControlCharacters"},
		{`{"outcome":"accepted","votes":1}`, "400 malformedBody"},
		{`{"outcome":"accepted","votes":"` + long + `","votesFor":"seven"}`, "422 decisionVotesTooLong"},
		{`{"outcome":"accepted","votes":"a\tb"}`, "422 decisionVotesHasControlCharacters"},
		{`{"outcome":"accepted","votesFor":"seven"}`, "422 decisionVoteCountMalformed"},
		{`{"outcome":"accepted","votesFor":"7"}`, "422 decisionVoteCountMalformed"},
		{`{"outcome":"accepted","votesAgainst":7.5}`, "422 decisionVoteCountMalformed"},
		{`{"outcome":"accepted","abstentions":-1}`, "422 decisionVoteCountMalformed"},
		{`{"outcome":"accepted","abstentions":100000}`, "422 decisionVoteCountMalformed"},
		{`{"outcome":"accepted","abstentions":true}`, "422 decisionVoteCountMalformed"},
		{`{"outcome":"accepted","abstentions":99999,"votesFor":0}`, "200"},
	} {
		if got, _ := answerTo(t, s, f.Token, "PUT", path+items[0].ID+"/outcome", c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.body, got, c.want)
		}
	}
	if got, _ := answerTo(t, s, f.Token, "PUT", path+newID()+"/outcome", `not even JSON`); got != "404" {
		t.Errorf("an unknown item before the body: %s", got)
	}

	f.Decisions.Statuses = slicesWithout(f.Decisions.Statuses, f.Decisions.StatusOfRole("rejected"))
	if got, _ := answerTo(t, s, f.Token, "PUT", path+items[1].ID+"/outcome", `{"outcome":"rejected"}`); got != "422 meetingOutcomeHasNoStatus" || items[1].Outcome != nil {
		t.Errorf("a tool without the status: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "PUT", path+items[1].ID+"/outcome", `{"outcome":"rejected","votesFor":"seven"}`); got != "422 decisionVoteCountMalformed" {
		t.Errorf("the votes before the status: %s", got)
	}

	// A closed meeting takes a correction and no first outcome.
	now := s.now
	meeting.ClosedAt, meeting.ClosedByID = &now, f.Bruno.ID
	if got, _ := answerTo(t, s, f.Token, "PUT", path+items[1].ID+"/outcome", `{"outcome":"postponed"}`); got != "422 meetingClosed" {
		t.Errorf("a first outcome on a closed meeting: %s", got)
	}
	got, closed := answerTo(t, s, f.Token, "PUT", path+items[0].ID+"/outcome", `{"outcome":"deferred"}`)
	if got != "200" || items[0].Outcome.Outcome != "deferred" {
		t.Errorf("a correction on a closed meeting: %s", got)
	}
	shown := itemsOf(t, closed)
	flags := func(item map[string]any) [3]any {
		return [3]any{item["mayMove"], item["mayTakeOff"], item["mayRecordOutcome"]}
	}
	if flags(shown[0]) != [3]any{false, false, true} || flags(shown[1]) != [3]any{false, false, false} || closed["mayClose"] != false ||
		closed["closed"] != true || closed["closedAt"] != timestamp(now) || closed["closedBy"].(map[string]any)["id"] != f.Bruno.ID {
		t.Errorf("a closed meeting reads %v", closed)
	}
}

func slicesWithout(statuses []*DecisionStatus, gone *DecisionStatus) []*DecisionStatus {
	var rest []*DecisionStatus
	for _, status := range statuses {
		if status != gone {
			rest = append(rest, status)
		}
	}
	return rest
}

// Closing refuses in Minka's order: a closed meeting, the body, the word of
// the audience, the people and what they go with; then an empty agenda and
// items the writer may not change, both 404; last the people who cannot be
// told, which leaves the meeting open.
func TestClosingIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	path := meetingOf(f.OnceEvent) + "/close"
	meeting := f.OnceEvent.MeetingOn("")
	people := func(ids ...string) string { return `"notifyIds":["` + strings.Join(ids, `","`) + `"]` }

	for _, c := range []struct{ name, body, want string }{
		{"no JSON", `nonsense`, "400 malformedBody"},
		{"a list", `[]`, "400 malformedBody"},
		{"an audience that is no string", `{"audience":1}`, "400 malformedBody"},
		{"an unknown audience", `{"audience":"all",` + people("x") + `}`, "422 meetingAudienceUnknown"},
		{"people that are no list", `{"notifyIds":"x"}`, "400 malformedBody"},
		{"a person that is no identifier", `{"audience":"nobody",` + people("anke") + `}`, "422 personNotAProjectMember"},
		{"chosen without people", `{"audience":"chosen"}`, "422 notifyIdsMissing"},
		{"chosen with an empty list", `{"audience":"chosen","notifyIds":[]}`, "422 notifyIdsMissing"},
		{"people without an audience", `{` + people(f.Anke.ID) + `}`, "422 notifyIdsWithoutChosenAudience"},
		{"people for nobody", `{"audience":"nobody",` + people(f.Anke.ID) + `}`, "422 notifyIdsWithoutChosenAudience"},
		{"an empty agenda", `{}`, "404"},
		{"an empty agenda before the people", `{"audience":"chosen",` + people(f.Stranger.ID) + `}`, "404"},
	} {
		if got, _ := answerTo(t, s, f.Token, "POST", path, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	meeting.PutOnAgenda(f.ProposedDecision)
	for _, c := range []struct{ name, body, want string }{
		{"a stranger", `{"audience":"chosen",` + people(f.Anke.ID, f.Stranger.ID) + `}`, "422 personNotAProjectMember"},
		{"a client of an event not shown to clients", `{"audience":"chosen",` + people(f.Client.ID) + `}`, "422 personCannotSeeIt"},
		{"a client before a stranger", `{"audience":"chosen",` + people(f.Client.ID, f.Stranger.ID) + `}`, "422 personCannotSeeIt"},
	} {
		if got, _ := answerTo(t, s, f.Token, "POST", path, c.body); got != c.want || meeting.ClosedAt != nil || len(s.Notifications) != 0 {
			t.Errorf("%s: %s, want %s, and the meeting open", c.name, got, c.want)
		}
	}

	// A decision in the trash does not count, so its item alone is no agenda.
	f.ProposedDecision.Trashed = true
	if got, _ := answerTo(t, s, f.Token, "POST", path, `{}`); got != "404" {
		t.Errorf("an agenda of a trashed decision: %s", got)
	}
	f.ProposedDecision.Trashed = false

	got, closed := answerTo(t, s, f.Token, "POST", path, `{"audience":"chosen",`+people(strings.ToUpper(f.Anke.ID), f.Bruno.ID, f.Anke.ID)+`}`)
	if got != "200" || closed["closed"] != true || closed["closedBy"].(map[string]any)["id"] != f.Bruno.ID || closed["mayClose"] != false {
		t.Fatalf("closing: %s %v", got, closed)
	}
	if len(s.Notifications) != 1 || s.Notifications[0].PersonID != f.Anke.ID || s.Notifications[0].Occasion != "meeting.closed" {
		t.Errorf("the people told: %v", s.Notifications)
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path, `nonsense`); got != "422 meetingClosed" {
		t.Errorf("a second close before the body: %s", got)
	}
}

// Closing without an audience tells everybody of the project but the writer
// and the clients an event of the team is not shown to; nobody tells nobody.
func TestClosingTellsItsAudience(t *testing.T) {
	for audience, told := range map[string]bool{`{}`: true, `{"audience":null}`: true, `{"audience":"everybody"}`: true, `{"audience":"nobody"}`: false} {
		s, f := New(t)
		f.OnceEvent.MeetingOn("").PutOnAgenda(f.ProposedDecision)
		if got, _ := answerTo(t, s, f.Token, "POST", meetingOf(f.OnceEvent)+"/close", audience); got != "200" {
			t.Fatalf("%s: %s", audience, got)
		}
		var people []string
		for _, line := range s.Notifications {
			people = append(people, line.PersonID)
		}
		hearsOfIt := func(id string) bool {
			for _, person := range people {
				if person == id {
					return true
				}
			}
			return false
		}
		if !slices.Contains(f.Project.MemberIDs, f.Agent.ID) {
			t.Fatal("the agent is no member of the project, so the test asks nothing about agents")
		}
		if hearsOfIt(f.Anke.ID) != told || hearsOfIt(f.Bruno.ID) || hearsOfIt(f.Client.ID) || hearsOfIt(f.Stranger.ID) || hearsOfIt(f.Agent.ID) {
			t.Errorf("%s told %v", audience, people)
		}
	}
}

// A client sees only the decisions shown to them, numbered from 1 without a
// gap, may move and record those, names no hidden item as a neighbour, and
// closes nothing while an item is hidden from them.
func TestAClientReadsTheirOwnAgenda(t *testing.T) {
	s, f := New(t)
	second := aSecondProposal(f)
	token := withAClient(s, f, f.OnceEvent)
	meeting := f.OnceEvent.MeetingOn("")
	items := meeting.PutOnAgenda(second, f.ProposedDecision)
	hidden, shown := items[0], items[1]
	path := meetingOf(f.OnceEvent)

	got, read := answerTo(t, s, token, "GET", path, "")
	listed := itemsOf(t, read)
	if got != "200" || len(listed) != 1 || listed[0]["id"] != shown.ID || listed[0]["number"] != 1.0 || listed[0]["current"] != true ||
		listed[0]["mayMove"] != true || listed[0]["mayRecordOutcome"] != true || read["mayClose"] != false {
		t.Fatalf("the client reads %s %v", got, read)
	}
	for _, c := range []struct{ name, method, address, body, want string }{
		{"closing", "POST", "/close", `{}`, "404"},
		{"a hidden item", "POST", "/agenda/" + hidden.ID + "/moves", `{}`, "404"},
		{"a hidden outcome", "PUT", "/agenda/" + hidden.ID + "/outcome", `{"outcome":"accepted"}`, "404"},
		{"a hidden item taken off", "DELETE", "/agenda/" + hidden.ID, ``, "404"},
		{"a hidden neighbour", "POST", "/agenda/" + shown.ID + "/moves", `{"previousItemId":"` + hidden.ID + `"}`, "422 agendaPlaceUnknown"},
		{"a hidden decision", "POST", "/agenda", `{"decisionIds":["` + second.ID + `"]}`, "422 decisionUnknown"},
		{"their own item", "POST", "/agenda/" + shown.ID + "/moves", `{}`, "200"},
		{"their own outcome", "PUT", "/agenda/" + shown.ID + "/outcome", `{"outcome":"deferred"}`, "200"},
	} {
		if got, _ := answerTo(t, s, token, c.method, path+c.address, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	f.OnceEvent.ShownToClients = false
	if got, _ := answerTo(t, s, token, "GET", path, ""); got != "404" {
		t.Errorf("an event hidden from the client: %s", got)
	}
}

// A span names the day of each occurrence of a series, the day its schedule
// gives also where the occurrence was moved, leaves a cancelled day out, and
// names none for an event that does not repeat; My calendar does the same.
func TestAnOccurrenceNamesItsDay(t *testing.T) {
	s, f := New(t)
	start, end := time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC), time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	f.WeeklyEvent.MovedDays = map[string]EventMove{"2026-10-26": {StartsAt: &start, EndsAt: &end}, "2026-10-22": {StartsAt: &start, EndsAt: &end}}
	f.WeeklyEvent.CancelledDays = []string{"2026-11-02"}
	f.WeeklyEvent.ParticipantIDs = []string{f.Bruno.ID}
	want := []any{
		map[string]any{"startsAt": "2026-10-19T08:00:00.000000Z", "endsAt": "2026-10-19T08:30:00.000000Z", "firstDay": nil, "lastDay": nil, "day": "2026-10-19"},
		map[string]any{"startsAt": "2026-10-28T13:00:00.000000Z", "endsAt": "2026-10-28T14:00:00.000000Z", "firstDay": nil, "lastDay": nil, "day": "2026-10-26"},
		map[string]any{"startsAt": "2026-11-09T09:00:00.000000Z", "endsAt": "2026-11-09T09:30:00.000000Z", "firstDay": nil, "lastDay": nil, "day": "2026-11-09"},
	}

	for _, address := range []string{"/api/v1/calendars/" + f.Calendar.ID + "/events", "/api/v1/me/events"} {
		_, span := answerTo(t, s, f.Token, "GET", address+"?from=2026-10-19&to=2026-11-09", "")
		var series, once map[string]any
		for _, listed := range span["events"].([]any) {
			switch event := listed.(map[string]any); event["id"] {
			case f.WeeklyEvent.ID:
				series = event
			case f.AllDayEvent.ID, f.OnceEvent.ID:
				once = event
			}
		}
		if series == nil || !reflect.DeepEqual(series["occurrences"], want) {
			t.Errorf("%s: the series takes place at %v, want %v", address, series["occurrences"], want)
		}
		if once != nil {
			if day, there := once["occurrences"].([]any)[0].(map[string]any)["day"]; !there || day != nil {
				t.Errorf("%s: an event that does not repeat names the day %v", address, day)
			}
		}
		if _, there := series["day"]; there {
			t.Errorf("%s: the line itself names a day", address)
		}
	}

	// The meeting of the moved occurrence goes by the day of the schedule.
	if got, _ := answerTo(t, s, f.Token, "GET", meetingOf(f.WeeklyEvent)+"?day=2026-10-28", ""); got != "404" {
		t.Errorf("the day a moved occurrence takes place on: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "GET", meetingOf(f.WeeklyEvent)+"?day=2026-10-26", ""); got != "200" {
		t.Errorf("the day the schedule gives: %s", got)
	}
	// A span that only the moved occurrence reaches lists it.
	_, span := answerTo(t, s, f.Token, "GET", "/api/v1/calendars/"+f.Calendar.ID+"/events?from=2026-10-28&to=2026-10-28", "")
	if events := span["events"].([]any); len(events) != 1 || !reflect.DeepEqual(events[0].(map[string]any)["occurrences"], want[1:2]) {
		t.Errorf("the span of the moved day: %v", span["events"])
	}
}

// A Minka from before meetings has no address for one and names no day.
func TestAMinkaWithoutMeetings(t *testing.T) {
	s, f := New(t)
	s.KnowsNoMeetings = true
	for _, request := range [][2]string{{"GET", ""}, {"POST", "/agenda"}, {"POST", "/close"}, {"DELETE", "/agenda/" + newID()}, {"POST", "/agenda/" + newID() + "/moves"}, {"PUT", "/agenda/" + newID() + "/outcome"}} {
		got, answer := answerTo(t, s, f.Token, request[0], meetingOf(f.OnceEvent)+request[1], `{}`)
		if got != "404" || answer != nil {
			t.Errorf("%s %s: %s %v", request[0], request[1], got, answer)
		}
	}
	_, span := answerTo(t, s, f.Token, "GET", "/api/v1/calendars/"+f.Calendar.ID+"/events?from=2026-10-19&to=2026-10-19", "")
	if _, there := span["events"].([]any)[0].(map[string]any)["occurrences"].([]any)[0].(map[string]any)["day"]; there {
		t.Error("an occurrence names a day")
	}
}
