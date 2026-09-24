package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// maxNotesLength is the most characters the server keeps as the notes of a
// card, counted after the line breaks were made uniform.
const maxNotesLength = 100000

// apiCodeNotesChanged is what Minka answers a guarded write over notes that
// were changed since they were read.
const apiCodeNotesChanged = "notesChanged"

// expectNone is the value of --expect for notes nobody has written yet.
const expectNone = "none"

// notesTimeLayout is the one form updatedAt has in the API: UTC with
// microseconds. --expect takes nothing else, so that a guard always names a
// time the server can match exactly.
const notesTimeLayout = "2006-01-02T15:04:05.000000Z"

func (a *app) newCardsNotesCmd() *cobra.Command {
	return group("notes", "Read and write the working notes on the back of a card",
		a.newCardsNotesShowCmd(),
		a.newCardsNotesSetCmd(),
	)
}

func (a *app) newCardsNotesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <cardId>",
		Short: "Show the working notes of a card",
		Long: `Shows the working notes on the back of a card: one text, read as light
Markdown and printed as it is stored, and who changed it last. The updatedAt
of the notes is what cards notes set --expect takes to write over them.`,
		Example: `  minka cards notes show MNK-3T
  minka cards notes show 0199... --json`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			notes, err := s.client.CardNotes(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			summary := "No working notes on the card yet"
			if notes.Value.UpdatedAt != nil {
				summary = "Working notes of the card, " + lastChange(notes.Value)
			}
			return a.ok(success{
				data:        notes.Raw,
				summary:     summary,
				breadcrumbs: notesBreadcrumbs(notes.Value),
				text:        func(w io.Writer) { renderNotes(w, notes.Value) },
			})
		},
	}
}

func (a *app) newCardsNotesSetCmd() *cobra.Command {
	var expect string
	var force bool

	cmd := &cobra.Command{
		Use:   "set <cardId> <text|->",
		Short: "Replace the working notes of a card",
		Long: `Replaces the working notes of a card as a whole with the text, read as light
Markdown and stored as it is sent. An empty text clears them. Give - as the
text to read it from stdin. Writing notifies nobody.

Somebody else may have changed the notes since you read them. Give the
updatedAt you read with --expect, or --expect none for notes nobody had
written yet: notes changed since are then left as they stand, the command
prints them and exits with 7 (meta.apiCode notesChanged). Merge your text into
them and send it again with their updatedAt. --force writes over whatever
stands, and one of the two flags is required.`,
		Example: `  minka cards notes set MNK-3T - --expect 2026-09-24T11:53:14.123456Z < notes.md
  minka cards notes set MNK-3T - --expect none < notes.md
  minka cards notes set MNK-3T "" --force`,
		Args: requireArgs("cardId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			guard, err := notesGuard(cmd, expect, force)
			if err != nil {
				return err
			}
			text, err := a.readValue("the notes", args[1])
			if err != nil {
				return err
			}
			if n := utf8.RuneCountInString(strings.ReplaceAll(text, "\r\n", "\n")); n > maxNotesLength {
				return &Error{Code: CodeUsage, Message: fmt.Sprintf("the notes have %d characters, Minka keeps at most %d", n, maxNotesLength)}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			notes, err := s.client.WriteCardNotes(a.ctx, args[0], text, guard)
			if err != nil {
				return notesFailure(s.fail(err), err)
			}

			summary := "Wrote the working notes of the card"
			return a.ok(success{
				data:        notes.Raw,
				summary:     summary,
				breadcrumbs: notesBreadcrumbs(notes.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s, %s.\n%s\n", summary, lastChange(notes.Value), notes.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&expect, "expect", "", "The updatedAt of the notes you read, or none for notes nobody had written yet")
	flags.BoolVar(&force, "force", false, "Write over the notes whatever stands")

	return cmd
}

// notesGuard turns --expect and --force into what the write sends: a guard,
// or none for a write that may replace whatever stands.
func notesGuard(cmd *cobra.Command, expect string, force bool) (*api.NotesGuard, error) {
	expected := cmd.Flags().Changed("expect")
	switch {
	case expected && force:
		return nil, usageErrorf(cmd, "--expect and --force exclude each other")
	case force:
		return nil, nil
	case !expected:
		return nil, &Error{
			Code:    CodeUsage,
			Message: "say which notes the text may replace: give --expect or --force",
			Hint:    "minka cards notes show <cardId> gives the updatedAt for --expect; --expect none is for notes nobody has written yet.",
		}
	case expect == expectNone:
		return &api.NotesGuard{}, nil
	}
	if _, err := time.Parse(notesTimeLayout, expect); err != nil {
		return nil, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("--expect takes the updatedAt of the notes as Minka gives it, such as 2026-09-24T11:53:14.123456Z, or none; %q is not", expect),
			Hint:    "minka cards notes show <cardId> gives the updatedAt to copy.",
		}
	}
	return &api.NotesGuard{UpdatedAt: &expect}, nil
}

// notesFailure adds to a refused write what an agent needs to merge: the notes
// as they stand, as the data of the error and printed for a person.
func notesFailure(e *Error, err error) *Error {
	var apiErr *api.Error
	if e.APICode != apiCodeNotesChanged || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return e
	}
	var body struct {
		Notes json.RawMessage `json:"notes"`
	}
	var current api.CardNotes
	if json.Unmarshal(apiErr.Body, &body) != nil || json.Unmarshal(body.Notes, &current) != nil {
		return e
	}

	e.Message = "the working notes were changed since they were read, nothing was written"
	e.Data = body.Notes
	e.Hint = "Merge your text into the notes as they stand and send it again with --expect " + expectationOf(current) + "."
	e.Text = func(w io.Writer) {
		fmt.Fprintln(w, "The notes as they stand:")
		fmt.Fprintln(w)
		renderNotes(w, current)
	}
	return e
}

func renderNotes(w io.Writer, notes api.CardNotes) {
	if notes.UpdatedAt == nil {
		fmt.Fprintln(w, "No working notes yet.")
		return
	}
	if notes.Body != "" {
		fmt.Fprintln(w, strings.TrimRight(notes.Body, "\n"))
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "Last changed by %s, %s. To write over it: --expect %s\n", changedBy(notes), formatTime(*notes.UpdatedAt), *notes.UpdatedAt)
}

// lastChange names who wrote the notes last and when, in local time.
func lastChange(notes api.CardNotes) string {
	if notes.UpdatedAt == nil {
		return "never written"
	}
	return fmt.Sprintf("last changed by %s, %s", changedBy(notes), formatTime(*notes.UpdatedAt))
}

func changedBy(notes api.CardNotes) string {
	if notes.UpdatedBy == nil {
		return "somebody"
	}
	return notes.UpdatedBy.Name
}

// expectationOf is the value of --expect that writes over the notes.
func expectationOf(notes api.CardNotes) string {
	if notes.UpdatedAt == nil {
		return expectNone
	}
	return *notes.UpdatedAt
}

func notesBreadcrumbs(notes api.CardNotes) []Breadcrumb {
	return []Breadcrumb{
		{Action: "set", Cmd: "minka cards notes set " + notes.CardID + " - --expect " + expectationOf(notes), Description: "Replace the notes, unless somebody changed them since"},
		{Action: "card", Cmd: "minka cards show " + notes.CardID, Description: "Show the card"},
	}
}
