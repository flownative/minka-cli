package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	// hookEvent is when the signal goes out: after every tool call of the
	// agent, which is as often as anything happens in a session at all.
	hookEvent = "PostToolUse"

	// hookMatcher is every tool.
	hookMatcher = "*"

	// hookTimeout is what the hook may take, in seconds. The command bounds
	// its own request far below this; the entry is there for the case where
	// something else holds it up.
	hookTimeout = 10

	hookInstalled    = "installed"
	hookCurrent      = "current"
	hookNotInstalled = "not installed"
)

func (a *app) newHooksCmd() *cobra.Command {
	return group("hooks", "Install the hook that reports what you are working on",
		a.newHooksInstallCmd(),
		a.newHooksStatusCmd(),
	)
}

func (a *app) newHooksInstallCmd() *cobra.Command {
	var file, command string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Write the hook that runs minka heartbeat after every tool call",
		Long: `Writes a hook into the settings of Claude Code.

Without --file that is ~/.claude/settings.json, where it holds for every
project, which is where it belongs: every session on the machine reports its
own cards through it, whatever project it was started in.

--file names another file, such as .claude/settings.local.json for one project
alone. Not .claude/settings.json: that one is meant to be checked in, and the
entry names a path of this machine, which no other checkout has. Where the
file lies in a working tree of git, minka says so; --command minka writes a
bare command instead of a path.

The file belongs to whoever wrote it: everything in it is kept, the hook is
added beside whatever else is there, and a file that carries this hook already
is left alone. A file that is not a JSON object, or whose hooks are not shaped
the way Claude Code writes them, is refused rather than repaired: the hint then
carries the entry to add by hand.

The entry names the minka that writes it by its full path, so the hook does
not depend on what is on the PATH of whoever starts the agent. --command
writes another one.`,
		Example: `  minka hooks install
  minka hooks install --file .claude/settings.local.json`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			path, err := a.settingsPath(file)
			if err != nil {
				return err
			}
			if command == "" {
				command = a.heartbeatCommand()
			}
			settings, err := readSettings(path, command)
			if err != nil {
				return err
			}

			found, err := heartbeatHookIn(settings, command)
			if err != nil {
				return refusedSettings(path, err, command)
			}
			if found != "" {
				return a.ok(hookSuccess(path, hookCurrent, found, "The hook is already in "+path))
			}

			if err := addHeartbeatHook(settings, command); err != nil {
				return refusedSettings(path, err, command)
			}
			if err := writeSettings(path, settings); err != nil {
				return &Error{Code: CodeConfig, Message: err.Error()}
			}
			return a.ok(hookSuccess(path, hookInstalled, command, "Wrote the hook to "+path))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Settings file to write into (default: ~/.claude/settings.json)")
	cmd.Flags().StringVar(&command, "command", "", "Command line the hook runs (default: this minka and 'heartbeat')")
	return cmd
}

func (a *app) newHooksStatusCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Tell whether the hook reports what you are working on",
		Args:  requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			path, err := a.settingsPath(file)
			if err != nil {
				return err
			}
			settings, err := readSettings(path, a.heartbeatCommand())
			if err != nil {
				return err
			}
			found, err := heartbeatHookIn(settings, a.heartbeatCommand())
			if err != nil {
				return refusedSettings(path, err, a.heartbeatCommand())
			}

			state, summary := hookNotInstalled, "No hook in "+path+" reports what you are working on"
			if found != "" {
				state, summary = hookCurrent, "The hook in "+path+" runs "+found
			}
			s := hookSuccess(path, state, found, summary)
			if state != hookCurrent {
				s.breadcrumbs = []Breadcrumb{{Action: "install", Cmd: "minka hooks install", Description: "Write the hook"}}
			}
			return a.ok(s)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Settings file to look into (default: ~/.claude/settings.json)")
	return cmd
}

func hookSuccess(path, state, command, summary string) success {
	s := success{
		data:    map[string]any{"path": path, "state": state, "event": hookEvent, "command": nullable(command)},
		summary: summary,
		text:    func(w io.Writer) { fmt.Fprintf(w, "%s: %s\n", path, state) },
	}
	if command != "" && !theProgramIsThere(command) {
		s.notices = append(s.notices, "the hook runs "+command+", and there is no such program; it reports nothing and says nothing about it")
	}
	if tree := workingTreeOf(path); tree != "" {
		s.notices = append(s.notices, path+" lies in the git working tree "+tree+
			"; a hook that names a path of this machine is no use to another checkout, so keep the file out of what is committed, or write a bare command with --command minka")
	}
	return s
}

// theProgramIsThere says whether the command of a hook can be run at all. A
// binary that moved leaves a hook that fails after every tool call without a
// word, and a status that says the hook is there would be the last place
// anybody looks.
func theProgramIsThere(command string) bool {
	words := strings.Fields(command)
	if len(words) == 0 {
		return false
	}
	program := strings.Trim(words[0], `"'`)
	if !strings.ContainsRune(program, filepath.Separator) {
		_, err := exec.LookPath(program)
		return err == nil
	}
	info, err := os.Stat(program)
	return err == nil && !info.IsDir()
}

// workingTreeOf names the git working tree a file lies in, and nothing where
// it lies in none.
func workingTreeOf(path string) string {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// settingsPath is the settings file a command works on: the one named, or the
// one Claude Code reads for every project.
func (a *app) settingsPath(named string) (string, error) {
	if named != "" {
		absolute, err := filepath.Abs(named)
		if err != nil {
			return "", &Error{Code: CodeConfig, Message: err.Error()}
		}
		return absolute, nil
	}
	home := a.getenv("HOME")
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return "", &Error{Code: CodeConfig, Message: "cannot find the home directory: " + err.Error(), Hint: "Name the settings file with --file."}
		}
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// heartbeatCommand is the command line the hook runs: this very binary, by the
// path it was started from. A hook that cannot find its command fails after
// every tool call and says nothing about it.
func (a *app) heartbeatCommand() string {
	binary, err := os.Executable()
	if err != nil {
		return "minka heartbeat"
	}
	return binary + " heartbeat"
}

// readSettings reads the settings, an empty set where there is no file yet. A
// file that is no JSON object at all is refused with the entry to add by hand,
// the same way one of an unknown shape is: whoever holds such a file needs
// that entry most of all.
func readSettings(path, command string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, &Error{Code: CodeConfig, Message: err.Error()}
	}
	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil || settings == nil {
		return nil, refusedSettings(path, shapeError("it is not a JSON object"), command)
	}
	return settings, nil
}

// errMangledSettings marks a settings file shaped in a way this command does
// not write into. It is a refusal on purpose: the file belongs to whoever
// wrote it, and a hook added to something that is not the list Claude Code
// reads would be lost at best.
var errMangledSettings = errors.New("settings")

func shapeError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errMangledSettings, fmt.Sprintf(format, args...))
}

func refusedSettings(path string, why error, command string) *Error {
	message := strings.TrimPrefix(why.Error(), errMangledSettings.Error()+": ")
	e := &Error{Code: CodeConfig, Message: path + ": " + message}
	if command != "" {
		e.Hint = "Add this to the settings by hand, beside whatever is already there:\n" + hookSnippet(command)
	}
	return e
}

// heartbeatHookIn finds the command of a hook of ours, and says nothing where
// the file carries none. A shape it does not know is an error rather than an
// answer: whether a hook is there cannot be told from a list that is none.
//
// Ours is a minka that reports, whatever path it was written with, and the
// command this minka would write itself, whatever it is called.
func heartbeatHookIn(settings map[string]any, mine string) (string, error) {
	groups, err := hookGroups(settings)
	if err != nil {
		return "", err
	}
	for _, entry := range groups {
		group, ok := entry.(map[string]any)
		if !ok {
			return "", shapeError("an entry of hooks.%s is not an object", hookEvent)
		}
		hooks, ok := group["hooks"]
		if !ok {
			continue
		}
		list, ok := hooks.([]any)
		if !ok {
			return "", shapeError("the hooks of an entry of hooks.%s are not a list", hookEvent)
		}
		for _, one := range list {
			hook, ok := one.(map[string]any)
			if !ok {
				return "", shapeError("a hook of hooks.%s is not an object", hookEvent)
			}
			if command, ok := hook["command"].(string); ok && (isHeartbeat(command) || command == mine) {
				return command, nil
			}
		}
	}
	return "", nil
}

func hookGroups(settings map[string]any) ([]any, error) {
	hooks, ok := settings["hooks"]
	if !ok || hooks == nil {
		return nil, nil
	}
	all, ok := hooks.(map[string]any)
	if !ok {
		return nil, shapeError("hooks is not an object")
	}
	event, ok := all[hookEvent]
	if !ok || event == nil {
		return nil, nil
	}
	groups, ok := event.([]any)
	if !ok {
		return nil, shapeError("hooks.%s is not a list", hookEvent)
	}
	return groups, nil
}

// isHeartbeat says whether a command line is a minka reporting what it is
// working on, whatever path it was written with.
func isHeartbeat(command string) bool {
	words := strings.Fields(command)
	if len(words) < 2 {
		return false
	}
	if filepath.Base(strings.Trim(words[0], `"'`)) != "minka" {
		return false
	}
	for _, word := range words[1:] {
		if word == "heartbeat" {
			return true
		}
	}
	return false
}

// addHeartbeatHook puts the hook beside whatever the file holds already. A
// field that stands there as null is as good as one that is not there, the way
// reading it treats it: the two must not answer differently, or a status that
// advises installing would be followed by an install that refuses.
func addHeartbeatHook(settings map[string]any, command string) error {
	all, ok := settings["hooks"].(map[string]any)
	if !ok {
		if value, taken := settings["hooks"]; taken && value != nil {
			return shapeError("hooks is not an object")
		}
		all = map[string]any{}
		settings["hooks"] = all
	}
	groups, err := hookGroups(settings)
	if err != nil {
		return err
	}
	all[hookEvent] = append(groups, map[string]any{
		"matcher": hookMatcher,
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": command,
			"timeout": hookTimeout,
		}},
	})
	return nil
}

// writeSettings saves the settings where they lie, keeping the permissions of
// a file that is already there. A file behind a symbolic link is written where
// the link points and stays a link.
func writeSettings(path string, settings map[string]any) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}

	temp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer os.Remove(temp.Name())

	_, err = temp.Write(append(content, '\n'))
	if err == nil {
		err = temp.Chmod(mode)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func hookSnippet(command string) string {
	snippet := map[string]any{"hooks": map[string]any{hookEvent: []any{map[string]any{
		"matcher": hookMatcher,
		"hooks":   []any{map[string]any{"type": "command", "command": command, "timeout": hookTimeout}},
	}}}}
	encoded, err := json.MarshalIndent(snippet, "", "  ")
	if err != nil {
		return ""
	}
	return string(encoded)
}
