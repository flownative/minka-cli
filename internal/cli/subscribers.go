package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func (a *app) newCardsSubscribeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "subscribe <cardId> <person>",
		Short: "Have a person notified about a card",
		Long: `Puts a person on the list of people a card notifies about its comments, its due
date and its completion. Only members of the project of the card can be on the
list. Subscribing somebody who is on it already changes nothing and succeeds.

Minka puts people on the list by itself when they create a card, are assigned
to it or comment on it.

` + personHelp,
		Example: `  minka cards subscribe 0199... me`,
		Args:    requireArgs("cardId", "person"),
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
			members, err := s.client.People(a.ctx, card.Value.ProjectID)
			if err != nil {
				return s.fail(err)
			}
			person, err := resolvePerson(args[1], members.Value, a.meID(s))
			if err != nil {
				return err
			}

			subscribed, err := s.client.Subscribe(a.ctx, card.Value.ID, person.ID)
			if err != nil {
				return s.fail(err)
			}

			name := nameOf(person, subscribed.Value.Subscribers)
			summary := fmt.Sprintf("Subscribed %s to card %s", name, subscribed.Value.Title)
			if hasPerson(card.Value.Subscribers, person.ID) {
				summary = fmt.Sprintf("%s was subscribed to card %s already", name, subscribed.Value.Title)
			}
			return a.ok(success{
				data:        subscribed.Raw,
				summary:     summary,
				breadcrumbs: cardBreadcrumbs(subscribed.Value.ID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newCardsUnsubscribeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unsubscribe <cardId> <person>",
		Short: "Stop notifying a person about a card",
		Long: `Takes a person off the list of people a card notifies. Minka then also stops
putting them back by itself when they comment or are assigned. Taking off
somebody who is not on the list succeeds as well.

` + personHelp,
		Example: `  minka cards unsubscribe 0199... me`,
		Args:    requireArgs("cardId", "person"),
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
			members, err := s.client.People(a.ctx, card.Value.ProjectID)
			if err != nil {
				return s.fail(err)
			}
			person, err := resolvePerson(args[1], peopleOnCardFirst(card.Value.Subscribers, members.Value), a.meID(s))
			if err != nil {
				return err
			}

			if err := s.client.Unsubscribe(a.ctx, card.Value.ID, person.ID); err != nil {
				return s.fail(err)
			}

			wasSubscribed := hasPerson(card.Value.Subscribers, person.ID)
			name := nameOf(person, card.Value.Subscribers)
			summary := fmt.Sprintf("Unsubscribed %s from card %s", name, card.Value.Title)
			if !wasSubscribed {
				summary = fmt.Sprintf("%s was not subscribed to card %s", name, card.Value.Title)
			}
			return a.ok(success{
				data:        map[string]any{"cardId": card.Value.ID, "personId": person.ID, "wasSubscribed": wasSubscribed},
				summary:     summary,
				breadcrumbs: cardBreadcrumbs(card.Value.ID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}
