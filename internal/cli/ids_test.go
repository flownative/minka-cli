package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// anyCaseUUID finds an ID in what went out, whichever case it is written in.
var anyCaseUUID = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// idInCapitals is one run in which every ID is typed in capitals, and the
// inputs of its command that carry one.
type idInCapitals struct {
	command string   // the command path, without "minka"
	inputs  []string // the flags and the named positions it gives an ID in
	local   bool     // it keeps to this machine and sends nothing
	prepare func(e *testEnv)
	args    func(e *testEnv, f *fakeapi.Fixture) []string
}

// The IDs of two steps, with letters in them for capitals to change.
const openStepID = "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"
const doneStepID = "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f7"

// storedBlobID is a file stored and put nowhere yet, with letters in its ID.
const storedBlobID = "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5fb"

func withStepsSpelledInLetters(e *testEnv) {
	e.fixture.CardInTriage.Steps = []*fakeapi.Step{{ID: openStepID, Title: "Check"}, {ID: doneStepID, Title: "Checked", Completed: true}}
}

func asAgent(e *testEnv) {
	e.vars["MINKA_TOKEN"] = e.fixture.AgentToken
}

var up = strings.ToUpper

// idsInCapitals names every input that takes an ID, in every command that has
// one; the test below holds the command tree against it.
var idsInCapitals = []idInCapitals{
	{command: "cards show", inputs: []string{"<cardId>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{up(f.CardInTriage.ID)}
	}},
	{command: "comments react", inputs: []string{"<cardId>", "<commentId>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{up(f.CardInTriage.ID), up(f.CardInTriage.Comments[0].ID), "🚀"}
	}},
	{command: "boards archive", inputs: []string{"<boardId>", "--before"}, prepare: func(e *testEnv) {
		e.server.PutInTheArchive(e.fixture.CardInDoing)
		e.server.PutInTheArchive(e.fixture.CardInTriage)
	}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{up(f.Board.ID), "--before", up(f.CardInTriage.ID)}
	}},
	{command: "cards assign", inputs: []string{"<person>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, up(f.Anke.ID)}
	}},
	{command: "cards unassign", inputs: []string{"<person>"}, prepare: func(e *testEnv) {
		e.fixture.CardInTriage.AssigneeIDs = []string{e.fixture.Anke.ID}
	}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, up(f.Anke.ID)}
	}},
	{command: "cards subscribe", inputs: []string{"<person>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, up(f.Anke.ID)}
	}},
	{command: "cards unsubscribe", inputs: []string{"<person>"}, prepare: func(e *testEnv) {
		e.fixture.CardInTriage.SubscriberIDs = []string{e.fixture.Bruno.ID, e.fixture.Anke.ID}
	}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, up(f.Anke.ID)}
	}},
	{command: "cards create", inputs: []string{"--board", "--column", "--assignee"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--board", up(f.Board.ID), "--column", up(f.Doing.ID), "--assignee", up(f.Anke.ID), "--title", "Pack"}
	}},
	{command: "cards move", inputs: []string{"--column", "--after"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, "--column", up(f.Doing.ID), "--after", up(f.CardInDoing.ID)}
	}},
	{command: "cards move", inputs: []string{"--before"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, "--column", f.Doing.ID, "--before", up(f.CardInDoing.ID)}
	}},
	{command: "steps complete", inputs: []string{"<step>"}, prepare: withStepsSpelledInLetters, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, up(openStepID)}
	}},
	{command: "steps reopen", inputs: []string{"<step>"}, prepare: withStepsSpelledInLetters, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, up(doneStepID)}
	}},
	{command: "steps delete", inputs: []string{"<step>"}, prepare: withStepsSpelledInLetters, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.CardInTriage.ID, up(openStepID)}
	}},
	{command: "columns on-hold", inputs: []string{"<column>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{up(f.Doing.ID)}
	}},
	{command: "columns on-hold", inputs: []string{"--board"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"Doing", "--board", up(f.Board.ID)}
	}},
	{command: "conversations with", inputs: []string{"<person>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{up(f.Anke.ID)}
	}},
	{command: "conversations with", inputs: []string{"--project"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"Anke", "--project", up(f.Project.ID)}
	}},
	{command: "checkins create", inputs: []string{"--tool", "--ask"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--tool", up(f.CheckinsTool.ID), "--title", "How is it going?", "--rhythm", "weekly", "--weekday", "mon", "--time", "09:00", "--ask", up(f.Anke.ID)}
	}},
	{command: "checkins edit", inputs: []string{"--ask"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.DailyQuestion.ID, "--ask", up(f.Anke.ID)}
	}},
	{command: "checkins answers list", inputs: []string{"--person"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.DailyQuestion.ID, "--person", up(f.Anke.ID)}
	}},
	{command: "checkins answers list", inputs: []string{"--before"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.DailyQuestion.ID, "--before", up(f.AnkesAnswer.ID)}
	}},
	{command: "docs list", inputs: []string{"--folder"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.Files.ID, "--folder", up(f.Folder.ID)}
	}},
	{command: "docs create", inputs: []string{"--tool", "--folder", "--notify"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--tool", up(f.Files.ID), "--folder", up(f.Folder.ID), "--title", "Costs", "--audience", "chosen", "--notify", up(f.Anke.ID), "--body", "Four hundred."}
	}},
	{command: "docs uploads create", inputs: []string{"--tool", "--project", "--folder"}, args: func(e *testEnv, f *fakeapi.Fixture) []string {
		return []string{aFileToUpload(e), "--tool", up(f.Files.ID), "--project", up(f.Project.ID), "--folder", up(f.Folder.ID)}
	}},
	{command: "docs uploads create", inputs: []string{"--blob"}, prepare: func(e *testEnv) {
		e.server.Uploads = append(e.server.Uploads, &fakeapi.Upload{ID: storedBlobID, Circle: e.fixture.Project.ID, Filename: "notes.txt",
			ContentType: "text/plain", Content: []byte("Four hundred."), Size: 13, UploaderID: e.fixture.Bruno.ID})
	}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--blob", up(storedBlobID), "--tool", f.Files.ID}
	}},
	{command: "docs comments create", inputs: []string{"--document"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"Me too", "--document", up(f.Document.ID)}
	}},
	{command: "docs comments create", inputs: []string{"--upload"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"Me too", "--upload", up(f.FilesUpload.ID)}
	}},
	{command: "docs comments react", inputs: []string{"--document"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.Document.Comments[0].ID, "👍", "--document", up(f.Document.ID)}
	}},
	{command: "docs comments react", inputs: []string{"--upload"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.FilesUpload.Comments[0].ID, "👍", "--upload", up(f.FilesUpload.ID)}
	}},
	{command: "docs comments unreact", inputs: []string{"--document"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.Document.Comments[0].ID, "👍", "--document", up(f.Document.ID)}
	}},
	{command: "docs comments unreact", inputs: []string{"--upload"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.FilesUpload.Comments[0].ID, "👍", "--upload", up(f.FilesUpload.ID)}
	}},
	{command: "docs comments edit", inputs: []string{"--upload"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.FilesUpload.Comments[0].ID, "Printed twice", "--upload", up(f.FilesUpload.ID)}
	}},
	{command: "docs comments edit", inputs: []string{"--document"}, prepare: func(e *testEnv) {
		e.fixture.Document.Comments[0].AuthorID = e.fixture.Bruno.ID
	}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.Document.Comments[0].ID, "I can help twice", "--document", up(f.Document.ID)}
	}},
	{command: "docs comments delete", inputs: []string{"--upload"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.FilesUpload.Comments[0].ID, "--upload", up(f.FilesUpload.ID)}
	}},
	{command: "docs comments delete", inputs: []string{"--document"}, prepare: func(e *testEnv) {
		e.fixture.Document.Comments[0].AuthorID = e.fixture.Bruno.ID
	}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.Document.Comments[0].ID, "--document", up(f.Document.ID)}
	}},
	{command: "messages create", inputs: []string{"--client-message-id"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.Chat.ID, "Tent is booked", "--client-message-id", up(openStepID)}
	}},
	{command: "posts create", inputs: []string{"--tool", "--category", "--notify"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--tool", up(f.Posts.ID), "--category", up(f.Announcement.ID), "--title", "Setting up", "--audience", "chosen", "--notify", up(f.Anke.ID), "--body", "Friday."}
	}},
	{command: "events create", inputs: []string{"--tool", "--participant"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--tool", up(f.Calendar.ID), "--title", "Setting up", "--first-day", "2026-10-20", "--participant", up(f.Anke.ID)}
	}},
	{command: "events edit", inputs: []string{"--participant"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.OnceEvent.ID, "--participant", up(f.Anke.ID)}
	}},
	{command: "posts edit", inputs: []string{"--category"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.Post.ID, "--category", up(f.Heartbeat.ID)}
	}},
	{command: "todogroups create", inputs: []string{"--list"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--list", up(f.TodoList.ID), "--title", "Lights"}
	}},
	{command: "todogroups move", inputs: []string{"--after"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.StageGroup.ID, "--after", up(f.KitchenGroup.ID)}
	}},
	{command: "todogroups move", inputs: []string{"--before"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.KitchenGroup.ID, "--before", up(f.StageGroup.ID)}
	}},
	{command: "todolists create", inputs: []string{"--tool"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--tool", up(f.Todos.ID), "--title", "Packing"}
	}},
	{command: "todos create", inputs: []string{"--list", "--group", "--assignee", "--notify-when-done"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--list", up(f.TodoList.ID), "--group", up(f.StageGroup.ID), "--title", "Book the van", "--assignee", up(f.Anke.ID), "--notify-when-done", up(f.Anke.ID)}
	}},
	{command: "todos edit", inputs: []string{"--assignee", "--notify-when-done"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.LooseTodo.ID, "--assignee", up(f.Anke.ID), "--notify-when-done", up(f.Anke.ID)}
	}},
	{command: "todos series edit", inputs: []string{"--assignee", "--notify-when-done"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.Series.ID, "--assignee", up(f.Anke.ID), "--notify-when-done", up(f.Anke.ID), "--force"}
	}},
	{command: "todos mine", inputs: []string{"--after"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{"--after", up(f.GroupedTodo.ID)}
	}},
	{command: "todos move", inputs: []string{"--list", "--group", "--after"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.LooseTodo.ID, "--list", up(f.TodoList.ID), "--group", up(f.StageGroup.ID), "--after", up(f.GroupedTodo.ID)}
	}},
	{command: "todos move", inputs: []string{"--before"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.LooseTodo.ID, "--group", f.StageGroup.ID, "--before", up(f.GroupedTodo.ID)}
	}},
	{command: "todos subtasks create", inputs: []string{"--assignee"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.GroupedTodo.ID, "Measure", "--assignee", up(f.Anke.ID)}
	}},
	{command: "todos subtasks edit", inputs: []string{"<subtask>", "--assignee"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.GroupedTodo.ID, up(f.GroupedTodo.Subtasks[0].ID), "--assignee", up(f.Bruno.ID)}
	}},
	{command: "todos subtasks move", inputs: []string{"<subtask>", "--after"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.GroupedTodo.ID, up(f.GroupedTodo.Subtasks[0].ID), "--after", up(f.GroupedTodo.Subtasks[1].ID)}
	}},
	{command: "todos subtasks move", inputs: []string{"--before"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.GroupedTodo.ID, f.GroupedTodo.Subtasks[1].ID, "--before", up(f.GroupedTodo.Subtasks[0].ID)}
	}},
	{command: "todos subtasks complete", inputs: []string{"<subtask>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.GroupedTodo.ID, up(f.GroupedTodo.Subtasks[0].ID)}
	}},
	{command: "todos subtasks reopen", inputs: []string{"<subtask>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.GroupedTodo.ID, up(f.GroupedTodo.Subtasks[1].ID)}
	}},
	{command: "todos subtasks delete", inputs: []string{"<subtask>"}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{f.GroupedTodo.ID, up(f.GroupedTodo.Subtasks[0].ID)}
	}},
	{command: "uploads create", inputs: []string{"--project"}, args: func(e *testEnv, f *fakeapi.Fixture) []string {
		return []string{aFileToUpload(e), "--project", up(f.Project.ID)}
	}},
	{command: "uploads create", inputs: []string{"--conversation"}, args: func(e *testEnv, f *fakeapi.Fixture) []string {
		return []string{aFileToUpload(e), "--conversation", up(f.Conversation.ID)}
	}},
	{command: "work start", inputs: []string{"<card>"}, prepare: asAgent, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{up(f.CardInTriage.ID)}
	}},
	{command: "work stop", inputs: []string{"<card>"}, local: true, prepare: func(e *testEnv) {
		asAgent(e)
		e.run("", "work", "start", e.fixture.CardInTriage.ID)
	}, args: func(_ *testEnv, f *fakeapi.Fixture) []string {
		return []string{up(f.CardInTriage.ID)}
	}},
}

func aFileToUpload(e *testEnv) string {
	path := filepath.Join(e.t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("Four hundred."), 0o600); err != nil {
		e.t.Fatal(err)
	}
	return path
}

// Lowercase is the form Minka keeps an ID in, and the one every version of it
// reads; one in capitals named nothing before Minka began to read IDs in any
// case, at some addresses without a word. Whatever case an ID is typed in, it
// goes out in lowercase.
func TestAnIDTypedInCapitalsGoesOutInLowercase(t *testing.T) {
	for _, run := range idsInCapitals {
		t.Run(run.command+" "+strings.Join(run.inputs, " "), func(t *testing.T) {
			e := newTestEnv(t)
			if run.prepare != nil {
				run.prepare(e)
			}
			before := len(e.server.Requests())
			args := append(strings.Fields(run.command), run.args(e, e.fixture)...)
			_, r := e.runJSON("", args...)
			expectCode(t, r, ExitOK)

			sent := e.server.Requests()[before:]
			if len(sent) == 0 && !run.local {
				t.Fatal("no request went out")
			}
			for _, request := range sent {
				for _, part := range []string{request.Path, request.Query, string(request.Body)} {
					for _, id := range anyCaseUUID.FindAllString(part, -1) {
						if id != strings.ToLower(id) {
							t.Errorf("%s %s carries the ID %s in capitals", request.Method, request.Path, id)
						}
					}
				}
			}
		})
	}
}

// A step or a subtask named by its ID in capitals is the line with that ID:
// the command finds it among the lines it read, says which one it took off,
// and takes it off.
func TestAStepNamedInCapitalsIsReallyDeleted(t *testing.T) {
	e := newTestEnv(t)
	withStepsSpelledInLetters(e)

	env, r := e.runJSON("", "steps", "delete", e.fixture.CardInTriage.ID, up(openStepID))
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["wasOnCard"] != true || data["stepId"] != openStepID {
		t.Errorf("the step was not found on the card: %s", env.Data)
	}
	if len(e.fixture.CardInTriage.Steps) != 1 || e.fixture.CardInTriage.Steps[0].ID != doneStepID {
		t.Fatalf("the step is still on the card: %v", e.fixture.CardInTriage.Steps)
	}

	subtask := e.fixture.GroupedTodo.Subtasks[0].ID
	env, r = e.runJSON("", "todos", "subtasks", "delete", e.fixture.GroupedTodo.ID, up(subtask))
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["wasOnTodo"] != true || data["subtaskId"] != subtask {
		t.Errorf("the subtask was not found on the to-do: %s", env.Data)
	}
	if slices.ContainsFunc(e.fixture.GroupedTodo.Subtasks, func(s *fakeapi.Subtask) bool { return s.ID == subtask }) {
		t.Fatal("the subtask is still on the to-do")
	}
}

// A person named in capitals together with an audience that takes nobody
// hears what is wrong with the audience, not that the person is no member.
func TestAPersonToTellInCapitalsIsReadAsThePerson(t *testing.T) {
	for _, command := range [][]string{
		{"docs", "create", "--tool", "{files}"},
		{"posts", "create", "--tool", "{posts}"},
	} {
		e := newTestEnv(t)
		args := slices.Clone(command)
		args[3] = map[string]string{"{files}": e.fixture.Files.ID, "{posts}": e.fixture.Posts.ID}[args[3]]
		args = append(args, "--title", "Costs", "--audience", "everybody", "--notify", up(e.fixture.Anke.ID), "--body", "Four hundred.")

		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeNotifyIDsWithoutChosenAudience {
			t.Errorf("%s: apiCode %v, want %s", command[0], env.Meta["apiCode"], apiCodeNotifyIDsWithoutChosenAudience)
		}
	}
}

// Every ID a command is given as an argument is handed on in lowercase; a
// short identifier of a card stays as it was typed and is looked up.
func TestAnArgumentThatIsAnIDIsHandedOnInLowercase(t *testing.T) {
	cmd := &cobra.Command{Use: "try"}
	args := []string{up(openStepID), "MNK-1A", "Some Text"}
	if err := requireArgs("cardId", "cardId", "text")(cmd, args); err != nil {
		t.Fatal(err)
	}
	if want := []string{openStepID, "MNK-1A", "Some Text"}; !slices.Equal(args, want) {
		t.Fatalf("arguments %q, want %q", args, want)
	}
}

// idInputsExcepted take no ID a person types: a sequence number, a place Minka
// hands over as it is, or an identifier of Basecamp.
var idInputsExcepted = map[string]string{
	"messages list --after":     "a sequence number",
	"messages list --before":    "a sequence number",
	"posts list --before":       "a place Minka hands over",
	"todos completed --before":  "a place Minka hands over",
	"import basecamp --project": "an ID of Basecamp",
}

// idFlagNames are the flags that take an ID wherever they are. The tree test
// finds a flag that takes an ID only by this list or by its help, which has to
// say "ID"; a new flag that takes one and says neither slips past it. A call of
// requireID on one line with the flag name written out, or a map key of such a
// name, is held against this list. A call split over lines, one with the name
// in a variable or on another receiver is not: todogroups move passes its flag
// in a variable and cards move passes neighbourCard, and both are covered only
// because --after and --before are on the list.
var idFlagNames = []string{"after", "before", "assignee", "notify-when-done", "notify", "ask", "person", "list", "group", "tool", "board", "folder", "document", "upload", "blob", "category", "project", "conversation", "column", "client-message-id", "participant"}

// idPositions are the positions that take an ID among other names for a thing;
// a position whose name ends in "Id" takes nothing else and is held by the
// test of the arguments above.
var idPositions = []string{"<person>", "<card>", "<step>", "<subtask>", "<column>"}

// A command that takes an ID is run with it in capitals above, or said here
// why it is not.
func TestEveryInputThatTakesAnIDIsRunInCapitals(t *testing.T) {
	covered := map[string]bool{}
	for _, run := range idsInCapitals {
		for _, input := range run.inputs {
			covered[run.command+" "+input] = true
		}
	}

	root := newApp(t.Context(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, func(string) string { return "" }).newRootCmd()
	for _, path := range commandPaths(root) {
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil {
			t.Fatal(err)
		}
		var inputs []string
		for _, position := range idPositions {
			if strings.Contains(cmd.Use, position) {
				inputs = append(inputs, position)
			}
		}
		cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
			if slices.Contains(idFlagNames, flag.Name) || regexp.MustCompile(`\bIDs?\b`).MatchString(flag.Usage) {
				inputs = append(inputs, "--"+flag.Name)
			}
		})
		for _, input := range inputs {
			key := path + " " + input
			if !covered[key] && idInputsExcepted[key] == "" {
				t.Errorf("%s takes an ID and is not run with one in capitals", key)
			}
		}
	}
}

// Every flag a command checks with requireID is one the tree test knows as
// taking an ID, whatever its help says.
func TestEveryFlagCheckedAsAnIDIsKnownToTakeOne(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := regexp.MustCompile(`requireID\(cmd, "--([a-z-]+)"|"--([a-z-]+)": &`)
	found := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range checked.FindAllStringSubmatch(string(content), -1) {
			name := match[1] + match[2]
			found++
			if !slices.Contains(idFlagNames, name) {
				t.Errorf("%s checks --%s as an ID, and idFlagNames does not name it", source, name)
			}
		}
	}
	if found == 0 {
		t.Fatal("no flag checked as an ID was found; the pattern no longer matches the code")
	}
}

// Every position of every command whose name ends in "Id" takes an ID typed in
// capitals and hands it on in lowercase.
func TestEveryPositionThatTakesAnIDHandsItOnInLowercase(t *testing.T) {
	placeholder := regexp.MustCompile(`^<([^>]+)>$`)
	root := newApp(t.Context(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, func(string) string { return "" }).newRootCmd()
	positions := 0
	for _, path := range commandPaths(root) {
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil {
			t.Fatal(err)
		}
		var args, names []string
		for _, word := range strings.Fields(cmd.Use)[1:] {
			match := placeholder.FindStringSubmatch(word)
			if match == nil {
				// Flags, alternatives and optional positions follow the
				// required ones.
				break
			}
			names = append(names, match[1])
			switch {
			case strings.HasSuffix(match[1], "Id"):
				args = append(args, up(openStepID))
				positions++
			case match[1] == "card":
				args = append(args, "MNK-1A")
			default:
				args = append(args, "some-name")
			}
		}
		if cmd.Args == nil || len(args) == 0 {
			continue
		}
		if err := cmd.Args(cmd, args); err != nil {
			t.Errorf("%s %q: %v", path, args, err)
			continue
		}
		for i, name := range names {
			if strings.HasSuffix(name, "Id") && args[i] != openStepID {
				t.Errorf("%s hands <%s> on as %s", path, name, args[i])
			}
		}
	}
	if positions == 0 {
		t.Fatalf("no position named ...Id was found in %d commands", len(commandPaths(root)))
	}
}

// A project named for a lookup is checked as an ID before anything is asked.
func TestAConversationLooksUpNoNameInAProjectThatIsNoID(t *testing.T) {
	e := newTestEnv(t)
	before := len(e.server.Requests())

	_, r := e.runJSON("", "conversations", "with", "Anke", "--project", "nonsense")
	expectCode(t, r, ExitUsage)
	if sent := e.server.Requests()[before:]; len(sent) != 0 {
		t.Fatalf("%d requests went out, the first to %s", len(sent), sent[0].Path)
	}
}
