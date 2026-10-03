package mod

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// What is written is what lies in the repository, but for the one line that
// names the minka to run: a mod that drifted from its tests here would be
// tested by nothing.
func TestTheFilesAreTheOnesInTheRepositoryRunningTheMinkaNamed(t *testing.T) {
	files, err := Files("/opt/homebrew/bin/minka")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".claude-plugin/marketplace.json",
		"minka-heartbeat/.claude-plugin/plugin.json",
		"minka-heartbeat/hooks/hooks.json",
		"minka-heartbeat/hooks/register.js",
	}
	if len(files) != len(want) {
		t.Fatalf("%d files, want %v", len(files), want)
	}
	for _, name := range want {
		inTheRepository, err := os.ReadFile(filepath.Join("marketplace", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if name == "minka-heartbeat/hooks/register.js" {
			inTheRepository = bytes.Replace(inTheRepository, programLine, []byte("const minka = \"/opt/homebrew/bin/minka\"\n"), 1)
		}
		if !bytes.Equal(files[name], inTheRepository) {
			t.Errorf("%s is not what the repository holds:\n%s", name, files[name])
		}
	}
}

// A path is written as a string the module reads as that path, whatever it
// holds.
func TestTheProgramIsWrittenAsAStringOfJavaScript(t *testing.T) {
	files, err := Files(`/Users/o'brien/bin/"minka"`)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(files["minka-heartbeat/hooks/register.js"], []byte(`const minka = "/Users/o'brien/bin/\"minka\""`+"\n")) {
		t.Errorf("the module names the program as:\n%s", files["minka-heartbeat/hooks/register.js"])
	}
}

// The names Claude Code installs the mod by are the names its files declare.
func TestThePluginIsNamedAsTheMarketplaceDeclaresIt(t *testing.T) {
	files, err := Files("minka")
	if err != nil {
		t.Fatal(err)
	}
	var marketplace struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(files[".claude-plugin/marketplace.json"], &marketplace); err != nil {
		t.Fatal(err)
	}
	var plugin struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(files["minka-heartbeat/.claude-plugin/plugin.json"], &plugin); err != nil {
		t.Fatal(err)
	}
	if len(marketplace.Plugins) != 1 {
		t.Fatalf("the marketplace lists %d plugins", len(marketplace.Plugins))
	}
	listed := marketplace.Plugins[0]
	if got := listed.Name + "@" + marketplace.Name; got != Plugin || plugin.Name != listed.Name || listed.Source != "./minka-heartbeat" {
		t.Errorf("the marketplace lists %s from %s, the plugin calls itself %s; want %s", got, listed.Source, plugin.Name, Plugin)
	}
}

// What the module runs is what the heartbeat takes: the session named, and the
// marks of a beat of the mod.
func TestTheModuleRunsTheHeartbeatOfItsSession(t *testing.T) {
	files, err := Files("minka")
	if err != nil {
		t.Fatal(err)
	}
	module := string(files["minka-heartbeat/hooks/register.js"])
	for _, argv := range []string{
		`[minka, 'heartbeat', '--session', session, '--from-mod']`,
		`[minka, 'heartbeat', '--session', session, '--from-mod', '--idle']`,
	} {
		if !strings.Contains(module, argv) {
			t.Errorf("the module does not run %s", argv)
		}
	}
	var hooks struct {
		Modules []string `json:"modules"`
	}
	if err := json.Unmarshal(files["minka-heartbeat/hooks/hooks.json"], &hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks.Modules) != 1 || hooks.Modules[0] != "./register.js" {
		t.Errorf("hooks.json loads %v", hooks.Modules)
	}
}

// The mod runs its own tests where Claude Code is there to run them, on the
// files as minka writes them. Claude Code is no tool of the build, so where it
// is missing, or will not load mods in the shell that runs the tests, the test
// says so and passes over it.
func TestTheModPassesClaudeCodesChecks(t *testing.T) {
	claude, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude is not on the PATH")
	}
	files, err := Files("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tests, err := os.ReadFile(filepath.Join("marketplace", "minka-heartbeat", "tests", "heartbeat.test.ts"))
	if err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(dir, "minka-heartbeat")
	if err := os.MkdirAll(filepath.Join(plugin, "tests"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "tests", "heartbeat.test.ts"), tests, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"plugin", "validate", "--strict", dir},
		{"plugin", "validate", "--strict", plugin},
		{"plugin", "test", plugin},
	} {
		output, err := exec.Command(claude, args...).CombinedOutput()
		if strings.Contains(string(output), "hooks modules are turned off") {
			t.Skip("Claude Code loads no mods in this shell: " + strings.TrimSpace(string(output)))
		}
		if err != nil {
			t.Errorf("claude %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
}
