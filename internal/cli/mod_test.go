package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/work"
	"github.com/flownative/minka-cli/mod"
)

// The mod of a Claude Code process names its session with --session: the
// commands it runs are not given $CLAUDE_CODE_SESSION_ID.
const sessionOfTheMod = "the-session-of-the-mod"

// beatOfTheMod is one beat as the mod sends it: after a step, or while a turn is
// under way.
func (e *testEnv) beatOfTheMod(at time.Time) map[string]any {
	e.t.Helper()
	env, r := e.runWithJSON(at, "heartbeat", "--session", sessionOfTheMod, "--from-mod")
	expectCode(e.t, r, ExitOK)
	return decode(e.t, env.Data)
}

// idleBeatOfTheMod is a beat of the mod while no turn is under way.
func (e *testEnv) idleBeatOfTheMod(at time.Time) map[string]any {
	e.t.Helper()
	env, r := e.runWithJSON(at, "heartbeat", "--session", sessionOfTheMod, "--from-mod", "--idle")
	expectCode(e.t, r, ExitOK)
	return decode(e.t, env.Data)
}

// startWorkInTheSessionOfTheMod takes a card up the way a session with the mod
// does: typed in its Bash tool, which carries the session in the environment,
// and followed by the beat the mod sends after the tool call.
func (e *testEnv) startWorkInTheSessionOfTheMod(at time.Time) {
	e.t.Helper()
	e.vars[sessionVariable] = sessionOfTheMod
	expectCode(e.t, e.runAt(at, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	delete(e.vars, sessionVariable)
	e.beatOfTheMod(at)
}

// listOfTheMod is what the store holds for the session of the mod.
func (e *testEnv) listOfTheMod() map[string]any {
	e.t.Helper()
	e.vars[sessionVariable] = sessionOfTheMod
	defer delete(e.vars, sessionVariable)
	return e.storedList()
}

// The safeguard, and what the mod is for: a session whose process runs the mod
// goes dark three minutes after its process ended. The mod beats every thirty
// seconds and then stops; a keepalive of the host, which another session's
// hook may have started, ticks on through the twenty-five minutes it would
// keep a session alight, and not one of its ticks reports the session of the
// mod. So the last signal of that session is no later than its last beat, and
// the board forgets the card three minutes after that.
func TestASessionOfTheModGoesDarkThreeMinutesAfterItsLastBeat(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.startWorkInTheSessionOfTheMod(began)
	lastBeat := began
	for at := began; at.Before(began.Add(10 * time.Minute)); at = at.Add(30 * time.Second) {
		e.beatOfTheMod(at)
		lastBeat = at
	}
	lastSent, err := time.Parse(time.RFC3339, e.listOfTheMod()["sentAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	sent := len(e.signals())

	var endings []string
	for at := lastBeat.Add(30 * time.Second); at.Before(lastBeat.Add(nobodyIsAtWorkAfter)); at = at.Add(30 * time.Second) {
		env, r := e.runKeepaliveThatIsToldToStop(at)
		expectCode(t, r, ExitOK)
		if ended, _ := decode(t, env.Data)["ended"].(string); !strings.Contains(ended, "mod") {
			endings = append(endings, at.Format(time.TimeOnly)+": "+ended)
		}
	}
	if got := len(e.signals()); got != sent {
		t.Errorf("%d signals after the last beat of the mod, want none", got-sent)
	}
	if len(endings) > 0 {
		t.Errorf("the keepalive ended for other reasons than the mod reporting: %v", endings[0])
	}
	// A keepalive that is told to stop has its requests cut off, so what it
	// set out to send is what it claimed in the store before the request.
	if got := e.listOfTheMod()["sentAt"]; got != asStored(lastSent) {
		t.Errorf("the keepalive claimed a signal of the session at %v", got)
	}
	if dark := lastSent.Add(work.HeldFor); dark.After(lastBeat.Add(work.HeldFor)) {
		t.Errorf("the board forgets the card at %s, later than three minutes after the last beat at %s", dark, lastBeat)
	}
}

// The same safeguard where the store changes under a tick: a signal of the
// keepalive for a list of the mod goes nowhere, whatever the keepalive read a
// moment before.
func TestTheKeepaliveSignalsNothingForASessionOfTheMod(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.startWorkInTheSessionOfTheMod(began)
	sent := len(e.signals())

	beat := e.signalAsTheKeepalive(began.Add(2*time.Minute), sessionOfTheMod)
	if beat.sent || beat.reason != theModReports {
		t.Errorf("the keepalive signalled %+v, want nothing for the mod's session", beat)
	}
	if got := len(e.signals()); got != sent {
		t.Errorf("%d signals went out", got-sent)
	}
}

// A beat of the mod marks the list as one the mod reports and starts no
// keepalive: the process that beats is the one thing that knows it is alive.
func TestABeatOfTheModStartsNoKeepalive(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.vars[sessionVariable] = sessionOfTheMod
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	delete(e.vars, sessionVariable)

	data := e.beatOfTheMod(began)
	if data["sent"] != true || data["session"] != sessionOfTheMod {
		t.Fatalf("the beat said %v", data)
	}
	if data["keepalive"] != notStarted(theModReports) {
		t.Errorf("the keepalive %v", data["keepalive"])
	}
	if started := e.keepalives.Load(); started != 0 {
		t.Errorf("%d keepalives were started", started)
	}
	if e.listOfTheMod()["mod"] != true {
		t.Errorf("the list is not marked as the mod's: %v", e.listOfTheMod())
	}
}

// Where the hook still runs beside the mod, its beat starts no keepalive for
// the session either: the keepalive would leave the list alone and end at once,
// and the next tool call would start the next one.
func TestABeatOfTheHookInASessionOfTheModStartsNoKeepalive(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.startWorkInTheSessionOfTheMod(began)

	e.vars[sessionVariable] = sessionOfTheMod
	env, r := e.runWithJSON(began.Add(time.Minute), "heartbeat")
	expectCode(t, r, ExitOK)
	if became := decode(t, env.Data)["keepalive"]; became != notStarted(theModReports) {
		t.Errorf("the keepalive %v", became)
	}
	if started := e.keepalives.Load(); started != 0 {
		t.Errorf("%d keepalives were started", started)
	}
}

// A beat while no turn is under way keeps the cards alight for as long as a
// keepalive would after the last step, and says nothing about the work: a
// Claude Code left open over night does not keep its cards glowing until the
// morning, nor keep them past the working day a card is kept for.
func TestAnIdleBeatKeepsTheCardsOnlyAsLongAsTheKeepaliveWould(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.startWorkInTheSessionOfTheMod(began)
	list := e.listOfTheMod()
	beatAt, seenAt := list["beatAt"], list["cards"].([]any)[0].(map[string]any)["seenAt"]

	lastSent := began
	for at := began.Add(30 * time.Second); at.Before(began.Add(nobodyIsAtWorkAfter)); at = at.Add(30 * time.Second) {
		if data := e.idleBeatOfTheMod(at); data["sent"] == true {
			lastSent = at
		}
	}
	if lastSent.Before(began.Add(nobodyIsAtWorkAfter - 2*time.Minute)) {
		t.Errorf("idle beats reported last at %s, want them to keep the card alight for %s", lastSent, nobodyIsAtWorkAfter)
	}
	list = e.listOfTheMod()
	if list["beatAt"] != beatAt || list["cards"].([]any)[0].(map[string]any)["seenAt"] != seenAt {
		t.Errorf("an idle beat said somebody is at work: %v", list)
	}

	sent := len(e.signals())
	for at := began.Add(nobodyIsAtWorkAfter); at.Before(began.Add(3 * time.Hour)); at = at.Add(30 * time.Second) {
		e.idleBeatOfTheMod(at)
	}
	if got := len(e.signals()); got != sent {
		t.Errorf("%d idle beats reported after nobody was at work for %s", got-sent, nobodyIsAtWorkAfter)
	}
	if data := e.idleBeatOfTheMod(began.Add(3 * time.Hour)); !strings.Contains(data["reason"].(string), "nobody has been at work") {
		t.Errorf("the idle beat says %q", data["reason"])
	}

	// The next step is work again, and lights the card at once.
	if data := e.beatOfTheMod(began.Add(3 * time.Hour)); data["sent"] != true {
		t.Errorf("the next step sent nothing: %v", data)
	}
}

// An idle beat does not light again what the server forgot while nobody was at
// work: the last signal is older than the three minutes the server counts it
// for, and nobody has been at the store since, though that is well within the
// twenty-five minutes an idle session is kept alight. Only a step does.
func TestAnIdleBeatDoesNotRelightWhatTheServerForgot(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.startWorkInTheSessionOfTheMod(began)
	sent := len(e.signals())

	data := e.idleBeatOfTheMod(began.Add(work.HeldFor + time.Minute))
	if data["sent"] != false || !strings.Contains(data["reason"].(string), "forgotten") {
		t.Errorf("the idle beat said %v", data)
	}
	if got := len(e.signals()); got != sent {
		t.Errorf("%d signals went out", got-sent)
	}
	if data := e.beatOfTheMod(began.Add(work.HeldFor + time.Minute)); data["sent"] != true {
		t.Errorf("the next step sent nothing: %v", data)
	}
}

// The mod reports one session; a beat that names none reports nothing, so
// that the list of no session is never kept alight from inside a process.
func TestABeatOfTheModWithoutASessionReportsNothing(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	env, r := e.runWithJSON(began, "heartbeat", "--from-mod")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["sent"] != false || data["reason"] != "the mod names no session" {
		t.Errorf("the beat said %v", data)
	}
	if len(e.signals()) != 0 {
		t.Error("a signal went out")
	}
}

func TestInstallingTheModWritesItAndTellsClaudeCode(t *testing.T) {
	e := newTestEnv(t)
	dir := filepath.Join(filepath.Dir(e.vars["MINKA_CONFIG"]), "claude-mod")

	env, r := e.runJSON("", "hooks", "install", "--mod")
	expectCode(t, r, ExitOK)
	written := decode(t, env.Data)["mod"].(map[string]any)
	if written["path"] != dir || written["state"] != modInstalled || written["plugin"] != mod.Plugin {
		t.Errorf("the install said %v", written)
	}
	want := [][]string{
		{"plugin", "marketplace", "add", dir},
		{"plugin", "install", mod.Plugin, "--scope", "user"},
	}
	if !slices.EqualFunc(e.claudeRan, want, slices.Equal) {
		t.Errorf("ran claude %v, want %v", e.claudeRan, want)
	}

	module, err := os.ReadFile(filepath.Join(dir, "minka-heartbeat", "hooks", "register.js"))
	if err != nil {
		t.Fatal(err)
	}
	program, _ := os.Executable()
	if !strings.Contains(string(module), `const minka = "`+program+`"`) {
		t.Errorf("the mod does not run this minka, %s", program)
	}

	env, r = e.runJSON("", "hooks", "install", "--mod")
	expectCode(t, r, ExitOK)
	if state := decode(t, env.Data)["mod"].(map[string]any)["state"]; state != modCurrent {
		t.Errorf("a second install says %v", state)
	}
	if len(e.claudeRan) != 4 {
		t.Errorf("a second install did not tell Claude Code again: %v", e.claudeRan)
	}
}

// Where Claude Code cannot be told, the mod is written all the same and the
// hint carries what to run by hand.
func TestInstallingTheModWithoutClaudeSaysWhatToRun(t *testing.T) {
	e := newTestEnv(t)
	e.claudeFails = errors.New("claude is not on the PATH")
	dir := filepath.Join(filepath.Dir(e.vars["MINKA_CONFIG"]), "claude-mod")

	env, r := e.runJSON("", "hooks", "install", "--mod")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "claude plugin marketplace add "+dir) || !strings.Contains(env.Hint, "claude plugin install "+mod.Plugin) {
		t.Errorf("the hint is %q", env.Hint)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude-plugin", "marketplace.json")); err != nil {
		t.Errorf("the mod was not written: %v", err)
	}
}

func TestTheModTakesNeitherAFileNorACommand(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "hooks", "install", "--mod", "--file", filepath.Join(t.TempDir(), "s.json"), "--json"), ExitUsage)
	expectCode(t, e.run("", "hooks", "install", "--mod", "--command", "minka", "--json"), ExitUsage)
	if len(e.claudeRan) != 0 {
		t.Errorf("ran claude %v", e.claudeRan)
	}
}

// The hook may stay beside the mod, and the install says that it may go.
func TestInstallingTheModNamesTheHookBesideIt(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "hooks", "install", "--json"), ExitOK)

	env, r := e.runJSON("", "hooks", "install", "--mod")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "the mod does all it did") {
		t.Errorf("the notice is %q", env.Notice)
	}
}

// The status says whether the mod is written as this minka writes it and
// whether the settings tell Claude Code to load it.
func TestTheStatusSaysWhatBecameOfTheMod(t *testing.T) {
	e := newTestEnv(t)
	settings := filepath.Join(e.vars["HOME"], ".claude", "settings.json")

	modOf := func() map[string]any {
		env, r := e.runJSON("", "hooks", "status")
		expectCode(t, r, ExitOK)
		return decode(t, env.Data)["mod"].(map[string]any)
	}
	if got := modOf(); got["state"] != modNotInstalled || got["enabled"] != false {
		t.Errorf("before the install: %v", got)
	}

	expectCode(t, e.run("", "hooks", "install", "--mod", "--json"), ExitOK)
	if got := modOf(); got["state"] != modCurrent || got["enabled"] != false {
		t.Errorf("written, and nothing in the settings: %v", got)
	}

	if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"enabledPlugins": {"`+mod.Plugin+`": true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := modOf(); got["state"] != modCurrent || got["enabled"] != true {
		t.Errorf("written and enabled: %v", got)
	}

	module := filepath.Join(filepath.Dir(e.vars["MINKA_CONFIG"]), "claude-mod", "minka-heartbeat", "hooks", "register.js")
	if err := os.WriteFile(module, []byte("const minka = '/gone/minka'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := modOf(); got["state"] != modStale {
		t.Errorf("written by another minka: %v", got)
	}
}

// Claude Code reads its settings from $CLAUDE_CONFIG_DIR where that is set, and
// so do the status and the install: a status that looked into ~/.claude would
// call a mod Claude Code loads one it does not.
func TestTheSettingsAreReadWhereClaudeConfigDirPoints(t *testing.T) {
	e := newTestEnv(t)
	e.vars["CLAUDE_CONFIG_DIR"] = filepath.Join(t.TempDir(), "claude")
	settings := filepath.Join(e.vars["CLAUDE_CONFIG_DIR"], "settings.json")
	expectCode(t, e.run("", "hooks", "install", "--json"), ExitOK)
	content, err := os.ReadFile(settings)
	if err != nil {
		t.Fatalf("the hook is not in %s: %v", settings, err)
	}
	enabled := strings.Replace(string(content), "{", `{"enabledPlugins": {"`+mod.Plugin+`": true},`, 1)
	if err := os.WriteFile(settings, []byte(enabled), 0o600); err != nil {
		t.Fatal(err)
	}
	expectCode(t, e.run("", "hooks", "install", "--mod", "--json"), ExitOK)

	env, r := e.runJSON("", "hooks", "status")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["path"] != settings || data["state"] != hookCurrent {
		t.Errorf("the status read %v and found the hook %v", data["path"], data["state"])
	}
	if got := data["mod"].(map[string]any); got["enabled"] != true {
		t.Errorf("the status says of the mod %v", got)
	}
	if _, err := os.Stat(filepath.Join(e.vars["HOME"], ".claude")); err == nil {
		t.Error("something was written into ~/.claude")
	}
}

// A minka whose path holds a space is found all the same: the path of the mod
// is a path and not a command line.
func TestTheModFindsAProgramWhosePathHoldsASpace(t *testing.T) {
	e := newTestEnv(t)
	program := filepath.Join(t.TempDir(), "with space", "minka")
	if err := os.MkdirAll(filepath.Dir(program), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(program, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := e.runWith(func(a *app) { a.executable = func() (string, error) { return program, nil } },
		"", "hooks", "install", "--mod", "--json")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "there is no such program") {
		t.Errorf("the install says %s is not there:\n%s", program, r.stdout)
	}
}

// The hint is a command line to paste, so a directory that holds a space is
// quoted.
func TestTheHintQuotesTheDirectoryOfTheMod(t *testing.T) {
	e := newTestEnv(t)
	e.vars["MINKA_CONFIG"] = filepath.Join(t.TempDir(), "with space", "config.json")
	e.claudeFails = errors.New("claude is not on the PATH")
	dir := filepath.Join(filepath.Dir(e.vars["MINKA_CONFIG"]), "claude-mod")

	env, r := e.runJSON("", "hooks", "install", "--mod")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "claude plugin marketplace add '"+dir+"'\n") {
		t.Errorf("the hint is %q", env.Hint)
	}
}
