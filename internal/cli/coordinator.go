package cli

import (
	"encoding/json"
	"errors"
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
	cmd := group("coordinator", "Claim, hand over, release and show the session that coordinates a board",
		a.newBoardsCoordinatorClaimCmd(),
		a.newBoardsCoordinatorReleaseCmd(),
		a.newBoardsCoordinatorShowCmd(),
	)
	cmd.Long = `A board has at most one coordinator: one session of one person — a human or
an agent — that decides who works on what. The board shows it, with the
session and when it was last active.

The entry expires 4 hours after its last sign of life: a claim, or a wait of
"minka notifications wait" by the same person. Another session of the same
person takes it over with a claim of its own; another person is refused while
it holds. Anybody who may edit the board can release it, to clear a stale or
rogue entry.`
	return cmd
}

func (a *app) newBoardsCoordinatorClaimCmd() *cobra.Command {
	var session string
	cmd := &cobra.Command{
		Use:   "claim <boardId> --session <name>",
		Short: "Make a session of yours the coordinator of a board, or renew it",
		Long: fmt.Sprintf(`Makes the session named with --session the coordinator of the board, or
renews it where it is already. Another session of the same person hands over
to this one.

--session is required: a name you choose for people to read on the board,
such as "planning" or "Planung März": 1 to %d characters, blanks and
umlauts included, none of them a control or format character, as Minka
takes it. It is sent as written, blanks around it included. $%s is never
taken for it: a name nobody can read tells the people on the board nothing.

While another person coordinates the board, the claim exits 9 (validation)
with meta.apiCode coordinatorTaken; data and the hint name the holder and
until when the entry holds.`, maxCoordinatorSession, sessionVariable),
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
				return coordinatorRefusal(s, args[0], err)
			}
			coordinator := claimed.Value
			return a.ok(success{
				data:    claimed.Raw,
				summary: "Coordinating the board as " + describeCoordinator(coordinator),
				breadcrumbs: []Breadcrumb{
					{Action: "wait", Cmd: "minka notifications wait --board " + args[0] + " --json", Description: "Wait for news; every wait keeps the entry alive"},
					{Action: "release", Cmd: "minka boards coordinator release " + args[0], Description: "Give the board up"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Coordinating the board as %s.\n", describeCoordinator(coordinator))
					renderCoordinatorTimes(w, coordinator)
				},
			})
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "The name of the session that coordinates, readable on the board (required)")
	return cmd
}

// coordinatorRefusal says who holds a board that was refused, in the hint and
// in data, so that the caller can ask them or wait for the entry to expire.
func coordinatorRefusal(s *session, boardID string, err error) *Error {
	e := s.fail(err)
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != api.CodeCoordinatorTaken {
		return e
	}
	var body struct {
		Coordinator json.RawMessage `json:"coordinator"`
	}
	var holder api.Coordinator
	if json.Unmarshal(apiErr.Body, &body) != nil || len(body.Coordinator) == 0 || json.Unmarshal(body.Coordinator, &holder) != nil || holder.Person.Name == "" {
		e.Hint = "Somebody else coordinates this board. minka boards coordinator show " + boardID + " names them."
		return e
	}
	e.Data = body.Coordinator
	e.Hint = fmt.Sprintf("%s coordinates this board, last active %s; the entry holds until %s unless renewed. Ask them to hand over, wait for it to expire, or — where it is stale — release it with: minka boards coordinator release %s",
		describeCoordinator(holder), formatTime(holder.LastSeenAt), formatTime(holder.ExpiresAt), boardID)
	return e
}

func (a *app) newBoardsCoordinatorReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "release <boardId>",
		Short: "Take the coordinator off a board",
		Long: `Takes the coordinator off the board, whoever holds it: the holder gives it up,
and anybody who may edit the board can clear a stale or rogue entry. A board
nobody coordinates succeeds as well.`,
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
				summary: "Nobody coordinates the board now",
				breadcrumbs: []Breadcrumb{
					{Action: "claim", Cmd: "minka boards coordinator claim " + args[0] + " --session <name>", Description: "Coordinate the board"},
				},
				text: func(w io.Writer) { fmt.Fprintln(w, "Nobody coordinates the board now.") },
			})
		},
	}
}

func (a *app) newBoardsCoordinatorShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <boardId>",
		Short: "Show who coordinates a board",
		Long: `Shows the coordinator of the board as the board names it: the person, the
session, since when, when it was last active and when the entry expires.
data is null where nobody coordinates the board or the entry has expired.

A client of the project is not shown the coordinator at all: data is null
then as well, and a notice says that the board does not show it, so null
alone never means nobody for a client.`,
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
			// is not the same as naming nobody.
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(board.Raw, &fields)
			shown, named := fields["coordinator"]
			if !named || len(shown) == 0 {
				shown = json.RawMessage("null")
			}
			coordinator := board.Value.Coordinator
			summary := "Nobody coordinates " + board.Value.Title
			var notices []string
			switch {
			case !named:
				summary = board.Value.Title + " does not show you who coordinates it"
				notices = append(notices, "the board does not show you who coordinates it, as for a client of its project; data null says nothing about whether somebody does")
			case coordinator != nil:
				summary = board.Value.Title + " is coordinated by " + describeCoordinator(*coordinator)
			}
			return a.ok(success{
				data:    shown,
				summary: summary,
				notices: notices,
				breadcrumbs: []Breadcrumb{
					{Action: "claim", Cmd: "minka boards coordinator claim " + board.Value.ID + " --session <name>", Description: "Coordinate the board, or take it over from another session of yours"},
				},
				text: func(w io.Writer) {
					switch {
					case !named:
						fmt.Fprintln(w, "This board does not show you who coordinates it.")
						return
					case coordinator == nil:
						fmt.Fprintln(w, "Nobody coordinates this board.")
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

// describeCoordinator names a coordinator the way the board does: the person,
// what they are, and the session.
func describeCoordinator(c api.Coordinator) string {
	return fmt.Sprintf("%s (%s, session %s)", c.Person.Name, c.Person.Kind, c.Session)
}

func renderCoordinatorTimes(w io.Writer, c api.Coordinator) {
	fmt.Fprintf(w, "Since: %s\nLast active: %s\nExpires: %s\n", formatTime(c.Since), formatTime(c.LastSeenAt), formatTime(c.ExpiresAt))
}
