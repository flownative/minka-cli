package fakeapi

import (
	"net/http"
	"slices"
	"strings"
)

func (s *Server) registerTemplateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/templates/todo-lists", s.authenticated(s.todoListTemplates))
}

// todoListTemplates answers the templates in use of the library the person
// is a member of. Somebody in none, a client above all, is told 404.
func (s *Server) todoListTemplates(w http.ResponseWriter, _ *http.Request, person *Person) {
	tool := s.templateLibraryOf(person)
	if tool == nil {
		notFound(w)
		return
	}
	templates := []any{}
	for _, list := range tool.Lists {
		if !list.Trashed && !list.Archived {
			templates = append(templates, s.todoListItemResource(tool, list))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projectId": tool.ProjectID, "toolId": tool.ID, "templates": templates})
}

// templateLibraryOf is the To-dos tool of the library the person is a member
// of, nil for somebody in none.
func (s *Server) templateLibraryOf(person *Person) *TodosTool {
	for _, tool := range s.readableTodosTools() {
		if tool.Archived {
			continue
		}
		if project := s.visibleProject(tool.ProjectID, person); project != nil && project.TemplateLibrary {
			return tool
		}
	}
	return nil
}

// isTheTemplateLibrary says whether the tool stands in the library, where
// nothing takes a date or a tick.
func (s *Server) isTheTemplateLibrary(tool *TodosTool) bool {
	project := s.projectWithID(tool.ProjectID)
	return project != nil && project.TemplateLibrary
}

// refusedAsDatedInTheLibrary refuses a day for a to-do or a subtask of a
// template; taking one off is no refusal.
func (s *Server) refusedAsDatedInTheLibrary(w http.ResponseWriter, tool *TodosTool, days ...*string) bool {
	if !s.isTheTemplateLibrary(tool) || !slices.ContainsFunc(days, func(day *string) bool { return deref(day) != "" }) {
		return false
	}
	unprocessable(w, "templateTakesNoDates", "A template takes no dates; they are set on the list made from it.")
	return true
}

func (s *Server) refusedAsTickedInTheLibrary(w http.ResponseWriter, tool *TodosTool) bool {
	if !s.isTheTemplateLibrary(tool) {
		return false
	}
	unprocessable(w, "templateTakesNoTicks", "Nothing in a template is ticked off; that happens on the list made from it.")
	return true
}

// addTodoListFromTemplate makes a fresh copy of a template on top of the tool,
// in the order of the controller: the library, the tool, the template, the
// title, and only after a refused write the project and the template once more.
func (s *Server) addTodoListFromTemplate(w http.ResponseWriter, tool *TodosTool, templateID string, title *string, person *Person) {
	library := s.templateLibraryOf(person)
	if library == nil {
		templateUnknown(w)
		return
	}
	if tool == library {
		unprocessable(w, "templateIntoLibrary", "A list is made from a template in a project; within the templates, duplicate it on the web.")
		return
	}
	templateID = strings.ToLower(templateID)
	if !uuidPattern.MatchString(templateID) {
		templateUnknown(w)
		return
	}
	template := listOfTool(library, templateID)
	if template == nil {
		templateUnknown(w)
		return
	}
	if title == nil {
		if template.Archived {
			templateArchived(w)
			return
		}
		title = &template.Title
	}
	if !validTitle(w, "todoList", "a list", *title) {
		return
	}
	project := s.projectWithID(tool.ProjectID)
	if project == nil || project.Archived || template.Archived || tool.Archived {
		switch {
		case s.refusedAsArchived(w, project):
		case template.Archived:
			templateArchived(w)
		default:
			notFound(w)
		}
		return
	}

	leftOff := s.peopleLeftOff(library, template, project)
	list := s.freshCopy(template, project, phpTrim(*title), person)
	tool.Lists = append([]*TodoList{list}, tool.Lists...)

	resource := s.todoListResource(tool, list)
	resource["leftOff"] = leftOff
	w.Header().Set("Location", s.URL+"/api/v1/todo-lists/"+list.ID)
	writeJSON(w, http.StatusCreated, resource)
}

// freshCopy is a template as a new list of the project: its groups and its
// to-dos out of the trash, open and without a day, carrying only the people
// who are members there. Comments and series stay with the template, and the
// clients of the project are not shown the new list.
func (s *Server) freshCopy(template *TodoList, project *Project, title string, person *Person) *TodoList {
	isAMember := func(id string) bool { return slices.Contains(project.MemberIDs, id) }
	members := func(ids []string) []string {
		var kept []string
		for _, id := range ids {
			if isAMember(id) {
				kept = append(kept, id)
			}
		}
		return kept
	}

	list := &TodoList{ID: newID(), Title: title, Description: template.Description, CreatorID: person.ID, HiddenFromClients: true}
	groupCopies := map[string]string{}
	for _, group := range template.Groups {
		if group.Trashed {
			continue
		}
		groupCopy := &TodoGroup{ID: newID(), Title: group.Title, Color: group.Color}
		groupCopies[group.ID] = groupCopy.ID
		list.Groups = append(list.Groups, groupCopy)
	}
	for _, todo := range countedTodos(template) {
		todoCopy := &Todo{ID: newID(), Title: todo.Title, GroupID: groupCopies[todo.GroupID], Notes: todo.Notes,
			AssigneeIDs: members(todo.AssigneeIDs), NotifiedIDs: members(todo.NotifiedIDs), CreatedAt: s.now, CreatorID: person.ID}
		for _, subtask := range todo.Subtasks {
			subtaskCopy := &Subtask{ID: newID(), Title: subtask.Title}
			if isAMember(subtask.AssigneeID) {
				subtaskCopy.AssigneeID = subtask.AssigneeID
			}
			todoCopy.Subtasks = append(todoCopy.Subtasks, subtaskCopy)
		}
		list.Todos = append(list.Todos, todoCopy)
	}
	return list
}

// peopleLeftOff names those on the to-dos and subtasks a copy takes along who
// are no members of the project, as far as they are on the team of the
// library: others are left off without a word. The order is the name's.
func (s *Server) peopleLeftOff(library *TodosTool, template *TodoList, project *Project) []any {
	team := s.projectWithID(library.ProjectID)
	var ids []string
	for _, todo := range countedTodos(template) {
		ids = append(ids, todo.AssigneeIDs...)
		ids = append(ids, todo.NotifiedIDs...)
		for _, subtask := range todo.Subtasks {
			if subtask.AssigneeID != "" {
				ids = append(ids, subtask.AssigneeID)
			}
		}
	}
	var left []*Person
	for _, id := range withoutRepeats(ids) {
		if person := s.People[id]; person != nil && !slices.Contains(project.MemberIDs, id) && slices.Contains(team.MemberIDs, id) {
			left = append(left, person)
		}
	}
	slices.SortFunc(left, func(a, b *Person) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	names := []any{}
	for _, person := range left {
		names = append(names, map[string]any{"id": person.ID, "name": person.Name})
	}
	return names
}

func templateUnknown(w http.ResponseWriter) {
	unprocessable(w, "templateUnknown", "There is no such template among those you see.")
}

func templateArchived(w http.ResponseWriter) {
	unprocessable(w, "templateArchived", "This template is archived. A list is made only from a template in use.")
}
