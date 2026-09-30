package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// projectNotFoundHint names why a project is at no address: it is not there,
// or it is somebody else's, and a client is not told apart from somebody
// outside the project.
const projectNotFoundHint = "It does not exist, you are not a member, or you work with it as a client: only its team renames and describes a project. Your projects: minka projects list"

// projectEditUnknownHint is for a 404 that is no answer of Minka's API: the
// host's Minka has no address to rename a project at, whatever the project.
const projectEditUnknownHint = "Its Minka is older than this minka expects and cannot rename or describe a project yet."

func (a *app) newProjectsEditCmd() *cobra.Command {
	var name, description string

	cmd := &cobra.Command{
		Use:   "edit <projectId>",
		Short: "Rename a project or change its description",
		Long: `Changes the name, the description or both; at least one of --name and
--description is required. What is not given stays as it is, even when
somebody else changed it meanwhile.

The description is plain text, a sentence or two on what the project is
about, and is stored as it is sent with the white space around it trimmed:
no Markdown is converted and nothing in it becomes a link or a mention.
--description "" empties it. --name "" does not empty the name: a project
needs one, so an empty name exits 9 (validation) with meta.apiCode
projectNameMissing. A name is one line of at most 200 characters, a
description at most 2000 characters; Minka refuses anything longer with
projectNameTooLong or projectDescriptionTooLong, and a name with a line
break or another control character with projectNameHasControlCharacters.

Only the project's team may change it. A client of the project gets exit 2
like somebody who is not in it at all. An archived project takes no change:
exit 9 with projectArchived. A host whose Minka is older than this address
answers with a 404 of its web instead of its API: exit 2, and the message and
the hint say that the host cannot rename a project, not that the project is
missing.

The answer is the project as "projects list" gives it.`,
		Example: `  minka projects edit 0199... --name "Summer party 2027"
  minka projects edit 0199... --description - < about.txt
  minka projects edit 0199... --description ""`,
		Args: requireArgs("projectId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			var input api.UpdateProjectInput
			if cmd.Flags().Changed("name") {
				input.Name = &name
			}
			if cmd.Flags().Changed("description") {
				text, err := a.readValue("the description", description)
				if err != nil {
					return err
				}
				input.Description = &text
			}
			if input.Name == nil && input.Description == nil {
				return usageErrorf(cmd, "nothing to change, give --name, --description or both")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			project, err := s.client.UpdateProject(a.ctx, args[0], input)
			if err != nil {
				return projectEditRefusal(s, err)
			}

			summary := "Changed project " + project.Value.Name
			return a.ok(success{
				data:    project.Raw,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "boards", Cmd: "minka boards list " + project.Value.ID, Description: "List the boards of the project"},
					{Action: "projects", Cmd: "minka projects list", Description: "List your projects"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\n", summary, project.Value.AppURL)
					if project.Value.Description != "" {
						fmt.Fprintf(w, "\n%s\n", project.Value.Description)
					}
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "New name; an empty one is refused, a name cannot be removed")
	flags.StringVar(&description, "description", "", "New description in plain text, - to read it from stdin, empty to remove it")

	return cmd
}

// projectEditRefusal tells Minka's own 404, which carries its JSON body, from
// the page of a host whose Minka does not know the address at all.
func projectEditRefusal(s *session, err error) *Error {
	e := s.fail(err)
	if e.Code != CodeNotFound {
		return e
	}
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Body == nil {
		e.Message = s.host + " cannot rename or describe a project"
		e.Hint = projectEditUnknownHint
		return e
	}
	e.Hint = projectNotFoundHint
	return e
}
