package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// newSetupEnv is a test environment nobody is signed in to, the way a machine
// is before its first setup.
func newSetupEnv(t *testing.T) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	return e
}

// onATerminal lets the CLI believe it talks to a person, who pastes token when
// asked for one.
func onATerminal(token string) func(*app) {
	return func(a *app) {
		a.stdinIsTerminal = func() bool { return true }
		a.stdoutIsTerminal = func() bool { return true }
		a.readSecret = func() (string, error) {
			if token == "" {
				return "", errors.New("nobody was to be asked for a token")
			}
			return token + "\n", nil
		}
	}
}

func TestSetupSignsInAndInstallsTheSkill(t *testing.T) {
	e := newSetupEnv(t)

	r := e.runWith(onATerminal(e.fixture.Token), e.server.URL+"\n\n", "setup")
	expectCode(t, r, ExitOK)

	info, err := os.Stat(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config file has permissions %04o, want 0600", info.Mode().Perm())
	}
	if _, err := os.Stat(e.installedSkillPath()); err != nil {
		t.Fatalf("the skill was not installed: %v", err)
	}
	if strings.Contains(r.stdout+r.stderr, e.fixture.Token) {
		t.Fatal("the token was printed")
	}

	status := e.run("", "auth", "status", "--json")
	expectCode(t, status, ExitOK)
}

func TestSetupInstallsNoSkillWithoutAYes(t *testing.T) {
	e := newSetupEnv(t)

	r := e.runWith(onATerminal(e.fixture.Token), e.server.URL+"\nn\n", "setup")
	expectCode(t, r, ExitOK)
	if _, err := os.Stat(e.installedSkillPath()); err == nil {
		t.Fatal("the skill was installed although the answer was no")
	}
}

func TestSetupNeedsATerminal(t *testing.T) {
	e := newSetupEnv(t)

	env, r := e.runJSON(e.server.URL+"\n\n", "setup")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "minka auth login") || !strings.Contains(env.Hint, "minka skill install") || !strings.Contains(env.Hint, "minka hooks install") {
		t.Fatalf("hint %q does not name the single steps", env.Hint)
	}
	if len(e.server.Requests()) != 0 {
		t.Fatal("a request was sent")
	}
	if _, err := os.Stat(e.installedSkillPath()); err == nil {
		t.Fatal("the skill was installed")
	}
}

// setupJSON runs setup on a terminal and reads the envelope it prints.
func (e *testEnv) setupJSON(stdin string, args ...string) (envelope, result) {
	e.t.Helper()
	r := e.runWith(onATerminal(e.fixture.Token), stdin, append([]string{"setup", "--json"}, args...)...)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\nstdout: %s\nstderr: %s", err, r.stdout, r.stderr)
	}
	return env, r
}

const settingsWithTheOldHook = `{"theme": "dark", "hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/opt/homebrew/bin/minka heartbeat", "timeout": 10}]}]}}`

// After signing in, setup installs the mod the way hooks install does, old
// hook and all.
func TestSetupInstallsTheModAndTakesOutTheOldHook(t *testing.T) {
	e := newSetupEnv(t)
	writeFile(t, e.claudeSettings(), settingsWithTheOldHook, 0o600)

	env, r := e.setupJSON(e.server.URL + "\nn\n")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if got := data["mod"].(map[string]any); got["state"] != modInstalled || got["path"] != e.modDir() {
		t.Errorf("setup says of the mod %v", got)
	}
	if hook, _ := data["oldHook"].(map[string]any); hook["removed"] != true {
		t.Errorf("setup says of the old hook %v", data["oldHook"])
	}
	if len(e.claudeRan) != 2 {
		t.Errorf("ran claude %v", e.claudeRan)
	}
	if got := readFile(t, e.claudeSettings()); got != `{"theme": "dark"}` {
		t.Errorf("the settings are now %s", got)
	}
	if !strings.Contains(r.stderr, "Installed the mod") || !strings.Contains(r.stderr, "Removed the old hook") {
		t.Errorf("setup does not say what it did: %s", r.stderr)
	}
}

// The safeguard: a machine without Claude Code on the PATH is set up all the
// same. Setup says so and how to install the mod later, signs in, exits 0, and
// leaves the old hook where it is, since nothing loads the mod.
func TestSetupGoesOnWithoutClaude(t *testing.T) {
	e := newSetupEnv(t)
	e.claudeFails = errNoClaude
	writeFile(t, e.claudeSettings(), settingsWithTheOldHook, 0o600)

	env, r := e.setupJSON(e.server.URL + "\nn\n")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if got := data["mod"].(map[string]any); got["state"] != modWritten || got["reason"] != errNoClaude.Error() {
		t.Errorf("setup says of the mod %v", got)
	}
	if data["oldHook"] != nil {
		t.Errorf("setup says of the old hook %v", data["oldHook"])
	}
	if !strings.Contains(r.stderr, "not on the PATH") || !strings.Contains(r.stderr, "minka hooks install") {
		t.Errorf("setup does not say that Claude Code is missing: %s", r.stderr)
	}
	if got := readFile(t, e.claudeSettings()); got != settingsWithTheOldHook {
		t.Errorf("the settings changed: %s", got)
	}
	expectCode(t, e.run("", "auth", "status", "--json"), ExitOK)
}

// Another failure of Claude Code does not stop setup either: the sign-in is
// done, and the lines to run by hand are on the screen.
func TestSetupGoesOnWhereClaudeCodeRefusesTheMod(t *testing.T) {
	e := newSetupEnv(t)
	e.claudeFails = errors.New("claude plugin install: exit status 1: no such marketplace")

	env, r := e.setupJSON(e.server.URL + "\nn\n")
	expectCode(t, r, ExitOK)
	if got := decode(t, env.Data)["mod"].(map[string]any); got["state"] != modWritten {
		t.Errorf("setup says of the mod %v", got)
	}
	if !strings.Contains(r.stderr, "claude plugin marketplace add "+e.modDir()) {
		t.Errorf("setup does not say what to run: %s", r.stderr)
	}
}

// The safeguard: --no-mod installs nothing — no file of the mod, no call of
// Claude Code — and leaves the settings of Claude Code alone.
func TestSetupWithNoModInstallsNothing(t *testing.T) {
	e := newSetupEnv(t)
	writeFile(t, e.claudeSettings(), settingsWithTheOldHook, 0o600)

	env, r := e.setupJSON(e.server.URL+"\nn\n", "--no-mod")
	expectCode(t, r, ExitOK)
	if got := decode(t, env.Data)["mod"].(map[string]any); got["state"] != modSkipped {
		t.Errorf("setup says of the mod %v", got)
	}
	if len(e.claudeRan) != 0 {
		t.Errorf("ran claude %v", e.claudeRan)
	}
	if _, err := os.Stat(e.modDir()); err == nil {
		t.Error("the mod was written")
	}
	if got := readFile(t, e.claudeSettings()); got != settingsWithTheOldHook {
		t.Errorf("the settings changed: %s", got)
	}
	if hook, _ := decode(t, env.Data)["oldHook"].(map[string]any); hook["removed"] != false {
		t.Errorf("setup says of the old hook %v", decode(t, env.Data)["oldHook"])
	}
	if !strings.Contains(env.Notice, "leftover") {
		t.Errorf("setup does not name the old hook: %q", env.Notice)
	}
}

// A Claude Code too old to load mods does not stop setup either; it says to
// update Claude Code and keeps the old hook.
func TestSetupGoesOnWithAClaudeCodeThatCannotLoadMods(t *testing.T) {
	e := newSetupEnv(t)
	e.claudeVersion = "2.1.211 (Claude Code)\n"
	writeFile(t, e.claudeSettings(), settingsWithTheOldHook, 0o600)

	env, r := e.setupJSON(e.server.URL + "\nn\n")
	expectCode(t, r, ExitOK)
	got := decode(t, env.Data)["mod"].(map[string]any)
	if reason, _ := got["reason"].(string); got["state"] != modWritten || !strings.Contains(reason, "cannot load mods") {
		t.Errorf("setup says of the mod %v", got)
	}
	if !strings.Contains(r.stderr, "update it") {
		t.Errorf("setup does not say to update Claude Code: %s", r.stderr)
	}
	if len(e.claudeRan) != 0 {
		t.Errorf("ran claude %v", e.claudeRan)
	}
	if got := readFile(t, e.claudeSettings()); got != settingsWithTheOldHook {
		t.Errorf("the settings changed: %s", got)
	}
}

// A mod that cannot be written does not stop setup: the sign-in is done.
func TestSetupGoesOnWhereTheModCannotBeWritten(t *testing.T) {
	e := newSetupEnv(t)
	writeFile(t, e.modDir(), "in the way", 0o600)

	env, r := e.setupJSON(e.server.URL + "\nn\n")
	expectCode(t, r, ExitOK)
	got := decode(t, env.Data)["mod"].(map[string]any)
	if got["state"] != modNotInstalled || got["reason"] == nil {
		t.Errorf("setup says of the mod %v", got)
	}
	if !strings.Contains(r.stderr, "could not be written") {
		t.Errorf("setup does not say so: %s", r.stderr)
	}
	expectCode(t, e.run("", "auth", "status", "--json"), ExitOK)
}

func TestSetupKeepsATokenThatWorks(t *testing.T) {
	e := newSetupEnv(t)
	first := e.runWith(onATerminal(e.fixture.Token), e.server.URL+"\nn\n", "setup")
	expectCode(t, first, ExitOK)

	// An empty answer takes the host signed in to last, and nobody is asked
	// for a token again.
	r := e.runWith(onATerminal(""), "\nn\n", "setup")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stderr, "Already signed in") {
		t.Fatalf("stderr does not say that the sign-in was kept: %s", r.stderr)
	}
}

func TestSetupSavesNoTokenTheHostRefuses(t *testing.T) {
	e := newSetupEnv(t)

	r := e.runWith(onATerminal("minka_pat_wrong"), e.server.URL+"\n\n", "setup")
	expectCode(t, r, ExitAuth)
	if _, err := os.Stat(e.vars["MINKA_CONFIG"]); err == nil {
		t.Fatal("a config file was written for a token the host refused")
	}
	if _, err := os.Stat(e.installedSkillPath()); err == nil {
		t.Fatal("setup went on after the sign-in failed")
	}
}

func TestSetupAsksAgainForAHostItCannotUse(t *testing.T) {
	e := newSetupEnv(t)

	r := e.runWith(onATerminal(e.fixture.Token), "http://example.com\n"+e.server.URL+"\nn\n", "setup")
	expectCode(t, r, ExitOK)
	if strings.Count(r.stderr, "Minka host") != 2 {
		t.Fatalf("the host was not asked for a second time: %s", r.stderr)
	}
}

// nobodyTypesAToken fails the test when setup asks for a token.
func nobodyTypesAToken(t *testing.T) func(*app) {
	return func(a *app) {
		onATerminal("")(a)
		a.readSecret = func() (string, error) {
			t.Error("setup asked for a token")
			return "", errors.New("nobody is there")
		}
	}
}

func TestSetupSendsASavedTokenToItsOwnHostOnly(t *testing.T) {
	e := newSetupEnv(t)
	first := e.runWith(onATerminal(e.fixture.Token), e.server.URL+"\nn\n", "setup")
	expectCode(t, first, ExitOK)

	other, _ := fakeapi.New(t)
	for token, person := range other.Tokens {
		delete(other.Tokens, token)
		other.Tokens["minka_pat_of_the_other_host"] = person
		break
	}

	r := e.runWith(onATerminal("minka_pat_of_the_other_host"), "n\n", "setup", "--host", other.URL)
	expectCode(t, r, ExitOK)
	for _, request := range other.Requests() {
		if strings.Contains(request.Header.Get("Authorization"), e.fixture.Token) {
			t.Fatal("the token of the first host was sent to the second")
		}
	}

	status, _ := e.runJSON("", "auth", "status")
	if got := decode(t, status.Data)["host"]; got != other.URL {
		t.Fatalf("default host %v, want the host set up last, %s", got, other.URL)
	}
}

func TestSetupMakesAHostItIsAlreadySignedInToTheDefault(t *testing.T) {
	e := newSetupEnv(t)
	other, otherFixture := fakeapi.New(t)
	expectCode(t, e.runWith(onATerminal(e.fixture.Token), "n\n", "setup", "--host", e.server.URL), ExitOK)
	expectCode(t, e.runWith(onATerminal(otherFixture.Token), "n\n", "setup", "--host", other.URL), ExitOK)

	r := e.runWith(nobodyTypesAToken(t), "n\n", "setup", "--host", e.server.URL)
	expectCode(t, r, ExitOK)

	status, _ := e.runJSON("", "auth", "status")
	if got := decode(t, status.Data)["host"]; got != e.server.URL {
		t.Fatalf("default host %v, want %s", got, e.server.URL)
	}
}

func TestSetupAsksForNoTokenWhenTheHostDoesNotAnswer(t *testing.T) {
	e := newSetupEnv(t)
	expectCode(t, e.runWith(onATerminal(e.fixture.Token), e.server.URL+"\nn\n", "setup"), ExitOK)
	e.server.Close()

	r := e.runWith(nobodyTypesAToken(t), "\nn\n", "setup")
	expectCode(t, r, ExitNetwork)
}

func TestSetupStopsAtAConfigFileItCannotUseBeforeAskingForAToken(t *testing.T) {
	e := newSetupEnv(t)
	path := e.vars["MINKA_CONFIG"]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hosts":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := e.runWith(nobodyTypesAToken(t), e.server.URL+"\n\n", "setup")
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != 0 {
		t.Fatal("a request was sent")
	}
}

func TestSetupKeepsTheTokenOutOfTheEnvelope(t *testing.T) {
	e := newSetupEnv(t)

	r := e.runWith(onATerminal(e.fixture.Token), e.server.URL+"\n\n", "setup", "--json")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout+r.stderr, e.fixture.Token) {
		t.Fatal("the token was printed")
	}
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		t.Fatalf("stdout is not one JSON envelope: %v\n%s", err, r.stdout)
	}
}

func TestSetupEndsWhenTheInputEnds(t *testing.T) {
	e := newSetupEnv(t)

	r := e.runWith(nobodyTypesAToken(t), "", "setup")
	expectCode(t, r, ExitUsage)
}

func TestSetupSaysNothingTwiceAboutADifferingSkill(t *testing.T) {
	e := newSetupEnv(t)
	e.writeInstalledSkill("an older skill")

	r := e.runWith(onATerminal(e.fixture.Token), e.server.URL+"\nn\n", "setup")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stderr, "Warning:") {
		t.Fatalf("setup warns about the skill it just asked about: %s", r.stderr)
	}
}

// setup is what repairs a configuration by hand, so it has to walk over a default
// host that cannot be read. It neither prints that value in its question — where
// a password written into it would reach a transcript — nor offers it as the
// answer, which taking would only be refused.
func TestSetupOffersNoDefaultHostThatCannotBeRead(t *testing.T) {
	const password = "s3cr3t-password"
	e := newSetupEnv(t)
	e.writeConfig(`{"defaultHost":"https://robert:` + password + `@minka.example.com","hosts":{}}`)

	// An empty answer first, which has nothing to fall back on here.
	r := e.runWith(onATerminal(e.fixture.Token), "\n"+e.server.URL+"\n\n", "setup")
	expectCode(t, r, ExitOK)
	out := r.stdout + r.stderr
	if strings.Contains(out, password) {
		t.Errorf("setup printed the password: %s", out)
	}
	if !strings.Contains(out, "Minka host, such as https://minka.example.com: ") {
		t.Errorf("setup offered an answer it cannot take: %s", out)
	}
	expectCode(t, e.run("", "auth", "status", "--json"), ExitOK)
}
