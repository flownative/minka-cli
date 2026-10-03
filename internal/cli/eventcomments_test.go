package cli

import (
	"strings"
	"testing"
)

// Minka answers an event it does not show, somebody else's comment and a
// comment under another event all as 404; the hint names what can be done
// about each.
func TestTheRefusalsOfAnEventCommentSayWhereToLook(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	ofAnke := f.OnceEvent.Comments[0]

	env, r := e.runJSON("", "events", "comments", "list", f.OtherEvent.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != eventNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}

	env, r = e.runJSON("", "events", "comments", "edit", f.OnceEvent.ID, ofAnke.ID, "Mine now")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "only the author") || !strings.Contains(env.Hint, "event") {
		t.Fatalf("hint %q", env.Hint)
	}

	env, r = e.runJSON("", "events", "comments", "react", f.WeeklyEvent.ID, ofAnke.ID, "👍")
	expectCode(t, r, ExitNotFound)
	if strings.Contains(env.Hint, "somebody else") || !strings.Contains(env.Hint, "minka events comments list <eventId>") {
		t.Fatalf("hint %q", env.Hint)
	}
}

// events show carries no comments, so the guard over files reads them from
// the list, and its hint sends there for the bodySource.
func TestAnEventCommentWithFilesIsGuardedFromTheListOfComments(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	own := f.OnceEvent.Comments[1]
	own.Body = "<div>" + aMention + "</div>"

	env, r := e.runJSON("", "events", "comments", "edit", f.OnceEvent.ID, own.ID, "Plain")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") || !strings.Contains(env.Hint, "events comments list <eventId> --json") {
		t.Fatalf("hint %q", env.Hint)
	}
	runOK(t, e, "events", "comments", "edit", f.OnceEvent.ID, own.ID, "Plain", "--drop-attachments")
	if own.Body != "<p>Plain</p>" {
		t.Fatalf("body %q", own.Body)
	}
}

// The help names the event with its own article: the summaries the group
// lists, and the help of each command.
func TestTheHelpOfEventCommentsSaysAnEvent(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "events", "comments", "--help")
	expectCode(t, r, ExitOK)
	if strings.Count(r.stdout, "on an event") != 7 {
		t.Errorf("events comments --help:\n%s", r.stdout)
	}
	for _, sub := range []string{"list", "create", "edit", "delete", "react", "unreact"} {
		r := e.run("", "events", "comments", sub, "--help")
		expectCode(t, r, ExitOK)
		if strings.Contains(r.stdout, " a event") {
			t.Errorf("events comments %s --help:\n%s", sub, r.stdout)
		}
	}
	env, r := e.runJSON("", "events", "comments", "react", e.fixture.WeeklyEvent.ID, e.fixture.OnceEvent.Comments[0].ID, "👍")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "The comments of an event:") {
		t.Errorf("hint %q", env.Hint)
	}
}

// What events show says of comments leads to where they are read.
func TestAnEventPointsToItsComments(t *testing.T) {
	e := newTestEnv(t)
	id := e.fixture.OnceEvent.ID

	r := e.run("", "events", "show", id, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Comments: 2, read with minka events comments list "+id) {
		t.Errorf("events show:\n%s", r.stdout)
	}
	found := false
	for _, crumb := range runOK(t, e, "events", "show", id).Breadcrumbs {
		found = found || crumb.Cmd == "minka events comments list "+id
	}
	if !found {
		t.Error("events show has no breadcrumb to the comments")
	}
	if r := e.run("", "events", "comments", "list", "--help"); !strings.Contains(r.stdout, "events show carries only their number") {
		t.Errorf("help:\n%s", r.stdout)
	}
}
