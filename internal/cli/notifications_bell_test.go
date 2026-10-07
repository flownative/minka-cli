package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// The id of a line a wait printed is what show and mark-read take: show hands
// the line over as Minka sent it and marks nothing read; mark-read does, and
// hands over the number on the bell after it.
func TestTheIDOfAWaitedLineIsShownAndMarkedRead(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	waited, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	id, _ := linesOf(t, waited)[0]["id"].(string)

	env, r := e.runJSON("", "notifications", "show", strings.ToUpper(id))
	expectCode(t, r, ExitOK)
	shown := decode(t, env.Data)
	if shown["id"] != id || shown["read"] != false || shown["title"] != "Moved to another column: "+e.fixture.CardInDoing.Title || shown["unreadCount"] != float64(1) {
		t.Fatalf("shown %v", shown)
	}
	if crumbFor(t, env.Breadcrumbs, "mark-read") != "minka notifications mark-read "+id {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}
	if line := e.server.Notifications[0]; line.Read {
		t.Fatal("showing a line marked it read")
	}

	env, r = e.runJSON("", "notifications", "mark-read", id)
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); len(data) != 1 || data["unreadCount"] != float64(0) || !strings.Contains(env.Summary, "0 unread notifications") {
		t.Fatalf("data %s, summary %q", env.Data, env.Summary)
	}
	if !e.server.Notifications[0].Read {
		t.Fatal("the line was not marked read")
	}
	if sent := e.server.Requests(); sent[len(sent)-1].Method != "POST" || sent[len(sent)-1].Path != "/api/v1/notifications/"+id+"/read" {
		t.Fatalf("sent %+v", sent[len(sent)-1])
	}

	env, r = e.runJSON("", "notifications", "count")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["unreadCount"] != float64(0) {
		t.Fatalf("count %s", env.Data)
	}
}

// The text view of a line says the push's sentence, the project and whether
// it is read.
func TestALineReadsAsItsPushSaid(t *testing.T) {
	e := newTestEnv(t)
	line := e.server.Notify(&fakeapi.Notification{
		PersonID: e.fixture.Bruno.ID, Occasion: "person.mentioned", CardID: e.fixture.CardInTriage.ID,
		ProjectID: e.fixture.Project.ID, Summary: "Anke mentioned you in “Write the CLI”",
	})

	r := e.run("", "notifications", "show", line.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"Anke mentioned you in “Write the CLI”\n", "Project: " + e.fixture.Project.Name + "\n", "Read:    no\n", "ID:      " + line.ID + "\n"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout %q lacks %q", r.stdout, want)
		}
	}
}

// A line of somebody else, and one about what the person may no longer see,
// exit 2 with a hint that names both; nothing is marked read.
func TestALineThatIsNotThereExits2(t *testing.T) {
	e := newTestEnv(t)
	agents := e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Agent.ID, Occasion: "comment.written"})
	for _, command := range []string{"show", "mark-read"} {
		env, r := e.runJSON("", "notifications", command, agents.ID)
		expectCode(t, r, ExitNotFound)
		if env.Hint != notificationNotFoundHint {
			t.Errorf("%s: hint %q", command, env.Hint)
		}
	}
	if agents.Read {
		t.Fatal("somebody else's line was marked read")
	}
}

// A line already read offers no mark-read, and a failure that is no 404 gets
// no hint about lines that are not there.
func TestALineReadOffersNoMarkReadAndAFailureNoNotFoundHint(t *testing.T) {
	e := newTestEnv(t)
	line := e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "comment.written", Read: true})

	env, r := e.runJSON("", "notifications", "show", line.ID)
	expectCode(t, r, ExitOK)
	for _, crumb := range env.Breadcrumbs {
		if crumb.Action == "mark-read" {
			t.Fatalf("a read line offers %+v", crumb)
		}
	}

	for _, command := range []string{"show", "mark-read"} {
		e.server.FailNextWith(502)
		env, r = e.runJSON("", "notifications", command, line.ID)
		expectCode(t, r, ExitAPI)
		if env.Hint == notificationNotFoundHint {
			t.Errorf("%s: a server error got the hint of a 404", command)
		}
	}
}

// The safeguard against a lost line: a page that comes back short because a
// row of it made no line ends the run, and its cursor stands behind that row,
// so the next wait goes on behind it. Every line is printed once, none twice,
// none lost.
func TestAPageCutShortByRowsWithoutALineLosesNothing(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)
	e.server.WhileWaiting = nil

	// The last row of the first page makes no line, so the page carries one
	// line less than it holds and its cursor stands behind its last line.
	var written []string
	var retired *fakeapi.Notification
	for i := range api.NotificationsPageMax + 30 {
		if i == api.NotificationsPageMax-1 {
			retired = e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "retired.kind"})
			continue
		}
		line := e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "comment.written", Summary: fmt.Sprintf("line %d", i)})
		written = append(written, line.ID)
	}

	var printed []string
	for run := range 2 {
		env, r := e.runWaitJSON(nil)
		expectCode(t, r, ExitOK)
		for _, line := range linesOf(t, env) {
			printed = append(printed, line["id"].(string))
		}
		if mark, _ := e.markOf(""); run == 0 && mark != fakeapi.CursorOf(retired) {
			t.Fatalf("after the short page the mark is %q, want behind the row without a line, %q", mark, fakeapi.CursorOf(retired))
		}
	}
	if got, want := strings.Join(printed, ","), strings.Join(written, ","); got != want {
		t.Fatalf("printed %d lines, want the %d written, each once and in order", len(printed), len(written))
	}
	if mark, _ := e.markOf(""); mark != fakeapi.CursorOf(e.server.Notifications[len(e.server.Notifications)-1]) {
		t.Fatalf("mark %q is not behind the last line", mark)
	}
}

// The count is the bell's, handed over as Minka sent it.
func TestTheCountIsTheNumberOnTheBell(t *testing.T) {
	e := newTestEnv(t)
	e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "comment.written"})
	e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "reaction.added"})

	env, r := e.runJSON("", "notifications", "count")
	expectCode(t, r, ExitOK)
	var count map[string]any
	if err := json.Unmarshal(env.Data, &count); err != nil || count["unreadCount"] != float64(2) || env.Summary != "2 unread notifications" {
		t.Fatalf("data %s, summary %q", env.Data, env.Summary)
	}
}
