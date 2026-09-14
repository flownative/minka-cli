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

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
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

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type app struct {
	ctx    context.Context
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string

	stdinIsTerminal  func() bool
	stdoutIsTerminal func() bool
	readSecret       func() (string, error)

	hostFlag   string
	jsonFlag   bool
	quietFlag  bool
	styledFlag bool
}

// Run executes the CLI and returns its exit code.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	return newApp(ctx, stdin, stdout, stderr, getenv).run(args)
}

func newApp(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) *app {
	a := &app{ctx: ctx, stdin: stdin, stdout: stdout, stderr: stderr, getenv: getenv}
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
7 API error, 8 ambiguous, 9 validation.

Sign in first:
  minka auth login --host https://minka.example.com

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
		a.newAuthCmd(),
		a.newMeCmd(),
		a.newProjectsCmd(),
		a.newBoardsCmd(),
		a.newPeopleCmd(),
		a.newCardsCmd(),
		a.newStepsCmd(),
		a.newCommentsCmd(),
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
// "Id" has to be an ID, so that nothing else reaches the path of a request.
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
				if err := requireID(cmd, name, args[i]); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func requireID(cmd *cobra.Command, name, value string) error {
	if !uuidPattern.MatchString(value) {
		return usageErrorf(cmd, "%s %q is not an ID; IDs are UUIDs such as 0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", name, value)
	}
	return nil
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
			return &session{client: api.New(host, envToken, userAgent()), host: host, tokenSource: tokenFromEnvironment}, nil
		}
	}

	path, file, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = file.DefaultHost
	}
	if host == "" {
		return nil, &Error{Code: CodeAuth, Message: "not signed in to any Minka host", Hint: "Run: minka auth login --host https://minka.example.com"}
	}
	token := file.Hosts[host].Token
	if token == "" {
		return nil, &Error{Code: CodeAuth, Message: fmt.Sprintf("not signed in to %s", host), Hint: "Run: minka auth login --host " + host}
	}

	return &session{client: api.New(host, token, userAgent()), host: host, tokenSource: tokenFromConfig, configPath: path}, nil
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
