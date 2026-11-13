package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/spf13/cobra"
)

const unseenCard = "(a card you cannot open)"

// cardNameForMinka turns the way people and the skill name a card into one
// Minka reads: one leading # comes off, a short or a long address of this
// host gives up the name in it, and an ID goes out in lowercase. Anything
// else goes out as given, for Minka to answer.
func (s *session) cardNameForMinka(value string) string {
	named := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if address, err := url.Parse(named); err == nil && address.Host != "" {
		if host, err := config.NormalizeHost(address.Scheme + "://" + address.Host); err != nil || host != s.host {
			return named
		}
		path := strings.TrimRight(address.Path, "/")
		if shortID, found := strings.CutPrefix(path, "/go/card/"); found && shortID != "" && !strings.Contains(shortID, "/") {
			return shortID
		}
		if at := strings.LastIndex(path, "/cards/"); at >= 0 {
			if id, ok := asID(path[at+len("/cards/"):]); ok {
				return id
			}
		}
		return named
	}
	id, _ := asID(named)
	return id
}

// linkedCardName is how a text view names a card at either end of a card step:
// the short identifier in front of the title where the board has one.
func linkedCardName(card api.LinkedCard) string {
	if !card.Seen() {
		return unseenCard
	}
	if card.ShortID != nil {
		return "#" + *card.ShortID + " " + card.Title
	}
	return card.Title
}

// linkedCardAddress is the short address of a linked card, which carries none
// of its own, made on the host of its long address; the long one where the
// board has no short identifiers.
func linkedCardAddress(card api.LinkedCard) string {
	if card.ShortID == nil {
		return card.AppURL
	}
	long, err := url.Parse(card.AppURL)
	if err != nil || long.Host == "" {
		return card.AppURL
	}
	return long.Scheme + "://" + long.Host + "/go/card/" + *card.ShortID
}

// cardStepEntry shows a card step as its card stands: no tick box of its own,
// the column saying whether it is done.
func cardStepEntry(card api.LinkedCard) string {
	if !card.Seen() {
		return "↳ " + unseenCard
	}
	people := ""
	if len(card.Assignees) > 0 {
		people = " (" + assigneeNames(card.Assignees) + ")"
	}
	return fmt.Sprintf("↳ %s  [%s]%s %s", linkedCardName(card), card.Column.Title, people, linkedCardAddress(card))
}

// stepOfLine names the cards a card is a step of, and nothing where there are
// none.
func stepOfLine(parents []api.LinkedCard) string {
	if len(parents) == 0 {
		return ""
	}
	names := make([]string, len(parents))
	for i, parent := range parents {
		names[i] = linkedCardName(parent)
	}
	return "Step of: " + strings.Join(names, ", ")
}

// stepName names a step in a summary: by its words, or by its card or its
// to-do, or by its position where the reader cannot see that.
func stepName(steps []api.Step, step api.Step) string {
	position := slices.IndexFunc(steps, func(read api.Step) bool { return read.ID == step.ID }) + 1
	switch {
	case step.Card != nil && step.Card.Seen():
		return "card step " + linkedCardName(*step.Card)
	case step.Card != nil:
		return fmt.Sprintf("step %d, a card you cannot open", position)
	case step.Todo != nil && step.Todo.Seen():
		return fmt.Sprintf("to-do step %q", step.Todo.Title)
	case step.Todo != nil:
		return fmt.Sprintf("step %d, a to-do you cannot open", position)
	}
	return fmt.Sprintf("step %q", step.Title)
}

// capitalized puts a summary that starts with a step name at the start of a
// sentence.
func capitalized(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// stepIsACardHint says where the title, people, day and column of a card step
// are changed: on its card. A card the reader cannot see cannot be changed,
// so only the link is left to remove.
func stepIsACardHint(cardID string, steps []api.Step, step api.Step) string {
	position := slices.IndexFunc(steps, func(read api.Step) bool { return read.ID == step.ID }) + 1
	if step.Card == nil || !step.Card.Seen() {
		return fmt.Sprintf("This step is a card you cannot open, so neither it nor the step can be changed. Only the link can be removed: minka cards steps delete %s %d", cardID, position)
	}
	return "This step is a card: its title, people, due day and column are the card's own. Change that card, for example minka cards move " + step.Card.ID + " --column Done to finish it."
}

// addCardStep makes another card a step of a card.
func (a *app) addCardStep(cardID, child string) error {
	s, err := a.session()
	if err != nil {
		return err
	}
	named := s.cardNameForMinka(child)
	given := cardID
	if err := s.resolveCard(a.ctx, &cardID); err != nil {
		return err
	}
	step, err := s.client.AddCardStep(a.ctx, cardID, named)
	if err != nil {
		e := s.fail(err)
		switch e.APICode {
		case apiCodeStepMissing:
			// A Minka without card steps passes over the card and misses the
			// text every step of its own needs.
			return &Error{Code: CodeNotFound, Message: s.host + " does not know a card as the step of another", Hint: cardStepsUnknownHint, APICode: e.APICode, HTTPStatus: e.HTTPStatus}
		case apiCodeStepCardUnknown:
			e.Hint = stepCardUnknownHint
		case apiCodeStepCardIsThisCard:
			e.Hint = "A card cannot be a step of itself. Name another card with --card."
		case apiCodeStepTitleAndCard:
			e.Hint = "minka sends either a text or a card, never both, so this answer points at a fault in minka."
		case apiCodeCardArchived:
			e.Hint = "The card lies in the archive of its board. Bring it back with: minka cards unarchive " + cardID
		}
		unreadStepHint(e, cardID)
		return e
	}

	shown, breadcrumbs := named, stepBreadcrumbs(cardID)
	if linked := step.Value.Card; linked != nil && linked.Seen() {
		shown = linkedCardName(*linked)
		breadcrumbs = append(breadcrumbs, Breadcrumb{Action: "show", Cmd: "minka cards show " + linked.ID, Description: "Show the card that is the step"})
	}
	parent := cardAsNamed(given, cardID)
	summary := fmt.Sprintf("Added card %s as a step of card %s", shown, parent)
	if step.StatusCode == http.StatusOK {
		summary = fmt.Sprintf("Card %s was a step of card %s already", shown, parent)
	}
	return a.ok(success{
		data:        step.Raw,
		summary:     summary,
		breadcrumbs: breadcrumbs,
		text: func(w io.Writer) {
			fmt.Fprintf(w, "%s.\n%s\n", summary, stepEntry(step.Value))
		},
	})
}

func (a *app) newCardsStepsMakeCardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "make-card <cardId> <step>",
		Short: "Make a step in words into a card of its own",
		Long: `Makes a step in words into a new card in Triage of the same board, with the
words of the step as its title and the step's people and due day. The step
keeps its place on the list and becomes a card step: it reads as the new card
stands from then on. A tick on the step does not carry over; the new card is in
Triage all the same. Words longer than a card's title of 200 characters are cut
with an ellipsis and stand whole in the description.

data is the new card. A step that is a card already is exit 9 (validation)
with stepIsACard, and one that is a to-do with stepIsATodo: a to-do does not
become a card. Words Minka refuses as the title of a card, and people who
may not be on a card of the project, refuse it with exit 9 as a new card would
be refused, and nothing is written. A Minka that knows no card steps yet is
exit 2 (not_found).

` + stepHelp,
		Example: `  minka cards steps make-card MNK-12 3`,
		Args:    requireArgs("cardId", "step"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			step, err := resolveStep(card.Value.Steps, args[1])
			if err != nil {
				return err
			}

			made, err := s.client.MakeStepIntoCard(a.ctx, card.Value.ID, step.ID)
			if err != nil {
				e := stepRefusal(s, err)
				var apiErr *api.Error
				switch {
				case e.Code == CodeNotFound && errors.As(err, &apiErr) && apiErr.Body == nil:
					// A Minka without the address answers with a page of its
					// web, not with the API's own 404.
					e.Message = s.host + " does not know a card as the step of another"
					e.Hint = cardStepsUnknownHint
				case e.APICode == apiCodeStepIsACard:
					e.Hint = "The step is a card already, so there is nothing to make. Read it with: minka cards steps list " + card.Value.ID
				case e.APICode == apiCodeStepIsATodo:
					e.Hint = "The step is a to-do, and a to-do does not become a card: only a step in words does. The to-do stays what it is, and the step with it."
				}
				return e
			}

			position := slices.IndexFunc(card.Value.Steps, func(read api.Step) bool { return read.ID == step.ID }) + 1
			name := made.Value.Title
			if made.Value.ShortID != nil {
				name = "#" + *made.Value.ShortID + " " + name
			}
			summary := fmt.Sprintf("Made %s into card %s in %s", stepName(card.Value.Steps, step), name, made.Value.Column.Title)
			return a.ok(success{
				data:    made.Raw,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka cards show " + made.Value.ID, Description: "Show the new card"},
					{Action: "list", Cmd: "minka cards steps list " + card.Value.ID, Description: "List the steps of the card it came from"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\n", summary, cardAddress(made.Value))
					if position > 0 {
						fmt.Fprintf(w, "Step %d of card %s now is that card.\n", position, card.Value.Title)
					}
				},
			})
		},
	}
}
