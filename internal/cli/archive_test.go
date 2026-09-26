package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// everyWrite is one call per command that writes into a project, against the
// fixture of e. It first puts there what some of them need: a comment, a step
// and a card in the trash.
func everyWrite(e *testEnv) map[string][]string {
	e.t.Helper()
	f := e.fixture
	card, chat := f.CardInDoing.ID, f.Chat.ID
	message := f.Chat.Messages[0].ID

	expectCode(e.t, e.run("", "comments", "create", card, "A comment", "--json"), ExitOK)
	expectCode(e.t, e.run("", "steps", "create", card, "A step", "--json"), ExitOK)
	expectCode(e.t, e.run("", "cards", "delete", f.CardInTriage.ID, "--json"), ExitOK)
	comment := f.CardInDoing.Comments[len(f.CardInDoing.Comments)-1].ID

	return map[string][]string{
		"cards create":      {"cards", "create", "--board", f.Board.ID, "--title", "New"},
		"cards edit":        {"cards", "edit", card, "--title", "Renamed"},
		"cards move":        {"cards", "move", card, "--column", "not_now"},
		"cards done":        {"cards", "done", card},
		"cards archive":     {"cards", "archive", card},
		"cards unarchive":   {"cards", "unarchive", card},
		"cards delete":      {"cards", "delete", card},
		"cards restore":     {"cards", "restore", f.CardInTriage.ID},
		"cards assign":      {"cards", "assign", card, f.Anke.ID},
		"cards unassign":    {"cards", "unassign", card, f.Anke.ID},
		"cards subscribe":   {"cards", "subscribe", card, f.Anke.ID},
		"cards unsubscribe": {"cards", "unsubscribe", card, f.Anke.ID},
		"cards notes set":   {"cards", "notes", "set", card, "Notes", "--force"},
		"columns on-hold":   {"columns", "on-hold", f.Doing.ID},
		"steps create":      {"steps", "create", card, "Another step"},
		"steps complete":    {"steps", "complete", card, "A step"},
		"steps reopen":      {"steps", "reopen", card, "A step"},
		"steps delete":      {"steps", "delete", card, "A step"},
		"comments create":   {"comments", "create", card, "Another comment"},
		"comments edit":     {"comments", "edit", card, comment, "Other words"},
		"comments delete":   {"comments", "delete", card, comment},
		"comments react":    {"comments", "react", card, comment, "👍"},
		"comments unreact":  {"comments", "unreact", card, comment, "👍"},
		"messages create":   {"messages", "create", chat, "A message"},
		"messages edit":     {"messages", "edit", chat, message, "Another word"},
		"messages delete":   {"messages", "delete", chat, message},
		"messages react":    {"messages", "react", chat, message, "👍"},
		"messages unreact":  {"messages", "unreact", chat, message, "👍"},
	}
}

// notAWriteIntoAProject are the commands that read, that write nowhere on the
// host, or that write where no project is: the read mark of a chat is taken in
// an archived project as well, and a conversation hangs on none. An upload is
// tested by itself, because it needs a file.
var notAWriteIntoAProject = []string{
	"auth login", "auth logout", "auth status", "setup", "doctor", "me", "version", "commands",
	"skill show", "skill install", "skill status",
	"projects list", "boards list", "boards show", "boards archive", "people list", "trash list",
	"cards mine", "cards show", "cards notes show", "steps list", "chats list", "messages list",
	"chats mark-read", "conversations list", "conversations with", "conversations people",
	"uploads create", "uploads download", "import basecamp",
	"work start", "work stop", "work list", "heartbeat", "hooks install", "hooks status",
}

// A command that is new has to be put into one of the two lists, so that a way
// of writing cannot go untested.
func TestEveryCommandIsKnownToWriteIntoAProjectOrNot(t *testing.T) {
	e := newTestEnv(t)
	known := map[string]bool{}
	for name := range everyWrite(e) {
		known[name] = true
	}
	for _, name := range notAWriteIntoAProject {
		if known[name] {
			t.Errorf("%q is in both lists", name)
		}
		known[name] = true
	}

	root := newApp(t.Context(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, func(string) string { return "" }).newRootCmd()
	for _, name := range commandPaths(root) {
		if !known[name] {
			t.Errorf("%q is in neither list: does it write into a project?", name)
		}
		delete(known, name)
	}
	for name := range known {
		t.Errorf("%q is listed but no command", name)
	}
}

func TestEveryWriteIntoAnArchivedProjectSaysThatItIsArchived(t *testing.T) {
	for name := range everyWrite(newTestEnv(t)) {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			args := everyWrite(e)[name]
			writesBefore := len(e.server.WritingRequests())
			e.fixture.Project.Archived = true

			env, r := e.runJSON("", args...)
			expectCode(t, r, ExitValidation)
			if env.Code != CodeValidation || env.Meta["apiCode"] != "projectArchived" {
				t.Fatalf("code %q, apiCode %v", env.Code, env.Meta["apiCode"])
			}
			if env.Retryable {
				t.Fatal("retrying cannot help while the project is archived")
			}
			if !strings.Contains(env.Hint, "minka projects list --archived") || !strings.Contains(env.Hint, "still be read") {
				t.Fatalf("hint %q does not say what an archived project is", env.Hint)
			}
			if len(e.server.WritingRequests()) != writesBefore+1 {
				t.Fatal("the command did not try to write")
			}
		})
	}
}

func TestAnUploadIntoAnArchivedProjectSaysThatItIsArchived(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.Project.Archived = true
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("a few words"), 0o600); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "uploads", "create", file, "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "projectArchived" || !strings.Contains(env.Hint, "--archived") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestAnArchivedProjectCanStillBeRead(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.Project.Archived = true

	for _, args := range [][]string{
		{"boards", "list", e.fixture.Project.ID},
		{"boards", "show", e.fixture.Board.ID},
		{"cards", "show", e.fixture.CardInDoing.ID},
		{"steps", "list", e.fixture.CardInDoing.ID},
		{"people", "list", e.fixture.Project.ID},
		{"trash", "list", e.fixture.Project.ID},
		{"chats", "list", e.fixture.Project.ID},
		{"messages", "list", e.fixture.Chat.ID},
		{"chats", "mark-read", e.fixture.Chat.ID},
	} {
		if _, r := e.runJSON("", args...); r.code != ExitOK {
			t.Errorf("%v exits %d: %s", args, r.code, r.stdout)
		}
	}
}

func TestSomebodyOutsideAnArchivedProjectLearnsNothingAboutIt(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.Project.Archived = true
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken

	env, r := e.runJSON("", "comments", "create", e.fixture.CardInDoing.ID, "A comment")
	expectCode(t, r, ExitNotFound)
	if env.Meta["apiCode"] == "projectArchived" {
		t.Fatal("somebody outside the project is told that it is archived")
	}
	if !strings.Contains(env.Hint, "trash") {
		t.Fatalf("hint %q does not name the trash", env.Hint)
	}
}

func TestProjectsListKeepsTheArchivedOnesBehindAFlag(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.Project.Archived = true

	names := func(args ...string) (string, []string) {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitOK)
		var projects []struct{ Name, Status string }
		if err := json.Unmarshal(env.Data, &projects); err != nil {
			t.Fatal(err)
		}
		var list []string
		for _, p := range projects {
			list = append(list, p.Name+":"+p.Status)
		}
		return env.Summary, list
	}

	if _, inUse := names("projects", "list"); len(inUse) != 0 {
		t.Fatalf("projects in use: %v", inUse)
	}
	summary, archived := names("projects", "list", "--archived")
	if len(archived) != 1 || archived[0] != e.fixture.Project.Name+":archived" {
		t.Fatalf("archived projects: %v", archived)
	}
	if !strings.Contains(summary, "archived") {
		t.Fatalf("summary %q does not say which projects these are", summary)
	}
	if got := e.server.Requests()[len(e.server.Requests())-1].Query; got != "status=archived" {
		t.Fatalf("query %q", got)
	}
}

func TestCardsMineLeavesOutArchivedProjects(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "cards", "assign", e.fixture.CardInDoing.ID, "me", "--json"), ExitOK)
	before, _ := e.runJSON("", "cards", "mine")
	if !strings.Contains(string(before.Data), e.fixture.CardInDoing.ID) {
		t.Fatal("the card is not among one's own to begin with")
	}

	e.fixture.Project.Archived = true
	after, _ := e.runJSON("", "cards", "mine")
	if strings.Contains(string(after.Data), e.fixture.CardInDoing.ID) {
		t.Fatal("the card of an archived project is still among one's own")
	}
}

func TestAMessageSentBeforeTheArchiveIsRefusedLikeAnyOther(t *testing.T) {
	e := newTestEnv(t)
	id := "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"
	_, first := e.runJSON("", "messages", "create", e.fixture.Chat.ID, "A word", "--client-message-id", id)
	expectCode(t, first, ExitOK)
	e.fixture.Project.Archived = true

	env, again := e.runJSON("", "messages", "create", e.fixture.Chat.ID, "A word", "--client-message-id", id)
	expectCode(t, again, ExitValidation)
	if env.Meta["apiCode"] != "projectArchived" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	if env.Retryable {
		t.Fatal("a repeated send reads as retryable, although trying again cannot help")
	}
}

func TestAConversationIsWrittenToWhateverIsArchived(t *testing.T) {
	e := newTestEnv(t)
	for _, project := range e.server.Projects {
		project.Archived = true
	}
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("a few words"), 0o600); err != nil {
		t.Fatal(err)
	}

	conversation := e.fixture.Conversation.ID
	expectCode(t, e.run("", "messages", "create", conversation, "A word", "--json"), ExitOK)
	expectCode(t, e.run("", "uploads", "create", file, "--conversation", conversation, "--json"), ExitOK)
}

func TestProjectsInUseAreNeverPassedOffAsArchived(t *testing.T) {
	e := newTestEnv(t)
	e.server.IgnoresProjectStatus = true

	env, r := e.runJSON("", "projects", "list", "--archived")
	expectCode(t, r, ExitAPI)
	if strings.Contains(r.stdout, e.fixture.Project.Name) {
		t.Fatalf("a project in use is listed as archived: %s", r.stdout)
	}
	if env.Hint == "" {
		t.Fatal("no hint")
	}
}

// commandPaths names every command that runs something, below the groups that
// only hold others, the way it is called without "minka".
func commandPaths(root *cobra.Command) []string {
	var paths []string
	var walk func(parent *cobra.Command, prefix string)
	walk = func(parent *cobra.Command, prefix string) {
		for _, cmd := range parent.Commands() {
			if cmd.Hidden || cmd.Name() == "help" {
				continue
			}
			if cmd.HasSubCommands() {
				walk(cmd, prefix+cmd.Name()+" ")
				continue
			}
			paths = append(paths, prefix+cmd.Name())
		}
	}
	walk(root, "")
	return paths
}
