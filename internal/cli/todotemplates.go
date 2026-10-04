package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// What Minka answers a list asked of a template, or a template asked for a
// date or a tick.
const (
	apiCodeTemplateUnknown      = "templateUnknown"
	apiCodeTemplateArchived     = "templateArchived"
	apiCodeTemplateIntoLibrary  = "templateIntoLibrary"
	apiCodeTemplateTakesNoDates = "templateTakesNoDates"
	apiCodeTemplateTakesNoTicks = "templateTakesNoTicks"
)

const templateTakesNoDatesHint = "The to-do stands in a template, and a template takes no days: they are set on the list made from it, which starts without any. Give no day here; taking one off is allowed."

const templateTakesNoTicksHint = "The to-do stands in a template, where nothing is ticked off: that happens on a list made from it with minka todolists start --tool <toolId> --template <templateId>."

func (a *app) newTodoListsTemplatesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "templates",
		Short: "List the to-do list templates of the organization",
		Long: `Lists the templates in use, in their order, as "todolists list" lists the
lists of a tool, and names the project and the To-dos tool of the library they
stand in. A template is the team's alone: a client, and anybody outside the
organization's team, is in no library and exits 2 (not_found).

A template is a list of that tool and is read and changed as one: "todolists
show", "todolists edit", "todogroups …", "todos …" with the IDs given here. A
new template is "todolists create --tool <library toolId>", and the archived
ones are "todolists list <library toolId> --archived". A template takes no days
and no ticks: Minka refuses them with 9 (validation) and meta.apiCode
templateTakesNoDates or templateTakesNoTicks.

"todolists start" makes a list of a project from a template.`,
		Example: `  minka todolists templates --json`,
		Args:    requireArgs(),
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			library, err := s.client.TodoListTemplates(a.ctx)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "You are in no template library: templates are the team's alone, and a client or anybody outside the organization's team has none."
				}
				return e
			}

			toolID := library.Value.ToolID
			return a.ok(success{
				data:    library.Raw,
				summary: plural(len(library.Value.Templates), "template", "templates"),
				breadcrumbs: []Breadcrumb{
					{Action: "start", Cmd: "minka todolists start --tool <toolId> --template <templateId>", Description: "Make a list of a project from a template"},
					{Action: "show", Cmd: "minka todolists show <templateId>", Description: "Read a template with its groups and to-dos"},
					{Action: "archive", Cmd: "minka todolists list " + toolID + " --archived", Description: "List the archived templates"},
					{Action: "create", Cmd: "minka todolists create --tool " + toolID + " --title <title>", Description: "Start a new template"},
				},
				text: func(w io.Writer) { renderTodoListTemplates(w, library.Value) },
			})
		},
	}
}

func (a *app) newTodoListsStartCmd() *cobra.Command {
	var toolID, templateID, title string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Make a to-do list of a project from a template",
		Long: `Makes a list on top of a To-dos tool from a template: its description, its
groups with their colours and order, its to-dos with their notes and
subtasks. The new list starts fresh, nothing ticked off and no days on it, and
its files are copies in the project of the list. Comments and repeating to-dos
stay with the template. Nobody is notified, and the clients of the project are
not shown the new list.

The list takes the template's title unless --title is given; --title "" is
refused (todoListTitleMissing). There is no flag for a description, a day or
people: Minka takes none of them here.

The people on the to-dos and subtasks stay where they are members of the
project. The others are left off, and the answer names them in leftOff, the
summary and the text; [] where nobody was left off. Somebody who is no longer
on the organization's team is left off without being named.

Every call makes another list: running it twice gives two.

Refusals: templateUnknown (no template you see: unknown, in the trash, a list of
a project, or you are in no library), templateArchived, templateIntoLibrary
(--tool is the library's own tool) and projectArchived, all 9 (validation); a
tool that is unknown or no longer in use exits 2 (not_found).`,
		Example: `  minka todolists start --tool 0199... --template 0199... --json
  minka todolists start --tool 0199... --template 0199... --title "Monthly closing (10-2026)"`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--tool", &toolID); err != nil {
				return err
			}
			if err := requireID(cmd, "--template", &templateID); err != nil {
				return err
			}
			input := api.StartTodoListInput{Template: templateID}
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			list, err := s.client.StartTodoList(a.ctx, toolID, input)
			if err != nil {
				return startRefusal(s, err, templateID)
			}

			summary := "Started list " + list.Value.Title + " in " + list.Value.Tool.Title + " from a template"
			if left := list.Value.LeftOff; len(left) > 0 {
				noMember := "no member"
				if len(left) > 1 {
					noMember = "no members"
				}
				summary += fmt.Sprintf("; left off %s, %s of %s: %s", plural(len(left), "person", "people"), noMember, list.Value.Project.Name, personNames(left))
			}
			return a.ok(success{
				data:        list.Raw,
				summary:     summary,
				breadcrumbs: todoListBreadcrumbs(list.Value.TodoList),
				text:        func(w io.Writer) { renderStartedTodoList(w, list.Value) },
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&toolID, "tool", "", "ID of the To-dos tool the list goes into")
	flags.StringVar(&templateID, "template", "", "ID of the template, as todolists templates lists it")
	flags.StringVar(&title, "title", "", "Title of the list; the template's where it is left out")

	return cmd
}

// startRefusal says what to do about a list a template did not make. A 404
// is about the tool: everything about the template is a refusal of its own.
func startRefusal(s *session, err error, templateID string) *Error {
	e := s.fail(err)
	switch {
	case e.APICode == apiCodeTemplateUnknown:
		e.Hint = "There is no such template among those you see: it is unknown, in the trash or a list of a project, or you are in no template library, as a client is not. List the templates with: minka todolists templates"
	case e.APICode == apiCodeTemplateArchived:
		e.Hint = "The template lies in the archive of the library. Bring it back with minka todolists unarchive " + templateID + ", or start from one in use: minka todolists templates"
	case e.APICode == apiCodeTemplateIntoLibrary:
		e.Hint = "--tool names the library's own To-dos tool. A list is made from a template in a project; a template is copied as a template on the web (Duplicate). List the tools of a project with: minka todolists tools <projectId>"
	case strings.HasPrefix(e.APICode, "todoListTitle"):
		e.Hint = "Leave --title out to give the list the template's title."
	case e.Code == CodeNotFound:
		e.Hint = "There is no such To-dos tool, it belongs to a project you are not a member of, or it is no longer in use: a list from a template goes only into a tool in use. List the tools of a project with: minka todolists tools <projectId>"
	}
	return e
}

func personNames(people []api.PersonReference) string {
	names := make([]string, len(people))
	for i, person := range people {
		names[i] = person.Name
	}
	return strings.Join(names, ", ")
}

func renderTodoListTemplates(w io.Writer, library api.TodoListTemplates) {
	fmt.Fprintf(w, "Library: project %s, tool %s\n\n", library.ProjectID, library.ToolID)
	if len(library.Templates) == 0 {
		fmt.Fprintln(w, "No templates.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tTITLE\tTO-DOS")
	for _, template := range library.Templates {
		fmt.Fprintf(table, "%s\t%s\t%d\n", template.ID, template.Title, template.OpenCount)
	}
	_ = table.Flush()
}

func renderStartedTodoList(w io.Writer, list api.StartedTodoList) {
	fmt.Fprintf(w, "Started list %s in %s from a template.\nList: %s\n%s\n", list.Title, list.Tool.Title, list.ID, list.AppURL)
	if len(list.LeftOff) == 0 {
		fmt.Fprintln(w, "Left off: nobody")
		return
	}
	fmt.Fprintf(w, "Left off, no members of %s, so the list does not carry them:\n", list.Project.Name)
	for _, person := range list.LeftOff {
		fmt.Fprintf(w, "- %s (%s)\n", person.Name, person.ID)
	}
}
