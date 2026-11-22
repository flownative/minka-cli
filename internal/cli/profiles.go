package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/spf13/cobra"
)

// Where the account of a command comes from, strongest first.
const (
	chosenByProfileFlag = "--profile"
	chosenByHostFlag    = "--host"
	chosenByProfileEnv  = "$MINKA_PROFILE"
	chosenByHostEnv     = "$MINKA_HOST"
	chosenByDefault     = "default"

	// chosenByStore is the account a beat reports for because the list of its
	// session lies there.
	chosenByStore = "the list of the session"
)

// choice is the account a command works with.
type choice struct {
	profile  string // empty where no profile stands for the account
	host     string // as a sign-in writes it; empty where nothing names one
	source   string
	envToken string // $MINKA_TOKEN, where it is the token of the choice
}

// key is what the state of the account on this machine is kept under: the name
// of its profile, else its host. A profile name never reads as a host, so the
// two cannot meet.
func (c choice) key() string {
	if c.profile != "" && c.envToken == "" {
		return c.profile
	}
	return c.host
}

// requestedChoice says where a command is told its account from, before the
// configuration is read: --profile, then --host, then $MINKA_PROFILE, then
// $MINKA_HOST, else the default profile. A flag outweighs every variable of the
// environment, and the two flags together contradict each other.
func (a *app) requestedChoice() (source, value string, err error) {
	if a.profileFlag != "" && a.hostFlag != "" {
		return "", "", &Error{Code: CodeUsage, Message: "--profile and --host both name the account of a command", Hint: "Give one of them: a profile names its host itself."}
	}
	switch {
	case a.profileFlag != "":
		return chosenByProfileFlag, a.profileFlag, nil
	case a.hostFlag != "":
		return chosenByHostFlag, a.hostFlag, nil
	case a.getenv("MINKA_PROFILE") != "":
		return chosenByProfileEnv, a.getenv("MINKA_PROFILE"), nil
	case a.getenv("MINKA_HOST") != "":
		return chosenByHostEnv, a.getenv("MINKA_HOST"), nil
	default:
		return chosenByDefault, "", nil
	}
}

// chooseAccount finds the account of a command, and the configuration where it
// was read. The configuration is read only where it is needed: $MINKA_HOST and
// $MINKA_TOKEN sign a command in without one.
//
// $MINKA_TOKEN belongs to $MINKA_HOST. It is the token of a command whose host
// comes from --host or $MINKA_HOST and is that host, and of no other: a profile
// names an account of its own, which may be another one on the same host.
// Without useEnvToken the token is passed over, for a command about the
// configuration itself.
func (a *app) chooseAccount(useEnvToken bool) (choice, string, *config.File, error) {
	source, value, err := a.requestedChoice()
	if err != nil {
		return choice{}, "", nil, err
	}
	envToken := a.getenv("MINKA_TOKEN")
	if useEnvToken && envToken != "" {
		envHostValue := a.getenv("MINKA_HOST")
		if envHostValue == "" {
			return choice{}, "", nil, &Error{Code: CodeUsage, Message: "$MINKA_TOKEN is set without $MINKA_HOST", Hint: "Set $MINKA_HOST to the host the token belongs to."}
		}
		envHost, err := config.NormalizeHost(envHostValue)
		if err != nil {
			return choice{}, "", nil, &Error{Code: CodeUsage, Message: "$MINKA_HOST: " + err.Error()}
		}
		if source == chosenByProfileFlag || source == chosenByProfileEnv {
			a.noteOnce("the token in $MINKA_TOKEN was not used: it belongs to $MINKA_HOST, and " + source + " names a profile, which signs in with its own token")
		}
		if source == chosenByHostFlag || source == chosenByHostEnv {
			host, err := config.NormalizeHost(value)
			if err != nil {
				return choice{}, "", nil, &Error{Code: CodeUsage, Message: err.Error()}
			}
			if host == envHost {
				return choice{host: host, source: source, envToken: envToken}, "", nil, nil
			}
		}
	}

	path, file, err := a.loadConfig()
	if err != nil {
		return choice{}, "", nil, err
	}
	c, err := chooseIn(file, path, source, value)
	return c, path, file, err
}

// chooseIn finds the account of a command in the configuration.
//
// A host stands for the one profile on it. Where several are, it stands for the
// default profile if that is among them, and for none otherwise: which account
// writes in an organization is not to be guessed. A host without a profile is a
// host not signed in to.
func chooseIn(file *config.File, path, source, value string) (choice, error) {
	switch source {
	case chosenByProfileFlag, chosenByProfileEnv:
		if err := config.CheckProfileName(value); err != nil {
			return choice{}, profileNameError(source, err)
		}
		return profileChoice(file, path, value, source)
	case chosenByHostFlag, chosenByHostEnv:
		host, err := config.NormalizeHost(value)
		if err != nil {
			return choice{}, &Error{Code: CodeUsage, Message: err.Error()}
		}
		names := file.ProfilesOnHost(host)
		switch {
		case len(names) == 0:
			return choice{host: host, source: source}, nil
		case len(names) == 1:
			return choice{profile: names[0], host: host, source: source}, nil
		}
		if name, ok := file.HostProfile(host); ok && name == file.DefaultProfile {
			return choice{profile: name, host: host, source: source}, nil
		}
		return choice{}, &Error{
			Code:    CodeAmbiguous,
			Message: fmt.Sprintf("%d profiles sign in to %s: %s", len(names), host, strings.Join(names, ", ")),
			Hint:    "Choose one with --profile <name>, or set $MINKA_PROFILE.",
		}
	default:
		if file.DefaultProfile == "" {
			return choice{source: source}, nil
		}
		if _, ok := file.Profiles[file.DefaultProfile]; !ok {
			return choice{}, &Error{
				Code:    CodeConfig,
				Message: fmt.Sprintf("the default profile %q is not among the profiles in %s", file.DefaultProfile, path),
				Hint:    "Make one of them the default: minka profiles default <name>",
			}
		}
		return profileChoice(file, path, file.DefaultProfile, source)
	}
}

// profileChoice is the choice of a profile by its name. The host is read the
// way a sign-in writes it, so that every process makes the same key of it, and
// one that cannot be read is refused rather than carried into everything that
// prints a host.
func profileChoice(file *config.File, path, name, source string) (choice, error) {
	profile, ok := file.Profiles[name]
	if !ok {
		return choice{}, unknownProfileError(file, name)
	}
	host, err := config.NormalizeHost(profile.Host)
	if err != nil {
		return choice{}, &Error{
			Code:    CodeConfig,
			Message: fmt.Sprintf("the host of the profile %s in %s cannot be read: %s", name, path, err),
			Hint:    "Correct \"host\" of that profile in the file, or take the profile out: minka profiles remove " + name,
		}
	}
	return choice{profile: name, host: host, source: source}, nil
}

func profileNameError(source string, err error) *Error {
	return &Error{Code: CodeUsage, Message: source + ": " + err.Error(), Hint: "A profile name is lower case letters, digits, \"-\" and \".\", such as flownative; minka profiles list shows them."}
}

// profileNamed refuses a name that is no profile name, before it is said back
// as one, and a profile that is not there.
func profileNamed(file *config.File, what, name string) error {
	if err := config.CheckProfileName(name); err != nil {
		return profileNameError(what, err)
	}
	if _, ok := file.Profiles[name]; !ok {
		return unknownProfileError(file, name)
	}
	return nil
}

func unknownProfileError(file *config.File, name string) *Error {
	e := &Error{Code: CodeUsage, Message: "there is no profile " + name}
	if names := file.ProfileNames(); len(names) > 0 {
		e.Hint = "The profiles are: " + strings.Join(names, ", ") + ". A new one is made by signing in: minka auth login --profile " + name + " --host <url>"
	} else {
		e.Hint = "There are no profiles yet. A profile is made by signing in: minka auth login --profile " + name + " --host <url>"
	}
	return e
}

// defaultToReport is the default profile and its host, for a report that names
// them beside the account a command uses. Both are empty where there is none.
// Where it or the configuration cannot be read, they are empty as well and the
// problem says why: a command given its account otherwise gets past both, while
// every command without one fails on them.
func (a *app) defaultToReport() (profile, host, problem string) {
	path, file, err := a.loadConfig()
	if err != nil {
		return "", "", err.Error()
	}
	if file.DefaultProfile == "" {
		return "", "", ""
	}
	c, err := chooseIn(file, path, chosenByDefault, "")
	if err != nil {
		return "", "", err.Error()
	}
	return c.profile, c.host, ""
}

// accountInWords says which account a command uses and how it stands to the
// default profile, naming the default only where the two differ.
func accountInWords(c choice, defaultProfile, problem string) string {
	var account string
	switch {
	case c.envToken != "":
		account = "the token of $MINKA_TOKEN goes to " + c.host + ", named by " + c.source
	case c.profile == "":
		account = "no profile signs in to " + c.host + ", named by " + c.source
	case c.source == chosenByDefault:
		return "the profile " + c.profile + " is the default profile"
	case c.profile == defaultProfile:
		return "the profile " + c.profile + " comes from " + c.source + " and is the default profile"
	default:
		account = "the profile " + c.profile + " comes from " + c.source
	}
	switch {
	case problem != "":
		return account + ", and the default profile cannot be read: " + problem
	case defaultProfile == "":
		return account + ", and no default profile is set"
	default:
		return account + ", the default profile is " + defaultProfile
	}
}

// noAccountError is the refusal of a command that names no account where the
// configuration names no default profile either. Nothing picks a profile by
// itself, because the default is a choice and not what happened to be left over.
func noAccountError(file *config.File) *Error {
	if len(file.Profiles) == 0 {
		return &Error{Code: CodeAuth, Message: "not signed in to any Minka host", Hint: "Run: minka auth login --host https://minka.example.com"}
	}
	return &Error{
		Code:    CodeAuth,
		Message: "no default profile is set",
		Hint:    "Give the profile with --profile <name>, or make one the default: minka profiles default <name>. The profiles are: " + strings.Join(file.ProfileNames(), ", "),
	}
}

func (a *app) newProfilesCmd() *cobra.Command {
	return group("profiles", "List, rename and remove the profiles you sign in with, and choose the default",
		a.newProfilesListCmd(),
		a.newProfilesRenameCmd(),
		a.newProfilesRemoveCmd(),
		a.newProfilesDefaultCmd(),
	)
}

func (a *app) newProfilesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the profiles, the default one and the one a command uses now",
		Long: `Shows every profile: its name, its host, the account it signed in as, whether it
is the default, and which one a command run here now uses and why — --profile,
--host, $MINKA_PROFILE, $MINKA_HOST or the default.

It asks nothing of the network. 'active' is null where no profile stands for
the account of a command, such as one signed in through $MINKA_TOKEN, and
'activeProblem' says why where the choice fails.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			path, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			c, _, _, chooseErr := a.chooseAccount(true)
			problem := ""
			if chooseErr != nil {
				problem = chooseErr.Error()
			}

			rows := []map[string]any{}
			for _, name := range file.ProfileNames() {
				profile := file.Profiles[name]
				rows = append(rows, map[string]any{
					"name": name, "host": config.HostForOutput(profile.Host),
					"personName": nullable(profile.PersonName), "emailAddress": nullable(profile.EmailAddress),
					"isDefault": name == file.DefaultProfile, "isActive": chooseErr == nil && c.envToken == "" && name == c.profile,
				})
			}
			var active any
			if chooseErr == nil && c.profile != "" && c.envToken == "" {
				active = map[string]any{"profile": c.profile, "source": c.source}
			}
			summary := plural(len(rows), "profile", "profiles")
			return a.ok(success{
				data: map[string]any{
					"profiles": rows, "defaultProfile": nullable(file.DefaultProfile), "active": active,
					"activeSource": nullable(c.source), "activeProblem": nullable(problem), "configPath": path,
				},
				summary: summary,
				text: func(w io.Writer) {
					if len(rows) == 0 {
						fmt.Fprintln(w, "No profile. Sign in with: minka auth login --host <url>")
						return
					}
					for _, name := range file.ProfileNames() {
						profile := file.Profiles[name]
						marks := ""
						if name == file.DefaultProfile {
							marks += " (default)"
						}
						if chooseErr == nil && c.envToken == "" && name == c.profile {
							marks += " (active: " + c.source + ")"
						}
						account := profile.PersonName
						if profile.EmailAddress != "" {
							account += " <" + profile.EmailAddress + ">"
						}
						fmt.Fprintf(w, "%s  %s  %s%s\n", name, config.HostForOutput(profile.Host), account, marks)
					}
					switch {
					case problem != "":
						fmt.Fprintf(w, "\nNo profile is active: %s\n", problem)
					case c.envToken != "":
						fmt.Fprintf(w, "\nNo profile is active: the token of $MINKA_TOKEN goes to %s.\n", c.host)
					}
				},
			})
		},
	}
}

func (a *app) newProfilesRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Give a profile another name",
		Long: `Gives a profile another name. It stays the default where it was, and the cards
it holds in the list of 'minka work start' go along with it.`,
		Example: "  minka profiles rename minka.flownative.com flownative",
		Args:    requireArgs("old", "new"),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, to := args[0], args[1]
			if err := config.CheckProfileName(to); err != nil {
				return profileNameError("the new name", err)
			}
			path, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			if err := profileNamed(file, "the profile", from); err != nil {
				return err
			}
			if from == to {
				return usageErrorf(cmd, "the profile is called %s already", to)
			}
			if _, taken := file.Profiles[to]; taken {
				return &Error{Code: CodeUsage, Message: "there is a profile " + to + " already", Hint: "Choose another name, or take that profile out first: minka profiles remove " + to}
			}
			if err := file.RenameProfile(from, to); err != nil {
				return &Error{Code: CodeUsage, Message: err.Error()}
			}
			if err := file.Save(path); err != nil {
				return &Error{Code: CodeConfig, Message: err.Error()}
			}
			// The store follows the configuration, so that what it writes for an
			// older version per host is the lists of the profile by its new name.
			var notices []string
			if err := a.renameInTheStore(from, to); err != nil {
				notices = append(notices, "the cards of the profile could not be moved to its new name, so its list starts empty: "+err.Error())
			}
			summary := "Renamed the profile " + from + " to " + to
			return a.ok(success{
				data:    map[string]any{"from": from, "to": to, "isDefault": file.DefaultProfile == to},
				notices: notices,
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newProfilesRemoveCmd() *cobra.Command {
	var revoke bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a profile and its token, and revoke the token with --revoke",
		Long: `Removes a profile and its token from the config file, as 'minka auth logout'
does for the profile a command uses. Without --revoke the token itself stays
valid on the server until it is revoked there.

--revoke revokes the token on the server first, and only then removes the
profile: it holds the one copy of the token this machine has, so it is kept
when the revocation fails, and the command exits with the code of that failure.

Removing the default profile leaves no default: the next sign-in sets one, or
minka profiles default <name> names one of the profiles left.`,
		Example: "  minka profiles remove familie --revoke",
		Args:    requireArgs("name"),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			if err := profileNamed(file, "the profile", args[0]); err != nil {
				return err
			}
			return a.removeProfile(path, file, args[0], revoke)
		},
	}
	cmd.Flags().BoolVar(&revoke, "revoke", false, "Revoke the token on the server before removing the profile")
	return cmd
}

func (a *app) newProfilesDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <name>",
		Short: "Make a profile the default",
		Long: `Makes a profile the default: the one a command uses that is given neither
--profile nor --host, and finds neither $MINKA_PROFILE nor $MINKA_HOST.`,
		Example: "  minka profiles default flownative",
		Args:    requireArgs("name"),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			if err := profileNamed(file, "the profile", args[0]); err != nil {
				return err
			}
			previous := file.DefaultProfile
			file.DefaultProfile = args[0]
			if err := file.Save(path); err != nil {
				return &Error{Code: CodeConfig, Message: err.Error()}
			}
			summary := args[0] + " is the default profile"
			return a.ok(success{
				data:    map[string]any{"defaultProfile": args[0], "previousDefaultProfile": nullable(previous), "host": config.HostForOutput(file.Profiles[args[0]].Host)},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}
