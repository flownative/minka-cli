package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/markdown"
)

func decisionsBlockOf(toolID, filters string) string {
	return `<minka-decisions tool="` + toolID + `"` + filters + `></minka-decisions>`
}

func decisionsMarkerOf(toolID string) string {
	return `<!-- minka:decisions tool="` + toolID + `" -->`
}

// The body of the text view holds the marker alone; the rows follow in a
// section of their own, named by the block's position and attributes.
// --body prints the body alone, and it goes back as the same element. Rows
// copied into a body are refused before anything is sent.
func TestADocumentShowsItsBlockAsAMarkerAndItsRowsApart(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Document.Body = "<p>It holds:</p>" + decisionsBlockOf(f.Decisions.ID, "")

	text := e.run("", "docs", "show", f.Document.ID, "--styled")
	expectCode(t, text, ExitOK)
	body, rest, found := strings.Cut(text.stdout[strings.Index(text.stdout, "## Body\n\n"):], "\n## Decisions blocks (1)\n")
	if !found || !strings.Contains(body, "It holds:\n\n"+decisionsMarkerOf(f.Decisions.ID)+"\n") || strings.Contains(body, markdown.ReadOutLead) {
		t.Fatalf("the body is not the marker alone, or no rows follow it:\n%s", text.stdout)
	}
	rows := "### Position 0: tool=\"" + f.Decisions.ID + "\"\n\n> " + markdown.ReadOutLead + " Decisions\n> - DEC-1 We met on Tuesdays — In force\n> - DEC-2 We meet on Mondays — In force\n> - DEC-3 We buy a tent — Proposed\n> - DEC-4 The fee rises to 40 euros — Draft"
	if !strings.Contains(rest, rows) {
		t.Fatalf("the rows do not follow the body:\n%s", text.stdout)
	}

	shown := runOK(t, e, "docs", "show", f.Document.ID)
	alone := e.run("", "docs", "show", f.Document.ID, "--body")
	expectCode(t, alone, ExitOK)
	if alone.stdout != "It holds:\n\n"+decisionsMarkerOf(f.Decisions.ID)+"\n" || !strings.Contains(alone.stderr, "--expect "+decode(t, shown.Data)["updatedAt"].(string)) {
		t.Fatalf("--body printed %q, on stderr %q", alone.stdout, alone.stderr)
	}
	_, r := e.runJSON(alone.stdout, "docs", "edit", f.Document.ID, "--body", "-", "--force")
	expectCode(t, r, ExitOK)
	if sent := e.lastWriteBody()["body"]; sent != "<p>It holds:</p>\n"+decisionsBlockOf(f.Decisions.ID, "") {
		t.Fatalf("sent %q", sent)
	}

	writes := len(e.server.WritingRequests())
	copied := alone.stdout + "\n" + rest[strings.Index(rest, "> "):]
	env, r := e.runJSON(copied, "docs", "edit", f.Document.ID, "--body", "-", "--force")
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes || !strings.Contains(env.Hint, "read out") {
		t.Fatalf("copied rows: hint %q, %d writes", env.Hint, len(e.server.WritingRequests())-writes)
	}

	if _, r := e.runJSON("", "docs", "show", f.Document.ID, "--body"); r.code != ExitUsage {
		t.Errorf("--body with --json exits %d", r.code)
	}
}

// JSON hands the blocks over as Minka answers them.
func TestTheBlocksOfAPostAreInItsData(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Post.Body = "<p>It holds:</p>" + decisionsBlockOf(f.Decisions.ID, "")

	env := runOK(t, e, "posts", "show", f.Post.ID)
	blocks, _ := decode(t, env.Data)["decisionBlocks"].([]any)
	if len(blocks) != 1 || blocks[0].(map[string]any)["markdown"] == "" {
		t.Fatalf("decisionBlocks %v", blocks)
	}

	text := e.run("", "posts", "show", f.Post.ID, "--styled")
	if !strings.Contains(text.stdout, "## Decisions blocks (1)\n\n### Position 0: tool=\""+f.Decisions.ID+"\"\n\n> "+markdown.ReadOutLead+" Decisions\n") {
		t.Fatalf("the view of a post shows no rows:\n%s", text.stdout)
	}
	if alone := e.run("", "posts", "show", f.Post.ID, "--body"); alone.stdout != "It holds:\n\n"+decisionsMarkerOf(f.Decisions.ID)+"\n" {
		t.Errorf("--body printed %q", alone.stdout)
	}
}

// A marker that cannot be a block stops the write before anything is sent.
func TestAMalformedMarkerSendsNothing(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	env, r := e.runJSON("", "docs", "edit", f.Document.ID, "--force", "--body", `<!-- minka:decisions tool="`+f.Decisions.ID+`" summary="x" -->`)
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != 0 {
		t.Fatal("a request was sent")
	}
	if !strings.Contains(env.Error, `"summary"`) || !strings.Contains(env.Hint, "minka decisions tools <projectId>") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
}

// Minka's refusal of a block keeps its message and says where the
// identifiers come from; a locked document says what unlocks it, for an
// edit and for restoring a version.
func TestARefusedBlockAndALockedDocumentSayWhatToDo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	unknown := "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	marker := `<!-- minka:decisions tool="` + f.Decisions.ID + `" status="` + unknown + `" -->`

	for _, command := range [][]string{
		{"docs", "edit", f.Document.ID, "--force", "--body", marker},
		{"posts", "edit", f.Post.ID, "--body", "It holds:\n\n" + marker},
	} {
		env, r := e.runJSON("", command...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeDecisionBlockInvalid || !strings.Contains(env.Error, `"status"`) || !strings.Contains(env.Hint, "minka decisions statuses <toolId>") {
			t.Errorf("%s: apiCode %v, error %q, hint %q", command[0], env.Meta["apiCode"], env.Error, env.Hint)
		}
	}

	versions := runOK(t, e, "docs", "versions", "list", f.Document.ID)
	var list []struct{ ID string }
	if err := json.Unmarshal(versions.Data, &list); err != nil || len(list) == 0 {
		t.Fatalf("versions %s: %v", versions.Data, err)
	}
	versionID := list[0].ID
	e.server.LockDocument(f.Document.ID)
	for _, command := range [][]string{
		{"docs", "edit", f.Document.ID, "--force", "--body", marker},
		{"docs", "versions", "restore", f.Document.ID, versionID},
	} {
		env, r := e.runJSON("", command...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeDocumentLocked || !strings.Contains(env.Hint, "minka docs unlock "+f.Document.ID+";") {
			t.Errorf("%s: apiCode %v, hint %q", strings.Join(command[:2], " "), env.Meta["apiCode"], env.Hint)
		}
	}
	if text := e.run("", "docs", "show", f.Document.ID, "--styled"); !strings.Contains(text.stdout, "Locked: yes") {
		t.Errorf("the view does not say the document is locked:\n%s", text.stdout)
	}
}

// An export is read as files: a page carries the marker and the rows, and a
// page with a block is read again on every run, since its rows change
// without it.
func TestAnExportedPageCarriesTheRowsOfItsBlocks(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	// A page whose references all lead into the export, which is otherwise
	// taken from the cache.
	b.costs.Body += decisionsBlockOf(f.Decisions.ID, "")

	for _, format := range []string{"markdown", "html"} {
		out := filepath.Join(t.TempDir(), "book")
		runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--format", format)
		f.Decision.Title = "We meet on Mondays, " + format
		env := runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--format", format)
		// The tools page refers outside the export and is read again anyway.
		if read := decodeExport(t, env.Data).DocumentsRead; read != 2 {
			t.Errorf("%s: %d pages read again, want the one with a block and the one referring outside", format, read)
		}
		name := map[string]string{"markdown": "what-the-tent-costs.md", "html": "what-the-tent-costs.html"}[format]
		page := readFile(t, filepath.Join(out, name))
		if !strings.Contains(page, "minka:decisions tool=\""+f.Decisions.ID+"\"") || !strings.Contains(page, markdown.ReadOutLead) || !strings.Contains(page, "We meet on Mondays, "+format) {
			t.Errorf("%s: the page does not carry the marker and today's rows:\n%s", format, page)
		}
	}
}
