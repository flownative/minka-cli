package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
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
	if ids := idsOf(t, top, "folders"); len(ids) != 2 || ids[0] != f.Folder.ID || ids[1] != f.FolderShownToClients.ID {
		t.Fatalf("the folders at the top are %v: one in the archive is among them, or one is missing", ids)
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

// The CLI holds the audience as a word and a person as an ID, and nothing more:
// which people go with which audience is left to Minka.
func TestSayingWhomADocumentReachesIsRequired(t *testing.T) {
	f := newTestEnv(t).fixture

	for name, args := range map[string][]string{
		"no audience":              {"docs", "create", "--tool", f.Files.ID, "--title", "A page"},
		"an audience that is none": {"docs", "create", "--tool", f.Files.ID, "--title", "A page", "--audience", "some"},
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

// Which people go with which audience is Minka's rule, and the CLI keeps none
// of its own: the request goes out, and the answer says which flag to change.
// Minka reads the request before the write meets the project, so both refusals
// come after an unknown folder and before an archived project.
func TestAnAudienceAndTheNamesUnderItAreRefusedTogetherByMinka(t *testing.T) {
	for name, one := range map[string]struct {
		rest []string
		code string
		says string
	}{
		"chosen naming nobody": {
			[]string{"--audience", "chosen"}, apiCodeNotifyIDsMissing, "Give one --notify per person",
		},
		"everybody naming somebody": {
			[]string{"--audience", "everybody", "--notify", "me"}, apiCodeNotifyIDsWithoutChosenAudience, "Leave the named people out",
		},
		"nobody naming somebody": {
			[]string{"--audience", "nobody", "--notify", "me"}, apiCodeNotifyIDsWithoutChosenAudience, "Leave the named people out",
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON("", newDocument(e, one.rest...)...)
			expectCode(t, r, ExitValidation)
			if env.Meta["apiCode"] != one.code {
				t.Fatalf("apiCode %v, want %s", env.Meta["apiCode"], one.code)
			}
			if !strings.Contains(env.Hint, one.says) {
				t.Errorf("hint %q does not say %q", env.Hint, one.says)
			}
		})
	}

	e := newTestEnv(t)
	env, r := e.runJSON("", newDocument(e, "--audience", "chosen",
		"--folder", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6")...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeFolderUnknown {
		t.Errorf("apiCode %v, want the folder before the people it names", env.Meta["apiCode"])
	}

	e.fixture.Project.Archived = true
	env, r = e.runJSON("", newDocument(e, "--audience", "nobody", "--notify", "me")...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeNotifyIDsWithoutChosenAudience {
		t.Errorf("apiCode %v, want the request before the project", env.Meta["apiCode"])
	}
}

// newDocument is the command line of a document in the tool of the fixture,
// with whatever the case adds to it.
func newDocument(e *testEnv, rest ...string) []string {
	return append([]string{"docs", "create", "--tool", e.fixture.Files.ID, "--title", "A page"}, rest...)
}

// Somebody the document is not shown to is no mistake in the request: the
// person is named rightly and may not be told, and only naming somebody else
// or writing where they are let in gets the document written.
func TestAClientIsToldOnlyAboutADocumentInAFolderShownToThem(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	create := newDocument(e, "--audience", "chosen", "--notify", f.Client.ID)
	before := len(f.Files.Documents)

	// At the top of the tool, where a new document is shown to no client, and
	// in a folder the project does not show them.
	for _, folder := range []string{"", f.Folder.ID} {
		args := create
		if folder != "" {
			args = append(create, "--folder", folder)
		}
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodePersonCannotSeeIt {
			t.Fatalf("apiCode %v for folder %q", env.Meta["apiCode"], folder)
		}
		if !strings.Contains(env.Hint, "refused again") || !strings.Contains(env.Hint, "Name somebody else") {
			t.Errorf("the hint does not say that repeating is no use: %q", env.Hint)
		}
		if env.Retryable {
			t.Error("the refusal reads as one a repeat could get past")
		}
	}
	if len(f.Files.Documents) != before {
		t.Fatalf("%d documents after the refusals, want the %d of before", len(f.Files.Documents), before)
	}

	// The same person, told about a document standing where the project lets
	// its clients in.
	_, r := e.runJSON("", append(create, "--folder", f.FolderShownToClients.ID)...)
	expectCode(t, r, ExitOK)

	// Of one person, membership is answered first: a client of no project of
	// this Minka hears that they do not belong, not that something is hidden
	// from them.
	outside, r := e.runJSON("", newDocument(e, "--audience", "chosen", "--notify", f.ForeignClient.ID)...)
	expectCode(t, r, ExitValidation)
	if outside.Meta["apiCode"] != "personNotAProjectMember" {
		t.Fatalf("apiCode %v, want the membership before the visibility", outside.Meta["apiCode"])
	}

	// Of a list, the first name that is refused decides.
	for want, names := range map[string][]string{
		apiCodePersonCannotSeeIt:  {f.Client.ID, f.Stranger.ID},
		"personNotAProjectMember": {f.Stranger.ID, f.Client.ID},
	} {
		args := newDocument(e, "--audience", "chosen", "--notify", names[0], "--notify", names[1])
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != want {
			t.Errorf("apiCode %v, want %s for the name given first", env.Meta["apiCode"], want)
		}
	}
}

// A folder shown to clients lets them in only where the tool does too.
func TestAClientIsToldNothingInAToolNotShownToThem(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Files.ShownToClients = false

	env, r := e.runJSON("", newDocument(e, "--audience", "chosen", "--notify", f.Client.ID, "--folder", f.FolderShownToClients.ID)...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePersonCannotSeeIt {
		t.Fatalf("apiCode %v, want the tool to keep the client out of a folder shown to them", env.Meta["apiCode"])
	}
}

// Only the topmost folder of a chain says whether the clients are shown what
// stands below it; a folder further down has no say.
func TestTheTopmostFolderDecidesWhatAClientIsToldAbout(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	hiddenBelowShown := &fakeapi.Folder{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e501", Title: "Inside", ParentID: f.FolderShownToClients.ID}
	shownBelowHidden := &fakeapi.Folder{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e502", Title: "Outside", ParentID: f.Folder.ID, ShownToClients: true}
	f.Files.Folders = append(f.Files.Folders, hiddenBelowShown, shownBelowHidden)
	create := newDocument(e, "--audience", "chosen", "--notify", f.Client.ID)

	_, r := e.runJSON("", append(create, "--folder", hiddenBelowShown.ID)...)
	expectCode(t, r, ExitOK)

	env, r := e.runJSON("", append(create, "--folder", shownBelowHidden.ID)...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePersonCannotSeeIt {
		t.Fatalf("apiCode %v, want the hidden folder at the top to decide", env.Meta["apiCode"])
	}
}

// An agent works with what its principal is shown, so the agent of a client is
// told only what a client is.
func TestTheAgentOfAClientIsToldAsAClientIs(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	agent := &fakeapi.Person{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e503", Name: "Claude (Nele)", Email: "claude-nele@agents.invalid", PrincipalID: f.Client.ID}
	e.server.People[agent.ID] = agent
	f.Project.MemberIDs = append(f.Project.MemberIDs, agent.ID)
	create := newDocument(e, "--audience", "chosen", "--notify", agent.ID)

	env, r := e.runJSON("", create...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePersonCannotSeeIt {
		t.Fatalf("apiCode %v, want the agent kept out as its client is", env.Meta["apiCode"])
	}
	_, r = e.runJSON("", append(create, "--folder", f.FolderShownToClients.ID)...)
	expectCode(t, r, ExitOK)
}

// Minka finds somebody named a second time already on the list and answers as
// for somebody it could not put there. The writer is on it from the start and
// passed over, however often they are named.
func TestSomebodyNamedTwiceIsRefusedAsNoMember(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", newDocument(e, "--audience", "chosen", "--notify", f.Anke.ID, "--notify", f.Anke.ID)...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePersonNotAProjectMember {
		t.Fatalf("apiCode %v, want the second naming refused", env.Meta["apiCode"])
	}
	if !strings.Contains(env.Hint, "named twice") {
		t.Errorf("the hint does not name the repetition: %q", env.Hint)
	}

	_, r = e.runJSON("", newDocument(e, "--audience", "chosen", "--notify", "me", "--notify", "me", "--notify", f.Anke.ID)...)
	expectCode(t, r, ExitOK)
}

// Whom a document tells is looked up after it is written, so a project that
// takes no writes says so before anybody is looked for.
func TestSomebodyOutsideTheProjectIsNamedAfterTheProjectItself(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	create := []string{"docs", "create", "--tool", f.Files.ID, "--title", "A page", "--audience", "chosen", "--notify", f.Stranger.ID}

	env, r := e.runJSON("", create...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "personNotAProjectMember" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	f.Project.Archived = true
	archived, r := e.runJSON("", create...)
	expectCode(t, r, ExitValidation)
	if archived.Meta["apiCode"] != apiCodeProjectArchived {
		t.Fatalf("apiCode %v, want the project before the person", archived.Meta["apiCode"])
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
	for _, kind := range []string{"--document", "--upload"} {
		t.Run(kind, func(t *testing.T) {
			e := newTestEnv(t)
			entry := e.fixture.Document.ID
			if kind == "--upload" {
				entry = e.fixture.FilesUpload.ID
			}
			run := func(action string, rest ...string) (envelope, result) {
				return e.runJSON("", append([]string{"docs", "comments", action, kind, entry}, rest...)...)
			}

			written, r := run("create", "**Yes**")
			expectCode(t, r, ExitOK)
			comment := decode(t, written.Data)
			if comment["body"] != "<p><strong>Yes</strong></p>" {
				t.Fatalf("body %v: the Markdown was not converted", comment["body"])
			}
			commentID, _ := comment["id"].(string)

			changed, r := run("edit", commentID, "No")
			expectCode(t, r, ExitOK)
			if body := decode(t, changed.Data)["body"]; body != "<p>No</p>" {
				t.Fatalf("body %v", body)
			}

			_, r = run("react", commentID, "👍")
			expectCode(t, r, ExitOK)
			_, r = run("unreact", commentID, "👍")
			expectCode(t, r, ExitOK)
			_, r = run("delete", commentID)
			expectCode(t, r, ExitOK)
			_, again := run("delete", commentID)
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
