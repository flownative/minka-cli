package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

const (
	apiCodeNotAllowedToWriteConventions = "notAllowedToWriteConventions"
	apiCodeConventionsInvalid           = "conventionsInvalid"
)

// conventionsHelp says what the conventions of a board are and who writes
// them.
const conventionsHelp = `"How we work here" is a Markdown text per board on how the team works on it,
kept in versions with writer and time. It describes how work on the board is
done and nothing more: it grants no rights and widens nobody's permissions.

Only a human owner or admin of the organization who is on the project's team
writes it. Everybody else, agents always whatever their role, is refused with
exit 4 (forbidden) and meta.apiCode notAllowedToWriteConventions. A client of the project
neither reads nor writes it.`

func (a *app) newBoardsConventionsCmd() *cobra.Command {
	cmd := group("conventions", "Read how the team works on a board, or write it as a human owner or admin",
		a.newBoardsConventionsShowCmd(),
		a.newBoardsConventionsSetCmd(),
	)
	cmd.Long = conventionsHelp
	return cmd
}

func (a *app) newBoardsConventionsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <boardId>",
		Short: "Show how the team works on a board",
		Long: `Prints the conventions of a board as boards show does: a line with the
writer, the version and the time, the Markdown as stored, and an end marker.
Invisible format characters, such as bidi overrides, are shown as <U+XXXX>,
with a notice. data is the conventions field of the board as Minka sent it,
with text, updatedAt, updatedBy and version, or null where nobody has written
any yet. A client of the project is not shown them: data is null then as
well, and a notice says so.

` + conventionsHelp,
		Example: `  minka boards conventions show 0199...
  minka boards conventions show 0199... --json`,
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
			conventions := board.Value.Conventions
			data := json.RawMessage("null")
			var fields map[string]json.RawMessage
			if json.Unmarshal(board.Raw, &fields) == nil && len(fields["conventions"]) > 0 {
				data = fields["conventions"]
			}
			summary := "Nobody has written how the team works on " + board.Value.Title + " yet"
			notices := conventionsNotices(conventions)
			switch {
			case !namesField(board.Raw, "conventions"):
				summary = board.Value.Title + " does not show you how its team works"
				notices = append(notices, "the board does not show you its conventions, as for a client of its project")
			case conventions != nil:
				summary = fmt.Sprintf("How the team works on %s, version %d by %s", board.Value.Title, conventions.Version, conventions.UpdatedBy.Name)
			}
			return a.ok(success{
				data:    data,
				summary: summary,
				notices: notices,
				breadcrumbs: []Breadcrumb{
					{Action: "board", Cmd: "minka boards show " + board.Value.ID, Description: "Show the board with its columns, their roles and its cards"},
				},
				text: func(w io.Writer) {
					if !namesField(board.Raw, "conventions") {
						fmt.Fprintf(w, "%s.\n", summary)
						return
					}
					renderConventions(w, conventions)
				},
			})
		},
	}
}

func (a *app) newBoardsConventionsSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <boardId> <text|->",
		Short: "Write how the team works on a board (human owners and admins only)",
		Long: `Writes a new version of the conventions of a board from the text, Markdown
sent and stored as it is; give - to read it from stdin. An empty text empties
them. The text in force again writes no version. Every account that
coordinates the board right now, but the writer, is told of a new version.

Agents do not write the conventions: Minka refuses them always, and the text
is for their people to set. At most 20,000 characters, no control characters
but tabs and line breaks: exit 9 with meta.apiCode conventionsInvalid.

` + conventionsHelp,
		Example: `  minka boards conventions set 0199... - < how-we-work.md
  minka boards conventions set 0199... ""`,
		Args: requireArgs("boardId", "text"),
		RunE: func(_ *cobra.Command, args []string) error {
			text, err := a.readValue("the conventions", args[1])
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			written, err := s.client.WriteConventions(a.ctx, args[0], text)
			if err != nil {
				return conventionsRefusal(s, err)
			}

			breadcrumbs := []Breadcrumb{
				{Action: "show", Cmd: "minka boards conventions show " + args[0], Description: "Read the conventions in force"},
			}
			if written.StatusCode == 204 {
				summary := "The board has no conventions, and an empty text writes none"
				return a.ok(success{
					data:        json.RawMessage("null"),
					summary:     summary,
					breadcrumbs: breadcrumbs,
					text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
				})
			}
			conventions := written.Value
			summary := fmt.Sprintf("Version %d of the conventions is in force, by %s, %s", conventions.Version, conventions.UpdatedBy.Name, formatTime(conventions.UpdatedAt))
			return a.ok(success{
				data:        written.Raw,
				summary:     summary,
				breadcrumbs: breadcrumbs,
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

// conventionsRefusal classifies a refused write of the conventions and names
// the way on.
func conventionsRefusal(s *session, err error) *Error {
	e := s.fail(err)
	switch {
	case e.APICode == apiCodeNotAllowedToWriteConventions:
		e.Hint = "Only a human owner or admin of the organization who is on the project's team writes how the team works on a board; agents are refused always, whatever their role. Nothing was written. Ask such a person to write it."
	case e.APICode == apiCodeConventionsInvalid:
		e.Hint = "The conventions take at most 20,000 characters and no control characters but tabs and line breaks."
	case e.Code == CodeNotFound:
		e.Hint = "The board does not exist, lies in the trash, or belongs to a project you are not on the team of; a client of the project neither reads nor writes its conventions."
	}
	return e
}
