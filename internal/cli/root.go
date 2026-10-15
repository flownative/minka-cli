// Package cli implements the minka commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/cliupdate"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/readmark"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// version is set at build time.
var version = "dev"

const (
	maxTokenBytes = 4096
	maxInputBytes = 1 << 20
)

const (
	tokenFromConfig      = "config"
	tokenFromEnvironment = "environment"
)

// shortIdentifierPattern is the shape of what people call a card by, such as
// "MNK-1A". It is a little wider than what the server reads, which leaves out
// four letters behind the hyphen: what is no card answers 404 there, and a
// pattern kept narrower here could only fall behind.
var shortIdentifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{1,9}-[0-9A-Za-z]{1,12}$`)

// uuidPattern is the shape of an ID in either case. Minka keeps an ID in
// lowercase, so one somebody typed goes through asID before it is sent.
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// asID reads a value as an ID in the form Minka keeps it in, lowercase.
// Anything that is no ID comes back unchanged, with ok false.
func asID(value string) (id string, ok bool) {
	if !uuidPattern.MatchString(value) {
		return value, false
	}
	return strings.ToLower(value), true
}

type app struct {
	ctx    context.Context
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string

	stdinIsTerminal  func() bool
	stdoutIsTerminal func() bool
	readSecret       func() (string, error)

	// What import reads a foreign system through, replaced in tests.
	basecampSource basecamp.Source
	basecampFiles  basecamp.Files
	avatarFetcher  basecamp.AvatarFetcher
	// retryPause waits before a request is asked again; by the clock when nil.
	retryPause basecamp.Pause
	now        func() time.Time

	// executable is the path this minka was started from, which the mod is
	// written to run. Tests name another one.
	executable func() (string, error)

	// runClaude runs the command line of Claude Code. It is an injection point
	// so that no test installs anything into the Claude Code of whoever runs it.
	runClaude func(args []string) (string, error)

	// markWait is how long a command waits for another minka to finish noting
	// which comments it has shown.
	markWait time.Duration

	// pause waits for a while unless the context ends first; tests move a
	// clock instead.
	pause func(ctx context.Context, d time.Duration) error

	hostFlag   string
	jsonFlag   bool
	quietFlag  bool
	styledFlag bool

	// fromMod is set on a beat of the mod that runs inside a Claude Code
	// process. The mod reports one named session, so such a beat without a
	// session reports nothing.
	fromMod bool

	// idle is set on a beat that says the process is alive and nothing more: it
	// writes neither the sign of life of a card nor the mark of a beat, and it
	// reports a list only for a while after the last step.
	idle bool

	// sessionFlag is the session a command was told it speaks for. It
	// outweighs the environment.
	sessionFlag string

	// skillIsTheTopic is set by a command that reports on the installed skill
	// itself, so that it does not say the same thing twice.
	skillIsTheTopic bool

	// ownVersion is the version this minka compares with what Minka names;
	// tests give it one of their own.
	ownVersion string

	// seen is what the answers of this run said about the versions of minka,
	// and updateState what became of it once settled.
	seen          versionsSeen
	updateSettled bool
	updateState   cliupdate.State

	// bytesAreTheOutput is set by a command that writes a file to standard
	// output. Everything it has to say then goes to standard error, a failure
	// included: half a file followed by an envelope is neither.
	bytesAreTheOutput bool
}

// Signals are what ends a command early: Ctrl-C on a terminal, and the
// termination a supervisor such as Kubernetes sends. Both cancel the context,
// so that a command tidies up what it was writing.
var Signals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// Run executes the CLI and returns its exit code.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	return newApp(ctx, stdin, stdout, stderr, getenv).run(args)
}

func newApp(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) *app {
	a := &app{ctx: ctx, stdin: stdin, stdout: stdout, stderr: stderr, getenv: getenv}
	a.basecampSource = basecamp.CLI{}
	a.basecampFiles = basecamp.Downloader{}
	a.avatarFetcher = basecamp.NewAvatarClient()
	a.now = time.Now
	a.ownVersion = version
	a.markWait = 5 * time.Second
	a.pause = pauseFor
	a.runClaude = a.runClaudeCommand
	a.executable = os.Executable
	a.stdinIsTerminal = func() bool { return isTerminal(stdin) }
	a.stdoutIsTerminal = func() bool { return isTerminal(stdout) }
	a.readSecret = func() (string, error) {
		file, ok := stdin.(*os.File)
		if !ok {
			return "", errors.New("stdin is not a terminal")
		}
		secret, err := term.ReadPassword(int(file.Fd()))
		return string(secret), err
	}
	return a
}

func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func (a *app) run(args []string) int {
	root := a.newRootCmd()
	root.SetArgs(args)
	root.SetIn(a.stdin)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)

	err := root.ExecuteContext(a.ctx)
	if err == nil {
		// A command that printed nothing still remembers what it learned.
		a.settleCLIUpdate(noChannel)
		return ExitOK
	}

	// Errors cobra creates itself are about the command line: an unknown
	// command, a missing required flag.
	var cliErr *Error
	if !errors.As(err, &cliErr) {
		cliErr = &Error{Code: CodeUsage, Message: err.Error(), Hint: "Run 'minka --help' for usage."}
	}

	// A flag error stops parsing before the output flags are read.
	for _, arg := range args {
		switch arg {
		case "--json":
			a.jsonFlag = true
		case "--quiet", "-q":
			a.quietFlag = true
		case "--styled":
			a.styledFlag = true
		}
	}
	a.fail(cliErr)

	return cliErr.ExitCode()
}

func (a *app) newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "minka",
		Short: "Work with Minka from the command line",
		Long: `minka works with projects, boards and cards of a Minka instance through its API.

Output is text on a terminal and JSON otherwise. --json prints an envelope
{ok, data, summary, notice, breadcrumbs}; --quiet prints only data. Failures
print {ok: false, error, code, retryable, hint, meta} and exit with a code per
class: 1 usage, 2 not found, 3 not signed in, 4 forbidden, 6 network,
7 API error, 8 ambiguous, 9 validation, 11 unread comments, 12 nothing new
within the time a wait was given.

First steps, on a terminal:
  minka setup

Without a terminal, sign in with:
  minka auth login --host https://minka.example.com --with-token < token.txt

$MINKA_HOST and $MINKA_TOKEN sign in without the config file. The token is
only used for the host in $MINKA_HOST.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.CompletionOptions.HiddenDefaultCmd = true

	flags := root.PersistentFlags()
	flags.StringVar(&a.hostFlag, "host", "", "Minka host (default: $MINKA_HOST, else the host signed in to last)")
	flags.BoolVar(&a.jsonFlag, "json", false, "Print a JSON envelope")
	flags.BoolVarP(&a.quietFlag, "quiet", "q", false, "Print only the data as JSON")
	flags.BoolVar(&a.styledFlag, "styled", false, "Print text even when stdout is not a terminal")

	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageErrorf(cmd, "%s", err)
	})

	root.AddCommand(
		a.newSetupCmd(),
		a.newConfigCmd(),
		a.newAuthCmd(),
		a.newMeCmd(),
		a.newProjectsCmd(),
		a.newBoardsCmd(),
		a.newPeopleCmd(),
		a.newCardsCmd(),
		a.newColumnsCmd(),
		a.newCommentsCmd(),
		a.newChatsCmd(),
		a.newMessagesCmd(),
		a.newConversationsCmd(),
		a.newDocsCmd(),
		a.newTodoListsCmd(),
		a.newTodoGroupsCmd(),
		a.newTodosCmd(),
		a.newCheckinsCmd(),
		a.newPostsCmd(),
		a.newEventsCmd(),
		a.newRemindersCmd(),
		a.newUploadsCmd(),
		a.newTrashCmd(),
		a.newImportCmd(),
		a.newWorkCmd(),
		a.newHeartbeatCmd(),
		a.newNotificationsCmd(),
		a.newHooksCmd(),
		a.newSkillCmd(),
		a.newDoctorCmd(),
		a.newCommandsCmd(root),
		a.newVersionCmd(),
	)

	return root
}

// group is a command that only holds subcommands. Run bare it shows its help;
// with an argument it is a usage error, not help with exit code 0.
func group(use, short string, children ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErrorf(cmd, "unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(children...)
	return cmd
}

// requireArgs checks the positional arguments. An argument whose name ends in
// "Id" has to be an ID, so that nothing else reaches the path of a request,
// and is handed on to the command in the form Minka knows it by.
func requireArgs(names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < len(names) {
			return usageErrorf(cmd, "missing %s", strings.Join(names[len(args):], " and "))
		}
		if len(args) > len(names) {
			return usageErrorf(cmd, "unexpected argument %q", args[len(names)])
		}
		for i, name := range names {
			if strings.HasSuffix(name, "Id") {
				if err := requireID(cmd, name, &args[i]); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

// requireID keeps anything that is no ID out of the path of a request and
// writes an ID in lowercase, the form Minka keeps it in. A card may also be
// named by its short identifier, which is looked up before the command goes on
// with the ID.
func requireID(cmd *cobra.Command, name string, value *string) error {
	if id, ok := asID(*value); ok {
		*value = id
		return nil
	}
	if name == "cardId" || name == neighbourCard {
		if shortIdentifierPattern.MatchString(*value) {
			return nil
		}
		return usageErrorf(cmd, "%s %q is not an ID; a card is named by a UUID such as 0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6 or by its short identifier such as MNK-1A", name, *value)
	}
	return usageErrorf(cmd, "%s %q is not an ID; IDs are UUIDs such as 0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", name, *value)
}

func userAgent() string {
	return "minka-cli/" + version
}

func (a *app) loadConfig() (string, *config.File, error) {
	path, err := config.Path(a.getenv)
	if err != nil {
		return "", nil, &Error{Code: CodeConfig, Message: err.Error()}
	}
	file, err := config.Load(path)
	if err != nil {
		e := &Error{Code: CodeConfig, Message: err.Error()}
		if errors.Is(err, config.ErrInsecurePermissions) {
			e.Hint = "The file holds access tokens. Restrict it to your user with chmod 600."
		}
		return "", nil, e
	}
	return path, file, nil
}

// explicitHost is the host given with --host, else $MINKA_HOST. It is empty
// when neither is set.
func (a *app) explicitHost() (string, error) {
	value := a.hostFlag
	if value == "" {
		value = a.getenv("MINKA_HOST")
	}
	if value == "" {
		return "", nil
	}
	host, err := config.NormalizeHost(value)
	if err != nil {
		return "", &Error{Code: CodeUsage, Message: err.Error()}
	}
	return host, nil
}

type session struct {
	client      *api.Client
	host        string
	tokenSource string
	configPath  string // empty when the token came from the environment
	reader      string // who reads comments here, named without the token
}

// session finds the host and the token of a command.
//
// The host is --host, else $MINKA_HOST, else the default host of the config
// file. $MINKA_TOKEN belongs to $MINKA_HOST and is used for that host only.
// For any other host the token comes from the config file, so a token is
// never sent to a host it was not given for.
func (a *app) session() (*session, error) {
	host, err := a.explicitHost()
	if err != nil {
		return nil, err
	}

	if envToken := a.getenv("MINKA_TOKEN"); envToken != "" {
		envHostValue := a.getenv("MINKA_HOST")
		if envHostValue == "" {
			return nil, &Error{Code: CodeUsage, Message: "$MINKA_TOKEN is set without $MINKA_HOST", Hint: "Set $MINKA_HOST to the host the token belongs to."}
		}
		envHost, err := config.NormalizeHost(envHostValue)
		if err != nil {
			return nil, &Error{Code: CodeUsage, Message: "$MINKA_HOST: " + err.Error()}
		}
		if host == envHost {
			return &session{client: a.newClient(host, envToken), host: host, tokenSource: tokenFromEnvironment, reader: readmark.Reader(envToken)}, nil
		}
	}

	path, file, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	if host == "" {
		if host, err = defaultHostOf(file, path); err != nil {
			return nil, err
		}
	}
	if host == "" {
		return nil, &Error{Code: CodeAuth, Message: "not signed in to any Minka host", Hint: "Run: minka auth login --host https://minka.example.com"}
	}
	token := file.Hosts[host].Token
	if token == "" {
		e := &Error{Code: CodeAuth, Message: "not signed in to " + config.HostForOutput(host), Hint: "Run: minka auth login --host " + host}
		if written := hostWrittenAnotherWay(file, host); written != "" {
			e.Hint = "The configuration keeps this host as " + config.HostForOutput(written) +
				", the same host written another way, and the token under that entry is looked for under " + host +
				". Run: minka auth login --host " + host + " — or write that entry as " + host + "."
		}
		return nil, e
	}

	return &session{client: a.newClient(host, token), host: host, tokenSource: tokenFromConfig, configPath: path, reader: readmark.Reader(token)}, nil
}

// hostToPaste is the host a hint may put into a command line. Where what may be
// printed of a value is no address to sign in to, the line names what is to be
// filled in instead. That is no address on purpose: whoever runs the line as it
// stands, a person or an agent, must not reach a host that is not theirs. It is a
// plain word, so a shell hands it on and the line fails where it should, at
// minka.
func hostToPaste(shown string) string {
	if host, err := config.NormalizeHost(shown); err == nil {
		return host
	}

	return "YOUR-INSTANCE-ADDRESS"
}

// hostWrittenAnotherWay names an entry of the configuration that means the host
// asked for but is not written the way a sign-in writes it. The token under such
// an entry is never found, and a refusal that named only the host it looked for
// would name one that stands nowhere in the file.
func hostWrittenAnotherWay(file *config.File, host string) string {
	for written, entry := range file.Hosts {
		if written == host || entry.Token == "" {
			continue
		}
		if same, err := config.NormalizeHost(written); err == nil && same == host {
			return written
		}
	}
	return ""
}

const neighbourCard = "the neighbour"

// resolveCard replaces the short identifier of a card by its ID, so that a
// command goes on with the one address every other request takes.
func (s *session) resolveCard(ctx context.Context, card *string) error {
	if uuidPattern.MatchString(*card) {
		return nil
	}
	identifier := strings.ToUpper(*card)
	found, err := s.client.CardByShortIdentifier(ctx, identifier)
	if err != nil {
		e := s.fail(err)
		if e.Code == CodeNotFound {
			e.Message = "no card you can see goes by " + identifier
			e.Hint = "It does not exist, its board shows no short identifiers, it is in the trash or in a project you are not a member of, or two of your organizations use this prefix. The ID of a card always works; minka trash list <projectId> gives the ID of a card in the trash."
		}
		return e
	}
	// What comes back goes into the path of the requests that follow.
	if !uuidPattern.MatchString(found.Value.ID) {
		return &Error{Code: CodeAPI, Message: s.host + " answered for " + identifier + " with a card that has no ID"}
	}
	*card = found.Value.ID
	return nil
}

// fail classifies a failed request of this session.
func (s *session) fail(err error) *Error {
	e := apiError(err, s.host)
	if e.Code == CodeAuth && s.tokenSource == tokenFromEnvironment {
		e.Hint = "The token in $MINKA_TOKEN is missing, unknown, revoked or expired."
	}
	return e
}

// readValue returns a text argument, or stdin when the argument is "-".
func (a *app) readValue(what, value string) (string, error) {
	if value != "-" {
		return value, nil
	}
	if a.stdinIsTerminal() {
		return "", &Error{Code: CodeUsage, Message: fmt.Sprintf("%s is to be read from stdin, but stdin is a terminal", what), Hint: "Pipe the text in, for example: < notes.md"}
	}
	content, err := io.ReadAll(io.LimitReader(a.stdin, maxInputBytes+1))
	if err != nil {
		return "", &Error{Code: CodeUsage, Message: fmt.Sprintf("reading %s from stdin: %s", what, err)}
	}
	if len(content) > maxInputBytes {
		return "", &Error{Code: CodeUsage, Message: fmt.Sprintf("%s on stdin is longer than %d bytes", what, maxInputBytes)}
	}
	return string(content), nil
}

func (a *app) newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the version of minka",
		Args:  requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			return a.ok(success{
				data:    map[string]string{"version": version},
				summary: "minka " + version,
				text:    func(w io.Writer) { fmt.Fprintf(w, "minka %s\n", version) },
			})
		},
	}
}

// pauseFor waits for d, or until the context ends.
func pauseFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
