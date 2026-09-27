package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTheToolsOfAProjectAreListed(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "docs", "tools", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	var tools []struct{ ID, Title, AppURL string }
	if err := json.Unmarshal(env.Data, &tools); err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].ID != e.fixture.Files.ID {
		t.Fatalf("tools %+v", tools)
	}
	if tools[0].AppURL == "" {
		t.Error("a tool without an address")
	}
}

func TestAFolderListsWhatStandsInItAndNothingElse(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "docs", "list", f.Files.ID)
	expectCode(t, r, ExitOK)
	top := decode(t, env.Data)
	if ids := idsOf(t, top, "documents"); len(ids) != 1 || ids[0] != f.Document.ID {
		t.Fatalf("the documents at the top are %v: a draft or one in the archive is among them", ids)
	}
	if ids := idsOf(t, top, "folders"); len(ids) != 1 || ids[0] != f.Folder.ID {
		t.Fatalf("the folders at the top are %v", ids)
	}
	if top["folder"] != nil || top["archived"] != false {
		t.Fatalf("the top of a tool stands in no folder and in no archive: %v, %v", top["folder"], top["archived"])
	}
	if lead := decode(t, rawAt(t, top, "documents", 0))["lead"]; lead != "Eight people on Friday." {
		t.Fatalf("lead %q", lead)
	}

	inFolder, r := e.runJSON("", "docs", "list", f.Files.ID, "--folder", f.Folder.ID)
	expectCode(t, r, ExitOK)
	folder := decode(t, inFolder.Data)
	if ids := idsOf(t, folder, "documents"); len(ids) != 1 || ids[0] != f.DocumentInFolder.ID {
		t.Fatalf("the documents of the folder are %v", ids)
	}
	if ids := idsOf(t, folder, "uploads"); len(ids) != 1 || ids[0] != f.FilesUpload.ID {
		t.Fatalf("the uploads of the folder are %v", ids)
	}
	if ids := idsOf(t, folder, "folders"); len(ids) != 0 {
		t.Fatalf("a folder in the archive is listed: %v", ids)
	}
	if count := decode(t, rawAt(t, top, "folders", 0))["entryCount"]; count != float64(2) {
		t.Fatalf("entryCount %v, want the two entries a reader finds in the folder", count)
	}
}

func TestTheInsideOfAnArchivedFolderIsListedAndSaysSo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "docs", "list", f.Files.ID, "--folder", f.ArchivedFolder.ID)
	expectCode(t, r, ExitOK)
	entries := decode(t, env.Data)
	if entries["archived"] != true {
		t.Fatal("the inside of an archived folder does not say that it is archived")
	}
	if ids := idsOf(t, entries, "documents"); len(ids) != 1 || ids[0] != f.DocumentBelowTheArchivedFolder.ID {
		t.Fatalf("the documents of the archived folder are %v", ids)
	}
	if line := decode(t, rawAt(t, entries, "documents", 0)); line["archived"] != true {
		t.Fatal("a line inside an archived folder does not say that it is archived")
	}
	if !strings.Contains(env.Summary, "archive") {
		t.Fatalf("summary %q does not say that this is the archive", env.Summary)
	}
}

func TestADocumentIsReadWithItsBodyAndComments(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "docs", "show", f.Document.ID)
	expectCode(t, r, ExitOK)
	document := decode(t, env.Data)
	if document["body"] != "<p>Eight people on <strong>Friday</strong>.</p>" {
		t.Fatalf("body %v", document["body"])
	}
	if document["updatedAt"] != document["createdAt"] || document["editedAt"] != nil {
		t.Fatalf("a document nobody wrote over stands at the moment it was made: %v, %v, %v", document["createdAt"], document["updatedAt"], document["editedAt"])
	}
	if comments, _ := document["comments"].([]any); len(comments) != 1 {
		t.Fatalf("comments %v", document["comments"])
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "edit"), "--expect "+document["updatedAt"].(string)) {
		t.Fatalf("the breadcrumbs do not hand over the guard: %+v", env.Breadcrumbs)
	}

	text := e.run("", "docs", "show", f.Document.ID, "--styled")
	expectCode(t, text, ExitOK)
	if !strings.Contains(text.stdout, "Eight people on **Friday**.") {
		t.Fatalf("the body is not printed as Markdown:\n%s", text.stdout)
	}
	if !strings.Contains(text.stdout, "To write over it: --expect ") {
		t.Fatalf("the text output does not say what writes over it:\n%s", text.stdout)
	}
}

func TestADraftAndAForeignDocumentAreAtNoAddress(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "docs", "show", e.fixture.DraftDocument.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "draft") {
		t.Fatalf("hint %q does not name the draft", env.Hint)
	}

	_, foreign := e.runJSON("", "docs", "show", e.fixture.OtherDocument.ID)
	expectCode(t, foreign, ExitNotFound)
}

func TestAnUploadIsReadWithTheIdentifierItsBytesAreFetchedBy(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "docs", "uploads", "show", f.FilesUpload.ID)
	expectCode(t, r, ExitOK)
	upload := decode(t, env.Data)
	file := decode(t, rawOf(t, upload, "file"))
	if file["id"] == upload["id"] {
		t.Fatal("the upload and its file are the same identifier")
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "download"), file["id"].(string)) {
		t.Fatalf("the breadcrumbs do not name the file to download: %+v", env.Breadcrumbs)
	}
	if _, r := e.runJSON("", "docs", "show", f.FilesUpload.ID); r.code != ExitNotFound {
		t.Fatal("an upload answers at the address of a document")
	}
}

// The one refusal Minka does not give: a body without an element it serves is
// kept as the characters it consists of. Nothing says so afterwards, so the
// conversion has to happen in the CLI.
func TestMarkdownReachesTheServerAsMarkup(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "docs", "create", "--tool", e.fixture.Files.ID, "--title", "A page", "--audience", "nobody", "--body", "# Heading\n\n**bold**")
	expectCode(t, r, ExitOK)
	document := decode(t, env.Data)
	body, _ := document["body"].(string)
	if !strings.Contains(body, "<h1>Heading</h1>") || !strings.Contains(body, "<strong>bold</strong>") {
		t.Fatalf("body %q: the Markdown was not converted", body)
	}
	if strings.Contains(body, "# Heading") {
		t.Fatalf("body %q: the Markdown was stored as the characters it consists of", body)
	}
}

func TestABodySentAsHtmlWithoutMarkupSaysWhatBecomesOfIt(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "docs", "create", "--tool", e.fixture.Files.ID, "--title", "A page", "--audience", "nobody",
		"--body", "# Heading", "--body-format", "html")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "characters it consists of") {
		t.Fatalf("notice %q does not say that the text is kept as characters", env.Notice)
	}
	if body, _ := decode(t, env.Data)["body"].(string); !strings.Contains(body, "# Heading") {
		t.Fatalf("body %q", body)
	}
}

func TestADocumentIsWrittenWhereItIsAskedFor(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "docs", "create", "--tool", f.Files.ID, "--title", "What the tent costs again",
		"--folder", f.Folder.ID, "--audience", "chosen", "--notify", f.Anke.ID)
	expectCode(t, r, ExitOK)
	document := decode(t, env.Data)
	if folder := decode(t, rawOf(t, document, "folder")); folder["id"] != f.Folder.ID {
		t.Fatalf("folder %v", folder)
	}
	if document["body"] != "" {
		t.Fatalf("a document written without a body has one: %v", document["body"])
	}
	body := e.lastWriteBody()
	if body["audience"] != "chosen" {
		t.Fatalf("audience %v", body["audience"])
	}
	if notify, _ := body["notifyIds"].([]any); len(notify) != 1 || notify[0] != f.Anke.ID {
		t.Fatalf("notifyIds %v", body["notifyIds"])
	}
}

func TestSayingWhomADocumentReachesIsRequiredAndHoldsTogether(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, args := range map[string][]string{
		"no audience":              {"docs", "create", "--tool", f.Files.ID, "--title", "A page"},
		"an audience that is none": {"docs", "create", "--tool", f.Files.ID, "--title", "A page", "--audience", "some"},
		"chosen without anybody":   {"docs", "create", "--tool", f.Files.ID, "--title", "A page", "--audience", "chosen"},
		"notify without chosen":    {"docs", "create", "--tool", f.Files.ID, "--title", "A page", "--audience", "everybody", "--notify", f.Anke.ID},
		"notify by name":           {"docs", "create", "--tool", f.Files.ID, "--title", "A page", "--audience", "chosen", "--notify", "Anke"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			_, r := e.runJSON("", args...)
			expectCode(t, r, ExitUsage)
			if len(e.server.WritingRequests()) != 0 {
				t.Fatal("the command wrote all the same")
			}
		})
	}
}

func TestADocumentIsWrittenOverOnlyWhereItStandsAsItWasRead(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	read, _ := e.runJSON("", "docs", "show", f.Document.ID)
	updatedAt := decode(t, read.Data)["updatedAt"].(string)

	written, r := e.runJSON("", "docs", "edit", f.Document.ID, "--title", "How the stage goes up quickly", "--expect", updatedAt)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["expectedUpdatedAt"] != updatedAt {
		t.Fatalf("expectedUpdatedAt %v, want the moment as it was read", body["expectedUpdatedAt"])
	}
	if !strings.Contains(written.Notice, "whole second") {
		t.Fatalf("notice %q does not say how far the guard reaches", written.Notice)
	}

	env, refused := e.runJSON("", "docs", "edit", f.Document.ID, "--body", "Later still", "--expect", updatedAt)
	expectCode(t, refused, ExitValidation)
	if env.Meta["apiCode"] != apiCodeDocumentChanged {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	if title := decode(t, env.Data)["title"]; title != "How the stage goes up quickly" {
		t.Fatalf("the refusal does not hand over the document as it stands: %v", title)
	}
	if !strings.Contains(env.Hint, "--expect "+decode(t, env.Data)["updatedAt"].(string)) {
		t.Fatalf("hint %q does not say what to send next", env.Hint)
	}
}

func TestAWriteOverADocumentSaysWhichOneItMayReplace(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, one := range map[string]struct {
		args []string
		says string
	}{
		"neither":           {[]string{"docs", "edit", f.Document.ID, "--title", "New"}, "--expect or --force"},
		"both":              {[]string{"docs", "edit", f.Document.ID, "--title", "New", "--force", "--expect", "2026-09-26T14:03:12.000000Z"}, "exclude each other"},
		"none":              {[]string{"docs", "edit", f.Document.ID, "--title", "New", "--expect", "none"}, "notes of a card"},
		"no moment":         {[]string{"docs", "edit", f.Document.ID, "--title", "New", "--expect", "yesterday"}, "updatedAt of the document"},
		"nothing to change": {[]string{"docs", "edit", f.Document.ID, "--force"}, "nothing to change"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON("", one.args...)
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Error, one.says) {
				t.Fatalf("error %q does not say %q", env.Error, one.says)
			}
			if len(e.server.WritingRequests()) != 0 {
				t.Fatal("the command wrote all the same")
			}
		})
	}
}

// The field a write without an expectation must not carry. Null means "no
// notes yet" on the back of a card and "no expectation" here, so a guard
// that sent it would say the opposite of what it means somewhere.
func TestAWriteWithoutAnExpectationSendsNoExpectationAtAll(t *testing.T) {
	e := newTestEnv(t)

	expectCode(t, e.run("", "docs", "edit", e.fixture.Document.ID, "--title", "Renamed", "--force", "--json"), ExitOK)
	body := e.lastWriteBody()
	if _, sent := body["expectedUpdatedAt"]; sent {
		t.Fatalf("--force sends an expectation: %v", body)
	}
}

func TestATitleIsReplacedAndNeverEmptied(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "docs", "edit", f.Document.ID, "--title", "", "--force")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "documentTitleMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	emptied, r := e.runJSON("", "docs", "edit", f.Document.ID, "--body", "", "--force")
	expectCode(t, r, ExitOK)
	if body := decode(t, emptied.Data)["body"]; body != "" {
		t.Fatalf(`--body "" left %v`, body)
	}
}

func TestAnArchivedEntryIsReadAndTakesNoWrite(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.FilesUpload.Archived = true

	shown, r := e.runJSON("", "docs", "show", f.ArchivedDocument.ID)
	expectCode(t, r, ExitOK)
	if decode(t, shown.Data)["archived"] != true {
		t.Fatal("an archived document does not say that it is archived")
	}

	for name, one := range map[string]struct {
		args []string
		code string
	}{
		"the document itself":       {[]string{"docs", "edit", f.ArchivedDocument.ID, "--title", "New", "--force"}, apiCodeDocumentArchived},
		"a folder above it":         {[]string{"docs", "edit", f.DocumentBelowTheArchivedFolder.ID, "--title", "New", "--force"}, apiCodeDocumentArchived},
		"a comment on it":           {[]string{"docs", "comments", "create", "--document", f.ArchivedDocument.ID, "A word"}, apiCodeDocumentArchived},
		"a comment on an upload":    {[]string{"docs", "comments", "create", "--upload", f.FilesUpload.ID, "A word"}, apiCodeUploadArchived},
		"a reaction on its comment": {[]string{"docs", "comments", "react", "--upload", f.FilesUpload.ID, f.FilesUpload.Comments[0].ID, "👍"}, apiCodeUploadArchived},
	} {
		t.Run(name, func(t *testing.T) {
			env, r := e.runJSON("", one.args...)
			expectCode(t, r, ExitValidation)
			if env.Meta["apiCode"] != one.code {
				t.Fatalf("apiCode %v, want %s", env.Meta["apiCode"], one.code)
			}
			if !strings.Contains(env.Hint, "archive") {
				t.Fatalf("hint %q", env.Hint)
			}
		})
	}
}

func TestAFolderThatTakesNothingNewIsNamedAsSuch(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, one := range map[string]struct {
		folder string
		code   string
	}{
		"in the archive": {f.ArchivedFolder.ID, apiCodeFolderArchived},
		"of no tool":     {"0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", apiCodeFolderUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			env, r := e.runJSON("", "docs", "create", "--tool", f.Files.ID, "--title", "A page", "--audience", "nobody", "--folder", one.folder)
			expectCode(t, r, ExitValidation)
			if env.Meta["apiCode"] != one.code {
				t.Fatalf("apiCode %v, want %s", env.Meta["apiCode"], one.code)
			}
		})
	}
}

func TestACommentIsWrittenChangedReactedToAndTakenBackUnderBothKindsOfEntry(t *testing.T) {
	f := newTestEnv(t).fixture
	for name, flag := range map[string][]string{
		"a document": {"--document", f.Document.ID},
		"an upload":  {"--upload", f.FilesUpload.ID},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			where := []string{flag[0], flag[1]}
			if flag[0] == "--document" {
				where[1] = e.fixture.Document.ID
			} else {
				where[1] = e.fixture.FilesUpload.ID
			}

			written, r := e.runJSON("", append(append([]string{"docs", "comments", "create"}, where...), "**Yes**")...)
			expectCode(t, r, ExitOK)
			commentID := decode(t, written.Data)["id"].(string)
			if body := decode(t, written.Data)["body"]; body != "<p><strong>Yes</strong></p>" {
				t.Fatalf("body %v: the Markdown was not converted", body)
			}

			changed, r := e.runJSON("", append(append([]string{"docs", "comments", "edit"}, where...), commentID, "No")...)
			expectCode(t, r, ExitOK)
			if body := decode(t, changed.Data)["body"]; body != "<p>No</p>" {
				t.Fatalf("body %v", body)
			}

			expectCode(t, e.run("", append(append([]string{"docs", "comments", "react"}, where...), commentID, "👍", "--json")...), ExitOK)
			expectCode(t, e.run("", append(append([]string{"docs", "comments", "unreact"}, where...), commentID, "👍", "--json")...), ExitOK)
			expectCode(t, e.run("", append(append([]string{"docs", "comments", "delete"}, where...), commentID, "--json")...), ExitOK)
			_, again := e.runJSON("", append(append([]string{"docs", "comments", "delete"}, where...), commentID)...)
			expectCode(t, again, ExitNotFound)
		})
	}
}

func TestACommentSaysWhichKindOfEntryItHangsOn(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, args := range map[string][]string{
		"neither": {"docs", "comments", "create", "A word"},
		"both":    {"docs", "comments", "create", "--document", f.Document.ID, "--upload", f.FilesUpload.ID, "A word"},
		"no ID":   {"docs", "comments", "create", "--document", "the-first-one", "A word"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			_, r := e.runJSON("", args...)
			expectCode(t, r, ExitUsage)
			if len(e.server.WritingRequests()) != 0 {
				t.Fatal("the command wrote all the same")
			}
		})
	}

	// The identifier of one kind is nothing at the address of the other, so
	// the flag decides and the CLI never has to guess.
	_, r := e.runJSON("", "docs", "comments", "create", "--upload", f.Document.ID, "A word")
	expectCode(t, r, ExitNotFound)
}

func TestOnlyTheAuthorChangesAComment(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	byAnke := f.Document.Comments[0].ID

	env, r := e.runJSON("", "docs", "comments", "edit", "--document", f.Document.ID, byAnke, "Mine now")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "author") {
		t.Fatalf("hint %q does not say who may change a comment", env.Hint)
	}
}

func idsOf(t *testing.T, entries map[string]any, key string) []string {
	t.Helper()
	list, ok := entries[key].([]any)
	if !ok {
		t.Fatalf("%q is no list: %v", key, entries[key])
	}
	ids := make([]string, len(list))
	for i, one := range list {
		entry, ok := one.(map[string]any)
		if !ok {
			t.Fatalf("%q holds something that is no entry: %v", key, one)
		}
		ids[i], _ = entry["id"].(string)
	}
	return ids
}

func rawAt(t *testing.T, entries map[string]any, key string, at int) json.RawMessage {
	t.Helper()
	list, _ := entries[key].([]any)
	if at >= len(list) {
		t.Fatalf("%q holds %d entries", key, len(list))
	}
	raw, err := json.Marshal(list[at])
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func rawOf(t *testing.T, value map[string]any, key string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value[key])
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func crumbFor(t *testing.T, crumbs []Breadcrumb, action string) string {
	t.Helper()
	for _, crumb := range crumbs {
		if crumb.Action == action {
			return crumb.Cmd
		}
	}
	t.Fatalf("no breadcrumb %q among %+v", action, crumbs)
	return ""
}
