package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

const personHelp = `A person is given by ID, as "me", by e-mail address or by name. A name has to
match exactly one person, ignoring case.`

const mentionHelp = `Every person carries a mention: the markup that names them in a text. Put
mention.markup from --json into a descriptionSource or a bodySource unchanged
and send that back as HTML; the person is then named, notified and subscribed
to the card. A mention holds in the circle it was asked for, so one of a
project belongs in texts of that project and one of a conversation in that
conversation.`

func (a *app) newPeopleCmd() *cobra.Command {
	return group("people", "List the people of a project", a.newPeopleListCmd())
}

func (a *app) newPeopleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <projectId>",
		Short: "List the members of a project, the people its cards can be assigned to",
		Long: "Lists the members of a project: whom its cards can be assigned to, and whom\n" +
			"its texts may name.\n\n" + mentionHelp,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			people, err := s.client.People(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    people.Raw,
				summary: plural(len(people.Value), "person", "people"),
				breadcrumbs: []Breadcrumb{
					{Action: "assign", Cmd: "minka cards assign <cardId> <person>", Description: "Assign a member to a card"},
					{Action: "mention", Cmd: "minka comments create <cardId> - --format html", Description: "Write a text that carries mention.markup"},
				},
				text: func(w io.Writer) { renderPeople(w, people.Value) },
			})
		},
	}
}

func (a *app) newConversationsPeopleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "people <conversationId>",
		Short: "List the two people of a private conversation",
		Long: "Lists the people of a private conversation, whom its messages may name.\n" +
			"Somebody who is not one of the two finds nothing there.\n\n" + mentionHelp,
		Args: requireArgs("conversationId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			people, err := s.client.ConversationPeople(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    people.Raw,
				summary: plural(len(people.Value), "person", "people"),
				breadcrumbs: []Breadcrumb{
					{Action: "mention", Cmd: "minka messages create " + args[0] + " - --format html", Description: "Say something that carries mention.markup"},
				},
				text: func(w io.Writer) { renderPeople(w, people.Value) },
			})
		},
	}
}

func renderPeople(w io.Writer, people []api.ProjectMember) {
	if len(people) == 0 {
		fmt.Fprintln(w, "No people.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tE-MAIL\tKIND")
	for _, person := range people {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", person.ID, person.Name, person.EmailAddress, natureOf(person))
	}
	_ = table.Flush()
}

func (a *app) newCardsAssignCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "assign <cardId> <person>",
		Short: "Assign a member of the project to a card",
		Long: `Assigns one person to a card. Only members of the project of the card can be
assigned. Assigning somebody who is assigned already changes nothing and
succeeds.

` + personHelp,
		Example: `  minka cards assign 0199... me
  minka cards assign 0199... anke@example.com`,
		Args: requireArgs("cardId", "person"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
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

			assigned, err := s.client.AssignCard(a.ctx, card.Value.ID, person.ID)
			if err != nil {
				return s.fail(err)
			}

			name := nameOf(person, assigned.Value.Assignees)
			summary := fmt.Sprintf("Assigned %s to card %s", name, assigned.Value.Title)
			if assigned.StatusCode == http.StatusOK {
				summary = fmt.Sprintf("%s was assigned to card %s already", name, assigned.Value.Title)
			}
			return a.ok(success{
				data:        assigned.Raw,
				summary:     summary,
				breadcrumbs: cardBreadcrumbs(assigned.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\n", summary, assigned.Value.AppURL)
				},
			})
		},
	}
}

func (a *app) newCardsUnassignCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unassign <cardId> <person>",
		Short: "Take a person off a card",
		Long: `Takes one person off a card. Taking off somebody who is not assigned changes
nothing and succeeds. Somebody who left the project can still be taken off.

` + personHelp,
		Example: `  minka cards unassign 0199... me`,
		Args:    requireArgs("cardId", "person"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
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

			person, err := resolvePerson(args[1], peopleOnCardFirst(card.Value.Assignees, members.Value), a.meID(s))
			if err != nil {
				return err
			}

			if err := s.client.UnassignCard(a.ctx, card.Value.ID, person.ID); err != nil {
				return s.fail(err)
			}

			wasAssigned := false
			for _, assignee := range card.Value.Assignees {
				wasAssigned = wasAssigned || assignee.ID == person.ID
			}
			name := nameOf(person, card.Value.Assignees)
			summary := fmt.Sprintf("Took %s off card %s", name, card.Value.Title)
			if !wasAssigned {
				summary = fmt.Sprintf("%s was not assigned to card %s", name, card.Value.Title)
			}
			return a.ok(success{
				data:        map[string]any{"cardId": card.Value.ID, "personId": person.ID, "wasAssigned": wasAssigned},
				summary:     summary,
				breadcrumbs: cardBreadcrumbs(card.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\n", summary, card.Value.AppURL)
				},
			})
		},
	}
}

// peopleOnCardFirst lists the people named on a card before the other members
// of its project, each once, so that somebody who left the project is still
// found. A person who is still a member keeps the e-mail address the member
// list has for them.
func peopleOnCardFirst(onCard []api.PersonReference, members []api.ProjectMember) []api.ProjectMember {
	memberByID := make(map[string]api.ProjectMember, len(members))
	for _, member := range members {
		memberByID[member.ID] = member
	}
	candidates := make([]api.ProjectMember, 0, len(onCard)+len(members))
	known := map[string]bool{}
	for _, person := range onCard {
		candidate, isMember := memberByID[person.ID]
		if !isMember {
			candidate = api.ProjectMember{ID: person.ID, Name: person.Name}
		}
		candidates = append(candidates, candidate)
		known[person.ID] = true
	}
	for _, member := range members {
		if !known[member.ID] {
			candidates = append(candidates, member)
		}
	}
	return candidates
}

// natureOf says what sort of being somebody is, and for an agent whom it acts
// for.
func natureOf(person api.ProjectMember) string {
	if person.Principal != nil {
		return person.Kind + " for " + person.Principal.Name
	}
	return person.Kind
}

func hasPerson(people []api.PersonReference, id string) bool {
	for _, person := range people {
		if person.ID == id {
			return true
		}
	}
	return false
}

// resolveMembers finds the people to assign to a new card on a board, before
// the card is created.
func (a *app) resolveMembers(s *session, board *api.Board, values []string) ([]api.ProjectMember, error) {
	if len(values) == 0 {
		return nil, nil
	}
	members, err := s.client.People(a.ctx, board.ProjectID)
	if err != nil {
		return nil, s.fail(err)
	}

	var resolved []api.ProjectMember
	seen := map[string]bool{}
	for _, value := range values {
		person, err := resolvePerson(value, members.Value, a.meID(s))
		if err != nil {
			return nil, err
		}
		if !seen[person.ID] {
			seen[person.ID] = true
			resolved = append(resolved, person)
		}
	}
	return resolved, nil
}

// meID returns a lookup of the ID of the person of the token, which asks the
// server only when "me" is used.
func (a *app) meID(s *session) func() (string, error) {
	return func() (string, error) {
		me, err := s.client.Me(a.ctx)
		if err != nil {
			return "", s.fail(err)
		}
		return me.Value.ID, nil
	}
}

// resolvePerson finds a person among candidates by ID, "me", e-mail address or
// name. An ID that matches no candidate is passed on, so the server says why
// it refuses it.
func resolvePerson(value string, candidates []api.ProjectMember, me func() (string, error)) (api.ProjectMember, error) {
	wanted := strings.TrimSpace(value)
	if strings.EqualFold(wanted, "me") {
		id, err := me()
		if err != nil {
			return api.ProjectMember{}, err
		}
		wanted = id
	}

	for _, candidate := range candidates {
		if candidate.ID == wanted || (candidate.EmailAddress != "" && strings.EqualFold(candidate.EmailAddress, wanted)) {
			return candidate, nil
		}
	}

	var matches []api.ProjectMember
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.Name, wanted) {
			matches = append(matches, candidate)
		}
	}
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return api.ProjectMember{}, &Error{
			Code:    CodeAmbiguous,
			Message: fmt.Sprintf("%q matches %d people", value, len(matches)),
			Hint:    "Give the ID or the e-mail address. Candidates: " + describePeople(matches),
		}
	case uuidPattern.MatchString(wanted):
		return api.ProjectMember{ID: wanted}, nil
	default:
		return api.ProjectMember{}, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("nobody in the project matches %q", value),
			Hint:    "People: " + describePeople(candidates),
		}
	}
}

func describePeople(people []api.ProjectMember) string {
	var names []string
	for _, person := range people {
		if person.EmailAddress != "" {
			names = append(names, fmt.Sprintf("%s <%s> (%s)", person.Name, person.EmailAddress, person.ID))
		} else {
			names = append(names, fmt.Sprintf("%s (%s)", person.Name, person.ID))
		}
	}
	return strings.Join(names, "; ")
}

// nameOf is the name of a person, taken from a list of assignees when the
// person was given only by ID.
func nameOf(person api.ProjectMember, assignees []api.PersonReference) string {
	if person.Name != "" {
		return person.Name
	}
	for _, assignee := range assignees {
		if assignee.ID == person.ID {
			return assignee.Name
		}
	}
	return person.ID
}

func assigneeNames(assignees []api.PersonReference) string {
	names := make([]string, len(assignees))
	for i, assignee := range assignees {
		names[i] = assignee.Name
	}
	return strings.Join(names, ", ")
}
