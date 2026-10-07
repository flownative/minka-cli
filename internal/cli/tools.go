package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/flownative/minka-cli/internal/api"
)

// oneTool describes the read of one tool by its ID alone, which every group of
// a kind of tool offers beside the list of the tools of a project.
type oneTool struct {
	group        string // the command group, which names what the tool holds
	kind         string // the kind of tool, as a sentence names it
	holds        string // the command that reads what the tool holds
	notFoundHint string
	read         func(ctx context.Context, client *api.Client, toolID string) (raw json.RawMessage, tool api.Tool, extra string, err error)
}

func (a *app) newOneToolCmd(t oneTool) *cobra.Command {
	return &cobra.Command{
		Use:   "tool <toolId>",
		Short: fmt.Sprintf("Show a %s with the project it stands in", t.kind),
		Long: fmt.Sprintf(`Shows a %s by its ID alone: its title, the project it stands in and its
address on the web. A tool the project no longer lists among those in use is
shown too. What it holds is read with "%s".`, t.kind, t.holds),
		Args: requireArgs("toolId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			raw, tool, extra, err := t.read(a.ctx, s.client, args[0])
			if err != nil {
				return toolRefusal(s, err, t.notFoundHint)
			}

			return a.ok(success{
				data:    raw,
				summary: fmt.Sprintf("%s %s in %s", t.kind, tool.Title, tool.Project.Name),
				breadcrumbs: []Breadcrumb{
					{Action: "holds", Cmd: "minka " + strings.Replace(t.holds, "<toolId>", tool.ID, 1), Description: "Read what the tool holds"},
					{Action: "tools", Cmd: "minka " + t.group + " tools " + tool.ProjectID, Description: "List the tools of this kind in the project"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s\nProject: %s\nID:      %s\nURL:     %s\n", tool.Title, tool.Project.Name, tool.ID, tool.AppURL)
					if extra != "" {
						fmt.Fprintln(w, extra)
					}
				},
			})
		},
	}
}

// asTool reads a tool whose answer is a plain tool.
func asTool(read func(client *api.Client, ctx context.Context, toolID string) (api.Result[api.Tool], error)) func(context.Context, *api.Client, string) (json.RawMessage, api.Tool, string, error) {
	return func(ctx context.Context, client *api.Client, toolID string) (json.RawMessage, api.Tool, string, error) {
		tool, err := read(client, ctx, toolID)
		if err != nil {
			return nil, api.Tool{}, "", err
		}
		return tool.Raw, tool.Value, "", nil
	}
}

func (a *app) newTodoListsToolCmd() *cobra.Command {
	return a.newOneToolCmd(oneTool{
		group: "todolists", kind: "To-dos tool", holds: "todolists list <toolId>",
		notFoundHint: "There is no such To-dos tool, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you. List the tools of a project with: minka todolists tools <projectId>",
		read:         asTool((*api.Client).TodosTool),
	})
}

func (a *app) newPostsToolCmd() *cobra.Command {
	return a.newOneToolCmd(oneTool{
		group: "posts", kind: "Posts tool", holds: "posts list <toolId>",
		notFoundHint: postsToolNotFoundHint,
		read:         asTool((*api.Client).PostsTool),
	})
}

func (a *app) newDocsToolCmd() *cobra.Command {
	return a.newOneToolCmd(oneTool{
		group: "docs", kind: "Docs & Files tool", holds: "docs list <toolId>",
		notFoundHint: filesToolNotFoundHint + " The tools of a project: minka docs tools <projectId>",
		read:         asTool((*api.Client).FilesTool),
	})
}

func (a *app) newCheckinsToolCmd() *cobra.Command {
	return a.newOneToolCmd(oneTool{
		group: "checkins", kind: "Check-ins tool", holds: "checkins list <toolId>",
		notFoundHint: checkinsToolNotFoundHint,
		read:         asTool((*api.Client).CheckinsTool),
	})
}

// A calendar answers with what it shows beside its events, which the text
// view names as well.
func (a *app) newEventsToolCmd() *cobra.Command {
	return a.newOneToolCmd(oneTool{
		group: "events", kind: "calendar", holds: "events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD",
		notFoundHint: calendarNotFoundHint,
		read: func(ctx context.Context, client *api.Client, toolID string) (json.RawMessage, api.Tool, string, error) {
			calendar, err := client.Calendar(ctx, toolID)
			if err != nil {
				return nil, api.Tool{}, "", err
			}
			return calendar.Raw, calendar.Value.Tool, "Shows:   " + calendar.Value.Content, nil
		},
	})
}
