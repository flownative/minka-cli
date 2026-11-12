package cli

import (
	"context"
	"errors"
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
	var withToken, makeDefault bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to a Minka host with an access token",
		Long: `Signs in to a Minka host with a personal access token. The token is checked
against the host and then saved in the config file, which only your user can
read.

The default host, which a command without --host and $MINKA_HOST goes to,
does not move: the first host signed in to becomes it, and later sign-ins
leave it where it is. --make-default makes this host the default, as does
minka config set host <url> for a host signed in to already.

Without --with-token the token is asked for on the terminal, without echo.
With --with-token it is read from stdin. No flag takes the token itself, so it
does not end up in the shell history or the process list.

The config file is ~/.config/minka/config.json, or $MINKA_CONFIG.`,
		Example: `  minka auth login --host https://minka.localbeach.net
  minka auth login --host https://minka.localbeach.net --with-token < token.txt
  minka auth login --host https://minka.example.com --make-default`,
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

			path, me, defaultHost, err := a.signIn(host, token, makeDefault)
			if err != nil {
				return err
			}

			summary := fmt.Sprintf("Signed in to %s as %s; it is the default host", host, me.Value.Name)
			defaultLine := host + " is the default host."
			breadcrumbs := []Breadcrumb{
				{Action: "projects", Cmd: "minka projects list", Description: "List your projects"},
			}
			if defaultHost != host {
				shown := config.HostForOutput(defaultHost)
				summary = fmt.Sprintf("Signed in to %s as %s; the default host stays %s", host, me.Value.Name, shown)
				defaultLine = "The default host stays " + shown + ". Give --host " + host + " to a command to reach this host,\nor make it the default: minka config set host " + host
				breadcrumbs = []Breadcrumb{
					{Action: "projects", Cmd: "minka projects list --host " + host, Description: "List your projects on " + host},
					{Action: "make-default", Cmd: "minka config set host " + host, Description: "Make " + host + " the default host"},
				}
			}

			return a.ok(success{
				data: map[string]any{
					"host": host, "configPath": path, "person": me.Raw,
					"defaultHost": config.HostForOutput(defaultHost), "isDefault": defaultHost == host,
				},
				summary:     summary,
				breadcrumbs: breadcrumbs,
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Signed in to %s as %s <%s>.\nThe token is saved in %s.\n%s\n", host, me.Value.Name, me.Value.EmailAddress, path, defaultLine)
				},
			})
		},
	}
	cmd.Flags().BoolVar(&withToken, "with-token", false, "Read the token from stdin")
	cmd.Flags().BoolVar(&makeDefault, "make-default", false, "Make this host the default host, even when another one is")

	return cmd
}

// signIn checks a token against its host and saves it as the token of that
// host. A token the host does not accept is not saved, so the config file never
// holds one that does not work. It returns the default host as it stands after
// the sign-in.
//
// The default host is where every command without --host goes, those of other
// sessions on this machine included, so signing in to another host does not
// move it unless that is asked for. Where there is none yet, or one no command
// can read, the host signed in to takes its place: there is nothing a command
// could reach that it would take away.
func (a *app) signIn(host, token string, makeDefault bool) (string, api.Result[api.Me], string, error) {
	me, err := a.newClient(host, token).Me(a.ctx)
	if err != nil {
		e := apiError(err, host)
		if e.Code == CodeAuth {
			e.Message = fmt.Sprintf("%s did not accept the token, nothing was saved", host)
			e.Hint = "Check that the token is complete and not revoked or expired."
		}
		return "", api.Result[api.Me]{}, "", e
	}

	path, file, err := a.loadConfig()
	if err != nil {
		return "", api.Result[api.Me]{}, "", err
	}
	file.Hosts[host] = config.HostEntry{Token: token, PersonName: me.Value.Name, EmailAddress: me.Value.EmailAddress}
	// An empty default host fails to normalise like an unreadable one, so both
	// count as no default here.
	defaultHost, unreadable := config.NormalizeHost(file.DefaultHost)
	if makeDefault || unreadable != nil {
		file.DefaultHost, defaultHost = host, host
	}
	if err := file.Save(path); err != nil {
		return "", api.Result[api.Me]{}, "", &Error{Code: CodeConfig, Message: err.Error()}
	}
	return path, me, defaultHost, nil
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
		Long: `Checks the token against the host and shows who it acts as, and names the
default host where the host comes from --host or $MINKA_HOST and is another
one. Exits with 3 when there is no token or the host does not accept it.`,
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
			source := a.hostSource()
			defaultHost, problem := a.defaultHostToReport()
			hosts := hostsInWords(s.host, source, defaultHost, problem)

			return a.ok(success{
				data: map[string]any{
					"host": s.host, "hostSource": source, "defaultHost": nullable(defaultHost),
					"tokenSource": s.tokenSource, "person": me.Raw,
				},
				summary: fmt.Sprintf("Signed in to %s as %s; %s", s.host, me.Value.Name, hosts),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Signed in to %s as %s <%s>.\n", s.host, me.Value.Name, me.Value.EmailAddress)
					if me.Value.Principal != nil {
						fmt.Fprintf(w, "Acting for %s.\n", me.Value.Principal.Name)
					}
					fmt.Fprintf(w, "%s.\n", strings.ToUpper(hosts[:1])+hosts[1:])
					fmt.Fprintf(w, "The token comes from %s.\n", tokenSource)
				},
			})
		},
	}
}

// hostToSignOutOf is the entry a sign-out takes away: the host it was told, else
// the default host of the configuration, and either of them as the file writes
// it.
//
// Taking a token away has to reach the entry that is there, so a value that
// matches one letter for letter is that entry, before anything is read of it: a
// host read the way a sign-in writes it would look past an entry written in
// another form and leave its token where it stands, and one written in a form
// nothing can read at all — refused everywhere else, because a session under it
// would carry the value into everything that prints a host — would have no way
// out of the file at all. Nothing goes anywhere from here, so strictness would
// buy nothing and cost the only way to be rid of such an entry: no command lists
// the hosts, and its token stays valid on the server. A canonical entry is
// untouched by this, because a canonical value matches it letter for letter.
func (a *app) hostToSignOutOf(file *config.File) (string, error) {
	given := a.hostFlag
	if given == "" {
		given = a.getenv("MINKA_HOST")
	}
	if given == "" {
		return file.DefaultHost, nil
	}
	if _, written := file.Hosts[given]; written {
		return given, nil
	}

	return a.explicitHost()
}

func (a *app) newAuthLogoutCmd() *cobra.Command {
	var revoke bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove the token of a host from the config file, and revoke it with --revoke",
		Long: `Removes the token of the host from the config file. Without --revoke the token
itself stays valid on the server until it is revoked there.

--revoke revokes the token on the server first, and only then removes the
entry: the entry is the one copy of the token this machine has, so it is kept
when the revocation fails, and the command exits with the code of that failure.
Revoking is safe to repeat: a token the server no longer accepts counts as
revoked, the entry is removed, and a notice says so. Only the token of the
entry is revoked; a token in $MINKA_TOKEN is never touched.

Without --host it is the default host. Signing out of the default host leaves
no default: the next sign-in sets one, or minka config set host <url> names one
of the hosts still signed in to. The entry is reached as the
file writes it, so an address written in by hand can be taken out again; with
--revoke the token goes to that address read like every host, and an entry no
host can be read from keeps its token and exits 1.`,
		Example: `  minka auth logout
  minka auth logout --host https://minka.example.com --revoke`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			path, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			host, err := a.hostToSignOutOf(file)
			if err != nil {
				return err
			}
			if host == "" {
				return a.ok(success{
					data:    map[string]any{"host": nil, "removed": false, "revoked": false},
					summary: "Not signed in to any host",
					text:    func(w io.Writer) { fmt.Fprintln(w, "Not signed in to any host.") },
				})
			}

			// The entry is reached by the value the file holds; what is printed
			// of it is the address alone, which is not where a password belongs.
			shown := config.HostForOutput(host)
			var notices []string
			revoked := false
			if entry, standing := file.Hosts[host]; revoke && standing && entry.Token != "" {
				alreadyGone, err := a.revokeToken(host, entry.Token)
				if err != nil {
					return err
				}
				revoked = true
				if alreadyGone {
					notices = append(notices, shown+" no longer accepted the token: it was revoked, had expired or belonged to an agent that stepped down, so there was nothing left to revoke")
				}
			}
			// A sign-out reaches the file alone, so a token given in the
			// environment is still valid, and whoever asked to revoke has to
			// hear that.
			if revoke && a.getenv("MINKA_TOKEN") != "" {
				notices = append(notices, "the token in $MINKA_TOKEN was not revoked: a sign-out revokes only the token of the config file. Revoke it in Minka's account area, or sign it in with minka auth login --with-token and sign out again with --revoke")
			}

			wasDefault := file.DefaultHost == host
			removed := file.RemoveHost(host)
			// The hosts left keep their tokens, and none of them becomes the
			// default by being left over; whoever relied on the default has to
			// hear that there is none now.
			if removed && wasDefault && len(file.Hosts) > 0 {
				notices = append(notices, shown+" was the default host, and no host is the default now: give --host <url> to a command, or make a host signed in to the default with: minka config set host <url>")
			}
			if removed {
				if err := file.Save(path); err != nil {
					e := &Error{Code: CodeConfig, Message: err.Error()}
					if revoked {
						e.Message = "the token is revoked, but its entry could not be taken out of the config file: " + err.Error()
						e.Data = map[string]any{"host": shown, "removed": false, "revoked": true}
					}
					return e
				}
			}

			summary := "Not signed in to " + shown
			if removed {
				summary = "Signed out of " + shown
			}
			answer := success{
				data:    map[string]any{"host": shown, "removed": removed, "revoked": revoked},
				summary: summary,
				notices: notices,
				text: func(w io.Writer) {
					switch {
					case !removed:
						fmt.Fprintf(w, "%s.\n", summary)
					case revoked:
						fmt.Fprintf(w, "%s. The token is revoked on the server.\n", summary)
					default:
						fmt.Fprintf(w, "%s. The token stays valid on the server until it is revoked.\n", summary)
					}
				},
			}
			// Saying that there is nothing here would be the whole answer only if
			// no token stood under the same host written another way. It does, and
			// nothing looks for it there, so the answer says where it is.
			if !removed {
				if written := hostWrittenAnotherWay(file, host); written != "" {
					elsewhere := config.HostForOutput(written)
					command := "minka auth logout --host " + elsewhere
					if revoke {
						command += " --revoke"
					}
					answer.notices = append(answer.notices, "the configuration keeps a token under "+elsewhere+
						", the same host written another way; take that entry out with: "+command)
				}
			}
			return a.ok(answer)
		},
	}
	cmd.Flags().BoolVar(&revoke, "revoke", false, "Revoke the token on the server before removing it")

	return cmd
}

// revokeToken revokes a token at the host its entry names, and says whether
// the host had stopped accepting it before. That counts as revoked: what was
// asked for, a token nobody can use, holds either way, and a second run after
// a first that arrived unseen ends the same. Only Minka's own 401 says so: one
// of a proxy or a password page in front of it says nothing about the token,
// which may well be valid behind it.
//
// The token goes to the entry's host read like every host a request goes to,
// never to the value as the file writes it: an entry that reads as no host
// sends nothing.
func (a *app) revokeToken(host, token string) (alreadyGone bool, err error) {
	address, err := config.NormalizeHost(host)
	if err != nil {
		return false, &Error{
			Code:    CodeConfig,
			Message: "the token was not revoked and its entry is kept: " + err.Error(),
			Hint:    "Sign out without --revoke, which takes the entry out as the file writes it, and revoke the token in Minka's account area.",
		}
	}
	err = a.newClient(address, token).RevokeOwnToken(a.ctx)
	if err == nil {
		return false, nil
	}
	var refusal *api.Error
	if errors.As(err, &refusal) && refusal.RefusedByMinka {
		return true, nil
	}
	e := apiError(err, address)
	e.Message = "the token was not revoked and its entry is kept: " + e.Message
	const elsewhere = "Sign out without --revoke and revoke the token in Minka's account area."
	const again = "Run the same command with --revoke again: a token the server no longer accepts counts as revoked."
	interrupted := errors.Is(err, context.Canceled)
	e.Retryable = e.Code == CodeNetwork && !interrupted || e.Code == CodeAPI && e.HTTPStatus >= 500
	switch {
	case interrupted:
		e.Hint = again
	case e.Retryable:
		e.Hint = again + " Otherwise, sign out without --revoke and revoke the token in Minka's account area."
	case e.Code == CodeAuth:
		e.Hint = "The 401 did not come from Minka's sign-in but from something in front of it, such as a proxy asking for a password, and says nothing about the token. " + elsewhere
	case e.Code == CodeNotFound:
		e.Hint = "A Minka older than v0.53.0 has no address to revoke a token at. " + elsewhere
	default:
		e.Hint = elsewhere
	}
	return false, e
}
