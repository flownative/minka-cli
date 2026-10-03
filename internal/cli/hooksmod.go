package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/localfile"
	"github.com/flownative/minka-cli/mod"
)

const (
	modInstalled    = "installed"
	modCurrent      = "current"
	modStale        = "stale"
	modNotInstalled = "not installed"
)

// claudeCommands are what makes Claude Code load the mod from where minka wrote
// it: the directory as a marketplace, and the plugin from it for every project.
// Both may be run again: Claude Code answers a second time that it has them.
func claudeCommands(dir string) [][]string {
	return [][]string{
		{"plugin", "marketplace", "add", dir},
		{"plugin", "install", mod.Plugin, "--scope", "user"},
	}
}

// runClaudeCommand runs the claude command line, and says what it printed
// where it failed.
func (a *app) runClaudeCommand(args []string) error {
	program, err := exec.LookPath("claude")
	if err != nil {
		return errors.New("claude is not on the PATH")
	}
	output, err := exec.CommandContext(a.ctx, program, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// installTheMod writes the mod beside the configuration and has Claude Code
// install it from there.
//
// Every install writes what differs from the files of this minka, so that the
// mod a machine runs is the one of the minka that last installed it. Claude
// Code loads a plugin of a directory marketplace from that directory, so the
// new files take effect with the next session, with no update in Claude Code.
func (a *app) installTheMod() error {
	dir, err := a.modPath()
	if err != nil {
		return err
	}
	program := a.minkaProgram()
	files, err := mod.Files(program)
	if err != nil {
		return &Error{Code: CodeConfig, Message: err.Error()}
	}
	changed, err := writeTheMod(dir, files)
	if err != nil {
		return &Error{Code: CodeConfig, Message: err.Error()}
	}

	for _, args := range claudeCommands(dir) {
		if err := a.runClaude(args); err != nil {
			return &Error{
				Code:    CodeConfig,
				Message: "the mod is written to " + dir + ", and Claude Code was not told of it: " + err.Error(),
				Hint:    "Run these yourself:\n" + strings.Join(claudeCommandLines(dir), "\n"),
			}
		}
	}

	state, summary := modCurrent, "The mod in "+dir+" is current and installed in Claude Code"
	if changed {
		state, summary = modInstalled, "Wrote the mod to "+dir+" and installed it in Claude Code"
	}
	s := success{
		data:    map[string]any{"mod": map[string]any{"path": dir, "plugin": mod.Plugin, "state": state, "program": program}},
		summary: summary,
		text:    func(w io.Writer) { fmt.Fprintf(w, "%s: %s\n", dir, state) },
	}
	s.notices = append(s.notices, "the mod takes effect in Claude Code sessions started from now on")
	if !theProgramIsThere(program) {
		s.notices = append(s.notices, "the mod runs "+program+", and there is no such program; it reports nothing and says nothing about it")
	}
	if notice := a.theHookIsThereAsWell(); notice != "" {
		s.notices = append(s.notices, notice)
	}
	return a.ok(s)
}

// theHookIsThereAsWell says so where the settings of the machine still carry
// the hook beside the mod. The two do not report twice, because a signal goes
// out at most once a minute whoever asks, so the hook is left where it is: it
// costs one process after every tool call and nothing else.
func (a *app) theHookIsThereAsWell() string {
	path, err := a.settingsPath("")
	if err != nil {
		return ""
	}
	settings, err := readSettings(path, "")
	if err != nil {
		return ""
	}
	found, err := heartbeatHookIn(settings, a.heartbeatCommand())
	if err != nil || found == "" {
		return ""
	}
	return "the hook in " + path + " runs " + found + " after every tool call as well; the mod does all it did, so the hook may go, and where it stays it costs a process per tool call and reports nothing twice"
}

func claudeCommandLines(dir string) []string {
	var lines []string
	for _, args := range claudeCommands(dir) {
		quoted := make([]string, len(args))
		for i, arg := range args {
			quoted[i] = shellWord(arg)
		}
		lines = append(lines, "claude "+strings.Join(quoted, " "))
	}
	return lines
}

// shellWord is an argument as a shell reads it back unchanged: a plain word as
// it is, anything else in single quotes.
func shellWord(arg string) string {
	plain := arg != "" && strings.IndexFunc(arg, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("@%+=:,./_-", r))
	}) < 0
	if plain {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// modStatus says what is on disk of the mod and whether the settings tell
// Claude Code to load it.
func (a *app) modStatus(settings map[string]any) (map[string]any, string, error) {
	dir, err := a.modPath()
	if err != nil {
		return nil, "", err
	}
	files, err := mod.Files(a.minkaProgram())
	if err != nil {
		return nil, "", &Error{Code: CodeConfig, Message: err.Error()}
	}
	state := theModOnDisk(dir, files)
	enabled := false
	if plugins, ok := settings["enabledPlugins"].(map[string]any); ok {
		enabled, _ = plugins[mod.Plugin].(bool)
	}

	var summary string
	switch {
	case state == modNotInstalled:
		summary = "No mod is written to " + dir
	case !enabled:
		summary = "The mod in " + dir + " is " + state + ", and Claude Code is not told to load it"
	default:
		summary = "The mod in " + dir + " is " + state + " and loads in every Claude Code session"
	}
	return map[string]any{"path": dir, "plugin": mod.Plugin, "state": state, "enabled": enabled}, summary, nil
}

// theModOnDisk compares what lies in the directory with what this minka would
// write there. A mod written by another minka, or another version of it, is
// stale: it may run a program that is not there any more.
func theModOnDisk(dir string, files map[string][]byte) string {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return modNotInstalled
	}
	for name, content := range files {
		there, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(there, content) {
			return modStale
		}
	}
	return modCurrent
}

// writeTheMod writes every file of the mod that differs from what lies there,
// and answers whether it wrote any.
func writeTheMod(dir string, files map[string][]byte) (bool, error) {
	changed := false
	for _, name := range slices.Sorted(maps.Keys(files)) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if there, err := os.ReadFile(path); err == nil && bytes.Equal(there, files[name]) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return changed, fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
		}
		if err := localfile.Write(path, ".mod-*", files[name]); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// modPath is where the mod lies: beside the configuration, like everything
// else minka keeps on a machine.
func (a *app) modPath() (string, error) {
	path, err := config.Path(a.getenv)
	if err != nil {
		return "", &Error{Code: CodeConfig, Message: err.Error()}
	}
	return filepath.Join(filepath.Dir(path), "claude-mod"), nil
}

// minkaProgram is the minka the mod runs: this very binary, by the path it was
// started from. Started through a link such as the one Homebrew keeps on the
// PATH, that is the link, which survives an upgrade; a firewall that knows
// minka by that path lets the beats of the mod through as well.
func (a *app) minkaProgram() string {
	binary, err := a.executable()
	if err != nil {
		return "minka"
	}
	return binary
}
