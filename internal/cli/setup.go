package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/spf13/cobra"
)

// newSetupCmd walks a person through the first steps. It asks questions, so it
// needs a terminal; an agent or a script takes the single steps, which need
// none.
func (a *app) newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Sign in and install the skill, step by step",
		Long: `Walks through the first steps on a terminal: asks for the Minka host, takes an
access token without echo, checks it against the host, saves it, and offers to
install the skill for agents.

Create the token on your account page in Minka, for an agent of yours. It is
shown once.

Without a terminal, take the single steps instead:
  minka auth login --host <url> --with-token < token.txt
  minka skill install`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !a.stdinIsTerminal() || !a.stdoutIsTerminal() {
				return &Error{
					Code:    CodeUsage,
					Message: "setup asks questions and needs a terminal",
					Hint:    "Take the single steps: minka auth login --host <url> --with-token < token.txt, then minka skill install",
				}
			}
			return a.runSetup(cmd)
		},
	}
	return cmd
}

func (a *app) runSetup(cmd *cobra.Command) error {
	input := bufio.NewReader(a.stdin)

	// A config file that cannot be used has to stop setup before the token is
	// asked for: a token is shown once, and one that cannot be saved is lost.
	path, file, err := a.loadConfig()
	if err != nil {
		return err
	}

	// Setup says itself what it does about the skill.
	a.skillIsTheTopic = true

	host, err := a.setupHost(input, hostToOfferAgain(file))
	if err != nil {
		return err
	}

	person, err := a.setupSignIn(cmd, host, path, file)
	if err != nil {
		return err
	}

	skillPath, skillState, err := a.setupSkill(input)
	if err != nil {
		return err
	}

	return a.ok(success{
		data: map[string]any{
			"host": host, "configPath": path, "person": person.Raw,
			"skill": map[string]string{"path": skillPath, "state": skillState},
		},
		summary: fmt.Sprintf("Signed in to %s as %s", host, person.Value.Name),
		breadcrumbs: []Breadcrumb{
			{Action: "projects", Cmd: "minka projects list", Description: "List your projects"},
			{Action: "mine", Cmd: "minka cards mine", Description: "List the cards you are assigned to"},
			{Action: "doctor", Cmd: "minka doctor", Description: "Check the setup"},
		},
		text: func(w io.Writer) {
			fmt.Fprintf(w, "\nAll set. Try:\n  minka projects list\n  minka cards mine\n  minka doctor\n")
		},
	})
}

// hostToOfferAgain is the host of the configuration where it can be read, and
// nothing where it cannot. An entry setup is there to repair is no answer to
// offer: taking it would only be refused, and putting it in the question would
// print a value that may hold anything.
func hostToOfferAgain(file *config.File) string {
	host, err := config.NormalizeHost(file.DefaultHost)
	if err != nil {
		return ""
	}

	return host
}

// setupHost is --host or $MINKA_HOST when given, else what the person types;
// an empty answer takes the host signed in to last.
func (a *app) setupHost(input *bufio.Reader, fallback string) (string, error) {
	if host, err := a.explicitHost(); err != nil || host != "" {
		return host, err
	}

	for {
		question := "Minka host, such as https://minka.example.com: "
		if fallback != "" {
			question = "Minka host [" + fallback + "]: "
		}
		answer, err := a.ask(input, question)
		if err != nil {
			return "", err
		}
		if answer == "" {
			answer = fallback
		}
		if answer == "" {
			continue
		}
		host, err := config.NormalizeHost(answer)
		if err != nil {
			fmt.Fprintf(a.stderr, "%s\n", err)
			continue
		}
		return host, nil
	}
}

// setupSignIn keeps a token that works and asks for one otherwise. The saved
// token it tries is the one of this host, never the one of another.
//
// Only a host that refuses the saved token is a reason to ask for a new one.
// A host that does not answer would not accept a new token either, and the
// person would have created one for nothing.
func (a *app) setupSignIn(cmd *cobra.Command, host, path string, file *config.File) (api.Result[api.Me], error) {
	if saved := file.Hosts[host].Token; saved != "" {
		me, err := api.New(host, saved, userAgent()).Me(a.ctx)
		if err == nil {
			fmt.Fprintf(a.stderr, "Already signed in to %s as %s.\n", host, me.Value.Name)
			if file.DefaultHost != host {
				file.DefaultHost = host
				if err := file.Save(path); err != nil {
					return api.Result[api.Me]{}, &Error{Code: CodeConfig, Message: err.Error()}
				}
			}
			return me, nil
		}
		if e := apiError(err, host); e.Code != CodeAuth {
			return api.Result[api.Me]{}, e
		}
		fmt.Fprintf(a.stderr, "%s no longer accepts the saved token.\n", host)
	}

	fmt.Fprintf(a.stderr, "Create an access token on your account page, %s/account, for an agent of yours.\nIt is shown once. Paste it here; it is not echoed.\n", host)
	token, err := a.readToken(cmd, host, false)
	if err != nil {
		return api.Result[api.Me]{}, err
	}
	path, me, err := a.signIn(host, token)
	if err != nil {
		return api.Result[api.Me]{}, err
	}
	fmt.Fprintf(a.stderr, "Signed in to %s as %s. The token is saved in %s.\n", host, me.Value.Name, path)
	return me, nil
}

// setupSkill offers to install the skill where that would change something
// and is possible; it never installs without a yes.
func (a *app) setupSkill(input *bufio.Reader) (string, string, error) {
	skillsDir, err := a.skillsDir("")
	if err != nil {
		fmt.Fprintf(a.stderr, "The skill is not installed: %s. Install it with: minka skill install --dir <dir>\n", err)
		return "", skillMissing, nil
	}
	path := filepath.Join(skillsDir, skillName, skillFile)
	state := skillState(path)
	if state == skillCurrent || firstSymlink(skillsDir, filepath.Join(skillsDir, skillName)) != "" {
		return path, state, nil
	}

	answer, err := a.ask(input, "Install the skill for Claude Code at "+path+"? [Y/n] ")
	if err != nil {
		return "", state, err
	}
	if answer != "" && !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		fmt.Fprintf(a.stderr, "Skipped. Install it any time with: minka skill install\n")
		return path, state, nil
	}
	if _, err := installSkill(skillsDir); err != nil {
		return "", state, err
	}
	fmt.Fprintf(a.stderr, "Installed the skill at %s.\n", path)
	return path, skillCurrent, nil
}

// ask prints a question to stderr, so stdout stays what the command prints,
// and reads one line.
func (a *app) ask(input *bufio.Reader, question string) (string, error) {
	fmt.Fprint(a.stderr, question)
	line, err := input.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(a.stderr)
		return "", &Error{Code: CodeUsage, Message: "the input ended before setup was done"}
	}
	return strings.TrimSpace(line), nil
}
