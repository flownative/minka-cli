package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/fakeapi"
	"github.com/flownative/minka-cli/internal/markdown"
)

type testEnv struct {
	t       *testing.T
	server  *fakeapi.Server
	fixture *fakeapi.Fixture
	vars    map[string]string
}

type result struct {
	stdout string
	stderr string
	code   int
}

type envelope struct {
	OK          bool            `json:"ok"`
	Data        json.RawMessage `json:"data"`
	Summary     string          `json:"summary"`
	Notice      string          `json:"notice"`
	Breadcrumbs []Breadcrumb    `json:"breadcrumbs"`
	Error       string          `json:"error"`
	Code        string          `json:"code"`
	Retryable   bool            `json:"retryable"`
	Hint        string          `json:"hint"`
	Meta        map[string]any  `json:"meta"`
}

// newTestEnv starts a fake API and signs the CLI in to it through
// $MINKA_HOST and $MINKA_TOKEN, with a config file in a temporary directory.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	server, fixture := fakeapi.New(t)
	return &testEnv{t: t, server: server, fixture: fixture, vars: map[string]string{
		"MINKA_CONFIG": filepath.Join(t.TempDir(), "minka", "config.json"),
		"MINKA_HOST":   server.URL,
		"MINKA_TOKEN":  fixture.Token,
	}}
}

func (e *testEnv) run(stdin string, args ...string) result {
	e.t.Helper()
	return e.runWith(nil, stdin, args...)
}

// runWith runs the CLI after configure has replaced parts of the app, such as
// the detection of a terminal.
func (e *testEnv) runWith(configure func(*app), stdin string, args ...string) result {
	e.t.Helper()
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), strings.NewReader(stdin), &stdout, &stderr, func(name string) string { return e.vars[name] })
	if configure != nil {
		configure(a)
	}
	code := a.run(args)
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// runJSON runs a command with --json and decodes the envelope.
func (e *testEnv) runJSON(stdin string, args ...string) (envelope, result) {
	e.t.Helper()
	r := e.run(stdin, append(args, "--json")...)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\nstdout: %s\nstderr: %s", err, r.stdout, r.stderr)
	}
	return env, r
}

func (e *testEnv) lastWriteBody() map[string]any {
	e.t.Helper()
	writes := e.server.WritingRequests()
	if len(writes) == 0 {
		e.t.Fatal("no writing request was sent")
	}
	var body map[string]any
	if err := json.Unmarshal(writes[len(writes)-1].Body, &body); err != nil {
		e.t.Fatalf("request body is not JSON: %v", err)
	}
	return body
}

func decode(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("data is not a JSON object: %v: %s", err, raw)
	}
	return value
}

func expectCode(t *testing.T, r result, want int) {
	t.Helper()
	if r.code != want {
		t.Fatalf("exit code %d, want %d\nstdout: %s\nstderr: %s", r.code, want, r.stdout, r.stderr)
	}
}

func TestLoginSavesTheTokenInAFileOnlyTheUserCanRead(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")

	env, r := e.runJSON(e.fixture.Token+"\n", "auth", "login", "--with-token")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, e.fixture.Token) || strings.Contains(r.stderr, e.fixture.Token) {
		t.Error("the token appears in the output")
	}
	if !strings.Contains(env.Summary, "Bruno") {
		t.Errorf("summary %q", env.Summary)
	}

	info, err := os.Stat(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file permissions %04o, want 0600", perm)
	}
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if file.DefaultHost != e.server.URL || file.Hosts[e.server.URL].Token != e.fixture.Token {
		t.Errorf("config %+v", file)
	}

	// Signed in, a command needs neither host nor token from the environment.
	delete(e.vars, "MINKA_HOST")
	env, r = e.runJSON("", "me")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["name"] != "Bruno" {
		t.Errorf("me: %s", env.Data)
	}
}

func TestLoginWithARefusedTokenSavesNothing(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")

	env, r := e.runJSON("minka_pat_wrong\n", "auth", "login", "--with-token")
	expectCode(t, r, ExitAuth)
	if env.Code != CodeAuth {
		t.Errorf("code %q", env.Code)
	}
	if _, err := os.Stat(e.vars["MINKA_CONFIG"]); !os.IsNotExist(err) {
		t.Errorf("config file exists: %v", err)
	}
}

func TestLoginRefusesATokenThatIsNotOneLine(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("minka_pat_bruno\nsecond line\n", "auth", "login", "--with-token")
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != 0 {
		t.Error("the token was sent")
	}
}

func TestLoginWithoutATerminalNeedsWithToken(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON(e.fixture.Token, "auth", "login")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--with-token") {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestAConfigFileOthersCanReadIsNotUsed(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	path := e.vars["MINKA_CONFIG"]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := `{"defaultHost":"` + e.server.URL + `","hosts":{"` + e.server.URL + `":{"token":"minka_pat_bruno"}}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "me")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeConfig || !strings.Contains(env.Hint, "chmod 600") {
		t.Errorf("code %q, hint %q", env.Code, env.Hint)
	}
	if len(e.server.Requests()) != 0 {
		t.Error("a request was sent with the token of an insecure file")
	}
}

func TestLogoutRemovesTheToken(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	expectCode(t, e.run(e.fixture.Token, "auth", "login", "--with-token"), ExitOK)

	env, r := e.runJSON("", "auth", "logout")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["removed"] != true {
		t.Errorf("data %s", env.Data)
	}

	_, r = e.runJSON("", "me")
	expectCode(t, r, ExitAuth)
}

func TestWithoutHostOrTokenTheCLIIsNotSignedIn(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	delete(e.vars, "MINKA_HOST")

	env, r := e.runJSON("", "projects", "list")
	expectCode(t, r, ExitAuth)
	if !strings.Contains(env.Hint, "minka auth login") {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestStatusNamesThePersonAndTheSourceOfTheToken(t *testing.T) {
	e := newTestEnv(t)
	e.vars["MINKA_TOKEN"] = "minka_pat_agent"

	r := e.run("", "auth", "status", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"Claude (Bruno)", "Acting for Bruno", "$MINKA_TOKEN"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestOutputIsJSONWhenStdoutIsNotATerminal(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "me")
	expectCode(t, r, ExitOK)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil || !env.OK {
		t.Errorf("not an envelope: %s", r.stdout)
	}
}

func TestJSONDataIsTheResourceAsTheAPISentIt(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "boards", "show", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	board := decode(t, env.Data)
	columns := board["columns"].([]any)
	triage := columns[0].(map[string]any)
	if _, present := triage["customTitle"]; !present || triage["customTitle"] != nil {
		t.Errorf("customTitle should be present and null: %v", triage)
	}
	if len(env.Breadcrumbs) == 0 {
		t.Error("no breadcrumbs")
	}

	r = e.run("", "boards", "show", e.fixture.Board.ID, "--quiet")
	expectCode(t, r, ExitOK)
	if bare := decode(t, json.RawMessage(r.stdout)); bare["id"] != e.fixture.Board.ID {
		t.Errorf("--quiet should print the bare board: %s", r.stdout)
	}
}

func TestBoardsShowAsText(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"# API-3 Abnahme",
		"## Triage",
		"- " + e.fixture.CardInTriage.ID + "  Write the CLI (description, 1 comment)",
		"## Doing",
		"## Done",
		"(no cards)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestCardsShowAsText(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "cards", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"# Write the CLI", "Column: Triage (triage,", "## Description\n\nWith **Go**", "## Comments (1)", "### Bruno", "First comment"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestProjectsListShowsOnlyTheProjectsOfThePerson(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "projects", "list", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Test-Projekt") || strings.Contains(r.stdout, "Secret") {
		t.Errorf("output:\n%s", r.stdout)
	}
}

func TestCreateSendsMarkdownAsHTMLAndLandsInTriage(t *testing.T) {
	e := newTestEnv(t)
	description := "# Heading\n\n![i](https://example.com/i.png)\n\nSome *emphasis* and `code`."

	env, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "Neue Karte „mit“ Umlauten", "--description", description)
	expectCode(t, r, ExitOK)

	body := e.lastWriteBody()
	wantHTML := "<h1>Heading</h1>\n<p><a href=\"https://example.com/i.png\">i</a></p>\n<p>Some <em>emphasis</em> and <code>code</code>.</p>"
	if body["description"] != wantHTML {
		t.Errorf("description sent\n got: %q\nwant: %q", body["description"], wantHTML)
	}
	if body["title"] != "Neue Karte „mit“ Umlauten" {
		t.Errorf("title sent %q", body["title"])
	}
	if _, present := body["columnId"]; present {
		t.Error("columnId sent without --column")
	}
	if !strings.Contains(env.Notice, markdown.WarningImages) {
		t.Errorf("notice %q", env.Notice)
	}
	card := decode(t, env.Data)
	if card["column"].(map[string]any)["type"] != "triage" {
		t.Errorf("card not in Triage: %v", card["column"])
	}
}

func TestWarningsGoToStderrInTextOutput(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--description", "![i](https://example.com/i.png)", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stderr, "Warning: "+markdown.WarningImages) {
		t.Errorf("stderr: %q", r.stderr)
	}
	if strings.Contains(r.stdout, "Warning") {
		t.Errorf("warning on stdout: %q", r.stdout)
	}
}

func TestCreateReadsTheDescriptionFromStdin(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("Zeile mit \"Anführungszeichen\"\nund Umbruch\n", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--description", "-")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["description"]; got != "<p>Zeile mit &quot;Anführungszeichen&quot;\nund Umbruch</p>" {
		t.Errorf("description sent %q", got)
	}
}

func TestCreateSendsHTMLUnchangedWhenAskedTo(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--description", "<p>raw</p>", "--description-format", "html")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["description"]; got != "<p>raw</p>" {
		t.Errorf("description sent %q", got)
	}
}

func TestCreateInAColumnGivenByTitle(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--column", "doing")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["columnId"]; got != e.fixture.Doing.ID {
		t.Errorf("columnId sent %q", got)
	}
}

func TestCreateInDoneIsRefusedWithoutWriting(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--column", "done")
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != 0 {
		t.Error("a card was created")
	}
}

func TestEditWithoutChangesSendsNothing(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "edit", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != 0 {
		t.Error("a request was sent")
	}
}

func TestEditWithAnEmptyDescriptionRemovesIt(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "cards", "edit", e.fixture.CardInTriage.ID, "--description", "")
	expectCode(t, r, ExitOK)

	body := e.lastWriteBody()
	if len(body) != 1 || body["description"] != "" {
		t.Errorf("body sent %v", body)
	}
	if decode(t, env.Data)["description"] != "" {
		t.Errorf("card %s", env.Data)
	}
}

func TestMovePlacements(t *testing.T) {
	tests := []struct {
		name  string
		args  func(f *fakeapi.Fixture) []string
		wants func(f *fakeapi.Fixture) map[string]any
	}{
		{
			"without placement",
			func(*fakeapi.Fixture) []string { return nil },
			func(f *fakeapi.Fixture) map[string]any { return map[string]any{"columnId": f.Doing.ID} },
		},
		{
			"top",
			func(*fakeapi.Fixture) []string { return []string{"--top"} },
			func(f *fakeapi.Fixture) map[string]any {
				return map[string]any{"columnId": f.Doing.ID, "position": "top"}
			},
		},
		{
			"bottom",
			func(*fakeapi.Fixture) []string { return []string{"--bottom"} },
			func(f *fakeapi.Fixture) map[string]any {
				return map[string]any{"columnId": f.Doing.ID, "position": "bottom"}
			},
		},
		{
			"after",
			func(f *fakeapi.Fixture) []string { return []string{"--after", f.CardInDoing.ID} },
			func(f *fakeapi.Fixture) map[string]any {
				return map[string]any{"columnId": f.Doing.ID, "previousCardId": f.CardInDoing.ID}
			},
		},
		{
			"before",
			func(f *fakeapi.Fixture) []string { return []string{"--before", f.CardInDoing.ID} },
			func(f *fakeapi.Fixture) map[string]any {
				return map[string]any{"columnId": f.Doing.ID, "nextCardId": f.CardInDoing.ID}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := newTestEnv(t)
			args := append([]string{"cards", "move", e.fixture.CardInTriage.ID, "--column", "Doing"}, test.args(e.fixture)...)
			env, r := e.runJSON("", args...)
			expectCode(t, r, ExitOK)

			body := e.lastWriteBody()
			want := test.wants(e.fixture)
			if len(body) != len(want) {
				t.Errorf("body sent %v, want %v", body, want)
			}
			for key, value := range want {
				if body[key] != value {
					t.Errorf("%s sent %v, want %v", key, body[key], value)
				}
			}
			if decode(t, env.Data)["column"].(map[string]any)["id"] != e.fixture.Doing.ID {
				t.Errorf("card not in Doing: %s", env.Data)
			}
		})
	}
}

func TestMoveWithTwoPlacementsIsAUsageError(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "move", e.fixture.CardInTriage.ID, "--column", "Doing", "--top", "--after", e.fixture.CardInDoing.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != 0 {
		t.Error("a request was sent")
	}
}

func TestMoveNextToACardOutsideTheTargetColumnIsRefused(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "move", e.fixture.CardInTriage.ID, "--column", "triage", "--after", e.fixture.CardInDoing.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != 0 {
		t.Error("the card was moved")
	}
}

func TestDoneCompletesACardOnlyOnce(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "cards", "done", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if body["columnId"] != e.fixture.Done.ID || body["position"] != "top" {
		t.Errorf("body sent %v", body)
	}
	if decode(t, env.Data)["completedAt"] == nil {
		t.Errorf("card not completed: %s", env.Data)
	}

	env, r = e.runJSON("", "cards", "done", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitOK)
	if writes := e.server.WritingRequests(); len(writes) != 1 {
		t.Errorf("%d moves sent, want 1", len(writes))
	}
	if !strings.Contains(env.Summary, "already done") {
		t.Errorf("summary %q", env.Summary)
	}
}

func TestACommentIsSentAsHTMLWithItsLineBreaks(t *testing.T) {
	e := newTestEnv(t)
	text := "**Bold** and <b>raw</b> & kept\nsecond line"

	env, r := e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, text)
	expectCode(t, r, ExitOK)
	want := "<p><strong>Bold</strong> and &lt;b&gt;raw&lt;/b&gt; &amp; kept<br>second line</p>"
	if body := e.lastWriteBody(); len(body) != 1 || body["body"] != want {
		t.Errorf("body sent %v", body)
	}
	if !strings.Contains(env.Notice, markdown.WarningRawHTML) {
		t.Errorf("notice %q", env.Notice)
	}
}

func TestACommentInHTMLIsSentAsItIs(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, "<p>as <em>is</em></p>", "--format", "html")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["body"]; got != "<p>as <em>is</em></p>" {
		t.Errorf("body sent %q", got)
	}

	_, r = e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, "x", "--format", "text")
	expectCode(t, r, ExitUsage)
}

func TestACommentWithNothingMinkaKeepsIsRefused(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, "[x]: https://example.com")
	expectCode(t, r, ExitUsage)
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestCommentsShowAsMarkdownAndOldPlainOnesAsTheyAre(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	card.Comments = append(card.Comments,
		&fakeapi.Comment{ID: "66666666-6666-4666-8666-666666666661", AuthorID: e.fixture.Anke.ID, Body: "<p><strong>Rich</strong><br>two</p>"},
		&fakeapi.Comment{ID: "66666666-6666-4666-8666-666666666662", AuthorID: e.fixture.Anke.ID, Body: "old *plain* text\nwith a < sign"},
	)

	r := e.run("", "cards", "show", card.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"First comment", "**Rich**\\\ntwo", "old *plain* text\nwith a < sign"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestCommentFromStdin(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("Zwei\nZeilen\n", "comments", "create", e.fixture.CardInTriage.ID, "-")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["body"]; got != "<p>Zwei<br>Zeilen</p>" {
		t.Errorf("body sent %q", got)
	}
}

func TestFailuresHaveACodeAndAnExitCodePerClass(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	tests := []struct {
		name      string
		prepare   func(e *testEnv)
		args      func(f *fakeapi.Fixture) []string
		exitCode  int
		code      string
		apiCode   string
		retryable bool
	}{
		{
			name: "unknown card",
			args: func(*fakeapi.Fixture) []string {
				return []string{"cards", "show", "00000000-0000-4000-8000-000000000000"}
			},
			exitCode: ExitNotFound, code: CodeNotFound, apiCode: "notFound",
		},
		{
			name:     "card of a project the person is not in",
			args:     func(f *fakeapi.Fixture) []string { return []string{"cards", "show", f.OtherCard.ID} },
			exitCode: ExitNotFound, code: CodeNotFound, apiCode: "notFound",
		},
		{
			name:     "refused token",
			prepare:  func(e *testEnv) { e.vars["MINKA_TOKEN"] = "minka_pat_revoked" },
			args:     func(*fakeapi.Fixture) []string { return []string{"me"} },
			exitCode: ExitAuth, code: CodeAuth, apiCode: "unauthenticated",
		},
		{
			name: "validation",
			args: func(f *fakeapi.Fixture) []string {
				return []string{"cards", "create", "--board", f.Board.ID, "--title", "   "}
			},
			exitCode: ExitValidation, code: CodeValidation, apiCode: "cardTitleMissing",
		},
		{
			name:     "server error without a JSON body",
			prepare:  func(e *testEnv) { e.server.FailNextWith(http.StatusBadGateway) },
			args:     func(*fakeapi.Fixture) []string { return []string{"me"} },
			exitCode: ExitAPI, code: CodeAPI, apiCode: "serverError", retryable: true,
		},
		{
			name:     "server error on a write",
			prepare:  func(e *testEnv) { e.server.FailNextWith(http.StatusBadGateway) },
			args:     func(f *fakeapi.Fixture) []string { return []string{"comments", "create", f.CardInTriage.ID, "hi"} },
			exitCode: ExitAPI, code: CodeAPI, apiCode: "serverError", retryable: false,
		},
		{
			name:     "unreachable host",
			prepare:  func(e *testEnv) { e.vars["MINKA_HOST"] = closedURL },
			args:     func(*fakeapi.Fixture) []string { return []string{"me"} },
			exitCode: ExitNetwork, code: CodeNetwork, retryable: true,
		},
		{
			name:     "unreachable host on a write",
			prepare:  func(e *testEnv) { e.vars["MINKA_HOST"] = closedURL },
			args:     func(f *fakeapi.Fixture) []string { return []string{"comments", "create", f.CardInTriage.ID, "hi"} },
			exitCode: ExitNetwork, code: CodeNetwork, retryable: false,
		},
		{
			name:     "unknown flag",
			args:     func(*fakeapi.Fixture) []string { return []string{"me", "--nope"} },
			exitCode: ExitUsage, code: CodeUsage,
		},
		{
			name:     "unknown command",
			args:     func(*fakeapi.Fixture) []string { return []string{"nope"} },
			exitCode: ExitUsage, code: CodeUsage,
		},
		{
			name:     "unknown subcommand",
			args:     func(*fakeapi.Fixture) []string { return []string{"cards", "nope"} },
			exitCode: ExitUsage, code: CodeUsage,
		},
		{
			name:     "missing argument",
			args:     func(*fakeapi.Fixture) []string { return []string{"cards", "show"} },
			exitCode: ExitUsage, code: CodeUsage,
		},
		{
			name:     "missing required flag",
			args:     func(f *fakeapi.Fixture) []string { return []string{"cards", "create", "--board", f.Board.ID} },
			exitCode: ExitUsage, code: CodeUsage,
		},
		{
			name: "two columns with the same title",
			prepare: func(e *testEnv) {
				title := "doing"
				e.fixture.Board.Columns = append(e.fixture.Board.Columns, &fakeapi.Column{ID: "11111111-1111-4111-8111-111111111111", Type: "column", CustomTitle: &title})
			},
			args: func(f *fakeapi.Fixture) []string {
				return []string{"cards", "move", f.CardInTriage.ID, "--column", "Doing"}
			},
			exitCode: ExitAmbiguous, code: CodeAmbiguous,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := newTestEnv(t)
			if test.prepare != nil {
				test.prepare(e)
			}
			env, r := e.runJSON("", test.args(e.fixture)...)
			expectCode(t, r, test.exitCode)
			if env.OK || env.Code != test.code || env.Error == "" || env.Retryable != test.retryable {
				t.Errorf("envelope %+v", env)
			}
			if test.apiCode != "" && (env.Meta == nil || env.Meta["apiCode"] != test.apiCode) {
				t.Errorf("meta %v, want apiCode %q", env.Meta, test.apiCode)
			}
		})
	}
}

func TestFailuresInTextOutputGoToStderr(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "", "--styled")
	expectCode(t, r, ExitValidation)
	if r.stdout != "" || !strings.Contains(r.stderr, "Error: A card needs a title. (cardTitleMissing)") {
		t.Errorf("stdout %q, stderr %q", r.stdout, r.stderr)
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	e := newTestEnv(t)
	e.server.FailNextWith(http.StatusFound)

	env, r := e.runJSON("", "me")
	expectCode(t, r, ExitAPI)
	if env.Meta["apiCode"] != "unexpectedRedirect" {
		t.Errorf("meta %v", env.Meta)
	}
	if n := len(e.server.Requests()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestValuesThatAreNoIDsAreRefusedBeforeSending(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"cards", "show", "../me"},
		{"cards", "show", ".."},
		{"boards", "list", "x/y"},
		{"cards", "create", "--board", "..", "--title", "x"},
		{"cards", "move", e.fixture.CardInTriage.ID, "--column", "triage", "--after", ".."},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "is not an ID") {
			t.Errorf("%v: error %q", args, env.Error)
		}
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestRequestsCarryTheTokenAndJSONHeaders(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "comments", "create", e.fixture.CardInTriage.ID, "hi", "--json"), ExitOK)

	for _, request := range e.server.Requests() {
		if got := request.Header.Get("Authorization"); got != "Bearer "+e.fixture.Token {
			t.Errorf("%s %s: Authorization %q", request.Method, request.Path, got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("%s %s: Accept %q", request.Method, request.Path, got)
		}
		if request.Method != http.MethodGet && request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s: Content-Type %q", request.Method, request.Path, request.Header.Get("Content-Type"))
		}
	}
}

func TestAGroupWithoutSubcommandShowsHelp(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "cards")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "minka cards [command]") {
		t.Errorf("stdout %q", r.stdout)
	}
}

func TestTheCatalogListsEveryCommandWithItsFlags(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "commands", "--quiet")
	expectCode(t, r, ExitOK)

	var catalog struct {
		Commands    []catalogCommand `json:"commands"`
		GlobalFlags []catalogFlag    `json:"globalFlags"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &catalog); err != nil {
		t.Fatal(err)
	}

	byName := map[string]catalogCommand{}
	for _, cmd := range catalog.Commands {
		byName[cmd.Command] = cmd
	}
	for _, name := range []string{"minka auth login", "minka me", "minka projects list", "minka boards list", "minka boards show", "minka cards show", "minka cards create", "minka cards edit", "minka cards move", "minka cards done", "minka cards delete", "minka cards assign", "minka cards unassign", "minka people list", "minka steps list", "minka steps create", "minka steps complete", "minka steps reopen", "minka steps delete", "minka comments create", "minka chats list", "minka chats mark-read", "minka messages list", "minka messages create", "minka messages edit", "minka messages delete"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("catalog misses %q", name)
		}
	}
	var moveFlags []string
	for _, flag := range byName["minka cards move"].Flags {
		moveFlags = append(moveFlags, flag.Name)
	}
	if strings.Join(moveFlags, ",") != "after,before,bottom,column,top" {
		t.Errorf("flags of cards move: %v", moveFlags)
	}
	if len(catalog.GlobalFlags) == 0 {
		t.Error("no global flags")
	}
}

func TestTheEnvironmentTokenIsOnlySentToItsHost(t *testing.T) {
	e := newTestEnv(t)
	other, otherFixture := fakeapi.New(t)

	_, r := e.runJSON("", "me", "--host", other.URL)
	expectCode(t, r, ExitAuth)
	if n := len(other.Requests()); n != 0 {
		t.Fatalf("%d requests reached the other host", n)
	}

	file := &config.File{Hosts: map[string]config.HostEntry{other.URL: {Token: otherFixture.StrangerToken}}}
	if err := file.Save(e.vars["MINKA_CONFIG"]); err != nil {
		t.Fatal(err)
	}
	_, r = e.runJSON("", "me", "--host", other.URL)
	expectCode(t, r, ExitOK)
	for _, request := range other.Requests() {
		if got := request.Header.Get("Authorization"); got != "Bearer "+otherFixture.StrangerToken {
			t.Errorf("the other host got %q", got)
		}
	}
}

func TestTheEnvironmentTokenNeedsItsHost(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_HOST")

	env, r := e.runJSON("", "me")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "MINKA_HOST") {
		t.Errorf("error %q", env.Error)
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestAFailedWriteTellsToReadBackBeforeRetrying(t *testing.T) {
	e := newTestEnv(t)
	e.server.FailNextWith(http.StatusBadGateway)

	env, r := e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, "hi")
	expectCode(t, r, ExitAPI)
	if env.Retryable || env.Hint != writeMayHaveHappened {
		t.Errorf("envelope %+v", env)
	}
}

func TestQuietPrintsWarningsToStderr(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--description", "![i](https://example.com/i.png)", "--quiet")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stderr, "Warning: "+markdown.WarningImages) {
		t.Errorf("stderr %q", r.stderr)
	}
	if card := decode(t, json.RawMessage(r.stdout)); card["title"] != "x" {
		t.Errorf("stdout %q", r.stdout)
	}
}

func TestCreateInNotNowOrDoneIsRefusedHoweverTheColumnIsGiven(t *testing.T) {
	e := newTestEnv(t)
	for _, column := range []string{e.fixture.Done.ID, e.fixture.NotNow.ID, "Not now", "not-now", "done"} {
		_, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--column", column)
		expectCode(t, r, ExitUsage)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d cards created", n)
	}
}

func TestMarkdownWithNothingMinkaKeepsDoesNotRemoveTheDescription(t *testing.T) {
	e := newTestEnv(t)
	for _, description := range []string{"[x]: https://example.com", "[a]: /a\n[b]: /b"} {
		_, r := e.runJSON("", "cards", "edit", e.fixture.CardInTriage.ID, "--description", description)
		expectCode(t, r, ExitUsage)
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestTheDescriptionFormatIsCheckedWithoutADescription(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "edit", e.fixture.CardInTriage.ID, "--title", "x", "--description-format", "bogus")
	expectCode(t, r, ExitUsage)
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestDoneTakesNoPlacement(t *testing.T) {
	e := newTestEnv(t)
	for _, placement := range [][]string{{"--bottom"}, {"--after", e.fixture.CardInDoing.ID}, {"--before", e.fixture.CardInDoing.ID}} {
		args := append([]string{"cards", "move", e.fixture.CardInTriage.ID, "--column", "done"}, placement...)
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d moves sent", n)
	}
}

func TestATypeThatIsAlsoTheTitleOfAnotherColumnIsAmbiguous(t *testing.T) {
	e := newTestEnv(t)
	title := "Done"
	e.fixture.Board.Columns = append(e.fixture.Board.Columns, &fakeapi.Column{ID: "22222222-2222-4222-8222-222222222222", Type: "column", CustomTitle: &title})

	env, r := e.runJSON("", "cards", "move", e.fixture.CardInTriage.ID, "--column", "done")
	expectCode(t, r, ExitAmbiguous)
	if !strings.Contains(env.Hint, "22222222-2222-4222-8222-222222222222") || !strings.Contains(env.Hint, e.fixture.Done.ID) {
		t.Errorf("hint %q", env.Hint)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d moves sent", n)
	}
}

func TestInputOverTheLimitsIsRefusedWithoutSending(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON(strings.Repeat("x", maxInputBytes+1), "comments", "create", e.fixture.CardInTriage.ID, "-")
	expectCode(t, r, ExitUsage)

	delete(e.vars, "MINKA_TOKEN")
	_, r = e.runJSON(strings.Repeat("x", maxTokenBytes+1), "auth", "login", "--with-token")
	expectCode(t, r, ExitUsage)

	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestLoginAsksForTheTokenOnATerminal(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")

	r := e.runWith(func(a *app) {
		a.stdinIsTerminal = func() bool { return true }
		a.readSecret = func() (string, error) { return e.fixture.Token + "\n", nil }
	}, "", "auth", "login", "--json")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stderr, "Access token for "+e.server.URL) {
		t.Errorf("no prompt on stderr: %q", r.stderr)
	}
	if strings.Contains(r.stdout+r.stderr, e.fixture.Token) {
		t.Error("the token appears in the output")
	}
	if file, err := config.Load(e.vars["MINKA_CONFIG"]); err != nil || file.Hosts[e.server.URL].Token != e.fixture.Token {
		t.Errorf("token not saved: %+v, %v", file, err)
	}
}

func TestTextIsNotReadFromATerminal(t *testing.T) {
	e := newTestEnv(t)
	r := e.runWith(func(a *app) {
		a.stdinIsTerminal = func() bool { return true }
	}, "", "comments", "create", e.fixture.CardInTriage.ID, "-", "--json")
	expectCode(t, r, ExitUsage)
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

func TestLogoutWithoutATokenSaysSo(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")

	env, r := e.runJSON("", "auth", "logout")
	expectCode(t, r, ExitOK)
	if !strings.HasPrefix(env.Summary, "Not signed in") {
		t.Errorf("summary %q", env.Summary)
	}
}

func TestACommentWithWindowsLineEndingsIsAccepted(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("one\r\ntwo", "comments", "create", e.fixture.CardInTriage.ID, "-")
	expectCode(t, r, ExitOK)
}

func TestDeleteMovesACardToTheTrash(t *testing.T) {
	e := newTestEnv(t)
	id := e.fixture.CardInTriage.ID

	env, r := e.runJSON("", "cards", "delete", id)
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["id"] != id || data["deleted"] != true {
		t.Errorf("data %s", env.Data)
	}
	writes := e.server.WritingRequests()
	if len(writes) != 1 || writes[0].Method != http.MethodDelete || writes[0].Path != "/api/v1/cards/"+id || len(writes[0].Body) != 0 {
		t.Errorf("requests %+v", writes)
	}

	_, r = e.runJSON("", "cards", "show", id)
	expectCode(t, r, ExitNotFound)

	env, r = e.runJSON("", "cards", "delete", id)
	expectCode(t, r, ExitNotFound)
	if env.Code != CodeNotFound || !strings.Contains(env.Hint, "trash") {
		t.Errorf("envelope %+v", env)
	}
}

func TestDeletingACardOfAnotherProjectIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "delete", e.fixture.OtherCard.ID)
	expectCode(t, r, ExitNotFound)
}

func TestTrashIsAnotherNameForDelete(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "cards", "trash", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "to the trash") {
		t.Errorf("stdout %q", r.stdout)
	}
}

func TestPeopleListShowsTheMembersOfAProject(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "people", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)

	var people []api.ProjectMember
	if err := json.Unmarshal(env.Data, &people); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, person := range people {
		names = append(names, person.Name)
	}
	if got := strings.Join(names, ","); got != "Bruno,Anke,Claude (Bruno)" {
		t.Errorf("people %q", got)
	}

	_, r = e.runJSON("", "people", "list", e.fixture.OtherProject.ID)
	expectCode(t, r, ExitNotFound)
}

func TestAssignFindsThePersonByIDMeEmailOrName(t *testing.T) {
	tests := map[string]func(f *fakeapi.Fixture) (string, string){
		"me":     func(f *fakeapi.Fixture) (string, string) { return "me", f.Bruno.ID },
		"e-mail": func(f *fakeapi.Fixture) (string, string) { return "Anke@Example.com", f.Anke.ID },
		"name":   func(f *fakeapi.Fixture) (string, string) { return "anke", f.Anke.ID },
		"ID":     func(f *fakeapi.Fixture) (string, string) { return f.Anke.ID, f.Anke.ID },
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			value, want := test(e.fixture)

			env, r := e.runJSON("", "cards", "assign", e.fixture.CardInTriage.ID, value)
			expectCode(t, r, ExitOK)
			if got := e.lastWriteBody()["personId"]; got != want {
				t.Errorf("personId sent %v, want %s", got, want)
			}
			var card api.Card
			if err := json.Unmarshal(env.Data, &card); err != nil || len(card.Assignees) != 1 || card.Assignees[0].ID != want {
				t.Errorf("card %s", env.Data)
			}
		})
	}
}

func TestAssigningTwiceSucceedsAndSaysSo(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "cards", "assign", e.fixture.CardInTriage.ID, "me", "--json"), ExitOK)

	env, r := e.runJSON("", "cards", "assign", e.fixture.CardInTriage.ID, "me")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "already") {
		t.Errorf("summary %q", env.Summary)
	}
}

func TestSomebodyOutsideTheProjectIsNotAssigned(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "cards", "assign", e.fixture.CardInTriage.ID, e.fixture.Stranger.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "personNotAProjectMember" {
		t.Errorf("meta %v", env.Meta)
	}

	before := len(e.server.WritingRequests())
	_, r = e.runJSON("", "cards", "assign", e.fixture.CardInTriage.ID, "Stranger")
	expectCode(t, r, ExitUsage)
	if n := len(e.server.WritingRequests()); n != before {
		t.Errorf("%d assignments sent for a name nobody in the project has", n-before)
	}
}

func TestANameOfTwoPeopleIsAmbiguous(t *testing.T) {
	e := newTestEnv(t)
	other := &fakeapi.Person{ID: "44444444-4444-4444-8444-444444444444", Name: "Anke", Email: "anke2@example.com"}
	e.server.People[other.ID] = other
	e.fixture.Project.MemberIDs = append(e.fixture.Project.MemberIDs, other.ID)

	env, r := e.runJSON("", "cards", "assign", e.fixture.CardInTriage.ID, "Anke")
	expectCode(t, r, ExitAmbiguous)
	if !strings.Contains(env.Hint, other.ID) || !strings.Contains(env.Hint, e.fixture.Anke.ID) {
		t.Errorf("hint %q", env.Hint)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d assignments sent", n)
	}
}

func TestUnassignTakesSomebodyOffAndSucceedsAgain(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	card.AssigneeIDs = []string{e.fixture.Anke.ID}

	env, r := e.runJSON("", "cards", "unassign", card.ID, "anke")
	expectCode(t, r, ExitOK)
	writes := e.server.WritingRequests()
	if len(writes) != 1 || writes[0].Method != http.MethodDelete || writes[0].Path != "/api/v1/cards/"+card.ID+"/assignees/"+e.fixture.Anke.ID {
		t.Errorf("requests %+v", writes)
	}
	if data := decode(t, env.Data); data["wasAssigned"] != true || len(card.AssigneeIDs) != 0 {
		t.Errorf("data %s, assignees %v", env.Data, card.AssigneeIDs)
	}

	env, r = e.runJSON("", "cards", "unassign", card.ID, "anke")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "was not assigned") {
		t.Errorf("summary %q", env.Summary)
	}
}

func TestUnassignFindsSomebodyWhoLeftTheProject(t *testing.T) {
	e := newTestEnv(t)
	former := &fakeapi.Person{ID: "33333333-3333-4333-8333-333333333333", Name: "Former", Email: "former@example.com"}
	e.server.People[former.ID] = former
	e.fixture.CardInTriage.AssigneeIDs = []string{former.ID}

	_, r := e.runJSON("", "cards", "unassign", e.fixture.CardInTriage.ID, "former")
	expectCode(t, r, ExitOK)
	if writes := e.server.WritingRequests(); len(writes) != 1 || !strings.HasSuffix(writes[0].Path, "/assignees/"+former.ID) {
		t.Errorf("requests %+v", writes)
	}
}

func TestCreateAssignsThePeopleItFoundBeforeCreating(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--assignee", "me", "--assignee", "anke@example.com", "--assignee", "Bruno")
	expectCode(t, r, ExitOK)

	writes := e.server.WritingRequests()
	if len(writes) != 1 || !strings.HasSuffix(writes[0].Path, "/cards") {
		t.Fatalf("requests %+v", writes)
	}
	ids, _ := e.lastWriteBody()["assigneeIds"].([]any)
	if len(ids) != 2 || ids[0] != e.fixture.Bruno.ID || ids[1] != e.fixture.Anke.ID {
		t.Errorf("assigneeIds sent %v", ids)
	}
	var card api.Card
	if err := json.Unmarshal(env.Data, &card); err != nil || len(card.Assignees) != 2 {
		t.Errorf("card %s", env.Data)
	}

	e = newTestEnv(t)
	_, r = e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--assignee", "Nobody")
	expectCode(t, r, ExitUsage)
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d requests written for a person nobody matches", n)
	}
}

func TestAssigneesAppearInTheTextOutput(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInTriage.AssigneeIDs = []string{e.fixture.Anke.ID, e.fixture.Bruno.ID}

	r := e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "assigned to Anke, Bruno") {
		t.Errorf("board:\n%s", r.stdout)
	}
	r = e.run("", "cards", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Assigned to: Anke, Bruno") {
		t.Errorf("card:\n%s", r.stdout)
	}
}

func TestCreateWithADueDate(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--due", "2026-09-28")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["dueOn"]; got != "2026-09-28" {
		t.Errorf("dueOn sent %v", got)
	}
	if decode(t, env.Data)["dueOn"] != "2026-09-28" {
		t.Errorf("card %s", env.Data)
	}

	_, r = e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "y")
	expectCode(t, r, ExitOK)
	if _, sent := e.lastWriteBody()["dueOn"]; sent {
		t.Error("dueOn sent without --due")
	}
}

func TestCreateWithSomebodyOutsideTheProjectLeavesNoCard(t *testing.T) {
	e := newTestEnv(t)
	before := len(e.fixture.Triage.Cards)

	env, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--assignee", "me", "--assignee", e.fixture.Stranger.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "personNotAProjectMember" {
		t.Errorf("meta %v", env.Meta)
	}
	if n := len(e.fixture.Triage.Cards); n != before {
		t.Errorf("%d cards in Triage, want %d", n, before)
	}
}

func TestEditSetsAndRemovesTheDueDate(t *testing.T) {
	e := newTestEnv(t)
	id := e.fixture.CardInTriage.ID

	env, r := e.runJSON("", "cards", "edit", id, "--due", "2026-09-28")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["dueOn"] != "2026-09-28" {
		t.Errorf("body sent %v", body)
	}
	if decode(t, env.Data)["dueOn"] != "2026-09-28" {
		t.Errorf("card %s", env.Data)
	}

	env, r = e.runJSON("", "cards", "edit", id, "--due", "")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["dueOn"] != "" {
		t.Errorf("body sent %v", body)
	}
	if card := decode(t, env.Data); card["dueOn"] != nil {
		t.Errorf("due date not removed: %s", env.Data)
	}
}

func TestADueDateThatIsNoDayIsRefused(t *testing.T) {
	e := newTestEnv(t)
	for _, value := range []string{"2026-02-30", "28.09.2026", "tomorrow", "2026-9-8"} {
		env, r := e.runJSON("", "cards", "edit", e.fixture.CardInTriage.ID, "--due", value)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != "cardDueDateNotADate" {
			t.Errorf("%s: meta %v", value, env.Meta)
		}
	}
	if e.fixture.CardInTriage.DueOn != nil {
		t.Errorf("due date set to %q", *e.fixture.CardInTriage.DueOn)
	}
}

func TestTheDueDateAppearsInTheTextOutput(t *testing.T) {
	e := newTestEnv(t)
	due := "2026-09-28"
	e.fixture.CardInTriage.DueOn = &due

	r := e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "due 2026-09-28") {
		t.Errorf("board:\n%s", r.stdout)
	}
	r = e.run("", "cards", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Due: 2026-09-28") {
		t.Errorf("card:\n%s", r.stdout)
	}
}

// withSteps gives the card in Triage three steps, two of them with the same
// text.
func withSteps(e *testEnv) []*fakeapi.Step {
	steps := []*fakeapi.Step{
		{ID: "55555555-5555-4555-8555-555555555551", Title: "Call the caterer"},
		{ID: "55555555-5555-4555-8555-555555555552", Title: "Book the tent", Completed: true},
		{ID: "55555555-5555-4555-8555-555555555553", Title: "Book the tent"},
	}
	e.fixture.CardInTriage.Steps = steps
	return steps
}

func TestStepsListShowsTheStepsInOrder(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)

	env, r := e.runJSON("", "steps", "list", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitOK)
	var steps []api.Step
	if err := json.Unmarshal(env.Data, &steps); err != nil || len(steps) != 3 || !steps[1].Completed || steps[0].Title != "Call the caterer" {
		t.Errorf("data %s", env.Data)
	}
	if !strings.Contains(env.Summary, "1 of 3 steps") {
		t.Errorf("summary %q", env.Summary)
	}

	r = e.run("", "steps", "list", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "2. [x] Book the tent") {
		t.Errorf("text:\n%s", r.stdout)
	}
}

func TestStepsCreateAddsAStepAtTheEnd(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)

	env, r := e.runJSON("", "steps", "create", e.fixture.CardInTriage.ID, "Zeltverleih anrufen")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["title"] != "Zeltverleih anrufen" {
		t.Errorf("body sent %v", body)
	}
	var card api.Card
	if err := json.Unmarshal(env.Data, &card); err != nil || len(card.Steps) != 4 || card.Steps[3].Title != "Zeltverleih anrufen" {
		t.Errorf("card %s", env.Data)
	}

	_, r = e.runJSON("Aufbau\n", "steps", "create", e.fixture.CardInTriage.ID, "-")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["title"]; got != "Aufbau\n" {
		t.Errorf("title sent %q", got)
	}
}

func TestAStepThatIsNoLineIsRefused(t *testing.T) {
	e := newTestEnv(t)
	for value, code := range map[string]string{
		"   ":                    "stepMissing",
		strings.Repeat("ä", 501): "stepTooLong",
		"first\tsecond":          "stepHasControlCharacters",
	} {
		env, r := e.runJSON("", "steps", "create", e.fixture.CardInTriage.ID, value)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != code {
			t.Errorf("%q: meta %v, want %s", value, env.Meta, code)
		}
	}
	if n := len(e.fixture.CardInTriage.Steps); n != 0 {
		t.Errorf("%d steps added", n)
	}
}

func TestCompleteAndReopenFindTheStepByPositionTextOrID(t *testing.T) {
	e := newTestEnv(t)
	steps := withSteps(e)
	card := e.fixture.CardInTriage.ID

	_, r := e.runJSON("", "steps", "complete", card, "1")
	expectCode(t, r, ExitOK)
	writes := e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodPatch || last.Path != "/api/v1/cards/"+card+"/steps/"+steps[0].ID {
		t.Errorf("request %s %s", last.Method, last.Path)
	}
	if body := e.lastWriteBody(); len(body) != 1 || body["completed"] != true || !steps[0].Completed {
		t.Errorf("body sent %v", body)
	}

	env, r := e.runJSON("", "steps", "complete", card, "call THE caterer")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "already") {
		t.Errorf("summary %q", env.Summary)
	}

	_, r = e.runJSON("", "steps", "reopen", card, steps[1].ID)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["completed"] != false || steps[1].Completed {
		t.Errorf("body sent %v", body)
	}
}

func TestAStepTextOfTwoStepsIsAmbiguous(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)

	env, r := e.runJSON("", "steps", "complete", e.fixture.CardInTriage.ID, "book the tent")
	expectCode(t, r, ExitAmbiguous)
	if !strings.Contains(env.Error, "2 and 3") {
		t.Errorf("error %q", env.Error)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d requests written", n)
	}
}

func TestAPositionOutsideTheListIsRefused(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)
	for _, position := range []string{"0", "4", "-1"} {
		_, r := e.runJSON("", "steps", "delete", e.fixture.CardInTriage.ID, position)
		expectCode(t, r, ExitUsage)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d requests written", n)
	}
}

func TestStepsDeleteRemovesAStep(t *testing.T) {
	e := newTestEnv(t)
	// The fake removes the step from the same slice, so its ID is taken first.
	removedID := withSteps(e)[1].ID
	card := e.fixture.CardInTriage

	env, r := e.runJSON("", "steps", "delete", card.ID, "2")
	expectCode(t, r, ExitOK)
	writes := e.server.WritingRequests()
	if len(writes) != 1 || writes[0].Method != http.MethodDelete || writes[0].Path != "/api/v1/cards/"+card.ID+"/steps/"+removedID {
		t.Errorf("requests %+v", writes)
	}
	if len(card.Steps) != 2 || decode(t, env.Data)["wasOnCard"] != true {
		t.Errorf("steps %v, data %s", card.Steps, env.Data)
	}
}

func TestStepsAppearOnTheCard(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)

	r := e.run("", "cards", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"## Steps (1/3)", "1. [ ] Call the caterer", "2. [x] Book the tent"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestTheBoardSaysHowFarTheStepsHaveGot(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)

	env, r := e.runJSON("", "boards", "show", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	var board api.Board
	if err := json.Unmarshal(env.Data, &board); err != nil {
		t.Fatal(err)
	}
	if card := board.Columns[0].Cards[0]; card.StepCount != 3 || card.CompletedStepCount != 1 {
		t.Errorf("card %+v", card)
	}

	r = e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "steps 1/3") {
		t.Errorf("board:\n%s", r.stdout)
	}
}

func messageSeqs(t *testing.T, raw json.RawMessage) []int {
	t.Helper()
	var messages []api.ChatMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatalf("data is no list of messages: %v: %s", err, raw)
	}
	seqs := make([]int, len(messages))
	for i, message := range messages {
		seqs[i] = message.Seq
	}
	return seqs
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestChatsListShowsWhatIsUnread(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "chats", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	var chats []api.Chat
	if err := json.Unmarshal(env.Data, &chats); err != nil || len(chats) != 1 || chats[0].Name != "Planung" || chats[0].UnreadCount != 2 {
		t.Errorf("data %s", env.Data)
	}

	_, r = e.runJSON("", "chats", "list", e.fixture.OtherProject.ID)
	expectCode(t, r, ExitNotFound)
}

func TestMessagesListReadsBackAndCatchesUp(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat.ID

	for _, test := range []struct {
		args  []string
		query string
		seqs  []int
	}{
		{nil, "", []int{1, 2, 3}},
		{[]string{"--limit", "2"}, "limit=2", []int{2, 3}},
		{[]string{"--before", "3", "--limit", "1"}, "before=3&limit=1", []int{2}},
		{[]string{"--after", "2"}, "after=2", []int{3}},
	} {
		env, r := e.runJSON("", append([]string{"messages", "list", chat}, test.args...)...)
		expectCode(t, r, ExitOK)
		if got := messageSeqs(t, env.Data); !sameInts(got, test.seqs) {
			t.Errorf("%v: seqs %v, want %v", test.args, got, test.seqs)
		}
		requests := e.server.Requests()
		if got := requests[len(requests)-1].Query; got != test.query {
			t.Errorf("%v: query %q, want %q", test.args, got, test.query)
		}
	}
}

func TestMessagesListRefusesWhatIsNoPage(t *testing.T) {
	e := newTestEnv(t)

	_, r := e.runJSON("", "messages", "list", e.fixture.Chat.ID, "--before", "3", "--after", "1")
	expectCode(t, r, ExitUsage)
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}

	env, r := e.runJSON("", "messages", "list", e.fixture.Chat.ID, "--limit", "0")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "chatMessagesLimitNotANumber" {
		t.Errorf("meta %v", env.Meta)
	}

	_, r = e.runJSON("", "messages", "list", e.fixture.OtherChat.ID)
	expectCode(t, r, ExitNotFound)
}

func TestMessagesCreateWritesOncePerClientMessageID(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	id := "77777777-7777-4777-8777-777777777777"

	env, r := e.runJSON("", "messages", "create", chat.ID, "Zelt kommt um 9", "--client-message-id", id)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 2 || body["body"] != "<p>Zelt kommt um 9</p>" || body["clientMessageId"] != id {
		t.Errorf("body sent %v", body)
	}
	if decode(t, env.Data)["seq"] != float64(4) {
		t.Errorf("message %s", env.Data)
	}

	env, r = e.runJSON("", "messages", "create", chat.ID, "Zelt kommt um 9", "--client-message-id", id)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "already") || len(chat.Messages) != 4 {
		t.Errorf("summary %q, %d messages", env.Summary, len(chat.Messages))
	}

	_, r = e.runJSON("", "messages", "create", chat.ID, "ohne Namen")
	expectCode(t, r, ExitOK)
	if _, sent := e.lastWriteBody()["clientMessageId"]; sent {
		t.Error("clientMessageId sent without --client-message-id")
	}
}

func TestMessagesCreateRefusesWhatIsNoMessage(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "messages", "create", e.fixture.Chat.ID, "   ")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "chatMessageMissing" {
		t.Errorf("meta %v", env.Meta)
	}

	_, r = e.runJSON("", "messages", "create", e.fixture.Chat.ID, "x", "--client-message-id", "gestern")
	expectCode(t, r, ExitUsage)
}

func TestAFailedPostCanBeSentAgainWithItsClientMessageID(t *testing.T) {
	e := newTestEnv(t)

	e.server.FailNextWith(http.StatusBadGateway)
	env, r := e.runJSON("", "messages", "create", e.fixture.Chat.ID, "x", "--client-message-id", "77777777-7777-4777-8777-777777777777")
	expectCode(t, r, ExitAPI)
	if !env.Retryable || !strings.Contains(env.Hint, "same --client-message-id") {
		t.Errorf("envelope %+v", env)
	}

	e.server.FailNextWith(http.StatusBadGateway)
	env, r = e.runJSON("", "messages", "create", e.fixture.Chat.ID, "x")
	expectCode(t, r, ExitAPI)
	if env.Retryable {
		t.Errorf("envelope %+v", env)
	}
}

func TestOnlyTheAuthorChangesOrRemovesAMessage(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	own, others := chat.Messages[0], chat.Messages[1]

	env, r := e.runJSON("", "messages", "edit", chat.ID, own.ID, "Hallo zusammen")
	expectCode(t, r, ExitOK)
	if message := decode(t, env.Data); message["bodyText"] != "Hallo zusammen" || message["editedAt"] == nil {
		t.Errorf("message %s", env.Data)
	}

	env, r = e.runJSON("", "messages", "edit", chat.ID, others.ID, "Nein")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "author") {
		t.Errorf("hint %q", env.Hint)
	}

	_, r = e.runJSON("", "messages", "delete", chat.ID, own.ID)
	expectCode(t, r, ExitOK)
	_, r = e.runJSON("", "messages", "delete", chat.ID, own.ID)
	expectCode(t, r, ExitOK)
	if !own.Removed {
		t.Error("own message not removed")
	}

	_, r = e.runJSON("", "messages", "delete", chat.ID, others.ID)
	expectCode(t, r, ExitNotFound)
	if others.Removed {
		t.Error("a message of somebody else was removed")
	}
}

func TestMarkReadUsesTheLastMessageUnlessToldOtherwise(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat

	_, r := e.runJSON("", "chats", "mark-read", chat.ID)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["seq"] != float64(3) {
		t.Errorf("body sent %v", body)
	}
	if mark := chat.ReadMarks[e.fixture.Bruno.ID]; mark != 3 {
		t.Errorf("read mark %d", mark)
	}

	e = newTestEnv(t)
	chat = e.fixture.Chat
	_, r = e.runJSON("", "chats", "mark-read", chat.ID, "--seq", "2")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["seq"] != float64(2) || chat.ReadMarks[e.fixture.Bruno.ID] != 2 {
		t.Errorf("body sent %v, mark %d", body, chat.ReadMarks[e.fixture.Bruno.ID])
	}
	if n := len(e.server.Requests()); n != 1 {
		t.Errorf("%d requests, want only the mark", n)
	}
}

func TestMessagesAsText(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	chat.Messages[2].Removed, chat.Messages[2].BodyText = true, ""

	r := e.run("", "messages", "list", chat.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"#2 Anke", "Zelt ist bestellt", "#3 Bruno", "(removed)"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestAMessageIsSentAsMarkdownTextOrHTML(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat

	for _, test := range []struct {
		args []string
		body string
	}{
		{[]string{"**Zelt** ist\nbestellt: https://example.com"}, `<p><strong>Zelt</strong> ist<br>bestellt: <a href="https://example.com">https://example.com</a></p>`},
		{[]string{"**as typed**\nsecond", "--format", "text"}, "**as typed**\nsecond"},
		{[]string{"<p>as <em>is</em></p>", "--format", "html"}, "<p>as <em>is</em></p>"},
	} {
		_, r := e.runJSON("", append([]string{"messages", "create", chat.ID}, test.args...)...)
		expectCode(t, r, ExitOK)
		if got := e.lastWriteBody()["body"]; got != test.body {
			t.Errorf("%v: body sent %q, want %q", test.args, got, test.body)
		}
	}

	_, r := e.runJSON("", "messages", "edit", chat.ID, chat.Messages[0].ID, "*neu*")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["body"]; got != "<p><em>neu</em></p>" {
		t.Errorf("edit sent %q", got)
	}
}

func TestAMessageInAnUnknownFormatOrWithNothingMinkaKeepsIsRefused(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat

	_, r := e.runJSON("", "messages", "create", chat.ID, "x", "--format", "rtf")
	expectCode(t, r, ExitUsage)
	_, r = e.runJSON("", "messages", "edit", chat.ID, chat.Messages[0].ID, "x", "--format", "rtf")
	expectCode(t, r, ExitUsage)
	_, r = e.runJSON("", "messages", "create", chat.ID, "[x]: https://example.com")
	expectCode(t, r, ExitUsage)
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}
