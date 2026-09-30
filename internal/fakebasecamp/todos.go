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
	// Status is the list's own, "active" when empty. A group standing in a list
	// that is not in use says its list's status instead and that it inherits it,
	// as Basecamp does. ShownStatus replaces what the list says about it,
	// wherever it is listed.
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
	// VisibleToClients is what the list says about clients, which is its own word
	// and not that of its tool. A group carries one as well, which no bundle has a
	// word for.
	VisibleToClients *bool
	// Color is the colour of a group, "white" when empty, which is what every
	// group of the account says unless somebody coloured it. A list says null.
	Color string
	// OnlyInItsList has the account leave a group out of the archived and
	// trashed records of the project, so that a run meets it among the groups of
	// its list alone.
	OnlyInItsList bool
	// Colorless has a group say null for its colour, which no group of the
	// account was seen to do.
	Colorless bool
	// EmptyColor has a group say "" for its colour, which no group of the
	// account was seen to do either.
	EmptyColor bool
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
// one assigned to somebody who has left the account, two that repeat, and two
// hanging on the tool itself and in no list of it, one in use and one on the
// shelf.
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
		{ID: 9411, Title: "Before the launch", Color: "blue", Creator: &ada, CreatedAt: at(1, 14), UpdatedAt: at(1, 14),
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
		// A colour Minka has no name for, spelt as none of its names is.
		{ID: 9413, Status: "trashed", Title: "Given up", Color: "Purple", Creator: &ada, CreatedAt: at(1, 16), UpdatedAt: at(3, 16), Todos: []Todo{
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
			Subscribers: []Person{edsger, ada, former},
			Comments: []Comment{{ID: 9471, Creator: &grace, CreatedAt: at(2, 12), UpdatedAt: at(2, 12), Content: `<div>Which paper?</div>`,
				Boosts: []Boost{{ID: 9482, Content: "📄", CreatedAt: at(2, 14), Booster: &ada}}}},
			Boosts: []Boost{{ID: 9481, Content: "👍", CreatedAt: at(2, 13), Booster: &grace}}},
		{ID: 9422, Title: "Book the room", Creator: &ada, CreatedAt: at(2, 9), UpdatedAt: at(2, 9),
			Completion: &Completion{At: at(3, 9), By: &grace}},
		{ID: 9423, Status: "trashed", Title: "Cancelled", Creator: &ada, CreatedAt: at(2, 10), UpdatedAt: at(3, 10),
			Subscribers: []Person{ada}},
	}

	// A second list with a group of its own: a to-do that named a group of
	// another list would be seen naming it.
	later := TodoList{ID: 9402, Title: "Later", Creator: &grace, CreatedAt: at(1, 9), UpdatedAt: at(1, 9),
		Groups: []TodoList{{ID: 9416, Title: "Some day", Colorless: true, Creator: &grace, CreatedAt: at(1, 19), UpdatedAt: at(1, 19), Todos: []Todo{
			// Repeating in a rhythm of another kind: the second Tuesday of every
			// month, as the account writes it.
			{ID: 9429, Title: "Think about it", Creator: &grace, CreatedAt: at(3, 8), UpdatedAt: at(3, 8), DueOn: "2025-04-08",
				Repetition: &Repetition{Frequency: "every_month", Days: []int{2}, WeekInstance: interval(2), StartDate: "2025-04-08"}},
		}}}}
	finished := TodoList{ID: 9403, Status: "archived", Title: "Finished", Creator: &ada, CreatedAt: at(1, 10), UpdatedAt: at(3, 10)}
	dropped := TodoList{ID: 9404, Status: "trashed", Title: "Dropped", Creator: &ada, CreatedAt: at(1, 11), UpdatedAt: at(3, 11)}

	todos := tool(9080, "todoset", "todosets", "To-dos", true, 1, &ada)
	todos.CreatedAt, todos.UpdatedAt = at(1, 7), at(3, 12)
	// The account shows them as they stand, the topmost first, and a bundle
	// writes them the other way round.
	todos.TodoLists = []TodoList{launch, later, finished, dropped}
	// Hanging on the tool itself and in no list of it: one in use, which the
	// account hands out under the tool's own identifier alone, and one on the
	// shelf, which it hands out among the records of the project.
	todos.Todos = []Todo{
		{ID: 9431, Title: "In no list at all", Creator: &ada, CreatedAt: at(3, 10), UpdatedAt: at(3, 10)},
		{ID: 9432, Status: "archived", Title: "In no list and on the shelf", Creator: &ada, CreatedAt: at(3, 11), UpdatedAt: at(3, 11)},
	}

	chores := tool(9081, "todoset", "todosets", "Chores", true, 2, &grace)
	chores.CreatedAt, chores.UpdatedAt = at(1, 7), at(3, 12)
	// A to-do that repeats every Monday, as the account writes it.
	chores.TodoLists = []TodoList{{ID: 9405, Title: "Every week", Creator: &grace, CreatedAt: at(1, 12), UpdatedAt: at(1, 12),
		Todos: []Todo{{ID: 9427, Title: "Water the plants", Creator: &grace, CreatedAt: at(2, 22), UpdatedAt: at(2, 22), DueOn: "2025-04-07",
			Repetition: &Repetition{Frequency: "every_week", Days: []int{1}, StartDate: "2025-04-07"}}}}}

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
	// Status is the to-do's own, "active" when empty. A to-do standing in a list
	// or a group that is not in use says that one's status instead and that it
	// inherits it, as Basecamp does. ShownStatus replaces what the to-do says
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
	// Repetition is the schedule of a to-do that repeats; a to-do that repeats
	// nothing carries none. OddRepetition is sent in its place as it stands, for
	// a schedule of a shape the account was never seen to send.
	Repetition    *Repetition
	OddRepetition map[string]any
	// EmptyDays has the account send the days as empty text rather than leaving
	// them out, the way a source writes a field it has nothing for.
	EmptyDays bool
	Files     []File
	Comments  []Comment
	Boosts    []Boost
	// Twice has the list of to-dos hold this one twice.
	Twice bool
	// VisibleToClients is what the to-do says about clients, which no bundle has a
	// word for: a to-do follows its list.
	VisibleToClients *bool
	// Subscribers are whom the account hands the to-do to; nil is nobody.
	Subscribers []Person
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
					found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list, ofItsOwn(list.status())))
					if list.Twice {
						found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list, ofItsOwn(list.status())))
					}
				}
				return a.pageOf(found, page), true
			}
		}
	case todoGroupsPattern.MatchString(path):
		match := todoGroupsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			if list, at, known := a.theTodoList(p, match[2]); known {
				var found []map[string]any
				for i := range list.Groups {
					group := &list.Groups[i]
					if group.status() != "active" {
						continue
					}
					found = append(found, a.todoListJSON(p, list.ID, "Todolist", group, standingIn(group.status(), at)))
					if group.Twice {
						found = append(found, a.todoListJSON(p, list.ID, "Todolist", group, standingIn(group.status(), at)))
					}
				}
				return a.pageOf(found, page), true
			}
		}
	case todosPattern.MatchString(path):
		match := todosPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			done := match[3] != ""
			// A To-dos tool answers under this path as a list of it does, and
			// hands out the to-dos hanging on itself.
			if tool, known := a.theTodoset(p, match[2]); known {
				return a.pageOf(a.todosInAPlace(p, tool.ID, "Todoset", tool.Todos, ofItsOwn("active"), done), page), true
			}
			if list, at, known := a.theTodoList(p, match[2]); known {
				return a.pageOf(a.todosInAPlace(p, list.ID, "Todolist", list.Todos, at, done), page), true
			}
		}
	case todoItemsPattern.MatchString(path):
		match := todoItemsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var found []map[string]any
			a.eachTodoPlace(p, func(parentID int64, parentType string, todos []Todo, at standing) {
				for i := range todos {
					one := &todos[i]
					if its := standingIn(one.status(), at); its.status == match[2] {
						found = append(found, a.todoJSON(p, parentID, parentType, one, its))
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
				at := ofItsOwn(list.status())
				if at.status == match[2] {
					found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list, at))
					if list.Twice {
						found = append(found, a.todoListJSON(p, tool.ID, "Todoset", list, at))
					}
				}
				for i := range list.Groups {
					group := &list.Groups[i]
					its := standingIn(group.status(), at)
					if its.status != match[2] || group.OnlyInItsList {
						continue
					}
					found = append(found, a.todoListJSON(p, list.ID, "Todolist", group, its))
					if group.Twice {
						found = append(found, a.todoListJSON(p, list.ID, "Todolist", group, its))
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

// eachTodo visits the to-dos of every list and group of a project, and those
// hanging on a To-dos tool itself.
func (a *Account) eachTodo(p *Project, visit func(*Todo)) {
	a.eachTodoPlace(p, func(_ int64, _ string, todos []Todo, _ standing) {
		for i := range todos {
			visit(&todos[i])
		}
	})
}

// eachTodoPlace visits every place of a project that holds to-dos, with what a
// record standing there says it hangs on and the status the place hands down:
// the lists of its To-dos tools, the groups of those lists, and the tools
// themselves, on which Basecamp lets a to-do hang straight.
func (a *Account) eachTodoPlace(p *Project, visit func(parentID int64, parentType string, todos []Todo, at standing)) {
	for t := range p.Dock {
		if tool := &p.Dock[t]; tool.Type == "todosets" {
			visit(tool.ID, "Todoset", tool.Todos, ofItsOwn("active"))
		}
	}
	a.eachTodoList(p, func(_ *Tool, list *TodoList) {
		at := ofItsOwn(list.status())
		visit(list.ID, "Todolist", list.Todos, at)
		for i := range list.Groups {
			group := &list.Groups[i]
			visit(group.ID, "Todolist", group.Todos, standingIn(group.status(), at))
		}
	})
}

// todosInAPlace are the to-dos of one place as the account hands them out: those
// that are open or those that are done, as Basecamp keeps the two apart.
func (a *Account) todosInAPlace(p *Project, parentID int64, parentType string, todos []Todo, at standing, done bool) []map[string]any {
	var found []map[string]any
	for i := range todos {
		one := &todos[i]
		if one.status() != "active" || (one.Completion != nil) != done {
			continue
		}
		its := standingIn(one.status(), at)
		found = append(found, a.todoJSON(p, parentID, parentType, one, its))
		if one.Twice {
			found = append(found, a.todoJSON(p, parentID, parentType, one, its))
		}
	}
	return found
}

// theTodoset finds a To-dos tool of a project by its ID, switched off or not.
func (a *Account) theTodoset(p *Project, id string) (*Tool, bool) {
	for t := range p.Dock {
		if tool := &p.Dock[t]; tool.Type == "todosets" && strconv.FormatInt(tool.ID, 10) == id {
			return tool, true
		}
	}
	return nil, false
}

// theTodoList finds a list or a group of a project by its ID, with the status
// it hands down to what stands in it.
func (a *Account) theTodoList(p *Project, id string) (*TodoList, standing, bool) {
	var found *TodoList
	var at standing
	a.eachTodoList(p, func(_ *Tool, list *TodoList) {
		if strconv.FormatInt(list.ID, 10) == id {
			found, at = list, ofItsOwn(list.status())
		}
		for i := range list.Groups {
			if group := &list.Groups[i]; strconv.FormatInt(group.ID, 10) == id {
				found, at = group, standingIn(group.status(), ofItsOwn(list.status()))
			}
		}
	})
	return found, at, found != nil
}

// standing is the status a record says it has and whether it says it inherits
// it from the place it stands in.
type standing struct {
	status    string
	inherited bool
}

func ofItsOwn(status string) standing { return standing{status: status} }

// standingIn is what Basecamp says of a record standing in a place: in a place
// that is not in use, the place's status, inherited; in a place in use, the
// record's own. A record with a status of its own in a place that is not in use
// says the place's as well here, which no live account has shown.
func standingIn(own string, place standing) standing {
	if place.status != "active" {
		return standing{status: place.status, inherited: true}
	}
	return ofItsOwn(own)
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

// shownStatus is what the list says about its status where it is listed: what
// it stands as, unless a test makes it something a bundle has no word for.
func (l *TodoList) shownStatus(its standing) string {
	if l.ShownStatus != "" {
		return l.ShownStatus
	}
	return its.status
}

func (t *Todo) status() string {
	if t.Status == "" {
		return "active"
	}
	return t.Status
}

// shownStatus is what the to-do says about its status where it is listed: what
// it stands as, unless a test makes it something a bundle has no word for.
func (t *Todo) shownStatus(its standing) string {
	if t.ShownStatus != "" {
		return t.ShownStatus
	}
	return its.status
}

// Repetition is the schedule of a to-do that repeats. The account writes it as
// Basecamp does: all ten keys of a schedule, null the ones the rhythm has no use
// for, and the days as a list, empty for every_year. Only the newest occurrence
// of a series carries one; a to-do that repeats nothing, an older occurrence and
// a done one carry **no such key at all**, which is the only thing that tells
// them apart.
type Repetition struct {
	// Frequency is the rhythm: every_day_of_month, every_month, every_week,
	// every_year or custom_month, as the account was seen to hold them.
	Frequency string
	// Days are the days the rhythm names: weekdays with Sunday 0 for
	// every_week, every_month and custom_month, the day of the month for
	// every_day_of_month.
	Days []int
	// StartDate and EndDate are days of the shape YYYY-MM-DD, the start the
	// to-do's due day; a schedule that does not run out has no end, and none
	// was seen with one.
	StartDate string
	EndDate   string
	// Hour and Minute are the time of day it comes at, 0 where nil, as every
	// schedule of the account says. Duration and WeekInterval were only ever
	// seen null, WeekInstance is the n-th weekday of the month and
	// MonthInterval counts the months of custom_month.
	Hour, Minute, Duration, MonthInterval, WeekInstance, WeekInterval *int
}

// interval is one of the numbers a schedule counts in, for a fixture that names
// it; a schedule leaves out what its rhythm has no use for.
func interval(value int) *int { return &value }

// repetitionJSON is a schedule as Basecamp hands it out, with every key it holds.
func repetitionJSON(r *Repetition) map[string]any {
	number := func(value *int) any {
		if value == nil {
			return nil
		}
		return *value
	}
	days := r.Days
	if days == nil {
		days = []int{}
	}
	timeOfDay := func(value *int) int {
		if value == nil {
			return 0
		}
		return *value
	}
	day := func(value string) any {
		if value == "" {
			return nil
		}
		return value
	}
	return map[string]any{
		"frequency": r.Frequency, "days": days,
		"start_date": day(r.StartDate), "end_date": day(r.EndDate),
		"hour": timeOfDay(r.Hour), "minute": timeOfDay(r.Minute), "duration": number(r.Duration),
		"month_interval": number(r.MonthInterval), "week_instance": number(r.WeekInstance),
		"week_interval": number(r.WeekInterval),
	}
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

func (a *Account) todoJSON(p *Project, parentID int64, parentType string, t *Todo, its standing) map[string]any {
	people := func(named []Person) []map[string]any {
		all := []map[string]any{}
		for _, one := range named {
			all = append(all, a.person(one))
		}
		return all
	}
	answer := map[string]any{
		"id": t.ID, "status": t.shownStatus(its), "title": t.Title, "content": t.Title, "type": "Todo",
		"description": t.Notes, "description_attachments": a.filesJSON(t.Files),
		"parent":    map[string]any{"id": parentID, "type": parentType},
		"starts_on": t.day(t.StartsOn), "due_on": t.day(t.DueOn),
		"assignees": people(t.Assignees), "completion_subscribers": people(t.NotifiedWhenDone),
		"completed": t.Completion != nil, "subtasks_count": t.Subtasks,
		"comments_count": len(t.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, t.ID),
		"boosts_count": len(t.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, t.ID),
		"created_at": t.CreatedAt, "updated_at": t.UpdatedAt, "inherits_status": its.inherited,
	}
	if t.VisibleToClients != nil {
		answer["visible_to_clients"] = *t.VisibleToClients
	}
	if t.Creator != nil {
		answer["creator"] = a.person(*t.Creator)
	}
	if t.Completion != nil {
		answer["completion"] = a.completionJSON(*t.Completion)
	}
	// A to-do that repeats nothing comes without the key, not with an empty one.
	if t.Repetition != nil {
		answer["repetition_schedule"] = repetitionJSON(t.Repetition)
	}
	if t.OddRepetition != nil {
		answer["repetition_schedule"] = t.OddRepetition
	}
	return answer
}

func (a *Account) todoListJSON(p *Project, parentID int64, parentType string, l *TodoList, its standing) map[string]any {
	answer := map[string]any{
		"id": l.ID, "status": l.shownStatus(its), "title": l.Title, "name": l.Title, "type": "Todolist",
		"description": l.Description, "description_attachments": a.filesJSON(l.Files),
		"parent":         map[string]any{"id": parentID, "type": parentType},
		"comments_count": len(l.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, l.ID),
		"boosts_count": len(l.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, l.ID),
		"todos_url":  fmt.Sprintf("%sbuckets/%d/todolists/%d/todos.json", a.root(), p.ID, l.ID),
		"groups_url": fmt.Sprintf("%sbuckets/%d/todolists/%d/groups.json", a.root(), p.ID, l.ID),
		"created_at": l.CreatedAt, "updated_at": l.UpdatedAt, "inherits_status": its.inherited,
	}
	if l.VisibleToClients != nil {
		answer["visible_to_clients"] = *l.VisibleToClients
	}
	if l.Creator != nil {
		answer["creator"] = a.person(*l.Creator)
	}
	answer["color"] = nil
	if parentType == "Todolist" && !l.Colorless {
		answer["color"] = l.Color
		if l.Color == "" && !l.EmptyColor {
			answer["color"] = "white"
		}
	}
	return answer
}
