package cli

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// Where Minka refuses a text whatever is sent, nothing can be lost, so the
// guard over files stands aside and the request goes out: Minka's refusal is
// the answer, with --drop-attachments and without, and never a hint at a flag
// that only leads on to that refusal.
func TestTheGuardOverFilesStandsAsideWhereMinkaRefusesTheWrite(t *testing.T) {
	withMention := "<div>See " + aMention + "</div>"
	ownComment := func(f *fakeapi.Fixture) *fakeapi.Comment {
		return &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e521", AuthorID: f.Bruno.ID, Body: withMention, CreatedAt: time.Now()}
	}

	for name, c := range map[string]struct {
		prepare func(e *testEnv) []string
		exit    int
		apiCode string
	}{
		"a card of the archive": {func(e *testEnv) []string {
			card := e.fixture.CardInTriage
			card.Description = withMention
			e.server.PutInTheArchive(card)
			return []string{"cards", "edit", card.ID, "--description", "Plain"}
		}, ExitValidation, apiCodeCardArchived},
		"a card in an archived project": {func(e *testEnv) []string {
			e.fixture.CardInTriage.Description = withMention
			e.fixture.Project.Archived = true
			return []string{"cards", "edit", e.fixture.CardInTriage.ID, "--description", "Plain"}
		}, ExitValidation, apiCodeProjectArchived},
		"a comment of somebody else on a card": {func(e *testEnv) []string {
			card := e.fixture.CardInTriage
			others := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e522", AuthorID: e.fixture.Anke.ID, Body: withMention, CreatedAt: time.Now()}
			card.Comments = append(card.Comments, others)
			return []string{"comments", "edit", card.ID, others.ID, "Plain"}
		}, ExitNotFound, ""},
		"a comment of somebody else on a card of the archive": {func(e *testEnv) []string {
			card := e.fixture.CardInTriage
			others := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e524", AuthorID: e.fixture.Anke.ID, Body: withMention, CreatedAt: time.Now()}
			card.Comments = append(card.Comments, others)
			e.server.PutInTheArchive(card)
			return []string{"comments", "edit", card.ID, others.ID, "Plain"}
		}, ExitValidation, apiCodeCardArchived},
		"a comment of somebody else on a card in an archived project": {func(e *testEnv) []string {
			card := e.fixture.CardInTriage
			others := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e525", AuthorID: e.fixture.Anke.ID, Body: withMention, CreatedAt: time.Now()}
			card.Comments = append(card.Comments, others)
			e.fixture.Project.Archived = true
			return []string{"comments", "edit", card.ID, others.ID, "Plain"}
		}, ExitValidation, apiCodeProjectArchived},
		"an own comment on a card of the archive": {func(e *testEnv) []string {
			card := e.fixture.CardInTriage
			card.Comments[0].Body = withMention
			e.server.PutInTheArchive(card)
			return []string{"comments", "edit", card.ID, card.Comments[0].ID, "Plain"}
		}, ExitValidation, apiCodeCardArchived},
		"an own comment on a card in an archived project": {func(e *testEnv) []string {
			card := e.fixture.CardInTriage
			card.Comments[0].Body = withMention
			e.fixture.Project.Archived = true
			return []string{"comments", "edit", card.ID, card.Comments[0].ID, "Plain"}
		}, ExitValidation, apiCodeProjectArchived},
		"a document of the archive": {func(e *testEnv) []string {
			e.fixture.ArchivedDocument.Body = withMention
			return []string{"docs", "edit", e.fixture.ArchivedDocument.ID, "--body", "Plain", "--force"}
		}, ExitValidation, apiCodeDocumentArchived},
		"a document below an archived folder": {func(e *testEnv) []string {
			e.fixture.DocumentBelowTheArchivedFolder.Body = withMention
			return []string{"docs", "edit", e.fixture.DocumentBelowTheArchivedFolder.ID, "--body", "Plain", "--force"}
		}, ExitValidation, apiCodeDocumentArchived},
		"a document in an archived project": {func(e *testEnv) []string {
			e.fixture.Document.Body = withMention
			e.fixture.Project.Archived = true
			return []string{"docs", "edit", e.fixture.Document.ID, "--body", "Plain", "--force"}
		}, ExitValidation, apiCodeProjectArchived},
		"an upload of the archive": {func(e *testEnv) []string {
			upload := e.fixture.FilesUpload
			upload.Description, upload.Archived = withMention, true
			return []string{"docs", "uploads", "edit", upload.ID, "--description", "Plain", "--force"}
		}, ExitValidation, apiCodeUploadArchived},
		"an upload below an archived folder": {func(e *testEnv) []string {
			upload := e.fixture.FilesUpload
			upload.Description, upload.FolderID = withMention, e.fixture.ArchivedFolder.ID
			return []string{"docs", "uploads", "edit", upload.ID, "--description", "Plain", "--force"}
		}, ExitValidation, apiCodeUploadArchived},
		"an upload in an archived project": {func(e *testEnv) []string {
			e.fixture.FilesUpload.Description = withMention
			e.fixture.Project.Archived = true
			return []string{"docs", "uploads", "edit", e.fixture.FilesUpload.ID, "--description", "Plain", "--force"}
		}, ExitValidation, apiCodeProjectArchived},
		"a comment of somebody else on a document": {func(e *testEnv) []string {
			document := e.fixture.Document
			document.Comments[0].Body = withMention
			return []string{"docs", "comments", "edit", "--document", document.ID, document.Comments[0].ID, "Plain"}
		}, ExitNotFound, ""},
		"a comment of somebody else below an archived folder": {func(e *testEnv) []string {
			document := e.fixture.DocumentBelowTheArchivedFolder
			others := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e526", AuthorID: e.fixture.Anke.ID, Body: withMention, CreatedAt: time.Now()}
			document.Comments = append(document.Comments, others)
			return []string{"docs", "comments", "edit", "--document", document.ID, others.ID, "Plain"}
		}, ExitValidation, apiCodeDocumentArchived},
		"an own comment on a document of the archive": {func(e *testEnv) []string {
			document := e.fixture.ArchivedDocument
			mine := ownComment(e.fixture)
			document.Comments = append(document.Comments, mine)
			return []string{"docs", "comments", "edit", "--document", document.ID, mine.ID, "Plain"}
		}, ExitValidation, apiCodeDocumentArchived},
		"an own comment on an upload of the archive in a folder in use": {func(e *testEnv) []string {
			upload := e.fixture.FilesUpload
			upload.Archived = true
			upload.Comments[0].Body = withMention
			return []string{"docs", "comments", "edit", "--upload", upload.ID, upload.Comments[0].ID, "Plain"}
		}, ExitValidation, apiCodeUploadArchived},
		"an own comment on an upload in an archived project": {func(e *testEnv) []string {
			upload := e.fixture.FilesUpload
			upload.Comments[0].Body = withMention
			e.fixture.Project.Archived = true
			return []string{"docs", "comments", "edit", "--upload", upload.ID, upload.Comments[0].ID, "Plain"}
		}, ExitValidation, apiCodeProjectArchived},
		"an own answer in an archived project": {func(e *testEnv) []string {
			e.fixture.BrunosAnswer.Body = withMention
			e.fixture.Project.Archived = true
			return []string{"checkins", "answers", "edit", e.fixture.BrunosAnswer.ID, "Plain"}
		}, ExitValidation, apiCodeProjectArchived},
		"a to-do of a list in the archive": {func(e *testEnv) []string {
			e.fixture.ArchivedListTodo.Notes = withMention
			return []string{"todos", "edit", e.fixture.ArchivedListTodo.ID, "--notes", "Plain"}
		}, ExitValidation, apiCodeTodoListArchived},
		"a to-do in an archived project": {func(e *testEnv) []string {
			e.fixture.LooseTodo.Notes = withMention
			e.fixture.Project.Archived = true
			return []string{"todos", "edit", e.fixture.LooseTodo.ID, "--notes", "Plain"}
		}, ExitValidation, apiCodeProjectArchived},
		"a list of the archive": {func(e *testEnv) []string {
			e.fixture.ArchivedList.Description = withMention
			return []string{"todolists", "edit", e.fixture.ArchivedList.ID, "--description", "Plain"}
		}, ExitValidation, apiCodeTodoListArchived},
		"a list in an archived project": {func(e *testEnv) []string {
			e.fixture.TodoList.Description = withMention
			e.fixture.Project.Archived = true
			return []string{"todolists", "edit", e.fixture.TodoList.ID, "--description", "Plain"}
		}, ExitValidation, apiCodeProjectArchived},
		"an own message in a chat of an archived project": {func(e *testEnv) []string {
			chat := e.fixture.Chat
			chat.Messages[0].BodyHTML = withMention
			e.fixture.Project.Archived = true
			return []string{"messages", "edit", chat.ID, chat.Messages[0].ID, "Plain"}
		}, ExitValidation, apiCodeProjectArchived},
		"a message in a chat that is not there": {func(e *testEnv) []string {
			return []string{"messages", "edit", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5ff", e.fixture.Chat.Messages[0].ID, "Plain"}
		}, ExitNotFound, ""},
		"a message of somebody else": {func(e *testEnv) []string {
			chat := e.fixture.Chat
			chat.Messages[1].BodyHTML = withMention
			return []string{"messages", "edit", chat.ID, chat.Messages[1].ID, "Plain"}
		}, ExitNotFound, ""},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			args := c.prepare(e)
			for _, flags := range [][]string{nil, {"--drop-attachments"}} {
				env, r := e.runJSON("", slices.Concat(args, flags)...)
				expectCode(t, r, c.exit)
				if c.apiCode != "" && env.Meta["apiCode"] != c.apiCode {
					t.Fatalf("%v: apiCode %v, want %s", flags, env.Meta["apiCode"], c.apiCode)
				}
				if strings.Contains(env.Hint, "--drop-attachments") {
					t.Fatalf("%v: hint %q points at a flag Minka refuses anyway", flags, env.Hint)
				}
			}
			if patches := patchesIn(e.server.WritingRequests()); patches != 2 {
				t.Fatalf("%d requests went out, want Minka asked both times", patches)
			}
		})
	}
}

// A Minka that asks the entry alone changes an own comment under an entry that
// lies in the archive only through a folder above it, so the files of that
// comment can be lost and the guard stands there. The fake asks the folders
// as well, so what gets through with --drop-attachments is its refusal.
func TestTheGuardOverFilesStandsBelowAnArchivedFolder(t *testing.T) {
	withMention := "<div>" + aMention + "</div>"
	for name, prepare := range map[string]func(e *testEnv) ([]string, string, *fakeapi.Comment){
		"a document": func(e *testEnv) ([]string, string, *fakeapi.Comment) {
			document := e.fixture.DocumentBelowTheArchivedFolder
			mine := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e523", AuthorID: e.fixture.Bruno.ID, Body: withMention, CreatedAt: time.Now()}
			document.Comments = append(document.Comments, mine)
			return []string{"--document", document.ID, mine.ID}, apiCodeDocumentArchived, mine
		},
		"an upload": func(e *testEnv) ([]string, string, *fakeapi.Comment) {
			upload := e.fixture.FilesUpload
			upload.FolderID = e.fixture.ArchivedFolder.ID
			upload.Comments[0].Body = withMention
			return []string{"--upload", upload.ID, upload.Comments[0].ID}, apiCodeUploadArchived, upload.Comments[0]
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			target, code, mine := prepare(e)
			args := slices.Concat([]string{"docs", "comments", "edit"}, target, []string{"Plain"})

			env, r := e.runJSON("", args...)
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Hint, "--drop-attachments") || len(e.server.WritingRequests()) != 0 {
				t.Fatalf("hint %q, %d writes", env.Hint, len(e.server.WritingRequests()))
			}

			env, r = e.runJSON("", slices.Concat(args, []string{"--drop-attachments"})...)
			expectCode(t, r, ExitValidation)
			if env.Meta["apiCode"] != code || patchesIn(e.server.WritingRequests()) != 1 || mine.Body != withMention {
				t.Fatalf("apiCode %v, %d changes sent, comment %q", env.Meta["apiCode"], patchesIn(e.server.WritingRequests()), mine.Body)
			}
		})
	}
}

// On an entry in use Minka takes the text, so the guard stands: exit 1 with
// the hint and nothing sent, and --drop-attachments writes.
func TestTheGuardOverFilesStandsOnDocumentsInUse(t *testing.T) {
	withMention := "<div>See " + aMention + "</div>"
	for name, prepare := range map[string]func(e *testEnv) ([]string, func() string){
		"a document": func(e *testEnv) ([]string, func() string) {
			document := e.fixture.Document
			document.Body = withMention
			return []string{"docs", "edit", document.ID, "--body", "Plain", "--force"}, func() string { return document.Body }
		},
		"an upload": func(e *testEnv) ([]string, func() string) {
			upload := e.fixture.FilesUpload
			upload.Description = withMention
			return []string{"docs", "uploads", "edit", upload.ID, "--description", "Plain", "--force"}, func() string { return upload.Description }
		},
		"an own comment on a document": func(e *testEnv) ([]string, func() string) {
			document := e.fixture.Document
			mine := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e527", AuthorID: e.fixture.Bruno.ID, Body: withMention, CreatedAt: time.Now()}
			document.Comments = append(document.Comments, mine)
			return []string{"docs", "comments", "edit", "--document", document.ID, mine.ID, "Plain"}, func() string { return mine.Body }
		},
		"an own comment on an upload": func(e *testEnv) ([]string, func() string) {
			upload := e.fixture.FilesUpload
			upload.Comments[0].Body = withMention
			return []string{"docs", "comments", "edit", "--upload", upload.ID, upload.Comments[0].ID, "Plain"}, func() string { return upload.Comments[0].Body }
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			args, stored := prepare(e)
			env, r := e.runJSON("", args...)
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Hint, "--drop-attachments") || len(e.server.WritingRequests()) != 0 {
				t.Fatalf("hint %q, %d writes", env.Hint, len(e.server.WritingRequests()))
			}
			dropped := runOK(t, e, slices.Concat(args, []string{"--drop-attachments"})...)
			if !strings.Contains(dropped.Notice, "dropped") || strings.Contains(stored(), "action-text-attachment") {
				t.Fatalf("notice %q, stored %q", dropped.Notice, stored())
			}
		})
	}
}

// A comment edit that keeps every file and mention costs no read of the
// person of the token and none of the projects.
func TestACommentEditThatKeepsItsFilesReadsNobody(t *testing.T) {
	e := newTestEnv(t)
	kept := "<div>Still " + aMention + "</div>"
	upload := e.fixture.FilesUpload
	upload.Comments[0].Body = "<div>" + aMention + "</div>"
	card := e.fixture.CardInTriage
	card.Comments[0].Body = "<div>" + aMention + "</div>"

	runOK(t, e, "docs", "comments", "edit", "--upload", upload.ID, upload.Comments[0].ID, kept, "--format", "html")
	runOK(t, e, "comments", "edit", card.ID, card.Comments[0].ID, kept, "--format", "html")
	for _, request := range e.server.Requests() {
		if request.Path == "/api/v1/me" || request.Path == "/api/v1/projects" {
			t.Fatalf("read %s although nothing was about to be refused", request.Path)
		}
	}
}

// Minka answers an answer of somebody else as one that is not there before it
// asks whether the project is archived, so exit 2 holds in an archived project
// too, whether this side answers it or Minka does.
func TestSomebodyElsesAnswerIsNotFoundBeforeTheArchivedProject(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Project.Archived = true

	_, r := e.runJSON("", "checkins", "answers", "edit", f.AnkesAnswer.ID, "Mine now")
	expectCode(t, r, ExitNotFound)
	if patchesIn(e.server.WritingRequests()) != 1 {
		t.Fatal("an answer without files was not sent")
	}

	f.AnkesAnswer.Body = "<p>See " + aMention + "</p>"
	_, r = e.runJSON("", "checkins", "answers", "edit", f.AnkesAnswer.ID, "Mine now")
	expectCode(t, r, ExitNotFound)
}

func patchesIn(requests []fakeapi.RecordedRequest) int {
	count := 0
	for _, request := range requests {
		if request.Method == http.MethodPatch {
			count++
		}
	}
	return count
}
