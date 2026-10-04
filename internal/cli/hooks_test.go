package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/mod"
)

// claudeSettings is where these tests put the settings of Claude Code: in the
// home of the test, never in the home of whoever runs it.
func (e *testEnv) claudeSettings() string {
	return filepath.Join(e.vars["HOME"], ".claude", "settings.json")
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func (e *testEnv) modDir() string {
	return filepath.Join(filepath.Dir(e.vars["MINKA_CONFIG"]), "claude-mod")
}

func TestInstallingWritesTheModAndTellsClaudeCode(t *testing.T) {
	e := newTestEnv(t)
	dir := e.modDir()

	env, r := e.runJSON("", "hooks", "install")
	expectCode(t, r, ExitOK)
	written := decode(t, env.Data)["mod"].(map[string]any)
	if written["path"] != dir || written["state"] != modInstalled || written["plugin"] != mod.Plugin {
		t.Errorf("the install said %v", written)
	}
	if hook := decode(t, env.Data)["oldHook"]; hook != nil {
		t.Errorf("the install found an old hook where there is no settings file: %v", hook)
	}
	want := [][]string{
		{"plugin", "marketplace", "add", dir},
		{"plugin", "install", mod.Plugin, "--scope", "user"},
	}
	if !slices.EqualFunc(e.claudeRan, want, slices.Equal) {
		t.Errorf("ran claude %v, want %v", e.claudeRan, want)
	}

	module := readFile(t, filepath.Join(dir, "minka-heartbeat", "hooks", "register.js"))
	program, _ := os.Executable()
	if !strings.Contains(module, `const minka = "`+program+`"`) {
		t.Errorf("the mod does not run this minka, %s", program)
	}

	env, r = e.runJSON("", "hooks", "install")
	expectCode(t, r, ExitOK)
	if state := decode(t, env.Data)["mod"].(map[string]any)["state"]; state != modCurrent {
		t.Errorf("a second install says %v", state)
	}
	if len(e.claudeRan) != 4 {
		t.Errorf("a second install did not tell Claude Code again: %v", e.claudeRan)
	}
	if _, err := os.Stat(e.claudeSettings()); err == nil {
		t.Error("the install wrote settings of Claude Code itself")
	}
}

// --mod is what install does anyway, so a script that passes it keeps working.
// --file and --command are no flags of install or status.
func TestTheFlagsOfInstallAreThoseOfTheMod(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "hooks", "install", "--mod")
	expectCode(t, r, ExitOK)
	if state := decode(t, env.Data)["mod"].(map[string]any)["state"]; state != modInstalled {
		t.Errorf("install --mod says %v", state)
	}
	ran := len(e.claudeRan)
	expectCode(t, e.run("", "hooks", "install", "--file", filepath.Join(t.TempDir(), "s.json"), "--json"), ExitUsage)
	expectCode(t, e.run("", "hooks", "install", "--command", "minka", "--json"), ExitUsage)
	expectCode(t, e.run("", "hooks", "status", "--file", filepath.Join(t.TempDir(), "s.json"), "--json"), ExitUsage)
	if len(e.claudeRan) != ran {
		t.Errorf("ran claude %v", e.claudeRan[ran:])
	}

	root := newApp(t.Context(), strings.NewReader(""), &strings.Builder{}, &strings.Builder{}, func(string) string { return "" }).newRootCmd()
	install, _, err := root.Find([]string{"hooks", "install"})
	if err != nil {
		t.Fatal(err)
	}
	if flag := install.Flags().Lookup("mod"); flag == nil || !flag.Hidden {
		t.Error("--mod is not a hidden flag of hooks install")
	}
}

// Where Claude Code cannot be told, the mod is written all the same, the hint
// carries what to run by hand, and the old hook stays: a session left with
// neither would report nothing.
func TestInstallingWithoutClaudeSaysWhatToRunAndKeepsTheHook(t *testing.T) {
	e := newTestEnv(t)
	e.claudeFails = errNoClaude
	dir := e.modDir()
	settings := `{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/opt/homebrew/bin/minka heartbeat", "timeout": 10}]}]}}`
	writeFile(t, e.claudeSettings(), settings, 0o600)

	env, r := e.runJSON("", "hooks", "install")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "claude plugin marketplace add "+dir) || !strings.Contains(env.Hint, "claude plugin install "+mod.Plugin) {
		t.Errorf("the hint is %q", env.Hint)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude-plugin", "marketplace.json")); err != nil {
		t.Errorf("the mod was not written: %v", err)
	}
	if got := readFile(t, e.claudeSettings()); got != settings {
		t.Errorf("the settings changed although the mod is not installed:\n%s", got)
	}
}

// The hint is a command line to paste, so a directory that holds a space is
// quoted.
func TestTheHintQuotesTheDirectoryOfTheMod(t *testing.T) {
	e := newTestEnv(t)
	e.vars["MINKA_CONFIG"] = filepath.Join(t.TempDir(), "with space", "config.json")
	e.claudeFails = errNoClaude

	env, r := e.runJSON("", "hooks", "install")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "claude plugin marketplace add '"+e.modDir()+"'\n") {
		t.Errorf("the hint is %q", env.Hint)
	}
}

// A minka whose path holds a space is found all the same: the path of the mod
// is a path and not a command line.
func TestTheModFindsAProgramWhosePathHoldsASpace(t *testing.T) {
	e := newTestEnv(t)
	program := filepath.Join(t.TempDir(), "with space", "minka")
	writeFile(t, program, "#!/bin/sh\n", 0o700)
	r := e.runWith(func(a *app) { a.executable = func() (string, error) { return program, nil } },
		"", "hooks", "install", "--json")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "there is no such program") {
		t.Errorf("the install says %s is not there:\n%s", program, r.stdout)
	}
}

// The status names the mod: whether it is written as this minka writes it and
// whether the settings tell Claude Code to load it.
func TestTheStatusSaysWhatBecameOfTheMod(t *testing.T) {
	e := newTestEnv(t)

	modOf := func() (map[string]any, envelope) {
		env, r := e.runJSON("", "hooks", "status")
		expectCode(t, r, ExitOK)
		return decode(t, env.Data)["mod"].(map[string]any), env
	}
	if got, env := modOf(); got["state"] != modNotInstalled || got["enabled"] != false || len(env.Breadcrumbs) == 0 {
		t.Errorf("before the install: %v, breadcrumbs %v", got, env.Breadcrumbs)
	}

	expectCode(t, e.run("", "hooks", "install", "--json"), ExitOK)
	if got, _ := modOf(); got["state"] != modCurrent || got["enabled"] != false {
		t.Errorf("written, and nothing in the settings: %v", got)
	}

	writeFile(t, e.claudeSettings(), `{"enabledPlugins": {"`+mod.Plugin+`": true}}`, 0o600)
	if got, env := modOf(); got["state"] != modCurrent || got["enabled"] != true || len(env.Breadcrumbs) != 0 {
		t.Errorf("written and enabled: %v, breadcrumbs %v", got, env.Breadcrumbs)
	}

	module := filepath.Join(e.modDir(), "minka-heartbeat", "hooks", "register.js")
	writeFile(t, module, "const minka = '/gone/minka'\n", 0o600)
	if got, _ := modOf(); got["state"] != modStale {
		t.Errorf("written by another minka: %v", got)
	}
}

// An old hook is a leftover: the status names it and points at the install
// that takes it out, without touching the file.
func TestTheStatusNamesAnOldHookAsALeftover(t *testing.T) {
	e := newTestEnv(t)
	settings := `{"enabledPlugins": {"` + mod.Plugin + `": true}, "hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/opt/homebrew/bin/minka heartbeat", "timeout": 10}]}]}}`
	writeFile(t, e.claudeSettings(), settings, 0o600)
	expectCode(t, e.run("", "hooks", "install", "--json"), ExitOK)
	writeFile(t, e.claudeSettings(), settings, 0o600)

	env, r := e.runJSON("", "hooks", "status")
	expectCode(t, r, ExitOK)
	hook, _ := decode(t, env.Data)["oldHook"].(map[string]any)
	if hook["command"] != "/opt/homebrew/bin/minka heartbeat" || hook["path"] != e.claudeSettings() || hook["removed"] != false {
		t.Errorf("the status says of the old hook %v", hook)
	}
	if !strings.Contains(env.Notice, "leftover") {
		t.Errorf("the notice is %q", env.Notice)
	}
	if len(env.Breadcrumbs) != 1 || env.Breadcrumbs[0].Cmd != "minka hooks install" {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}
	if got := readFile(t, e.claudeSettings()); got != settings {
		t.Errorf("the status changed the settings:\n%s", got)
	}
}

// Claude Code reads its settings from $CLAUDE_CONFIG_DIR where that is set, and
// so do the status and the install: a status that looked into ~/.claude would
// call a mod Claude Code loads one it does not, and the install would leave the
// hook Claude Code runs where it is.
func TestTheSettingsAreReadWhereClaudeConfigDirPoints(t *testing.T) {
	e := newTestEnv(t)
	e.vars["CLAUDE_CONFIG_DIR"] = filepath.Join(t.TempDir(), "claude")
	settings := filepath.Join(e.vars["CLAUDE_CONFIG_DIR"], "settings.json")
	writeFile(t, settings, `{"enabledPlugins": {"`+mod.Plugin+`": true}, "hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat", "timeout": 10}]}]}}`, 0o600)

	env, r := e.runJSON("", "hooks", "install")
	expectCode(t, r, ExitOK)
	if hook, _ := decode(t, env.Data)["oldHook"].(map[string]any); hook["path"] != settings || hook["removed"] != true {
		t.Errorf("the install says of the old hook %v", hook)
	}
	if got := readFile(t, settings); got != `{"enabledPlugins": {"`+mod.Plugin+`": true}}` {
		t.Errorf("the settings are now %s", got)
	}

	env, r = e.runJSON("", "hooks", "status")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if got := data["mod"].(map[string]any); got["enabled"] != true || got["settings"] != settings {
		t.Errorf("the status says of the mod %v", got)
	}
	if _, err := os.Stat(filepath.Join(e.vars["HOME"], ".claude")); err == nil {
		t.Error("something was written into ~/.claude")
	}
}

// The safeguard: taking the old hook out takes out the entry minka wrote and
// nothing else. The hooks of others stay, under the same event and others, a
// hook that runs minka for something else stays, and every byte around the
// entry is as it was written: the order of the keys, the indentation, the
// spaces Claude Code would not write.
func TestTakingTheOldHookOutLeavesEverythingElseAsItWas(t *testing.T) {
	e := newTestEnv(t)
	before := `{
    "model":   "opus",
    "hooks": {
        "PreToolUse": [ { "matcher": "Bash", "hooks": [ { "type": "command", "command": "guard.sh" } ] } ],
        "PostToolUse": [
            {"matcher": "Edit",
             "hooks": [{"type": "command", "command": "format.sh"}]},
            { "matcher": "*", "hooks": [
                { "type": "command", "command": "/usr/local/bin/minka work list" },
                { "type": "command", "command": "/opt/homebrew/bin/minka heartbeat", "timeout": 10 },
                { "type": "command", "command": "notify \"done\"" }
            ] },
            {
              "matcher": "*",
              "hooks": [
                {
                  "type": "command",
                  "command": "/Users/somebody/with space/minka heartbeat",
                  "timeout": 10
                }
              ]
            }
        ]
    },
    "theme":"dark"
}
`
	want := `{
    "model":   "opus",
    "hooks": {
        "PreToolUse": [ { "matcher": "Bash", "hooks": [ { "type": "command", "command": "guard.sh" } ] } ],
        "PostToolUse": [
            {"matcher": "Edit",
             "hooks": [{"type": "command", "command": "format.sh"}]},
            { "matcher": "*", "hooks": [
                { "type": "command", "command": "/usr/local/bin/minka work list" },
                { "type": "command", "command": "notify \"done\"" }
            ] }
        ]
    },
    "theme":"dark"
}
`
	writeFile(t, e.claudeSettings(), before, 0o640)

	env, r := e.runJSON("", "hooks", "install")
	expectCode(t, r, ExitOK)
	if hook, _ := decode(t, env.Data)["oldHook"].(map[string]any); hook["removed"] != true || hook["command"] != "/opt/homebrew/bin/minka heartbeat" {
		t.Errorf("the install says of the old hook %v", hook)
	}
	if got := readFile(t, e.claudeSettings()); got != want {
		t.Errorf("the settings are now\n%s\nwant\n%s", got, want)
	}
	info, err := os.Stat(e.claudeSettings())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("the settings have the mode %04o, want 0640", info.Mode().Perm())
	}
}

// What only held the hook goes with it, so that a file whose only hook was
// minka's reads as it would without it.
func TestWhatOnlyHeldTheOldHookGoesWithIt(t *testing.T) {
	for name, files := range map[string][2]string{
		"the only key": {
			`{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat", "timeout": 10}]}]}}`,
			`{}`,
		},
		"between others": {
			"{\n  \"model\": \"opus\",\n  \"hooks\": {\n    \"PostToolUse\": [\n      {\n        \"matcher\": \"*\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"/opt/homebrew/bin/minka heartbeat\",\n            \"timeout\": 10\n          }\n        ]\n      }\n    ]\n  },\n  \"theme\": \"dark\"\n}\n",
			"{\n  \"model\": \"opus\",\n  \"theme\": \"dark\"\n}\n",
		},
		"the last key": {
			"{\n  \"model\": \"opus\",\n  \"hooks\": {\"PostToolUse\": [{\"matcher\": \"*\", \"hooks\": [{\"type\": \"command\", \"command\": \"minka heartbeat\"}]}]}\n}",
			"{\n  \"model\": \"opus\"\n}",
		},
		"beside another event": {
			`{"hooks": {"Stop": [], "PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`,
			`{"hooks": {"Stop": []}}`,
		},
		"a group of somebody else's": {
			`{"hooks": {"PostToolUse": [{"matcher": "*", "description": "mine", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`,
			`{"hooks": {"PostToolUse": [{"matcher": "*", "description": "mine", "hooks": []}]}}`,
		},
		"written twice": {
			`{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}, {"matcher": "*", "hooks": [{"type": "command", "command": "/opt/homebrew/bin/minka heartbeat"}]}]}, "theme": "dark"}`,
			`{"theme": "dark"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			writeFile(t, e.claudeSettings(), files[0], 0o600)
			expectCode(t, e.run("", "hooks", "install", "--json"), ExitOK)
			if got := readFile(t, e.claudeSettings()); got != files[1] {
				t.Errorf("the settings are now\n%s\nwant\n%s", got, files[1])
			}
		})
	}
}

// A settings file behind a link is changed where the link points, and the link
// stays a link.
func TestTheOldHookIsTakenOutWhereTheLinkPoints(t *testing.T) {
	e := newTestEnv(t)
	target := filepath.Join(t.TempDir(), "dotfiles", "settings.json")
	writeFile(t, target, `{"theme": "dark", "hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`, 0o600)
	if err := os.MkdirAll(filepath.Dir(e.claudeSettings()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, e.claudeSettings()); err != nil {
		t.Fatal(err)
	}

	expectCode(t, e.run("", "hooks", "install", "--json"), ExitOK)
	if info, err := os.Lstat(e.claudeSettings()); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the settings are no link any more: %v", err)
	}
	if got := readFile(t, target); got != `{"theme": "dark"}` {
		t.Errorf("the file the link points at is now %s", got)
	}
}

// Settings that are no JSON are no file to change: the install leaves them as
// they are and says so, and the mod is installed all the same.
func TestSettingsThatAreNoJSONAreLeftAlone(t *testing.T) {
	e := newTestEnv(t)
	broken := `{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}, // a comment` + "\n}"
	writeFile(t, e.claudeSettings(), broken, 0o600)

	env, r := e.runJSON("", "hooks", "install")
	expectCode(t, r, ExitOK)
	if got := readFile(t, e.claudeSettings()); got != broken {
		t.Errorf("the settings changed:\n%s", got)
	}
	if state := decode(t, env.Data)["mod"].(map[string]any)["state"]; state != modInstalled {
		t.Errorf("the mod is %v", state)
	}
	if !strings.Contains(env.Notice, "cannot be read as JSON") {
		t.Errorf("the install says %q", env.Notice)
	}

	env, r = e.runJSON("", "hooks", "status")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "cannot be read as JSON") {
		t.Errorf("the status says %q", env.Notice)
	}
}

// Only the command line minka wrote is its hook: a program called minka, by
// whatever path, and heartbeat.
func TestOnlyWhatMinkaWroteIsReadAsItsHook(t *testing.T) {
	for command, want := range map[string]bool{
		"/opt/homebrew/bin/minka heartbeat":           true,
		"minka heartbeat":                             true,
		"/Users/a b/bin/minka heartbeat":              true,
		`"/Users/a b/bin/minka" heartbeat`:            true,
		"/usr/local/bin/minka heartbeat --json":       false,
		"minka work list":                             false,
		"heartbeat":                                   false,
		"/usr/bin/heartbeat minka":                    false,
		"minkactl heartbeat":                          false,
		"echo minka heartbeat":                        false,
		"echo /x/minka heartbeat":                     false,
		"cd /repo && ./bin/minka heartbeat":           false,
		"nice -n 5 /opt/homebrew/bin/minka heartbeat": false,
		"MINKA_HOST=x /x/minka heartbeat":             false,
		"/bin/echo /opt/homebrew/bin/minka heartbeat": false,
		"./bin/minka heartbeat":                       false,
		" minka heartbeat":                            false,
		"minka heartbeat ":                            false,
	} {
		if got := isTheHookOfMinka(command); got != want {
			t.Errorf("%q is read as %v, want %v", command, got, want)
		}
	}
}

// Taking the hook out goes by the file as it is after Claude Code wrote into
// it: what the install of the mod put there is kept.
func TestTheOldHookIsTakenOutAfterClaudeCodeWroteItsOwn(t *testing.T) {
	e := newTestEnv(t)
	writeFile(t, e.claudeSettings(), `{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`, 0o600)
	r := e.runWith(func(a *app) {
		standIn := a.runClaude
		a.runClaude = func(args []string) (string, error) {
			if len(args) > 1 && args[1] == "install" {
				writeFile(t, e.claudeSettings(), `{"enabledPlugins": {"`+mod.Plugin+`": true}, "hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}}`, 0o600)
			}
			return standIn(args)
		}
	}, "", "hooks", "install", "--json")
	expectCode(t, r, ExitOK)
	if got := readFile(t, e.claudeSettings()); got != `{"enabledPlugins": {"`+mod.Plugin+`": true}}` {
		t.Errorf("the settings are now %s", got)
	}
}
