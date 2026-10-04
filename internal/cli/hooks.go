package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) newHooksCmd() *cobra.Command {
	return group("hooks", "Install the mod that reports what you are working on",
		a.newHooksInstallCmd(),
		a.newHooksStatusCmd(),
	)
}

func (a *app) newHooksInstallCmd() *cobra.Command {
	var withTheMod bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the mod that runs minka heartbeat inside Claude Code",
		Long: `Installs the mod: code that runs inside the Claude Code process and runs
minka heartbeat at the start of every turn, after every tool call and every
thirty seconds, for as long as that process runs. When the process ends, so do
its beats, and the board forgets its cards three minutes after the last one.

minka writes the mod beside its configuration and runs 'claude plugin
marketplace add' and 'claude plugin install' for it, for every project. Running
it again writes the mod of this minka, and Claude Code loads it in every
session started after that. Where claude cannot be run, the mod is written all
the same and the hint carries the two command lines to run by hand.

Only Claude Code 2.1.287 and later load mods. An older one is not told of the
mod: it would take the plugin and never run it.

Once Claude Code has the mod, the hook an earlier minka wrote into
~/.claude/settings.json, or settings.json in $CLAUDE_CONFIG_DIR, is taken out:
that entry and nothing else, the rest of the file left as it was written.
'minka setup' does all of this as well.`,
		Example: `  minka hooks install`,
		Args:    requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			installed, err := a.installTheMod()
			if err != nil {
				return err
			}
			if installed.untold != nil {
				hint := "Run these yourself:\n" + strings.Join(claudeCommandLines(installed.dir), "\n")
				var tooOld cannotLoadMods
				if errors.As(installed.untold, &tooOld) {
					hint = "Update Claude Code, then run: minka hooks install"
				}
				return &Error{
					Code:    CodeConfig,
					Message: "the mod is written to " + installed.dir + ", and Claude Code was not told of it: " + installed.untold.Error(),
					Hint:    hint,
				}
			}

			summary := "The mod in " + installed.dir + " is current and installed in Claude Code"
			if installed.state == modInstalled {
				summary = "Wrote the mod to " + installed.dir + " and installed it in Claude Code"
			}
			if installed.hook.removed {
				summary += "; removed the old hook from " + installed.hook.path
			}
			return a.ok(success{
				data:    map[string]any{"mod": installed.data(), "oldHook": installed.hook.data()},
				summary: summary,
				notices: append([]string{"the mod takes effect in Claude Code sessions started from now on"}, installed.notices()...),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s: %s\n", installed.dir, installed.state)
					if installed.hook.removed {
						fmt.Fprintf(w, "Removed the old hook from %s\n", installed.hook.path)
					}
				},
			})
		},
	}
	// Scripts written for the time the mod had to be asked for keep working.
	cmd.Flags().BoolVar(&withTheMod, "mod", false, "Install the mod; it is what install does anyway")
	_ = cmd.Flags().MarkHidden("mod")
	return cmd
}

func (a *app) newHooksStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Tell whether the mod reports what you are working on",
		Long: `Tells whether the mod of 'minka hooks install' is written and loaded: 'state'
is current where it is what this minka writes, stale where another minka wrote
it, and 'enabled' says whether the settings of Claude Code tell it to load the
mod. Where the Claude Code on the PATH is too old to load mods at all,
'reason' says so.

'oldHook' names the hook an earlier minka wrote into those settings, where it
is still there: a leftover that runs minka after every tool call beside the
mod. 'minka hooks install' takes it out.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			path, err := a.settingsPath()
			if err != nil {
				return err
			}
			modState, summary, err := a.modStatus(path)
			if err != nil {
				return err
			}
			var notices []string
			var tooOld cannotLoadMods
			if errors.As(a.claudeLoadsMods(), &tooOld) {
				modState["reason"] = tooOld.Error()
				summary = fmt.Sprintf("The mod in %s is %s, and %s", modState["path"], modState["state"], tooOld)
				notices = append(notices, tooOld.Error()+"; until then nothing reports from inside Claude Code")
			}
			hook, err := findTheOldHook(path)
			if err != nil {
				notices = append(notices, err.Error()+", so whether it still carries the old hook of minka cannot be told")
			}
			if hook.found() {
				summary += ". The old hook in " + path + " is still there"
				notices = append(notices, "the old hook in "+path+" runs "+hook.command+" after every tool call; it is a leftover, and minka hooks install takes it out")
			}

			s := success{
				data:    map[string]any{"mod": modState, "oldHook": hook.data()},
				summary: summary,
				notices: notices,
				text: func(w io.Writer) {
					loaded := "not enabled"
					if modState["enabled"] == true {
						loaded = "enabled"
					}
					fmt.Fprintf(w, "%s: %s, %s\n", modState["path"], modState["state"], loaded)
					if hook.found() {
						fmt.Fprintf(w, "%s: old hook %s\n", path, hook.command)
					}
				},
			}
			if modState["state"] != modCurrent || modState["enabled"] != true || hook.found() {
				s.breadcrumbs = append(s.breadcrumbs, Breadcrumb{Action: "install", Cmd: "minka hooks install", Description: "Write the mod, install it in Claude Code and take out the old hook"})
			}
			return a.ok(s)
		},
	}
	return cmd
}

// theProgramIsThere says whether a program, named by its path or found on the
// PATH, can be run. A path is taken whole, spaces and all.
func theProgramIsThere(program string) bool {
	if !strings.ContainsRune(program, filepath.Separator) {
		_, err := exec.LookPath(program)
		return err == nil
	}
	info, err := os.Stat(program)
	return err == nil && !info.IsDir()
}

// settingsPath is the settings file Claude Code reads for every project, which
// lies in $CLAUDE_CONFIG_DIR where that is set.
func (a *app) settingsPath() (string, error) {
	if dir := a.getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return "", &Error{Code: CodeConfig, Message: err.Error()}
		}
		return filepath.Join(absolute, "settings.json"), nil
	}
	home := a.getenv("HOME")
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return "", &Error{Code: CodeConfig, Message: "cannot find the home directory: " + err.Error()}
		}
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}
