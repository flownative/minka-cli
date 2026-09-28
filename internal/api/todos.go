package api

import (
	"context"
	"net/http"
	"net/url"
)

// The To-dos tool of a project: its lists, the groups in a list and the
// to-dos standing in either. A list, a group and a to-do are addressed by
// their own identifier alone.

// TodosTool is one To-dos tool of a project.
type TodosTool struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	AppURL string `json:"appUrl"`
}

// TodosToolReference names the tool a list stands in.
type TodosToolReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// TodoListItem is one list as the lists of a tool name it, without its
// to-dos. Description is the whole stored HTML, not a lead.
type TodoListItem struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Archived       bool   `json:"archived"`
	OpenCount      int    `json:"openCount"`
	CompletedCount int    `json:"completedCount"`
	AppURL         string `json:"appUrl"`
}

// TodoList is one list with its groups and its open to-dos. The done ones are
// only counted here.
type TodoList struct {
	ID                string             `json:"id"`
	ProjectID         string             `json:"projectId"`
	ToolID            string             `json:"toolId"`
	Project           ProjectReference   `json:"project"`
	Tool              TodosToolReference `json:"tool"`
	Title             string             `json:"title"`
	Description       string             `json:"description"`       // the stored HTML, empty where the list has none
	DescriptionSource string             `json:"descriptionSource"` // the description as the editor opens it
	Attachments       []Attachment       `json:"attachments"`
	// Archived says the list lies in the archive of its tool: readable, and
	// closed to every write but the one that brings it back or trashes it.
	Archived       bool        `json:"archived"`
	OpenCount      int         `json:"openCount"`
	CompletedCount int         `json:"completedCount"`
	Todos          []TodoLine  `json:"todos"` // the open to-dos standing in no group
	Groups         []TodoGroup `json:"groups"`
	CommentCount   int         `json:"commentCount"`
	Comments       []Comment   `json:"comments"` // oldest first
	AppURL         string      `json:"appUrl"`
}

// TodoGroup is a named heading of a list with its open to-dos in their order.
type TodoGroup struct {
	ID     string     `json:"id"`
	ListID string     `json:"listId"`
	Title  string     `json:"title"`
	Todos  []TodoLine `json:"todos"`
}

// TodoLine is one to-do as a list names it.
type TodoLine struct {
	ID                    string            `json:"id"`
	Title                 string            `json:"title"`
	Completed             bool              `json:"completed"`
	CompletedAt           *string           `json:"completedAt"` // nil while open
	GroupID               *string           `json:"groupId"`     // nil for a to-do standing in its list directly
	Assignees             []PersonReference `json:"assignees"`
	StartsOn              *string           `json:"startsOn"`
	DueOn                 *string           `json:"dueOn"`
	IsOverdue             bool              `json:"isOverdue"`
	CommentCount          int               `json:"commentCount"`
	SubtaskCount          int               `json:"subtaskCount"`
	CompletedSubtaskCount int               `json:"completedSubtaskCount"`
	AppURL                string            `json:"appUrl"`
}

// CreateTodoListInput starts a list. Description is HTML and may be left out;
// one that carries no element the server serves is stored as the characters
// it consists of, without a word about it.
type CreateTodoListInput struct {
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
}

// UpdateTodoListInput changes a list. What it leaves out stays as it is; an
// empty Description empties the description, an empty Title is refused.
type UpdateTodoListInput struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
}

// MoveTodoGroupInput names where a group goes in its list: behind
// PreviousGroupID, or in front of NextGroupID. With neither it goes to the
// top of the list.
type MoveTodoGroupInput struct {
	PreviousGroupID string `json:"previousGroupId,omitempty"`
	NextGroupID     string `json:"nextGroupId,omitempty"`
}

// TodosTools lists the To-dos tools of a project that are in use, in the
// order of the project page.
func (c *Client) TodosTools(ctx context.Context, projectID string) (Result[[]TodosTool], error) {
	return request[[]TodosTool](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/todos", nil)
}

// TodoLists lists the lists on the page of a tool, in their order, or those
// in its archive, the one archived last first. Nothing of it is paged.
func (c *Client) TodoLists(ctx context.Context, toolID string, archived bool) (Result[[]TodoListItem], error) {
	path := "/todos-tools/" + url.PathEscape(toolID) + "/lists"
	if archived {
		path = "/todos-tools/" + url.PathEscape(toolID) + "/archive"
	}
	return request[[]TodoListItem](ctx, c, http.MethodGet, path, nil)
}

// TodoList reads one list, in use or in the archive.
func (c *Client) TodoList(ctx context.Context, listID string) (Result[TodoList], error) {
	return request[TodoList](ctx, c, http.MethodGet, todoListPath(listID), nil)
}

// CreateTodoList starts a list on top of a tool.
func (c *Client) CreateTodoList(ctx context.Context, toolID string, input CreateTodoListInput) (Result[TodoList], error) {
	return request[TodoList](ctx, c, http.MethodPost, "/todos-tools/"+url.PathEscape(toolID)+"/lists", input)
}

func (c *Client) UpdateTodoList(ctx context.Context, listID string, input UpdateTodoListInput) (Result[TodoList], error) {
	return request[TodoList](ctx, c, http.MethodPatch, todoListPath(listID), input)
}

// ArchiveTodoList takes a list into the archive of its tool.
func (c *Client) ArchiveTodoList(ctx context.Context, listID string) (Result[TodoList], error) {
	return request[TodoList](ctx, c, http.MethodPost, todoListPath(listID)+"/archive", nil)
}

// UnarchiveTodoList brings a list back onto the page of its tool.
func (c *Client) UnarchiveTodoList(ctx context.Context, listID string) (Result[TodoList], error) {
	return request[TodoList](ctx, c, http.MethodPost, todoListPath(listID)+"/unarchive", nil)
}

// DeleteTodoList puts a list into the trash, its groups and to-dos with it.
func (c *Client) DeleteTodoList(ctx context.Context, listID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, todoListPath(listID), nil)
	return err
}

// CreateTodoGroup adds a group at the end of a list.
func (c *Client) CreateTodoGroup(ctx context.Context, listID, title string) (Result[TodoGroup], error) {
	return request[TodoGroup](ctx, c, http.MethodPost, todoListPath(listID)+"/groups", map[string]string{"title": title})
}

// RenameTodoGroup gives a group a new title, the one thing of a group that
// changes.
func (c *Client) RenameTodoGroup(ctx context.Context, groupID, title string) (Result[TodoGroup], error) {
	return request[TodoGroup](ctx, c, http.MethodPatch, todoGroupPath(groupID), map[string]string{"title": title})
}

// MoveTodoGroup puts a group elsewhere in its list. A group never leaves its
// list.
func (c *Client) MoveTodoGroup(ctx context.Context, groupID string, input MoveTodoGroupInput) (Result[TodoGroup], error) {
	return request[TodoGroup](ctx, c, http.MethodPost, todoGroupPath(groupID)+"/moves", input)
}

// DeleteTodoGroup puts a group into the trash, its to-dos with it.
func (c *Client) DeleteTodoGroup(ctx context.Context, groupID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, todoGroupPath(groupID), nil)
	return err
}

func todoListPath(listID string) string {
	return "/todo-lists/" + url.PathEscape(listID)
}

func todoGroupPath(groupID string) string {
	return "/todo-groups/" + url.PathEscape(groupID)
}
