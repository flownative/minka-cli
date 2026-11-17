package api

import (
	"context"
	"net/http"
	"net/url"
)

// CreateProjectInput is a new project: its name, and the description and the
// category of the organization's list where given.
type CreateProjectInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
}

// CreateProject starts a project, and answers with it as the list of projects
// shows it.
func (c *Client) CreateProject(ctx context.Context, input CreateProjectInput) (Result[Project], error) {
	return request[Project](ctx, c, http.MethodPost, "/projects", input)
}

// AddProjectMember takes somebody of the organization into a project. The
// answer is the member as the list of people shows them: 201 where they are
// new in the project, 200 where they were in it already.
func (c *Client) AddProjectMember(ctx context.Context, projectID, personID string) (Result[ProjectMember], error) {
	return request[ProjectMember](ctx, c, http.MethodPost, projectPath(projectID)+"/people", struct {
		PersonID string `json:"personId"`
	}{personID})
}

// RemoveProjectMember takes somebody out of a project, the agents acting for
// them with them.
func (c *Client) RemoveProjectMember(ctx context.Context, projectID, personID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, projectPath(projectID)+"/people/"+url.PathEscape(personID), nil)
	return err
}

// ProjectTool is a tool of any type as the project page lists it.
type ProjectTool struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Type      string `json:"type"` // such as "board", "chat" or "files"
	Title     string `json:"title"`
	Position  int    `json:"position"` // from 1, counting the tools the reader sees
	AppURL    string `json:"appUrl"`
}

// ProjectTools lists every tool of a project in the order of its page.
func (c *Client) ProjectTools(ctx context.Context, projectID string) (Result[[]ProjectTool], error) {
	return request[[]ProjectTool](ctx, c, http.MethodGet, projectPath(projectID)+"/tools", nil)
}

// AddProjectToolInput is a new tool: its type, as the API names it, and its
// title.
type AddProjectToolInput struct {
	Type  string `json:"type"`
	Title string `json:"title"`
}

// AddProjectTool puts a new tool at the end of the project page.
func (c *Client) AddProjectTool(ctx context.Context, projectID string, input AddProjectToolInput) (Result[ProjectTool], error) {
	return request[ProjectTool](ctx, c, http.MethodPost, projectPath(projectID)+"/tools", input)
}

// RenameProjectTool gives a tool of any type a new title.
func (c *Client) RenameProjectTool(ctx context.Context, projectID, toolID, title string) (Result[ProjectTool], error) {
	return request[ProjectTool](ctx, c, http.MethodPatch, projectToolPath(projectID, toolID), struct {
		Title string `json:"title"`
	}{title})
}

// MoveProjectToolInput names the neighbour a tool goes next to; with neither,
// it goes in front.
type MoveProjectToolInput struct {
	PreviousToolID string `json:"previousToolId,omitempty"`
	NextToolID     string `json:"nextToolId,omitempty"`
}

// MoveProjectTool puts a tool elsewhere on the project page, and answers with
// it and its new position.
func (c *Client) MoveProjectTool(ctx context.Context, projectID, toolID string, input MoveProjectToolInput) (Result[ProjectTool], error) {
	return request[ProjectTool](ctx, c, http.MethodPost, projectToolPath(projectID, toolID)+"/moves", input)
}

// TrashProjectTool puts a tool in the trash of its project, from where only
// the web brings it back.
func (c *Client) TrashProjectTool(ctx context.Context, projectID, toolID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, projectToolPath(projectID, toolID), nil)
	return err
}

func projectPath(projectID string) string {
	return "/projects/" + url.PathEscape(projectID)
}

func projectToolPath(projectID, toolID string) string {
	return projectPath(projectID) + "/tools/" + url.PathEscape(toolID)
}
