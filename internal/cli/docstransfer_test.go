package cli

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// onTheTeamOfTheOtherProject lets the signed-in person reach the Docs & Files
// tool of the other project as one of its team.
func onTheTeamOfTheOtherProject(e *testEnv) {
	e.fixture.OtherProject.MemberIDs = append(e.fixture.OtherProject.MemberIDs, e.fixture.Bruno.ID)
}

// A document moves into a tool of another project with --tool: the ID goes
// out in lowercase beside the place, the summary names where it stands now,
// and a notice says what the move did besides. A label of the old project
// comes off, one of the new project stays.
func TestADocumentMovesIntoAToolOfAnotherProject(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	onTheTeamOfTheOtherProject(e)
	oldLabel := e.server.AddLabel(f.Project.ID, "", "Urgent", "red")
	newLabel := e.server.AddLabel(f.OtherProject.ID, f.OtherFiles.ID, "Filed", "blue")
	e.server.PutLabelOn(f.Document.ID, oldLabel)
	e.server.PutLabelOn(f.Document.ID, newLabel)

	env := runOK(t, e, "docs", "move", f.Document.ID, "--tool", strings.ToUpper(f.OtherFiles.ID), "--top-level")
	body := e.lastWriteBody()
	if body["toolId"] != f.OtherFiles.ID {
		t.Fatalf("body %v", body)
	}
	if value, given := body["folderId"]; !given || value != nil {
		t.Fatalf("body %v: the top level goes out as null", body)
	}
	if env.Summary != "Moved document How the stage goes up to the top of Secret documents in project Secret" {
		t.Fatalf("summary %q", env.Summary)
	}
	if env.Notice != intoAnotherToolNotice {
		t.Fatalf("notice %q", env.Notice)
	}
	data := decode(t, env.Data)
	if data["id"] != f.Document.ID || data["toolId"] != f.OtherFiles.ID || data["projectId"] != f.OtherProject.ID {
		t.Fatalf("data %s", env.Data)
	}
	labels, _ := data["labels"].([]any)
	if len(labels) != 1 || labels[0].(map[string]any)["id"] != newLabel.ID {
		t.Fatalf("labels %v: only the label of the new project stays", labels)
	}
	if !slices.Contains(f.OtherFiles.Documents, f.Document) || slices.Contains(f.Files.Documents, f.Document) {
		t.Fatal("the document did not change its tool")
	}
	if shown := decode(t, runOK(t, e, "docs", "show", f.Document.ID).Data); shown["toolId"] != f.OtherFiles.ID {
		t.Fatalf("read back %v", shown)
	}
}

// A folder takes everything in it along into the other tool, the place it
// stood in drops it from its order, and the folder arrives hidden from
// clients. Its file moves to the other project with it.
func TestAFolderMovesIntoAnotherToolWithEverythingInIt(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	onTheTeamOfTheOtherProject(e)
	f.Folder.ShownToClients = true
	runOK(t, e, "docs", "order", f.Files.ID, "manual")

	env := runOK(t, e, "docs", "folders", "move", f.Folder.ID, "--tool", f.OtherFiles.ID, "--top-level")
	if env.Summary != "Moved folder Plans to the top of Secret documents in project Secret" || env.Notice != intoAnotherToolNotice {
		t.Fatalf("summary %q, notice %q", env.Summary, env.Notice)
	}
	if !slices.Contains(f.OtherFiles.Documents, f.DocumentInFolder) || !slices.Contains(f.OtherFiles.Uploads, f.FilesUpload) || slices.Contains(f.Files.Folders, f.Folder) {
		t.Fatal("what stands in the folder did not go along")
	}
	if f.Folder.ShownToClients {
		t.Fatal("the folder arrived shown to clients")
	}
	if slices.Contains(f.Files.Arranged[""], f.Folder.ID) {
		t.Fatal("the place it left keeps it in its order")
	}
	for _, file := range e.server.Uploads {
		if file.ID == f.FilesUpload.BlobID && file.Circle != f.OtherProject.ID {
			t.Fatalf("the file of the upload stays in project %s", file.Circle)
		}
	}
}

// An entry that arrives lands first among its kind in a place ordered
// automatically and at the end of one arranged by hand.
func TestAnEntryArrivesOnTopOfItsKindOrAtTheEndOfAnOrderByHand(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	onTheTeamOfTheOtherProject(e)

	runOK(t, e, "docs", "move", f.Document.ID, "--tool", f.OtherFiles.ID, "--top-level")
	if f.OtherFiles.Documents[0] != f.Document {
		t.Fatal("the document is not first among the documents there")
	}

	f.OtherFiles.Arranged = map[string][]string{"": {f.Document.ID, f.OtherDocument.ID}}
	runOK(t, e, "docs", "uploads", "move", f.FilesUpload.ID, "--tool", f.OtherFiles.ID, "--top-level")
	if order := f.OtherFiles.Arranged[""]; order[len(order)-1] != f.FilesUpload.ID {
		t.Fatalf("order %v: the upload does not stand at the end", order)
	}
}

// --tool names the entry's own tool as well, and the move stays within it. A
// move without --tool sends no toolId and draws no notice.
func TestToolNamingTheOwnToolMovesWithinIt(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "docs", "move", f.Document.ID, "--tool", f.Files.ID, "--folder", f.Folder.ID)
	if f.Document.FolderID != f.Folder.ID || env.Summary != "Moved document How the stage goes up to Plans in Documents in project Test-Projekt" {
		t.Fatalf("summary %q", env.Summary)
	}

	env = runOK(t, e, "docs", "move", f.Document.ID, "--top-level")
	if _, sent := e.lastWriteBody()["toolId"]; sent || env.Notice != "" || env.Summary != "Moved document How the stage goes up to the top of Documents" {
		t.Fatalf("summary %q, notice %q, body %v", env.Summary, env.Notice, e.lastWriteBody())
	}
}

// A neighbour does not go with --tool, and a --tool that is no ID is refused:
// both before a request goes out.
func TestToolIsRefusedBesideANeighbour(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, flags := range map[string][]string{
		"after":    {"--top-level", "--after", f.OtherDocument.ID},
		"before":   {"--top-level", "--before", f.OtherDocument.ID},
		"no ID":    {"--top-level", "--tool", "Secret"},
		"no place": {},
	} {
		t.Run(name, func(t *testing.T) {
			args := append([]string{"docs", "move", f.Document.ID, "--tool", f.OtherFiles.ID}, flags...)
			if name == "no ID" {
				args = append([]string{"docs", "move", f.Document.ID}, flags...)
			}
			before := len(e.server.Requests())
			env, r := e.runJSON("", args...)
			expectCode(t, r, ExitUsage)
			if len(e.server.Requests()) != before {
				t.Fatal("a request went out")
			}
			if (name == "after" || name == "before") && !strings.Contains(env.Error, "on top of its kind") {
				t.Fatalf("error %q does not say where the entry goes", env.Error)
			}
		})
	}
}

// What Minka refuses a move into another tool comes back as exit 9 with a hint
// that speaks of the other tool.
func TestARefusedMoveIntoAnotherToolSaysWhatToDo(t *testing.T) {
	for name, c := range map[string]struct {
		prepare func(e *testEnv) []string
		code    string
		hint    []string
	}{
		"a tool of a project one is not on the team of": {func(e *testEnv) []string {
			return []string{"docs", "move", e.fixture.Document.ID, "--tool", e.fixture.OtherFiles.ID, "--top-level"}
		}, apiCodeToolUnknown, []string{"on the team of", "minka docs tools <projectId>"}},
		"a folder of the entry's own tool": {func(e *testEnv) []string {
			onTheTeamOfTheOtherProject(e)
			return []string{"docs", "move", e.fixture.Document.ID, "--tool", e.fixture.OtherFiles.ID, "--folder", e.fixture.Folder.ID}
		}, apiCodeFolderUnknown, []string{"no folder of the tool --tool names", "every folder of another tool is none"}},
		"an archived target project": {func(e *testEnv) []string {
			onTheTeamOfTheOtherProject(e)
			e.fixture.OtherProject.Archived = true
			return []string{"docs", "move", e.fixture.Document.ID, "--tool", e.fixture.OtherFiles.ID, "--top-level"}
		}, apiCodeProjectArchived, []string{"the one of the tool --tool names"}},
		"a file shared with what stays": {func(e *testEnv) []string {
			onTheTeamOfTheOtherProject(e)
			f := e.fixture
			f.Files.Uploads = append(f.Files.Uploads, &fakeapi.FilesUpload{ID: "0199aaaa-0000-7000-8000-000000000001", Title: "copy.pdf", BlobID: f.FilesUpload.BlobID, CreatorID: f.Bruno.ID})
			return []string{"docs", "uploads", "move", f.FilesUpload.ID, "--tool", f.OtherFiles.ID, "--top-level"}
		}, apiCodeEntrySharesFiles, []string{"within its project only", "does not say which file"}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON("", c.prepare(e)...)
			expectCode(t, r, ExitValidation)
			if env.Meta["apiCode"] != c.code {
				t.Fatalf("apiCode %v, want %s", env.Meta["apiCode"], c.code)
			}
			for _, word := range c.hint {
				if !strings.Contains(env.Hint, word) {
					t.Fatalf("hint %q does not say %q", env.Hint, word)
				}
			}
		})
	}
}

// The CLI never sends a neighbour with --tool; should a Minka answer so all the
// same, the hint says where an entry goes there.
func TestNeighboursAcrossToolsIsNamed(t *testing.T) {
	refusal := entryOrderRefusal(&session{host: "minka.example"}, &api.Error{Method: http.MethodPost, StatusCode: 422, Code: apiCodeNeighboursAcrossTools, Message: "An entry moving into another tool goes on top of its kind there."}, documentKind, unknownID, "move")
	if refusal.ExitCode() != ExitValidation || !strings.Contains(refusal.Hint, "on top of its kind") {
		t.Fatalf("refusal %+v", refusal)
	}
}

// A Minka that reads no toolId moves the entry within its own tool; the answer
// that stands elsewhere than --tool says is exit 7 with that answer.
func TestAMinkaThatMovesWithinToolsOnlyIsNamed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	onTheTeamOfTheOtherProject(e)
	e.server.MovesWithinToolsOnly = true

	env, r := e.runJSON("", "docs", "move", f.DocumentInFolder.ID, "--tool", f.OtherFiles.ID, "--top-level")
	expectCode(t, r, ExitAPI)
	if env.Error != "this Minka does not move entries between tools: the document stands at the top of Documents now, or stood there already" || env.Notice != "" {
		t.Fatalf("error %q, notice %q", env.Error, env.Notice)
	}
	if !strings.Contains(env.Hint, "does not know where it stood") || !strings.Contains(env.Hint, "minka docs move "+f.DocumentInFolder.ID+" --folder <its old folder>") {
		t.Fatalf("hint %q", env.Hint)
	}
	if decode(t, env.Data)["toolId"] != f.Files.ID {
		t.Fatalf("data %s", env.Data)
	}
}
