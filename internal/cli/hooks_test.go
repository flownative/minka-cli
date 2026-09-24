package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func settingsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readSettingsFile(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("%s is not JSON any more: %v\n%s", path, err, content)
	}
	return settings
}

func TestInstallingTheHookWritesItWhereClaudeCodeReadsIt(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "hooks", "install")
	expectCode(t, r, ExitOK)

	path := filepath.Join(e.vars["HOME"], ".claude", "settings.json")
	if decode(t, env.Data)["path"] != path {
		t.Errorf("wrote to %v, want %s", decode(t, env.Data)["path"], path)
	}
	settings := readSettingsFile(t, path)
	groups := settings["hooks"].(map[string]any)["PostToolUse"].([]any)
	if len(groups) != 1 {
		t.Fatalf("%d entries, want 1", len(groups))
	}
	hook := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if !strings.HasSuffix(hook["command"].(string), " heartbeat") {
		t.Errorf("the hook runs %q", hook["command"])
	}
	if hook["type"] != "command" {
		t.Errorf("the hook is of type %v", hook["type"])
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("permissions of the settings: %v %v", info.Mode().Perm(), err)
	}
}

// The safeguard: the file belongs to the user, so everything in it is kept.
func TestInstallingTheHookKeepsWhatTheSettingsAlreadyHold(t *testing.T) {
	e := newTestEnv(t)
	path := settingsFile(t, `{
	  "model": "opus",
	  "hooks": {
	    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "guard.sh"}]}],
	    "PostToolUse": [{"matcher": "Edit", "hooks": [{"type": "command", "command": "format.sh"}]}]
	  }
	}`)

	expectCode(t, e.run("", "hooks", "install", "--file", path, "--json"), ExitOK)

	settings := readSettingsFile(t, path)
	if settings["model"] != "opus" {
		t.Errorf("model is %v", settings["model"])
	}
	hooks := settings["hooks"].(map[string]any)
	if len(hooks["PreToolUse"].([]any)) != 1 {
		t.Error("the hooks before a tool call are gone")
	}
	groups := hooks["PostToolUse"].([]any)
	if len(groups) != 2 {
		t.Fatalf("%d entries after a tool call, want 2", len(groups))
	}
	theirs := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if theirs["command"] != "format.sh" {
		t.Errorf("the hook of somebody else is %v", theirs["command"])
	}
}

func TestInstallingTheHookTwiceChangesNothing(t *testing.T) {
	e := newTestEnv(t)
	path := settingsFile(t, `{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/usr/local/bin/minka heartbeat"}]}]}}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "hooks", "install", "--file", path)
	expectCode(t, r, ExitOK)
	if state := decode(t, env.Data)["state"]; state != hookCurrent {
		t.Errorf("state %v, want %q", state, hookCurrent)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("the settings were rewritten:\n%s", after)
	}
}

// The safeguard: a file this command does not understand is refused instead of
// repaired, because a settings file that is mangled is worse than one without
// the hook.
func TestSettingsThatAreNotUnderstoodAreRefusedAndLeftAlone(t *testing.T) {
	for name, content := range map[string]string{
		"not JSON":              "{ this is not json",
		"not an object":         `["a list"]`,
		"hooks is no object":    `{"hooks": "none"}`,
		"the event is no list":  `{"hooks": {"PostToolUse": {"matcher": "*"}}}`,
		"an entry is no object": `{"hooks": {"PostToolUse": ["format.sh"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			path := settingsFile(t, content)

			env, r := e.runJSON("", "hooks", "install", "--file", path)
			expectCode(t, r, ExitUsage)
			if env.Code != CodeConfig {
				t.Errorf("code %q, want %q", env.Code, CodeConfig)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != content {
				t.Errorf("the settings were changed:\n%s", after)
			}
		})
	}
}

// A field that stands there as null is read as "no hooks", so it has to be
// written into as well: otherwise the status advises an install that then
// refuses the very file it looked at.
func TestHooksThatStandThereAsNullAreWrittenIntoAllTheSame(t *testing.T) {
	e := newTestEnv(t)
	path := settingsFile(t, `{"model": "opus", "hooks": null}`)

	env, r := e.runJSON("", "hooks", "status", "--file", path)
	expectCode(t, r, ExitOK)
	if state := decode(t, env.Data)["state"]; state != hookNotInstalled {
		t.Fatalf("state %v, want %q", state, hookNotInstalled)
	}

	expectCode(t, e.run("", "hooks", "install", "--file", path, "--json"), ExitOK)
	settings := readSettingsFile(t, path)
	if settings["model"] != "opus" {
		t.Errorf("model is %v", settings["model"])
	}
	if groups := settings["hooks"].(map[string]any)["PostToolUse"].([]any); len(groups) != 1 {
		t.Errorf("%d entries, want 1", len(groups))
	}
}

// The safeguard: the settings are written by renaming a file that is complete.
// Where that cannot be done, what stands there is left whole rather than
// emptied first and lost.
func TestSettingsThatCannotBeWrittenAreLeftWhole(t *testing.T) {
	e := newTestEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	content := `{"model": "opus"}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	r := e.run("", "hooks", "install", "--file", path, "--json")
	expectCode(t, r, ExitUsage)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != content {
		t.Errorf("the settings were changed by a write that failed:\n%s", after)
	}
}

func TestAHookWhoseProgramIsGoneSaysSo(t *testing.T) {
	e := newTestEnv(t)
	gone := filepath.Join(t.TempDir(), "minka")
	path := settingsFile(t, `{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "`+gone+` heartbeat"}]}]}}`)

	env, r := e.runJSON("", "hooks", "status", "--file", path)
	expectCode(t, r, ExitOK)
	if state := decode(t, env.Data)["state"]; state != hookCurrent {
		t.Fatalf("state %v", state)
	}
	if !strings.Contains(env.Notice, "no such program") {
		t.Errorf("nothing says that the program is gone: %q", env.Notice)
	}
}

func TestSettingsInAWorkingTreeOfGitSayThatThePathIsThisMachinesOwn(t *testing.T) {
	e := newTestEnv(t)
	tree := t.TempDir()
	if err := os.Mkdir(filepath.Join(tree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tree, ".claude", "settings.json")

	env, r := e.runJSON("", "hooks", "install", "--file", path)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "working tree") {
		t.Errorf("nothing says that the file may be committed: %q", env.Notice)
	}
}

// Whatever the command does not understand, it hands over the entry to write
// by hand — a file that is no JSON at all most of all, because there is
// nothing else left to do with it.
func TestARefusedSettingsFileCarriesTheEntryToAddByHand(t *testing.T) {
	for name, content := range map[string]string{
		"a shape it does not know": `{"hooks": {"PostToolUse": {"matcher": "*"}}}`,
		"no JSON at all":           "not json at all",
		"no object":                `[1, 2, 3]`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			path := settingsFile(t, content)

			env, r := e.runJSON("", "hooks", "install", "--file", path)
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Hint, "PostToolUse") || !strings.Contains(env.Hint, "heartbeat") {
				t.Errorf("the hint does not carry the entry: %q", env.Hint)
			}
		})
	}
}

func TestTheStatusOfTheHookSaysWhetherAnythingReports(t *testing.T) {
	e := newTestEnv(t)
	path := filepath.Join(t.TempDir(), "settings.json")

	env, r := e.runJSON("", "hooks", "status", "--file", path)
	expectCode(t, r, ExitOK)
	if state := decode(t, env.Data)["state"]; state != hookNotInstalled {
		t.Errorf("state %v, want %q", state, hookNotInstalled)
	}
	if len(env.Breadcrumbs) == 0 {
		t.Error("nothing says how to install it")
	}

	expectCode(t, e.run("", "hooks", "install", "--file", path, "--json"), ExitOK)
	env, r = e.runJSON("", "hooks", "status", "--file", path)
	expectCode(t, r, ExitOK)
	if state := decode(t, env.Data)["state"]; state != hookCurrent {
		t.Errorf("state %v, want %q", state, hookCurrent)
	}
}

func TestOnlyAMinkaReportingIsReadAsTheHookOfMinka(t *testing.T) {
	ours := map[string]bool{
		"/opt/homebrew/bin/minka heartbeat":     true,
		"minka heartbeat":                       true,
		"/usr/local/bin/minka heartbeat --json": true,
		"minka work list":                       false,
		"heartbeat":                             false,
		"/usr/bin/heartbeat minka":              false,
		"minkactl heartbeat":                    false,
	}
	for command, want := range ours {
		if got := isHeartbeat(command); got != want {
			t.Errorf("%q is read as %v, want %v", command, got, want)
		}
	}
}
