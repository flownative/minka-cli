package fakeapi

import (
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ForwardsTool is a tool of mails forwarded into a project, which only an
// import brings.
type ForwardsTool struct {
	ID        string
	ProjectID string
	Title     string
}

// TrashedTool is a tool taken out of its project, which only the web brings
// back.
type TrashedTool struct {
	ID        string
	ProjectID string
	Type      string
	Title     string
	TrashedAt time.Time
	TrashedBy string // person ID
	seq       int
}

// maxToolTitleLength counts characters, as Minka does.
const maxToolTitleLength = 200

// addableToolTypes are the types the project page offers a button for, in the
// order Minka names them; forwards, which only an import brings, is none.
var addableToolTypes = []string{"board", "chat", "posts", "todos", "checkins", "calendar", "decisions", "files"}

// projectTool is a tool of any type as the list of a project's tools sees it.
type projectTool struct {
	id, projectID, toolType, appURL string
	title                           *string
	inUse                           bool
	shownToClients                  bool
	trash                           func()
}

// createProject starts a project as the web does: the creator joins it, and an
// agent brings the person it acts for along. A client of the organization,
// and an agent working for one, starts none.
func (s *Server) createProject(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	// Each field is read and judged before the next one is read, so a later
	// field that is no text does not hide what is wrong with an earlier one.
	name, ok := stringField(w, fields, "name")
	if !ok || !validProjectName(w, valueOr(name, "")) {
		return
	}
	description, ok := stringField(w, fields, "description")
	if !ok {
		return
	}
	if utf8.RuneCountInString(phpTrim(valueOr(description, ""))) > maxProjectDescriptionLength {
		unprocessable(w, "projectDescriptionTooLong", "The description of a project is too long.")
		return
	}
	category, ok := stringField(w, fields, "category")
	if !ok {
		return
	}
	filedUnder := ""
	if category != nil && strings.TrimSpace(*category) != "" {
		index := slices.IndexFunc(s.ProjectCategories, func(one string) bool {
			return strings.EqualFold(one, strings.TrimSpace(*category))
		})
		if index < 0 {
			unprocessable(w, "projectCategoryUnknown", "The organization files no projects under a category of this name.")
			return
		}
		filedUnder = s.ProjectCategories[index]
	}
	if s.isAClient(person.ID) {
		writeJSON(w, http.StatusForbidden, errorBody("organizationTeamOnly", "Only somebody on the team of the organization starts a project, and an agent only together with the person it acts for."))
		return
	}

	members := []string{person.ID}
	if person.PrincipalID != "" {
		members = append(members, person.PrincipalID)
	}
	project := &Project{
		ID: newID(), Name: phpTrim(*name), Description: phpTrim(valueOr(description, "")),
		MemberIDs: members, Category: filedUnder, WithoutClientAccess: true,
	}
	s.Projects = append(s.Projects, project)
	writeJSON(w, http.StatusCreated, s.projectResource(project, person))
}

func valueOr(value *string, otherwise string) string {
	if value == nil {
		return otherwise
	}
	return *value
}

// addProjectMember takes somebody of the organization into a project. Who is
// in already is answered with 200, also in an archived project; what keeps
// anybody else out is told in the order of Minka's handler.
func (s *Server) addProjectMember(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	personID, ok := stringField(w, fields, "personId")
	if !ok {
		return
	}
	if personID == nil {
		unprocessable(w, "personIdMissing", "Adding somebody needs the person to add.")
		return
	}
	added := s.People[*personID]
	if added != nil && !added.Retired && slices.Contains(project.MemberIDs, added.ID) {
		writeJSON(w, http.StatusOK, s.memberResource(project.ID, added))
		return
	}
	switch {
	case !uuidPattern.MatchString(*personID) || added == nil:
		unprocessable(w, "personNotAnOrganizationMember", "Only somebody who belongs to the organization joins one of its projects.")
	case s.refusedAsArchived(w, project):
	case added.Retired:
		unprocessable(w, "agentRetired", "An agent that has left service joins no project.")
	case added.Client && project.WithoutClientAccess:
		unprocessable(w, "projectWithoutClientAccess", "A client joins only a project that works with clients.")
	case added.PrincipalID != "" && !slices.Contains(project.MemberIDs, added.PrincipalID):
		unprocessable(w, "principalNotAProjectMember", "An agent joins only a project the person it acts for is in.")
	default:
		project.MemberIDs = append(project.MemberIDs, added.ID)
		writeJSON(w, http.StatusCreated, s.memberResource(project.ID, added))
	}
}

// removeProjectMember takes somebody out of a project, and the agents acting
// for them with them. Anybody may leave, also an archived project and also a
// client; taking out somebody else is the team's, and the last human of the
// team stays.
func (s *Server) removeProjectMember(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	removedID := r.PathValue("personId")
	if project == nil || !slices.Contains(project.MemberIDs, removedID) {
		notFound(w)
		return
	}
	leaving := removedID == person.ID
	if !leaving && s.refusedAsArchived(w, project) {
		return
	}
	if !leaving && s.isAClient(person.ID) {
		notFound(w)
		return
	}
	goes := func(id string) bool {
		return id == removedID || s.People[id] != nil && s.People[id].PrincipalID == removedID
	}
	if !slices.ContainsFunc(project.MemberIDs, func(id string) bool { return !goes(id) && s.isOnTheHumanTeam(id) }) {
		unprocessable(w, "lastProjectMember", "A project keeps at least one other human on its team.")
		return
	}
	project.MemberIDs = slices.DeleteFunc(project.MemberIDs, goes)
	w.WriteHeader(http.StatusNoContent)
}

// isOnTheHumanTeam is a human who works on the team, not as a client.
func (s *Server) isOnTheHumanTeam(personID string) bool {
	member := s.People[personID]
	return member != nil && member.PrincipalID == "" && !member.Client
}

// projectTools lists the tools of a project in the order of its page; a
// client sees those shown to clients.
func (s *Server) projectTools(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	list := []any{}
	for i, tool := range s.visibleTools(project, person) {
		list = append(list, toolOfAProjectResource(tool, i+1))
	}
	writeJSON(w, http.StatusOK, list)
}

// addProjectTool puts a new tool at the end of the project page, filled as
// the web fills it.
func (s *Server) addProjectTool(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.projectOfTheTeam(r, person)
	if project == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	toolType, ok := stringField(w, fields, "type")
	if !ok {
		return
	}
	if !slices.Contains(addableToolTypes, valueOr(toolType, "")) {
		unprocessable(w, "toolTypeUnknown", `A tool is one of the types "board", "chat", "posts", "todos", "checkins", "calendar", "decisions", "files".`)
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok || !validToolTitle(w, valueOr(title, "")) || s.refusedAsArchived(w, project) {
		return
	}

	s.placeTools(project, s.orderedTools(project))
	id := s.addToolOfType(project.ID, *toolType, phpTrim(*title))
	project.ToolOrder = append(project.ToolOrder, id)
	s.answerWithTool(w, http.StatusCreated, project, person, id)
}

// addToolOfType makes a tool as the web makes it: a board with its system
// columns, a Decisions tool with a status per role.
func (s *Server) addToolOfType(projectID, toolType, title string) string {
	id := newID()
	switch toolType {
	case "board":
		s.Boards = append(s.Boards, &Board{ID: id, ProjectID: projectID, Title: title, Columns: []*Column{
			{ID: newID(), Type: "triage"}, {ID: newID(), Type: "not_now"}, {ID: newID(), Type: "done"},
		}})
	case "chat":
		s.Chats = append(s.Chats, &Chat{ID: id, ProjectID: projectID, Name: title, ReadMarks: map[string]int{}})
	case "posts":
		s.PostsTools = append(s.PostsTools, &PostsTool{ID: id, ProjectID: projectID, Title: title})
	case "todos":
		s.TodosTools = append(s.TodosTools, &TodosTool{ID: id, ProjectID: projectID, Title: title})
	case "checkins":
		s.CheckinsTools = append(s.CheckinsTools, &CheckinsTool{ID: id, ProjectID: projectID, Title: title})
	case "calendar":
		s.Calendars = append(s.Calendars, &CalendarTool{ID: id, ProjectID: projectID, Title: title})
	case "decisions":
		tool := NewDecisionsTool(projectID, title)
		tool.ID, tool.ShownToClients = id, false
		s.DecisionsTools = append(s.DecisionsTools, tool)
	case "files":
		s.FilesTools = append(s.FilesTools, &FilesTool{ID: id, ProjectID: projectID, Title: title})
	}
	return id
}

// renameProjectTool renames a tool of any type, forwards included.
func (s *Server) renameProjectTool(w http.ResponseWriter, r *http.Request, person *Person) {
	project, tool := s.toolOfTheTeam(r, person)
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
	if title == nil {
		unprocessable(w, "nothingToChange", `The body names no "title".`)
		return
	}
	if !validToolTitle(w, *title) || s.refusedAsArchived(w, project) {
		return
	}
	*tool.title = phpTrim(*title)
	s.answerWithTool(w, http.StatusOK, project, person, tool.id)
}

// moveProjectTool puts a tool right after the previous one named, else right
// before the next one, else in front. With both, the next one is only
// checked.
func (s *Server) moveProjectTool(w http.ResponseWriter, r *http.Request, person *Person) {
	project, tool := s.toolOfTheTeam(r, person)
	if tool == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	visible := s.visibleTools(project, person)
	var neighbours [2]string
	for i, name := range []string{"previousToolId", "nextToolId"} {
		value, ok := stringField(w, fields, name)
		if !ok {
			return
		}
		if value == nil || *value == "" {
			continue
		}
		if *value == tool.id || !slices.ContainsFunc(visible, func(one *projectTool) bool { return one.id == *value }) {
			unprocessable(w, "toolPlaceUnknown", "A neighbour of a move is another tool of the same project.")
			return
		}
		neighbours[i] = *value
	}
	if s.refusedAsArchived(w, project) {
		return
	}

	order := slices.DeleteFunc(s.orderedTools(project), func(one *projectTool) bool { return one.id == tool.id })
	at := 0
	switch previous, next := neighbours[0], neighbours[1]; {
	case previous != "":
		at = slices.IndexFunc(order, func(one *projectTool) bool { return one.id == previous }) + 1
	case next != "":
		at = slices.IndexFunc(order, func(one *projectTool) bool { return one.id == next })
	}
	s.placeTools(project, slices.Insert(order, at, tool))
	s.answerWithTool(w, http.StatusOK, project, person, tool.id)
}

// trashProjectTool puts a tool in the trash of its project, from where only
// the web brings it back.
func (s *Server) trashProjectTool(w http.ResponseWriter, r *http.Request, person *Person) {
	project, tool := s.toolOfTheTeam(r, person)
	if tool == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, project) {
		return
	}
	tool.trash()
	s.trashSeq++
	s.TrashedTools = append(s.TrashedTools, &TrashedTool{
		ID: tool.id, ProjectID: project.ID, Type: tool.toolType, Title: *tool.title,
		TrashedAt: s.now, TrashedBy: person.ID, seq: s.trashSeq,
	})
	w.WriteHeader(http.StatusNoContent)
}

// trashedToolEntries are the rows of the trash for the tools of a project.
func (s *Server) trashedToolEntries(project *Project) []trashRow {
	var rows []trashRow
	for _, trashed := range s.TrashedTools {
		if trashed.ProjectID != project.ID {
			continue
		}
		rows = append(rows, trashRow{trashed.TrashedAt, trashed.seq, map[string]any{
			"kind": "tool", "id": trashed.ID, "title": trashed.Title, "toolType": trashed.Type,
			"board": nil, "column": nil,
			"trashedAt": timestampOrNil(nilIfZero(trashed.TrashedAt)), "trashedBy": s.personReferenceOrNil(trashed.TrashedBy),
			"restorable": false, "blockedBy": nil,
			"appUrl": s.URL + "/projects/" + project.ID + "/trash",
		}})
	}
	return rows
}

// projectOfTheTeam is the project of the address for somebody on its team; a
// client is told what an outsider is told.
func (s *Server) projectOfTheTeam(r *http.Request, person *Person) *Project {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil || s.isAClient(person.ID) {
		return nil
	}
	return project
}

// toolOfTheTeam is the tool of the address among the tools its project lists.
func (s *Server) toolOfTheTeam(r *http.Request, person *Person) (*Project, *projectTool) {
	project := s.projectOfTheTeam(r, person)
	if project == nil {
		return nil, nil
	}
	for _, tool := range s.visibleTools(project, person) {
		if tool.id == r.PathValue("toolId") {
			return project, tool
		}
	}
	return nil, nil
}

func (s *Server) answerWithTool(w http.ResponseWriter, status int, project *Project, person *Person, toolID string) {
	for i, tool := range s.visibleTools(project, person) {
		if tool.id == toolID {
			writeJSON(w, status, toolOfAProjectResource(tool, i+1))
			return
		}
	}
	notFound(w)
}

func toolOfAProjectResource(tool *projectTool, position int) map[string]any {
	return map[string]any{
		"id": tool.id, "projectId": tool.projectID, "type": tool.toolType, "title": *tool.title,
		"position": position, "appUrl": tool.appURL,
	}
}

// visibleTools are the tools in use a person sees on the project page, in its
// order; the positions count these alone.
func (s *Server) visibleTools(project *Project, person *Person) []*projectTool {
	client := s.isAClient(person.ID)
	var visible []*projectTool
	for _, tool := range s.orderedTools(project) {
		if tool.inUse && (tool.shownToClients || !client) {
			visible = append(visible, tool)
		}
	}
	return visible
}

// placeTools writes down the order of every tool of the project.
func (s *Server) placeTools(project *Project, tools []*projectTool) {
	project.ToolOrder = nil
	for _, tool := range tools {
		project.ToolOrder = append(project.ToolOrder, tool.id)
	}
}

// orderedTools are all tools of a project out of the trash, those its order
// names first.
func (s *Server) orderedTools(project *Project) []*projectTool {
	tools := s.toolsOf(project.ID)
	placed := func(tool *projectTool) int {
		if i := slices.Index(project.ToolOrder, tool.id); i >= 0 {
			return i
		}
		return len(project.ToolOrder)
	}
	slices.SortStableFunc(tools, func(a, b *projectTool) int { return placed(a) - placed(b) })
	return tools
}

// toolsOf gathers the tools of a project from the lists of every type, in the
// order of the types.
func (s *Server) toolsOf(projectID string) []*projectTool {
	var tools []*projectTool
	for _, b := range s.Boards {
		if b.ProjectID == projectID && !b.Trashed {
			tools = append(tools, &projectTool{id: b.ID, projectID: projectID, toolType: "board", appURL: s.boardURL(b), title: &b.Title, inUse: true,
				trash: func() { b.Trashed = true }})
		}
	}
	for _, c := range s.Chats {
		if !c.isConversation() && c.ProjectID == projectID {
			tools = append(tools, &projectTool{id: c.ID, projectID: projectID, toolType: "chat", appURL: s.URL + "/projects/" + projectID + "/chats/" + c.ID, title: &c.Name, inUse: true,
				trash: func() { s.Chats = slices.DeleteFunc(s.Chats, func(one *Chat) bool { return one == c }) }})
		}
	}
	for _, t := range s.PostsTools {
		if t.ProjectID == projectID {
			tools = append(tools, &projectTool{id: t.ID, projectID: projectID, toolType: "posts", appURL: s.postsToolURL(t), title: &t.Title, inUse: !t.Archived, shownToClients: t.ShownToClients,
				trash: func() { s.PostsTools = slices.DeleteFunc(s.PostsTools, func(one *PostsTool) bool { return one == t }) }})
		}
	}
	for _, t := range s.TodosTools {
		if t.ProjectID == projectID && !t.Trashed {
			tools = append(tools, &projectTool{id: t.ID, projectID: projectID, toolType: "todos", appURL: s.todosToolURL(t), title: &t.Title, inUse: !t.Archived,
				trash: func() { t.Trashed = true }})
		}
	}
	for _, t := range s.CheckinsTools {
		if t.ProjectID == projectID && !t.Trashed {
			tools = append(tools, &projectTool{id: t.ID, projectID: projectID, toolType: "checkins", appURL: s.checkinsToolURL(t), title: &t.Title, inUse: !t.Archived,
				trash: func() { t.Trashed = true }})
		}
	}
	for _, t := range s.FilesTools {
		if t.ProjectID == projectID {
			tools = append(tools, &projectTool{id: t.ID, projectID: projectID, toolType: "files", appURL: s.filesToolURL(t), title: &t.Title, inUse: true, shownToClients: t.ShownToClients,
				trash: func() { s.FilesTools = slices.DeleteFunc(s.FilesTools, func(one *FilesTool) bool { return one == t }) }})
		}
	}
	for _, t := range s.ForwardsTools {
		if t.ProjectID == projectID {
			tools = append(tools, &projectTool{id: t.ID, projectID: projectID, toolType: "forwards", appURL: s.URL + "/projects/" + projectID + "/forwards/" + t.ID, title: &t.Title, inUse: true,
				trash: func() {
					s.ForwardsTools = slices.DeleteFunc(s.ForwardsTools, func(one *ForwardsTool) bool { return one == t })
				}})
		}
	}
	for _, t := range s.Calendars {
		if t.ProjectID == projectID {
			tools = append(tools, &projectTool{id: t.ID, projectID: projectID, toolType: "calendar", appURL: s.calendarURL(t), title: &t.Title, inUse: !t.Archived, shownToClients: t.ShownToClients,
				trash: func() { s.Calendars = slices.DeleteFunc(s.Calendars, func(one *CalendarTool) bool { return one == t }) }})
		}
	}
	for _, t := range s.DecisionsTools {
		if t.ProjectID == projectID {
			tools = append(tools, &projectTool{id: t.ID, projectID: projectID, toolType: "decisions", appURL: s.decisionsToolURL(t), title: &t.Title, inUse: !t.Archived, shownToClients: t.ShownToClients,
				trash: func() {
					s.DecisionsTools = slices.DeleteFunc(s.DecisionsTools, func(one *DecisionsTool) bool { return one == t })
				}})
		}
	}
	return tools
}

// validToolTitle refuses a title as Minka refuses a name: too long, with a
// control character, or empty once trimmed.
func validToolTitle(w http.ResponseWriter, title string) bool {
	title = phpTrim(title)
	switch {
	case utf8.RuneCountInString(title) > maxToolTitleLength:
		unprocessable(w, "toolTitleTooLong", "The title of a tool is too long.")
	case strings.IndexFunc(title, unicode.IsControl) >= 0:
		unprocessable(w, "toolTitleHasControlCharacters", "The title of a tool must not contain control characters.")
	case title == "":
		unprocessable(w, "toolTitleMissing", "A tool needs a title.")
	default:
		return true
	}
	return false
}
