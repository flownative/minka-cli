package fakebasecamp

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// TodoList is a list of a To-dos tool. Basecamp makes a group of a list one of
// these as well, and the account answers both the same way.
type TodoList struct {
	ID int64
	// Status is "active" when empty. ShownStatus replaces what the list says
	// about it, wherever it is listed.
	Status      string
	ShownStatus string
	Title       string
	Description string
	Creator     *Person
	CreatedAt   string
	UpdatedAt   string
	Files       []File
	Comments    []Comment
	Boosts      []Boost
	// Groups are the groups of a list, in the order the account lists them. A
	// group has none of its own.
	Groups []TodoList
	// Todos are the to-dos standing in the list or in the group, whatever their
	// status; the account lists what is done apart from what is open, as
	// Basecamp does.
	Todos []Todo
	// Twice has the account list this one twice wherever it lists it, as a list
	// that moves under a walk does: among the lists of a tool, among the groups of
	// a list, and among the recordings of the project.
	Twice bool
}

// todoProject is a project with every case of a To-dos tool: a tool whose lists
// the account shows in the order they stand in, one on the shelf and one in the
// trash, groups in use, one in the trash and one on the shelf, a group carrying
// what no group of a bundle can carry, a second tool of the same kind, and a
// tool that is switched off whose list, group and to-do hang on nothing a run
// extracts.
//
// Its to-dos hold every case as well: in the list and in a group, with days,
// with people on them and with subtasks, one done after the source last touched
// it and one done before, one in the trash, one in a group that is in the trash,
// and one assigned to somebody who has left the account.
func todoProject(ada, grace, edsger, former Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2025-04-%02dT%02d:00:00.000Z", day, hour) }

	plan := File{ID: 5061, SGID: SGID("gid://bc3/Attachment/5061?expires_in"), Key: FileKey(5061), Filename: "plan.png", ContentType: "image/png", Bytes: 512}

	launch := TodoList{
		ID: 9401, Title: "Launch", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(2, 8),
		Description: `<div>Everything for the launch, with ` + Mention(SGID("gid://bc3/Person/102?expires_in"), "", 102, "Grace Sample") + `</div>` + EmbeddedFile(plan.SGID, "plan.png"),
		Files:       []File{plan},
		Comments: []Comment{
			{ID: 9451, Creator: &grace, CreatedAt: at(2, 9), UpdatedAt: at(2, 9), Content: `<div>Good list</div>`,
				Boosts: []Boost{{ID: 9462, Content: "🙌", CreatedAt: at(2, 11), Booster: &ada}}},
		},
		Boosts: []Boost{{ID: 9461, Content: "👍", CreatedAt: at(2, 10), Booster: &edsger}},
	}
	sketch := File{ID: 5062, SGID: SGID("gid://bc3/Attachment/5062?expires_in"), Key: FileKey(5062), Filename: "sketch.png", ContentType: "image/png", Bytes: 256}
	// A group with everything a group cannot carry: a text with a file, comments
	// and boosts. None of it reaches a bundle, and none of it is asked for.
	launch.Groups = []TodoList{
		{ID: 9411, Title: "Before the launch", Creator: &ada, CreatedAt: at(1, 14), UpdatedAt: at(1, 14),
			// Assigned to somebody who has left the account, whom the bundle
			// carries as retired.
			Todos: []Todo{{ID: 9424, Title: "Choose a date", Creator: &ada, CreatedAt: at(2, 18), UpdatedAt: at(2, 18),
				Assignees: []Person{former}}},
			Description: `<div>What comes first</div>` + EmbeddedFile(sketch.SGID, "sketch.png"), Files: []File{sketch},
			Comments: []Comment{{ID: 9455, Creator: &grace, CreatedAt: at(2, 14), UpdatedAt: at(2, 14), Content: `<div>Good heading</div>`}},
			Boosts:   []Boost{{ID: 9465, Content: "👀", CreatedAt: at(2, 15), Booster: &grace}}},
		{ID: 9412, Title: "After the launch", Creator: &grace, CreatedAt: at(1, 15), UpdatedAt: at(1, 15), Todos: []Todo{
			// Done before the source last touched it, and nobody on it.
			{ID: 9425, Title: "Send thanks", Creator: &grace, CreatedAt: at(2, 19), UpdatedAt: at(3, 19),
				Completion: &Completion{At: at(2, 20), By: &grace}},
		}},
		{ID: 9413, Status: "trashed", Title: "Given up", Creator: &ada, CreatedAt: at(1, 16), UpdatedAt: at(3, 16), Todos: []Todo{
			{ID: 9426, Title: "Of a group in the trash", Creator: &ada, CreatedAt: at(2, 21), UpdatedAt: at(2, 21)},
		}},
		// A group on the shelf, which no bundle has a word for, with a to-do in use
		// standing in it: the to-do falls with the group and has to be counted.
		{ID: 9414, Status: "archived", Title: "On the shelf", Creator: &ada, CreatedAt: at(1, 17), UpdatedAt: at(3, 17), Todos: []Todo{
			{ID: 9430, Title: "Of a group on the shelf", Creator: &ada, CreatedAt: at(3, 9), UpdatedAt: at(3, 9)},
		}},
	}

	invitations := File{ID: 5063, SGID: SGID("gid://bc3/Attachment/5063?expires_in"), Key: FileKey(5063), Filename: "card.png", ContentType: "image/png", Bytes: 128}
	// In the list itself: one that is open with everything a to-do carries, one
	// that is done later than the source says it was touched, and one in the
	// trash.
	launch.Todos = []Todo{
		{ID: 9421, Title: "Print the invitations", Creator: &ada, CreatedAt: at(2, 8), UpdatedAt: at(2, 8),
			Notes:    `<div>On thick paper, says ` + Mention(SGID("gid://bc3/Person/104?expires_in"), "", 104, "Edsger Invented") + `</div>` + EmbeddedFile(invitations.SGID, "card.png"),
			Files:    []File{invitations},
			StartsOn: "2025-04-20", DueOn: "2025-05-01",
			// Not in the order of their IDs, and one of them twice.
			Assignees: []Person{grace, ada, grace}, NotifiedWhenDone: []Person{edsger, edsger}, Subtasks: 2,
			Comments: []Comment{{ID: 9471, Creator: &grace, CreatedAt: at(2, 12), UpdatedAt: at(2, 12), Content: `<div>Which paper?</div>`,
				Boosts: []Boost{{ID: 9482, Content: "📄", CreatedAt: at(2, 14), Booster: &ada}}}},
			Boosts: []Boost{{ID: 9481, Content: "👍", CreatedAt: at(2, 13), Booster: &grace}}},
		{ID: 9422, Title: "Book the room", Creator: &ada, CreatedAt: at(2, 9), UpdatedAt: at(2, 9),
			Completion: &Completion{At: at(3, 9), By: &grace}},
		{ID: 9423, Status: "trashed", Title: "Cancelled", Creator: &ada, CreatedAt: at(2, 10), UpdatedAt: at(3, 10)},
	}

	// A second list with a group of its own: a to-do that named a group of
	// another list would be seen naming it.
	later := TodoList{ID: 9402, Title: "Later", Creator: &grace, CreatedAt: at(1, 9), UpdatedAt: at(1, 9),
		Groups: []TodoList{{ID: 9416, Title: "Some day", Creator: &grace, CreatedAt: at(1, 19), UpdatedAt: at(1, 19), Todos: []Todo{
			{ID: 9429, Title: "Think about it", Creator: &grace, CreatedAt: at(3, 8), UpdatedAt: at(3, 8)},
		}}}}
	finished := TodoList{ID: 9403, Status: "archived", Title: "Finished", Creator: &ada, CreatedAt: at(1, 10), UpdatedAt: at(3, 10)}
	dropped := TodoList{ID: 9404, Status: "trashed", Title: "Dropped", Creator: &ada, CreatedAt: at(1, 11), UpdatedAt: at(3, 11)}

	todos := tool(9080, "todoset", "todosets", "To-dos", true, 1, &ada)
	todos.CreatedAt, todos.UpdatedAt = at(1, 7), at(3, 12)
	// The account shows them as they stand, the topmost first, and a bundle
	// writes them the other way round.
	todos.TodoLists = []TodoList{launch, later, finished, dropped}

	chores := tool(9081, "todoset", "todosets", "Chores", true, 2, &grace)
	chores.CreatedAt, chores.UpdatedAt = at(1, 7), at(3, 12)
	chores.TodoLists = []TodoList{{ID: 9405, Title: "Every week", Creator: &grace, CreatedAt: at(1, 12), UpdatedAt: at(1, 12),
		Todos: []Todo{{ID: 9427, Title: "Water the plants", Creator: &grace, CreatedAt: at(2, 22), UpdatedAt: at(2, 22)}}}}

	retired := tool(9082, "todoset", "todosets", "Old to-dos", false, 0, &ada)
	retired.TodoLists = []TodoList{
		{ID: 9406, Status: "archived", Title: "Of a tool nobody sees", Creator: &ada, CreatedAt: at(1, 13), UpdatedAt: at(3, 13),
			Groups: []TodoList{{ID: 9415, Status: "trashed", Title: "Of a list nobody sees", Creator: &ada, CreatedAt: at(1, 18), UpdatedAt: at(3, 18)}},
			Todos:  []Todo{{ID: 9428, Status: "trashed", Title: "Of a list nobody sees", Creator: &ada, CreatedAt: at(2, 23), UpdatedAt: at(3, 23)}}},
	}

	return Project{
		ID: 7008, Name: "To-do Project", Status: "active",
		CreatedAt: at(1, 7), UpdatedAt: at(3, 12),
		Members: []int64{101, 102, 104},
		Dock:    []Tool{todos, chores, retired},
	}
}

// Todo is a to-do of a list or of one of its groups.
type Todo struct {
	ID int64
	// Status is "active" when empty. ShownStatus replaces what the to-do says
	// about it, wherever it is listed.
	Status      string
	ShownStatus string
	Title       string
	// Notes are the notes of a to-do as Basecamp renders them.
	Notes     string
	StartsOn  string
	DueOn     string
	Creator   *Person
	CreatedAt string
	UpdatedAt string
	Assignees []Person
	// NotifiedWhenDone are the people the source tells when the to-do is done.
	NotifiedWhenDone []Person
	Completion       *Completion
	// Subtasks is how many subtasks the to-do has, of which a bundle carries
	// none.
	Subtasks int
	// EmptyDays has the account send the days as empty text rather than leaving
	// them out, the way a source writes a field it has nothing for.
	EmptyDays bool
	Files     []File
	Comments  []Comment
	Boosts    []Boost
	// Twice has the list of to-dos hold this one twice.
	Twice bool
}

var (
	todoListsPattern      = regexp.MustCompile(`^buckets/([0-9]+)/todosets/([0-9]+)/todolists\.json$`)
	todoGroupsPattern     = regexp.MustCompile(`^buckets/([0-9]+)/todolists/([0-9]+)/groups\.json$`)
	todoRecordingsPattern = regexp.MustCompile(`^projects/recordings\.json\?type=Todolist&bucket=([0-9]+)&status=(archived|trashed)$`)
	todosPattern          = regexp.MustCompile(`^buckets/([0-9]+)/todolists/([0-9]+)/todos\.json(\?completed=true)?$`)
	todoItemsPattern      = regexp.MustCompile(`^projects/recordings\.json\?type=Todo&bucket=([0-9]+)&status=(archived|trashed)$`)
)

// todos answers the paths of To-dos tools. The second value tells whether the
// path was one of them.
func (a *Account) todos(path string, page int) ([]byte, bool) {
	switch {
	case todoListsPattern.MatchString(path):
		match := todoListsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			for t := range p.Dock {
				tool := &p.Dock[t]
				if tool.Type != "todosets" || strconv.FormatInt(tool.ID, 10) != match[2] {
					continue
				}
				var found []map[string]any
				for i := range tool.TodoLists {
					list := &tool.TodoLists[i]
					if list.status() != "active" {
						continue
					}
					found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list))
					if list.Twice {
						found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list))
					}
				}
				return a.pageOf(found, page), true
			}
		}
	case todoGroupsPattern.MatchString(path):
		match := todoGroupsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			if list, known := a.theTodoList(p, match[2]); known {
				var found []map[string]any
				for i := range list.Groups {
					group := &list.Groups[i]
					if group.status() != "active" {
						continue
					}
					found = append(found, a.todoListJSON(p, list.ID, "Todolist", group))
					if group.Twice {
						found = append(found, a.todoListJSON(p, list.ID, "Todolist", group))
					}
				}
				return a.pageOf(found, page), true
			}
		}
	case todosPattern.MatchString(path):
		match := todosPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			if list, known := a.theTodoList(p, match[2]); known {
				done := match[3] != ""
				var found []map[string]any
				for i := range list.Todos {
					one := &list.Todos[i]
					if one.status() != "active" || (one.Completion != nil) != done {
						continue
					}
					found = append(found, a.todoJSON(p, list, one))
					if one.Twice {
						found = append(found, a.todoJSON(p, list, one))
					}
				}
				return a.pageOf(found, page), true
			}
		}
	case todoItemsPattern.MatchString(path):
		match := todoItemsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var found []map[string]any
			a.eachTodoListAndGroup(p, func(list *TodoList) {
				for i := range list.Todos {
					if one := &list.Todos[i]; one.status() == match[2] {
						found = append(found, a.todoJSON(p, list, one))
					}
				}
			})
			// One list across all To-dos tools, whatever list a to-do stands in.
			sort.SliceStable(found, func(i, j int) bool { return found[i]["id"].(int64) > found[j]["id"].(int64) })
			return a.pageOf(found, page), true
		}
	case todoRecordingsPattern.MatchString(path):
		match := todoRecordingsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var found []map[string]any
			a.eachTodoList(p, func(tool *Tool, list *TodoList) {
				if list.status() == match[2] {
					found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list))
					if list.Twice {
						found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list))
					}
				}
				for i := range list.Groups {
					group := &list.Groups[i]
					if group.status() != match[2] {
						continue
					}
					found = append(found, a.todoListJSON(p, list.ID, "Todolist", group))
					if group.Twice {
						found = append(found, a.todoListJSON(p, list.ID, "Todolist", group))
					}
				}
			})
			// One list across all To-dos tools, whatever tool a list is of.
			sort.SliceStable(found, func(i, j int) bool { return found[i]["id"].(int64) > found[j]["id"].(int64) })
			return a.pageOf(found, page), true
		}
	}
	return nil, false
}

// eachTodoListAndGroup visits the lists of every To-dos tool of a project and
// the groups of those lists. The account answers for a group whatever it answers
// for a list, so a run that asks for what no group of a bundle can carry gets an
// answer and not a refusal: what such a run does with it is then what a test
// sees.
func (a *Account) eachTodoListAndGroup(p *Project, visit func(*TodoList)) {
	a.eachTodoList(p, func(_ *Tool, list *TodoList) {
		visit(list)
		for i := range list.Groups {
			visit(&list.Groups[i])
		}
	})
}

// eachTodo visits the to-dos of every list and group of a project.
func (a *Account) eachTodo(p *Project, visit func(*Todo)) {
	a.eachTodoListAndGroup(p, func(list *TodoList) {
		for i := range list.Todos {
			visit(&list.Todos[i])
		}
	})
}

// theTodoList finds a list or a group of a project by its ID.
func (a *Account) theTodoList(p *Project, id string) (*TodoList, bool) {
	var found *TodoList
	a.eachTodoList(p, func(_ *Tool, list *TodoList) {
		if strconv.FormatInt(list.ID, 10) == id {
			found = list
		}
		for i := range list.Groups {
			if strconv.FormatInt(list.Groups[i].ID, 10) == id {
				found = &list.Groups[i]
			}
		}
	})
	return found, found != nil
}

// eachTodoList visits the lists of every To-dos tool of a project, those of
// tools that are switched off included. A group is no list of its own here: it
// hangs on the list that holds it.
func (a *Account) eachTodoList(p *Project, visit func(*Tool, *TodoList)) {
	for t := range p.Dock {
		for l := range p.Dock[t].TodoLists {
			visit(&p.Dock[t], &p.Dock[t].TodoLists[l])
		}
	}
}

func (l *TodoList) status() string {
	if l.Status == "" {
		return "active"
	}
	return l.Status
}

// shownStatus is what the list says about its status where it is listed, which a
// test can make something a bundle has no word for.
func (l *TodoList) shownStatus() string {
	if l.ShownStatus != "" {
		return l.ShownStatus
	}
	return l.status()
}

func (t *Todo) status() string {
	if t.Status == "" {
		return "active"
	}
	return t.Status
}

// shownStatus is what the to-do says about its status where it is listed, which
// a test can make something a bundle has no word for.
func (t *Todo) shownStatus() string {
	if t.ShownStatus != "" {
		return t.ShownStatus
	}
	return t.status()
}

// day is what the account says of a day the to-do has none for: nothing, or the
// empty text a source writes instead.
func (t *Todo) day(value string) any {
	if value != "" {
		return value
	}
	if t.EmptyDays {
		return ""
	}
	return nil
}

func (a *Account) todoJSON(p *Project, in *TodoList, t *Todo) map[string]any {
	people := func(named []Person) []map[string]any {
		all := []map[string]any{}
		for _, one := range named {
			all = append(all, a.person(one))
		}
		return all
	}
	answer := map[string]any{
		"id": t.ID, "status": t.shownStatus(), "title": t.Title, "content": t.Title, "type": "Todo",
		"description": t.Notes, "description_attachments": a.filesJSON(t.Files),
		"parent":    map[string]any{"id": in.ID, "type": "Todolist"},
		"starts_on": t.day(t.StartsOn), "due_on": t.day(t.DueOn),
		"assignees": people(t.Assignees), "completion_subscribers": people(t.NotifiedWhenDone),
		"completed": t.Completion != nil, "subtasks_count": t.Subtasks,
		"comments_count": len(t.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, t.ID),
		"boosts_count": len(t.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, t.ID),
		"created_at": t.CreatedAt, "updated_at": t.UpdatedAt,
	}
	if t.Creator != nil {
		answer["creator"] = a.person(*t.Creator)
	}
	if t.Completion != nil {
		answer["completion"] = a.completionJSON(*t.Completion)
	}
	return answer
}

func (a *Account) todoListJSON(p *Project, parentID int64, parentType string, l *TodoList) map[string]any {
	answer := map[string]any{
		"id": l.ID, "status": l.shownStatus(), "title": l.Title, "name": l.Title, "type": "Todolist",
		"description": l.Description, "description_attachments": a.filesJSON(l.Files),
		"parent":         map[string]any{"id": parentID, "type": parentType},
		"comments_count": len(l.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, l.ID),
		"boosts_count": len(l.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, l.ID),
		"todos_url":  fmt.Sprintf("%sbuckets/%d/todolists/%d/todos.json", a.root(), p.ID, l.ID),
		"groups_url": fmt.Sprintf("%sbuckets/%d/todolists/%d/groups.json", a.root(), p.ID, l.ID),
		"created_at": l.CreatedAt, "updated_at": l.UpdatedAt,
	}
	if l.Creator != nil {
		answer["creator"] = a.person(*l.Creator)
	}
	return answer
}
