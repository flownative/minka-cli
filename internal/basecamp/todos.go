package basecamp

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/flownative/minka-cli/internal/bundle"
)

// todoList is a list of a To-dos tool. Basecamp makes a group of a list one of
// these as well, and what a record hangs on is what tells the two apart.
type todoList struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	// Description is the text of a list as Basecamp renders it; there is no
	// stored form of it.
	Description   string     `json:"description"`
	Files         []bodyFile `json:"description_attachments"`
	Parent        hangsOn    `json:"parent"`
	CommentsCount int        `json:"comments_count"`
	BoostsCount   int        `json:"boosts_count"`
}

// hangsOn is what a record of Basecamp stands in.
type hangsOn struct {
	ID int64 `json:"id"`
}

// todo is a to-do of a list, standing in the list itself or in one of its
// groups.
type todo struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	// Description is the notes of a to-do as Basecamp renders them; there is no
	// stored form of them.
	Description string     `json:"description"`
	Files       []bodyFile `json:"description_attachments"`
	Parent      hangsOn    `json:"parent"`
	StartsOn    *string    `json:"starts_on"`
	DueOn       *string    `json:"due_on"`
	Assignees   []person   `json:"assignees"`
	// NotifiedWhenDone are the people the source tells when the to-do is done.
	NotifiedWhenDone []person    `json:"completion_subscribers"`
	Completion       *completion `json:"completion"`
	// SubtasksCount is how many subtasks the to-do has. A bundle carries none,
	// so the number is all that is left of them.
	SubtasksCount int `json:"subtasks_count"`
	CommentsCount int `json:"comments_count"`
	BoostsCount   int `json:"boosts_count"`
}

// todoStatuses are the statuses a bundle has a word for in a group and in a
// to-do. The shelf is the list's alone: a group and a to-do know the trash and
// nothing beyond it.
var todoStatuses = map[string]bool{bundle.StatusActive: true, bundle.StatusTrashed: true}

// todoTool collects the lists of one To-dos tool.
type todoTool struct {
	id int64
	// gone are the lists on the shelf and in the trash, in the order Basecamp
	// lists them, and shown the lists in use as Basecamp shows them, the topmost
	// first.
	gone  []todoList
	shown []todoList
}

// inFileOrder are the lists of a tool in the order a bundle writes them. Minka
// puts every new list on top of its tool, so the lists in use come the other way
// round; what has left the list goes first, where it cannot push the rest about.
func (t *todoTool) inFileOrder() []todoList {
	all := make([]todoList, 0, len(t.gone)+len(t.shown))
	all = append(all, t.gone...)
	for i := len(t.shown) - 1; i >= 0; i-- {
		all = append(all, t.shown[i])
	}
	return all
}

// extractTodos adds the lists of every To-dos tool and what stands in them,
// project by project and in the order of the tools.
func (e *Extractor) extractTodos(ctx context.Context, w *bundle.Writer) error {
	for _, t := range e.talks {
		if len(t.todosets) == 0 {
			continue
		}
		if err := e.extractTodosOfProject(ctx, w, t.bucket, t.todosets); err != nil {
			return err
		}
	}
	return nil
}

// extractTodosOfProject adds the To-dos tools of one project.
//
// Basecamp lists the lists of a tool itself, and the archived and the trashed
// ones of a project in one list across all its To-dos tools, those that are
// switched off included. Such a record names only what it hangs on, so every
// list of every tool is known before the first one is written, and a record
// hanging on nothing the run extracts is left out and counted.
//
// A walk is no snapshot: a list that is moved between two requests can be
// missed, or met twice, and then it is written once.
func (e *Extractor) extractTodosOfProject(ctx context.Context, w *bundle.Writer, bucket int64, ids []int64) error {
	projectID := sourceID(bucket)
	tools := make([]*todoTool, 0, len(ids))
	byTool := map[int64]*todoTool{}
	met := map[int64]bool{}

	for _, id := range ids {
		tool := &todoTool{id: id}
		tools = append(tools, tool)
		byTool[id] = tool

		listed, err := getList[todoList](ctx, e.Source, fmt.Sprintf("buckets/%d/todosets/%d/todolists.json", bucket, id))
		if err != nil {
			return err
		}
		for _, found := range listed {
			carried, err := e.metTodoList(met, found, projectID)
			if err != nil {
				return err
			}
			if carried {
				tool.shown = append(tool.shown, found)
			}
		}
	}

	shelved, err := e.shelvedTodoLists(ctx, bucket)
	if err != nil {
		return err
	}
	// What Basecamp lists as a to-do list and no tool of the run holds is a
	// group of a list, which is known only once every list is.
	var mayBeGroups []todoList
	for _, found := range shelved {
		tool, mine := byTool[found.Parent.ID]
		if !mine {
			mayBeGroups = append(mayBeGroups, found)
			continue
		}
		carried, err := e.metTodoList(met, found, projectID)
		if err != nil {
			return err
		}
		if carried {
			tool.gone = append(tool.gone, found)
		}
	}

	carried := map[int64]bool{}
	for _, tool := range tools {
		for _, found := range tool.inFileOrder() {
			carried[found.ID] = true
		}
	}
	groupsGone := map[int64][]todoList{}
	for _, found := range mayBeGroups {
		if !carried[found.Parent.ID] {
			e.todosLeftOut++
			continue
		}
		taken, err := e.metTodoGroup(met, found, projectID)
		if err != nil {
			return err
		}
		if taken {
			groupsGone[found.Parent.ID] = append(groupsGone[found.Parent.ID], found)
		}
	}

	shelvedTodos, err := e.shelvedTodos(ctx, bucket)
	if err != nil {
		return err
	}
	placed := map[int64]bool{}

	for _, tool := range tools {
		for _, found := range tool.inFileOrder() {
			if err := e.writeTodoList(ctx, w, bucket, tool.id, found); err != nil {
				return err
			}
			groups, err := e.writeTodoGroups(ctx, w, bucket, tool.id, found.ID, groupsGone[found.ID], met, projectID)
			if err != nil {
				return err
			}
			if err := e.writeTodos(ctx, w, bucket, tool.id, found.ID, groups, shelvedTodos, placed); err != nil {
				return err
			}
		}
	}
	for _, found := range shelvedTodos {
		if !placed[found.ID] {
			e.todosLeftOut++
		}
	}
	return nil
}

// shelvedTodoLists are the lists of a project that are on the shelf or in the
// trash, whatever To-dos tool they belong to.
func (e *Extractor) shelvedTodoLists(ctx context.Context, bucket int64) ([]todoList, error) {
	var shelved []todoList
	for _, status := range []string{bundle.StatusArchived, bundle.StatusTrashed} {
		listed, err := getList[todoList](ctx, e.Source, fmt.Sprintf("projects/recordings.json?type=Todolist&bucket=%d&status=%s", bucket, status))
		if err != nil {
			return nil, err
		}
		shelved = append(shelved, listed...)
	}
	return shelved, nil
}

// metTodoList holds a list to what a bundle can carry, and passes over one the
// walk has met before.
func (e *Extractor) metTodoList(met map[int64]bool, found todoList, projectID string) (bool, error) {
	if met[found.ID] {
		return false, nil
	}
	met[found.ID] = true
	return e.met("to-do list", found.ID, found.Status, found.recording, "project", projectID)
}

// metTodoGroup holds a group to what a bundle can carry. A group knows the trash
// and no shelf, so a group the source has put on the shelf is left out and
// counted: a bundle has no word for it.
func (e *Extractor) metTodoGroup(met map[int64]bool, found todoList, projectID string) (bool, error) {
	if met[found.ID] {
		return false, nil
	}
	met[found.ID] = true
	return e.metAmong(todoStatuses, "group", found.ID, found.Status, found.recording, "project", projectID)
}

// writeTodoGroups adds the groups of one list: those in use as Basecamp lists
// them, then the ones it has thrown away. A group goes to the end of its list,
// so the order of the file is the order of the list.
//
// Nothing hangs on a group. The format has no word for a comment, a reaction or
// a file of one, so such a record would wait in Minka for ever: none is written
// and none is asked for.
func (e *Extractor) writeTodoGroups(ctx context.Context, w *bundle.Writer, bucket, tool, list int64, gone []todoList, met map[int64]bool, projectID string) ([]todoList, error) {
	listed, err := getList[todoList](ctx, e.Source, fmt.Sprintf("buckets/%d/todolists/%d/groups.json", bucket, list))
	if err != nil {
		return nil, err
	}
	groups := make([]todoList, 0, len(listed)+len(gone))
	for _, found := range listed {
		taken, err := e.metTodoGroup(met, found, projectID)
		if err != nil {
			return nil, err
		}
		if taken {
			groups = append(groups, found)
		}
	}
	groups = append(groups, gone...)

	for _, found := range groups {
		title, err := itemTitle("group", found.ID, found.Title)
		if err != nil {
			return nil, err
		}
		origin, err := e.record("group", found.ID, found.recording)
		if err != nil {
			return nil, err
		}
		err = w.Add(bundle.TodoGroups, bundle.TodoGroup{
			Record:          origin,
			ProjectSourceID: projectID,
			ToolSourceID:    sourceID(tool),
			ListSourceID:    sourceID(list),
			Title:           title,
			Status:          found.Status,
		})
		if err != nil {
			return nil, err
		}
	}
	return groups, nil
}

// writeTodoList adds a list with its text, its comments and its boosts.
func (e *Extractor) writeTodoList(ctx context.Context, w *bundle.Writer, bucket, tool int64, found todoList) error {
	title, err := itemTitle("to-do list", found.ID, found.Title)
	if err != nil {
		return err
	}
	origin, err := e.record("to-do list", found.ID, found.recording)
	if err != nil {
		return err
	}
	description, files, err := e.rewriteBody(found.Description, "", found.Files)
	if err != nil {
		return err
	}

	record := bundle.TodoList{
		Record:          origin,
		ProjectSourceID: sourceID(bucket),
		ToolSourceID:    sourceID(tool),
		Title:           title,
		Status:          found.Status,
	}
	// A list without a text has none.
	if description != "" {
		record.Description = &description
	}
	if err := w.Add(bundle.TodoLists, record); err != nil {
		return err
	}
	e.measure(bundle.TodoLists, description, LongestBodyRecord{Record: "to-do list", SourceID: origin.SourceID, In: "project", InSourceID: sourceID(bucket)})
	e.todoListsByStatus[found.Status]++
	if err := e.attachFiles(ctx, w, bundle.OwnerTodoList, origin, files); err != nil {
		return err
	}
	return e.writeDiscussion(ctx, w, bucket, bundle.ParentTodoList, bundle.TargetTodoList, found.ID, found.CommentsCount, found.BoostsCount)
}

// shelvedTodos are the to-dos of a project that are on the shelf or in the
// trash, whatever list or group they stand in.
func (e *Extractor) shelvedTodos(ctx context.Context, bucket int64) ([]todo, error) {
	var shelved []todo
	for _, status := range []string{bundle.StatusArchived, bundle.StatusTrashed} {
		listed, err := getList[todo](ctx, e.Source, fmt.Sprintf("projects/recordings.json?type=Todo&bucket=%d&status=%s", bucket, status))
		if err != nil {
			return nil, err
		}
		shelved = append(shelved, listed...)
	}
	return shelved, nil
}

// writeTodos adds the to-dos of one list: those standing in the list itself,
// then those of each of its groups in the order of the groups. A to-do goes to
// the end of its list or its group, so the order of the file is the order of the
// list: the to-dos that are open as Basecamp lists them, then the ones that are
// done, then the ones it has thrown away.
//
// A to-do that stands in neither a list nor a group of the bundle is left out and
// counted by whoever has walked every list.
func (e *Extractor) writeTodos(ctx context.Context, w *bundle.Writer, bucket, tool, list int64, groups []todoList, shelved []todo, placed map[int64]bool) error {
	for _, in := range append([]todoList{{ID: list}}, groups...) {
		group := &in.ID
		if in.ID == list {
			group = nil
		}
		found, err := e.todosIn(ctx, bucket, in.ID, shelved, placed)
		if err != nil {
			return err
		}
		for _, one := range found {
			if err := e.writeTodo(ctx, w, bucket, tool, list, group, one); err != nil {
				return err
			}
		}
	}
	return nil
}

// todosIn are the to-dos standing in one list or group, in the order they stand
// there: those that are open, then those that are done, then the ones the source
// has thrown away. Basecamp lists what is done apart from what is open.
//
// A walk is no snapshot: a to-do that is moved between two requests can be
// missed, or met twice, and then it is written once.
func (e *Extractor) todosIn(ctx context.Context, bucket, in int64, shelved []todo, placed map[int64]bool) ([]todo, error) {
	var found []todo
	for _, path := range []string{
		fmt.Sprintf("buckets/%d/todolists/%d/todos.json", bucket, in),
		fmt.Sprintf("buckets/%d/todolists/%d/todos.json?completed=true", bucket, in),
	} {
		listed, err := getList[todo](ctx, e.Source, path)
		if err != nil {
			return nil, err
		}
		found = append(found, listed...)
	}
	for _, one := range shelved {
		if one.Parent.ID == in {
			found = append(found, one)
		}
	}

	carried := make([]todo, 0, len(found))
	for _, one := range found {
		if placed[one.ID] {
			continue
		}
		placed[one.ID] = true
		taken, err := e.metAmong(todoStatuses, "to-do", one.ID, one.Status, one.recording, "project", sourceID(bucket))
		if err != nil {
			return nil, err
		}
		if taken {
			carried = append(carried, one)
		}
	}
	return carried, nil
}

// writeTodo adds a to-do with its notes, its days, the people on it and the tick
// the source put on it, and what hangs on it: its files, its comments and its
// boosts.
//
// Subtasks are not carried: the format has no word for them, so all that is left
// of them is how many there were. A to-do that had them is poorer in Minka than
// at the source, which is what the report says.
func (e *Extractor) writeTodo(ctx context.Context, w *bundle.Writer, bucket, tool, list int64, group *int64, found todo) error {
	title, err := itemTitle("to-do", found.ID, found.Title)
	if err != nil {
		return err
	}
	startsOn, dueOn, err := daysOfATodo(found)
	if err != nil {
		return err
	}
	origin, err := e.record("to-do", found.ID, found.recording)
	if err != nil {
		return err
	}
	completedAt, completedBy, err := e.completed(found.Completion)
	if err != nil {
		return err
	}
	if origin.UpdatedAt, err = touchedAtLeastWhenDone(found.ID, origin, completedAt); err != nil {
		return err
	}
	assignees, err := e.peopleOn(found.Assignees)
	if err != nil {
		return err
	}
	notified, err := e.peopleOn(found.NotifiedWhenDone)
	if err != nil {
		return err
	}
	notes, files, err := e.rewriteBody(found.Description, "", found.Files)
	if err != nil {
		return err
	}

	record := bundle.Todo{
		Record:          origin,
		ProjectSourceID: sourceID(bucket),
		ToolSourceID:    sourceID(tool),
		ListSourceID:    sourceID(list),
		Title:           title,
		StartsOn:        startsOn,
		DueOn:           dueOn,
		Assignees:       assignees,
		NotifyWhenDone:  notified,
		CompletedAt:     completedAt,
		CompletedBy:     completedBy,
		Status:          found.Status,
	}
	if group != nil {
		named := sourceID(*group)
		record.GroupSourceID = &named
	}
	// A to-do without notes has none.
	if notes != "" {
		record.Notes = &notes
	}
	if err := w.Add(bundle.Todos, record); err != nil {
		return err
	}
	e.measure(bundle.Todos, notes, LongestBodyRecord{Record: "to-do", SourceID: origin.SourceID, In: "project", InSourceID: sourceID(bucket)})
	e.todosByStatus[found.Status]++
	if completedAt == nil {
		e.todosNotDone++
	}
	if len(assignees) > 0 || len(notified) > 0 {
		e.todosWithPeople++
	}
	if found.SubtasksCount > 0 {
		e.todosWithSubtasks++
		e.subtasksLeftOut += found.SubtasksCount
	}
	if err := e.attachFiles(ctx, w, bundle.OwnerTodo, origin, files); err != nil {
		return err
	}
	return e.writeDiscussion(ctx, w, bucket, bundle.ParentTodo, bundle.TargetTodo, found.ID, found.CommentsCount, found.BoostsCount)
}

// peopleOn are the people a to-do names, in the order of their IDs and each
// once, however the source lists them: two runs over an unchanged account write
// the same bytes.
func (e *Extractor) peopleOn(named []person) ([]string, error) {
	sorted := append([]person(nil), named...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	taken := map[int64]bool{}
	ids := make([]string, 0, len(sorted))
	for i := range sorted {
		if taken[sorted[i].ID] {
			continue
		}
		taken[sorted[i].ID] = true
		reference, err := e.people.refer(&sorted[i], false)
		if err != nil {
			return nil, err
		}
		if reference != nil {
			ids = append(ids, *reference)
		}
	}
	return ids, nil
}

// daysOfATodo are the days a to-do runs, held to what Minka takes: a day of
// another shape than YYYY-MM-DD, and a start after the day the to-do is due, end
// the run, because the applier would refuse them too and a day nobody can read
// is a day nobody can put right afterwards.
func daysOfATodo(found todo) (startsOn, dueOn *string, err error) {
	for _, day := range []struct {
		field string
		value *string
	}{
		{"starts on", found.StartsOn},
		{"is due on", found.DueOn},
	} {
		if day.value == nil || *day.value == "" {
			continue
		}
		if _, aDate := aDay(*day.value); !aDate {
			return nil, nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp says that to-do %d %s a day of a shape that is none", found.ID, day.field)}
		}
	}
	startsOn, dueOn = found.StartsOn, found.DueOn
	if startsOn != nil && dueOn != nil && *startsOn > *dueOn {
		return nil, nil, &Error{Kind: API, Message: fmt.Sprintf("to-do %d starts after the day it is due, which Minka does not take", found.ID)}
	}
	return startsOn, dueOn, nil
}

// touchedAtLeastWhenDone is the moment a to-do was last touched, never earlier
// than the moment it was done. Minka reads a record whose updatedAt has not
// moved since the run before and passes it over whole, so a to-do whose
// completion is the newer of the two would never get its tick.
func touchedAtLeastWhenDone(id int64, origin bundle.Record, completedAt *string) (string, error) {
	if completedAt == nil {
		return origin.UpdatedAt, nil
	}
	done, err := time.Parse(time.RFC3339Nano, *completedAt)
	if err != nil {
		return "", &Error{Kind: API, Message: fmt.Sprintf("Basecamp says that to-do %d was done at something that is no time", id)}
	}
	touched, err := time.Parse(time.RFC3339Nano, origin.UpdatedAt)
	if err != nil {
		return "", &Error{Kind: API, Message: fmt.Sprintf("Basecamp says that to-do %d was updated at something that is no time", id)}
	}
	if done.After(touched) {
		return *completedAt, nil
	}
	return origin.UpdatedAt, nil
}
