package cli

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// data is the meeting as Minka answered it: a field a newer Minka adds
// reaches the reader through every command that hands a meeting out.
func TestAMeetingIsHandedOutAsMinkaAnsweredIt(t *testing.T) {
	e := newTestEnv(t)
	withAnAgenda(e)
	f := e.fixture
	e.server.NewerMeetingFields = map[string]any{"somethingNew": "of tomorrow"}
	event := f.OnceEvent.ID
	third := &fakeapi.Decision{ID: newTestID(), Title: "A third", StatusID: f.Decisions.StatusOfRole("proposed").ID, CreatedAt: f.ProposedDecision.CreatedAt, CreatorID: f.Bruno.ID}
	f.Decisions.Decisions = append(f.Decisions.Decisions, third)

	for _, args := range [][]string{
		{"show", event},
		{"agenda", "add", event, third.ID},
		{"agenda", "move", event, "1", "--bottom"},
		{"outcome", event, "1", "deferred"},
		{"close", event, "--audience", "nobody"},
	} {
		env, r := e.runJSON("", append([]string{"events", "meeting"}, args...)...)
		expectCode(t, r, ExitOK)
		if decode(t, env.Data)["somethingNew"] != "of tomorrow" {
			t.Errorf("%v dropped a field of the answer: %s", args, env.Data)
		}
	}
}

// --bottom puts the last item of the agenda behind the item before it, never
// behind itself, and sends no neighbour where the item is the only one.
func TestTheLastItemGoesToTheBottom(t *testing.T) {
	e := newTestEnv(t)
	first, second := withAnAgenda(e)
	event := e.fixture.OnceEvent.ID
	lastBody := func() string {
		writes := e.server.WritingRequests()
		return strings.TrimSpace(string(writes[len(writes)-1].Body))
	}

	_, r := e.runJSON("", "events", "meeting", "agenda", "move", event, "2", "--bottom")
	expectCode(t, r, ExitOK)
	if want := `{"previousItemId":"` + first.ID + `"}`; lastBody() != want {
		t.Errorf("the last item went out with %s, want %s", lastBody(), want)
	}

	meeting := e.fixture.OnceEvent.MeetingOn("")
	meeting.Items = []*fakeapi.AgendaItem{second}
	_, r = e.runJSON("", "events", "meeting", "agenda", "move", event, "1", "--bottom")
	expectCode(t, r, ExitOK)
	if lastBody() != `{}` {
		t.Errorf("the only item went out with %s", lastBody())
	}
}

// Where the last item as read is gone by the move, the hint says so and does
// not blame --after or --before, which were not given.
func TestTheBottomSaysWhenTheLastItemIsGone(t *testing.T) {
	e := newTestEnv(t)
	withAnAgenda(e)
	event := e.fixture.OnceEvent.ID
	e.server.BeforeMovingAgendaItem = func(meeting *fakeapi.Meeting) { meeting.Items = meeting.Items[:1] }

	env, r := e.runJSON("", "events", "meeting", "agenda", "move", event, "1", "--bottom")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "agendaPlaceUnknown" || strings.Contains(env.Hint, "--after") ||
		!strings.Contains(env.Hint, "was last on the agenda when it was read is gone") || !strings.HasSuffix(env.Hint, "minka events meeting show "+event) {
		t.Errorf("%v, hint %q", env.Meta, env.Hint)
	}
}

// A cancelled day of a series reads as cancelled, offers no decision for its
// agenda, and a decision sent to it all the same is told why it is not found.
func TestACancelledDayReadsAsCancelled(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.WeeklyEvent.CancelledDays = []string{seriesDay}
	event := f.WeeklyEvent.ID

	env, r := e.runJSON("", "events", "meeting", "show", event, "--day", seriesDay)
	expectCode(t, r, ExitOK)
	if want := "The meeting " + f.WeeklyEvent.Title + " on " + seriesDay + ", cancelled: 0 items on the agenda, 0 with an outcome"; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
	if slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Action == "add" || b.Action == "close" || b.Action == "outcome" }) {
		t.Errorf("a cancelled day offers a change: %v", env.Breadcrumbs)
	}
	r = e.run("", "events", "meeting", "show", event, "--day", seriesDay, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "\nDay: "+seriesDay+"\nState: cancelled, this day of the series does not take place\nURL: ") {
		t.Errorf("the cancelled day reads:\n%s", r.stdout)
	}

	env, r = e.runJSON("", "events", "meeting", "agenda", "add", event, f.ProposedDecision.ID, "--day", seriesDay)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "a cancelled day takes no agenda") {
		t.Errorf("a decision for a cancelled day: hint %q", env.Hint)
	}
}

// The breadcrumbs of a series name its day again, and offer an outcome only
// where an item says one may be recorded.
func TestTheBreadcrumbsOfAMeetingFollowItsDayAndItsFlags(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	meeting := f.WeeklyEvent.MeetingOn(seriesDay)
	meeting.PutOnAgenda(f.ProposedDecision)
	event := f.WeeklyEvent.ID

	env, r := e.runJSON("", "events", "meeting", "show", event, "--day", seriesDay)
	expectCode(t, r, ExitOK)
	for _, cmd := range []string{
		"minka events meeting show " + event + " --day " + seriesDay,
		"minka events meeting agenda add " + event + " <decision> --day " + seriesDay,
		"minka events meeting outcome " + event + " <item> <outcome> --day " + seriesDay,
		"minka events meeting close " + event + " --day " + seriesDay + " --audience everybody|chosen|nobody",
	} {
		if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Cmd == cmd }) {
			t.Errorf("breadcrumbs lack %q: %v", cmd, env.Breadcrumbs)
		}
	}

	// Closed, and no item has an outcome to correct: nothing may be recorded.
	now := time.Now()
	meeting.ClosedAt = &now
	env, r = e.runJSON("", "events", "meeting", "show", event, "--day", seriesDay)
	expectCode(t, r, ExitOK)
	if slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Action == "outcome" }) {
		t.Errorf("an outcome is offered where none may be recorded: %v", env.Breadcrumbs)
	}

	env, r = e.runJSON("", "events", "list", f.Calendar.ID, "--from", "2026-10-19", "--to", "2026-10-25")
	expectCode(t, r, ExitOK)
	if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool {
		return b.Cmd == "minka events meeting show <eventId> --day <occurrences[].day>"
	}) {
		t.Errorf("a span does not point at the meetings: %v", env.Breadcrumbs)
	}
}

// The text view marks the decision of an item that lies in the archive, and
// says of an outcome whose word only a newer Minka knows that it does not
// know it.
func TestTheTextViewOfAnArchivedDecisionAndAnUnknownOutcome(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	items := f.OnceEvent.MeetingOn("").PutOnAgenda(f.ArchivedDecision, f.ProposedDecision)
	items[1].Outcome = &fakeapi.MeetingOutcome{StatusID: f.Decisions.StatusOfRole("holds").ID, RecordedByID: f.Anke.ID, RecordedAt: time.Date(2026, 10, 21, 12, 30, 0, 0, time.UTC)}

	r := e.run("", "events", "meeting", "show", f.OnceEvent.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, part := range []string{
		f.ArchivedDecision.Title + "  [In force, archived]\n",
		f.ProposedDecision.Title + "  [Proposed]\n",
		"     Outcome: an outcome this minka does not know (In force); recorded ",
	} {
		if !strings.Contains(r.stdout, part) {
			t.Errorf("the text lacks %q:\n%s", part, r.stdout)
		}
	}
}

// An occurrence names the day of its series only where it does not start on
// that day, by the local day of this machine for a timed one.
func TestAnOccurrenceNamesItsDayOnlyWhereItDiffers(t *testing.T) {
	noon := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local).Format(time.RFC3339Nano)
	later := time.Date(2026, 10, 28, 13, 0, 0, 0, time.Local).Format(time.RFC3339Nano)
	timed := api.EventTime{StartsAt: &noon, EndsAt: &later}
	first := "2026-10-29"
	allDay := api.EventTime{FirstDay: &first, LastDay: &first}
	day := func(value string) *string { return &value }

	for _, c := range []struct {
		name       string
		occurrence api.EventOccurrence
		want       string
	}{
		{"a timed one on its day", api.EventOccurrence{EventTime: timed, Day: day("2026-10-28")}, ""},
		{"a timed one moved", api.EventOccurrence{EventTime: timed, Day: day("2026-10-26")}, "  (day 2026-10-26 of the series)"},
		{"an all-day one on its day", api.EventOccurrence{EventTime: allDay, Day: day("2026-10-29")}, ""},
		{"an all-day one moved", api.EventOccurrence{EventTime: allDay, Day: day("2026-10-27")}, "  (day 2026-10-27 of the series)"},
		{"an event that does not repeat", api.EventOccurrence{EventTime: timed}, ""},
	} {
		if got := meetingDayOf(c.occurrence); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// What Minka refuses of a sentence, a note or a closed agenda comes with the
// hint of its own case.
func TestTheHintsOfASentenceANoteAndAClosedAgenda(t *testing.T) {
	e := newTestEnv(t)
	withAnAgenda(e)
	f := e.fixture
	event := f.OnceEvent.ID

	env, r := e.runJSON("", "events", "meeting", "outcome", event, "1", "accepted-with-changes", "--sentence", "We buy\ttwo tents")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "decisionTitleHasControlCharacters" || !strings.Contains(env.Hint, "--sentence is one line") {
		t.Errorf("a sentence with a tab: %v, hint %q", env.Meta, env.Hint)
	}
	env, r = e.runJSON("", "events", "meeting", "outcome", event, "1", "accepted", "--votes", "5\t1")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "decisionVotesHasControlCharacters" || !strings.Contains(env.Hint, "--votes is a note of one line") {
		t.Errorf("a note with a tab: %v, hint %q", env.Meta, env.Hint)
	}

	now := time.Now()
	f.OnceEvent.MeetingOn("").ClosedAt = &now
	env, r = e.runJSON("", "events", "meeting", "agenda", "add", event, f.Decision.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "meetingClosed" || !strings.Contains(env.Hint, "the agenda of another meeting") {
		t.Errorf("a decision for a closed meeting: %v, hint %q", env.Meta, env.Hint)
	}
}

// The outcome is taken in any case, as the role of a status is, and goes out
// in Minka's word.
func TestAnOutcomeIsTakenInAnyCase(t *testing.T) {
	e := newTestEnv(t)
	withAnAgenda(e)
	env, r := e.runJSON("", "events", "meeting", "outcome", e.fixture.OnceEvent.ID, "1", "Accepted-With-Changes", "--sentence", "We buy two tents")
	expectCode(t, r, ExitOK)
	if e.lastWriteBody()["outcome"] != "acceptedWithChanges" || !strings.Contains(env.Summary, " as accepted-with-changes in ") {
		t.Errorf("the body %v, summary %q", e.lastWriteBody(), env.Summary)
	}
}

// A decision shows the votes a meeting counted beside the note, in the form
// the outcome of the meeting shows them, and so does a version of it. A
// status is named by the role deferred as by any other.
func TestADecisionShowsTheVotesCounted(t *testing.T) {
	e := newTestEnv(t)
	first, _ := withAnAgenda(e)
	f := e.fixture
	_ = first
	runOK(t, e, "events", "meeting", "outcome", f.OnceEvent.ID, "1", "accepted", "--votes-for", "7", "--votes-against", "2", "--votes", "Show of hands")

	env := runOK(t, e, "decisions", "show", f.ProposedDecision.ID)
	if data := decode(t, env.Data); data["votesFor"] != 7.0 || data["votesAgainst"] != 2.0 || data["abstentions"] != nil {
		t.Errorf("data %s", env.Data)
	}
	r := e.run("", "decisions", "show", f.ProposedDecision.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "\nVotes: 7 for, 2 against, Show of hands\n") {
		t.Errorf("the decision reads:\n%s", r.stdout)
	}
	versions := f.ProposedDecision.Versions
	r = e.run("", "decisions", "versions", "show", f.ProposedDecision.ID, versions[len(versions)-1].ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "\nVotes: 7 for, 2 against, Show of hands\n") {
		t.Errorf("the version reads:\n%s", r.stdout)
	}
	r = e.run("", "decisions", "show", f.Decision.ID, "--styled")
	if strings.Contains(r.stdout, "Votes:") {
		t.Errorf("a decision nobody voted on names votes:\n%s", r.stdout)
	}

	env = runOK(t, e, "decisions", "status", f.DraftDecision.ID, "Deferred")
	if role := decode(t, env.Data)["status"].(map[string]any)["role"]; role != "deferred" {
		t.Errorf("the status named deferred is %v", role)
	}
	env, r = e.runJSON("", "decisions", "status", f.DraftDecision.ID, "postponed")
	expectCode(t, r, ExitValidation)
	if !strings.Contains(env.Hint, "proposed, deferred, holds") {
		t.Errorf("the hint does not name the role: %q", env.Hint)
	}
}

// A count of nought was counted: the line says "0 for" and does not leave it
// out as it leaves out a count nobody took.
func TestACountOfNoughtIsShown(t *testing.T) {
	e := newTestEnv(t)
	withAnAgenda(e)
	f := e.fixture
	runOK(t, e, "events", "meeting", "outcome", f.OnceEvent.ID, "1", "rejected", "--votes-for", "0", "--votes-against", "9")

	r := e.run("", "decisions", "show", f.ProposedDecision.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "\nVotes: 0 for, 9 against\n") {
		t.Errorf("the decision reads:\n%s", r.stdout)
	}
	r = e.run("", "events", "meeting", "show", f.OnceEvent.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "; votes: 0 for, 9 against; recorded ") {
		t.Errorf("the meeting reads:\n%s", r.stdout)
	}
}
