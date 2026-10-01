package fakeapi

import (
	"cmp"
	"encoding/base64"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// What the to-dos of a list take, as the server holds them.
const (
	maxTodoNotesLength    = 100000
	maxSubtaskLength      = 500
	todosOnAPage          = 50
	mostTodosOnAPage      = 100
	completedCursorLayout = "2006-01-02T15:04:05.000000-07:00"
)

// aWholeNumber is what a page size has to be before it is read as one.
var aWholeNumber = regexp.MustCompile(`^[0-9]{1,18}$`)

func (s *Server) registerTodoItemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/todo-lists/{listId}/todos", s.authenticated(s.addTodo))
	mux.HandleFunc("GET /api/v1/todo-lists/{listId}/completed", s.authenticated(s.completedTodos))
	mux.HandleFunc("GET /api/v1/todos/{todoId}", s.authenticated(s.todo))
	mux.HandleFunc("PATCH /api/v1/todos/{todoId}", s.authenticated(s.updateTodo))
	mux.HandleFunc("DELETE /api/v1/todos/{todoId}", s.authenticated(s.removeTodo))
	mux.HandleFunc("POST /api/v1/todos/{todoId}/complete", s.authenticated(s.completeTodo))
	mux.HandleFunc("POST /api/v1/todos/{todoId}/reopen", s.authenticated(s.reopenTodo))
	mux.HandleFunc("POST /api/v1/todos/{todoId}/moves", s.authenticated(s.moveTodo))
	mux.HandleFunc("POST /api/v1/todos/{todoId}/subtasks", s.authenticated(s.addSubtask))
	mux.HandleFunc("PATCH /api/v1/todos/{todoId}/subtasks/{subtaskId}", s.authenticated(s.updateSubtask))
	mux.HandleFunc("DELETE /api/v1/todos/{todoId}/subtasks/{subtaskId}", s.authenticated(s.removeSubtask))
	mux.HandleFunc("POST /api/v1/todos/{todoId}/subtasks/{subtaskId}/moves", s.authenticated(s.moveSubtask))
	mux.HandleFunc("GET /api/v1/me/todos", s.authenticated(s.myTodos))
}

// addTodo puts a to-do at the end of a list, or of one of its groups. Once the
// list is found, what the body says is held, the notes last, and only after
// the write refused does the server ask why: the project, the archive of the
// list, the group, the people, in that order.
func (s *Server) addTodo(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok1 := stringField(w, fields, "title")
	notes, ok2 := stringField(w, fields, "notes")
	startsOn, ok3 := stringField(w, fields, "startsOn")
	dueOn, ok4 := stringField(w, fields, "dueOn")
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return
	}
	assigneeIDs, ok := todoPeopleField(w, fields, "assigneeIds")
	if !ok {
		return
	}
	notifiedIDs, ok := todoPeopleField(w, fields, "notifiedIds")
	if !ok {
		return
	}
	groupID, ok := todoPlaceField(w, fields, "groupId")
	if !ok || !validTodoTitle(w, "todo", "a to-do", deref(title)) {
		return
	}
	start, due, ok := todoDates(w, deref(startsOn), deref(dueOn))
	if !ok {
		return
	}
	stored, ok := storedTodoNotes(w, deref(notes))
	if !ok || s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	if groupID != nil && groupOf(list, *groupID) == nil {
		todoPlaceUnknown(w)
		return
	}
	if !s.allMembers(w, tool, assigneeIDs) || !s.allMembers(w, tool, notifiedIDs) {
		return
	}

	todo := &Todo{ID: newID(), Title: phpTrim(deref(title)), GroupID: deref(groupID), Notes: stored, StartsOn: start, DueOn: due,
		AssigneeIDs: withoutRepeats(assigneeIDs), NotifiedIDs: withoutRepeats(notifiedIDs), CreatedAt: s.now, CreatorID: person.ID}
	list.Todos = append(list.Todos, todo)
	w.Header().Set("Location", s.URL+"/api/v1/todos/"+todo.ID)
	writeJSON(w, http.StatusCreated, s.todoResource(tool, list, todo))
}

func (s *Server) todo(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.todoResource(tool, list, todo))
}

// updateTodo changes any of title, notes, days and people. What the body
// leaves out stays; a day set to null or "" is taken off, and one day left
// alone is the due day. A list of people is the whole set the to-do is to
// carry: who stays keeps their place, who is new comes behind.
func (s *Server) updateTodo(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok1 := stringField(w, fields, "title")
	notes, ok2 := stringField(w, fields, "notes")
	startsOn, ok3 := stringField(w, fields, "startsOn")
	dueOn, ok4 := stringField(w, fields, "dueOn")
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return
	}
	assigneeIDs, ok := todoPeopleField(w, fields, "assigneeIds")
	if !ok {
		return
	}
	notifiedIDs, ok := todoPeopleField(w, fields, "notifiedIds")
	if !ok {
		return
	}
	_, namesStart := fields["startsOn"]
	_, namesDue := fields["dueOn"]
	if title == nil && notes == nil && !namesStart && !namesDue && assigneeIDs == nil && notifiedIDs == nil {
		unprocessable(w, "nothingToChange", "The body names neither a title nor notes nor days nor people.")
		return
	}
	if title != nil && !validTodoTitle(w, "todo", "a to-do", *title) {
		return
	}
	start, due := todo.StartsOn, todo.DueOn
	if namesStart || namesDue {
		wantedStart, wantedDue := deref(todo.StartsOn), deref(todo.DueOn)
		if namesStart {
			wantedStart = deref(startsOn)
		}
		if namesDue {
			wantedDue = deref(dueOn)
		}
		if start, due, ok = todoDates(w, wantedStart, wantedDue); !ok {
			return
		}
	}
	stored := todo.Notes
	if notes != nil {
		if stored, ok = storedTodoNotes(w, *notes); !ok {
			return
		}
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	if !s.allMembersBut(w, tool, assigneeIDs, todo.AssigneeIDs) || !s.allMembersBut(w, tool, notifiedIDs, todo.NotifiedIDs) {
		return
	}

	if title != nil {
		todo.Title = phpTrim(*title)
	}
	todo.Notes, todo.StartsOn, todo.DueOn = stored, start, due
	if assigneeIDs != nil {
		todo.AssigneeIDs = changedPeople(todo.AssigneeIDs, assigneeIDs)
	}
	if notifiedIDs != nil {
		todo.NotifiedIDs = changedPeople(todo.NotifiedIDs, notifiedIDs)
	}
	writeJSON(w, http.StatusOK, s.todoResource(tool, list, todo))
}

// completeTodo ticks a to-do off. One done already is answered like one
// ticked off now, unless the project or the list says why nothing may be
// written at all.
func (s *Server) completeTodo(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	if todo.CompletedAt == nil {
		at := s.momentAfterEveryCompletion(tool)
		todo.CompletedAt, todo.CompletedByID = &at, person.ID
	}
	writeJSON(w, http.StatusOK, s.todoResource(tool, list, todo))
}

// reopenTodo opens a done to-do again; one open already is no mistake either.
// Its subtasks stay as they are, as they did when it was ticked off.
func (s *Server) reopenTodo(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	todo.CompletedAt, todo.CompletedByID = nil, ""
	writeJSON(w, http.StatusOK, s.todoResource(tool, list, todo))
}

// moveTodo puts a to-do behind "previousTodoId", or where that is not named in
// front of "nextTodoId", or where neither is named at the top of its place,
// in "listId" of the same tool and in "groupId". A list left out is its own;
// a group left out is its own within the same list and none in another, and
// null is none. A neighbour has to stand in the list and the group the to-do
// goes to.
func (s *Server) moveTodo(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	listID, ok1 := todoPlaceField(w, fields, "listId")
	if !ok1 {
		return
	}
	groupID, ok2 := todoPlaceField(w, fields, "groupId")
	if !ok2 {
		return
	}
	previous, ok3 := todoPlaceField(w, fields, "previousTodoId")
	if !ok3 {
		return
	}
	next, ok4 := todoPlaceField(w, fields, "nextTodoId")
	if !ok4 {
		return
	}
	targetListID := list.ID
	if listID != nil {
		targetListID = *listID
	}
	targetGroupID := deref(groupID)
	if _, named := fields["groupId"]; !named && targetListID == list.ID {
		targetGroupID = todo.GroupID
	}
	for _, neighbour := range []*string{previous, next} {
		if neighbour != nil && (*neighbour == todo.ID || !s.isTodoIn(*neighbour, targetListID, targetGroupID)) {
			if !s.refusedAsNothingMoved(w, tool, list, targetListID) {
				todoPlaceUnknown(w)
			}
			return
		}
	}
	target := listOfTool(tool, targetListID)
	if s.refusedAsNothingMoved(w, tool, list, targetListID) {
		return
	}
	if target == nil || targetGroupID != "" && groupOf(target, targetGroupID) == nil {
		todoPlaceUnknown(w)
		return
	}

	list.Todos = slices.DeleteFunc(list.Todos, func(one *Todo) bool { return one == todo })
	at := slices.IndexFunc(target.Todos, func(one *Todo) bool { return one.GroupID == targetGroupID })
	switch {
	case previous != nil:
		at = slices.IndexFunc(target.Todos, func(one *Todo) bool { return one.ID == *previous }) + 1
	case next != nil:
		at = slices.IndexFunc(target.Todos, func(one *Todo) bool { return one.ID == *next })
	}
	todo.GroupID = targetGroupID
	target.Todos = slices.Insert(target.Todos, max(at, 0), todo)
	writeJSON(w, http.StatusOK, s.todoResource(tool, target, todo))
}

// removeTodo puts a to-do into the trash of its project.
func (s *Server) removeTodo(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	todo.Trashed = true
	w.WriteHeader(http.StatusNoContent)
}

// completedTodos reads the done to-dos of a list, in use or in the archive, the
// one done last first, a page at a time. "next" is where the page after this
// one starts and null on the last page, a page that holds the rest exactly
// included. The page is looked for before its size and its start are read.
func (s *Server) completedTodos(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list := s.visibleTodoList(r.PathValue("listId"), person)
	if list == nil {
		notFound(w)
		return
	}
	limit, ok := todosLimit(w, r.URL.Query().Get("limit"))
	if !ok {
		return
	}
	var boundary *completedCursor
	if before := r.URL.Query().Get("before"); before != "" {
		cursor, readable := readCompletedCursor(before)
		if !readable {
			unprocessable(w, "completedTodosBeforeUnknown", `The page starts where the page before ended, as its "next" names it.`)
			return
		}
		boundary = &cursor
	}

	var done []*Todo
	for _, todo := range countedTodos(list) {
		if todo.CompletedAt != nil && (boundary == nil || boundary.isAfter(todo)) {
			done = append(done, todo)
		}
	}
	slices.SortFunc(done, func(one, other *Todo) int {
		if order := other.CompletedAt.Compare(*one.CompletedAt); order != 0 {
			return order
		}
		return strings.Compare(other.ID, one.ID)
	})

	var next any
	if len(done) > limit {
		done = done[:limit]
		last := done[len(done)-1]
		next = completedCursor{at: *last.CompletedAt, todoID: last.ID}.String()
	}
	lines := []any{}
	for _, todo := range done {
		lines = append(lines, s.todoLineResource(tool, todo))
	}
	writeJSON(w, http.StatusOK, map[string]any{"todos": lines, "next": next})
}

// addSubtask puts a subtask at the end of a to-do. Its line, its person and its
// day are held before the archive is asked.
func (s *Server) addSubtask(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
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
	line, ok := subtaskLine(w, deref(title))
	if !ok {
		return
	}
	assigneeID, ok := subtaskPersonField(w, fields)
	if !ok {
		return
	}
	dueOn, ok := stringField(w, fields, "dueOn")
	if !ok {
		return
	}
	due, ok := subtaskDay(w, deref(dueOn))
	if !ok {
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) || assigneeID != "" && !s.allMembers(w, tool, []string{assigneeID}) {
		return
	}

	todo.Subtasks = append(todo.Subtasks, &Subtask{ID: newID(), Title: line, AssigneeID: assigneeID, DueOn: due})
	writeJSON(w, http.StatusCreated, s.todoResource(tool, list, todo))
}

// updateSubtask changes any of line, assignee, day and whether it is done.
// What the body leaves out stays, and null takes the assignee or the day off.
// The to-do is looked up before the body, the subtask only after it. Asking
// for what the subtask says already writes nothing, which an archived project
// or list answers the way it answers a write.
func (s *Server) updateSubtask(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
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
	completed, ok := booleanField(w, fields, "completed")
	if !ok {
		return
	}
	_, namesAssignee := fields["assigneeId"]
	_, namesDay := fields["dueOn"]
	changesLine := title != nil || namesAssignee || namesDay
	if !changesLine && completed == nil {
		unprocessable(w, "nothingToChange", "The body names neither a line nor an assignee nor a day nor whether the subtask is done.")
		return
	}
	var line string
	if title != nil {
		if line, ok = subtaskLine(w, *title); !ok {
			return
		}
	}
	assigneeID, ok := subtaskPersonField(w, fields)
	if !ok {
		return
	}
	dueOn, ok := stringField(w, fields, "dueOn")
	if !ok {
		return
	}
	due, ok := subtaskDay(w, deref(dueOn))
	if !ok {
		return
	}
	subtask := subtaskOf(todo, r.PathValue("subtaskId"))
	if subtask == nil {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	if namesAssignee && assigneeID != "" && !s.allMembers(w, tool, []string{assigneeID}) {
		return
	}

	if title != nil {
		subtask.Title = line
	}
	if namesAssignee {
		subtask.AssigneeID = assigneeID
	}
	if namesDay {
		subtask.DueOn = due
	}
	if completed != nil {
		subtask.Completed = *completed
	}
	writeJSON(w, http.StatusOK, s.todoResource(tool, list, todo))
}

// moveSubtask puts a subtask behind "previousSubtaskId", or in front of
// "nextSubtaskId", or at the top where neither is named. A neighbour has to be
// another subtask of the same to-do. The to-do and the subtask are looked up
// before the body.
func (s *Server) moveSubtask(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil {
		notFound(w)
		return
	}
	subtask := subtaskOf(todo, r.PathValue("subtaskId"))
	if subtask == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	previous, ok1 := todoPlaceField(w, fields, "previousSubtaskId")
	if !ok1 {
		return
	}
	next, ok2 := todoPlaceField(w, fields, "nextSubtaskId")
	if !ok2 {
		return
	}
	for _, neighbour := range []*string{previous, next} {
		if neighbour != nil && (*neighbour == subtask.ID || subtaskOf(todo, *neighbour) == nil) {
			if !s.refusedAsNothingWritten(w, tool, list) {
				todoPlaceUnknown(w)
			}
			return
		}
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}

	others := slices.DeleteFunc(slices.Clone(todo.Subtasks), func(one *Subtask) bool { return one == subtask })
	at := 0
	switch {
	case previous != nil:
		at = slices.IndexFunc(others, func(one *Subtask) bool { return one.ID == *previous }) + 1
	case next != nil:
		at = slices.IndexFunc(others, func(one *Subtask) bool { return one.ID == *next })
	}
	todo.Subtasks = slices.Insert(others, at, subtask)
	writeJSON(w, http.StatusOK, s.todoResource(tool, list, todo))
}

// removeSubtask takes a subtask off. One that is not there is what was asked
// for, unless nothing may be written at all; an address that names no subtask
// at all is 404.
func (s *Server) removeSubtask(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
	if todo == nil || !uuidPattern.MatchString(r.PathValue("subtaskId")) {
		notFound(w)
		return
	}
	if s.refusedAsNothingWritten(w, tool, list) {
		return
	}
	todo.Subtasks = slices.DeleteFunc(todo.Subtasks, func(one *Subtask) bool { return one.ID == r.PathValue("subtaskId") })
	w.WriteHeader(http.StatusNoContent)
}

// myTodos lists the open to-dos the person is assigned to or holds an open
// subtask of, across the projects in use, ordered by project, due day with
// those without one last, and title; a page at a time, the next one "after"
// the last to-do of this one, compared as that to-do stands now. Nothing in
// the answer says whether a page is the last.
func (s *Server) myTodos(w http.ResponseWriter, r *http.Request, person *Person) {
	limit, ok := todosLimit(w, r.URL.Query().Get("limit"))
	if !ok {
		return
	}
	var boundary *assignedTodo
	if after := strings.ToLower(r.URL.Query().Get("after")); after != "" {
		boundary = s.todoOfAProjectOf(after, person)
		if boundary == nil {
			unprocessable(w, "todosAfterUnknown", "The page starts after a to-do of one of your projects, named by its id.")
			return
		}
	}

	var mine []assignedTodo
	for _, tool := range s.readableTodosTools() {
		project := s.visibleProject(tool.ProjectID, person)
		if project == nil || project.Archived {
			continue
		}
		for _, list := range tool.Lists {
			if list.Trashed || list.Archived || !s.showsTheList(list, person) {
				continue
			}
			for _, todo := range countedTodos(list) {
				one := assignedTodo{project, tool, list, todo}
				if todo.CompletedAt == nil && (slices.Contains(todo.AssigneeIDs, person.ID) || len(openSubtasksOf(todo, person.ID)) > 0) &&
					(boundary == nil || compareAssigned(one, *boundary) > 0) {
					mine = append(mine, one)
				}
			}
		}
	}
	slices.SortFunc(mine, compareAssigned)
	if len(mine) > limit {
		mine = mine[:limit]
	}

	todos := []any{}
	for _, one := range mine {
		subtasks := []any{}
		for _, subtask := range openSubtasksOf(one.todo, person.ID) {
			subtasks = append(subtasks, s.subtaskResource(one.tool, subtask))
		}
		todos = append(todos, map[string]any{
			"id": one.todo.ID, "title": one.todo.Title,
			"project": map[string]any{"id": one.project.ID, "name": one.project.Name},
			"tool":    map[string]any{"id": one.tool.ID, "title": one.tool.Title},
			"list":    map[string]any{"id": one.list.ID, "title": one.list.Title},
			"dueOn":   one.todo.DueOn, "isOverdue": s.isOverdue(one.todo.DueOn, false),
			"isAssigned": slices.Contains(one.todo.AssigneeIDs, person.ID),
			"assignees":  s.peopleOnTodo(one.tool, one.todo.AssigneeIDs), "subtasks": subtasks,
			"commentCount": len(one.todo.Comments), "appUrl": s.todoURL(one.tool, one.todo),
		})
	}
	writeJSON(w, http.StatusOK, todos)
}

// assignedTodo is a to-do with where it stands, the way the page of somebody's
// tasks orders it.
type assignedTodo struct {
	project *Project
	tool    *TodosTool
	list    *TodoList
	todo    *Todo
}

func compareAssigned(one, other assignedTodo) int {
	return cmp.Or(
		strings.Compare(one.project.Name, other.project.Name),
		strings.Compare(one.project.ID, other.project.ID),
		strings.Compare(dayOrLast(one.todo.DueOn), dayOrLast(other.todo.DueOn)),
		strings.Compare(one.todo.Title, other.todo.Title),
		strings.Compare(one.todo.ID, other.todo.ID),
	)
}

// todoOfAProjectOf finds a to-do a page of somebody's to-dos may start after:
// one of a project they are a member of, done, in the trash or not.
func (s *Server) todoOfAProjectOf(todoID string, person *Person) *assignedTodo {
	if !uuidPattern.MatchString(todoID) {
		return nil
	}
	for _, tool := range s.TodosTools {
		project := s.visibleProject(tool.ProjectID, person)
		if project == nil {
			continue
		}
		for _, list := range tool.Lists {
			for _, todo := range list.Todos {
				if todo.ID == todoID {
					return &assignedTodo{project, tool, list, todo}
				}
			}
		}
	}
	return nil
}

func openSubtasksOf(todo *Todo, personID string) []*Subtask {
	var open []*Subtask
	for _, subtask := range todo.Subtasks {
		if !subtask.Completed && subtask.AssigneeID == personID {
			open = append(open, subtask)
		}
	}
	return open
}

// completedCursor is where a page of done to-dos ends: the moment the last one
// was ticked off and its identifier. The next page starts behind these values
// and not behind the to-do, so opening it again in between moves nothing.
type completedCursor struct {
	at     time.Time
	todoID string
}

func (c completedCursor) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.at.UTC().Format(completedCursorLayout) + "|" + c.todoID))
}

// isAfter says whether a done to-do comes on a page that starts behind the
// cursor.
func (c completedCursor) isAfter(todo *Todo) bool {
	if order := todo.CompletedAt.Compare(c.at); order != 0 {
		return order < 0
	}
	return todo.ID < c.todoID
}

func readCompletedCursor(value string) (completedCursor, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
	if err != nil {
		return completedCursor{}, false
	}
	moment, todoID, found := strings.Cut(string(decoded), "|")
	if !found || strings.Contains(todoID, "|") || !uuidPattern.MatchString(todoID) {
		return completedCursor{}, false
	}
	at, err := time.Parse(completedCursorLayout, moment)
	if err != nil || at.Format(completedCursorLayout) != moment {
		return completedCursor{}, false
	}
	return completedCursor{at: at, todoID: todoID}, true
}

// todosLimit reads the size of a page of to-dos. Left out it is the default,
// above the largest page it is cut down to that without a word, and anything
// but a whole number of at least one is refused.
func todosLimit(w http.ResponseWriter, value string) (int, bool) {
	if value == "" {
		return todosOnAPage, true
	}
	limit, err := strconv.Atoi(value)
	if !aWholeNumber.MatchString(value) || err != nil || limit < 1 {
		unprocessable(w, "todosLimitNotANumber", "The number of to-dos asked for has to be a whole number of at least one.")
		return 0, false
	}
	return min(limit, mostTodosOnAPage), true
}

// momentAfterEveryCompletion is when a to-do is ticked off: after every to-do
// of the tool that was ticked off before it, so that the done to-dos have an
// order to read even while the clock of the fake stands still.
func (s *Server) momentAfterEveryCompletion(tool *TodosTool) time.Time {
	moment := s.now
	for _, list := range tool.Lists {
		for _, todo := range list.Todos {
			if todo.CompletedAt != nil && !todo.CompletedAt.Before(moment) {
				moment = todo.CompletedAt.Add(time.Second)
			}
		}
	}
	return moment
}

// refusedAsNothingMoved says why a move wrote nothing: what holds for the list
// the to-do comes from, or for the list of the same tool it was to go to.
func (s *Server) refusedAsNothingMoved(w http.ResponseWriter, tool *TodosTool, from *TodoList, toListID string) bool {
	if s.refusedAsNothingWritten(w, tool, from) {
		return true
	}
	if to := listOfTool(tool, toListID); to != nil && to.Archived {
		unprocessable(w, "todoListArchived", "This list is archived. Nothing in it changes until it is back on the page of its tool.")
		return true
	}
	return false
}

// isTodoIn says whether a to-do out of the trash stands in this list and this
// group, which is what a neighbour of a move has to do. The list is looked for
// in every tool, as the server does.
func (s *Server) isTodoIn(todoID, listID, groupID string) bool {
	for _, tool := range s.TodosTools {
		for _, list := range tool.Lists {
			if list.ID != listID {
				continue
			}
			for _, todo := range list.Todos {
				if todo.ID == todoID && todo.GroupID == groupID && !todo.Trashed {
					return true
				}
			}
		}
	}
	return false
}

// visibleTodo finds a to-do out of the trash, in a list in use or in the
// archive and in no group in the trash, of a project the person is a member of.
func (s *Server) visibleTodo(todoID string, person *Person) (*TodosTool, *TodoList, *Todo) {
	if !uuidPattern.MatchString(todoID) {
		return nil, nil, nil
	}
	for _, tool := range s.readableTodosTools() {
		for _, list := range tool.Lists {
			for _, todo := range list.Todos {
				if todo.ID != todoID {
					continue
				}
				if todo.Trashed || list.Trashed || todo.GroupID != "" && groupOf(list, todo.GroupID) == nil || s.visibleProject(tool.ProjectID, person) == nil ||
					!s.showsTheList(list, person) {
					return nil, nil, nil
				}
				return tool, list, todo
			}
		}
	}
	return nil, nil, nil
}

// listOfTool is a list of the tool out of the trash, in use or in the archive.
func listOfTool(tool *TodosTool, listID string) *TodoList {
	for _, list := range tool.Lists {
		if list.ID == listID && !list.Trashed {
			return list
		}
	}
	return nil
}

func subtaskOf(todo *Todo, subtaskID string) *Subtask {
	for _, subtask := range todo.Subtasks {
		if subtask.ID == subtaskID {
			return subtask
		}
	}
	return nil
}

// todoPeopleField reads a list of people of a to-do: nil where the body leaves
// it out or sets it to null, and an identifier that is none refused the way a
// stranger is.
func todoPeopleField(w http.ResponseWriter, fields map[string]any, name string) ([]string, bool) {
	value, present := fields[name]
	if !present || value == nil {
		return nil, true
	}
	list, ok := value.([]any)
	if !ok {
		malformed(w, `The field "`+name+`" has to be a list of strings.`)
		return nil, false
	}
	ids := make([]string, 0, len(list))
	for _, entry := range list {
		id, ok := entry.(string)
		if !ok {
			malformed(w, `The field "`+name+`" has to be a list of strings.`)
			return nil, false
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		if !uuidPattern.MatchString(id) {
			todoPersonNotAMember(w)
			return nil, false
		}
	}
	return ids, true
}

// subtaskPersonField reads the assignee of a subtask: empty for null, for ""
// and for a body that leaves it out.
func subtaskPersonField(w http.ResponseWriter, fields map[string]any) (string, bool) {
	id, ok := stringField(w, fields, "assigneeId")
	if !ok {
		return "", false
	}
	if deref(id) != "" && !uuidPattern.MatchString(*id) {
		todoPersonNotAMember(w)
		return "", false
	}
	return deref(id), true
}

// todoPlaceField reads the identifier of a list, a group or a neighbour. One
// that is no identifier names nothing that could be there.
func todoPlaceField(w http.ResponseWriter, fields map[string]any, name string) (*string, bool) {
	id, ok := stringField(w, fields, name)
	if !ok {
		return nil, false
	}
	if id != nil && !uuidPattern.MatchString(*id) {
		todoPlaceUnknown(w)
		return nil, false
	}
	return id, true
}

// allMembers refuses the write where somebody named is no member of the
// project of the tool.
func (s *Server) allMembers(w http.ResponseWriter, tool *TodosTool, ids []string) bool {
	return s.allMembersBut(w, tool, ids, nil)
}

// allMembersBut refuses the write where somebody named is no member of the
// project, except those the to-do carries already, who stay as they are.
func (s *Server) allMembersBut(w http.ResponseWriter, tool *TodosTool, ids, carried []string) bool {
	project := s.projectWithID(tool.ProjectID)
	for _, id := range ids {
		if !slices.Contains(carried, id) && !slices.Contains(project.MemberIDs, id) {
			todoPersonNotAMember(w)
			return false
		}
	}
	return true
}

func todoPersonNotAMember(w http.ResponseWriter) {
	unprocessable(w, "personNotAProjectMember", "Only a member of the project can be named on a to-do or a subtask.")
}

// changedPeople is the set of people a to-do carries after a change to the
// wanted set: who stays keeps their place, who is new comes behind in the
// order named.
func changedPeople(carried, wanted []string) []string {
	kept := slices.DeleteFunc(slices.Clone(carried), func(id string) bool { return !slices.Contains(wanted, id) })
	for _, id := range wanted {
		if !slices.Contains(kept, id) {
			kept = append(kept, id)
		}
	}
	return kept
}

func withoutRepeats(ids []string) []string {
	return changedPeople(nil, ids)
}

// todoDates holds the days of a to-do against what the server keeps: each one
// a day, the start not after the due day, and one day alone the due day,
// whichever field it was written in. A start on the due day is no start.
func todoDates(w http.ResponseWriter, startsOn, dueOn string) (*string, *string, bool) {
	start, due := phpTrim(startsOn), phpTrim(dueOn)
	if !isADay(start) || !isADay(due) {
		unprocessable(w, "todoDatesNotADate", "A day is written as YYYY-MM-DD.")
		return nil, nil, false
	}
	if start != "" && due != "" && start > due {
		unprocessable(w, "todoDatesOutOfOrder", "A to-do cannot start after it is due.")
		return nil, nil, false
	}
	if due == "" || start == due {
		return nil, dueDate(&start), true
	}
	return dueDate(&start), dueDate(&due), true
}

func subtaskDay(w http.ResponseWriter, value string) (*string, bool) {
	if !isADay(phpTrim(value)) {
		unprocessable(w, "subtaskDueNotADate", "The day of a subtask is written as YYYY-MM-DD.")
		return nil, false
	}
	return dueDate(&value), true
}

// isADay accepts no day at all, or a day that exists written as YYYY-MM-DD.
func isADay(value string) bool {
	if value == "" {
		return true
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	day, err := time.Parse("2006-01-02", value)
	return err == nil && day.Format("2006-01-02") == value
}

// subtaskLine holds the line of a subtask as the server holds it: the length
// first, then control characters, then whether anything is left.
func subtaskLine(w http.ResponseWriter, title string) (string, bool) {
	line := phpTrim(title)
	switch {
	case utf8.RuneCountInString(line) > maxSubtaskLength:
		unprocessable(w, "subtaskTooLong", "The line of a subtask is too long.")
	case strings.IndexFunc(line, unicode.IsControl) >= 0:
		unprocessable(w, "subtaskHasControlCharacters", "The line of a subtask must not contain control characters.")
	case line == "":
		unprocessable(w, "subtaskMissing", "A subtask needs a line of text.")
	default:
		return line, true
	}
	return "", false
}

// storedTodoNotes are notes as the server keeps them, held against their
// length as sent and again as they would be stored.
func storedTodoNotes(w http.ResponseWriter, value string) (string, bool) {
	const tooLong, message = "todoNotesTooLong", "The notes of a to-do are too long."
	stored, ok := storedRichText(w, tooLong, message, maxTodoNotesLength, value)
	if !ok {
		return "", false
	}
	if utf8.RuneCountInString(stored) > maxTodoNotesLength {
		unprocessable(w, tooLong, message)
		return "", false
	}
	return stored, true
}

// isOverdue says whether a day lies before the fake's today. Something done
// is never overdue.
func (s *Server) isOverdue(dueOn *string, completed bool) bool {
	return !completed && dueOn != nil && *dueOn < s.now.UTC().Format("2006-01-02")
}

// peopleOnTodo names the people of a to-do who are members of its project:
// somebody who left is still on it and shows nowhere.
func (s *Server) peopleOnTodo(tool *TodosTool, ids []string) []any {
	project := s.projectWithID(tool.ProjectID)
	people := []any{}
	for _, id := range ids {
		if slices.Contains(project.MemberIDs, id) {
			people = append(people, s.personReference(id))
		}
	}
	return people
}

func (s *Server) subtaskResource(tool *TodosTool, subtask *Subtask) map[string]any {
	var assignee any
	if subtask.AssigneeID != "" && slices.Contains(s.projectWithID(tool.ProjectID).MemberIDs, subtask.AssigneeID) {
		assignee = s.personReference(subtask.AssigneeID)
	}
	return map[string]any{
		"id": subtask.ID, "title": subtask.Title, "completed": subtask.Completed,
		"assignee": assignee, "dueOn": subtask.DueOn, "isOverdue": s.isOverdue(subtask.DueOn, subtask.Completed),
	}
}

func (s *Server) todoResource(tool *TodosTool, list *TodoList, todo *Todo) map[string]any {
	var group any
	if todo.GroupID != "" {
		group = map[string]any{"id": todo.GroupID, "title": groupOf(list, todo.GroupID).Title}
	}
	var completedBy any
	if todo.CompletedAt != nil && todo.CompletedByID != "" {
		completedBy = s.People[todo.CompletedByID].Name
	}
	subtasks := []any{}
	for _, subtask := range todo.Subtasks {
		subtasks = append(subtasks, s.subtaskResource(tool, subtask))
	}
	comments := []any{}
	for _, comment := range todo.Comments {
		comments = append(comments, s.commentResource(tool.ProjectID, comment))
	}
	return map[string]any{
		"id": todo.ID, "projectId": tool.ProjectID, "toolId": tool.ID, "listId": list.ID, "groupId": nullable(todo.GroupID),
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"list": map[string]any{"id": list.ID, "title": list.Title}, "group": group,
		"title": todo.Title, "completed": todo.CompletedAt != nil, "completedAt": timestampOrNil(todo.CompletedAt),
		"completedByName": completedBy,
		"notes":           todo.Notes, "notesSource": todo.Notes, "attachments": s.attachmentsIn(tool.ProjectID, todo.Notes),
		"startsOn": todo.StartsOn, "dueOn": todo.DueOn, "isOverdue": s.isOverdue(todo.DueOn, todo.CompletedAt != nil),
		"series":    seriesReference(todo),
		"assignees": s.peopleOnTodo(tool, todo.AssigneeIDs), "notified": s.peopleOnTodo(tool, todo.NotifiedIDs),
		"subtasks": subtasks, "listArchived": list.Archived,
		"commentCount": len(todo.Comments), "createdAt": timestamp(todo.CreatedAt), "creator": s.personReference(todo.CreatorID),
		"comments": comments, "appUrl": s.todoURL(tool, todo),
	}
}

// seriesReference names the series a to-do is an occurrence of and the day it
// stands for, and is null for a to-do that is none.
func seriesReference(todo *Todo) any {
	if todo.SeriesID == "" {
		return nil
	}
	return map[string]any{"id": todo.SeriesID, "occursOn": todo.OccursOn}
}

func (s *Server) todoURL(tool *TodosTool, todo *Todo) string {
	return s.todosToolURL(tool) + "/todos/" + todo.ID
}
