package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// The IDs of a file on the back of the card in triage and of its upload, with
// letters in them for capitals to change.
const (
	attachmentID     = "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5fc"
	attachmentBlobID = "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5fd"
)

// withAnAttachment puts research.md on the back of the card in triage, as
// Bruno stored it.
func withAnAttachment(e *testEnv) {
	withAFileOnTheBack(e, attachmentID, attachmentBlobID, "research.md", "text/markdown", []byte("# Research\n\n- tents hold 200\n"))
}

func withAFileOnTheBack(e *testEnv, fileID, blobID, name, contentType string, content []byte) {
	e.server.Uploads = append(e.server.Uploads, &fakeapi.Upload{ID: blobID, Circle: e.fixture.Project.ID, Filename: name,
		ContentType: contentType, Content: content, Size: len(content), UploaderID: e.fixture.Bruno.ID})
	card := e.fixture.CardInTriage
	card.BackFiles = append(card.BackFiles, &fakeapi.CardBackFile{ID: fileID, Filename: name,
		Versions: []fakeapi.CardBackFileVersion{{BlobID: blobID, UploadedByID: e.fixture.Bruno.ID}}})
}

func localFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func requestLines(requests []fakeapi.RecordedRequest) []string {
	var lines []string
	for _, request := range requests {
		lines = append(lines, request.Method+" "+request.Path)
	}
	return lines
}

// One command does both of Minka's requests, and reads the card and its back
// before the file travels.
func TestAFileGoesOnTheBackInOneCommand(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage

	env, r := e.runJSON("", "cards", "notes", "attachments", "create", card.ID, localFile(t, "research.md", "# Research\n"))
	expectCode(t, r, ExitOK)
	file := decode(t, env.Data)
	if file["filename"] != "research.md" || file["version"] != float64(1) || file["cardId"] != card.ID {
		t.Fatalf("data %s", env.Data)
	}

	sent := requestLines(e.server.Requests())
	want := []string{
		"GET /api/v1/cards/" + card.ID,
		"GET /api/v1/cards/" + card.ID + "/attachments",
		"POST /api/v1/projects/" + e.fixture.Project.ID + "/uploads",
		"POST /api/v1/cards/" + card.ID + "/attachments",
	}
	if strings.Join(sent, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests\n%s\nwant\n%s", strings.Join(sent, "\n"), strings.Join(want, "\n"))
	}
	stored := e.server.Uploads[len(e.server.Uploads)-1]
	if body := e.lastWriteBody(); len(body) != 1 || body["upload"] != stored.ID {
		t.Fatalf("body %v, stored %s", body, stored.ID)
	}
	if !strings.Contains(env.Notice, "name the attachment in the notes") {
		t.Errorf("notice %q does not ask for the line in the notes", env.Notice)
	}
}

// The same name again is the next version of the same attachment, which the
// summary says.
func TestTheSameNameAgainIsTheNextVersion(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID

	first, _ := e.runJSON("", "cards", "notes", "attachments", "create", card, localFile(t, "plan.md", "one"))
	second, r := e.runJSON("", "cards", "notes", "attachments", "create", card, localFile(t, "plan.md", "two"))
	expectCode(t, r, ExitOK)
	if decode(t, first.Data)["id"] != decode(t, second.Data)["id"] || decode(t, second.Data)["version"] != float64(2) {
		t.Fatalf("first %s, second %s", first.Data, second.Data)
	}
	if !strings.Contains(second.Summary, "version 2") {
		t.Errorf("summary %q", second.Summary)
	}

	shown := e.run("", "cards", "notes", "attachments", "show", card, "plan.md", "--styled")
	expectCode(t, shown, ExitOK)
	if shown.stdout != "two" {
		t.Errorf("stdout %q, want the newest version", shown.stdout)
	}
	list, _ := e.runJSON("", "cards", "notes", "attachments", "list", card)
	if len(listOfObjects(t, list.Data)) != 1 {
		t.Errorf("list %s", list.Data)
	}
}

// Where Minka would refuse only after the transfer, nothing travels: a card
// in the archive, a client who sees the front and not the back, a card that
// is not there.
func TestNothingTravelsWhereTheBackTakesNoFile(t *testing.T) {
	for name, setup := range map[string]struct {
		prepare func(e *testEnv) string
		code    int
		hint    string
	}{
		"archived card": {func(e *testEnv) string {
			expectCode(e.t, e.run("", "cards", "archive", e.fixture.CardInTriage.ID, "--json"), ExitOK)
			return e.fixture.CardInTriage.ID
		}, ExitValidation, "cards unarchive"},
		"client": {func(e *testEnv) string {
			asClient(e)
			return e.fixture.CardInTriage.ID
		}, ExitNotFound, "the team's alone"},
		"no card": {func(e *testEnv) string { return "0192f3a4-5b6c-4d7e-8f90-000000000000" }, ExitNotFound, "the team's alone"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			card := setup.prepare(e)
			writesBefore := len(e.server.WritingRequests())

			env, r := e.runJSON("", "cards", "notes", "attachments", "create", card, localFile(t, "research.md", "# Research\n"))
			expectCode(t, r, setup.code)
			if !strings.Contains(env.Hint, setup.hint) {
				t.Errorf("hint %q does not say %q", env.Hint, setup.hint)
			}
			if writes := e.server.WritingRequests()[writesBefore:]; len(writes) != 0 {
				t.Errorf("sent %v", requestLines(writes))
			}
		})
	}
}

// A second step refused after the first one stored the file hands the stored
// file over, and says how to try again without sending it anew.
func TestARefusedSecondStepHandsTheStoredFileOver(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	e.server.AfterStoring = func(*fakeapi.Upload) {
		// The card is put away between the two requests.
		for _, column := range e.fixture.Board.Columns {
			for i, one := range column.Cards {
				if one == card {
					column.Cards = append(column.Cards[:i], column.Cards[i+1:]...)
					e.server.Archive = append(e.server.Archive, &fakeapi.ArchivedCard{Card: card, Column: column})
					return
				}
			}
		}
	}

	env, r := e.runJSON("", "cards", "notes", "attachments", "create", card.ID, localFile(t, "research.md", "# Research\n"))
	expectCode(t, r, ExitValidation)
	stored := e.server.Uploads[len(e.server.Uploads)-1]
	if env.Meta["apiCode"] != apiCodeCardArchived {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	if decode(t, env.Data)["id"] != stored.ID || !strings.Contains(env.Hint, "--blob "+stored.ID) {
		t.Fatalf("data %s, hint %q", env.Data, env.Hint)
	}
}

func TestAStoredFileIsPutOnTheBackByItsID(t *testing.T) {
	e := newTestEnv(t)
	e.server.Uploads = append(e.server.Uploads, &fakeapi.Upload{ID: storedBlobID, Circle: e.fixture.Project.ID, Filename: "notes.txt",
		ContentType: "text/plain", Content: []byte("Four hundred."), Size: 13, UploaderID: e.fixture.Bruno.ID})

	env, r := e.runJSON("", "cards", "notes", "attachments", "create", e.fixture.CardInTriage.ID, "--blob", storedBlobID)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["filename"] != "notes.txt" {
		t.Fatalf("data %s", env.Data)
	}
	if writes := e.server.WritingRequests(); len(writes) != 1 {
		t.Fatalf("sent %v", requestLines(writes))
	}

	// Held now, it goes into no second place, and nothing else is to blame.
	env, r = e.runJSON("", "cards", "notes", "attachments", "create", e.fixture.CardInDoing.ID, "--blob", storedBlobID)
	expectRefusal(t, env, r, apiCodeUploadNotAttachable, "Give the file itself")
	if env.Data != nil && string(env.Data) != "null" {
		t.Errorf("data %s for a file this run did not store", env.Data)
	}
}

func TestAttachmentsCreateTakesAFileOrABlob(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID
	path := localFile(t, "a.md", "a")
	for _, args := range [][]string{
		{card},
		{card, path, "--blob", storedBlobID},
		{card, "--blob", storedBlobID, "--name", "b.md"},
		{card, "--blob", "nonsense"},
		{card, path, "another"},
	} {
		_, r := e.runJSON("", append([]string{"cards", "notes", "attachments", "create"}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
}

func TestTheAttachmentsAreListedByName(t *testing.T) {
	e := newTestEnv(t)
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a1", "0192f3a4-5b6c-4d7e-8f90-0000000000b1", "zelt.md", "text/markdown", []byte("z"))
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a2", "0192f3a4-5b6c-4d7e-8f90-0000000000b2", "Bühne.png", "image/png", onePixelPNG())
	withAnAttachment(e)

	env, r := e.runJSON("", "cards", "notes", "attachments", "list", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitOK)
	var names []string
	for _, file := range listOfObjects(t, env.Data) {
		names = append(names, file["filename"].(string))
	}
	if strings.Join(names, ",") != "Bühne.png,research.md,zelt.md" {
		t.Fatalf("order %v", names)
	}

	text := e.run("", "cards", "notes", "attachments", "list", e.fixture.CardInTriage.ID, "--styled")
	for _, want := range []string{"research.md", attachmentID, e.fixture.Bruno.Name} {
		if !strings.Contains(text.stdout, want) {
			t.Errorf("text lacks %q:\n%s", want, text.stdout)
		}
	}

	empty := e.run("", "cards", "notes", "attachments", "list", e.fixture.CardInDoing.ID, "--styled")
	if strings.TrimSpace(empty.stdout) != "No files on the back of the card." {
		t.Errorf("empty list:\n%s", empty.stdout)
	}
}

// The text is printed byte for byte and alone on its stream, whichever way
// the attachment is named.
func TestAnAttachmentIsPrintedAsText(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	card := e.fixture.CardInTriage.ID

	for _, named := range []string{"research.md", "RESEARCH.MD", "1", attachmentID, strings.ToUpper(attachmentID)} {
		r := e.run("", "cards", "notes", "attachments", "show", card, named, "--styled")
		expectCode(t, r, ExitOK)
		if r.stdout != "# Research\n\n- tents hold 200\n" {
			t.Errorf("%s: stdout %q", named, r.stdout)
		}
		if !strings.Contains(r.stderr, "version 1") || !strings.Contains(r.stderr, e.fixture.Bruno.Name) {
			t.Errorf("%s: stderr %q", named, r.stderr)
		}
	}

	env, r := e.runJSON("", "cards", "notes", "attachments", "show", card, "research.md")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["text"] != "# Research\n\n- tents hold 200\n" {
		t.Errorf("text %v", data["text"])
	}
	if file, _ := data["file"].(map[string]any); file["id"] != attachmentID {
		t.Errorf("file %v", data["file"])
	}
}

func TestAnAttachmentThatIsNotThereIsNamedAsSuch(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	card := e.fixture.CardInTriage.ID

	for named, code := range map[string]int{"plan.md": ExitUsage, "2": ExitUsage, "0192f3a4-5b6c-4d7e-8f90-000000000000": ExitNotFound} {
		env, r := e.runJSON("", "cards", "notes", "attachments", "show", card, named)
		expectCode(t, r, code)
		if code == ExitUsage && !strings.Contains(env.Hint, "research.md") {
			t.Errorf("%s: hint %q does not list what is there", named, env.Hint)
		}
		if code == ExitNotFound && !strings.Contains(env.Hint, "attachments list") {
			t.Errorf("%s: hint %q", named, env.Hint)
		}
	}
}

// Only a text is printed: a picture is refused before its bytes are fetched,
// and bytes that are no text when they arrive.
func TestOnlyATextIsPrinted(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a2", "0192f3a4-5b6c-4d7e-8f90-0000000000b2", "stage.png", "image/png", onePixelPNG())
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a3", "0192f3a4-5b6c-4d7e-8f90-0000000000b3", "data.bin", "application/octet-stream", []byte("ab\x00cd"))
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a4", "0192f3a4-5b6c-4d7e-8f90-0000000000b4", "latin.txt", "text/plain", []byte("Gr\xfc\xdfe"))

	before := len(e.server.Requests())
	r := e.run("", "cards", "notes", "attachments", "show", card, "stage.png")
	expectCode(t, r, ExitUsage)
	for _, request := range e.server.Requests()[before:] {
		if strings.HasSuffix(request.Path, "/content") {
			t.Errorf("the bytes of a picture were fetched: %s", request.Path)
		}
	}
	for _, name := range []string{"stage.png", "data.bin", "latin.txt"} {
		env, r := e.runJSON("", "cards", "notes", "attachments", "show", card, name)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Hint, "attachments download") {
			t.Errorf("%s: hint %q", name, env.Hint)
		}
		if r := e.run("", "cards", "notes", "attachments", "show", card, name, "--styled"); r.stdout != "" {
			t.Errorf("%s: printed %q", name, r.stdout)
		}
	}
}

func TestAShortAnswerPrintsNothing(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	e.server.SendsAFileShort = true

	r := e.run("", "cards", "notes", "attachments", "show", e.fixture.CardInTriage.ID, "research.md", "--styled")
	expectCode(t, r, ExitAPI)
	if r.stdout != "" {
		t.Errorf("printed %q", r.stdout)
	}
}

func TestAnAttachmentIsFetchedToDisk(t *testing.T) {
	e := newTestEnv(t)
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a2", "0192f3a4-5b6c-4d7e-8f90-0000000000b2", "stage.png", "image/png", onePixelPNG())
	card := e.fixture.CardInTriage.ID
	target := filepath.Join(t.TempDir(), "plan.png")

	env, r := e.runJSON("", "cards", "notes", "attachments", "download", card, "stage.png", "-o", target)
	expectCode(t, r, ExitOK)
	if written, err := os.ReadFile(target); err != nil || string(written) != string(onePixelPNG()) {
		t.Fatalf("written %v, %v", written, err)
	}
	expectMode(t, target, 0o600)
	if decode(t, env.Data)["path"] != target {
		t.Errorf("data %s", env.Data)
	}

	again := e.run("", "cards", "notes", "attachments", "download", card, "stage.png", "-o", target)
	expectCode(t, again, ExitUsage)

	r = e.run("", "cards", "notes", "attachments", "download", card, "stage.png", "-o", "-")
	expectCode(t, r, ExitOK)
	if r.stdout != string(onePixelPNG()) {
		t.Errorf("stdout carries more than the bytes")
	}
}

func TestDeletingTakesTheFileWithItsUploads(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	card := e.fixture.CardInTriage.ID

	env, r := e.runJSON("", "cards", "notes", "attachments", "delete", card, "research.md")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["id"] != attachmentID {
		t.Errorf("data %s", env.Data)
	}
	writes := e.server.WritingRequests()
	if len(writes) != 1 || writes[0].Method != http.MethodDelete || writes[0].Path != "/api/v1/cards/"+card+"/attachments/"+attachmentID {
		t.Fatalf("requests %v", requestLines(writes))
	}
	if len(e.fixture.CardInTriage.BackFiles) != 0 {
		t.Fatal("the file is still on the back")
	}
	_, r = e.runJSON("", "uploads", "download", attachmentBlobID, "-o", filepath.Join(t.TempDir(), "x"))
	expectCode(t, r, ExitNotFound)
}

func TestTheBackOfAnArchivedCardIsReadNotWritten(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	card := e.fixture.CardInTriage.ID
	expectCode(t, e.run("", "cards", "archive", card, "--json"), ExitOK)

	expectCode(t, e.run("", "cards", "notes", "attachments", "show", card, "research.md"), ExitOK)
	env, r := e.runJSON("", "cards", "notes", "attachments", "delete", card, "research.md")
	expectRefusal(t, env, r, apiCodeCardArchived, "cards unarchive")
}

// The whole back is the team's: a client who sees the front of a card gets
// the answer of a card it cannot see, and is told why.
func TestTheBackIsTheTeamsAlone(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	asClient(e)
	card := e.fixture.CardInTriage.ID
	expectCode(t, e.run("", "cards", "show", card, "--json"), ExitOK)

	for _, args := range [][]string{
		{"cards", "notes", "show", card},
		{"cards", "notes", "set", card, "Mine", "--force"},
		{"cards", "notes", "attachments", "list", card},
		{"cards", "notes", "attachments", "show", card, "research.md"},
		{"cards", "notes", "attachments", "show", card, attachmentID},
		{"cards", "notes", "attachments", "download", card, attachmentID, "-o", filepath.Join(t.TempDir(), "x")},
		{"cards", "notes", "attachments", "delete", card, attachmentID},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Hint, "the team's alone") {
			t.Errorf("%v: hint %q", args[2:4], env.Hint)
		}
	}
	if len(e.fixture.CardInTriage.BackFiles) != 1 || e.fixture.CardInTriage.Notes != "" {
		t.Fatal("a client wrote on the back")
	}
}

// The notes say where the files are, and the group's help says how to keep
// the back.
func TestTheNotesPointToTheAttachmentsAndTheHelpSaysTheRule(t *testing.T) {
	e := newTestEnv(t)
	env, _ := e.runJSON("", "cards", "notes", "show", e.fixture.CardInTriage.ID)
	found := false
	for _, crumb := range env.Breadcrumbs {
		found = found || strings.HasPrefix(crumb.Cmd, "minka cards notes attachments list ")
	}
	if !found {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}

	help := e.run("", "cards", "notes", "--help")
	for _, want := range []string{"language of the card's front", "not a chronicle", "40 to 60 lines", "Status · Plan", "one line of what", "team's alone"} {
		if !strings.Contains(strings.Join(strings.Fields(help.stdout), " "), want) {
			t.Errorf("the help of cards notes does not say %q", want)
		}
	}
}

func TestTheFakeKeepsAFileOnTheBackOutOfEveryOtherPlace(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	env, r := e.runJSON("", "docs", "uploads", "create", "--blob", attachmentBlobID, "--tool", e.fixture.Files.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeFileInUse {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

func contentRequests(e *testEnv, before int) []string {
	var fetched []string
	for _, request := range e.server.Requests()[before:] {
		if strings.HasSuffix(request.Path, "/content") {
			fetched = append(fetched, request.Path)
		}
	}
	return fetched
}

// Piped, the CLI answers in the envelope, so an example that sends the text
// into a file or a pager says --styled.
func TestTheExamplesOfShowPrintTheText(t *testing.T) {
	e := newTestEnv(t)
	help := e.run("", "cards", "notes", "attachments", "show", "--help")
	for _, line := range strings.Split(help.stdout, "\n") {
		if strings.Contains(line, "minka cards notes attachments show") && (strings.Contains(line, " > ") || strings.Contains(line, " | ")) && !strings.Contains(line, "--styled") {
			t.Errorf("example %q writes the envelope", line)
		}
	}
}

// Minka's names are case-sensitive: the name as written wins, and case is
// ignored only where that leaves one file.
func TestTheNameAsWrittenWinsOverOneInOtherCase(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a1", "0192f3a4-5b6c-4d7e-8f90-0000000000b1", "plan.md", "text/markdown", []byte("small"))
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a2", "0192f3a4-5b6c-4d7e-8f90-0000000000b2", "Plan.md", "text/markdown", []byte("Capital"))

	for name, want := range map[string]string{"plan.md": "small", "Plan.md": "Capital"} {
		r := e.run("", "cards", "notes", "attachments", "show", card, name, "--styled")
		expectCode(t, r, ExitOK)
		if r.stdout != want {
			t.Errorf("%s: %q", name, r.stdout)
		}
	}
	_, r := e.runJSON("", "cards", "notes", "attachments", "show", card, "PLAN.MD")
	expectCode(t, r, ExitAmbiguous)
}

// A terminal acts on control characters, so a text printed as it is carries
// none but tab and the line breaks; the envelope writes them out as escapes.
func TestControlCharactersAreNotPrintedToATerminal(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a1", "0192f3a4-5b6c-4d7e-8f90-0000000000b1", "colour.txt", "text/plain", []byte("\x1b[31mred\x07\r\n\tok\n"))
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a2", "0192f3a4-5b6c-4d7e-8f90-0000000000b2", "windows.txt", "text/plain", []byte("one\r\n\ttwo\r\n"))

	r := e.run("", "cards", "notes", "attachments", "show", card, "colour.txt", "--styled")
	expectCode(t, r, ExitUsage)
	if r.stdout != "" || !strings.Contains(r.stderr, "attachments download") {
		t.Errorf("stdout %q, stderr %q", r.stdout, r.stderr)
	}
	if r := e.run("", "cards", "notes", "attachments", "show", card, "windows.txt", "--styled"); r.stdout != "one\r\n\ttwo\r\n" {
		t.Errorf("tab and line breaks: %q", r.stdout)
	}
	env, r := e.runJSON("", "cards", "notes", "attachments", "show", card, "colour.txt")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["text"] != "\x1b[31mred\x07\r\n\tok\n" {
		t.Errorf("data %s", env.Data)
	}
}

// A file that is no text is refused on its first bytes: one that is short as
// well is refused as no text, not as short, because the rest was never read.
func TestAFileThatIsNoTextIsRefusedOnItsFirstBytes(t *testing.T) {
	e := newTestEnv(t)
	content := append([]byte("ab\x00"), []byte(strings.Repeat("x", 3*textProbeBytes))...)
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a1", "0192f3a4-5b6c-4d7e-8f90-0000000000b1", "data.bin", "application/octet-stream", content)
	e.server.SendsAFileShort = true

	env, r := e.runJSON("", "cards", "notes", "attachments", "show", e.fixture.CardInTriage.ID, "data.bin")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "no text") {
		t.Errorf("error %q", env.Error)
	}
}

// A text longer than the bytes looked at first comes whole, with a character
// cut off where they end.
func TestALongTextComesWhole(t *testing.T) {
	e := newTestEnv(t)
	text := strings.Repeat("a", textProbeBytes-1) + "ß" + strings.Repeat("ü", textProbeBytes)
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a1", "0192f3a4-5b6c-4d7e-8f90-0000000000b1", "long.md", "text/markdown", []byte(text))

	r := e.run("", "cards", "notes", "attachments", "show", e.fixture.CardInTriage.ID, "long.md", "--styled")
	expectCode(t, r, ExitOK)
	if r.stdout != text {
		t.Errorf("%d bytes printed, want %d", len(r.stdout), len(text))
	}
}

func TestAPictureNamedInCapitalsIsRefusedBeforeItsBytes(t *testing.T) {
	e := newTestEnv(t)
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a1", "0192f3a4-5b6c-4d7e-8f90-0000000000b1", "shot", "IMAGE/PNG", []byte("not looked at"))
	before := len(e.server.Requests())

	_, r := e.runJSON("", "cards", "notes", "attachments", "show", e.fixture.CardInTriage.ID, "shot")
	expectCode(t, r, ExitUsage)
	if fetched := contentRequests(e, before); len(fetched) != 0 {
		t.Errorf("fetched %v", fetched)
	}
}

// A client meets the back before the archive, as Minka asks.
func TestAClientIsToldOfTheBackBeforeTheArchive(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID
	expectCode(t, e.run("", "cards", "archive", card, "--json"), ExitOK)
	asClient(e)

	env, r := e.runJSON("", "cards", "notes", "attachments", "create", card, localFile(t, "research.md", "# Research\n"))
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "the team's alone") {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestATargetThatIsThereCostsNoTransfer(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	target := localFile(t, "research.md", "mine")
	before := len(e.server.Requests())

	_, r := e.runJSON("", "cards", "notes", "attachments", "download", e.fixture.CardInTriage.ID, "research.md", "-o", target)
	expectCode(t, r, ExitUsage)
	if fetched := contentRequests(e, before); len(fetched) != 0 {
		t.Errorf("fetched %v", fetched)
	}
	if written, _ := os.ReadFile(target); string(written) != "mine" {
		t.Errorf("the file there became %q", written)
	}
}

func TestANameThatIsNoNameHereIsNotFetched(t *testing.T) {
	e := newTestEnv(t)
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a1", "0192f3a4-5b6c-4d7e-8f90-0000000000b1", "../plan.md", "text/markdown", []byte("x"))
	before := len(e.server.Requests())

	env, r := e.runJSON("", "cards", "notes", "attachments", "download", e.fixture.CardInTriage.ID, "0192f3a4-5b6c-4d7e-8f90-0000000000a1")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "-o <path>") {
		t.Errorf("hint %q", env.Hint)
	}
	if fetched := contentRequests(e, before); len(fetched) != 0 {
		t.Errorf("fetched %v", fetched)
	}
}

func TestAShortDownloadLeavesNothingBehind(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	e.server.SendsAFileShort = true
	target := filepath.Join(t.TempDir(), "research.md")

	env, r := e.runJSON("", "cards", "notes", "attachments", "download", e.fixture.CardInTriage.ID, "research.md", "-o", target)
	expectCode(t, r, ExitAPI)
	if !strings.Contains(env.Error, "where the attachment says") {
		t.Errorf("error %q", env.Error)
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("a stump of the file was left")
	}
}

func TestTheBytesOnStandardOutputLeaveNoRoomForAnEnvelope(t *testing.T) {
	e := newTestEnv(t)
	withAnAttachment(e)
	for _, flag := range []string{"--json", "--quiet"} {
		r := e.run("", "cards", "notes", "attachments", "download", e.fixture.CardInTriage.ID, "research.md", "-o", "-", flag)
		expectCode(t, r, ExitUsage)
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

// An upload stored in this run and taken by something before the second step
// is no better for being named again: the hint says to run the command again.
func TestAnUploadTakenBetweenTheStepsIsStoredAnew(t *testing.T) {
	e := newTestEnv(t)
	e.server.AfterStoring = func(upload *fakeapi.Upload) { upload.StandsInAText = true }

	env, r := e.runJSON("", "cards", "notes", "attachments", "create", e.fixture.CardInTriage.ID, localFile(t, "research.md", "# Research\n"))
	expectRefusal(t, env, r, apiCodeUploadNotAttachable, "Running the command again stores the file anew")
	stored := e.server.Uploads[len(e.server.Uploads)-1]
	if decode(t, env.Data)["id"] != stored.ID || strings.Contains(env.Hint, "--blob") {
		t.Errorf("data %s, hint %q", env.Data, env.Hint)
	}
}

func TestAStoreThatIsNotHTTPSIsNotFollowed(t *testing.T) {
	e := newTestEnv(t)
	withAFileOnTheBack(e, "0192f3a4-5b6c-4d7e-8f90-0000000000a1", "0192f3a4-5b6c-4d7e-8f90-0000000000b1", "stage.png", "image/png", onePixelPNG())
	e.server.StoreURL = "http://store.invalid"

	env, r := e.runJSON("", "cards", "notes", "attachments", "download", e.fixture.CardInTriage.ID, "stage.png", "-o", filepath.Join(t.TempDir(), "x.png"))
	if r.code == ExitOK || !strings.Contains(env.Hint, "only an https one is followed") {
		t.Errorf("exit %d, hint %q", r.code, env.Hint)
	}
}
