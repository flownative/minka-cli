package cli

import (
	"strings"
	"testing"
)

func TestAReplyNamesTheMessageItAnswers(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	question := chat.Messages[1]

	env, r := e.runJSON("", "messages", "create", chat.ID, "Me!", "--reply-to", strings.ToUpper(question.ID))
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["replyTo"] != question.ID {
		t.Errorf("body sent %v, want replyTo %s", body, question.ID)
	}
	if decode(t, env.Data)["replyTo"] != question.ID || !strings.Contains(env.Summary, question.ID) {
		t.Errorf("summary %q, data %s", env.Summary, env.Data)
	}

	_, r = e.runJSON("", "messages", "create", chat.ID, "No reply")
	expectCode(t, r, ExitOK)
	if _, sent := e.lastWriteBody()["replyTo"]; sent {
		t.Error("replyTo sent without --reply-to")
	}
}

// What is no ID is refused here; a message of another chat is Minka's to
// refuse, and the hint says where an ID comes from.
func TestAReplyToWhatIsNoMessageOfTheChatIsRefused(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat

	_, r := e.runJSON("", "messages", "create", chat.ID, "Me!", "--reply-to", "yesterday")
	expectCode(t, r, ExitUsage)
	if writes := e.server.WritingRequests(); len(writes) != 0 {
		t.Fatalf("%d requests went out", len(writes))
	}

	env, r := e.runJSON("", "messages", "create", chat.ID, "Me!", "--reply-to", e.fixture.OtherChat.Messages[0].ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeReplyToUnknown || !strings.Contains(env.Hint, "minka messages list "+chat.ID+" --json") {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// A reply says which message it answers: by number and author where that one
// stands on the page, by its ID where it does not.
func TestAReplyIsShownWithWhatItAnswers(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	answered := chat.Messages[1]
	chat.Messages[2].ReplyTo = answered.ID
	chat.Messages[0].ReplyTo = "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"

	r := e.run("", "messages", "list", chat.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"#3 Bruno, ",
		"In reply to #2, Anke\nDanke",
		"In reply to message 0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6, not on this page\nHallo",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
	if strings.Count(r.stdout, "In reply to") != 2 {
		t.Errorf("a message that answers nothing is shown as a reply:\n%s", r.stdout)
	}
}

func TestAChatOfAProjectIsShownByItsID(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat

	env, r := e.runJSON("", "chats", "show", chat.ID)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	project, _ := data["project"].(map[string]any)
	if data["id"] != chat.ID || data["name"] != "Planung" || data["projectId"] != e.fixture.Project.ID || project["name"] != e.fixture.Project.Name {
		t.Errorf("data %s", env.Data)
	}
	if len(env.Breadcrumbs) != 2 || env.Breadcrumbs[0].Cmd != "minka messages list "+chat.ID || env.Breadcrumbs[1].Cmd != "minka chats list "+e.fixture.Project.ID {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}

	// A private conversation answers at the addresses of its messages, not here.
	env, r = e.runJSON("", "chats", "show", e.fixture.Conversation.ID)
	expectCode(t, r, ExitNotFound)
	for _, cause := range []string{"minka conversations list", "in the trash", "client of the project"} {
		if !strings.Contains(env.Hint, cause) {
			t.Errorf("hint %q does not name %q", env.Hint, cause)
		}
	}
	_, r = e.runJSON("", "chats", "show", e.fixture.OtherChat.ID)
	expectCode(t, r, ExitNotFound)
}

func TestAToDosToolIsShownByItsID(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todolists", "tool", f.Todos.ID)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["id"] != f.Todos.ID || data["title"] != f.Todos.Title || data["projectId"] != f.Project.ID {
		t.Errorf("data %s", env.Data)
	}
	if len(env.Breadcrumbs) == 0 || env.Breadcrumbs[0].Cmd != "minka todolists list "+f.Todos.ID {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}

	// A tool the project no longer names is still read at its own address.
	_, r = e.runJSON("", "todolists", "tool", f.TodosOutOfUse.ID)
	expectCode(t, r, ExitOK)

	env, r = e.runJSON("", "todolists", "tool", f.OtherTodos.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "minka todolists tools <projectId>") || !strings.Contains(env.Hint, "in the trash") {
		t.Errorf("hint %q does not say where the tools are listed, or that one may be in the trash", env.Hint)
	}
}

// Every kind of tool is read by its ID alone in the group of what it holds,
// with the project it stands in; one of a project the person is no member of
// is not there, and the hint says where the tools of a project are listed.
func TestEveryKindOfToolIsShownByItsID(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	for _, c := range []struct {
		group, id, title, other, holds string
	}{
		{"posts", f.Posts.ID, f.Posts.Title, f.OtherPosts.ID, "minka posts list " + f.Posts.ID},
		{"docs", f.Files.ID, f.Files.Title, f.OtherFiles.ID, "minka docs list " + f.Files.ID},
		{"checkins", f.CheckinsTool.ID, f.CheckinsTool.Title, f.OtherCheckinsTool.ID, "minka checkins list " + f.CheckinsTool.ID},
		{"events", f.Calendar.ID, f.Calendar.Title, f.OtherCalendar.ID, "minka events list " + f.Calendar.ID + " --from YYYY-MM-DD --to YYYY-MM-DD"},
	} {
		t.Run(c.group, func(t *testing.T) {
			env, r := e.runJSON("", c.group, "tool", c.id)
			expectCode(t, r, ExitOK)
			data := decode(t, env.Data)
			if data["id"] != c.id || data["title"] != c.title || data["projectId"] != f.Project.ID {
				t.Errorf("data %s", env.Data)
			}
			if len(env.Breadcrumbs) != 2 || env.Breadcrumbs[0].Cmd != c.holds || env.Breadcrumbs[1].Cmd != "minka "+c.group+" tools "+f.Project.ID {
				t.Errorf("breadcrumbs %v", env.Breadcrumbs)
			}

			env, r = e.runJSON("", c.group, "tool", c.other)
			expectCode(t, r, ExitNotFound)
			if !strings.Contains(env.Hint, "minka "+c.group+" tools <projectId>") || !strings.Contains(env.Hint, "client of the project") || !strings.Contains(env.Hint, "in the trash") {
				t.Errorf("hint %q", env.Hint)
			}
		})
	}

	// A calendar says what it shows beside its events, in the text view too.
	r := e.run("", "events", "tool", f.Calendar.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Shows:   eventsAndDatedItems") {
		t.Errorf("the text view does not say what the calendar shows:\n%s", r.stdout)
	}

	// Out of use, a tool is still read at its own ID.
	_, r = e.runJSON("", "posts", "tool", f.PostsOutOfUse.ID)
	expectCode(t, r, ExitOK)
	_, r = e.runJSON("", "events", "tool", f.CalendarOutOfUse.ID)
	expectCode(t, r, ExitOK)
}
