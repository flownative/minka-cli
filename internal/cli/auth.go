package cli

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/spf13/cobra"
)

func (a *app) newAuthCmd() *cobra.Command {
	return group("auth", "Sign in to and out of Minka hosts",
		a.newAuthLoginCmd(),
		a.newAuthStatusCmd(),
		a.newAuthLogoutCmd(),
	)
}

func (a *app) newAuthLoginCmd() *cobra.Command {
	var withToken bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to a Minka host with an access token",
		Long: `Signs in to a Minka host with a personal access token. The token is checked
against the host and then saved in the config file, which only your user can
read. The host becomes the default host.

Without --with-token the token is asked for on the terminal, without echo.
With --with-token it is read from stdin. No flag takes the token itself, so it
does not end up in the shell history or the process list.

The config file is ~/.config/minka/config.json, or $MINKA_CONFIG.`,
		Example: `  minka auth login --host https://minka.localbeach.net
  minka auth login --host https://minka.localbeach.net --with-token < token.txt`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			host, err := a.explicitHost()
			if err != nil {
				return err
			}
			if host == "" {
				return usageErrorf(cmd, "--host is required")
			}

			token, err := a.readToken(cmd, host, withToken)
			if err != nil {
				return err
			}

			path, me, err := a.signIn(host, token)
			if err != nil {
				return err
			}

			return a.ok(success{
				data:    map[string]any{"host": host, "configPath": path, "person": me.Raw},
				summary: fmt.Sprintf("Signed in to %s as %s", host, me.Value.Name),
				breadcrumbs: []Breadcrumb{
					{Action: "projects", Cmd: "minka projects list", Description: "List your projects"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Signed in to %s as %s <%s>.\nThe token is saved in %s.\n", host, me.Value.Name, me.Value.EmailAddress, path)
				},
			})
		},
	}
	cmd.Flags().BoolVar(&withToken, "with-token", false, "Read the token from stdin")

	return cmd
}

// signIn checks a token against its host and saves it as the token of that
// host, which becomes the default host. A token the host does not accept is
// not saved, so the config file never holds one that does not work.
func (a *app) signIn(host, token string) (string, api.Result[api.Me], error) {
	me, err := api.New(host, token, userAgent()).Me(a.ctx)
	if err != nil {
		e := apiError(err, host)
		if e.Code == CodeAuth {
			e.Message = fmt.Sprintf("%s did not accept the token, nothing was saved", host)
			e.Hint = "Check that the token is complete and not revoked or expired."
		}
		return "", api.Result[api.Me]{}, e
	}

	path, file, err := a.loadConfig()
	if err != nil {
		return "", api.Result[api.Me]{}, err
	}
	file.Hosts[host] = config.HostEntry{Token: token, PersonName: me.Value.Name, EmailAddress: me.Value.EmailAddress}
	file.DefaultHost = host
	if err := file.Save(path); err != nil {
		return "", api.Result[api.Me]{}, &Error{Code: CodeConfig, Message: err.Error()}
	}
	return path, me, nil
}

func (a *app) readToken(cmd *cobra.Command, host string, fromStdin bool) (string, error) {
	var token string
	if fromStdin {
		if a.stdinIsTerminal() {
			return "", &Error{
				Code:    CodeUsage,
				Message: "--with-token reads the token from stdin, but stdin is a terminal",
				Hint:    "Leave out --with-token to be asked for the token, or pipe it in: minka auth login --host " + host + " --with-token < token.txt",
			}
		}
		content, err := io.ReadAll(io.LimitReader(a.stdin, maxTokenBytes+1))
		if err != nil {
			return "", &Error{Code: CodeUsage, Message: "reading the token from stdin: " + err.Error()}
		}
		if len(content) > maxTokenBytes {
			return "", usageErrorf(cmd, "the token on stdin is longer than %d bytes", maxTokenBytes)
		}
		token = strings.TrimSpace(string(content))
	} else {
		if !a.stdinIsTerminal() {
			return "", &Error{
				Code:    CodeUsage,
				Message: "there is no terminal to ask for the token",
				Hint:    "Pipe the token in: minka auth login --host " + host + " --with-token < token.txt",
			}
		}
		fmt.Fprintf(a.stderr, "Access token for %s: ", host)
		secret, err := a.readSecret()
		fmt.Fprintln(a.stderr)
		if err != nil {
			return "", &Error{Code: CodeUsage, Message: "reading the token: " + err.Error()}
		}
		token = strings.TrimSpace(secret)
	}

	if token == "" {
		return "", usageErrorf(cmd, "the token is empty")
	}
	if strings.ContainsFunc(token, unicode.IsSpace) {
		return "", usageErrorf(cmd, "the token has to be one line without spaces")
	}

	return token, nil
}

func (a *app) newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the host and the person you are signed in as",
		Long: `Checks the token against the host and shows who it acts as. Exits with 3 when
there is no token or the host does not accept it.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			me, err := s.client.Me(a.ctx)
			if err != nil {
				return s.fail(err)
			}

			tokenSource := "the config file " + s.configPath
			if s.tokenSource == tokenFromEnvironment {
				tokenSource = "$MINKA_TOKEN"
			}

			return a.ok(success{
				data:    map[string]any{"host": s.host, "tokenSource": s.tokenSource, "person": me.Raw},
				summary: fmt.Sprintf("Signed in to %s as %s", s.host, me.Value.Name),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Signed in to %s as %s <%s>.\n", s.host, me.Value.Name, me.Value.EmailAddress)
					if me.Value.Principal != nil {
						fmt.Fprintf(w, "Acting for %s.\n", me.Value.Principal.Name)
					}
					fmt.Fprintf(w, "The token comes from %s.\n", tokenSource)
				},
			})
		},
	}
}

func (a *app) newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the token of a host from the config file",
		Long: `Removes the token of the host from the config file. The token itself stays
valid until it is revoked on the server.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			host, err := a.explicitHost()
			if err != nil {
				return err
			}
			path, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			if host == "" {
				host = file.DefaultHost
			}
			if host == "" {
				return a.ok(success{
					data:    map[string]any{"host": nil, "removed": false},
					summary: "Not signed in to any host",
					text:    func(w io.Writer) { fmt.Fprintln(w, "Not signed in to any host.") },
				})
			}

			removed := file.RemoveHost(host)
			if removed {
				if err := file.Save(path); err != nil {
					return &Error{Code: CodeConfig, Message: err.Error()}
				}
			}

			summary := "Not signed in to " + host
			if removed {
				summary = "Signed out of " + host
			}
			return a.ok(success{
				data:    map[string]any{"host": host, "removed": removed},
				summary: summary,
				text: func(w io.Writer) {
					if !removed {
						fmt.Fprintf(w, "%s.\n", summary)
						return
					}
					fmt.Fprintf(w, "%s. The token stays valid on the server until it is revoked.\n", summary)
				},
			})
		},
	}
}
