package basecamp

import (
	"context"
	"fmt"

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

	for _, tool := range tools {
		for _, found := range tool.inFileOrder() {
			if err := e.writeTodoList(ctx, w, bucket, tool.id, found); err != nil {
				return err
			}
			if err := e.writeTodoGroups(ctx, w, bucket, tool.id, found.ID, groupsGone[found.ID], met, projectID); err != nil {
				return err
			}
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
func (e *Extractor) writeTodoGroups(ctx context.Context, w *bundle.Writer, bucket, tool, list int64, gone []todoList, met map[int64]bool, projectID string) error {
	listed, err := getList[todoList](ctx, e.Source, fmt.Sprintf("buckets/%d/todolists/%d/groups.json", bucket, list))
	if err != nil {
		return err
	}
	groups := make([]todoList, 0, len(listed)+len(gone))
	for _, found := range listed {
		taken, err := e.metTodoGroup(met, found, projectID)
		if err != nil {
			return err
		}
		if taken {
			groups = append(groups, found)
		}
	}
	groups = append(groups, gone...)

	for _, found := range groups {
		title, err := itemTitle("group", found.ID, found.Title)
		if err != nil {
			return err
		}
		origin, err := e.record("group", found.ID, found.recording)
		if err != nil {
			return err
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
			return err
		}
	}
	return nil
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
