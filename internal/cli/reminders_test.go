package cli

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

func TestAReminderOfACardNamedByItsShortIdentifierIsSetByItsID(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.CardInTriage.ShortID = "MNK-AG"

	env := runOK(t, e, "reminders", "set", "card", "mnk-ag", "--preset", "tomorrow")
	last := e.server.Requests()[len(e.server.Requests())-1]
	if last.Method != http.MethodPut || last.Path != "/api/v1/me/reminders/card/"+f.CardInTriage.ID || string(last.Body) != `{"preset":"tomorrow"}` {
		t.Fatalf("sent %s %s %s", last.Method, last.Path, last.Body)
	}
	reminder := decode(t, env.Data)
	if reminder["kind"] != "card" || reminder["id"] != f.CardInTriage.ID || reminder["isDue"] != false ||
		reminder["remindAt"] != "2026-09-14T07:00:00.000000Z" {
		t.Fatalf("reminder %v", reminder)
	}
	if len(env.Breadcrumbs) == 0 || env.Breadcrumbs[1].Cmd != "minka reminders delete card "+f.CardInTriage.ID+" --outcome done" {
		t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
	}

	text := e.run("", "reminders", "set", "card", f.CardInTriage.ID, "--preset", "tomorrow", "--styled")
	expectCode(t, text, ExitOK)
	if !strings.Contains(text.stdout, "Reminds you of "+f.CardInTriage.Title+" (card "+f.CardInTriage.ID+")") {
		t.Fatalf("text %q", text.stdout)
	}
}

// Minka compares the kind in its own spelling and answers any other with
// 404, as though the thing were not there; the CLI takes it in any case.
func TestTheKindOfAReminderGoesOutInMinkasSpelling(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	runOK(t, e, "reminders", "set", "TODOLIST", strings.ToUpper(f.TodoList.ID), "--at", "2026-09-20T10:00:00+02:00")
	last := e.server.Requests()[len(e.server.Requests())-1]
	if last.Path != "/api/v1/me/reminders/todoList/"+f.TodoList.ID {
		t.Fatalf("sent to %s", last.Path)
	}
	runOK(t, e, "reminders", "delete", "todolist", f.TodoList.ID)
	last = e.server.Requests()[len(e.server.Requests())-1]
	if last.Path != "/api/v1/me/reminders/todoList/"+f.TodoList.ID {
		t.Fatalf("sent to %s", last.Path)
	}
}

// A moment in RFC 3339 goes out as it was written, with its fraction and its
// offset.
func TestAMomentGoesOutAsItWasWritten(t *testing.T) {
	e := newTestEnv(t)
	comment := e.fixture.Post.Comments[0]

	env := runOK(t, e, "reminders", "set", "comment", comment.ID, "--at", "2026-09-20T10:00:00.5+02:00")
	if body := e.lastWriteBody()["remindAt"]; body != "2026-09-20T10:00:00.5+02:00" {
		t.Fatalf("sent %q", body)
	}
	if reminder := decode(t, env.Data); reminder["remindAt"] != "2026-09-20T08:00:00.500000Z" || reminder["kind"] != "comment" {
		t.Fatalf("reminder %v", reminder)
	}
}

// What cannot be a reminder is refused before anything is asked.
func TestAReminderThatCannotBeIsRefusedBeforeAnythingIsSent(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID
	for _, args := range [][]string{
		{"reminders", "set", "cards", card, "--preset", "tomorrow"},
		{"reminders", "set", "card", card},
		{"reminders", "set", "card", card, "--preset", "tomorrow", "--at", "2026-09-20T10:00:00Z"},
		{"reminders", "set", "card", card, "--preset", ""},
		{"reminders", "set", "post", "MNK-1A", "--preset", "tomorrow"},
		{"reminders", "set", "comment", "not-an-id", "--preset", "tomorrow"},
		{"reminders", "set", "card", card, "--at", "2026-09-20 10:00"},
		{"reminders", "set", "card", card, "--at", "2026-09-20T10:00:00+0200"},
		{"reminders", "set", "card", card, "--at", "2026-09-20T10:00:00z"},
		{"reminders", "set", "card", card, "--at", "2026-02-30T10:00:00Z"},
		{"reminders", "delete", "thing", card},
		{"reminders", "delete", "post", "not-an-id"},
	} {
		before := len(e.server.Requests())
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if env.Code != CodeUsage {
			t.Errorf("%v: code %q", args, env.Code)
		}
		if sent := e.server.Requests()[before:]; len(sent) != 0 {
			t.Errorf("%v: %d requests went out, the first to %s", args, len(sent), sent[0].Path)
		}
	}
}

func TestARefusedReminderSaysWhatToDo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	card := f.CardInTriage.ID
	for _, c := range []struct {
		args    []string
		exit    int
		apiCode any
		hint    string
	}{
		{[]string{"set", "card", card, "--preset", "someday"}, ExitValidation, apiCodeReminderPresetUnknown, "in30Minutes"},
		{[]string{"set", "card", card, "--at", "2026-09-20 10:00"}, ExitUsage, nil, "RFC 3339"},
		{[]string{"set", "card", card, "--at", "2026-09-13T15:00:00+02:00"}, ExitValidation, apiCodeReminderTimeGone, "ahead"},
		{[]string{"set", "post", card, "--preset", "tomorrow"}, ExitNotFound, "notFound", "kind"},
		{[]string{"set", "card", f.OtherCard.ID, "--preset", "tomorrow"}, ExitNotFound, "notFound", "member"},
		{[]string{"delete", "card", card, "--outcome", "later"}, ExitValidation, apiCodeReminderOutcomeUnknown, "done"},
		{[]string{"delete", "card", card}, ExitNotFound, "notFound", "minka reminders list"},
	} {
		env, r := e.runJSON("", append([]string{"reminders"}, c.args...)...)
		if r.code != c.exit || env.Meta["apiCode"] != c.apiCode || !strings.Contains(env.Hint, c.hint) {
			t.Errorf("%v: exit %d, apiCode %v, hint %q", c.args, r.code, env.Meta["apiCode"], env.Hint)
		}
		if env.Retryable {
			t.Errorf("%v: a refusal is retryable", c.args)
		}
	}
}

func TestTheRemindersAreListedTheDueOnesFirst(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	now := e.server.Now()
	e.server.Reminders = []*fakeapi.Reminder{
		{PersonID: f.Bruno.ID, SubjectID: f.Post.ID, RemindAt: now.Add(time.Hour)},
		{PersonID: f.Bruno.ID, SubjectID: f.CardInDoing.ID, RemindAt: now.Add(-time.Hour)},
		{PersonID: f.Bruno.ID, SubjectID: f.OtherCard.ID, RemindAt: now.Add(-time.Minute)},
		{PersonID: f.Bruno.ID, SubjectID: f.Document.ID, RemindAt: now.Add(2 * time.Hour)},
	}

	env := runOK(t, e, "reminders", "list")
	reminders := listOfObjects(t, env.Data)
	if len(reminders) != 3 || reminders[0]["id"] != f.CardInDoing.ID || reminders[0]["isDue"] != true || reminders[1]["id"] != f.Post.ID {
		t.Fatalf("reminders %v", reminders)
	}
	if env.Summary != "3 reminders, 1 due" || env.Notice != "" {
		t.Fatalf("summary %q, notice %q", env.Summary, env.Notice)
	}

	text := e.run("", "reminders", "list", "--styled")
	expectCode(t, text, ExitOK)
	lines := strings.Split(strings.TrimSpace(text.stdout), "\n")
	if len(lines) != 4 || !strings.Contains(lines[1], "due") || !strings.Contains(lines[1], f.CardInDoing.Title) || strings.Contains(lines[2], "due") {
		t.Fatalf("text %q", text.stdout)
	}
}

// Minka reads 500 on either side and no page beyond, so a full side may be
// cut; the CLI says so of that side and not of the other.
func TestAFullSideOfTheRemindersSaysThereMayBeMore(t *testing.T) {
	for _, due := range []bool{true, false} {
		e := newTestEnv(t)
		f := e.fixture
		now := e.server.Now()
		for i := range api.RemindersMaxOnASide {
			id := f.Post.ID
			if i > 0 {
				id = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
			}
			at := now.Add(time.Duration(i+1) * time.Minute)
			if due {
				at = now.Add(-time.Duration(i+1) * time.Minute)
			}
			e.server.Reminders = append(e.server.Reminders, &fakeapi.Reminder{PersonID: f.Bruno.ID, SubjectID: id, RemindAt: at})
		}
		// Only one of the 500 is in sight: the side is cut, and nothing in the
		// list can tell.
		env := runOK(t, e, "reminders", "list")
		if len(listOfObjects(t, env.Data)) != 1 || env.Notice != "" {
			t.Fatalf("due %v: notice %q", due, env.Notice)
		}

		// With every one in sight, the side is full.
		e.server.Reminders = e.server.Reminders[:0]
		for i := range api.RemindersMaxOnASide {
			at := now.Add(time.Duration(i+1) * time.Minute)
			if due {
				at = now.Add(-time.Duration(i+1) * time.Minute)
			}
			e.server.Reminders = append(e.server.Reminders, &fakeapi.Reminder{PersonID: f.Bruno.ID, SubjectID: f.Post.ID, RemindAt: at})
		}
		env = runOK(t, e, "reminders", "list")
		named, other := "500 due reminders", "500 reminders ahead"
		if !due {
			named, other = other, named
		}
		if !strings.Contains(env.Notice, named) || strings.Contains(env.Notice, other) {
			t.Fatalf("due %v: notice %q", due, env.Notice)
		}
	}
}

// Without --outcome the CLI sends none and Minka takes dismissed; with one it
// is sent and recorded.
func TestADeletedReminderIsGoneWithItsOutcome(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	e.server.Reminders = []*fakeapi.Reminder{
		{PersonID: f.Bruno.ID, SubjectID: f.Post.ID},
		{PersonID: f.Bruno.ID, SubjectID: f.OtherCard.ID},
	}

	env := runOK(t, e, "reminders", "delete", "post", f.Post.ID)
	last := e.server.Requests()[len(e.server.Requests())-1]
	if last.Method != http.MethodDelete || last.Query != "" {
		t.Fatalf("sent %s with the query %q", last.Method, last.Query)
	}
	if data := decode(t, env.Data); data["outcome"] != "dismissed" || data["deleted"] != true || data["id"] != f.Post.ID {
		t.Fatalf("data %v", data)
	}

	// Out of sight, a reminder is still taken away by its kind and ID.
	runOK(t, e, "reminders", "delete", "card", f.OtherCard.ID, "--outcome", "done")
	if last := e.server.Requests()[len(e.server.Requests())-1]; last.Query != "outcome=done" {
		t.Fatalf("sent the query %q", last.Query)
	}
	removed := e.server.RemovedReminders
	if len(e.server.Reminders) != 0 || len(removed) != 2 || removed[0].Outcome != "dismissed" || removed[1].Outcome != "done" {
		t.Fatalf("left %v, removed %v", e.server.Reminders, removed)
	}
}

// The local check of --at is Minka's own: every moment of Minka's test of its
// reader is taken or refused here as there, so nothing Minka takes is
// refused before it is sent.
func TestAMomentIsCheckedAsMinkaReadsIt(t *testing.T) {
	for _, taken := range []string{
		"2099-03-01T09:30:00Z", "2099-03-01T09:30:00+02:00", "2099-03-01T09:30:00-05:30",
		"2099-03-01T09:30:00+00:00", "2099-03-01T09:30:00+14:00", "2099-03-01T09:30:00-14:00",
		"2099-03-01T09:30:00.123Z", "2099-03-01T09:30:00.123456+01:00", "2099-03-01T09:30:00.123456789Z",
		"2096-02-29T00:00:00Z",
	} {
		if !isRFC3339Moment(taken) {
			t.Errorf("%q is refused, and Minka takes it", taken)
		}
	}
	for _, refused := range []string{
		"", "2099-03-01T09:30:00", "2099-03-01T09:30:00 Europe/Berlin", "2099-03-01T09:30:00Europe/Berlin",
		"2099-03-01T09:30:00CEST", "2099-03-01T09:30:00UTC", "2099-03-01T09:30:00z", "2099-03-01t09:30:00Z",
		"2099-03-01 09:30:00Z", "2099-03-01T09:30:00+0200", "2099-03-01T09:30:00+02", "2099-03-01T09:30:00+25:00",
		"2099-03-01T09:30:00+14:01", "2099-03-01T09:30:00 +02:00", "2099-03-01T9:30:00Z", "2099-03-01T09:30Z",
		"99-03-01T09:30:00Z", "2099-03-01T09:30:00.Z", "2099-02-30T09:30:00Z", "2099-03-01T24:00:00Z",
		"2099-03-01T23:59:60Z", "2099-03-01T09:30:00Z\n", "2099-03-01", "tomorrow",
	} {
		if isRFC3339Moment(refused) {
			t.Errorf("%q is taken, and Minka refuses it", refused)
		}
	}
}

// A card named by something that is neither an ID nor a short identifier is
// refused under the name of the position it was given in.
func TestAnItemThatIsNoIDIsNamedAsTheItem(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "reminders", "set", "card", "foo", "--preset", "tomorrow")
	expectCode(t, r, ExitUsage)
	if !strings.HasPrefix(env.Error, "<item> ") || !strings.Contains(env.Error, "short identifier") {
		t.Fatalf("error %q", env.Error)
	}
}
