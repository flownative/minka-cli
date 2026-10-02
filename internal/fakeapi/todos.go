package fakeapi

import (
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// maxTodoListDescriptionLength is what the description of a list takes: as
// much as the description of a card does on the server, counted on what was
// sent and again on what would be stored.
const maxTodoListDescriptionLength = 100000

// TodosTool is a To-dos tool of a project: lists, the newest on top, each with
// its groups and its to-dos.
type TodosTool struct {
	ID        string
	ProjectID string
	Title     string

	// Archived is a tool whose status is no longer active: the project names
	// it among its tools no more, and at its own address it is read and
	// written as before.
	Archived bool

	// Trashed is a tool in the trash of its project: it, its lists and its
	// to-dos are at no address, and nobody's to-dos come from it.
	Trashed bool

	Lists []*TodoList // in the order of the page, the top first
}

// TodoList is one list of a tool. A list in the archive keeps its place among
// the lists and is read at its own address; it takes no write but the one that
// brings it back or puts it into the trash.
type TodoList struct {
	ID          string
	Title       string
	Description string // the HTML as stored
	Archived    bool
	ArchivedAt  time.Time
	Trashed     bool
	Groups      []*TodoGroup // in their order in the list
	Todos       []*Todo      // in their order, grouped or not
	Comments    []*Comment
	CreatorID   string

	// HiddenFromClients keeps the list from the clients of the project: to
	// them it, its to-dos and its series are at no address.
	HiddenFromClients bool

	Series []*TodoSeries // in the order they were made
}

// TodoGroup is a named heading of a list. A group in the trash takes its
// to-dos along: they are in no list and in no count.
type TodoGroup struct {
	ID    string
	Title string
	// Color is one of the colours of a board column, and empty for none. White
	// looks uncoloured, so a group given white carries none.
	Color   string
	Trashed bool
}

// todoGroupColors are the names a group takes as its colour, white included.
var todoGroupColors = []string{"white", "red", "orange", "yellow", "green", "blue", "aqua", "purple", "gray", "pink", "brown"}

// todoGroupColor reads the colour a body names for a group: nil where it names
// none or null, and a refusal for a value that is no string or no colour. The
// empty string is no colour either.
func todoGroupColor(w http.ResponseWriter, fields map[string]any) (*string, bool) {
	color, ok := stringField(w, fields, "color")
	if !ok || color == nil {
		return nil, ok
	}
	if !slices.Contains(todoGroupColors, *color) {
		unprocessable(w, "todoGroupColorUnknown", "The color has to be one of red, orange, yellow, green, blue, aqua, purple, gray, pink or brown, or white or null for none.")
		return nil, false
	}
	return color, true
}

// storedTodoGroupColor is how a group keeps the colour it was given.
func storedTodoGroupColor(color *string) string {
	if color == nil || *color == "white" {
		return ""
	}
	return *color
}

// Todo is one to-do of a list, standing in the list itself or in one of its
// groups. A done one is counted by its list and shown nowhere in it.
type Todo struct {
	ID            string
	Title         string
	GroupID       string // empty for a to-do standing in its list directly
	Notes         string // the HTML as stored
	StartsOn      *string
	DueOn         *string
	AssigneeIDs   []string // in the order they were put on
	NotifiedIDs   []string // the people told when it is done, in the same order
	Subtasks      []*Subtask
	CompletedAt   *time.Time
	CompletedByID string
	CreatedAt     time.Time
	CreatorID     string
	Comments      []*Comment
	Trashed       bool
	SeriesID      string // the series it is an occurrence of, empty for none
	OccursOn      string // the day of the series it stands for
}

// Subtask is one line of work under a to-do. It is no item of its own: it has
// no trash, and taking it off is the end of it.
type Subtask struct {
	ID         string
	Title      string
	Completed  bool
	AssigneeID string // empty for nobody
	DueOn      *string
}

func (s *Server) registerTodosRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectId}/todos", s.authenticated(s.todosTools))
	mux.HandleFunc("GET /api/v1/todos-tools/{toolId}", s.authenticated(s.todosTool))
	mux.HandleFunc("GET /api/v1/todos-tools/{toolId}/lists", s.authenticated(s.todoListsInUse))
	mux.HandleFunc("POST /api/v1/todos-tools/{toolId}/lists", s.authenticated(s.addTodoList))
	mux.HandleFunc("GET /api/v1/todos-tools/{toolId}/archive", s.authenticated(s.archivedTodoLists))
	mux.HandleFunc("GET /api/v1/todo-lists/{listId}", s.authenticated(s.todoList))
	mux.HandleFunc("PATCH /api/v1/todo-lists/{listId}", s.authenticated(s.updateTodoList))
	mux.HandleFunc("DELETE /api/v1/todo-lists/{listId}", s.authenticated(s.removeTodoList))
	mux.HandleFunc("POST /api/v1/todo-lists/{listId}/archive", s.authenticated(s.archiveTodoList))
	mux.HandleFunc("POST /api/v1/todo-lists/{listId}/unarchive", s.authenticated(s.unarchiveTodoList))
	mux.HandleFunc("POST /api/v1/todo-lists/{listId}/groups", s.authenticated(s.addTodoGroup))
	mux.HandleFunc("PATCH /api/v1/todo-groups/{groupId}", s.authenticated(s.editTodoGroup))
	mux.HandleFunc("DELETE /api/v1/todo-groups/{groupId}", s.authenticated(s.removeTodoGroup))
	mux.HandleFunc("POST /api/v1/todo-groups/{groupId}/moves", s.authenticated(s.moveTodoGroup))
	s.registerTodoItemRoutes(mux)
	s.registerTodoSeriesRoutes(mux)
}

// todosTools lists the To-dos tools of a project that are in use, in the
// order of the project page.
func (s *Server) todosTools(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}

	tools := []any{}
	for _, tool := range s.TodosTools {
		if tool.ProjectID == project.ID && !tool.Archived && !tool.Trashed {
			tools = append(tools, map[string]any{"id": tool.ID, "title": tool.Title, "appUrl": s.todosToolURL(tool)})
		}
	}
	writeJSON(w, http.StatusOK, tools)
}

// todosTool reads a tool by its identifier alone, with the project it stands
// in. A tool no longer in use is read as well.
func (s *Server) todosTool(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleTodosTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.toolResource(tool.ID, tool.ProjectID, tool.Title, s.todosToolURL(tool)))
}

// todoListsInUse lists the lists on the page of a tool, in their order and
// without their to-dos. Nothing is paged.
func (s *Server) todoListsInUse(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleTodosTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	lists := []any{}
	for _, list := range tool.Lists {
		if !list.Trashed && !list.Archived {
			lists = append(lists, s.todoListItemResource(tool, list))
		}
	}
	writeJSON(w, http.StatusOK, lists)
}

// archivedTodoLists lists the archive of a tool, the list archived last
// first, and the order of the page between lists archived in the same moment.
func (s *Server) archivedTodoLists(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleTodosTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	var archived []*TodoList
	for _, list := range tool.Lists {
		if !list.Trashed && list.Archived {
			archived = append(archived, list)
		}
	}
	slices.SortStableFunc(archived, func(a, b *TodoList) int { return b.ArchivedAt.Compare(a.ArchivedAt) })

	lists := []any{}
	for _, list := range archived {
		lists = append(lists, s.todoListItemResource(tool, list))
	}
	writeJSON(w, http.StatusOK, lists)
}

// addTodoList starts a list on top of a tool. The title is held before the
// description, as the controller does.
func (s *Server) addTodoList(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleTodosTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	description, ok := stringField(w, fields, "description")
	if !ok {
		return
	}
	if !validTodoTitle(w, "todoList", "a list", deref(title)) {
		return
	}
	stored, ok := storedTodoListDescription(w, deref(description))
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}

	list := &TodoList{ID: newID(), Title: phpTrim(deref(title)), Description: stored, CreatorID: person.ID}
	tool.Lists = append([]*TodoList{list}, tool.Lists...)
	w.Header().Set("Location", s.URL+"/api/v1/todo-lists/"+list.ID)
	writeJSON(w, http.StatusCreated, s.todoListResource(tool, list))
}

func (s *Server) todoList(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.todoListResource(tool, list))
}

// updateTodoList changes the title, the description or both. What the body
// leaves out, or sets to null, stays; an empty description empties it.
func (s *Server) updateTodoList(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	description, ok := stringField(w, fields, "description")
	if !ok {
		return
	}
	if title == nil && description == nil {
		unprocessable(w, "nothingToChange", "The body names neither a title nor a description.")
		return
	}
	if title != nil && !validTodoTitle(w, "todoList", "a list", *title) {
		return
	}
	stored := list.Description
	if description != nil {
		if stored, ok = storedTodoListDescription(w, *description); !ok {
			return
		}
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}

	if title != nil {
		list.Title = phpTrim(*title)
	}
	list.Description = stored
	writeJSON(w, http.StatusOK, s.todoListResource(tool, list))
}

// archiveTodoList takes a list into the archive of its tool. Why an archived
// list is not taken again is asked only after the write refused, so an
// archived project answers first.
func (s *Server) archiveTodoList(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	list.Archived = true
	list.ArchivedAt = s.momentAfterEveryArchiving(tool)
	writeJSON(w, http.StatusOK, s.todoListResource(tool, list))
}

// unarchiveTodoList brings a list back onto the page of its tool, where it
// takes the place it had.
func (s *Server) unarchiveTodoList(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if !list.Archived {
		unprocessable(w, "todoListNotArchived", "This list is not archived.")
		return
	}
	list.Archived = false
	list.ArchivedAt = time.Time{}
	writeJSON(w, http.StatusOK, s.todoListResource(tool, list))
}

// removeTodoList puts a list into the trash, from the page of its tool or
// from its archive, and its to-dos with it.
func (s *Server) removeTodoList(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	list.Trashed = true
	list.Archived = false
	w.WriteHeader(http.StatusNoContent)
}

// addTodoGroup adds a group at the end of a list.
func (s *Server) addTodoGroup(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	if !validTodoTitle(w, "todoGroup", "a group", deref(title)) {
		return
	}
	color, ok := todoGroupColor(w, fields)
	if !ok {
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}

	group := &TodoGroup{ID: newID(), Title: phpTrim(deref(title)), Color: storedTodoGroupColor(color)}
	list.Groups = append(list.Groups, group)
	writeJSON(w, http.StatusCreated, s.todoGroupResource(tool, list, group))
}

// editTodoGroup gives a group a new title, a new colour or both, together or
// not at all. A colour of null counts as named: it takes the colour away.
func (s *Server) editTodoGroup(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, group := s.visibleTodoGroup(r.PathValue("groupId"), person)
	if group == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	_, colorNamed := fields["color"]
	if title == nil && !colorNamed {
		unprocessable(w, "nothingToChange", "The body names neither a new title nor a color for the group.")
		return
	}
	if title != nil && !validTodoTitle(w, "todoGroup", "a group", *title) {
		return
	}
	color, ok := todoGroupColor(w, fields)
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if title != nil {
		group.Title = phpTrim(*title)
	}
	if colorNamed {
		group.Color = storedTodoGroupColor(color)
	}
	writeJSON(w, http.StatusOK, s.todoGroupResource(tool, list, group))
}

// moveTodoGroup puts a group behind "previousGroupId", or where that is not
// named in front of "nextGroupId", or where neither is named at the top of
// its list. A neighbour has to be another group of the same list; one that is
// no identifier at all is refused before the archive is asked.
func (s *Server) moveTodoGroup(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, group := s.visibleTodoGroup(r.PathValue("groupId"), person)
	if group == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	previous, ok := stringField(w, fields, "previousGroupId")
	if !ok {
		return
	}
	next, ok := stringField(w, fields, "nextGroupId")
	if !ok {
		return
	}
	for _, neighbour := range []*string{previous, next} {
		if neighbour != nil && !uuidPattern.MatchString(*neighbour) {
			todoPlaceUnknown(w)
			return
		}
	}
	for _, neighbour := range []*string{previous, next} {
		if neighbour != nil && (*neighbour == group.ID || groupOf(list, *neighbour) == nil) {
			if !s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
				todoPlaceUnknown(w)
			}
			return
		}
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}

	others := slices.DeleteFunc(slices.Clone(list.Groups), func(one *TodoGroup) bool { return one == group })
	at := 0
	switch {
	case previous != nil:
		at = slices.IndexFunc(others, func(one *TodoGroup) bool { return one.ID == *previous }) + 1
	case next != nil:
		at = slices.IndexFunc(others, func(one *TodoGroup) bool { return one.ID == *next })
	}
	list.Groups = slices.Insert(others, at, group)
	writeJSON(w, http.StatusOK, s.todoGroupResource(tool, list, group))
}

// removeTodoGroup puts a group into the trash, and its to-dos with it.
func (s *Server) removeTodoGroup(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, _, group := s.visibleTodoGroup(r.PathValue("groupId"), person)
	if group == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	group.Trashed = true
	w.WriteHeader(http.StatusNoContent)
}

// refusedAsNothingWritten says why a write on a list wrote nothing: the
// project is archived, or the list lies in the archive of its tool.
func (s *Server) refusedAsNothingWritten(w http.ResponseWriter, tool *TodosTool, list *TodoList) bool {
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return true
	}
	if list.Archived {
		unprocessable(w, "todoListArchived", "This list is archived. Nothing in it changes until it is back on the page of its tool.")
		return true
	}
	return false
}

// storedTodoListDescription is a description as the server keeps it. The
// length is held twice: against the text that was sent, and against the HTML
// it becomes, which escaping makes longer than plain text was.
func storedTodoListDescription(w http.ResponseWriter, value string) (string, bool) {
	const tooLong, message = "todoListDescriptionTooLong", "The description of a list is too long."
	stored, ok := storedRichText(w, tooLong, message, maxTodoListDescriptionLength, value)
	if !ok {
		return "", false
	}
	if utf8.RuneCountInString(stored) > maxTodoListDescriptionLength {
		unprocessable(w, tooLong, message)
		return "", false
	}
	return stored, true
}

func todoPlaceUnknown(w http.ResponseWriter) {
	unprocessable(w, "todoPlaceUnknown", "The list, group or neighbours named are not in this To-dos tool; taking something into another tool is done on the web.")
}

// validTodoTitle holds the title of a list or a group against what the server
// keeps, and names the case after what carries it.
func validTodoTitle(w http.ResponseWriter, thing, aThing, title string) bool {
	title = phpTrim(title)
	switch {
	case utf8.RuneCountInString(title) > maxTitleLength:
		unprocessable(w, thing+"TitleTooLong", "The title of "+aThing+" is too long.")
	case strings.IndexFunc(title, unicode.IsControl) >= 0:
		unprocessable(w, thing+"TitleHasControlCharacters", "The title of "+aThing+" must not contain control characters.")
	case title == "":
		unprocessable(w, thing+"TitleMissing", strings.ToUpper(aThing[:1])+aThing[1:]+" needs a title.")
	default:
		return true
	}
	return false
}

// momentAfterEveryArchiving is when a list goes into the archive: after every
// list of the tool that went before it, so that the archive has an order to
// read even while the clock of the fake stands still.
func (s *Server) momentAfterEveryArchiving(tool *TodosTool) time.Time {
	moment := s.now
	for _, list := range tool.Lists {
		if list.Archived && !list.ArchivedAt.Before(moment) {
			moment = list.ArchivedAt.Add(time.Second)
		}
	}
	return moment
}

// visibleTodosTool finds a tool that is not in the trash, in a project the
// person is a member of. A tool that is no longer in use is found as well.
func (s *Server) visibleTodosTool(toolID string, person *Person) *TodosTool {
	if !uuidPattern.MatchString(toolID) {
		return nil
	}
	for _, tool := range s.readableTodosTools() {
		if tool.ID == toolID && s.visibleProject(tool.ProjectID, person) != nil {
			return tool
		}
	}
	return nil
}

// visibleTodoList finds a list by its identifier alone, in use or in the
// archive, and nothing of the trash.
func (s *Server) visibleTodoList(listID string, person *Person) (*TodosTool, *TodoList) {
	if !uuidPattern.MatchString(listID) {
		return nil, nil
	}
	for _, tool := range s.readableTodosTools() {
		for _, list := range tool.Lists {
			if list.ID != listID || list.Trashed {
				continue
			}
			if s.visibleProject(tool.ProjectID, person) == nil || !s.showsTheList(list, person) {
				return nil, nil
			}
			return tool, list
		}
	}
	return nil, nil
}

// visibleTodoGroup finds a group of a list in use. A group of a list in the
// archive is at no address until the list is back.
func (s *Server) visibleTodoGroup(groupID string, person *Person) (*TodosTool, *TodoList, *TodoGroup) {
	if !uuidPattern.MatchString(groupID) {
		return nil, nil, nil
	}
	for _, tool := range s.readableTodosTools() {
		for _, list := range tool.Lists {
			group := groupOf(list, groupID)
			if group == nil || list.Trashed || list.Archived {
				continue
			}
			if s.visibleProject(tool.ProjectID, person) == nil || !s.showsTheList(list, person) {
				return nil, nil, nil
			}
			return tool, list, group
		}
	}
	return nil, nil, nil
}

// showsTheList says whether the list is shown to the person at all: a client
// is not shown one hidden from the clients.
func (s *Server) showsTheList(list *TodoList, person *Person) bool {
	return !list.HiddenFromClients || !s.isAClient(person.ID)
}

// groupOf is a group of the list that is out of the trash.
func groupOf(list *TodoList, groupID string) *TodoGroup {
	for _, group := range list.Groups {
		if group.ID == groupID && !group.Trashed {
			return group
		}
	}
	return nil
}

// countedTodos are the to-dos a list shows and counts: out of the trash, and
// in no group that is in it.
func countedTodos(list *TodoList) []*Todo {
	var todos []*Todo
	for _, todo := range list.Todos {
		if !todo.Trashed && (todo.GroupID == "" || groupOf(list, todo.GroupID) != nil) {
			todos = append(todos, todo)
		}
	}
	return todos
}

func openAndDone(list *TodoList) (int, int) {
	open, done := 0, 0
	for _, todo := range countedTodos(list) {
		if todo.CompletedAt == nil {
			open++
		} else {
			done++
		}
	}
	return open, done
}

// openTodoLines are the open to-dos of one group of a list, or those standing
// in no group where the group is empty, in their order.
func (s *Server) openTodoLines(tool *TodosTool, list *TodoList, groupID string) []any {
	lines := []any{}
	for _, todo := range countedTodos(list) {
		if todo.CompletedAt == nil && todo.GroupID == groupID {
			lines = append(lines, s.todoLineResource(tool, todo))
		}
	}
	return lines
}

func (s *Server) todoListItemResource(tool *TodosTool, list *TodoList) map[string]any {
	open, done := openAndDone(list)
	return map[string]any{
		"id": list.ID, "projectId": tool.ProjectID, "title": list.Title, "description": list.Description, "archived": list.Archived,
		"openCount": open, "completedCount": done, "appUrl": s.todoListURL(tool, list),
	}
}

func (s *Server) todoListResource(tool *TodosTool, list *TodoList) map[string]any {
	open, done := openAndDone(list)
	groups := []any{}
	for _, group := range list.Groups {
		if !group.Trashed {
			groups = append(groups, s.todoGroupResource(tool, list, group))
		}
	}
	comments := []any{}
	for _, comment := range list.Comments {
		comments = append(comments, s.commentResource(tool.ProjectID, comment))
	}
	return map[string]any{
		"id": list.ID, "projectId": tool.ProjectID, "toolId": tool.ID,
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"title": list.Title, "description": list.Description, "descriptionSource": list.Description,
		"attachments": s.attachmentsIn(tool.ProjectID, list.Description),
		"archived":    list.Archived, "openCount": open, "completedCount": done,
		"todos": s.openTodoLines(tool, list, ""), "groups": groups,
		"commentCount": len(list.Comments), "comments": comments,
		"appUrl": s.todoListURL(tool, list),
	}
}

func (s *Server) todoGroupResource(tool *TodosTool, list *TodoList, group *TodoGroup) map[string]any {
	var color any
	if group.Color != "" {
		color = group.Color
	}
	return map[string]any{"id": group.ID, "listId": list.ID, "title": group.Title, "color": color, "todos": s.openTodoLines(tool, list, group.ID)}
}

// todoLineResource is a to-do as a list names it: enough to tick it off or to
// open it.
func (s *Server) todoLineResource(tool *TodosTool, todo *Todo) map[string]any {
	done := 0
	for _, subtask := range todo.Subtasks {
		if subtask.Completed {
			done++
		}
	}
	return map[string]any{
		"id": todo.ID, "title": todo.Title, "completed": todo.CompletedAt != nil,
		"completedAt": timestampOrNil(todo.CompletedAt), "groupId": nullable(todo.GroupID),
		"assignees": s.peopleOnTodo(tool, todo.AssigneeIDs), "startsOn": todo.StartsOn, "dueOn": todo.DueOn,
		"isOverdue": s.isOverdue(todo.DueOn, todo.CompletedAt != nil), "seriesId": nullable(todo.SeriesID),
		"commentCount": len(todo.Comments), "subtaskCount": len(todo.Subtasks), "completedSubtaskCount": done,
		"appUrl": s.todoURL(tool, todo),
	}
}

func (s *Server) todosToolURL(tool *TodosTool) string {
	return s.URL + "/projects/" + tool.ProjectID + "/todos/" + tool.ID
}

func (s *Server) todoListURL(tool *TodosTool, list *TodoList) string {
	return s.todosToolURL(tool) + "/lists/" + list.ID
}

// readableTodosTools are the To-dos tools out of the trash, in use or not.
func (s *Server) readableTodosTools() []*TodosTool {
	var tools []*TodosTool
	for _, tool := range s.TodosTools {
		if !tool.Trashed {
			tools = append(tools, tool)
		}
	}
	return tools
}
