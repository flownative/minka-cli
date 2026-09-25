package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type catalogCommand struct {
	Command string        `json:"command"`
	Usage   string        `json:"usage"`
	Short   string        `json:"short"`
	Flags   []catalogFlag `json:"flags,omitempty"`
}

type catalogFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage"`
}

// newCommandsCmd lists every command, built from the command tree itself so
// that the catalog cannot drift from what exists.
func (a *app) newCommandsCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "List all commands with their flags",
		Args:  requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			var commands []catalogCommand
			var walk func(*cobra.Command)
			walk = func(parent *cobra.Command) {
				for _, cmd := range parent.Commands() {
					if cmd.Hidden || cmd.Name() == "help" {
						continue
					}
					if cmd.HasSubCommands() {
						walk(cmd)
						continue
					}
					commands = append(commands, catalogCommand{
						Command: cmd.CommandPath(),
						Usage:   cmd.UseLine(),
						Short:   cmd.Short,
						Flags:   flagsOf(cmd.LocalNonPersistentFlags()),
					})
				}
			}
			walk(root)
			globalFlags := flagsOf(root.PersistentFlags())

			return a.ok(success{
				data:    map[string]any{"commands": commands, "globalFlags": globalFlags},
				summary: plural(len(commands), "command", "commands"),
				text: func(w io.Writer) {
					for _, cmd := range commands {
						fmt.Fprintf(w, "%-50s %s\n", cmd.Usage, cmd.Short)
					}
				},
			})
		},
	}
}

func flagsOf(set *pflag.FlagSet) []catalogFlag {
	var flags []catalogFlag
	set.VisitAll(func(flag *pflag.Flag) {
		// A flag the help keeps to itself is minka's own business, and the
		// catalog is the help of whoever reads JSON.
		if flag.Name == "help" || flag.Hidden {
			return
		}
		defaultValue := flag.DefValue
		if defaultValue == "false" {
			defaultValue = ""
		}
		flags = append(flags, catalogFlag{
			Name:      flag.Name,
			Shorthand: flag.Shorthand,
			Type:      flag.Value.Type(),
			Default:   defaultValue,
			Usage:     flag.Usage,
		})
	})
	return flags
}
