package basecamp

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

// lostPlace is a list or a group the bundle does not carry. Whatever stood in it
// falls with it, and a walk of its own counts what that was.
type lostPlace struct {
	record string
	id     int64
	// aList tells a list from a group: a group holds to-dos alone.
	aList bool
}

// todoTool collects the lists of one To-dos tool.
type todoTool struct {
	id int64
	// gone are the lists on the shelf and in the trash, in the order Basecamp
	// lists them, and shown the lists in use as Basecamp shows them, the topmost
	// first.
	gone  []todoList
	shown []todoList
}

// inFileOrder are the lists of a tool in the order a bundle writes them.
func (t *todoTool) inFileOrder() []todoList {
	return inFileOrder(t.gone, t.shown)
}

// extractTodos adds the lists of every To-dos tool and what stands in them,
// project by project and in the order of the tools.
func (e *Extractor) extractTodos(ctx context.Context, w *bundle.Writer) error {
	for _, t := range e.talks {
		if len(t.todosets) == 0 {
			continue
		}
		if err := e.extractTodosOfProject(ctx, w, t); err != nil {
			return err
		}
	}
	return nil
}

// extractTodosOfProject adds the To-dos tools of one project.
//
// Basecamp lists the lists of a tool itself, and the archived and the trashed
// ones of a project in one list across all its To-dos tools, those that are
// switched off included. Such a record names only what it hangs on: a list the
// tool it stands in, a group its list. So what hangs on a To-dos tool of the
// project is a list and everything else a group, and every list is known before
// the first one is written.
//
// A walk is no snapshot: a list that is moved between two requests can be
// missed, or met twice, and then it is written once.
func (e *Extractor) extractTodosOfProject(ctx context.Context, w *bundle.Writer, t talk) error {
	bucket, projectID := t.bucket, sourceID(t.bucket)
	tools := make([]*todoTool, 0, len(t.todosets))
	byTool := map[int64]*todoTool{}
	met := map[int64]bool{}
	placed := map[int64]bool{}
	var lost []lostPlace

	for _, id := range t.todosets {
		tool := &todoTool{id: id}
		tools = append(tools, tool)
		byTool[id] = tool

		listed, err := getList[todoList](ctx, e.Source, fmt.Sprintf("buckets/%d/todosets/%d/todolists.json", bucket, id))
		if err != nil {
			return err
		}
		for _, found := range listed {
			if aSecondTime(met, found.ID) {
				continue
			}
			carried, err := e.met("to-do list", found.ID, found.Status, found.recording, "project", projectID)
			if err != nil {
				return err
			}
			if !carried {
				lost = append(lost, aListThatFell(found.ID))
				continue
			}
			tool.shown = append(tool.shown, found)
		}
	}

	shelved, err := e.shelvedTodoLists(ctx, bucket)
	if err != nil {
		return err
	}
	var groups []todoList
	for _, found := range shelved {
		if !t.everyTodoset[found.Parent.ID] {
			groups = append(groups, found)
			continue
		}
		if aSecondTime(met, found.ID) {
			continue
		}
		tool, extracted := byTool[found.Parent.ID]
		if !extracted {
			lost = append(lost, aListThatFell(found.ID))
			continue
		}
		carried, err := e.met("to-do list", found.ID, found.Status, found.recording, "project", projectID)
		if err != nil {
			return err
		}
		if !carried {
			lost = append(lost, aListThatFell(found.ID))
			continue
		}
		tool.gone = append(tool.gone, found)
	}

	carries := map[int64]bool{}
	for _, tool := range tools {
		for _, found := range tool.inFileOrder() {
			carries[found.ID] = true
		}
	}
	groupsGone := map[int64][]todoList{}
	var strays []todoList
	for _, found := range groups {
		if !carries[found.Parent.ID] {
			strays = append(strays, found)
			continue
		}
		if aSecondTime(met, found.ID) {
			continue
		}
		taken, err := e.metAmong(todoStatuses, "group", found.ID, found.Status, found.recording, "project", projectID)
		if err != nil {
			return err
		}
		if !taken {
			lost = append(lost, aGroupThatFell(found.ID))
			continue
		}
		groupsGone[found.Parent.ID] = append(groupsGone[found.Parent.ID], found)
	}

	shelvedTodos, err := e.shelvedTodos(ctx, bucket)
	if err != nil {
		return err
	}

	for _, tool := range tools {
		for _, found := range tool.inFileOrder() {
			if err := e.writeTodoList(ctx, w, bucket, tool.id, found); err != nil {
				return err
			}
			written, fell, err := e.writeTodoGroups(ctx, w, bucket, tool.id, found.ID, groupsGone[found.ID], met, projectID)
			if err != nil {
				return err
			}
			lost = append(lost, fell...)
			if err := e.writeTodos(ctx, w, bucket, tool.id, found.ID, written, shelvedTodos, placed); err != nil {
				return err
			}
		}
	}

	if err := e.countWhatFellWithAPlace(ctx, bucket, lost, strays, shelvedTodos, met, placed); err != nil {
		return err
	}
	// A to-do of a place the walk never saw at all, such as a list in use of a
	// tool that is switched off.
	for _, found := range shelvedTodos {
		if !placed[found.ID] {
			placed[found.ID] = true
			e.todosLeftOut.Todos++
		}
	}
	return nil
}

// aSecondTime tells a record the walk has met before from a new one, and
// remembers the new one. A walk is no snapshot: Basecamp can list a record twice,
// and a bundle writes it once.
func aSecondTime(met map[int64]bool, id int64) bool {
	if met[id] {
		return true
	}
	met[id] = true
	return false
}

func aListThatFell(id int64) lostPlace  { return lostPlace{record: "to-do list", id: id, aList: true} }
func aGroupThatFell(id int64) lostPlace { return lostPlace{record: "group", id: id} }

// countWhatFellWithAPlace counts the groups and the to-dos that stood in a list
// or a group the bundle does not carry, and names the place they fell with.
//
// They are asked for although none of them is written: what is left out is
// counted, and a record that falls with its place would otherwise be gone
// without a number — no run could say how much of the source a bundle holds. A
// group of such a list is a place of its own, so the walk grows while it goes.
func (e *Extractor) countWhatFellWithAPlace(ctx context.Context, bucket int64, lost []lostPlace, strays []todoList, shelvedTodos []todo, met, placed map[int64]bool) error {
	// A group of the archived and trashed records of the project falls with its
	// list where the walk reaches that list, and stands for itself where the walk
	// never sees it. Every place that is a list is known here, because the walk
	// itself only ever finds groups.
	inTheList := map[int64][]todoList{}
	lists := map[int64]bool{}
	for _, place := range lost {
		if place.aList {
			lists[place.id] = true
		}
	}
	for _, found := range strays {
		if lists[found.Parent.ID] {
			inTheList[found.Parent.ID] = append(inTheList[found.Parent.ID], found)
			continue
		}
		if !aSecondTime(met, found.ID) {
			lost = append(lost, aGroupThatFell(found.ID))
		}
	}

	for i := 0; i < len(lost); i++ {
		place := lost[i]
		fell := TodoPlaceLeftOut{Record: place.record, SourceID: sourceID(place.id)}

		if place.aList {
			e.todosLeftOut.Lists++
			inIt, _, err := listedUnlessItIsGone[todoList](ctx, e.Source, fmt.Sprintf("buckets/%d/todolists/%d/groups.json", bucket, place.id), false)
			if err != nil {
				return err
			}
			for _, found := range append(inIt, inTheList[place.id]...) {
				if aSecondTime(met, found.ID) {
					continue
				}
				fell.Groups++
				lost = append(lost, aGroupThatFell(found.ID))
			}
		} else {
			e.todosLeftOut.Groups++
		}

		for _, path := range todosOfAPlace(bucket, place.id) {
			inIt, _, err := listedUnlessItIsGone[todo](ctx, e.Source, path, false)
			if err != nil {
				return err
			}
			for _, one := range inIt {
				if aSecondTime(placed, one.ID) {
					continue
				}
				fell.Todos++
				e.todosLeftOut.Todos++
			}
		}
		for _, one := range shelvedTodos {
			if one.Parent.ID == place.id && !aSecondTime(placed, one.ID) {
				fell.Todos++
				e.todosLeftOut.Todos++
			}
		}

		// A place that took nothing with it says nothing worth reading.
		if fell.Groups > 0 || fell.Todos > 0 {
			e.todoPlacesLeftOut = append(e.todoPlacesLeftOut, fell)
		}
	}
	return nil
}

// todosOfAPlace are the two lists Basecamp keeps the to-dos of a list or a group
// in: what is open and what is done.
func todosOfAPlace(bucket, in int64) []string {
	return []string{
		fmt.Sprintf("buckets/%d/todolists/%d/todos.json", bucket, in),
		fmt.Sprintf("buckets/%d/todolists/%d/todos.json?completed=true", bucket, in),
	}
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

// writeTodoGroups adds the groups of one list: those in use as Basecamp lists
// them, then the ones it has thrown away. A group goes to the end of its list,
// so the order of the file is the order of the list. It hands back the groups it
// wrote, for the to-dos standing in them, and the ones it left out, for whoever
// counts what fell with them.
//
// A group knows the trash and no shelf, so one the source has put on the shelf is
// a word a bundle has none for. Nothing else hangs on a group: the format has no
// word for a comment, a reaction or a file of one, such a record would wait in
// Minka for ever, so none is written and none is asked for.
func (e *Extractor) writeTodoGroups(ctx context.Context, w *bundle.Writer, bucket, tool, list int64, gone []todoList, met map[int64]bool, projectID string) ([]todoList, []lostPlace, error) {
	listed, err := getList[todoList](ctx, e.Source, fmt.Sprintf("buckets/%d/todolists/%d/groups.json", bucket, list))
	if err != nil {
		return nil, nil, err
	}
	groups := make([]todoList, 0, len(listed)+len(gone))
	var fell []lostPlace
	for _, found := range listed {
		if aSecondTime(met, found.ID) {
			continue
		}
		taken, err := e.metAmong(todoStatuses, "group", found.ID, found.Status, found.recording, "project", projectID)
		if err != nil {
			return nil, nil, err
		}
		if !taken {
			fell = append(fell, aGroupThatFell(found.ID))
			continue
		}
		groups = append(groups, found)
	}
	groups = append(groups, gone...)

	for _, found := range groups {
		title, err := itemTitle("group", found.ID, found.Title)
		if err != nil {
			return nil, nil, err
		}
		origin, err := e.record("group", found.ID, found.recording)
		if err != nil {
			return nil, nil, err
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
			return nil, nil, err
		}
	}
	return groups, fell, nil
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
		Record:           origin,
		ProjectSourceID:  sourceID(bucket),
		ToolSourceID:     sourceID(tool),
		Title:            title,
		Status:           found.Status,
		VisibleToClients: visibleToClientsOf(found.recording),
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
// A to-do that stands in neither a list nor a group of the bundle is left where
// it is; the walk over the project counts it once every list is known.
func (e *Extractor) writeTodos(ctx context.Context, w *bundle.Writer, bucket, tool, list int64, groups []todoList, shelved []todo, placed map[int64]bool) error {
	// The list itself takes no group, and every group of it names itself.
	places := make([]*int64, 0, len(groups)+1)
	places = append(places, nil)
	for i := range groups {
		places = append(places, &groups[i].ID)
	}

	for _, group := range places {
		in := list
		if group != nil {
			in = *group
		}
		found, err := e.todosIn(ctx, bucket, in, shelved, placed)
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
	for _, path := range todosOfAPlace(bucket, in) {
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
		if aSecondTime(placed, one.ID) {
			continue
		}
		taken, err := e.metAmong(todoStatuses, "to-do", one.ID, one.Status, one.recording, "project", sourceID(bucket))
		if err != nil {
			return nil, err
		}
		if !taken {
			e.todosLeftOut.Todos++
			continue
		}
		carried = append(carried, one)
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
	completedAt, completedBy, err := e.completed("to-do", found.ID, found.Completion)
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
	startsOn, dueOn = aDayOrNone(found.StartsOn), aDayOrNone(found.DueOn)
	for _, day := range []struct {
		field string
		value *string
	}{
		{"starts on", startsOn},
		{"is due on", dueOn},
	} {
		if day.value == nil {
			continue
		}
		if _, aDate := aDay(*day.value); !aDate {
			return nil, nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp says that to-do %d %s %s", found.ID, day.field, noDay(*day.value))}
		}
	}
	if startsOn != nil && dueOn != nil && *startsOn > *dueOn {
		return nil, nil, &Error{Kind: API, Message: fmt.Sprintf("to-do %d starts after the day it is due, which Minka does not take", found.ID)}
	}
	return startsOn, dueOn, nil
}

// aDayOrNone is the day a field holds, and nothing where it holds none: a source
// that writes an empty field rather than leaving it out means the same thing, and
// a bundle says it one way.
func aDayOrNone(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return value
}

// touchedAtLeastWhenDone is the moment a to-do was last touched, never earlier
// than the moment it was done. Minka reads a record whose updatedAt has not
// moved since the run before and passes it over whole, so a to-do whose
// completion is the newer of the two would never get its tick.
func touchedAtLeastWhenDone(id int64, origin bundle.Record, completedAt *string) (string, error) {
	if completedAt == nil {
		return origin.UpdatedAt, nil
	}
	what := fmt.Sprintf("to-do %d", id)
	done, err := momentOf(what, "completed", *completedAt)
	if err != nil {
		return "", err
	}
	touched, err := momentOf(what, "updated", origin.UpdatedAt)
	if err != nil {
		return "", err
	}
	if done.After(touched) {
		return *completedAt, nil
	}
	return origin.UpdatedAt, nil
}
