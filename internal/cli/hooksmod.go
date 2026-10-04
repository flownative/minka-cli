package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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

	// modWritten is a mod written beside the configuration that Claude Code
	// was not told of, and modSkipped one setup was told not to install.
	modWritten = "written"
	modSkipped = "skipped"
)

// errNoClaude says that Claude Code cannot be run from here. Setup goes on
// without the mod then: signing in is worth having on a machine without it.
var errNoClaude = errors.New("claude is not on the PATH")

// claudeCommands are what makes Claude Code load the mod from where minka wrote
// it: the directory as a marketplace, and the plugin from it for every project.
// Both may be run again: Claude Code answers a second time that it has them.
func claudeCommands(dir string) [][]string {
	return [][]string{
		{"plugin", "marketplace", "add", dir},
		{"plugin", "install", mod.Plugin, "--scope", "user"},
	}
}

// runClaudeCommand runs the claude command line and answers what it printed
// on its standard output. What it says on standard error, such as a warning of
// Node or a notice of an update, is no answer, and only shown where it failed.
func (a *app) runClaudeCommand(args []string) (string, error) {
	program, err := exec.LookPath("claude")
	if err != nil {
		return "", errNoClaude
	}
	var stdout, stderr bytes.Buffer
	run := exec.CommandContext(a.ctx, program, args...)
	run.Stdout, run.Stderr = &stdout, &stderr
	if err := run.Run(); err != nil {
		said := strings.TrimSpace(stderr.String() + "\n" + stdout.String())
		return "", fmt.Errorf("claude %s: %w: %s", strings.Join(args, " "), err, said)
	}
	return stdout.String(), nil
}

// modsSince is the first version of Claude Code that loads a mod. An older one
// takes the plugin without a word and never runs it, so the install would
// report a mod that reports nothing.
var modsSince = [3]int{2, 1, 287}

// claudeLoadsMods says why the Claude Code on the PATH cannot load the mod,
// nil where it can. A version it cannot read counts as one that cannot: the
// hook stays then, which costs a process per tool call, where taking it would
// cost the reporting.
func (a *app) claudeLoadsMods() error {
	output, err := a.runClaude([]string{"--version"})
	if err != nil {
		return err
	}
	version, readable := claudeVersionOf(output)
	minimum := fmt.Sprintf("%d.%d.%d", modsSince[0], modsSince[1], modsSince[2])
	if !readable {
		return cannotLoadMods(fmt.Sprintf("the version of Claude Code cannot be read from %q, and only %s and later load mods", strings.TrimSpace(output), minimum))
	}
	for i := range version {
		if version[i] != modsSince[i] {
			if version[i] < modsSince[i] {
				return cannotLoadMods(fmt.Sprintf("Claude Code %d.%d.%d cannot load mods; update it to %s or later", version[0], version[1], version[2], minimum))
			}
			break
		}
	}
	return nil
}

// cannotLoadMods is a Claude Code that runs and would take the mod, and never
// load it. Telling it of the mod helps nobody; updating it does.
type cannotLoadMods string

func (c cannotLoadMods) Error() string { return string(c) }

// aVersion is a word that is a version: three numbers, and a pre-release or a
// build behind them, which says nothing about whether mods load.
var aVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+]\S*)?$`)

// claudeVersionOf reads the version from what claude --version prints, such
// as "2.1.289 (Claude Code)": the first word that is one, so that a line of a
// warning before it does not hide it. A word such as "v2.1.289" is not read:
// claude does not print one, and a version that cannot be read keeps the hook,
// which is the safe side.
func claudeVersionOf(output string) ([3]int, bool) {
	var version [3]int
	for _, word := range strings.Fields(output) {
		found := aVersion.FindStringSubmatch(word)
		if found == nil {
			continue
		}
		for i := range version {
			number, err := strconv.Atoi(found[i+1])
			if err != nil {
				return version, false
			}
			version[i] = number
		}
		return version, true
	}
	return version, false
}

// modInstall is what became of an install of the mod: where it lies, which
// minka it runs, whether Claude Code was told of it, and what became of the
// hook of minka in the settings of Claude Code.
type modInstall struct {
	dir     string
	program string
	state   string
	// untold says why Claude Code was not told of the mod, nil where it was.
	untold error
	hook   oldHook
}

func (m modInstall) data() map[string]any {
	data := map[string]any{"path": m.dir, "plugin": mod.Plugin, "state": m.state, "program": m.program}
	if m.untold != nil {
		data["reason"] = m.untold.Error()
	}
	return data
}

// notices are what an install has to say beyond its state: a program that is
// not there, and a hook that could not go.
func (m modInstall) notices() []string {
	var notices []string
	if m.state != modSkipped && !theProgramIsThere(m.program) {
		notices = append(notices, "the mod runs "+m.program+", and there is no such program; it reports nothing and says nothing about it")
	}
	if !m.hook.found() && m.hook.why != "" {
		notices = append(notices, m.hook.why+", so whether it still carries the old hook of minka cannot be told; where it does, take it out of hooks."+oldHookEvent+" by hand")
	}
	if m.hook.found() && m.state == modSkipped {
		notices = append(notices, "the old hook in "+m.hook.path+" runs "+m.hook.command+" after every tool call; it is a leftover, and minka hooks install takes it out")
	}
	if m.hook.found() && !m.hook.removed && m.state != modSkipped {
		notices = append(notices, "the old hook in "+m.hook.path+" runs "+m.hook.command+" after every tool call and could not be removed: "+m.hook.why+"; take it out of hooks."+oldHookEvent+" by hand")
	}
	return notices
}

// installTheMod writes the mod beside the configuration, has Claude Code
// install it from there, and then takes the hook of minka out of the settings
// of Claude Code. It fails only where the mod cannot be written; whether
// Claude Code was told of it is in the answer.
//
// Every install writes what differs from the files of this minka, so that the
// mod a machine runs is the one of the minka that last installed it. Claude
// Code loads a plugin of a directory marketplace from that directory, so the
// new files take effect with the next session, with no update in Claude Code.
//
// Claude Code is told only where its version loads mods, and the hook goes
// only once it has been told: a session left with neither would report
// nothing. It goes after Claude Code is told, because telling it writes the
// same settings file, and what it wrote has to be kept.
func (a *app) installTheMod() (modInstall, error) {
	dir, err := a.modPath()
	if err != nil {
		return modInstall{}, err
	}
	installed := modInstall{dir: dir, program: a.minkaProgram(), state: modCurrent}
	files, err := mod.Files(installed.program)
	if err != nil {
		return modInstall{}, &Error{Code: CodeConfig, Message: err.Error()}
	}
	changed, err := writeTheMod(dir, files)
	if err != nil {
		return modInstall{}, &Error{Code: CodeConfig, Message: err.Error()}
	}
	if changed {
		installed.state = modInstalled
	}

	if err := a.claudeLoadsMods(); err != nil {
		installed.state, installed.untold = modWritten, err
		return installed, nil
	}
	for _, args := range claudeCommands(dir) {
		if _, err := a.runClaude(args); err != nil {
			installed.state, installed.untold = modWritten, err
			return installed, nil
		}
	}

	path, err := a.settingsPath()
	if err != nil {
		return installed, nil
	}
	installed.hook, err = removeTheOldHook(path)
	if err != nil {
		installed.hook.why = err.Error()
	}
	return installed, nil
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
// Claude Code to load it. Settings that cannot be read tell it nothing.
func (a *app) modStatus(settingsPath string) (map[string]any, string, error) {
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
	var settings map[string]any
	if content, err := os.ReadFile(settingsPath); err == nil && json.Unmarshal(content, &settings) == nil {
		if plugins, ok := settings["enabledPlugins"].(map[string]any); ok {
			enabled, _ = plugins[mod.Plugin].(bool)
		}
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
	return map[string]any{"path": dir, "plugin": mod.Plugin, "state": state, "enabled": enabled, "settings": settingsPath}, summary, nil
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
