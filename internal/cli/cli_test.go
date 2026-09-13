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
	description := "# Heading\n\n#### Deep\n\nSome *emphasis* and `code`."

	env, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "Neue Karte „mit“ Umlauten", "--description", description)
	expectCode(t, r, ExitOK)

	body := e.lastWriteBody()
	wantHTML := "<h1>Heading</h1>\n<h3>Deep</h3>\n<p>Some <em>emphasis</em> and <code>code</code>.</p>"
	if body["description"] != wantHTML {
		t.Errorf("description sent\n got: %q\nwant: %q", body["description"], wantHTML)
	}
	if body["title"] != "Neue Karte „mit“ Umlauten" {
		t.Errorf("title sent %q", body["title"])
	}
	if _, present := body["columnId"]; present {
		t.Error("columnId sent without --column")
	}
	if !strings.Contains(env.Notice, markdown.WarningHeadings) {
		t.Errorf("notice %q", env.Notice)
	}
	card := decode(t, env.Data)
	if card["column"].(map[string]any)["type"] != "triage" {
		t.Errorf("card not in Triage: %v", card["column"])
	}
}

func TestWarningsGoToStderrInTextOutput(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--description", "#### Deep", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stderr, "Warning: "+markdown.WarningHeadings) {
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

func TestCommentIsSentAsPlainText(t *testing.T) {
	e := newTestEnv(t)
	text := "**Not** <b>rendered</b> & kept"

	env, r := e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, text)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["body"] != text {
		t.Errorf("body sent %v", body)
	}
	if decode(t, env.Data)["body"] != text {
		t.Errorf("comment %s", env.Data)
	}
}

func TestCommentFromStdin(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("Zwei\nZeilen\n", "comments", "create", e.fixture.CardInTriage.ID, "-")
	expectCode(t, r, ExitOK)
	if got := e.lastWriteBody()["body"]; got != "Zwei\nZeilen\n" {
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
	for _, name := range []string{"minka auth login", "minka me", "minka projects list", "minka boards list", "minka boards show", "minka cards show", "minka cards create", "minka cards edit", "minka cards move", "minka cards done", "minka cards delete", "minka comments create"} {
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
	r := e.run("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "x", "--description", "#### Deep", "--quiet")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stderr, "Warning: "+markdown.WarningHeadings) {
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
