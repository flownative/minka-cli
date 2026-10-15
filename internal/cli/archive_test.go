package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
	"github.com/spf13/cobra"
)

// everyWrite is one call per command that writes into a project, against the
// fixture of e. It first puts there what some of them need: a comment, a step,
// a card in the trash and entries of Docs & Files in the trash and the archive.
func everyWrite(e *testEnv) map[string][]string {
	e.t.Helper()
	f := e.fixture
	card, chat := f.CardInDoing.ID, f.Chat.ID
	message := f.Chat.Messages[0].ID

	document, upload := f.Document.ID, f.FilesUpload.ID
	trashedDocument, trashedUpload, trashedFolder, archivedUpload := putDocsAndFilesAway(e)
	dueOn := "2026-09-20"
	f.LooseTodo.DueOn = &dueOn

	expectCode(e.t, e.run("", "comments", "create", card, "A comment", "--json"), ExitOK)
	expectCode(e.t, e.run("", "cards", "steps", "create", card, "A step", "--json"), ExitOK)
	expectCode(e.t, e.run("", "cards", "delete", f.CardInTriage.ID, "--json"), ExitOK)
	expectCode(e.t, e.run("", "docs", "comments", "create", "--document", document, "A comment", "--json"), ExitOK)
	expectCode(e.t, e.run("", "posts", "comments", "create", f.Post.ID, "A comment", "--json"), ExitOK)
	expectCode(e.t, e.run("", "todos", "comments", "create", f.LooseTodo.ID, "A comment", "--json"), ExitOK)
	expectCode(e.t, e.run("", "todolists", "comments", "create", f.TodoList.ID, "A comment", "--json"), ExitOK)
	expectCode(e.t, e.run("", "columns", "create", "--board", f.Board.ID, "--title", "Review", "--json"), ExitOK)
	expectCode(e.t, e.run("", "cards", "notes", "attachments", "create", card, aFileToUpload(e), "--json"), ExitOK)
	comment := f.CardInDoing.Comments[len(f.CardInDoing.Comments)-1].ID
	postComment := f.Post.Comments[len(f.Post.Comments)-1].ID
	entryComment := f.Document.Comments[len(f.Document.Comments)-1].ID
	todoComment := f.LooseTodo.Comments[len(f.LooseTodo.Comments)-1].ID
	listComment := f.TodoList.Comments[len(f.TodoList.Comments)-1].ID
	eventComment := f.OnceEvent.Comments[len(f.OnceEvent.Comments)-1].ID
	version := listOfObjects(e.t, runOK(e.t, e, "docs", "versions", "list", document).Data)[0]["id"].(string)

	return map[string][]string{
		"projects edit":     {"projects", "edit", f.Project.ID, "--name", "Renamed"},
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

		"cards notes attachments create": {"cards", "notes", "attachments", "create", card, aFileToUpload(e)},
		"cards notes attachments delete": {"cards", "notes", "attachments", "delete", card, "notes.txt"},

		"columns on-hold":      {"columns", "on-hold", f.Doing.ID},
		"columns create":       {"columns", "create", "--board", f.Board.ID, "--title", "Shipped"},
		"columns edit":         {"columns", "edit", f.Doing.ID, "--title", "Renamed"},
		"columns move":         {"columns", "move", "Doing", "--board", f.Board.ID, "--after", "Review"},
		"columns delete":       {"columns", "delete", "Review", "--board", f.Board.ID},
		"cards steps create":   {"cards", "steps", "create", card, "Another step"},
		"cards steps edit":     {"cards", "steps", "edit", card, "A step", "--due", "2026-11-02"},
		"cards steps move":     {"cards", "steps", "move", card, "A step", "--top"},
		"cards steps complete": {"cards", "steps", "complete", card, "A step"},
		"cards steps reopen":   {"cards", "steps", "reopen", card, "A step"},
		"cards steps delete":   {"cards", "steps", "delete", card, "A step"},
		"comments create":      {"comments", "create", card, "Another comment"},
		"comments edit":        {"comments", "edit", card, comment, "Other words"},
		"comments delete":      {"comments", "delete", card, comment},
		"comments react":       {"comments", "react", card, comment, "👍"},
		"comments unreact":     {"comments", "unreact", card, comment, "👍"},
		"messages create":      {"messages", "create", chat, "A message"},
		"messages edit":        {"messages", "edit", chat, message, "Another word"},
		"messages delete":      {"messages", "delete", chat, message},
		"messages react":       {"messages", "react", chat, message, "👍"},
		"messages unreact":     {"messages", "unreact", chat, message, "👍"},

		"docs create":                  {"docs", "create", "--tool", f.Files.ID, "--title", "A new page", "--audience", "nobody"},
		"docs edit":                    {"docs", "edit", document, "--title", "Renamed", "--force"},
		"docs uploads create":          {"docs", "uploads", "create", aFileToUpload(e), "--tool", f.Files.ID, "--project", f.Project.ID},
		"docs uploads edit":            {"docs", "uploads", "edit", upload, "--title", "Renamed", "--force"},
		"docs uploads versions create": {"docs", "uploads", "versions", "create", upload, aFileToUpload(e)},
		"docs delete":                  {"docs", "delete", f.DocumentInFolder.ID},
		"docs restore":                 {"docs", "restore", trashedDocument},
		"docs archive":                 {"docs", "archive", document},
		"docs unarchive":               {"docs", "unarchive", f.ArchivedDocument.ID},
		"docs move":                    {"docs", "move", document, "--folder", f.Folder.ID},
		"docs versions restore":        {"docs", "versions", "restore", document, version},
		"docs uploads delete":          {"docs", "uploads", "delete", upload},
		"docs uploads restore":         {"docs", "uploads", "restore", trashedUpload},
		"docs uploads archive":         {"docs", "uploads", "archive", upload},
		"docs uploads unarchive":       {"docs", "uploads", "unarchive", archivedUpload},
		"docs uploads move":            {"docs", "uploads", "move", upload, "--top-level"},
		"docs folders create":          {"docs", "folders", "create", "--tool", f.Files.ID, "--title", "Offers"},
		"docs folders edit":            {"docs", "folders", "edit", f.Folder.ID, "--title", "Renamed"},
		"docs folders delete":          {"docs", "folders", "delete", f.FolderShownToClients.ID},
		"docs folders restore":         {"docs", "folders", "restore", trashedFolder},
		"docs folders archive":         {"docs", "folders", "archive", f.Folder.ID},
		"docs folders unarchive":       {"docs", "folders", "unarchive", f.ArchivedFolder.ID},
		"docs folders move":            {"docs", "folders", "move", f.FolderShownToClients.ID, "--folder", f.Folder.ID},
		"docs comments create":         {"docs", "comments", "create", "--document", document, "Another comment"},
		"docs comments edit":           {"docs", "comments", "edit", "--document", document, entryComment, "Other words"},
		"docs comments delete":         {"docs", "comments", "delete", "--document", document, entryComment},
		"docs comments react":          {"docs", "comments", "react", "--upload", upload, f.FilesUpload.Comments[0].ID, "👍"},
		"docs comments unreact":        {"docs", "comments", "unreact", "--upload", upload, f.FilesUpload.Comments[0].ID, "👍"},

		"todolists create":    {"todolists", "create", "--tool", f.Todos.ID, "--title", "A new list"},
		"todolists start":     {"todolists", "start", "--tool", f.Todos.ID, "--template", f.Template.ID, "--title", "A list from a template"},
		"todolists edit":      {"todolists", "edit", f.TodoList.ID, "--title", "Renamed"},
		"todolists archive":   {"todolists", "archive", f.TodoList.ID},
		"todolists unarchive": {"todolists", "unarchive", f.ArchivedList.ID},
		"todolists delete":    {"todolists", "delete", f.TodoList.ID},

		"todolists comments create":  {"todolists", "comments", "create", f.TodoList.ID, "Another comment"},
		"todolists comments edit":    {"todolists", "comments", "edit", f.TodoList.ID, listComment, "Other words"},
		"todolists comments delete":  {"todolists", "comments", "delete", f.TodoList.ID, listComment},
		"todolists comments react":   {"todolists", "comments", "react", f.TodoList.ID, listComment, "👍"},
		"todolists comments unreact": {"todolists", "comments", "unreact", f.TodoList.ID, listComment, "👍"},

		"todogroups create": {"todogroups", "create", "--list", f.TodoList.ID, "--title", "A new group"},
		"todogroups edit":   {"todogroups", "edit", f.StageGroup.ID, "--title", "Renamed"},
		"todogroups move":   {"todogroups", "move", f.KitchenGroup.ID, "--top"},
		"todogroups delete": {"todogroups", "delete", f.StageGroup.ID},

		"todos create":            {"todos", "create", "--list", f.TodoList.ID, "--title", "A new to-do"},
		"todos edit":              {"todos", "edit", f.LooseTodo.ID, "--title", "Renamed"},
		"todos complete":          {"todos", "complete", f.LooseTodo.ID},
		"todos reopen":            {"todos", "reopen", f.DoneTodo.ID},
		"todos move":              {"todos", "move", f.LooseTodo.ID, "--top"},
		"todos delete":            {"todos", "delete", f.LooseTodo.ID},
		"todos subtasks create":   {"todos", "subtasks", "create", f.GroupedTodo.ID, "A subtask"},
		"todos subtasks edit":     {"todos", "subtasks", "edit", f.GroupedTodo.ID, "1", "--title", "Renamed"},
		"todos subtasks complete": {"todos", "subtasks", "complete", f.GroupedTodo.ID, "1"},
		"todos subtasks reopen":   {"todos", "subtasks", "reopen", f.GroupedTodo.ID, "2"},
		"todos subtasks move":     {"todos", "subtasks", "move", f.GroupedTodo.ID, "2", "--top"},
		"todos subtasks delete":   {"todos", "subtasks", "delete", f.GroupedTodo.ID, "1"},
		"todos repeat":            {"todos", "repeat", f.LooseTodo.ID},
		"todos series edit":       {"todos", "series", "edit", f.Series.ID, "--title", "Renamed", "--force"},
		"todos series stop":       {"todos", "series", "stop", f.Series.ID},
		"todos series resume":     {"todos", "series", "resume", f.Series.ID},
		"todos series show-now":   {"todos", "series", "show-now", f.Series.ID},
		"todos comments create":   {"todos", "comments", "create", f.LooseTodo.ID, "Another comment"},
		"todos comments edit":     {"todos", "comments", "edit", f.LooseTodo.ID, todoComment, "Other words"},
		"todos comments delete":   {"todos", "comments", "delete", f.LooseTodo.ID, todoComment},
		"todos comments react":    {"todos", "comments", "react", f.LooseTodo.ID, todoComment, "👍"},
		"todos comments unreact":  {"todos", "comments", "unreact", f.LooseTodo.ID, todoComment, "👍"},

		"checkins create": {"checkins", "create", "--tool", f.CheckinsTool.ID, "--title", "New", "--rhythm", "daily",
			"--weekday", "mon", "--time", "09:00", "--ask", f.Anke.ID},
		"checkins edit":           {"checkins", "edit", f.DailyQuestion.ID, "--title", "Renamed"},
		"checkins pause":          {"checkins", "pause", f.DailyQuestion.ID},
		"checkins resume":         {"checkins", "resume", f.PausedQuestion.ID},
		"checkins answers create": {"checkins", "answers", "create", f.DailyQuestion.ID, "An answer"},
		"checkins answers edit":   {"checkins", "answers", "edit", f.BrunosAnswer.ID, "Other words"},

		"posts create":           {"posts", "create", "--tool", f.Posts.ID, "--title", "New", "--body", "Words", "--audience", "nobody"},
		"posts edit":             {"posts", "edit", f.Post.ID, "--title", "Renamed"},
		"posts pin":              {"posts", "pin", f.Post.ID},
		"posts unpin":            {"posts", "unpin", f.PinnedPost.ID},
		"posts archive":          {"posts", "archive", f.Post.ID},
		"posts unarchive":        {"posts", "unarchive", f.ArchivedPost.ID},
		"posts delete":           {"posts", "delete", f.Post.ID},
		"posts comments create":  {"posts", "comments", "create", f.Post.ID, "Another comment"},
		"posts comments edit":    {"posts", "comments", "edit", f.Post.ID, postComment, "Other words"},
		"posts comments delete":  {"posts", "comments", "delete", f.Post.ID, postComment},
		"posts comments react":   {"posts", "comments", "react", f.Post.ID, postComment, "👍"},
		"posts comments unreact": {"posts", "comments", "unreact", f.Post.ID, postComment, "👍"},

		"events create": {"events", "create", "--tool", f.Calendar.ID, "--title", "New", "--first-day", "2026-10-20"},
		"events edit":   {"events", "edit", f.OnceEvent.ID, "--title", "Renamed"},
		"events delete": {"events", "delete", f.OnceEvent.ID},

		"events settings": {"events", "settings", f.Calendar.ID, "--content", "eventsOnly"},

		"events comments create":  {"events", "comments", "create", f.OnceEvent.ID, "Another comment"},
		"events comments edit":    {"events", "comments", "edit", f.OnceEvent.ID, eventComment, "Other words"},
		"events comments delete":  {"events", "comments", "delete", f.OnceEvent.ID, eventComment},
		"events comments react":   {"events", "comments", "react", f.OnceEvent.ID, eventComment, "👍"},
		"events comments unreact": {"events", "comments", "unreact", f.OnceEvent.ID, eventComment, "👍"},

		"boards coordinator claim": {"boards", "coordinator", "claim", f.Board.ID, "--session", "planning"},
	}
}

// notAWriteIntoAProject are the commands that read, that write nowhere on the
// host, or that write where no project is: the read mark of a chat is taken in
// an archived project as well, and so is a reminder, which is one's own; a
// conversation hangs on none. The coordinator of a board is released in an
// archived project as well. A line of the bell is one's own, and one about an
// archived project is not found at all. An upload is tested by itself, because
// it needs a file.
var notAWriteIntoAProject = []string{
	"auth login", "auth logout", "auth status", "setup", "doctor", "me", "version", "commands",
	"skill show", "skill install", "skill status",
	"projects list", "boards list", "boards show", "boards archive", "people list", "trash list",
	"config get", "config set",
	"cards mine", "cards show", "cards notes show", "cards notes attachments list", "cards notes attachments show", "cards notes attachments download", "comments list", "comments unread", "cards steps list", "chats list", "chats show", "messages list",
	"docs tools", "docs tool", "docs list", "docs show", "docs uploads show",
	"docs versions list", "docs versions show", "docs uploads versions list",
	"todolists tools", "todolists tool", "todolists list", "todolists show", "todolists templates", "todolists comments list",
	"todos mine", "todos show", "todos completed", "todos subtasks list", "todos series list", "todos series show",
	"todos comments list",
	"checkins tools", "checkins tool", "checkins list", "checkins show", "checkins reminders",
	"checkins answers list", "checkins answers show",
	"posts tools", "posts tool", "posts categories", "posts list", "posts show", "posts comments list",
	"events tools", "events tool", "events list", "events mine", "events show", "events comments list",
	"reminders list", "reminders set", "reminders delete",
	"chats mark-read", "conversations list", "conversations with", "conversations people",
	"uploads create", "uploads download", "import basecamp",
	"work start", "work stop", "work list", "heartbeat", "hooks install", "hooks status",
	"notifications wait", "notifications count", "notifications show", "notifications mark-read",
	"boards coordinator release", "boards coordinator show",
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
		{"cards", "steps", "list", e.fixture.CardInDoing.ID},
		{"people", "list", e.fixture.Project.ID},
		{"trash", "list", e.fixture.Project.ID},
		{"chats", "list", e.fixture.Project.ID},
		{"messages", "list", e.fixture.Chat.ID},
		{"chats", "mark-read", e.fixture.Chat.ID},
		{"posts", "tools", e.fixture.Project.ID},
		{"posts", "categories", e.fixture.Project.ID},
		{"posts", "list", e.fixture.Posts.ID},
		{"posts", "list", e.fixture.Posts.ID, "--archived"},
		{"posts", "show", e.fixture.Post.ID},
		{"posts", "comments", "list", e.fixture.Post.ID},
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

// putDocsAndFilesAway puts a document, an upload and a folder of the fixture's
// tool into the trash and an upload into the archive, for the writes that
// bring them back.
func putDocsAndFilesAway(e *testEnv) (document, upload, folder, archivedUpload string) {
	f := e.fixture
	trashed := fakeapi.EntryState{Trashed: true, TrashedAt: time.Now(), TrashedBy: f.Bruno.ID}
	document, upload, folder, archivedUpload = "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e601", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e602",
		"0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e603", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e604"
	f.Files.Documents = append(f.Files.Documents, &fakeapi.Document{ID: document, Title: "Gone", CreatorID: f.Bruno.ID, EntryState: trashed})
	f.Files.Uploads = append(f.Files.Uploads,
		&fakeapi.FilesUpload{ID: upload, Title: "gone.pdf", BlobID: f.FilesUpload.BlobID, CreatorID: f.Bruno.ID, EntryState: trashed},
		&fakeapi.FilesUpload{ID: archivedUpload, Title: "away.pdf", BlobID: f.FilesUpload.BlobID, CreatorID: f.Bruno.ID, Archived: true})
	f.Files.Folders = append(f.Files.Folders, &fakeapi.Folder{ID: folder, Title: "Gone", CreatorID: f.Bruno.ID, EntryState: trashed})
	return document, upload, folder, archivedUpload
}
