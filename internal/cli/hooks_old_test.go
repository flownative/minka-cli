package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const theOldHookAlone = `{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/opt/homebrew/bin/minka heartbeat", "timeout": 10}]}]}}`

// The safeguard: a Claude Code too old to load mods takes the plugin without a
// word and never runs it. It is not told of the mod, the hook stays, and the
// install says to update Claude Code instead of calling the mod installed.
func TestAClaudeCodeThatCannotLoadModsKeepsTheHook(t *testing.T) {
	for name, version := range map[string]string{
		"older":      "2.1.211 (Claude Code)\n",
		"just older": "2.1.286 (Claude Code)\n",
		"unreadable": "Claude Code, some version\n",
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			e.claudeVersion = version
			writeFile(t, e.claudeSettings(), theOldHookAlone, 0o600)

			env, r := e.runJSON("", "hooks", "install")
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Error, "only") && !strings.Contains(env.Error, "cannot load mods") {
				t.Errorf("the error is %q", env.Error)
			}
			if !strings.Contains(env.Hint, "Update Claude Code") {
				t.Errorf("the hint is %q", env.Hint)
			}
			if len(e.claudeRan) != 0 {
				t.Errorf("Claude Code was told of the mod: %v", e.claudeRan)
			}
			if got := readFile(t, e.claudeSettings()); got != theOldHookAlone {
				t.Errorf("the settings changed: %s", got)
			}
		})
	}
}

// The first version that loads mods, and every later one, is told of the mod.
func TestTheVersionsThatLoadMods(t *testing.T) {
	for version, loads := range map[string]bool{
		"2.1.287 (Claude Code)":        true,
		"2.1.289 (Claude Code)":        true,
		"  2.1.289 (Claude Code)\n":    true,
		"2.2.0 (Claude Code)":          true,
		"3.0.0":                        true,
		"10.0.0":                       true,
		"Claude Code 2.1.289":          true,
		"2.1.290-beta.1 (Claude Code)": true,
		"2.1.290-rc1":                  true,

		"Warning: an update\n2.1.289 (Claude Code)": true,

		"2.1.286 (Claude Code)":     false,
		"2.1.286-rc1 (Claude Code)": false,
		"2.0.999 (Claude Code)":     false,
		"1.9.300 (Claude Code)":     false,
		"2.1 (Claude Code)":         false,
		"v2.1.290 (Claude Code)":    false,
		"2.1.290.4 (Claude Code)":   false,
		"":                          false,
	} {
		a := newApp(t.Context(), strings.NewReader(""), &strings.Builder{}, &strings.Builder{}, func(string) string { return "" })
		a.runClaude = func([]string) (string, error) { return version, nil }
		if err := a.claudeLoadsMods(); (err == nil) != loads {
			t.Errorf("%q: %v, want loading %v", version, err, loads)
		}
	}
}

// The safeguard: a version that cannot be read is refused as such, not taken
// for a version of nothing.
func TestAVersionThatCannotBeReadSaysSo(t *testing.T) {
	for _, printed := range []string{"Claude Code, some version\n", "v2.1.290 (Claude Code)\n", ""} {
		a := newApp(t.Context(), strings.NewReader(""), &strings.Builder{}, &strings.Builder{}, func(string) string { return "" })
		a.runClaude = func([]string) (string, error) { return printed, nil }
		err := a.claudeLoadsMods()
		if err == nil || !strings.Contains(err.Error(), "cannot be read") || strings.Contains(err.Error(), "0.0.0") {
			t.Errorf("%q: %v", printed, err)
		}
	}
}

// What claude says on standard error is no version: a warning there, and
// another line before the version on standard output, still let the mod in.
// The claude here is a script of the test, never the real one.
func TestAWarningBesideTheVersionLetsTheModIn(t *testing.T) {
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "claude"), "#!/bin/sh\n"+
		"echo 'Update available: 2.1.200 -> 2.1.300' >&2\n"+
		"if [ \"$1\" = --version ]; then echo 'A new version is out'; echo '2.1.289 (Claude Code)'; fi\n", 0o700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	e := newTestEnv(t)
	r := e.runWith(func(a *app) { a.runClaude = a.runClaudeCommand }, "", "hooks", "install", "--json")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, `"state": "installed"`) {
		t.Errorf("the install says %s", r.stdout)
	}
}

// The status says so where the Claude Code on the PATH cannot load the mod,
// whatever the settings tell it.
func TestTheStatusSaysWhereClaudeCodeCannotLoadTheMod(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "hooks", "install", "--json"), ExitOK)
	e.claudeVersion = "2.1.211 (Claude Code)\n"

	env, r := e.runJSON("", "hooks", "status")
	expectCode(t, r, ExitOK)
	got := decode(t, env.Data)["mod"].(map[string]any)
	if reason, _ := got["reason"].(string); !strings.Contains(reason, "2.1.211 cannot load mods") {
		t.Errorf("the status says of the mod %v", got)
	}
	if strings.Contains(env.Summary, "loads in every") || !strings.Contains(env.Notice, "cannot load mods") {
		t.Errorf("summary %q, notice %q", env.Summary, env.Notice)
	}
}

// What is not the hook of minka stays where Claude Code reads it: a hook of
// another type that names minka, and a group of another matcher, which keeps
// its now empty list.
func TestOnlyACommandHookOfEveryToolGoesWhole(t *testing.T) {
	for name, files := range map[string][2]string{
		"a prompt": {
			`{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "prompt", "command": "minka heartbeat"}, {"type": "command", "command": "minka heartbeat"}]}]}}`,
			`{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "prompt", "command": "minka heartbeat"}]}]}}`,
		},
		"a matcher of one tool": {
			`{"hooks": {"PostToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`,
			`{"hooks": {"PostToolUse": [{"matcher": "Bash", "hooks": []}]}}`,
		},
		"lines ending in CRLF": {
			"{\r\n  \"a\": 1,\r\n  \"hooks\": {\"PostToolUse\": [{\"matcher\": \"*\", \"hooks\": [{\"type\": \"command\", \"command\": \"minka heartbeat\"}]}]}\r\n}\r\n",
			"{\r\n  \"a\": 1\r\n}\r\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			writeFile(t, e.claudeSettings(), files[0], 0o600)
			expectCode(t, e.run("", "hooks", "install", "--json"), ExitOK)
			if got := readFile(t, e.claudeSettings()); got != files[1] {
				t.Errorf("the settings are now\n%q\nwant\n%q", got, files[1])
			}
		})
	}
}

// The safeguard: a key that stands twice on the way to the hook leaves the
// file alone, and the install and the status say why.
func TestAKeyTwiceOnTheWayToTheHookLeavesTheFileAlone(t *testing.T) {
	for name, settings := range map[string]string{
		"hooks":       `{"hooks": {"PreToolUse": []}, "hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`,
		"PostToolUse": `{"hooks": {"PostToolUse": [], "PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`,
		"matcher":     `{"hooks": {"PostToolUse": [{"matcher": "Bash", "matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`,
		"command":     `{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "rm -rf ~", "command": "minka heartbeat"}]}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			writeFile(t, e.claudeSettings(), settings, 0o600)

			env, r := e.runJSON("", "hooks", "install")
			expectCode(t, r, ExitOK)
			if got := readFile(t, e.claudeSettings()); got != settings {
				t.Errorf("the settings changed: %s", got)
			}
			if !strings.Contains(env.Notice, "twice") {
				t.Errorf("the install says %q", env.Notice)
			}
			env, r = e.runJSON("", "hooks", "status")
			expectCode(t, r, ExitOK)
			if !strings.Contains(env.Notice, "twice") {
				t.Errorf("the status says %q", env.Notice)
			}
		})
	}
}

// The safeguard behind every cut: what is left has to be JSON, or nothing is
// taken out at all.
func TestACutThatLeavesNoJSONIsRefused(t *testing.T) {
	content := []byte(`{"a": 1, "b": 2}`)
	if _, err := cutOut(content, 0, 1); err == nil {
		t.Error("a cut of the opening brace went through")
	}
	if _, err := cutOut(content, 6, 7); err == nil {
		t.Error("a cut that leaves a key without a value went through")
	}
	left, err := cutOut(content, 1, 9)
	if err != nil || string(left) != `{"b": 2}` {
		t.Errorf("a cut of a member left %q, %v", left, err)
	}
	if string(content) != `{"a": 1, "b": 2}` {
		t.Errorf("the cut changed what it was given: %s", content)
	}
}
