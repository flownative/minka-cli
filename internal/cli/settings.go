package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/spf13/cobra"
)

// setting is one value of the configuration file that a command may read and
// change. Hosts and tokens are not among them: they are what auth is for.
type setting struct {
	help string
	get  func(*config.File) any
	set  func(*config.File, string) error
}

var settings = map[string]setting{
	"comments.requireRead": {
		help: "Refuse a comment on a card that has comments not yet shown on this machine (true or false, default false)",
		get:  func(f *config.File) any { return f.Comments.RequireRead },
		set: func(f *config.File, value string) error {
			on, err := strconv.ParseBool(value)
			if err != nil || value != strconv.FormatBool(on) {
				return fmt.Errorf("comments.requireRead is true or false, not %q", value)
			}
			f.Comments.RequireRead = on
			return nil
		},
	},
}

func settingNames() []string {
	names := make([]string, 0, len(settings))
	for name := range settings {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func settingsHelp() string {
	var lines []string
	for _, name := range settingNames() {
		lines = append(lines, "  "+name+"  "+settings[name].help)
	}
	return strings.Join(lines, "\n")
}

func (a *app) newConfigCmd() *cobra.Command {
	return group("config", "Read and change the settings of this machine",
		a.newConfigGetCmd(),
		a.newConfigSetCmd(),
	)
}

func lookUpSetting(cmd *cobra.Command, name string) (setting, error) {
	found, ok := settings[name]
	if !ok {
		return setting{}, usageErrorf(cmd, "there is no setting %q; there are: %s", name, strings.Join(settingNames(), ", "))
	}
	return found, nil
}

func (a *app) newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Show a setting",
		Long:  "Shows a setting of the configuration file. The settings:\n\n" + settingsHelp(),
		Args:  requireArgs("key"),
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := lookUpSetting(cmd, args[0])
			if err != nil {
				return err
			}
			_, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			value := found.get(file)
			return a.ok(success{
				data:    map[string]any{"key": args[0], "value": value},
				summary: fmt.Sprintf("%s is %v", args[0], value),
				text:    func(w io.Writer) { fmt.Fprintf(w, "%v\n", value) },
			})
		},
	}
}

func (a *app) newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "set <key> <value>",
		Short:   "Change a setting",
		Long:    "Changes a setting in the configuration file, for every host signed in to on\nthis machine. The settings:\n\n" + settingsHelp(),
		Example: "  minka config set comments.requireRead true",
		Args:    requireArgs("key", "value"),
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := lookUpSetting(cmd, args[0])
			if err != nil {
				return err
			}
			path, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			if err := found.set(file, args[1]); err != nil {
				return usageErrorf(cmd, "%s", err)
			}
			if err := file.Save(path); err != nil {
				return &Error{Code: CodeConfig, Message: err.Error()}
			}
			value := found.get(file)
			summary := fmt.Sprintf("%s is %v", args[0], value)
			return a.ok(success{
				data:    map[string]any{"key": args[0], "value": value},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}
