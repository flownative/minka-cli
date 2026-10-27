package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// maxCoordinatorSession is the longest name of a session a board keeps.
const maxCoordinatorSession = 64

func (a *app) newBoardsCoordinatorCmd() *cobra.Command {
	cmd := group("coordinator", "Claim, hand over, release and show your session that coordinates a board",
		a.newBoardsCoordinatorClaimCmd(),
		a.newBoardsCoordinatorReleaseCmd(),
		a.newBoardsCoordinatorShowCmd(),
	)
	cmd.Long = `Every account — a human's or an agent's — coordinates a board with one session
at a time, and each account has an entry of its own: several agents
coordinate the same board side by side. Only the account sees its entry;
nobody else's is shown, and nobody else's stands in the way.

The entry expires 4 hours after its last sign of life: a claim, or a wait of
"minka notifications wait" by the same account. Another session of the same
account takes it over with a claim of its own. A forgotten entry blocks nobody.`
	return cmd
}

func (a *app) newBoardsCoordinatorClaimCmd() *cobra.Command {
	var session string
	cmd := &cobra.Command{
		Use:   "claim <boardId> --session <name>",
		Short: "Make a session of yours coordinate a board, or renew its entry",
		Long: fmt.Sprintf(`Makes the session named with --session coordinate the board for your account,
or renews its entry. Another session of your account hands over to this one.
Other accounts' entries play no part: a claim is never refused for them.

--session is required: a name you choose to recognise the session by, such
as "planning" or "Planung März": 1 to %d characters, blanks and umlauts
included, none of them a control or format character, as Minka takes it. It
is sent as written, blanks around it included. $%s is never taken for it: a
name nobody can read tells you nothing when you look the entry up.`, maxCoordinatorSession, sessionVariable),
		Example: `  minka boards coordinator claim 0199... --session planning --json`,
		Args:    requireArgs("boardId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if session == "" {
				return usageErrorf(cmd, "name the session that coordinates the board with --session, a name people can read on the board, such as planning")
			}
			if !isABoardSession(session) {
				return usageErrorf(cmd, "--session takes 1 to %d characters, none of them a control or format character, not %q", maxCoordinatorSession, session)
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			claimed, err := s.client.ClaimCoordinator(a.ctx, args[0], session)
			if err != nil {
				return s.fail(err)
			}
			coordinator := claimed.Value
			return a.ok(success{
				data:    claimed.Raw,
				summary: "You coordinate the board as " + describeCoordinator(coordinator),
				breadcrumbs: []Breadcrumb{
					{Action: "wait", Cmd: "minka notifications wait --board " + args[0] + " --json", Description: "Wait for news; every wait keeps the entry alive"},
					{Action: "release", Cmd: "minka boards coordinator release " + args[0], Description: "Stop coordinating the board"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "You coordinate the board as %s.\n", describeCoordinator(coordinator))
					renderCoordinatorTimes(w, coordinator)
				},
			})
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "The name of the session that coordinates, readable by people (required)")
	return cmd
}

func (a *app) newBoardsCoordinatorReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "release <boardId>",
		Short: "Stop coordinating a board",
		Long: `Removes the entry of your account from the board, and nobody else's. A board
you do not coordinate succeeds as well.`,
		Args: requireArgs("boardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.ReleaseCoordinator(a.ctx, args[0]); err != nil {
				return s.fail(err)
			}
			return a.ok(success{
				data:    map[string]any{"boardId": args[0], "coordinator": nil},
				summary: "You no longer coordinate the board",
				breadcrumbs: []Breadcrumb{
					{Action: "claim", Cmd: "minka boards coordinator claim " + args[0] + " --session <name>", Description: "Coordinate the board again"},
				},
				text: func(w io.Writer) { fmt.Fprintln(w, "You no longer coordinate the board.") },
			})
		},
	}
}

func (a *app) newBoardsCoordinatorShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <boardId>",
		Short: "Show whether a session of yours coordinates a board",
		Long: `Shows the entry the board names: the person, the session, since when, when it
was last active and when the entry expires. Minka names your account's own
entry and nobody else's; a Minka without entries per account names whoever
coordinates the board, so the entry is shown as it came, never as yours. data
is null where the board names none or the entry has expired.

A client of the project cannot coordinate and is not shown an entry at all:
data is null then as well, and a notice says that the board does not show
it.`,
		Args: requireArgs("boardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			board, err := s.client.Board(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			// A board leaves the field out for a client of its project, which
			// is not the same as naming no entry.
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(board.Raw, &fields)
			shown, named := fields["coordinator"]
			if !named || len(shown) == 0 {
				shown = json.RawMessage("null")
			}
			coordinator := board.Value.Coordinator
			summary := "No session of yours coordinates " + board.Value.Title
			var notices []string
			switch {
			case !named:
				summary = board.Value.Title + " does not show you a coordinator"
				notices = append(notices, "the board does not show you a coordinator, as for a client of its project, who cannot coordinate")
			case coordinator != nil:
				// Named as it came, since a Minka without entries per account
				// names whoever coordinates the board.
				summary = board.Value.Title + " is coordinated by " + describeCoordinator(*coordinator)
			}
			return a.ok(success{
				data:    shown,
				summary: summary,
				notices: notices,
				breadcrumbs: []Breadcrumb{
					{Action: "claim", Cmd: "minka boards coordinator claim " + board.Value.ID + " --session <name>", Description: "Coordinate the board, renew the entry or take it over from another session of yours"},
				},
				text: func(w io.Writer) {
					switch {
					case !named:
						fmt.Fprintln(w, "This board does not show you a coordinator.")
						return
					case coordinator == nil:
						fmt.Fprintln(w, "No session of yours coordinates this board.")
						return
					}
					fmt.Fprintf(w, "Coordinated by %s.\n", describeCoordinator(*coordinator))
					renderCoordinatorTimes(w, *coordinator)
				},
			})
		},
	}
}

// isABoardSession is what Minka keeps as the name of a session: 1 to
// maxCoordinatorSession characters, none of them a control or a format
// character. Blanks count, at either end as well, as Minka takes the name
// untrimmed.
func isABoardSession(session string) bool {
	if session == "" || utf8.RuneCountInString(session) > maxCoordinatorSession {
		return false
	}
	return !strings.ContainsFunc(session, func(r rune) bool { return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) })
}

// describeCoordinator names an entry the way the board does: the person, what
// they are, and the session.
func describeCoordinator(c api.Coordinator) string {
	return fmt.Sprintf("%s (%s, session %s)", c.Person.Name, c.Person.Kind, c.Session)
}

func renderCoordinatorTimes(w io.Writer, c api.Coordinator) {
	fmt.Fprintf(w, "Since: %s\nLast active: %s\nExpires: %s\n", formatTime(c.Since), formatTime(c.LastSeenAt), formatTime(c.ExpiresAt))
}
