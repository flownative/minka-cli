package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// commentedThings are the to-do, the list and an event of the fixture, each
// with the command its comments stand under and the key that names it in an
// answer.
func commentedThings(f *fakeapi.Fixture) map[string]struct {
	command, arg, id string
	comments         *[]*fakeapi.Comment
} {
	return map[string]struct {
		command, arg, id string
		comments         *[]*fakeapi.Comment
	}{
		"a to-do":  {"todos", "todoId", f.LooseTodo.ID, &f.LooseTodo.Comments},
		"a list":   {"todolists", "listId", f.TodoList.ID, &f.TodoList.Comments},
		"an event": {"events", "eventId", f.OnceEvent.ID, &f.OnceEvent.Comments},
	}
}

func TestACommentOnAToDoAListOrAnEventIsReadWrittenChangedReactedToAndTakenBack(t *testing.T) {
	for name := range commentedThings(newTestEnv(t).fixture) {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			thing := commentedThings(e.fixture)[name]

			env := runOK(t, e, thing.command, "comments", "create", thing.id, "Who **helps**?")
			if e.lastWriteBody()["body"] != "<p>Who <strong>helps</strong>?</p>" {
				t.Fatalf("sent %v", e.lastWriteBody())
			}
			if requests := e.server.Requests(); !strings.HasSuffix(requests[len(requests)-1].Path, "/"+thing.id+"/comments") {
				t.Fatalf("written to %s", requests[len(requests)-1].Path)
			}
			id := decode(t, env.Data)["id"].(string)

			comments := listOfObjects(t, runOK(t, e, thing.command, "comments", "list", thing.id).Data)
			if len(comments) == 0 || comments[len(comments)-1]["id"] != id {
				t.Fatalf("comments %v, want the new one last", comments)
			}
			edited := runOK(t, e, thing.command, "comments", "edit", thing.id, id, "Who helps on Saturday?")
			if decode(t, edited.Data)["editedAt"] == nil {
				t.Fatal("the edit left no moment")
			}

			react := runOK(t, e, thing.command, "comments", "react", thing.id, id, "👍")
			again := runOK(t, e, thing.command, "comments", "react", thing.id, id, "👍")
			if react.Summary == again.Summary || !strings.Contains(again.Summary, "already") {
				t.Fatalf("summaries %q and %q", react.Summary, again.Summary)
			}
			unreacted := runOK(t, e, thing.command, "comments", "unreact", thing.id, id, "👍")
			if data := decode(t, unreacted.Data); data[thing.arg] != thing.id || data["commentId"] != id || data["content"] != "👍" {
				t.Fatalf("data %v", data)
			}
			removed := runOK(t, e, thing.command, "comments", "delete", thing.id, id)
			if data := decode(t, removed.Data); data[thing.arg] != thing.id || data["commentId"] != id || data["deleted"] != true {
				t.Fatalf("data %v", data)
			}
			env, r := e.runJSON("", thing.command, "comments", "delete", thing.id, id)
			expectCode(t, r, ExitNotFound)
			if !strings.Contains(env.Hint, "only the author") {
				t.Fatalf("hint %q", env.Hint)
			}
		})
	}
}

// Somebody else's comment is not there to change, and a comment of another
// item is not under this one: Minka answers both as one that never was.
func TestACommentOfSomebodyElseOrOfAnotherItemIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	ofAnke := f.TodoList.Comments[0]

	env, r := e.runJSON("", "todolists", "comments", "edit", f.TodoList.ID, ofAnke.ID, "Mine now")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "only the author") || !strings.Contains(env.Hint, "to-do list") {
		t.Fatalf("hint %q", env.Hint)
	}

	ankesOnTheTodo := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e603", AuthorID: f.Anke.ID, Body: "<p>As it was</p>", CreatedAt: time.Now()}
	f.LooseTodo.Comments = append(f.LooseTodo.Comments, ankesOnTheTodo)
	env, r = e.runJSON("", "todos", "comments", "edit", f.LooseTodo.ID, ankesOnTheTodo.ID, "Mine now")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "only the author") {
		t.Fatalf("hint %q", env.Hint)
	}

	env, r = e.runJSON("", "todos", "comments", "react", f.LooseTodo.ID, ofAnke.ID, "👍")
	expectCode(t, r, ExitNotFound)
	if strings.Contains(env.Hint, "somebody else") || !strings.Contains(env.Hint, "minka todos comments list <todoId>") {
		t.Fatalf("hint %q", env.Hint)
	}

	env, r = e.runJSON("", "todolists", "comments", "react", f.TodoList.ID, ofAnke.ID, strings.Repeat("x", 21))
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "reactionTooLong" || !strings.Contains(env.Hint, "at most 20 characters") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	env, r = e.runJSON("", "todos", "comments", "list", f.OtherTodo.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
	env, r = e.runJSON("", "todolists", "comments", "create", f.OtherTodoList.ID, "Mine")
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoListNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

// An archived list takes no comment, neither under itself nor under its
// to-dos, and the hint names the way back; reading stays open. Somebody
// else's comment there is answered as the archive as well.
func TestUnderAnArchivedListEveryCommentWriteIsRefusedAsTheArchive(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	list, todo := f.ArchivedList, f.ArchivedListTodo
	list.Comments = []*fakeapi.Comment{{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e601", AuthorID: f.Anke.ID, Body: "<p>As it was</p>", CreatedAt: time.Now()}}
	todo.Comments = []*fakeapi.Comment{{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e602", AuthorID: f.Bruno.ID, Body: "<p>As it was</p>", CreatedAt: time.Now()}}

	for command, under := range map[string]struct {
		id, comment, hint string
	}{
		"todos":     {todo.ID, todo.Comments[0].ID, "minka todolists unarchive "},
		"todolists": {list.ID, list.Comments[0].ID, "minka todolists unarchive " + list.ID},
	} {
		for _, args := range [][]string{
			{"create", under.id, "More"},
			{"edit", under.id, under.comment, "Not after all"},
			{"delete", under.id, under.comment},
			{"react", under.id, under.comment, "👍"},
			{"unreact", under.id, under.comment, "👍"},
		} {
			env, r := e.runJSON("", append([]string{command, "comments"}, args...)...)
			expectCode(t, r, ExitValidation)
			hint := under.hint
			if command == "todos" && args[0] == "edit" {
				// The edit has read the to-do, and so knows which list to name.
				hint += list.ID
			}
			if env.Meta["apiCode"] != apiCodeTodoListArchived || !strings.Contains(env.Hint, hint) {
				t.Errorf("%s comments %s: apiCode %v, hint %q", command, args[0], env.Meta["apiCode"], env.Hint)
			}
		}
		if comments := listOfObjects(t, runOK(t, e, command, "comments", "list", under.id).Data); len(comments) != 1 {
			t.Errorf("%s: comments %v", command, comments)
		}
	}
}

// The guard over files keeps a comment of one's own from losing them. Under
// an archived list Minka refuses any text, so its answer comes and not the
// guard.
func TestACommentOfAToDoWithFilesIsGuardedOnlyWhereItCouldBeChanged(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	withAMention := func(id string) []*fakeapi.Comment {
		return []*fakeapi.Comment{{ID: id, AuthorID: f.Bruno.ID, Body: "<div>" + aMention + "</div>", CreatedAt: time.Now()}}
	}
	f.LooseTodo.Comments = withAMention("0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e611")
	f.TodoList.Comments = withAMention("0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e612")
	f.ArchivedListTodo.Comments = withAMention("0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e613")
	f.ArchivedList.Comments = withAMention("0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e614")

	for _, in := range []struct {
		command, id, comment string
	}{
		{"todos", f.LooseTodo.ID, f.LooseTodo.Comments[0].ID},
		{"todolists", f.TodoList.ID, f.TodoList.Comments[0].ID},
	} {
		env, r := e.runJSON("", in.command, "comments", "edit", in.id, in.comment, "Plain")
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Hint, "--drop-attachments") || !strings.Contains(env.Hint, in.command+" show --json") {
			t.Fatalf("%s: hint %q", in.command, env.Hint)
		}
		runOK(t, e, in.command, "comments", "edit", in.id, in.comment, "Plain", "--drop-attachments")
	}

	for _, in := range []struct {
		command, id, comment string
	}{
		{"todos", f.ArchivedListTodo.ID, f.ArchivedListTodo.Comments[0].ID},
		{"todolists", f.ArchivedList.ID, f.ArchivedList.Comments[0].ID},
	} {
		env, r := e.runJSON("", in.command, "comments", "edit", in.id, in.comment, "Plain")
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeTodoListArchived {
			t.Fatalf("%s: apiCode %v", in.command, env.Meta["apiCode"])
		}
	}
}

// A done to-do takes comments as an open one does.
func TestADoneToDoTakesComments(t *testing.T) {
	e := newTestEnv(t)
	runOK(t, e, "todos", "comments", "create", e.fixture.DoneTodo.ID, "Well done")
	if len(e.fixture.DoneTodo.Comments) != 1 {
		t.Fatalf("comments %v", e.fixture.DoneTodo.Comments)
	}
}

// todos show and todolists show carry the comments and point to writing one.
func TestAToDoAndAListAreShownWithTheirCommentsAndTheWayToComment(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.LooseTodo.Comments = []*fakeapi.Comment{{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e621", AuthorID: f.Anke.ID, Body: "<p>On <strong>Friday</strong></p>", CreatedAt: time.Now()}}

	for command, id := range map[string]string{"todos": f.LooseTodo.ID, "todolists": f.TodoList.ID} {
		r := e.run("", command, "show", id, "--styled")
		expectCode(t, r, ExitOK)
		if !strings.Contains(r.stdout, "## Comments (1)") {
			t.Errorf("%s show: no comments in\n%s", command, r.stdout)
		}
		env := runOK(t, e, command, "show", id)
		found := false
		for _, crumb := range env.Breadcrumbs {
			found = found || crumb.Cmd == "minka "+command+" comments create "+id+" <text|->"
		}
		if !found {
			t.Errorf("%s show: breadcrumbs %v", command, env.Breadcrumbs)
		}
	}
	if r := e.run("", "todos", "show", f.LooseTodo.ID, "--styled"); !strings.Contains(r.stdout, "On **Friday**") {
		t.Errorf("the comment is not shown as Markdown:\n%s", r.stdout)
	}
}

// The help of a reaction says where reactions go and where they do not, which
// is the help somebody needs before reacting to a post itself.
func TestTheReactHelpOfPostsSaysThatAPostTakesNone(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "posts", "comments", "react", "--help")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "A post itself does not") {
		t.Fatalf("help:\n%s", r.stdout)
	}
	if r := e.run("", "todos", "comments", "react", "--help"); strings.Contains(r.stdout, "A post itself") {
		t.Fatalf("the help of to-dos speaks of posts:\n%s", r.stdout)
	}
}
