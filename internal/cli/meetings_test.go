package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

const seriesDay = "2026-10-26"

// withAnAgenda puts the proposed decision and a second one, DEC-9, on the
// agenda of the event that does not repeat, and hands over their items.
func withAnAgenda(e *testEnv) (first, second *fakeapi.AgendaItem) {
	f := e.fixture
	f.ProposedDecision.ShortID = "DEC-2"
	other := &fakeapi.Decision{ID: newTestID(), Title: "We move the party", StatusID: f.Decisions.StatusOfRole("proposed").ID,
		CreatedAt: f.ProposedDecision.CreatedAt, UpdatedAt: f.ProposedDecision.CreatedAt, CreatorID: f.Bruno.ID, ShortID: "DEC-9"}
	f.Decisions.Decisions = append(f.Decisions.Decisions, other)
	items := f.OnceEvent.MeetingOn("").PutOnAgenda(f.ProposedDecision, other)
	return items[0], items[1]
}

// meetingRequests are the requests about meetings that went out since a
// count of requests, each as its method, the end of its path and its query.
func (e *testEnv) meetingRequests(since int) []string {
	var sent []string
	for _, request := range e.server.Requests()[since:] {
		if at := strings.Index(request.Path, "/meeting"); at >= 0 {
			sent = append(sent, strings.TrimSpace(request.Method+" "+request.Path[at:]+" "+request.Query))
		}
	}
	return sent
}

func agendaOf(t *testing.T, env envelope) []map[string]any {
	t.Helper()
	var meeting struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &meeting); err != nil {
		t.Fatalf("data is no meeting: %s", env.Data)
	}
	return meeting.Items
}

// The meeting is handed out as Minka answers it, and the text view shows its
// state, the agenda numbered with the current item marked, each decision with
// its short identifier, status and address, each outcome with its votes and
// who recorded it, and what the reader may do as the flags say.
func TestAMeetingIsShownWithItsAgenda(t *testing.T) {
	e := newTestEnv(t)
	first, second := withAnAgenda(e)
	f := e.fixture
	seven, two := 7, 2
	first.Outcome = &fakeapi.MeetingOutcome{Outcome: "acceptedWithChanges", StatusID: f.Decisions.StatusOfRole("holds").ID, Votes: "Show of hands",
		VotesFor: &seven, VotesAgainst: &two, RecordedByID: f.Anke.ID, RecordedAt: time.Date(2026, 10, 21, 12, 30, 0, 0, time.UTC)}

	env, r := e.runJSON("", "events", "meeting", "show", f.OnceEvent.ID)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["eventId"] != f.OnceEvent.ID || data["mayClose"] != true || len(agendaOf(t, env)) != 2 {
		t.Fatalf("data %s", env.Data)
	}
	if want := "The meeting " + f.OnceEvent.Title + ", open: 2 items on the agenda, 1 with an outcome"; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
	for _, cmd := range []string{
		"minka events meeting show " + f.OnceEvent.ID,
		"minka events meeting agenda add " + f.OnceEvent.ID + " <decision>",
		"minka events meeting outcome " + f.OnceEvent.ID + " <item> <outcome>",
		"minka events meeting close " + f.OnceEvent.ID + " --audience everybody|chosen|nobody",
	} {
		if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Cmd == cmd }) {
			t.Errorf("breadcrumbs lack %q: %v", cmd, env.Breadcrumbs)
		}
	}

	r = e.run("", "events", "meeting", "show", f.OnceEvent.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, part := range []string{
		"# Meeting: " + f.OnceEvent.Title + "\nEvent: " + f.OnceEvent.ID + "\nState: open\nYou may close it.\nURL: " + e.server.URL,
		"\n  1. DEC-2 " + f.ProposedDecision.Title + "  [Proposed]\n     " + e.server.URL,
		"\n     Item: " + first.ID + "\n     Outcome: accepted-with-changes (In force); votes: 7 for, 2 against, Show of hands; recorded ",
		" by Anke\n     You may record its outcome, move it, take it off.\n",
		"\n> 2. DEC-9 We move the party  [Proposed]\n",
		"\n     Item: " + second.ID + "\n     You may record its outcome, move it, take it off.\n",
	} {
		if !strings.Contains(r.stdout, part) {
			t.Errorf("the text lacks %q:\n%s", part, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "Day:") {
		t.Errorf("an event that does not repeat names a day:\n%s", r.stdout)
	}

	// A closed meeting says who closed it and offers only what its flags do.
	closedAt := time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC)
	meeting := f.OnceEvent.MeetingOn("")
	meeting.ClosedAt, meeting.ClosedByID = &closedAt, f.Bruno.ID
	env, r = e.runJSON("", "events", "meeting", "show", f.OnceEvent.ID)
	expectCode(t, r, ExitOK)
	for _, gone := range []string{"agenda add", "meeting close"} {
		if slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return strings.Contains(b.Cmd, gone) }) {
			t.Errorf("a closed meeting offers %q: %v", gone, env.Breadcrumbs)
		}
	}
	r = e.run("", "events", "meeting", "show", f.OnceEvent.ID, "--styled")
	if !strings.Contains(r.stdout, "State: closed "+formatTime("2026-10-21T13:00:00.000000Z")+" by Bruno\n") || strings.Contains(r.stdout, "You may close it") ||
		!strings.Contains(r.stdout, "by Anke\n     You may record its outcome.\n") || strings.Contains(r.stdout, "move it") {
		t.Errorf("the closed meeting reads:\n%s", r.stdout)
	}

	e.fixture.OnceEvent.Meetings = nil
	r = e.run("", "events", "meeting", "show", f.OnceEvent.ID, "--styled")
	if !strings.Contains(r.stdout, "State: open\nURL:") || !strings.Contains(r.stdout, "\nNo items on the agenda.\n") {
		t.Errorf("an empty agenda reads:\n%s", r.stdout)
	}
}

// The day goes into the query of every request and into no body, is sent
// whenever given, and is refused before any request where it is no day. A
// series without it is told where its days are found.
func TestTheDayOfASeriesGoesIntoTheQuery(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.ProposedDecision.ShortID = "DEC-2"
	item := f.WeeklyEvent.MeetingOn(seriesDay).PutOnAgenda(f.ProposedDecision)[0]
	event := f.WeeklyEvent.ID
	query := "day=" + seriesDay

	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"show", event}, []string{"GET /meeting " + query}},
		{[]string{"agenda", "move", event, "1", "--top"}, []string{"GET /meeting " + query, "POST /meeting/agenda/" + item.ID + "/moves " + query}},
		{[]string{"outcome", event, "DEC-2", "deferred"}, []string{"GET /meeting " + query, "PUT /meeting/agenda/" + item.ID + "/outcome " + query}},
		{[]string{"agenda", "remove", event, item.ID}, []string{"GET /meeting " + query, "DELETE /meeting/agenda/" + item.ID + " " + query}},
		{[]string{"agenda", "add", event, f.ProposedDecision.ID}, []string{"POST /meeting/agenda " + query}},
		{[]string{"close", event, "--audience", "nobody"}, []string{"POST /meeting/close " + query}},
	} {
		before := len(e.server.Requests())
		env, r := e.runJSON("", append(append([]string{"events", "meeting"}, c.args...), "--day", seriesDay)...)
		expectCode(t, r, ExitOK)
		if sent := e.meetingRequests(before); !slices.Equal(sent, c.want) {
			t.Errorf("%v sent %v, want %v", c.args, sent, c.want)
		}
		for _, request := range e.server.Requests()[before:] {
			if strings.Contains(string(request.Body), "day") {
				t.Errorf("%v carries the day in the body %s", c.args, request.Body)
			}
		}
		if c.args[0] == "show" && (decode(t, env.Data)["day"] != seriesDay || !strings.Contains(env.Summary, " on "+seriesDay)) {
			t.Errorf("the meeting of a day: summary %q, data %s", env.Summary, env.Data)
		}
	}

	// An event that does not repeat is sent the day as well: Minka reads none.
	before := len(e.server.Requests())
	_, r := e.runJSON("", "events", "meeting", "show", f.OnceEvent.ID, "--day", seriesDay)
	expectCode(t, r, ExitOK)
	if sent := e.meetingRequests(before); !slices.Equal(sent, []string{"GET /meeting " + query}) {
		t.Errorf("an event that does not repeat was asked %v", sent)
	}

	before = len(e.server.Requests())
	for _, day := range []string{"26.10.2026", "2026-10-32", "2026-1-5", "", "today"} {
		for _, args := range [][]string{
			{"show", event}, {"agenda", "add", event, "DEC-2"}, {"agenda", "move", event, "1", "--top"}, {"agenda", "remove", event, "1"},
			{"outcome", event, "1", "accepted"}, {"close", event, "--audience", "nobody"},
		} {
			env, r := e.runJSON("", append(append([]string{"events", "meeting"}, args...), "--day", day)...)
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Error, "is no day") {
				t.Errorf("%v --day %q: %q", args, day, env.Error)
			}
		}
	}
	if sent := e.server.Requests()[before:]; len(sent) != 0 {
		t.Fatalf("a day that is none sent %d requests, the first to %s", len(sent), sent[0].Path)
	}

	env, r := e.runJSON("", "events", "meeting", "show", event)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "meetingDayMissing" || !strings.Contains(env.Hint, "occurrences[].day") || !strings.Contains(env.Hint, "minka events list") || !strings.Contains(env.Hint, "not the first day") {
		t.Errorf("a series without a day: %v, hint %q", env.Meta, env.Hint)
	}
	env, r = e.runJSON("", "events", "meeting", "show", event, "--day", "2026-10-27")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "no day its schedule gives") || strings.Contains(env.Error, "does not know") {
		t.Errorf("a day the series does not meet on: %q, hint %q", env.Error, env.Hint)
	}
}

// Decisions go on by ID or short identifier, all in one request with their
// IDs in the order given; a name that is neither is refused before anything
// is asked, and an unknown short identifier before anything is written.
func TestDecisionsGoOnTheAgendaInOneRequest(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.ProposedDecision.ShortID = "DEC-2"
	second := &fakeapi.Decision{ID: newTestID(), Title: "We move the party", StatusID: f.Decisions.StatusOfRole("deferred").ID,
		CreatedAt: f.ProposedDecision.CreatedAt, UpdatedAt: f.ProposedDecision.CreatedAt, CreatorID: f.Bruno.ID, ShortID: "DEC-9"}
	f.Decisions.Decisions = append(f.Decisions.Decisions, second)
	event := f.OnceEvent.ID

	before := len(e.server.Requests())
	for _, named := range [][]string{{"We buy a tent"}, {"DEC-2", "not a name"}, {""}} {
		_, r := e.runJSON("", append([]string{"events", "meeting", "agenda", "add", event}, named...)...)
		expectCode(t, r, ExitUsage)
	}
	_, r := e.runJSON("", "events", "meeting", "agenda", "add", event)
	expectCode(t, r, ExitUsage)
	if sent := e.server.Requests()[before:]; len(sent) != 0 {
		t.Fatalf("a name that is none sent %d requests", len(sent))
	}
	_, r = e.runJSON("", "events", "meeting", "agenda", "add", event, "dec-9", "DEC-77")
	expectCode(t, r, ExitNotFound)
	if len(e.server.WritingRequests()) != 0 {
		t.Fatal("an unknown short identifier wrote")
	}

	env, r := e.runJSON("", "events", "meeting", "agenda", "add", event, "dec-9", strings.ToUpper(f.ProposedDecision.ID))
	expectCode(t, r, ExitOK)
	if writes := e.server.WritingRequests(); len(writes) != 1 {
		t.Fatalf("%d writes, want one", len(writes))
	}
	if ids, _ := e.lastWriteBody()["decisionIds"].([]any); len(ids) != 2 || ids[0] != second.ID || ids[1] != f.ProposedDecision.ID {
		t.Errorf("the body names %v", e.lastWriteBody())
	}
	items := agendaOf(t, env)
	if env.Summary != "Put 2 decisions on the agenda of the meeting "+f.OnceEvent.Title || len(items) != 2 || items[0]["decision"].(map[string]any)["id"] != second.ID {
		t.Errorf("summary %q, data %s", env.Summary, env.Data)
	}

	for _, c := range []struct{ decision, code, hint string }{
		{"DEC-2", "decisionAlreadyOnMeetingAgenda", "none was put on"},
		{f.Decision.ID, "decisionNotForAMeetingAgenda", "proposed or deferred"},
		{f.ArchivedDecision.ID, "decisionUnknown", "the archive"},
	} {
		env, r := e.runJSON("", "events", "meeting", "agenda", "add", event, c.decision)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != c.code || !strings.Contains(env.Hint, c.hint) {
			t.Errorf("%s: %v, hint %q", c.decision, env.Meta, env.Hint)
		}
	}
}

// An item is named by its ID, its number or its decision; an unknown number
// or name is a usage error that lists the agenda, and an ID on no item goes
// to Minka, which does not find it.
func TestAnAgendaItemIsNamedInThreeWays(t *testing.T) {
	e := newTestEnv(t)
	first, second := withAnAgenda(e)
	f := e.fixture
	event := f.OnceEvent.ID
	meeting := f.OnceEvent.MeetingOn("")
	order := func() string {
		var ids []string
		for _, item := range meeting.Items {
			ids = append(ids, map[string]string{first.ID: "first", second.ID: "second"}[item.ID])
		}
		return strings.Join(ids, " ")
	}

	for _, c := range []struct {
		args []string
		body string
		want string
	}{
		{[]string{"2", "--top"}, `{}`, "second first"},
		{[]string{"dec-9", "--bottom"}, `{"previousItemId":"` + first.ID + `"}`, "first second"},
		{[]string{strings.ToUpper(second.ID), "--before", "DEC-2"}, `{"nextItemId":"` + first.ID + `"}`, "second first"},
		{[]string{second.DecisionID, "--after", "2"}, `{"previousItemId":"` + first.ID + `"}`, "first second"},
		{[]string{"1", "--after", strings.ToUpper(f.Decisions.Decisions[len(f.Decisions.Decisions)-1].ID)}, `{"previousItemId":"` + second.ID + `"}`, "second first"},
		{[]string{"2", "--top"}, `{}`, "first second"},
	} {
		env, r := e.runJSON("", append([]string{"events", "meeting", "agenda", "move", event}, c.args...)...)
		expectCode(t, r, ExitOK)
		writes := e.server.WritingRequests()
		if sent := strings.TrimSpace(string(writes[len(writes)-1].Body)); sent != c.body || order() != c.want {
			t.Errorf("%v sent %s and left %q, want %s and %q", c.args, sent, order(), c.body, c.want)
		}
		if !strings.HasPrefix(env.Summary, "Moved DEC-") || len(agendaOf(t, env)) != 2 {
			t.Errorf("%v: summary %q", c.args, env.Summary)
		}
	}

	writes := len(e.server.WritingRequests())
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"3", "--top"}, ExitNotFound, "there is no item 3, the agenda has 2 items"},
		{[]string{"DEC-77", "--top"}, ExitNotFound, `no item of the agenda matches "DEC-77"`},
		{[]string{"1"}, ExitUsage, "exactly one of --top, --bottom, --after and --before"},
		{[]string{"1", "--top", "--bottom"}, ExitUsage, "exactly one of --top, --bottom, --after and --before"},
	} {
		env, r := e.runJSON("", append([]string{"events", "meeting", "agenda", "move", event}, c.args...)...)
		expectCode(t, r, c.code)
		if !strings.Contains(env.Error, c.want) {
			t.Errorf("%v: %q, want %q", c.args, env.Error, c.want)
		}
	}
	env, r := e.runJSON("", "events", "meeting", "agenda", "move", event, "3", "--top")
	expectCode(t, r, ExitNotFound)
	if env.Code != CodeNotFound || !strings.Contains(env.Hint, "gone from the agenda") ||
		!strings.Contains(env.Hint, "Agenda: 1. DEC-2 "+f.ProposedDecision.Title+" ("+first.ID+"); 2. DEC-9") {
		t.Errorf("the item on no place: %q, hint %q", env.Code, env.Hint)
	}
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("an item that fits no place wrote")
	}

	// A neighbour on no item goes out as given, for Minka to judge.
	for _, neighbour := range []string{"We move the party", "3", "DEC-77"} {
		env, r = e.runJSON("", "events", "meeting", "agenda", "move", event, "1", "--after", neighbour)
		expectCode(t, r, ExitValidation)
		writes := e.server.WritingRequests()
		if sent := strings.TrimSpace(string(writes[len(writes)-1].Body)); env.Meta["apiCode"] != "agendaPlaceUnknown" || sent != `{"previousItemId":"`+neighbour+`"}` {
			t.Errorf("--after %q sent %s and got %v", neighbour, sent, env.Meta)
		}
	}
	writes = len(e.server.WritingRequests())

	env, r = e.runJSON("", "events", "meeting", "agenda", "move", event, newTestID(), "--top")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "none of this agenda") || len(e.server.WritingRequests()) != writes+1 {
		t.Errorf("an ID on no item: hint %q", env.Hint)
	}
	env, r = e.runJSON("", "events", "meeting", "agenda", "move", event, "1", "--after", "1")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "agendaPlaceUnknown" || !strings.Contains(env.Hint, "the item itself") {
		t.Errorf("the item as its own neighbour: %v, hint %q", env.Meta, env.Hint)
	}
}

// Taking an item off hands out what the command knows, since Minka answers
// without a body, and says that the decision stays. A second time is not
// found, as Minka answers it.
func TestAnAgendaItemIsTakenOff(t *testing.T) {
	e := newTestEnv(t)
	first, _ := withAnAgenda(e)
	f := e.fixture

	env, r := e.runJSON("", "events", "meeting", "agenda", "remove", f.OnceEvent.ID, "DEC-2")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["eventId"] != f.OnceEvent.ID || data["itemId"] != first.ID || data["decisionId"] != f.ProposedDecision.ID || data["day"] != nil || len(data) != 4 {
		t.Errorf("data %s", env.Data)
	}
	if want := "Took DEC-2 " + f.ProposedDecision.Title + " off the agenda of the meeting " + f.OnceEvent.Title + "; the decision itself stays"; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
	if len(f.OnceEvent.Meetings[""].Items) != 1 {
		t.Fatal("the item is still on the agenda")
	}

	env, r = e.runJSON("", "events", "meeting", "agenda", "remove", f.OnceEvent.ID, first.ID)
	expectCode(t, r, ExitNotFound)
	if env.Code != CodeNotFound || !strings.Contains(env.Hint, "it is gone") {
		t.Errorf("a second removal: %q, hint %q", env.Code, env.Hint)
	}
	for _, gone := range []string{"DEC-2", "2"} {
		env, r = e.runJSON("", "events", "meeting", "agenda", "remove", f.OnceEvent.ID, gone)
		expectCode(t, r, ExitNotFound)
		if env.Code != CodeNotFound || !strings.Contains(env.Hint, "gone from the agenda") {
			t.Errorf("removing %s, which is on no item: %q, hint %q", gone, env.Code, env.Hint)
		}
	}

	e.fixture.WeeklyEvent.MeetingOn(seriesDay).PutOnAgenda(f.ProposedDecision)
	env, r = e.runJSON("", "events", "meeting", "agenda", "remove", f.WeeklyEvent.ID, "1", "--day", seriesDay)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["day"] != seriesDay || !strings.Contains(env.Summary, " on "+seriesDay) {
		t.Errorf("of a series: summary %q, data %s", env.Summary, env.Data)
	}
}

// The words of the command line go out as Minka's, with exactly the vote
// fields given and nothing read and carried over; --no-votes sends null.
func TestAnOutcomeSendsExactlyWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	first, _ := withAnAgenda(e)
	f := e.fixture
	event := f.OnceEvent.ID

	for _, c := range []struct {
		args    []string
		body    string
		summary string
	}{
		{[]string{"accepted", "--votes-for", "7", "--votes-against", "2", "--votes", "Show of hands"},
			`{"outcome":"accepted","votes":"Show of hands","votesAgainst":2,"votesFor":7}`, "Recorded DEC-2 We buy a tent as accepted in the meeting "},
		{[]string{"rejected"}, `{"outcome":"rejected"}`, "Corrected the outcome of DEC-2 We buy a tent to rejected in the meeting "},
		{[]string{"deferred", "--abstentions", "0"}, `{"abstentions":0,"outcome":"deferred"}`, "Corrected the outcome of DEC-2 We buy a tent to deferred"},
		{[]string{"accepted-with-changes", "--sentence", "We buy two tents", "--no-votes"},
			`{"outcome":"acceptedWithChanges","sentence":"We buy two tents","votes":null}`, "Corrected the outcome of DEC-2 We buy a tent to accepted-with-changes"},
		{[]string{"accepted", "--votes", ""}, `{"outcome":"accepted","votes":""}`, "Corrected the outcome of DEC-2 We buy two tents to accepted"},
		{[]string{"accepted", "--votes-for", "7", "--votes-against", "2", "--abstentions", "0"},
			`{"abstentions":0,"outcome":"accepted","votesAgainst":2,"votesFor":7}`, "Corrected the outcome of DEC-2 We buy two tents to accepted"},
		{[]string{"accepted", "--no-votes-for", "--no-votes-against", "--no-abstentions"},
			`{"abstentions":null,"outcome":"accepted","votesAgainst":null,"votesFor":null}`, "Corrected the outcome of DEC-2 We buy two tents to accepted"},
	} {
		env, r := e.runJSON("", append([]string{"events", "meeting", "outcome", event, "1"}, c.args...)...)
		expectCode(t, r, ExitOK)
		writes := e.server.WritingRequests()
		if sent := strings.TrimSpace(string(writes[len(writes)-1].Body)); sent != c.body {
			t.Errorf("%v sent %s, want %s", c.args, sent, c.body)
		}
		if !strings.HasPrefix(env.Summary, c.summary) || len(agendaOf(t, env)) != 2 {
			t.Errorf("%v: summary %q, want it to start with %q", c.args, env.Summary, c.summary)
		}
	}
	// The counts taken off are gone from the item and from the decision.
	if got := first.Outcome; got.VotesFor != nil || got.VotesAgainst != nil || got.Abstentions != nil || got.Votes != "" {
		t.Errorf("the item counts %+v", got)
	}
	if d := f.ProposedDecision; d.VotesFor != nil || d.VotesAgainst != nil || d.Abstentions != nil {
		t.Errorf("the decision counts %v %v %v", d.VotesFor, d.VotesAgainst, d.Abstentions)
	}
	_, r := e.runJSON("", "events", "meeting", "outcome", event, "1", "accepted", "--votes-for", "7", "--votes-against", "2", "--abstentions", "0")
	expectCode(t, r, ExitOK)
	if role := f.Decisions.StatusOfRole("holds"); f.ProposedDecision.StatusID != role.ID || f.ProposedDecision.Title != "We buy two tents" {
		t.Errorf("the decision stands as %q in %s", f.ProposedDecision.Title, f.ProposedDecision.StatusID)
	}

	r = e.run("", "events", "meeting", "outcome", event, "DEC-2", "rejected", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.HasPrefix(r.stdout, "Corrected the outcome of DEC-2 We buy two tents to rejected in the meeting "+f.OnceEvent.Title+".\n  1. DEC-2 We buy two tents  [Rejected]\n") ||
		!strings.Contains(r.stdout, "Outcome: rejected (Rejected); votes: 7 for, 2 against, 0 abstained; recorded ") {
		t.Errorf("the text:\n%s", r.stdout)
	}
}

// What an outcome cannot be is refused before any request: a word that is
// none, a sentence where none belongs or none where one is needed, the note
// both given and taken off, and a count out of its range.
func TestAnOutcomeIsCheckedBeforeAnyRequest(t *testing.T) {
	e := newTestEnv(t)
	withAnAgenda(e)
	before := len(e.server.Requests())

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"acceptedWithChanges"}, "the outcome is accepted, accepted-with-changes, rejected or deferred"},
		{[]string{"postponed"}, "the outcome is accepted"},
		{[]string{"accepted", "--sentence", "We buy two tents"}, "--sentence goes with accepted-with-changes alone"},
		{[]string{"rejected", "--sentence", ""}, "--sentence goes with accepted-with-changes alone"},
		{[]string{"accepted-with-changes"}, "needs the changed sentence"},
		{[]string{"accepted-with-changes", "--sentence", " "}, "needs the changed sentence"},
		{[]string{"accepted", "--votes", "5 : 1", "--no-votes"}, "--votes and --no-votes contradict"},
		{[]string{"accepted", "--votes-for", "-1"}, "--votes-for is a whole number from 0 to 99999"},
		{[]string{"accepted", "--votes-against", "100000"}, "--votes-against is a whole number from 0 to 99999"},
		{[]string{"accepted", "--abstentions", "100000"}, "--abstentions is a whole number from 0 to 99999"},
		{[]string{"accepted", "--votes-for", "7", "--no-votes-for"}, "--votes-for and --no-votes-for contradict"},
		{[]string{"accepted", "--votes-against", "2", "--no-votes-against"}, "--votes-against and --no-votes-against contradict"},
		{[]string{"accepted", "--abstentions", "0", "--no-abstentions"}, "--abstentions and --no-abstentions contradict"},
	} {
		env, r := e.runJSON("", append([]string{"events", "meeting", "outcome", e.fixture.OnceEvent.ID, "1"}, c.args...)...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, c.want) {
			t.Errorf("%v: %q, want %q", c.args, env.Error, c.want)
		}
	}
	if sent := e.server.Requests()[before:]; len(sent) != 0 {
		t.Fatalf("%d requests went out, the first to %s", len(sent), sent[0].Path)
	}
}

// Minka's refusals of an outcome come with what can be done about them.
func TestAnOutcomeMinkaRefusesSaysWhatToDo(t *testing.T) {
	e := newTestEnv(t)
	_, second := withAnAgenda(e)
	f := e.fixture
	event := f.OnceEvent.ID

	env, r := e.runJSON("", "events", "meeting", "outcome", event, "1", "accepted", "--votes", strings.Repeat("x", 101))
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "decisionVotesTooLong" || !strings.Contains(env.Hint, "at most 100 characters") {
		t.Errorf("a long note: %v, hint %q", env.Meta, env.Hint)
	}
	env, r = e.runJSON("", "events", "meeting", "outcome", event, "1", "accepted-with-changes", "--sentence", strings.Repeat("x", 201))
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "decisionTitleTooLong" || !strings.Contains(env.Hint, "at most 200 characters") {
		t.Errorf("a long sentence: %v, hint %q", env.Meta, env.Hint)
	}

	var statuses []*fakeapi.DecisionStatus
	for _, status := range f.Decisions.Statuses {
		if status.Role != "rejected" {
			statuses = append(statuses, status)
		}
	}
	f.Decisions.Statuses = statuses
	env, r = e.runJSON("", "events", "meeting", "outcome", event, "1", "rejected")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "meetingOutcomeHasNoStatus" || !strings.Contains(env.Hint, "minka decisions statuses") {
		t.Errorf("a tool without the status: %v, hint %q", env.Meta, env.Hint)
	}

	now := time.Now()
	f.OnceEvent.MeetingOn("").ClosedAt = &now
	env, r = e.runJSON("", "events", "meeting", "outcome", event, "2", "accepted")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "meetingClosed" || !strings.Contains(env.Hint, "minka decisions status "+second.DecisionID) {
		t.Errorf("a first outcome on a closed meeting: %v, hint %q", env.Meta, env.Hint)
	}
	for _, args := range [][]string{{"agenda", "add", event, f.Decision.ID}, {"agenda", "move", event, "1", "--bottom"}, {"agenda", "remove", event, "1"}, {"close", event, "--audience", "nobody"}} {
		env, r := e.runJSON("", append([]string{"events", "meeting"}, args...)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != "meetingClosed" || !strings.Contains(env.Hint, "closed") {
			t.Errorf("%v on a closed meeting: %v, hint %q", args, env.Meta, env.Hint)
		}
	}
}

// Closing needs its audience said, so that nobody is told by a flag left
// out; the people are named as for a decision and go out as IDs.
func TestClosingNeedsItsAudience(t *testing.T) {
	e := newTestEnv(t)
	withAnAgenda(e)
	f := e.fixture
	event := f.OnceEvent.ID

	before := len(e.server.Requests())
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "say whom the closed meeting is told to: --audience everybody, chosen or nobody"},
		{[]string{"--notify", "Anke"}, "say whom the closed meeting is told to"},
		{[]string{"--audience", "all"}, `--audience is everybody, chosen or nobody, not "all"`},
		{[]string{"--audience", ""}, "--audience is everybody, chosen or nobody"},
	} {
		env, r := e.runJSON("", append([]string{"events", "meeting", "close", event}, c.args...)...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, c.want) {
			t.Errorf("%v: %q, want %q", c.args, env.Error, c.want)
		}
	}
	if sent := e.server.Requests()[before:]; len(sent) != 0 || f.OnceEvent.Meetings[""].ClosedAt != nil {
		t.Fatalf("%d requests went out without an audience", len(sent))
	}

	for _, c := range []struct {
		args       []string
		code, hint string
	}{
		{[]string{"--audience", "chosen"}, "notifyIdsMissing", "one --notify per person"},
		{[]string{"--audience", "everybody", "--notify", "Anke"}, "notifyIdsWithoutChosenAudience", "--audience chosen"},
		{[]string{"--audience", "chosen", "--notify", f.Stranger.ID}, "personNotAProjectMember", "minka people list"},
		{[]string{"--audience", "chosen", "--notify", f.Client.ID}, "personCannotSeeIt", "The meeting stays open"},
	} {
		env, r := e.runJSON("", append([]string{"events", "meeting", "close", event}, c.args...)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != c.code || !strings.Contains(env.Hint, c.hint) || f.OnceEvent.Meetings[""].ClosedAt != nil {
			t.Errorf("%v: %v, hint %q", c.args, env.Meta, env.Hint)
		}
	}

	env, r := e.runJSON("", "events", "meeting", "close", event, "--audience", "chosen", "--notify", "Anke", "--notify", "me")
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if ids, _ := body["notifyIds"].([]any); body["audience"] != "chosen" || len(ids) != 2 || ids[0] != f.Anke.ID || ids[1] != f.Bruno.ID || len(body) != 2 {
		t.Errorf("the body went out as %v", body)
	}
	data := decode(t, env.Data)
	if env.Summary != "Closed the meeting "+f.OnceEvent.Title+", with the audience chosen" || data["closed"] != true || strings.Contains(env.Summary, "Anke") {
		t.Errorf("summary %q, data %s", env.Summary, env.Data)
	}
	if len(e.server.Notifications) != 1 || e.server.Notifications[0].PersonID != f.Anke.ID {
		t.Errorf("told %v", e.server.Notifications)
	}

	env, r = e.runJSON("", "events", "meeting", "close", event, "--audience", "nobody")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "meetingClosed" || !strings.Contains(env.Hint, "closed already") {
		t.Errorf("a second close: %v, hint %q", env.Meta, env.Hint)
	}
}

// Minka answers an empty agenda and items the writer may not change with the
// same 404 as an event that is not there, so the hint names all of it and
// does not pretend to know which.
func TestClosingWhatCannotBeClosedSaysBothReasons(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "events", "meeting", "close", e.fixture.OnceEvent.ID, "--audience", "everybody")
	expectCode(t, r, ExitNotFound)
	for _, part := range []string{"nothing to close here, or you may not close it", "the agenda is empty", "an item you may not change", "not there for you"} {
		if !strings.Contains(env.Hint, part) {
			t.Errorf("the hint lacks %q: %q", part, env.Hint)
		}
	}
	if strings.Contains(env.Error, "does not know meetings") {
		t.Errorf("an empty agenda reads as an old Minka: %q", env.Error)
	}
}

// A Minka without meetings answers with a page of its web: every command
// says the host does not know meetings, as a feature that is not there.
func TestAMinkaWithoutMeetings(t *testing.T) {
	e := newTestEnv(t)
	first, _ := withAnAgenda(e)
	event := e.fixture.OnceEvent.ID
	e.server.KnowsNoMeetings = true

	for _, args := range [][]string{
		{"show", event},
		{"agenda", "add", event, e.fixture.Decision.ID},
		{"agenda", "move", event, first.ID, "--top"},
		{"agenda", "remove", event, first.ID},
		{"outcome", event, first.ID, "accepted"},
		{"close", event, "--audience", "nobody"},
		{"close", event, "--audience", "chosen", "--notify", "Anke"},
	} {
		env, r := e.runJSON("", append([]string{"events", "meeting"}, args...)...)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Error, "does not know meetings") || env.Hint != meetingsUnknownHint {
			t.Errorf("%v: %q, hint %q", args, env.Error, env.Hint)
		}
	}

	r := e.run("", "events", "list", e.fixture.Calendar.ID, "--from", "2026-10-19", "--to", "2026-10-26", "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "of the series") {
		t.Errorf("a span without days names one:\n%s", r.stdout)
	}
}

// A span hands the day of every occurrence on, and the text view names it
// where the occurrence does not start on it, as for one that was moved.
func TestAnOccurrenceShowsTheDayThatNamesItsMeeting(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	// An all-day series, whose days are the same in every zone a test runs in:
	// Tuesdays, the second one moved to the Thursday after.
	market := &fakeapi.CalendarEvent{
		ID: newTestID(), Title: "Market day", FirstDay: "2026-10-20", LastDay: "2026-10-20", CreatedAt: f.OnceEvent.CreatedAt, CreatorID: f.Bruno.ID,
		Repetition:     &fakeapi.EventRepetition{Schedule: fakeapi.RepetitionSchedule{Rhythm: fakeapi.RepeatsWeekly, Weekdays: []int{2}, Interval: 1, StartsOn: "2026-10-20"}},
		MovedDays:      map[string]fakeapi.EventMove{"2026-10-27": {FirstDay: "2026-10-29", LastDay: "2026-10-29"}},
		ParticipantIDs: []string{f.Bruno.ID},
	}
	f.Calendar.Events = append(f.Calendar.Events, market)

	for _, command := range [][]string{{"events", "list", f.Calendar.ID}, {"events", "mine"}} {
		args := append(command, "--from", "2026-10-19", "--to", "2026-10-29")
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitOK)
		var span struct {
			Events []struct {
				ID          string           `json:"id"`
				Occurrences []map[string]any `json:"occurrences"`
			} `json:"events"`
		}
		if err := json.Unmarshal(env.Data, &span); err != nil {
			t.Fatal(err)
		}
		for _, event := range span.Events {
			switch event.ID {
			case f.WeeklyEvent.ID:
			case market.ID:
				if len(event.Occurrences) != 2 || event.Occurrences[0]["day"] != "2026-10-20" || event.Occurrences[1]["day"] != "2026-10-27" || event.Occurrences[1]["firstDay"] != "2026-10-29" {
					t.Errorf("%s: the series takes place at %v", command[1], event.Occurrences)
				}
			default:
				if day, there := event.Occurrences[0]["day"]; !there || day != nil {
					t.Errorf("%s: an event that does not repeat names the day %v", command[1], day)
				}
			}
		}

		r = e.run("", append(args, "--styled")...)
		expectCode(t, r, ExitOK)
		if want := "  2026-10-20, all day\n  2026-10-29, all day  (day 2026-10-27 of the series)\n"; !strings.Contains(r.stdout, want) {
			t.Errorf("%s lacks %q:\n%s", command[1], want, r.stdout)
		}
	}

	env, r := e.runJSON("", "events", "show", f.WeeklyEvent.ID)
	expectCode(t, r, ExitOK)
	if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool {
		return b.Cmd == "minka events meeting show "+f.WeeklyEvent.ID+" --day YYYY-MM-DD"
	}) {
		t.Errorf("a series does not point at its meetings: %v", env.Breadcrumbs)
	}
	env, r = e.runJSON("", "events", "show", f.OnceEvent.ID)
	expectCode(t, r, ExitOK)
	if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Cmd == "minka events meeting show "+f.OnceEvent.ID }) {
		t.Errorf("an event does not point at its meeting: %v", env.Breadcrumbs)
	}
}
