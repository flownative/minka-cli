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
		Short: "Sign in to a Minka host with an access token, as a profile",
		Long: `Signs in with a personal access token and keeps it as a profile: one account in
one organization. The token is checked against the host and then saved in the
config file, which only your user can read.

--profile <name> names the profile. A new name makes a new profile and needs
--host; a name that exists signs that profile in again, on its own host, and
--host may only repeat that host. Without --profile, a host with one profile
signs that one in again, a host with none gets a profile named after the host,
and a host with several needs --profile. Here, unlike everywhere else, --profile
and --host go together.

The default profile, which a command without --profile and --host uses, does
not move: the first profile becomes it, and later sign-ins leave it where it
is. --make-default makes this profile the default, as does
minka profiles default <name>.

Without --with-token the token is asked for on the terminal, without echo.
With --with-token it is read from stdin. No flag takes the token itself, so it
does not end up in the shell history or the process list.

The config file is ~/.config/minka/config.json, or $MINKA_CONFIG.`,
		Example: `  minka auth login --host https://minka.localbeach.net
  minka auth login --profile flownative --host https://minka.flownative.com --with-token < token.txt
  minka auth login --profile familie --host https://minka.example.com --make-default`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			name, host, err := a.profileToSignIn(cmd, file)
			if err != nil {
				return err
			}

			token, err := a.readToken(cmd, host, withToken)
			if err != nil {
				return err
			}

			path, me, file, err := a.signIn(name, host, token, makeDefault)
			if err != nil {
				return err
			}
			defaultProfile := file.DefaultProfile
			defaultHost := ""
			if profile, ok := file.Profiles[defaultProfile]; ok {
				defaultHost = config.HostForOutput(profile.Host)
			}

			summary := fmt.Sprintf("Signed in to %s as %s with the profile %s; it is the default profile", host, me.Value.Name, name)
			defaultLine := name + " is the default profile."
			breadcrumbs := []Breadcrumb{
				{Action: "projects", Cmd: "minka projects list", Description: "List your projects"},
			}
			if defaultProfile != name {
				summary = fmt.Sprintf("Signed in to %s as %s with the profile %s; the default profile stays %s", host, me.Value.Name, name, defaultProfile)
				defaultLine = "The default profile stays " + defaultProfile + ". Give --profile " + name + " to a command to use this one,\nor make it the default: minka profiles default " + name
				breadcrumbs = []Breadcrumb{
					{Action: "projects", Cmd: "minka projects list --profile " + name, Description: "List your projects with the profile " + name},
					{Action: "make-default", Cmd: "minka profiles default " + name, Description: "Make " + name + " the default profile"},
				}
			}

			return a.ok(success{
				data: map[string]any{
					"host": host, "configPath": path, "person": me.Raw,
					"defaultHost": nullable(defaultHost), "isDefault": defaultProfile == name,
					"profile": name, "defaultProfile": nullable(defaultProfile),
				},
				summary:     summary,
				breadcrumbs: breadcrumbs,
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Signed in to %s as %s <%s>, as the profile %s.\nThe token is saved in %s.\n%s\n", host, me.Value.Name, me.Value.EmailAddress, name, path, defaultLine)
				},
			})
		},
	}
	cmd.Flags().BoolVar(&withToken, "with-token", false, "Read the token from stdin")
	cmd.Flags().BoolVar(&makeDefault, "make-default", false, "Make this profile the default, even when another one is")

	return cmd
}

// profileToSignIn is the profile a sign-in is for, and its host.
//
// A profile keeps its host. Signing an existing name in to another host would
// change under every script that names the profile which organization it
// writes in, so that takes removing the profile first.
func (a *app) profileToSignIn(cmd *cobra.Command, file *config.File) (string, string, error) {
	host, err := a.explicitHost()
	if err != nil {
		return "", "", err
	}
	return a.profileOnHost(cmd, file, host, a.hostFlag != "")
}

// profileOnHost is profileToSignIn for a host found already, which is empty
// where none was given. hostNamed says whether somebody named the host for this
// command, rather than it coming from $MINKA_HOST.
func (a *app) profileOnHost(cmd *cobra.Command, file *config.File, host string, hostNamed bool) (string, string, error) {
	name := a.profileFlag
	source := chosenByProfileFlag
	if name == "" && a.hostFlag == "" {
		name, source = a.getenv("MINKA_PROFILE"), chosenByProfileEnv
	}

	if name != "" {
		if err := config.CheckProfileName(name); err != nil {
			return "", "", profileNameError(source, err)
		}
		existing, ok := file.Profiles[name]
		if !ok {
			if host == "" {
				return "", "", usageErrorf(cmd, "--host is required for the new profile %s", name)
			}
			return name, host, nil
		}
		own, err := config.NormalizeHost(existing.Host)
		if err != nil {
			return "", "", &Error{Code: CodeConfig, Message: "the host of the profile " + name + " cannot be read: " + err.Error(), Hint: "Take the profile out and sign in again: minka profiles remove " + name}
		}
		if hostNamed && host != own {
			return "", "", &Error{
				Code:    CodeUsage,
				Message: "the profile " + name + " signs in to " + own + ", not to " + host,
				Hint:    "A profile keeps its host. Sign in under another name with --profile <name>, or take this one out first: minka profiles remove " + name,
			}
		}
		return name, own, nil
	}

	if host == "" {
		return "", "", usageErrorf(cmd, "--host or --profile is required")
	}
	names := file.ProfilesOnHost(host)
	switch len(names) {
	case 0:
		return file.ProfileNameForHost(host), host, nil
	case 1:
		return names[0], host, nil
	}
	return "", "", &Error{
		Code:    CodeAmbiguous,
		Message: fmt.Sprintf("%d profiles sign in to %s: %s", len(names), host, strings.Join(names, ", ")),
		Hint:    "Name the profile to sign in with --profile <name>.",
	}
}

// signIn checks a token against its host and saves it as the token of a
// profile. A token the host does not accept is not saved, so the config file
// never holds one that does not work. It returns the configuration as it stands
// after the sign-in.
//
// The default profile is what every command without --profile and --host uses,
// those of other sessions on this machine included, so signing in with another
// profile does not move it unless that is asked for. Where there is none yet,
// or one no command can use, the profile signed in takes its place: there is
// nothing a command could reach that it would take away.
func (a *app) signIn(name, host, token string, makeDefault bool) (string, api.Result[api.Me], *config.File, error) {
	me, err := a.newClient(host, token).Me(a.ctx)
	if err != nil {
		e := apiError(err, host)
		if e.Code == CodeAuth {
			e.Message = fmt.Sprintf("%s did not accept the token, nothing was saved", host)
			e.Hint = "Check that the token is complete and not revoked or expired."
		}
		return "", api.Result[api.Me]{}, nil, e
	}

	path, file, err := a.loadConfig()
	if err != nil {
		return "", api.Result[api.Me]{}, nil, err
	}
	// A new profile starts with no cards: lists a failed move or drop left
	// under its name belong to another account, on whatever host.
	if _, exists := file.Profiles[name]; !exists {
		if err := a.dropFromTheStore(name); err != nil {
			e := &Error{Code: CodeConfig, Message: "nothing was saved: the list of cards on this machine may hold cards under the name " + name + " and could not be cleared: " + err.Error()}
			var cause *Error
			if errors.As(err, &cause) {
				e.Hint = cause.Hint
			}
			return "", api.Result[api.Me]{}, nil, e
		}
	}
	file.Profiles[name] = config.Profile{Host: host, Token: token, PersonName: me.Value.Name, EmailAddress: me.Value.EmailAddress}
	if _, err := chooseIn(file, path, chosenByDefault, ""); makeDefault || file.DefaultProfile == "" || err != nil {
		file.DefaultProfile = name
	}
	if err := file.Save(path); err != nil {
		return "", api.Result[api.Me]{}, nil, &Error{Code: CodeConfig, Message: err.Error()}
	}
	return path, me, file, nil
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
		Short: "Show the profile, the host and the person you are signed in as",
		Long: `Checks the token against the host and shows who it acts as, which profile that
is and where the choice came from: --profile, --host, $MINKA_PROFILE,
$MINKA_HOST or the default profile, which it names where it is another one.
Exits with 3 when there is no token or the host does not accept it.

data.profile is null where the token comes from $MINKA_TOKEN; data.profileSource
is --profile, --host, $MINKA_PROFILE, $MINKA_HOST or default.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			c, path, file, err := a.chooseAccount(true)
			if err != nil {
				return err
			}
			s, err := a.sessionOf(c, path, file)
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
			defaultProfile, defaultHost, problem := a.defaultToReport()
			account := accountInWords(c, defaultProfile, problem)

			return a.ok(success{
				data: map[string]any{
					"host": s.host, "hostSource": hostSource(c.source), "defaultHost": nullable(defaultHost),
					"profile": nullable(s.profile), "profileSource": c.source, "defaultProfile": nullable(defaultProfile),
					"tokenSource": s.tokenSource, "person": me.Raw,
				},
				summary: fmt.Sprintf("Signed in to %s as %s; %s", s.host, me.Value.Name, account),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Signed in to %s as %s <%s>.\n", s.host, me.Value.Name, me.Value.EmailAddress)
					if me.Value.Principal != nil {
						fmt.Fprintf(w, "Acting for %s.\n", me.Value.Principal.Name)
					}
					fmt.Fprintf(w, "%s.\n", strings.ToUpper(account[:1])+account[1:])
					fmt.Fprintf(w, "The token comes from %s.\n", tokenSource)
				},
			})
		},
	}
}

func (a *app) newAuthLogoutCmd() *cobra.Command {
	var revoke bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove the profile a command uses, and revoke its token with --revoke",
		Long: `Removes the profile a command uses, the one --profile, --host, $MINKA_PROFILE,
$MINKA_HOST or the default names, and its token from the config file, as
minka profiles remove <name> does. Without --revoke the token itself stays
valid on the server until it is revoked there.

--revoke revokes the token on the server first, and only then removes the
profile: it holds the one copy of the token this machine has, so it is kept
when the revocation fails, and the command exits with the code of that failure.
Revoking is safe to repeat: a token the server no longer accepts counts as
revoked, the profile is removed, and a notice says so. Only the token of the
profile is revoked; a token in $MINKA_TOKEN is never touched.

Signing out of the default profile leaves no default: the next sign-in sets
one, or minka profiles default <name> names one of the profiles left.`,
		Example: `  minka auth logout
  minka auth logout --profile familie --revoke`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			path, file, err := a.loadConfig()
			if err != nil {
				return err
			}
			c, err := a.profileToSignOutOf(path, file)
			if err != nil {
				return err
			}
			if c.profile == "" {
				var host any
				summary := "Not signed in to any host"
				if c.host != "" {
					host, summary = c.host, "Not signed in to "+c.host
				}
				var notices []string
				if revoke && a.getenv("MINKA_TOKEN") != "" {
					notices = append(notices, envTokenNotRevoked)
				}
				return a.ok(success{
					data:    map[string]any{"host": host, "profile": nil, "removed": false, "revoked": false},
					summary: summary,
					notices: notices,
					text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
				})
			}
			return a.removeProfile(path, file, c.profile, revoke)
		},
	}
	cmd.Flags().BoolVar(&revoke, "revoke", false, "Revoke the token on the server before removing it")

	return cmd
}

// profileToSignOutOf is the profile a sign-out takes away, chosen the way every
// command chooses it, with one difference: its host is not read first.
//
// Taking a token away has to reach the profile that is there. A profile whose
// host was written into the file in a form nothing can read is refused
// everywhere else, because a session under it would carry the value into
// everything that prints a host. Nothing goes anywhere from here, so strictness
// would buy nothing and cost the way out of the file. For the same reason a
// host given that cannot be read finds the profile whose host is written
// letter for letter like it. A host that can be read is chosen as everywhere
// else, so that of several profiles on it none is taken by chance.
func (a *app) profileToSignOutOf(path string, file *config.File) (choice, error) {
	source, value, err := a.requestedChoice()
	if err != nil {
		return choice{}, err
	}
	switch source {
	case chosenByProfileFlag, chosenByProfileEnv:
		if err := config.CheckProfileName(value); err != nil {
			return choice{}, profileNameError(source, err)
		}
		if _, ok := file.Profiles[value]; !ok {
			return choice{}, unknownProfileError(file, value)
		}
		return choice{profile: value, source: source}, nil
	case chosenByHostFlag, chosenByHostEnv:
		if _, err := config.NormalizeHost(value); err == nil {
			break
		}
		var written []string
		for _, name := range file.ProfileNames() {
			if file.Profiles[name].Host == value {
				written = append(written, name)
			}
		}
		switch len(written) {
		case 0:
		case 1:
			return choice{profile: written[0], source: source}, nil
		default:
			return choice{}, &Error{
				Code:    CodeAmbiguous,
				Message: fmt.Sprintf("%d profiles sign in to the host given: %s", len(written), strings.Join(written, ", ")),
				Hint:    "Name the profile to remove: minka profiles remove <name>",
			}
		}
	default:
		if _, ok := file.Profiles[file.DefaultProfile]; ok {
			return choice{profile: file.DefaultProfile, source: source}, nil
		}
	}
	return chooseIn(file, path, source, value)
}

// envTokenNotRevoked is what a sign-out with --revoke says where a token is
// given in the environment: a sign-out reaches the file alone, so that token is
// still valid, and whoever asked to revoke has to hear that.
const envTokenNotRevoked = "the token in $MINKA_TOKEN was not revoked: a sign-out revokes only the token of a profile. Revoke it in Minka's account area, or sign it in with minka auth login --with-token and sign out again with --revoke"

// removeProfile takes a profile and its token out of the configuration, and
// revokes the token first where that is asked for.
func (a *app) removeProfile(path string, file *config.File, name string, revoke bool) error {
	profile := file.Profiles[name]
	// The profile is reached by its name; what is printed of its host is the
	// address alone, which is not where a password belongs.
	shown := config.HostForOutput(profile.Host)
	var notices []string
	revoked := false
	if revoke && profile.Token != "" {
		alreadyGone, err := a.revokeToken(profile.Host, profile.Token)
		if err != nil {
			return err
		}
		revoked = true
		if alreadyGone {
			notices = append(notices, shown+" no longer accepted the token: it was revoked, had expired or belonged to an agent that stepped down, so there was nothing left to revoke")
		}
	}
	if revoke && a.getenv("MINKA_TOKEN") != "" {
		notices = append(notices, envTokenNotRevoked)
	}

	// The lists go before the profile does: under a name a later profile may
	// take on another host, they would be reported there with its token.
	if err := a.dropFromTheStore(name); err != nil {
		notices = append(notices, "the cards this profile was working on could not be taken out of the list on this machine: "+err.Error())
	}

	wasDefault := file.DefaultProfile == name
	file.RemoveProfile(name)
	// The profiles left keep their tokens, and none of them becomes the
	// default by being left over; whoever relied on the default has to hear
	// that there is none now.
	if wasDefault && len(file.Profiles) > 0 {
		notices = append(notices, name+" was the default profile, and no profile is the default now: give --profile <name> to a command, or make one the default with: minka profiles default <name>")
	}
	if err := file.Save(path); err != nil {
		e := &Error{Code: CodeConfig, Message: err.Error()}
		if revoked {
			e.Message = "the token is revoked, but its profile could not be taken out of the config file: " + err.Error()
			e.Data = map[string]any{"host": shown, "profile": name, "removed": false, "revoked": true}
		}
		return e
	}

	summary := "Signed out of " + shown + " and removed the profile " + name
	return a.ok(success{
		data:    map[string]any{"host": shown, "profile": name, "removed": true, "revoked": revoked},
		summary: summary,
		notices: notices,
		text: func(w io.Writer) {
			if revoked {
				fmt.Fprintf(w, "%s. The token is revoked on the server.\n", summary)
				return
			}
			fmt.Fprintf(w, "%s. The token stays valid on the server until it is revoked.\n", summary)
		},
	})
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
