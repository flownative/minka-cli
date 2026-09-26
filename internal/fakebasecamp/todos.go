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
	// Status is "active" when empty.
	Status      string
	Title       string
	Description string
	Creator     *Person
	CreatedAt   string
	UpdatedAt   string
	Files       []File
	Comments    []Comment
	Boosts      []Boost
	// Twice has the list of lists hold this one twice, as a list that moves
	// under a walk does.
	Twice bool
}

// todoProject is a project with every case of a To-dos tool: a tool whose lists
// the account shows in the order they stand in, one on the shelf and one in the
// trash, a second tool of the same kind, and a tool that is switched off whose
// list hangs on nothing a run extracts.
func todoProject(ada, grace, edsger Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
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
	later := TodoList{ID: 9402, Title: "Later", Creator: &grace, CreatedAt: at(1, 9), UpdatedAt: at(1, 9)}
	finished := TodoList{ID: 9403, Status: "archived", Title: "Finished", Creator: &ada, CreatedAt: at(1, 10), UpdatedAt: at(3, 10)}
	dropped := TodoList{ID: 9404, Status: "trashed", Title: "Dropped", Creator: &ada, CreatedAt: at(1, 11), UpdatedAt: at(3, 11)}

	todos := tool(9080, "todoset", "todosets", "To-dos", true, 1, &ada)
	todos.CreatedAt, todos.UpdatedAt = at(1, 7), at(3, 12)
	// The account shows them as they stand, the topmost first, and a bundle
	// writes them the other way round.
	todos.TodoLists = []TodoList{launch, later, finished, dropped}

	chores := tool(9081, "todoset", "todosets", "Chores", true, 2, &grace)
	chores.CreatedAt, chores.UpdatedAt = at(1, 7), at(3, 12)
	chores.TodoLists = []TodoList{{ID: 9405, Title: "Every week", Creator: &grace, CreatedAt: at(1, 12), UpdatedAt: at(1, 12)}}

	retired := tool(9082, "todoset", "todosets", "Old to-dos", false, 0, &ada)
	retired.TodoLists = []TodoList{
		{ID: 9406, Status: "archived", Title: "Of a tool nobody sees", Creator: &ada, CreatedAt: at(1, 13), UpdatedAt: at(3, 13)},
	}

	return Project{
		ID: 7008, Name: "To-do Project", Status: "active",
		CreatedAt: at(1, 7), UpdatedAt: at(3, 12),
		Members: []int64{101, 102, 104},
		Dock:    []Tool{todos, chores, retired},
	}
}

var (
	todoListsPattern      = regexp.MustCompile(`^buckets/([0-9]+)/todosets/([0-9]+)/todolists\.json$`)
	todoRecordingsPattern = regexp.MustCompile(`^projects/recordings\.json\?type=Todolist&bucket=([0-9]+)&status=(archived|trashed)$`)
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
	case todoRecordingsPattern.MatchString(path):
		match := todoRecordingsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var found []map[string]any
			a.eachTodoList(p, func(tool *Tool, list *TodoList) {
				if list.status() == match[2] {
					found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list))
				}
			})
			// One list across all To-dos tools, whatever tool a list is of.
			sort.SliceStable(found, func(i, j int) bool { return found[i]["id"].(int64) > found[j]["id"].(int64) })
			return a.pageOf(found, page), true
		}
	}
	return nil, false
}

// eachTodoList visits the lists of every To-dos tool of a project, those of
// tools that are switched off included.
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

func (a *Account) todoListJSON(p *Project, parentID int64, parentType string, l *TodoList) map[string]any {
	answer := map[string]any{
		"id": l.ID, "status": l.status(), "title": l.Title, "name": l.Title, "type": "Todolist",
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
