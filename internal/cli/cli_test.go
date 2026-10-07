package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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

	// claudeRan are the command lines of Claude Code a run would have run, but
	// for asking its version, claudeFails what each of them answers, and
	// claudeVersion what claude --version prints, a version that loads mods
	// where it is empty. No test runs the real one: it would install into the
	// Claude Code of whoever runs the tests.
	claudeRan     [][]string
	claudeFails   error
	claudeVersion string
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
// $MINKA_HOST and $MINKA_TOKEN, with a config file in a temporary directory
// and a home directory of its own, so that no test reads or writes the home
// of whoever runs it.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	server, fixture := fakeapi.New(t)
	return &testEnv{t: t, server: server, fixture: fixture, vars: map[string]string{
		"HOME":         t.TempDir(),
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
	ctx := context.Background()
	if len(args) > 0 && args[0] == "notifications" {
		// A wait whose guard broke would wait for ever; this ends it, so that
		// the test fails instead of hanging.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, waitDeadline)
		defer cancel()
	}
	a := newApp(ctx, strings.NewReader(stdin), &stdout, &stderr, func(name string) string { return e.vars[name] })
	a.runClaude = func(args []string) (string, error) {
		if len(args) == 1 && args[0] == "--version" {
			if e.claudeVersion == "" {
				return "2.1.289 (Claude Code)\n", e.claudeFails
			}
			return e.claudeVersion, e.claudeFails
		}
		e.claudeRan = append(e.claudeRan, args)
		return "", e.claudeFails
	}
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

// writeConfig puts a configuration file where the CLI of these tests reads one,
// with the permissions it insists on.
func (e *testEnv) writeConfig(content string) {
	e.t.Helper()
	path := e.vars["MINKA_CONFIG"]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
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

// A configuration written by hand can keep a host in a form no sign-in writes.
// The token under such an entry is never found, and a refusal that named only the
// host it looked for would name one that stands nowhere in the file and send
// somebody looking at the wrong end.
func TestAHostTheConfigurationWritesAnotherWayIsSaidSo(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	delete(e.vars, "MINKA_HOST")
	written := e.server.URL + "/"
	e.writeConfig(`{"defaultHost": "` + written + `", "hosts": {"` + written + `": {"token": "` + e.fixture.Token + `"}}}`)

	env, r := e.runJSON("", "me")
	expectCode(t, r, ExitAuth)
	if env.Code != CodeAuth {
		t.Errorf("code %q", env.Code)
	}
	if !strings.Contains(env.Hint, written) || !strings.Contains(env.Hint, "another way") {
		t.Errorf("the hint %q does not say that the configuration keeps the host in another form", env.Hint)
	}
	if !strings.Contains(env.Hint, "minka auth login --host "+e.server.URL) {
		t.Errorf("the hint %q does not say what puts it right", env.Hint)
	}
	// It names an entry of the file the token lies in, and an agent puts what it
	// reads into a log.
	if strings.Contains(env.Hint, e.fixture.Token) || strings.Contains(env.Error, e.fixture.Token) {
		t.Errorf("the refusal carries the token: %q %q", env.Error, env.Hint)
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

// The server refuses no text here: one that carries no element it serves is
// escaped and kept as the characters it consists of. The notice is the only
// word anybody gets about it, whichever text it was.
func TestHTMLWithoutMarkupSaysWhatBecomesOfItWhereverItIsSent(t *testing.T) {
	for name, sending := range map[string][]string{
		"a description": {"cards", "edit", "", "--description", "# Heading", "--description-format", "html"},
		"a comment":     {"comments", "create", "", "# Heading", "--format", "html"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			args := slices.Clone(sending)
			args[2] = e.fixture.CardInTriage.ID
			env, r := e.runJSON("", args...)
			expectCode(t, r, ExitOK)
			if !strings.Contains(env.Notice, "characters it consists of") {
				t.Fatalf("notice %q does not say that the text is kept as characters", env.Notice)
			}
		})
	}

	// A text that is nothing but an element the server serves draws no such
	// notice: it is kept as markup, and saying otherwise would send somebody
	// looking for a loss that never happened.
	e := newTestEnv(t)
	env, r := e.runJSON("", "cards", "edit", e.fixture.CardInTriage.ID, "--description", `<minka-embed url="https://example.com/x"></minka-embed>`, "--description-format", "html")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Fatalf("notice %q for a text the server keeps as markup", env.Notice)
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

// An agent is a person of its own, named after the human it acts for.
func TestAnAgentSaysWhomItActsFor(t *testing.T) {
	e := newTestEnv(t)
	e.vars["MINKA_TOKEN"] = "minka_pat_agent"

	env, r := e.runJSON("", "me")
	expectCode(t, r, ExitOK)
	var me api.Me
	if err := json.Unmarshal(env.Data, &me); err != nil {
		t.Fatal(err)
	}
	if me.Kind != "agent" || me.Principal == nil || me.Principal.Name != "Bruno" {
		t.Errorf("me %+v", me)
	}

	r = e.run("", "me", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"Kind: agent", "Acting for: Bruno"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "<>") {
		t.Errorf("an empty address in:\n%s", r.stdout)
	}

	// An agent that stepped down is in no list any more.
	env, r = e.runJSON("", "people", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	if strings.Contains(string(env.Data), e.fixture.RetiredAgent.ID) {
		t.Errorf("the retired agent is still listed: %s", env.Data)
	}

	// The people of a project say which of them is an agent, and for whom.
	env, r = e.runJSON("", "people", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	var people []api.ProjectMember
	if err := json.Unmarshal(env.Data, &people); err != nil {
		t.Fatal(err)
	}
	for _, person := range people {
		wantKind := "human"
		if person.ID == e.fixture.Agent.ID {
			wantKind = "agent"
		}
		if person.Kind != wantKind {
			t.Errorf("%s is a %q", person.Name, person.Kind)
		}
		if (person.Principal != nil) != (wantKind == "agent") {
			t.Errorf("principal of %s: %+v", person.Name, person.Principal)
		}
	}

	r = e.run("", "people", "list", e.fixture.Project.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "agent for Bruno") {
		t.Errorf("the agent is not named in:\n%s", r.stdout)
	}
}

// Naming somebody in a text needs markup only the server can make, and it
// comes with the people of the circle the text belongs to.
func TestThePeopleOfACircleCarryTheirMention(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "people", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	var members []api.ProjectMember
	if err := json.Unmarshal(env.Data, &members); err != nil {
		t.Fatal(err)
	}
	if len(members) == 0 {
		t.Fatalf("people %s", env.Data)
	}
	for _, member := range members {
		if member.Mention.Handle == "" || !strings.Contains(member.Mention.Markup, "@"+member.Name) ||
			!strings.Contains(member.Mention.Markup, `content-type="application/vnd.minka.person"`) {
			t.Errorf("mention of %s: %+v", member.Name, member.Mention)
		}
	}

	// Named in a comment, the markup travels to the server as it stands.
	anke := members[1]
	_, r = e.runJSON(anke.Mention.Markup+" schaust du?", "comments", "create", e.fixture.CardInTriage.ID, "-", "--format", "html")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody()["body"]; body != anke.Mention.Markup+" schaust du?" {
		t.Errorf("body sent %v", body)
	}

	// The two of a conversation carry mentions of their own conversation.
	env, r = e.runJSON("", "conversations", "people", e.fixture.Conversation.ID)
	expectCode(t, r, ExitOK)
	var partners []api.ProjectMember
	if err := json.Unmarshal(env.Data, &partners); err != nil {
		t.Fatal(err)
	}
	if len(partners) != 2 || !strings.Contains(partners[0].Mention.Handle, e.fixture.Conversation.ID) {
		t.Errorf("people of the conversation %s", env.Data)
	}

	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken
	_, r = e.runJSON("", "conversations", "people", e.fixture.Conversation.ID)
	expectCode(t, r, ExitNotFound)
}

// Reacting twice the same way changes nothing, and a reaction comes off
// whoever left it.
func TestAReactionIsLeftOnceAndComesOffAgain(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	comment := card.Comments[0]

	env, r := e.runJSON("", "comments", "react", card.ID, comment.ID, "🚀")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "Reacted 🚀") {
		t.Errorf("summary %q", env.Summary)
	}
	env, r = e.runJSON("", "comments", "react", card.ID, comment.ID, "🚀")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "already") {
		t.Errorf("summary of a second reaction %q", env.Summary)
	}
	if count := countOf(t, env.Data, "🚀"); count != 1 {
		t.Errorf("%d people behind one reaction of one person", count)
	}

	// Anke reacted 👍 first, then Bruno; the agent comes last.
	e.vars["MINKA_TOKEN"] = "minka_pat_agent"
	_, r = e.runJSON("", "comments", "react", card.ID, comment.ID, "👍")
	expectCode(t, r, ExitOK)
	e.vars["MINKA_TOKEN"] = e.fixture.Token

	// Taking one away takes your own, not the one that came last.
	_, r = e.runJSON("", "comments", "unreact", card.ID, comment.ID, "👍")
	expectCode(t, r, ExitOK)
	env, r = e.runJSON("", "cards", "show", card.ID)
	expectCode(t, r, ExitOK)
	var shown api.Card
	if err := json.Unmarshal(env.Data, &shown); err != nil {
		t.Fatal(err)
	}
	for _, reaction := range shown.Comments[0].Reactions {
		if reaction.Content != "👍" {
			continue
		}
		var names []string
		for _, person := range reaction.People {
			names = append(names, person.Name)
		}
		if strings.Join(names, ", ") != "Anke, "+e.fixture.Agent.Name {
			t.Errorf("👍 of %v, want Bruno gone and the others left", names)
		}
	}
	// Taking away what is not there succeeds.
	_, r = e.runJSON("", "comments", "unreact", card.ID, comment.ID, "🤷")
	expectCode(t, r, ExitOK)

	// The same on a message of a chat.
	message := e.fixture.Chat.Messages[0]
	env, r = e.runJSON("", "messages", "react", e.fixture.Chat.ID, message.ID, "👀")
	expectCode(t, r, ExitOK)
	if countOf(t, env.Data, "👀") != 1 {
		t.Errorf("message %s", env.Data)
	}
	_, r = e.runJSON("", "messages", "unreact", e.fixture.Chat.ID, message.ID, "👀")
	expectCode(t, r, ExitOK)
	if len(message.Reactions) != 0 {
		t.Errorf("reactions left: %+v", message.Reactions)
	}

	// A message of a private conversation takes a reaction the same way.
	private := e.fixture.Conversation.Messages[0]
	env, r = e.runJSON("", "messages", "react", e.fixture.Conversation.ID, private.ID, "👍")
	expectCode(t, r, ExitOK)
	if countOf(t, env.Data, "👍") != 1 {
		t.Errorf("message of a conversation %s", env.Data)
	}
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken
	_, r = e.runJSON("", "messages", "react", e.fixture.Conversation.ID, private.ID, "👎")
	expectCode(t, r, ExitNotFound)
	e.vars["MINKA_TOKEN"] = e.fixture.Token

	// A message of another chat of this person is as unreachable as one that
	// is not there.
	_, r = e.runJSON("", "messages", "react", e.fixture.Chat.ID, e.fixture.Conversation.Messages[0].ID, "👀")
	expectCode(t, r, ExitNotFound)
	_, r = e.runJSON("", "messages", "react", e.fixture.OtherChat.ID, message.ID, "👀")
	expectCode(t, r, ExitNotFound)
}

func TestAReactionIsShortAndSays_Something(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	comment := card.Comments[0]

	env, r := e.runJSON("", "comments", "react", card.ID, comment.ID, strings.Repeat("x", 21))
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "reactionTooLong" || !strings.Contains(env.Hint, "20 characters") {
		t.Errorf("envelope %+v", env)
	}

	env, r = e.runJSON("", "comments", "react", card.ID, comment.ID, "   ")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "reactionMissing" {
		t.Errorf("envelope %+v", env)
	}
}

// countOf is how many people stand behind one reaction of a comment or a
// message the server just answered with.
func countOf(t *testing.T, data json.RawMessage, content string) int {
	t.Helper()
	var text struct {
		Reactions []api.Reaction `json:"reactions"`
	}
	if err := json.Unmarshal(data, &text); err != nil {
		t.Fatalf("data carries no reactions: %v: %s", err, data)
	}
	for _, reaction := range text.Reactions {
		if reaction.Content == content {
			return len(reaction.People)
		}
	}
	return 0
}

// What people said about a comment or a message stands under it, counted
// together, and nothing stands there when nobody said anything.
func TestReactionsStandUnderACommentAndAMessage(t *testing.T) {
	e := newTestEnv(t)

	r := e.run("", "cards", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Reactions: 👍 Anke, Bruno; 🎉 Anke") {
		t.Errorf("reactions of the comment missing in:\n%s", r.stdout)
	}

	r = e.run("", "messages", "list", e.fixture.Chat.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Zelt ist bestellt\nReactions: 👍 Bruno") {
		t.Errorf("reactions of the message missing in:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout, "Hallo\nReactions:") {
		t.Errorf("a message nobody reacted to carries a line:\n%s", r.stdout)
	}

	// The JSON is the resource of the server, reactions and all.
	env, r := e.runJSON("", "cards", "show", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitOK)
	var card api.Card
	if err := json.Unmarshal(env.Data, &card); err != nil {
		t.Fatal(err)
	}
	reactions := card.Comments[0].Reactions
	if len(reactions) != 2 || reactions[0].Content != "👍" || len(reactions[0].People) != 2 || reactions[0].People[0].Name != "Anke" {
		t.Errorf("reactions %+v", reactions)
	}
}

// A private conversation is a chat without a project: it is not among the
// chats of a project, but its identifier reads and writes at the same place.
func TestAConversationIsAChatWithoutAProject(t *testing.T) {
	e := newTestEnv(t)
	conversation := e.fixture.Conversation

	env, r := e.runJSON("", "conversations", "list")
	expectCode(t, r, ExitOK)
	var conversations []api.Conversation
	if err := json.Unmarshal(env.Data, &conversations); err != nil {
		t.Fatalf("data is not a list of conversations: %v: %s", err, env.Data)
	}
	if len(conversations) != 1 {
		t.Fatalf("conversations %s", env.Data)
	}
	if first := conversations[0]; first.ID != conversation.ID || first.Name != e.fixture.Anke.Name ||
		first.Counterpart.ID != e.fixture.Anke.ID || first.UnreadCount != 1 ||
		first.LastMessageText == nil || *first.LastMessageText != "Hast du kurz Zeit?" {
		t.Errorf("conversation %+v", first)
	}

	// The chats of the project are the chats of the project.
	env, r = e.runJSON("", "chats", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	if strings.Contains(string(env.Data), conversation.ID) {
		t.Errorf("the conversation is among the chats of the project: %s", env.Data)
	}

	// Reading and writing go through the commands for messages.
	_, r = e.runJSON("", "messages", "create", conversation.ID, "Ja, gleich")
	expectCode(t, r, ExitOK)
	env, r = e.runJSON("", "messages", "list", conversation.ID)
	expectCode(t, r, ExitOK)
	if messages := decodeMessages(t, env.Data); len(messages) != 2 || messages[1].BodyText != "Ja, gleich" {
		t.Errorf("messages %s", env.Data)
	}

	// Somebody who is not one of the two finds nothing there.
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken
	_, r = e.runJSON("", "messages", "list", conversation.ID)
	expectCode(t, r, ExitNotFound)
}

func TestAConversationWithSomebodyIsBegunOnlyOnce(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "conversations", "with", e.fixture.Agent.ID)
	expectCode(t, r, ExitOK)
	begun := decode(t, env.Data)
	if begun["name"] != e.fixture.Agent.Name {
		t.Errorf("conversation %s", env.Data)
	}

	env, r = e.runJSON("", "conversations", "with", e.fixture.Agent.ID)
	expectCode(t, r, ExitOK)
	if again := decode(t, env.Data); again["id"] != begun["id"] {
		t.Errorf("a second conversation with the same person: %s", env.Data)
	}

	// A name is looked up among the members of a project, and only then.
	env, r = e.runJSON("", "conversations", "with", "anke@example.com")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "--project") {
		t.Errorf("error %q", env.Error)
	}
	env, r = e.runJSON("", "conversations", "with", "anke@example.com", "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	if withAnke := decode(t, env.Data); withAnke["id"] != e.fixture.Conversation.ID {
		t.Errorf("conversation %s", env.Data)
	}

	// Nobody, and oneself, are nobody to talk to.
	env, r = e.runJSON("", "conversations", "with", "00000000-0000-4000-8000-000000000000")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "organization") {
		t.Errorf("hint %q", env.Hint)
	}
	env, r = e.runJSON("", "conversations", "with", e.fixture.Bruno.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "yourself") {
		t.Errorf("hint %q", env.Hint)
	}
}

// The trash of a project says what was taken out of it, and what a long list
// leaves out.
func TestTheTrashListsWhatWasTakenOutOfAProject(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing

	env, r := e.runJSON("", "trash", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	if string(env.Data) != "[]" || env.Summary != "0 entries" {
		t.Errorf("an empty trash: %s, %q", env.Data, env.Summary)
	}

	_, r = e.runJSON("", "cards", "delete", card.ID)
	expectCode(t, r, ExitOK)
	env, r = e.runJSON("", "trash", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	var entries []api.TrashEntry
	if err := json.Unmarshal(env.Data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("trash %s", env.Data)
	}
	if entry := entries[0]; entry.Kind != "card" || entry.ID != card.ID || !entry.Restorable ||
		entry.Board == nil || entry.Board.Title != e.fixture.Board.Title ||
		entry.Column == nil || entry.Column.Title != "Doing" ||
		entry.TrashedBy == nil || entry.TrashedBy.Name != "Bruno" {
		t.Errorf("entry %+v", entries[0])
	}
	if env.Notice != "" {
		t.Errorf("notice %q", env.Notice)
	}

	r = e.run("", "trash", "list", e.fixture.Project.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"Review the API", "API-3 Abnahme / Doing", "by Bruno", "yes"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}

	// A trash longer than one answer says that it is.
	for range fakeapi.TrashEntries {
		e.server.Trash = append(e.server.Trash, &fakeapi.TrashedCard{
			Card: &fakeapi.Card{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", Title: "Older"}, Column: e.fixture.Doing,
		})
	}
	env, r = e.runJSON("", "trash", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	if err := json.Unmarshal(env.Data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != fakeapi.TrashEntries || !strings.Contains(env.Notice, "newest 200") {
		t.Errorf("%d entries, notice %q", len(entries), env.Notice)
	}

	// The trash of a project somebody is no member of is none of theirs.
	_, r = e.runJSON("", "trash", "list", e.fixture.OtherProject.ID)
	expectCode(t, r, ExitNotFound)
}

// A card in the trash comes back on top of the column it hung in, and only
// once: a second restore finds nothing there.
func TestACardComesBackFromTheTrashOnce(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing

	_, r := e.runJSON("", "cards", "delete", card.ID)
	expectCode(t, r, ExitOK)
	_, r = e.runJSON("", "cards", "show", card.ID)
	expectCode(t, r, ExitNotFound)

	env, r := e.runJSON("", "cards", "restore", card.ID)
	expectCode(t, r, ExitOK)
	if restored := decode(t, env.Data); restored["id"] != card.ID || decode(t, env.Data)["column"].(map[string]any)["title"] != "Doing" {
		t.Errorf("restored card %s", env.Data)
	}
	if first := e.fixture.Doing.Cards[0]; first != card {
		t.Errorf("the card did not come back on top: %s", first.Title)
	}

	env, r = e.runJSON("", "cards", "restore", card.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "trash") {
		t.Errorf("hint %q", env.Hint)
	}
	// A card in the trash of a project somebody is no member of stays there.
	e.server.Trash = append(e.server.Trash, &fakeapi.TrashedCard{Card: e.fixture.OtherCard, Column: e.fixture.OtherBoard.Columns[0]})
	_, r = e.runJSON("", "cards", "restore", e.fixture.OtherCard.ID)
	expectCode(t, r, ExitNotFound)
	if len(e.server.Trash) != 1 {
		t.Error("the card of the other project left the trash")
	}
}

// The cards somebody is responsible for come from every project at once, in
// the order of the page: by project, then by due date with the ones without a
// date last.
func TestMyCardsComeFromEveryProjectInTheOrderOfThePage(t *testing.T) {
	e := newTestEnv(t)
	fixture := e.fixture
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	fixture.CardInTriage.AssigneeIDs = []string{fixture.Bruno.ID}
	fixture.CardInTriage.DueOn = &tomorrow
	fixture.CardInDoing.AssigneeIDs = []string{fixture.Bruno.ID}
	fixture.CardInDoing.DueOn = &yesterday
	// A card without a due date stands behind every card that has one.
	undated := fixture.Doing.Cards[1]
	undated.AssigneeIDs = []string{fixture.Bruno.ID}
	// A card in Done and a card of somebody else are not somebody's work.
	done := &fakeapi.Card{ID: fixture.Done.ID + "-card", Title: "Done already", AssigneeIDs: []string{fixture.Bruno.ID}}
	fixture.Done.Cards = append(fixture.Done.Cards, done)
	fixture.OtherCard.AssigneeIDs = []string{fixture.Bruno.ID}

	env, r := e.runJSON("", "cards", "mine")
	expectCode(t, r, ExitOK)
	var cards []api.Card
	if err := json.Unmarshal(env.Data, &cards); err != nil {
		t.Fatalf("data is not a list of cards: %v: %s", err, env.Data)
	}
	var titles []string
	for _, card := range cards {
		titles = append(titles, card.Title)
	}
	if strings.Join(titles, ", ") != "Review the API, Write the CLI, "+undated.Title {
		t.Errorf("cards %v", titles)
	}
	if cards[0].Project.Name != fixture.Project.Name || cards[0].Board.Title != fixture.Board.Title {
		t.Errorf("card %+v", cards[0])
	}
	if env.Summary != "3 cards" {
		t.Errorf("summary %q", env.Summary)
	}
	if request := e.server.Requests()[0]; request.Path != "/api/v1/me/cards" || request.Query != "" {
		t.Errorf("request %s?%s", request.Path, request.Query)
	}

	r = e.run("", "cards", "mine", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"## Test-Projekt / API-3 Abnahme", "Review the API (Doing, due " + yesterday + ", overdue)", "Write the CLI (Triage, due " + tomorrow + ", 1 comment)"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

// A file travels as its own bytes, with what it is called in the query and
// what it is in the header, and comes back as the attachment for a text.
func TestAFileIsUploadedAsItIsAndComesBackAsAnAttachment(t *testing.T) {
	e := newTestEnv(t)
	content := onePixelPNG()
	path := filepath.Join(t.TempDir(), "plan.png")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "uploads", "create", path, "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	upload := decode(t, env.Data)
	if upload["filename"] != "plan.png" || upload["contentType"] != "image/png" || upload["kind"] != "image" {
		t.Errorf("upload %s", env.Data)
	}
	source, _ := upload["attachmentSource"].(string)
	if !strings.Contains(source, "<action-text-attachment ") || !strings.Contains(source, "sgid=") {
		t.Errorf("attachment source %q", source)
	}

	requests := e.server.Requests()
	if len(requests) != 1 {
		t.Fatalf("%d requests, want one", len(requests))
	}
	request := requests[0]
	if request.Query != "filename=plan.png" {
		t.Errorf("query %q", request.Query)
	}
	if got := request.Header.Get("Content-Type"); got != "image/png" {
		t.Errorf("content type %q", got)
	}
	// The length is announced, so that a file too large is refused before it
	// travels rather than after.
	if got := request.Header.Get("Content-Length"); got != strconv.Itoa(len(content)) {
		t.Errorf("content length %q, want %d", got, len(content))
	}
	if !bytes.Equal(request.Body, content) {
		t.Errorf("the bytes did not arrive as they are: %q", request.Body)
	}

	// What the name of a file says it is counts, even where its first bytes
	// look like something else.
	page := filepath.Join(filepath.Dir(path), "notes.txt")
	if err := os.WriteFile(page, []byte("<html><body>notes</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, r = e.runJSON("", "uploads", "create", page, "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	requests = e.server.Requests()
	if got := requests[len(requests)-1].Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("content type of a text file %q", got)
	}

	// The attachment goes into a text of the same project as it is.
	_, r = e.runJSON(source, "cards", "edit", e.fixture.CardInTriage.ID, "--description", "-", "--description-format", "html")
	expectCode(t, r, ExitOK)
	if description := e.fixture.CardInTriage.Description; !strings.Contains(description, "action-text-attachment") {
		t.Errorf("description %q", description)
	}
}

// A file of a conversation goes to the address of that conversation, and only
// the two people there may put one in.
func TestAFileGoesIntoTheCircleTheFlagNames(t *testing.T) {
	e := newTestEnv(t)
	path := filepath.Join(t.TempDir(), "plan.png")
	if err := os.WriteFile(path, onePixelPNG(), 0o600); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "uploads", "create", path, "--conversation", e.fixture.Conversation.ID)
	expectCode(t, r, ExitOK)
	source, _ := decode(t, env.Data)["attachmentSource"].(string)
	if !strings.Contains(source, "conversation:"+e.fixture.Conversation.ID) {
		t.Errorf("the attachment does not belong to the conversation: %q", source)
	}
	if request := e.server.Requests()[0]; request.Path != "/api/v1/messages/"+e.fixture.Conversation.ID+"/uploads" {
		t.Errorf("request %s", request.Path)
	}
	for _, crumb := range env.Breadcrumbs {
		if strings.Contains(crumb.Cmd, "cards") {
			t.Errorf("a conversation has no cards: %+v", crumb)
		}
	}

	// A file of a project carries a handle of that project, not of a talk.
	env, r = e.runJSON("", "uploads", "create", path, "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	if source, _ := decode(t, env.Data)["attachmentSource"].(string); strings.Contains(source, "conversation:") {
		t.Errorf("attachment of a project %q", source)
	}

	// Somebody outside the two finds nothing at that address.
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken
	env, r = e.runJSON("", "uploads", "create", path, "--conversation", e.fixture.Conversation.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "not in it") {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestAnUploadNamesOneCircleToGoInto(t *testing.T) {
	e := newTestEnv(t)
	path := filepath.Join(t.TempDir(), "plan.png")
	if err := os.WriteFile(path, onePixelPNG(), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{},
		{"--project", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", "--conversation", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f7"},
	} {
		env, r := e.runJSON("", append([]string{"uploads", "create", path}, args...)...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "--project or --conversation") {
			t.Errorf("error for %v: %q", args, env.Error)
		}
	}

	env, r := e.runJSON("", "uploads", "create", path, "--project", "not-an-id")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "not an ID") {
		t.Errorf("error %q", env.Error)
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests for uploads that name no circle", n)
	}
}

func TestAnUploadIsRefusedWhenTheFileOrItsTypeIsWrong(t *testing.T) {
	e := newTestEnv(t)
	dir := t.TempDir()
	png := filepath.Join(dir, "plan.png")
	if err := os.WriteFile(png, onePixelPNG(), 0o600); err != nil {
		t.Fatal(err)
	}
	// A name that promises a picture over words is one the server sees through.
	words := filepath.Join(dir, "notes.png")
	if err := os.WriteFile(words, []byte("plain words, no picture at all"), 0o600); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "uploads", "create", words, "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "contentTypeMismatch" || !strings.Contains(env.Hint, "--content-type") {
		t.Errorf("envelope %+v", env)
	}

	// Named for what it is, the same file goes through.
	_, r = e.runJSON("", "uploads", "create", words, "--project", e.fixture.Project.ID, "--content-type", "text/plain", "--name", "notes.txt")
	expectCode(t, r, ExitOK)
	if last := e.server.Requests()[len(e.server.Requests())-1]; last.Query != "filename=notes.txt" {
		t.Errorf("query %q", last.Query)
	}

	sent := len(e.server.Requests())
	env, r = e.runJSON("", "uploads", "create", filepath.Join(dir, "nothing.png"), "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "cannot read") {
		t.Errorf("a file that is not there: %q", env.Error)
	}
	env, r = e.runJSON("", "uploads", "create", dir, "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "is a directory, not a file") {
		t.Errorf("a directory: %q", env.Error)
	}
	// One byte over what Minka takes, and the bytes are never written: the
	// guard goes by the size the file says it has and reads none of it.
	tooBig := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(tooBig, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(tooBig, maxUploadBytes+1); err != nil {
		t.Fatal(err)
	}
	env, r = e.runJSON("", "uploads", "create", tooBig, "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "Minka takes at most") {
		t.Errorf("a file over the limit: %q", env.Error)
	}
	if now := len(e.server.Requests()); now != sent {
		t.Errorf("%d requests for files that cannot be sent", now-sent)
	}

	_, r = e.runJSON("", "uploads", "create", png, "--project", e.fixture.OtherProject.ID)
	expectCode(t, r, ExitNotFound)

	e.server.FailNextWith(http.StatusRequestEntityTooLarge)
	env, r = e.runJSON("", "uploads", "create", png, "--project", e.fixture.Project.ID)
	expectCode(t, r, ExitValidation)
	if env.Retryable {
		t.Errorf("envelope %+v", env)
	}
}

// What Minka takes for one file stands in three places: here, in the fake and
// in Minka's own uploads.maxBytes. A number that drifts apart here refuses
// files the server would have kept, which is what happened once already.
func TestTheSizeOfAFileIsTheOneMinkaTakes(t *testing.T) {
	const minkaTakes = 104857600
	if maxUploadBytes != minkaTakes {
		t.Errorf("the CLI takes %d bytes, Minka takes %d", maxUploadBytes, minkaTakes)
	}
	if fakeapi.MaxUploadBytes != minkaTakes {
		t.Errorf("the fake takes %d bytes, Minka takes %d", fakeapi.MaxUploadBytes, minkaTakes)
	}
}

// onePixelPNG is the smallest picture there is, so that a test has bytes that
// are what they say they are.
func onePixelPNG() []byte {
	content, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		panic(err)
	}
	return content
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
	for _, name := range []string{"minka auth login", "minka me", "minka projects list", "minka boards list", "minka boards show", "minka cards show", "minka cards create", "minka cards edit", "minka cards move", "minka cards done", "minka cards delete", "minka cards assign", "minka cards unassign", "minka people list", "minka steps list", "minka steps create", "minka steps complete", "minka steps reopen", "minka steps delete", "minka comments create", "minka chats list", "minka chats mark-read", "minka messages list", "minka messages create", "minka messages edit", "minka messages delete", "minka cards subscribe", "minka cards unsubscribe", "minka comments edit", "minka comments delete", "minka uploads create", "minka uploads download", "minka cards mine", "minka cards restore", "minka conversations list", "minka conversations with", "minka conversations people", "minka trash list", "minka comments react", "minka comments unreact", "minka messages react", "minka messages unreact", "minka columns on-hold"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("catalog misses %q", name)
		}
	}
	var moveFlags []string
	for _, flag := range byName["minka cards move"].Flags {
		moveFlags = append(moveFlags, flag.Name)
	}
	if strings.Join(moveFlags, ",") != "after,before,bottom,column,on-hold,top" {
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

// A host the CLI refuses is named in the refusal, and that refusal reaches an
// agent's transcript and its log. Whoever writes a password into the address
// keeps it: nothing of it stands in the output, whichever way the host was
// given and whichever way the CLI prints.
func TestARefusedHostKeepsItsPasswordOutOfTheOutput(t *testing.T) {
	const password = "s3cr3t-password"
	const withCredentials = "https://robert:" + password + "@minka.localbeach.net"
	e := newTestEnv(t)
	acceptedHost := e.vars["MINKA_HOST"]

	refusal := func(what string, r result) {
		t.Helper()
		expectCode(t, r, ExitUsage)
		out := r.stdout + r.stderr
		if strings.Contains(out, password) {
			t.Errorf("%s hands the password on: %s", what, out)
		}
		if !strings.Contains(out, "minka.localbeach.net") {
			t.Errorf("%s does not say which host it is about: %s", what, out)
		}
	}

	for _, mode := range []string{"--json", "--quiet", "--styled"} {
		e.vars["MINKA_HOST"] = acceptedHost
		refusal("--host with "+mode, e.run("", "me", mode, "--host", withCredentials))

		e.vars["MINKA_HOST"] = withCredentials
		refusal("$MINKA_HOST with "+mode, e.run("", "me", mode))
	}

	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests were sent", n)
	}
}

// A default host the configuration cannot read is no host: it is refused where
// it is read, and no command comes to a session under it. That is what keeps the
// value out of everything that prints a host — a status, a report, the hint of a
// refusal. What the refusal says of the value is the address alone, and it names
// the file to repair.
func TestADefaultHostThatCannotBeReadIsNoHost(t *testing.T) {
	const password = "s3cr3t-password"
	const withCredentials = "https://robert:" + password + "@minka.example.com"
	e := newTestEnv(t)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	// The dangerous shape: a token stands under the entry as it is written, so
	// nothing but the reading of the host stops a session from coming about.
	e.writeConfig(`{"defaultHost":"` + withCredentials + `","hosts":{"` + withCredentials + `":{"token":"minka_pat_written_by_hand"}}}`)

	// A session, and the store beside it, are the two ways into such an entry.
	for _, command := range [][]string{{"me"}, {"auth", "status"}, {"work", "list"}, {"work", "stop"}} {
		what := strings.Join(command, " ")
		env, r := e.runJSON("", append(command, "--json")...)
		expectCode(t, r, ExitUsage)
		if out := r.stdout + r.stderr; strings.Contains(out, password) {
			t.Errorf("%s hands the password on: %s", what, out)
		}
		if env.Code != CodeConfig {
			t.Errorf("%s answers with code %q", what, env.Code)
		}
		if !strings.Contains(env.Error, "the default host in "+e.vars["MINKA_CONFIG"]+" cannot be read") {
			t.Errorf("%s does not say that the default host of the configuration cannot be read: %s", what, env.Error)
		}
		if !strings.Contains(env.Error, "minka.example.com") {
			t.Errorf("%s does not say which host it is about: %s", what, env.Error)
		}
		if !strings.Contains(env.Hint, `"defaultHost"`) || !strings.Contains(env.Hint, "minka auth login --host https://minka.example.com") {
			t.Errorf("%s gives no way out: %s", what, env.Hint)
		}
	}

	// The report says it too, and never as a host that answers.
	reported, r := e.runJSON("", "doctor")
	expectCode(t, r, ExitOK)
	if out := r.stdout + r.stderr; strings.Contains(out, password) {
		t.Errorf("the report hands the password on: %s", out)
	}
	var report doctorReport
	if err := json.Unmarshal(reported.Data, &report); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sign-in", "host"} {
		if status := report.check(t, name).Status; status == checkOK {
			t.Errorf("the report calls %s fine", name)
		}
	}

	if n := len(e.server.Requests()); n != 0 {
		t.Errorf("%d requests were sent", n)
	}

	// Of an entry that cannot be read at all, what may be printed is too little
	// to sign in with, and a command line with the replacement in it would not
	// work. The hint then names an address to fill in.
	unreadable := newTestEnv(t)
	delete(unreadable.vars, "MINKA_HOST")
	delete(unreadable.vars, "MINKA_TOKEN")
	unreadable.writeConfig(`{"defaultHost":"https://robert:` + password + `@minka.example.com\u007f","hosts":{}}`)

	env, r := unreadable.runJSON("", "me")
	expectCode(t, r, ExitUsage)
	if out := r.stdout + r.stderr; strings.Contains(out, password) {
		t.Errorf("the refusal hands the password on: %s", out)
	}
	// What is left of such a value may hold anything, and a printed line goes
	// through a terminal and a log that read a control character as an order.
	if strings.ContainsRune(r.stdout+r.stderr, '\x7f') {
		t.Errorf("a control character of the value reached the output: %q", r.stdout+r.stderr)
	}
	if strings.Contains(env.Hint, "***") || !strings.Contains(env.Hint, "--host YOUR-INSTANCE-ADDRESS") {
		t.Errorf("the hint does not say what is to be filled in: %s", env.Hint)
	}
	// A line that looks ready to run would be run, by a person or by an agent,
	// and would reach a host that is not theirs.
	if strings.Contains(env.Hint, "--host https://") {
		t.Errorf("the hint names an address of its own: %s", env.Hint)
	}
}

// Signing out reaches the entry the file holds, and says so with the address
// alone.
func TestSigningOutOfAHandWrittenEntryPrintsNoPassword(t *testing.T) {
	const password = "s3cr3t-password"
	const withCredentials = "https://robert:" + password + "@minka.example.com"
	e := newTestEnv(t)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost":"` + withCredentials + `","hosts":{"` + withCredentials + `":{"token":"minka_pat_written_by_hand"}}}`)

	env, r := e.runJSON("", "auth", "logout")
	expectCode(t, r, ExitOK)
	if out := r.stdout + r.stderr; strings.Contains(out, password) {
		t.Errorf("signing out hands the password on: %s", out)
	}
	data := decode(t, env.Data)
	if data["host"] != "https://minka.example.com" || data["removed"] != true {
		t.Errorf("data of the sign-out: %v", data)
	}
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := file.Hosts[withCredentials]; ok {
		t.Error("the entry the file holds was not the one that was removed")
	}
}

// An entry nothing else can reach is still to be got rid of: no command lists the
// hosts of the configuration, and a token left standing there stays valid on the
// server. Signing out sends nothing anywhere, so it takes the entry as the file
// writes it, whether that can be read as a host or not.
func TestSigningOutOfAnEntryThatCannotBeRead(t *testing.T) {
	const secret = "minka_pat_in_the_wrong_place"
	const written = "https://minka.example.com;t=" + secret
	e := newTestEnv(t)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost":"` + e.server.URL + `","hosts":{` +
		`"` + e.server.URL + `":{"token":"minka_pat_signed_in"},` +
		`"` + written + `":{"token":"minka_pat_written_by_hand"}}}`)

	env, r := e.runJSON("", "auth", "logout", "--host", written)
	expectCode(t, r, ExitOK)
	if out := r.stdout + r.stderr; strings.Contains(out, secret) {
		t.Errorf("signing out hands on what stood behind the host: %s", out)
	}
	if data := decode(t, env.Data); data["host"] != "https://minka.example.com" || data["removed"] != true {
		t.Errorf("data of the sign-out: %v", data)
	}
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := file.Hosts[written]; ok {
		t.Error("the entry that cannot be read is still there")
	}
	if _, ok := file.Hosts[e.server.URL]; !ok {
		t.Error("the entry of another host was taken away with it")
	}

	// A value that stands nowhere in the file is a refusal all the same.
	_, r = e.runJSON("", "auth", "logout", "--host", "https://other.example.com;t=x")
	expectCode(t, r, ExitUsage)
}

// An entry written in a form a sign-in would not write is reached as it stands as
// well. Read first, a sign-out would look for the canonical spelling, find
// nothing and leave the token where it is — the very case the CLI itself teaches
// about. The other way round, with the canonical host named and the odd one in
// the file, the answer says which entry holds the token.
func TestSigningOutOfAnEntryWrittenAnotherWay(t *testing.T) {
	const written = "https://hand.example.com/"
	e := newTestEnv(t)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost":"` + e.server.URL + `","hosts":{` +
		`"` + e.server.URL + `":{"token":"minka_pat_signed_in"},` +
		`"` + written + `":{"token":"minka_pat_written_by_hand"}}}`)

	env, r := e.runJSON("", "auth", "logout", "--host", "https://hand.example.com")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["removed"] != false {
		t.Errorf("data of the sign-out: %v", data)
	}
	if !strings.Contains(env.Notice, written) {
		t.Errorf("the answer does not say which entry holds the token: %q", env.Notice)
	}

	env, r = e.runJSON("", "auth", "logout", "--host", written)
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["removed"] != true {
		t.Errorf("data of the sign-out: %v", data)
	}
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := file.Hosts[written]; ok {
		t.Error("the entry written another way is still there")
	}
	if _, ok := file.Hosts[e.server.URL]; !ok {
		t.Error("the entry of another host was taken away with it")
	}
}

// A request that cannot be made says which address it was reaching for, so that
// whoever reads the refusal knows what to look at.
func TestTheHintAfterAFailedRequestNamesWhatCouldNotBeReached(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	e := newTestEnv(t)
	e.vars["MINKA_HOST"] = "http://" + address

	env, r := e.runJSON("", "me")
	expectCode(t, r, ExitNetwork)
	if !strings.Contains(env.Hint, "127.0.0.1") {
		t.Errorf("the hint does not say what could not be reached: %s", env.Hint)
	}
}

// The error of a request has the address in it with the password replaced, the
// way the standard library writes one. The hint beside it renders the host and
// must not undo that in the same envelope. No host a session is built on carries
// a credential any more, so the hint is held to the rule here rather than through
// a run — and it appears after every failed request, not only where a sign-in is
// at stake.
func TestTheHintAfterAFailedRequestCarriesNoPassword(t *testing.T) {
	const password = "s3cr3t-password"
	failed := &api.NetworkError{
		Method: http.MethodGet,
		Err:    errors.New(`Get "http://robert:xxxxx@minka.example.com/api/v1/me": dial tcp: connection refused`),
	}

	e := apiError(failed, "http://robert:"+password+"@minka.example.com")
	if strings.Contains(e.Hint, password) {
		t.Errorf("the hint hands the password on: %s", e.Hint)
	}
	if !strings.Contains(e.Hint, "minka.example.com") {
		t.Errorf("the hint does not say what could not be reached: %s", e.Hint)
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
	// A client of the project is among them, and nothing says which one it is:
	// the API tells no client apart, so neither can a reader of this list.
	if got := strings.Join(names, ","); got != "Bruno,Anke,Claude (Bruno),Nele" {
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
	if err := json.Unmarshal(env.Data, &chats); err != nil || len(chats) != 1 || chats[0].Name != "Planung" || chats[0].UnreadCount != 1 {
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

// A message somebody takes back leaves the transcript. Only the answer to
// --after still names it, so that a client which has it drops it.
func TestARemovedMessageIsGoneFromTheTranscript(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	removed := chat.Messages[0]

	_, r := e.runJSON("", "messages", "delete", chat.ID, removed.ID)
	expectCode(t, r, ExitOK)

	for _, args := range [][]string{{}, {"--before", "3"}} {
		env, r := e.runJSON("", append([]string{"messages", "list", chat.ID}, args...)...)
		expectCode(t, r, ExitOK)
		for _, message := range decodeMessages(t, env.Data) {
			if message.ID == removed.ID {
				t.Errorf("messages list %v still carries the removed message: %s", args, env.Data)
			}
		}
	}

	env, r := e.runJSON("", "messages", "list", chat.ID, "--after", "0")
	expectCode(t, r, ExitOK)
	found := false
	for _, message := range decodeMessages(t, env.Data) {
		if message.ID == removed.ID {
			found = true
			if !message.Removed || message.BodyText != "" || message.BodyHTML != "" || message.BodySource != "" {
				t.Errorf("the removed message still carries something: %+v", message)
			}
		}
	}
	if !found {
		t.Errorf("--after does not carry the removed message: %s", env.Data)
	}

	r = e.run("", "messages", "list", chat.ID, "--after", "0", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "(removed)") {
		t.Errorf("the text does not mark the removed message:\n%s", r.stdout)
	}
}

func decodeMessages(t *testing.T, raw json.RawMessage) []api.ChatMessage {
	t.Helper()
	var messages []api.ChatMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatalf("data is not a list of messages: %v: %s", err, raw)
	}
	return messages
}

// unreadIn is what chats list says is unread in the chat of the fixture.
func unreadIn(t *testing.T, e *testEnv) int {
	t.Helper()
	env, r := e.runJSON("", "chats", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	var chats []api.Chat
	if err := json.Unmarshal(env.Data, &chats); err != nil || len(chats) != 1 {
		t.Fatalf("data %s", env.Data)
	}
	return chats[0].UnreadCount
}

// What one wrote oneself is nothing to come and read, and neither is a line
// an import brought or a message that was taken back.
func TestOnlyWhatSomebodyElseSaidIsUnread(t *testing.T) {
	e := newTestEnv(t)
	chat, conversation := e.fixture.Chat, e.fixture.Conversation

	_, r := e.runJSON("", "messages", "create", chat.ID, "Bin gleich da")
	expectCode(t, r, ExitOK)
	if unread := unreadIn(t, e); unread != 1 {
		t.Errorf("%d unread after writing oneself, want only the one of Anke", unread)
	}

	chat.Import(e.fixture.Anke.ID, "Aus dem alten Werkzeug", time.Date(2020, 1, 1, 9, 0, 0, 0, time.UTC))
	if unread := unreadIn(t, e); unread != 1 {
		t.Errorf("%d unread after an import, want 1", unread)
	}

	chat.Messages[1].Removed = true
	if unread := unreadIn(t, e); unread != 0 {
		t.Errorf("%d unread after the one of Anke was taken back, want 0", unread)
	}

	_, r = e.runJSON("", "messages", "create", conversation.ID, "Ja, gleich")
	expectCode(t, r, ExitOK)
	env, r := e.runJSON("", "conversations", "list")
	expectCode(t, r, ExitOK)
	var conversations []api.Conversation
	if err := json.Unmarshal(env.Data, &conversations); err != nil || len(conversations) != 1 || conversations[0].UnreadCount != 1 {
		t.Errorf("conversations %s, want the one word of Anke unread", env.Data)
	}
}

// aTranscriptWithLateLines leaves the chat of the fixture the way a second
// import run does: lines with new numbers standing between and in front of
// older ones. It answers with the seqs in the order of the transcript.
func aTranscriptWithLateLines(e *testEnv) []int {
	chat, anke := e.fixture.Chat, e.fixture.Anke.ID
	start := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	chat.Messages[0].CreatedAt = start
	chat.Messages[1].CreatedAt = start.Add(2 * time.Minute)
	chat.Messages[2].CreatedAt = start.Add(3 * time.Minute)
	chat.Import(anke, "Zwischen der ersten und der zweiten", start.Add(time.Minute))
	chat.Import(anke, "Vor allem anderen", start.Add(-time.Hour))
	chat.Import(anke, "Im selben Augenblick wie die zweite", start.Add(2*time.Minute))

	return []int{5, 1, 4, 2, 6, 3}
}

// beforeOf is the number the breadcrumb for reading back names.
func beforeOf(t *testing.T, env envelope) string {
	t.Helper()
	for _, crumb := range env.Breadcrumbs {
		if crumb.Action != "earlier" {
			continue
		}
		words := strings.Fields(crumb.Cmd)
		if n := len(words); n >= 2 && words[n-2] == "--before" {
			return words[n-1]
		}
	}
	t.Fatalf("no breadcrumb for reading back: %+v", env.Breadcrumbs)
	return ""
}

// A list stands in the order things were said. A seq names a message and is no
// place: the numbers down a page fall where an import brought a line later.
func TestATranscriptIsReadInTheOrderThingsWereSaid(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	aTranscriptWithLateLines(e)

	for _, test := range []struct {
		args []string
		seqs []int
	}{
		{nil, []int{5, 1, 4, 2, 6, 3}},
		{[]string{"--limit", "2"}, []int{6, 3}},
		// The cursor stands at the moment of its message, not at its number.
		{[]string{"--before", "2"}, []int{5, 1, 4}},
		{[]string{"--before", "4"}, []int{5, 1}},
		{[]string{"--before", "5"}, []int{}},
		{[]string{"--before", "6", "--limit", "2"}, []int{4, 2}},
		// A number no message carries is the place of the highest number
		// below it, and that message closes the page.
		{[]string{"--before", "99", "--limit", "3"}, []int{4, 2, 6}},
		// Catching up is cut off in the order of the changes and handed out
		// in the order of the transcript.
		{[]string{"--after", "3"}, []int{5, 4, 6}},
		{[]string{"--after", "1", "--limit", "3"}, []int{4, 2, 3}},
	} {
		env, r := e.runJSON("", append([]string{"messages", "list", chat.ID}, test.args...)...)
		expectCode(t, r, ExitOK)
		if got := messageSeqs(t, env.Data); !sameInts(got, test.seqs) {
			t.Errorf("%v: seqs %v, want %v", test.args, got, test.seqs)
		}
	}

	// A number that was taken back still names the moment its message was
	// said at, and what stood in front of it.
	chat.Messages[2].Removed = true
	env, r := e.runJSON("", "messages", "list", chat.ID, "--before", "3")
	expectCode(t, r, ExitOK)
	if got := messageSeqs(t, env.Data); !sameInts(got, []int{5, 1, 4, 2, 6}) {
		t.Errorf("before a removed message: seqs %v", got)
	}
}

// Whoever follows the breadcrumb back from the end reads every message once,
// wherever a page boundary falls among the late lines.
func TestReadingBackNeitherSkipsNorRepeatsAMessage(t *testing.T) {
	for _, pageLength := range []string{"1", "2", "3", "4"} {
		e := newTestEnv(t)
		chat := e.fixture.Chat
		whole := aTranscriptWithLateLines(e)

		env, r := e.runJSON("", "messages", "list", chat.ID, "--limit", pageLength)
		expectCode(t, r, ExitOK)
		read := messageSeqs(t, env.Data)
		for pages := 0; pages < 2*len(whole); pages++ {
			env, r = e.runJSON("", "messages", "list", chat.ID, "--limit", pageLength, "--before", beforeOf(t, env))
			expectCode(t, r, ExitOK)
			page := messageSeqs(t, env.Data)
			if len(page) == 0 {
				break
			}
			read = append(page, read...)
		}
		if !sameInts(read, whole) {
			t.Errorf("pages of %s read %v, want %v", pageLength, read, whole)
		}
	}
}

// The text shows a transcript in the order it came in: nothing on this side
// puts the messages back into the order of their numbers.
func TestMessagesAsTextKeepTheOrderOfTheTranscript(t *testing.T) {
	e := newTestEnv(t)
	aTranscriptWithLateLines(e)

	r := e.run("", "messages", "list", e.fixture.Chat.ID, "--styled")
	expectCode(t, r, ExitOK)
	at := -1
	for _, header := range []string{"#5 Anke", "#1 Bruno", "#4 Anke", "#2 Anke", "#6 Anke", "#3 Bruno"} {
		next := strings.Index(r.stdout, header)
		if next <= at {
			t.Fatalf("%q does not stand where it was said:\n%s", header, r.stdout)
		}
		at = next
	}
}

// A reader has read up to the highest number it has seen, which the last
// message of a list need not carry.
func TestReadingIsNotedByTheHighestNumberSeen(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	aTranscriptWithLateLines(e)
	// Somebody said this one by hand, so it waits to be read.
	chat.Messages[5].Imported = false

	env, r := e.runJSON("", "messages", "list", chat.ID)
	expectCode(t, r, ExitOK)
	read := ""
	for _, crumb := range env.Breadcrumbs {
		if crumb.Action == "read" {
			read = crumb.Cmd
		}
	}
	if want := "minka chats mark-read " + chat.ID + " --seq 6"; read != want {
		t.Errorf("breadcrumb %q, want %q", read, want)
	}

	_, r = e.runJSON("", "chats", "mark-read", chat.ID)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["seq"] != float64(6) {
		t.Errorf("body sent %v, want the highest number of the page", body)
	}
	if unread := unreadIn(t, e); unread != 0 {
		t.Errorf("%d unread after marking the chat read", unread)
	}
}

// The mark never passes what the chat has counted and never goes back.
func TestTheReadMarkHasACeilingAndOnlyGrows(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	bruno := e.fixture.Bruno.ID

	// A change takes a number of the same counter, so the ceiling lies above
	// the last message.
	_, r := e.runJSON("", "messages", "edit", chat.ID, chat.Messages[0].ID, "Hallo zusammen")
	expectCode(t, r, ExitOK)

	env, r := e.runJSON("", "chats", "mark-read", chat.ID, "--seq", "999")
	expectCode(t, r, ExitOK)
	if mark := chat.ReadMarks[bruno]; mark != 4 {
		t.Errorf("read mark %d, want the 4 the chat has counted", mark)
	}
	// The server does not say what it kept, so the summary says that the
	// number asked for may have been cut down.
	if !strings.Contains(env.Summary, "no more than the chat has counted") {
		t.Errorf("summary %q claims a mark the server may not keep", env.Summary)
	}

	env, r = e.runJSON("", "chats", "mark-read", chat.ID, "--seq", "2")
	expectCode(t, r, ExitOK)
	if mark := chat.ReadMarks[bruno]; mark != 4 {
		t.Errorf("read mark %d after an older number, want 4 still", mark)
	}
	if !strings.Contains(env.Summary, "never lowers the mark") {
		t.Errorf("summary %q claims a mark the server did not lower", env.Summary)
	}
}

// A chat numbers its messages and their changes with one counter, so a
// message written after a change leaves a number out.
func TestAMessageTakesTheNextNumberOfTheSharedCounter(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat

	_, r := e.runJSON("", "messages", "edit", chat.ID, chat.Messages[0].ID, "Hallo zusammen")
	expectCode(t, r, ExitOK)
	_, r = e.runJSON("", "messages", "create", chat.ID, "Bin gleich da")
	expectCode(t, r, ExitOK)

	env, r := e.runJSON("", "messages", "list", chat.ID)
	expectCode(t, r, ExitOK)
	if got := messageSeqs(t, env.Data); !sameInts(got, []int{1, 2, 3, 5}) {
		t.Errorf("seqs %v, want 1, 2, 3, 5", got)
	}
}

// A reaction given or taken away is a change of its message: catching up
// brings the message, and the counter of the chat has moved on.
func TestCatchingUpBringsAMessageSomebodyReactedTo(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	reactedTo := chat.Messages[0]

	changedAfter := func(after string) []int {
		t.Helper()
		env, r := e.runJSON("", "messages", "list", chat.ID, "--after", after)
		expectCode(t, r, ExitOK)
		return messageSeqs(t, env.Data)
	}

	_, r := e.runJSON("", "messages", "react", chat.ID, reactedTo.ID, "🎉")
	expectCode(t, r, ExitOK)
	if got := changedAfter("3"); !sameInts(got, []int{1}) || reactedTo.ChangeSeq != 4 {
		t.Errorf("after a reaction: seqs %v, changeSeq %d", got, reactedTo.ChangeSeq)
	}

	// The same reaction again changes nothing.
	_, r = e.runJSON("", "messages", "react", chat.ID, reactedTo.ID, "🎉")
	expectCode(t, r, ExitOK)
	if got := changedAfter("4"); len(got) != 0 {
		t.Errorf("after the same reaction again: seqs %v", got)
	}

	_, r = e.runJSON("", "messages", "unreact", chat.ID, reactedTo.ID, "🎉")
	expectCode(t, r, ExitOK)
	if got := changedAfter("4"); !sameInts(got, []int{1}) || reactedTo.ChangeSeq != 5 {
		t.Errorf("after the reaction was taken away: seqs %v, changeSeq %d", got, reactedTo.ChangeSeq)
	}

	// Taking away what nobody left changes nothing.
	_, r = e.runJSON("", "messages", "unreact", chat.ID, reactedTo.ID, "🎉")
	expectCode(t, r, ExitOK)
	if got := changedAfter("5"); len(got) != 0 {
		t.Errorf("after taking away nothing: seqs %v", got)
	}

	// The read mark may reach what the reactions counted.
	_, r = e.runJSON("", "chats", "mark-read", chat.ID, "--seq", "999")
	expectCode(t, r, ExitOK)
	if mark := chat.ReadMarks[e.fixture.Bruno.ID]; mark != 5 {
		t.Errorf("read mark %d, want 5", mark)
	}
}

// The last word of a conversation is the one said last, whatever its number.
func TestTheLastWordOfAConversationIsTheOneSaidLast(t *testing.T) {
	e := newTestEnv(t)
	conversation := e.fixture.Conversation

	_, r := e.runJSON("", "messages", "create", conversation.ID, "Ja, gleich")
	expectCode(t, r, ExitOK)
	conversation.Messages[0].CreatedAt = conversation.Messages[1].CreatedAt.Add(time.Hour)

	env, r := e.runJSON("", "conversations", "list")
	expectCode(t, r, ExitOK)
	var conversations []api.Conversation
	if err := json.Unmarshal(env.Data, &conversations); err != nil || len(conversations) != 1 ||
		conversations[0].LastMessageText == nil || *conversations[0].LastMessageText != "Hast du kurz Zeit?" {
		t.Errorf("conversations %s", env.Data)
	}
}

func TestMarkReadUsesTheHighestNumberUnlessToldOtherwise(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat

	env, r := e.runJSON("", "chats", "mark-read", chat.ID)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["seq"] != float64(3) {
		t.Errorf("body sent %v", body)
	}
	if want := "Marked chat " + chat.ID + " read up to message 3"; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
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
	// A file and a mention stand in the source, and in no other field.
	chat.Messages[0].BodyHTML = `<p>Hallo <action-text-attachment sgid="x" content-type="application/vnd.minka.person" content="@Anke"></action-text-attachment>, siehe <action-text-attachment sgid="y" content-type="image/png" url="https://minka.example/f/zelt.png" filename="zelt.png"></action-text-attachment></p>`
	chat.Messages[0].BodyText = "Hallo , siehe"

	r := e.run("", "messages", "list", chat.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"#2 Anke", "Zelt ist bestellt", "#3 Bruno", "@Anke", "![zelt.png](https://minka.example/f/zelt.png)"} {
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

func TestSubscribeAndUnsubscribeAPerson(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	bruno := e.fixture.Bruno

	env, r := e.runJSON("", "cards", "subscribe", card.ID, "me")
	expectCode(t, r, ExitOK)
	writes := e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodPost || last.Path != "/api/v1/cards/"+card.ID+"/subscribers/"+bruno.ID {
		t.Errorf("request %s %s", last.Method, last.Path)
	}
	var got api.Card
	if err := json.Unmarshal(env.Data, &got); err != nil || !hasPerson(got.Subscribers, bruno.ID) {
		t.Errorf("card %s", env.Data)
	}

	env, r = e.runJSON("", "cards", "subscribe", card.ID, "me")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "already") {
		t.Errorf("summary %q", env.Summary)
	}

	env, r = e.runJSON("", "cards", "unsubscribe", card.ID, "bruno@example.com")
	expectCode(t, r, ExitOK)
	writes = e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodDelete || last.Path != "/api/v1/cards/"+card.ID+"/subscribers/"+bruno.ID {
		t.Errorf("request %s %s", last.Method, last.Path)
	}
	if decode(t, env.Data)["wasSubscribed"] != true || len(card.SubscriberIDs) != 0 {
		t.Errorf("data %s, subscribers %v", env.Data, card.SubscriberIDs)
	}

	env, r = e.runJSON("", "cards", "unsubscribe", card.ID, "me")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["wasSubscribed"] != false {
		t.Errorf("data %s", env.Data)
	}
}

func TestSomebodyOutsideTheProjectIsNotSubscribed(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID

	env, r := e.runJSON("", "cards", "subscribe", card, e.fixture.Stranger.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "personNotAProjectMember" {
		t.Errorf("meta %v", env.Meta)
	}

	before := len(e.server.WritingRequests())
	_, r = e.runJSON("", "cards", "subscribe", card, "Stranger")
	expectCode(t, r, ExitUsage)
	if n := len(e.server.WritingRequests()); n != before {
		t.Errorf("%d requests written for a name nobody in the project has", n-before)
	}
}

func TestSubscribersAppearOnTheCard(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInTriage.SubscriberIDs = []string{e.fixture.Bruno.ID, e.fixture.Anke.ID}

	r := e.run("", "cards", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Subscribers: Anke, Bruno") {
		t.Errorf("card:\n%s", r.stdout)
	}
}

func TestSomebodyOnTheCardIsFoundByEmailAddress(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	card.AssigneeIDs = []string{e.fixture.Anke.ID}

	_, r := e.runJSON("", "cards", "unassign", card.ID, "anke@example.com")
	expectCode(t, r, ExitOK)
	if writes := e.server.WritingRequests(); len(writes) != 1 || !strings.HasSuffix(writes[0].Path, "/assignees/"+e.fixture.Anke.ID) {
		t.Errorf("requests %+v", writes)
	}
}

func TestADescriptionWithAttachmentsIsNotReplacedUnlessAsked(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	source := `<p>Plan</p><action-text-attachment sgid="abc" content-type="image/png" url="https://minka.example/f/1.png" filename="plan.png"></action-text-attachment>`
	card.Description = source

	env, r := e.runJSON("", "cards", "edit", card.ID, "--description", "new text")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") {
		t.Errorf("hint %q", env.Hint)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d writes sent", n)
	}

	_, r = e.runJSON("", "cards", "edit", card.ID, "--title", "only the title")
	expectCode(t, r, ExitOK)

	changed := `<p>Plan, changed</p><action-text-attachment sgid="abc" content-type="image/png" url="https://minka.example/f/1.png" filename="plan.png"></action-text-attachment>`
	env, r = e.runJSON(changed, "cards", "edit", card.ID, "--description", "-", "--description-format", "html")
	expectCode(t, r, ExitOK)
	if card.Description != changed || env.Notice != "" {
		t.Errorf("description %q, notice %q", card.Description, env.Notice)
	}

	r = e.run("", "cards", "show", card.ID, "--styled")
	if !strings.Contains(r.stdout, "![plan.png](https://minka.example/f/1.png)") {
		t.Errorf("card:\n%s", r.stdout)
	}

	env, r = e.runJSON("", "cards", "edit", card.ID, "--description", "new text", "--drop-attachments")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "attachments") || card.Description != "<p>new text</p>" {
		t.Errorf("notice %q, description %q", env.Notice, card.Description)
	}
}

func TestOnlyTheAuthorChangesOrRemovesAComment(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	own := card.Comments[0]
	others := &fakeapi.Comment{ID: "88888888-8888-4888-8888-888888888888", AuthorID: e.fixture.Anke.ID, Body: "<p>Ankes</p>"}
	card.Comments = append(card.Comments, others)

	env, r := e.runJSON("", "comments", "edit", card.ID, own.ID, "*geändert*\nzweite Zeile")
	expectCode(t, r, ExitOK)
	writes := e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodPatch || last.Path != "/api/v1/cards/"+card.ID+"/comments/"+own.ID {
		t.Errorf("request %s %s", last.Method, last.Path)
	}
	if body := e.lastWriteBody(); body["body"] != "<p><em>geändert</em><br>zweite Zeile</p>" {
		t.Errorf("body sent %v", body)
	}
	if comment := decode(t, env.Data); comment["editedAt"] == nil {
		t.Errorf("comment %s", env.Data)
	}

	env, r = e.runJSON("", "comments", "edit", card.ID, others.ID, "Nein")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "author") {
		t.Errorf("hint %q", env.Hint)
	}

	_, r = e.runJSON("", "comments", "delete", card.ID, own.ID)
	expectCode(t, r, ExitOK)
	_, r = e.runJSON("", "comments", "delete", card.ID, own.ID)
	expectCode(t, r, ExitNotFound)
	_, r = e.runJSON("", "comments", "delete", card.ID, others.ID)
	expectCode(t, r, ExitNotFound)
	if len(card.Comments) != 1 || card.Comments[0] != others {
		t.Errorf("comments left %v", card.Comments)
	}
}

func TestACommentWithAttachmentsIsNotReplacedUnlessAsked(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	own := card.Comments[0]
	own.Body = `<p>Hallo <action-text-attachment sgid="x" content-type="application/vnd.minka.person" content="@Anke"></action-text-attachment></p>`

	env, r := e.runJSON("", "comments", "edit", card.ID, own.ID, "Hallo Anke")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") {
		t.Errorf("hint %q", env.Hint)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Errorf("%d writes sent", n)
	}

	changed := `<p>Hallo nochmal <action-text-attachment sgid="x" content-type="application/vnd.minka.person" content="@Anke"></action-text-attachment></p>`
	_, r = e.runJSON(changed, "comments", "edit", card.ID, own.ID, "-", "--format", "html")
	expectCode(t, r, ExitOK)

	env, r = e.runJSON("", "comments", "edit", card.ID, own.ID, "Hallo", "--drop-attachments")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "attachments") {
		t.Errorf("notice %q", env.Notice)
	}
}

const (
	aVisibleReference = `<minka-ref kind="card" id="0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f0" title="Zelt *neu*" short-id="MNK-1" href="https://minka.example/go/card/MNK-1"></minka-ref>`
	aHiddenReference  = `<minka-ref kind="post" id="0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f1"></minka-ref>`
)

// A reference is shown by its address, while --json hands over the text
// exactly as the server served it, with the title and short identifier the
// view leaves out.
func TestAReferenceIsShownByItsAddressAndKeptAsServedInJSON(t *testing.T) {
	e := newTestEnv(t)
	served := "<p>See " + aVisibleReference + " and " + aHiddenReference + "</p>"
	e.fixture.CardInTriage.Description = served

	r := e.run("", "cards", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if want := "See <https://minka.example/go/card/MNK-1> and (a post you cannot open)"; !strings.Contains(r.stdout, want) {
		t.Errorf("styled output does not show %q:\n%s", want, r.stdout)
	}

	env, r := e.runJSON("", "cards", "show", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitOK)
	var card struct {
		Description       string `json:"description"`
		DescriptionSource string `json:"descriptionSource"`
	}
	if err := json.Unmarshal(env.Data, &card); err != nil {
		t.Fatal(err)
	}
	if card.Description != served || card.DescriptionSource != served {
		t.Errorf("JSON changed the text:\n description:       %q\n descriptionSource: %q", card.Description, card.DescriptionSource)
	}
	for _, attribute := range []string{`title="Zelt *neu*"`, `short-id="MNK-1"`, `href="https://minka.example/go/card/MNK-1"`} {
		if !strings.Contains(card.DescriptionSource, attribute) {
			t.Errorf("JSON lacks %s", attribute)
		}
	}
}

// The view of a text, edited and sent back, keeps a visible reference: its
// address is what Minka turns into the reference again. A hidden one has no
// address to be written by and counts as lost.
func TestTheGuardKeepsAReferenceByItsAddressAndNotAHiddenOne(t *testing.T) {
	for name, c := range map[string]struct {
		stored, markdown string
		kept             bool
	}{
		"visible, edited in its view": {"<p>See " + aVisibleReference + ".</p>", "See <https://minka.example/go/card/MNK-1>. More.", true},
		"visible, named as #MNK-1":    {"<p>See " + aVisibleReference + ".</p>", "See #MNK-1 now.", true},
		"visible, address in words":   {"<p>See " + aVisibleReference + ".</p>", "See https://minka.example/go/card/MNK-1.", true},
		"visible, given words":        {"<p>See " + aVisibleReference + ".</p>", "See [the card](https://minka.example/go/card/MNK-1).", false},
		"visible, dropped":            {"<p>See " + aVisibleReference + ".</p>", "Nothing to see.", false},
		"hidden, edited in its view":  {"<p>See " + aHiddenReference + "</p>", "See (a post you cannot open) now", false},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			card := e.fixture.CardInTriage
			card.Description = c.stored
			env, r := e.runJSON("", "cards", "edit", card.ID, "--description", c.markdown)
			if c.kept {
				expectCode(t, r, ExitOK)
				if env.Notice != "" {
					t.Errorf("notice %q", env.Notice)
				}
				return
			}
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Error, "references to other records") || strings.Contains(env.Error, "attachments") {
				t.Errorf("refusal %q does not name the references alone", env.Error)
			}
			if !strings.Contains(env.Hint, "--drop-attachments") || !strings.Contains(env.Hint, "<https://") {
				t.Errorf("hint %q", env.Hint)
			}
			if patchesIn(e.server.WritingRequests()) != 0 {
				t.Fatal("the description was sent")
			}
			env, r = e.runJSON("", "cards", "edit", card.ID, "--description", c.markdown, "--drop-attachments")
			expectCode(t, r, ExitOK)
			if env.Notice != "the references to other records of the text were dropped" {
				t.Errorf("notice %q", env.Notice)
			}
		})
	}
}

// A message is guarded like every other text, although no request reads it
// alone: the guard finds it in the transcript, also pages back.
func TestAMessageEditKeepsWhatMarkdownCannotHold(t *testing.T) {
	for name, c := range map[string]struct {
		stored, text, lost string
		olderMessages      int
		perPage            int
	}{
		"a hidden reference":                               {"<p>See " + aHiddenReference + "</p>", "See (a post you cannot open)", "references to other records", 0, 0},
		"a visible reference, dropped":                     {"<p>See " + aVisibleReference + "</p>", "See nothing", "references to other records", 0, 0},
		"a visible reference, kept":                        {"<p>See " + aVisibleReference + "</p>", "See <https://minka.example/go/card/MNK-1>", "", 0, 0},
		"a mention":                                        {"<p>" + aMention + " look</p>", "look", "attachments or mentions", 0, 0},
		"a mention and a reference":                        {"<p>" + aMention + aVisibleReference + "</p>", "look", "attachments or mentions and references to other records", 0, 0},
		"a mention two pages back":                         {"<p>" + aMention + " look</p>", "look", "attachments or mentions", 450, 0},
		"a mention behind pages smaller than asked for":    {"<p>" + aMention + " look</p>", "look", "attachments or mentions", 250, 100},
		"a reference kept in plain text":                   {"<p>See " + aVisibleReference + "</p>", "See https://minka.example/go/card/MNK-1", "", 0, 0},
		"a reference kept in plain text in angle brackets": {"<p>See " + aVisibleReference + "</p>", "See <https://minka.example/go/card/MNK-1> now", "", 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			chat := e.fixture.Chat
			own := chat.Messages[0]
			own.BodyHTML = c.stored
			e.server.MessagesPerPage = c.perPage
			for i := range c.olderMessages {
				chat.Messages = append(chat.Messages, &fakeapi.ChatMessage{
					ID: fmt.Sprintf("0192f3a4-5b6c-4d7e-8f90-%012d", i), Seq: 100 + i, ChangeSeq: 100 + i,
					AuthorID: e.fixture.Anke.ID, BodyText: "x", BodyHTML: "<p>x</p>", CreatedAt: own.CreatedAt.Add(time.Duration(i+1) * time.Second),
				})
			}
			format := []string{}
			if strings.Contains(name, "plain text") {
				format = []string{"--format", "text"}
			}
			env, r := e.runJSON("", slices.Concat([]string{"messages", "edit", chat.ID, own.ID, c.text}, format)...)
			if c.lost == "" {
				expectCode(t, r, ExitOK)
				if env.Notice != "" {
					t.Errorf("notice %q", env.Notice)
				}
				return
			}
			expectCode(t, r, ExitUsage)
			if want := "the message has " + c.lost + ", and the new text would drop them"; env.Error != want {
				t.Errorf("refusal %q, want %q", env.Error, want)
			}
			if patchesIn(e.server.WritingRequests()) != 0 {
				t.Fatal("the message was sent")
			}
			env, r = e.runJSON("", "messages", "edit", chat.ID, own.ID, c.text, "--drop-attachments")
			expectCode(t, r, ExitOK)
			if env.Notice != "the "+c.lost+" of the text were dropped" {
				t.Errorf("notice %q", env.Notice)
			}
		})
	}
}

// A transcript that cannot be read leaves the guard blind, so the edit stops
// there instead of going out and maybe dropping what it cannot see.
func TestAMessageEditStopsWhereTheTranscriptCannotBeRead(t *testing.T) {
	e := newTestEnv(t)
	chat := e.fixture.Chat
	own := chat.Messages[0]
	own.BodyHTML = "<p>See " + aHiddenReference + "</p>"
	e.server.FailNextWith(http.StatusBadGateway)
	_, r := e.runJSON("", "messages", "edit", chat.ID, own.ID, "See (a post you cannot open)", "--drop-attachments")
	expectCode(t, r, ExitAPI)
	if patchesIn(e.server.WritingRequests()) != 0 {
		t.Fatal("the message was sent although the guard could not read it")
	}
}
