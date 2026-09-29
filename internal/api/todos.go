package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
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
	Color  *string    `json:"color"` // nil for a group without a colour
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
	SeriesID              *string           `json:"seriesId"` // the series it is an occurrence of, nil for none
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

// CreateTodoGroupInput is a new group. A Color left out makes it one without.
type CreateTodoGroupInput struct {
	Title string  `json:"title"`
	Color *string `json:"color,omitempty"`
}

// UpdateTodoGroupInput changes a group, its title and its colour together or
// neither. What it leaves out stays as it is; a Color of "white" takes the
// colour away.
type UpdateTodoGroupInput struct {
	Title *string `json:"title,omitempty"`
	Color *string `json:"color,omitempty"`
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
func (c *Client) CreateTodoGroup(ctx context.Context, listID string, input CreateTodoGroupInput) (Result[TodoGroup], error) {
	return request[TodoGroup](ctx, c, http.MethodPost, todoListPath(listID)+"/groups", input)
}

// UpdateTodoGroup gives a group a new title, a new colour or both.
func (c *Client) UpdateTodoGroup(ctx context.Context, groupID string, input UpdateTodoGroupInput) (Result[TodoGroup], error) {
	return request[TodoGroup](ctx, c, http.MethodPatch, todoGroupPath(groupID), input)
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

// TodoListReference names the list a to-do stands in.
type TodoListReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// TodoGroupReference names the group a to-do stands in.
type TodoGroupReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Todo is one to-do with everything said under it.
type Todo struct {
	ID              string               `json:"id"`
	ProjectID       string               `json:"projectId"`
	ToolID          string               `json:"toolId"`
	ListID          string               `json:"listId"`
	GroupID         *string              `json:"groupId"` // nil for a to-do standing in its list directly
	Project         ProjectReference     `json:"project"`
	Tool            TodosToolReference   `json:"tool"`
	List            TodoListReference    `json:"list"`
	Group           *TodoGroupReference  `json:"group"`
	Title           string               `json:"title"`
	Completed       bool                 `json:"completed"`
	CompletedAt     *string              `json:"completedAt"`
	CompletedByName *string              `json:"completedByName"`
	Notes           string               `json:"notes"`       // the stored HTML, empty where the to-do has none
	NotesSource     string               `json:"notesSource"` // the notes as the editor opens them
	Attachments     []Attachment         `json:"attachments"`
	StartsOn        *string              `json:"startsOn"`
	DueOn           *string              `json:"dueOn"`
	IsOverdue       bool                 `json:"isOverdue"`
	Series          *TodoSeriesReference `json:"series"` // nil for a to-do that is no occurrence
	Assignees       []PersonReference    `json:"assignees"`
	Notified        []PersonReference    `json:"notified"` // the people told when it is done
	Subtasks        []TodoSubtask        `json:"subtasks"`
	// ListArchived says the list of the to-do lies in the archive of its tool:
	// the to-do is read, and nothing about it changes until the list is back.
	ListArchived bool            `json:"listArchived"`
	CommentCount int             `json:"commentCount"`
	CreatedAt    string          `json:"createdAt"`
	Creator      PersonReference `json:"creator"`
	Comments     []Comment       `json:"comments"` // oldest first
	AppURL       string          `json:"appUrl"`
}

// TodoSubtask is one line of work under a to-do. Assignee is nil for nobody,
// and for somebody who has left the project.
type TodoSubtask struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	Completed bool             `json:"completed"`
	Assignee  *PersonReference `json:"assignee"`
	DueOn     *string          `json:"dueOn"`
	IsOverdue bool             `json:"isOverdue"`
}

// CompletedTodos is one page of the done to-dos of a list. Next is where the
// page after it starts, and nil on the last page.
type CompletedTodos struct {
	Todos []TodoLine `json:"todos"`
	Next  *string    `json:"next"`
}

// AssignedTodo is an open to-do somebody is assigned to or holds an open
// subtask of, with where it stands. Subtasks are only the open ones of the
// person asking.
type AssignedTodo struct {
	ID           string             `json:"id"`
	Title        string             `json:"title"`
	Project      ProjectReference   `json:"project"`
	Tool         TodosToolReference `json:"tool"`
	List         TodoListReference  `json:"list"`
	DueOn        *string            `json:"dueOn"`
	IsOverdue    bool               `json:"isOverdue"`
	IsAssigned   bool               `json:"isAssigned"` // false where only a subtask of it is
	Assignees    []PersonReference  `json:"assignees"`
	Subtasks     []TodoSubtask      `json:"subtasks"`
	CommentCount int                `json:"commentCount"`
	AppURL       string             `json:"appUrl"`
}

// CreateTodoInput adds a to-do. Notes are HTML. One day alone is the due day,
// whichever field it is written in.
type CreateTodoInput struct {
	Title       string   `json:"title"`
	GroupID     string   `json:"groupId,omitempty"`
	Notes       *string  `json:"notes,omitempty"`
	StartsOn    *string  `json:"startsOn,omitempty"`
	DueOn       *string  `json:"dueOn,omitempty"`
	AssigneeIDs []string `json:"assigneeIds,omitempty"`
	NotifiedIDs []string `json:"notifiedIds,omitempty"`
}

// UpdateTodoInput changes a to-do. A nil field is left out and stays as it is.
// An empty day is sent as null, which takes the day off. A list of people is
// the whole set the to-do is to carry, so an empty one takes everybody off:
// it is sent as [] and never as null, which would change nothing.
type UpdateTodoInput struct {
	Title       *string
	Notes       *string
	StartsOn    *string
	DueOn       *string
	AssigneeIDs *[]string
	NotifiedIDs *[]string
}

func (in UpdateTodoInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if in.Title != nil {
		fields["title"] = *in.Title
	}
	if in.Notes != nil {
		fields["notes"] = *in.Notes
	}
	if in.StartsOn != nil {
		fields["startsOn"] = nullIfEmpty(*in.StartsOn)
	}
	if in.DueOn != nil {
		fields["dueOn"] = nullIfEmpty(*in.DueOn)
	}
	if in.AssigneeIDs != nil {
		fields["assigneeIds"] = append([]string{}, *in.AssigneeIDs...)
	}
	if in.NotifiedIDs != nil {
		fields["notifiedIds"] = append([]string{}, *in.NotifiedIDs...)
	}
	return json.Marshal(fields)
}

// MoveTodoInput says where a to-do goes: into ListID of the same tool, or its
// own list where that is empty; into GroupID, or into no group with NoGroup,
// and where neither is given into its own group within its own list and into
// none in another. It goes behind PreviousTodoID, or in front of NextTodoID,
// and with neither to the top of its place.
type MoveTodoInput struct {
	ListID         string
	GroupID        string
	NoGroup        bool
	PreviousTodoID string
	NextTodoID     string
}

func (in MoveTodoInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if in.ListID != "" {
		fields["listId"] = in.ListID
	}
	if in.GroupID != "" {
		fields["groupId"] = in.GroupID
	}
	if in.NoGroup {
		fields["groupId"] = nil
	}
	if in.PreviousTodoID != "" {
		fields["previousTodoId"] = in.PreviousTodoID
	}
	if in.NextTodoID != "" {
		fields["nextTodoId"] = in.NextTodoID
	}
	return json.Marshal(fields)
}

// CreateSubtaskInput adds a subtask at the end of a to-do.
type CreateSubtaskInput struct {
	Title      string `json:"title"`
	AssigneeID string `json:"assigneeId,omitempty"`
	DueOn      string `json:"dueOn,omitempty"`
}

// UpdateSubtaskInput changes a subtask. A nil field stays as it is; an empty
// assignee or day is sent as null, which takes it off.
type UpdateSubtaskInput struct {
	Title      *string
	AssigneeID *string
	DueOn      *string
	Completed  *bool
}

func (in UpdateSubtaskInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if in.Title != nil {
		fields["title"] = *in.Title
	}
	if in.AssigneeID != nil {
		fields["assigneeId"] = nullIfEmpty(*in.AssigneeID)
	}
	if in.DueOn != nil {
		fields["dueOn"] = nullIfEmpty(*in.DueOn)
	}
	if in.Completed != nil {
		fields["completed"] = *in.Completed
	}
	return json.Marshal(fields)
}

// MoveSubtaskInput names where a subtask goes among the subtasks of its
// to-do: behind PreviousSubtaskID, or in front of NextSubtaskID. With neither
// it goes to the top.
type MoveSubtaskInput struct {
	PreviousSubtaskID string `json:"previousSubtaskId,omitempty"`
	NextSubtaskID     string `json:"nextSubtaskId,omitempty"`
}

// Todo reads one to-do, in a list in use or in the archive.
func (c *Client) Todo(ctx context.Context, todoID string) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodGet, todoPath(todoID), nil)
}

// CreateTodo adds a to-do at the end of a list, or of a group of it.
func (c *Client) CreateTodo(ctx context.Context, listID string, input CreateTodoInput) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodPost, todoListPath(listID)+"/todos", input)
}

func (c *Client) UpdateTodo(ctx context.Context, todoID string, input UpdateTodoInput) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodPatch, todoPath(todoID), input)
}

// CompleteTodo ticks a to-do off; one done already stays as it is.
func (c *Client) CompleteTodo(ctx context.Context, todoID string) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodPost, todoPath(todoID)+"/complete", nil)
}

// ReopenTodo opens a done to-do again; one open already stays as it is.
func (c *Client) ReopenTodo(ctx context.Context, todoID string) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodPost, todoPath(todoID)+"/reopen", nil)
}

func (c *Client) MoveTodo(ctx context.Context, todoID string, input MoveTodoInput) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodPost, todoPath(todoID)+"/moves", input)
}

// DeleteTodo puts a to-do into the trash of its project.
func (c *Client) DeleteTodo(ctx context.Context, todoID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, todoPath(todoID), nil)
	return err
}

// CompletedTodos reads one page of the done to-dos of a list, the one done
// last first. A page after the first starts at the Next of the page before.
func (c *Client) CompletedTodos(ctx context.Context, listID string, limit int, before string) (Result[CompletedTodos], error) {
	values := url.Values{}
	values.Set("limit", strconv.Itoa(limit))
	if before != "" {
		values.Set("before", before)
	}
	return request[CompletedTodos](ctx, c, http.MethodGet, todoListPath(listID)+"/completed?"+values.Encode(), nil)
}

// MyTodosPageMax is the largest page of one's own to-dos the server serves. It
// cuts a larger one down to this without a word, and the answer carries no
// sign of its end but being shorter than asked for.
const MyTodosPageMax = 100

// ErrMyTodosPageOutOfBounds is a page size the server would refuse or cut.
var ErrMyTodosPageOutOfBounds = errors.New("a page of one's own to-dos holds 1 to 100 to-dos")

// MyTodos reads one page of the open to-dos of the person of the token across
// all projects. A page after the first starts after the to-do named in after.
// The limit is always sent and always within the bounds, so the server's
// silent cut is unreachable from here.
func (c *Client) MyTodos(ctx context.Context, limit int, after string) (Result[[]AssignedTodo], error) {
	if limit < 1 || limit > MyTodosPageMax {
		return Result[[]AssignedTodo]{}, ErrMyTodosPageOutOfBounds
	}
	values := url.Values{}
	values.Set("limit", strconv.Itoa(limit))
	if after != "" {
		values.Set("after", after)
	}
	return request[[]AssignedTodo](ctx, c, http.MethodGet, "/me/todos?"+values.Encode(), nil)
}

// CreateSubtask adds a subtask at the end of a to-do and answers the to-do.
func (c *Client) CreateSubtask(ctx context.Context, todoID string, input CreateSubtaskInput) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodPost, todoPath(todoID)+"/subtasks", input)
}

// UpdateSubtask changes a subtask and answers the to-do.
func (c *Client) UpdateSubtask(ctx context.Context, todoID, subtaskID string, input UpdateSubtaskInput) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodPatch, subtaskPath(todoID, subtaskID), input)
}

// MoveSubtask puts a subtask elsewhere among those of its to-do and answers
// the to-do.
func (c *Client) MoveSubtask(ctx context.Context, todoID, subtaskID string, input MoveSubtaskInput) (Result[Todo], error) {
	return request[Todo](ctx, c, http.MethodPost, subtaskPath(todoID, subtaskID)+"/moves", input)
}

// DeleteSubtask takes a subtask off, and succeeds for one that is gone
// already.
func (c *Client) DeleteSubtask(ctx context.Context, todoID, subtaskID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, subtaskPath(todoID, subtaskID), nil)
	return err
}

func todoPath(todoID string) string {
	return "/todos/" + url.PathEscape(todoID)
}

func subtaskPath(todoID, subtaskID string) string {
	return todoPath(todoID) + "/subtasks/" + url.PathEscape(subtaskID)
}

// nullIfEmpty is how a value that takes something off goes out: as null.
func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
