package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// settingUp and listingTools say what a host cannot do whose Minka answers a
// 404 that is no answer of its API: it has no address for it, whatever the
// project.
const (
	settingUp    = "set up projects, their members and their tools"
	listingTools = "list the tools of a project"
)

func olderMinkaHint(cannot string) string {
	return "Its Minka is older than this minka expects and cannot " + cannot + " yet."
}

const (
	peopleAddNotFoundHint    = "The project does not exist, you are not a member, or you work with it as a client: only its team adds people. Your projects: minka projects list"
	peopleRemoveNotFoundHint = "The project does not exist or you are not a member, the person is not in it, or you work with it as a client and named somebody else: a client can only take themselves out. Its people: minka people list <projectId>"
	toolsListNotFoundHint    = "The project does not exist or you are not a member of it. Your projects: minka projects list"
	toolNotFoundHint         = "The project does not exist, you are not a member, or you work with it as a client: only its team changes its tools. Or the tool is not among those the project lists: it is in the trash or out of use. Its tools: minka tools list <projectId>"
)

// setupHints say what to do about a refusal of setting up a project, where
// the code alone does not. A <projectId> in one is replaced by the project the
// command was given.
var setupHints = map[string]string{
	"organizationTeamOnly":          "Only somebody on the team of the organization starts a project, and an agent only when the person it acts for could join it.",
	"projectCategoryUnknown":        "The list of categories is kept on the web. Those your projects are filed under: minka projects list",
	"personNotAnOrganizationMember": "Only somebody who belongs to the organization joins one of its projects; people are invited on the web.",
	"principalNotAProjectMember":    "An agent joins only a project the person it acts for is in. Add that person first.",
	"agentRetired":                  "The agent has left service and joins no project.",
	"projectWithoutClientAccess":    "A client joins only a project that works with clients, which is set on the web.",
	"lastProjectMember":             "A project keeps at least one human on its team. Add somebody else first: minka people add <projectId> <personId>",
	"toolPlaceUnknown":              "The neighbour has to be another tool the project lists. Its tools: minka tools list <projectId>",
}

// setupRefusal tells Minka's own 404, which carries its JSON body, from the
// page of a host whose Minka does not know the address at all, and adds the
// hint a refusal needs.
func setupRefusal(s *session, err error, projectID, notFoundHint, cannot string) *Error {
	e := s.fail(err)
	if hint, ok := setupHints[e.APICode]; ok {
		e.Hint = hint
		if projectID != "" {
			e.Hint = strings.ReplaceAll(hint, "<projectId>", projectID)
		}
	}
	if e.Code != CodeNotFound {
		return e
	}
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Body == nil {
		e.Message = s.host + " cannot " + cannot + " yet"
		e.Hint = olderMinkaHint(cannot)
		return e
	}
	e.Hint = notFoundHint
	return e
}

func (a *app) newProjectsCreateCmd() *cobra.Command {
	var name, description, category string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Start a project",
		Long: `Starts a project as the web does. You are its first member; an agent starts
it together with the person it acts for, who joins as well. The project has no
tools yet: add them with "minka tools add".

--name is required: one line of at most 200 characters. The description is
plain text of at most 2000 characters, - reads it from stdin. --category files
the project under a category of the organization's list, named in any case;
a name the list does not hold exits 9 with meta.apiCode projectCategoryUnknown
and no project is started. The other refusals are projectNameMissing,
projectNameTooLong, projectNameHasControlCharacters and
projectDescriptionTooLong.

Only somebody on the team of the organization starts a project: a client of
the organization, and an agent acting for one, get exit 4 with
organizationTeamOnly. A host whose Minka is older than this address exits 2,
and the hint says that the host cannot set up projects yet.

The answer is the project as "projects list" gives it.`,
		Example: `  minka projects create --name "Summer party 2027"
  minka projects create --name "Website" --category Customers --description - < about.txt`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("name") {
				return usageErrorf(cmd, "give the project a name with --name")
			}
			input := api.CreateProjectInput{Name: name}
			if cmd.Flags().Changed("description") {
				text, err := a.readValue("the description", description)
				if err != nil {
					return err
				}
				input.Description = &text
			}
			if cmd.Flags().Changed("category") {
				input.Category = &category
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			project, err := s.client.CreateProject(a.ctx, input)
			if err != nil {
				return setupRefusal(s, err, "", olderMinkaHint(settingUp), settingUp)
			}

			summary := "Started project " + project.Value.Name
			return a.ok(success{
				data:    project.Raw,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "tools", Cmd: "minka tools add " + project.Value.ID + " --type board --title <title>", Description: "Add a tool to the project"},
					{Action: "people", Cmd: "minka people add " + project.Value.ID + " <personId>", Description: "Add somebody to the project"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nID:  %s\nURL: %s\n", summary, project.Value.ID, project.Value.AppURL)
					if project.Value.Category != nil {
						fmt.Fprintf(w, "Category: %s\n", *project.Value.Category)
					}
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "Name of the project (required)")
	flags.StringVar(&description, "description", "", "Description in plain text, - to read it from stdin")
	flags.StringVar(&category, "category", "", "Name of the organization's category to file the project under")

	return cmd
}

func (a *app) newPeopleAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <projectId> <personId>",
		Short: "Add somebody of the organization to a project",
		Long: `Adds a person of the organization to a project, by their ID. Somebody who is
in the project already is no refusal: the command succeeds and says so. The
answer is the member as "people list" gives them.

Minka refuses, with exit 9 and meta.apiCode: personNotAnOrganizationMember for
somebody outside the organization or an ID of nobody, principalNotAProjectMember
for an agent whose human is not in the project, agentRetired for an agent that
left service, projectWithoutClientAccess for a client of the organization and a
project that does not work with clients, and projectArchived for an archived
project.

Only the project's team adds people: a client of the project gets exit 2 like
somebody who is not in it.`,
		Example: `  minka people add 0199... 0198...`,
		Args:    requireArgs("projectId", "personId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			member, err := s.client.AddProjectMember(a.ctx, args[0], args[1])
			if err != nil {
				return setupRefusal(s, err, args[0], peopleAddNotFoundHint, settingUp)
			}

			summary := fmt.Sprintf("Added %s to the project", member.Value.Name)
			if member.StatusCode == http.StatusOK {
				summary = fmt.Sprintf("%s was in the project already", member.Value.Name)
			}
			return a.ok(success{
				data:    member.Raw,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "people", Cmd: "minka people list " + args[0], Description: "List the people of the project"},
				},
				text: func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newPeopleRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <projectId> <person>",
		Short: "Take somebody out of a project, or leave it",
		Long: `Takes a person out of a project; "me" leaves it. The agents acting for the
person go with them: taking out an agent's human takes out the agent too.

Anybody may leave a project, an archived one and one they are a client of
included. Taking out somebody else is the team's, and is refused in an
archived project with exit 9 and meta.apiCode projectArchived. The last human
on the team stays: exit 9 with lastProjectMember. Somebody who is not in the
project is exit 2.

` + personHelp + ` The person is looked up among the people of the project.`,
		Example: `  minka people remove 0199... anke@example.com
  minka people remove 0199... me`,
		Args: requireArgs("projectId", "person"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			members, err := s.client.People(a.ctx, args[0])
			if err != nil {
				return setupRefusal(s, err, args[0], peopleRemoveNotFoundHint, settingUp)
			}
			person, err := resolvePerson(args[1], members.Value, a.meID(s))
			if err != nil {
				return err
			}
			if err := s.client.RemoveProjectMember(a.ctx, args[0], person.ID); err != nil {
				return setupRefusal(s, err, args[0], peopleRemoveNotFoundHint, settingUp)
			}

			name := person.Name
			if name == "" {
				name = person.ID
			}
			summary := fmt.Sprintf("Took %s out of the project", name)
			if strings.EqualFold(strings.TrimSpace(args[1]), "me") {
				summary = "Left the project"
			}
			return a.ok(success{
				data:    map[string]any{"projectId": args[0], "personId": person.ID},
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "projects", Cmd: "minka projects list", Description: "List your projects"},
				},
				text: func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newToolsCmd() *cobra.Command {
	return group("tools", "List the tools of a project of every type, add, rename, move and trash them",
		a.newToolsListCmd(),
		a.newToolsAddCmd(),
		a.newToolsRenameCmd(),
		a.newToolsMoveCmd(),
		a.newToolsTrashCmd(),
	)
}

func toolBreadcrumbs(projectID string) []Breadcrumb {
	return []Breadcrumb{
		{Action: "tools", Cmd: "minka tools list " + projectID, Description: "List the tools of the project"},
	}
}

func (a *app) newToolsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <projectId>",
		Short: "List every tool of a project in the order of its page",
		Long: `Lists the tools of a project of every type, in the order of the project page:
each with its position from 1, its type, ID and title. The types are board,
chat, posts, todos, checkins, files, forwards, calendar and decisions. A
tool out of use or in the trash is not listed, and a client sees only the
tools shown to clients; the positions count the tools listed.`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			tools, err := s.client.ProjectTools(a.ctx, args[0])
			if err != nil {
				return setupRefusal(s, err, args[0], toolsListNotFoundHint, listingTools)
			}
			return a.ok(success{
				data:    tools.Raw,
				summary: plural(len(tools.Value), "tool", "tools"),
				breadcrumbs: []Breadcrumb{
					{Action: "add", Cmd: "minka tools add " + args[0] + " --type <type> --title <title>", Description: "Add a tool to the project"},
				},
				text: func(w io.Writer) {
					if len(tools.Value) == 0 {
						fmt.Fprintln(w, "No tools.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "POSITION\tTYPE\tID\tTITLE")
					for _, tool := range tools.Value {
						fmt.Fprintf(table, "%d\t%s\t%s\t%s\n", tool.Position, tool.Type, tool.ID, tool.Title)
					}
					_ = table.Flush()
				},
			})
		},
	}
}

func (a *app) newToolsAddCmd() *cobra.Command {
	var toolType, title string
	cmd := &cobra.Command{
		Use:   "add <projectId>",
		Short: "Add a tool to a project",
		Long: `Adds a tool at the end of the project page, filled as on the web: a board with
its system columns, Posts with the default kinds. --type and --title are
required.

The type is sent as given, and Minka decides which types it takes: today
board, chat, posts, todos, checkins, calendar, decisions and files, in lower
case. It refuses any other with exit 9 and meta.apiCode toolTypeUnknown, and
its message names the types it takes. A forwards tool cannot be added: only
an import brings one. A title is one line of at most 200 characters:
toolTitleMissing, toolTitleTooLong and toolTitleHasControlCharacters are its
refusals. An archived project takes no tool: exit 9 with projectArchived.

Only the project's team adds tools: a client of the project gets exit 2 like
somebody who is not in it. The answer is the tool as "tools list" gives it.`,
		Example: `  minka tools add 0199... --type board --title "Development"
  minka tools add 0199... --type files --title "Docs & Files"`,
		Args: requireArgs("projectId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("type") || !cmd.Flags().Changed("title") {
				return usageErrorf(cmd, "give the tool a --type and a --title")
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			tool, err := s.client.AddProjectTool(a.ctx, args[0], api.AddProjectToolInput{Type: toolType, Title: title})
			if err != nil {
				return setupRefusal(s, err, args[0], toolNotFoundHint, settingUp)
			}
			return a.answerWithTool(tool, fmt.Sprintf("Added %s %s at position %d", tool.Value.Type, tool.Value.Title, tool.Value.Position))
		},
	}
	cmd.Flags().StringVar(&toolType, "type", "", "Type of the tool, such as board or files; Minka decides which it takes")
	cmd.Flags().StringVar(&title, "title", "", "Title of the tool")
	return cmd
}

func (a *app) newToolsRenameCmd() *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "rename <projectId> <toolId>",
		Short: "Rename a tool of any type",
		Long: `Gives a tool of any type, forwards included, a new title; --title is required.
The refusals are those of "tools add": toolTitleMissing, toolTitleTooLong,
toolTitleHasControlCharacters and projectArchived. Only the project's team
renames tools. The answer is the tool as "tools list" gives it.`,
		Example: `  minka tools rename 0199... 0198... --title "Releases"`,
		Args:    requireArgs("projectId", "toolId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("title") {
				return usageErrorf(cmd, "give the new title with --title")
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			tool, err := s.client.RenameProjectTool(a.ctx, args[0], args[1], title)
			if err != nil {
				return setupRefusal(s, err, args[0], toolNotFoundHint, settingUp)
			}
			return a.answerWithTool(tool, fmt.Sprintf("Renamed %s to %s", tool.Value.Type, tool.Value.Title))
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "New title of the tool")
	return cmd
}

func (a *app) newToolsMoveCmd() *cobra.Command {
	var after, before string
	cmd := &cobra.Command{
		Use:   "move <projectId> <toolId>",
		Short: "Move a tool to another place on the project page",
		Long: `Puts a tool elsewhere on the project page: --after right behind the tool
named, --before right in front of it, and with neither of the two in front of
all. The two flags exclude each other. The neighbour has to be another tool
the project lists: one of another project exits 9 with meta.apiCode
toolPlaceUnknown, the tool itself exits 1 before anything is written. An
archived project is exit 9 with projectArchived. Only the project's team
moves tools.

The answer is the moved tool with its new position.`,
		Example: `  minka tools move 0199... 0198... --after 0197...
  minka tools move 0199... 0198...`,
		Args: requireArgs("projectId", "toolId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if after != "" && before != "" {
				return usageErrorf(cmd, "give --after or --before, not both")
			}
			var input api.MoveProjectToolInput
			for _, neighbour := range []struct {
				flag, value string
				id          *string
			}{{"--after", after, &input.PreviousToolID}, {"--before", before, &input.NextToolID}} {
				if neighbour.value == "" {
					continue
				}
				id := neighbour.value
				if err := requireID(cmd, neighbour.flag, &id); err != nil {
					return err
				}
				if id == args[1] {
					return usageErrorf(cmd, "%s names the tool that is to move; name another tool of the project", neighbour.flag)
				}
				*neighbour.id = id
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			tool, err := s.client.MoveProjectTool(a.ctx, args[0], args[1], input)
			if err != nil {
				return setupRefusal(s, err, args[0], toolNotFoundHint, settingUp)
			}
			return a.answerWithTool(tool, fmt.Sprintf("Moved %s %s to position %d", tool.Value.Type, tool.Value.Title, tool.Value.Position))
		},
	}
	cmd.Flags().StringVar(&after, "after", "", "Put the tool right behind this tool (ID)")
	cmd.Flags().StringVar(&before, "before", "", "Put the tool right in front of this tool (ID)")
	return cmd
}

func (a *app) newToolsTrashCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trash <projectId> <toolId>",
		Short: "Move a tool to the trash of its project",
		Long: `Moves a tool of any type to the trash of its project, with everything it holds.
"minka trash list <projectId>" shows it there as kind tool. It cannot be
brought back through the API: that is done in Minka on the web. A second
trash of the same tool is exit 2, an archived project exit 9 with
projectArchived. Only the project's team trashes tools.`,
		Example: `  minka tools trash 0199... 0198...`,
		Args:    requireArgs("projectId", "toolId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.TrashProjectTool(a.ctx, args[0], args[1]); err != nil {
				return setupRefusal(s, err, args[0], toolNotFoundHint, settingUp)
			}
			summary := "Moved tool " + args[1] + " to the trash"
			return a.ok(success{
				data:    map[string]any{"projectId": args[0], "toolId": args[1]},
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "trash", Cmd: "minka trash list " + args[0], Description: "List the trash of the project"},
					{Action: "tools", Cmd: "minka tools list " + args[0], Description: "List the tools of the project"},
				},
				text: func(w io.Writer) { fmt.Fprintf(w, "%s.\nIt comes back only on the web.\n", summary) },
			})
		},
	}
}

func (a *app) answerWithTool(tool api.Result[api.ProjectTool], summary string) error {
	return a.ok(success{
		data:        tool.Raw,
		summary:     summary,
		breadcrumbs: toolBreadcrumbs(tool.Value.ProjectID),
		text: func(w io.Writer) {
			fmt.Fprintf(w, "%s.\nID:  %s\nURL: %s\n", summary, tool.Value.ID, tool.Value.AppURL)
		},
	})
}
