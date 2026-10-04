package fakeapi

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
)

// datedItem is a to-do, a card or a step due on a day, as a span lists it
// beside the events. Steps stand only on "My calendar", which also names the
// project.
type datedItem struct {
	id, kind, projectID, toolID string
	cardID                      string // the card of a step, empty for the others
	title, dueOn, placeTitle    string // the place is the list of a to-do, the board of a card, the card of a step
	appURL                      string // a step opens its card
	project                     *Project
}

func (item datedItem) resource() map[string]any {
	resource := map[string]any{
		"id": item.id, "kind": item.kind, "projectId": item.projectID, "toolId": item.toolID, "cardId": nullable(item.cardID),
		"title": item.title, "dueOn": item.dueOn, "placeTitle": item.placeTitle, "appUrl": item.appURL,
	}
	if item.project != nil {
		resource["project"] = map[string]any{"id": item.project.ID, "name": item.project.Name}
	}
	return resource
}

// byDueDayAndTitle is the order of the dated items, alike in a calendar's span
// and on My calendar: the earliest first, then by title with only the ASCII
// letters lowercased and compared byte by byte, then by identifier.
func byDueDayAndTitle(one, other datedItem) int {
	return cmp.Or(strings.Compare(one.dueOn, other.dueOn), strings.Compare(asciiLower(one.title), asciiLower(other.title)), strings.Compare(one.id, other.id))
}

// asciiLower lowercases A to Z and leaves every other byte as it is, as
// PostgreSQL's lower() under the collation "C" and PHP's strtolower() do: an
// "Ä" stays an "Ä".
func asciiLower(text string) string {
	lowered := []byte(text)
	for i, b := range lowered {
		if 'A' <= b && b <= 'Z' {
			lowered[i] = b + ('a' - 'A')
		}
	}
	return string(lowered)
}

// openCard is a card still to be done, the one rule by which a calendar's
// span, My calendar and the cards one is assigned to leave cards out: one in
// Done, and one completed while it stands in another column.
func openCard(column *Column, card *Card) bool {
	return column.Type != "done" && card.CompletedAt == nil
}

func dueIn(dueOn *string, from, to string) bool {
	return dueOn != nil && *dueOn >= from && *dueOn <= to
}

// datedItemsOfProject are the open to-dos and cards of a project due between
// two days, whoever they belong to, as a calendar of it shows them. What is
// done, in the trash, in the archive, or in a list, group or column that is
// not in use is left out, and so is a to-do of a list hidden from a client.
// Whether the project is archived does not matter: a span is read there too.
func (s *Server) datedItemsOfProject(projectID, from, to string, person *Person) []datedItem {
	var items []datedItem
	for _, tool := range s.readableTodosTools() {
		if tool.ProjectID != projectID {
			continue
		}
		for _, list := range tool.Lists {
			if list.Trashed || list.Archived || !s.showsTheList(list, person) {
				continue
			}
			for _, todo := range countedTodos(list) {
				if todo.CompletedAt == nil && dueIn(todo.DueOn, from, to) {
					items = append(items, s.datedTodo(tool, list, todo, nil))
				}
			}
		}
	}
	for _, board := range s.Boards {
		if board.ProjectID != projectID {
			continue
		}
		for _, column := range board.Columns {
			for _, card := range column.Cards {
				if openCard(column, card) && dueIn(card.DueOn, from, to) {
					items = append(items, s.datedCard(board, card, nil))
				}
			}
		}
	}
	slices.SortFunc(items, byDueDayAndTitle)
	return items
}

func (s *Server) datedTodo(tool *TodosTool, list *TodoList, todo *Todo, project *Project) datedItem {
	return datedItem{
		id: todo.ID, kind: "todo", projectID: tool.ProjectID, toolID: tool.ID, title: todo.Title, dueOn: *todo.DueOn,
		placeTitle: list.Title, appURL: s.todoURL(tool, todo), project: project,
	}
}

func (s *Server) datedCard(board *Board, card *Card, project *Project) datedItem {
	return datedItem{
		id: card.ID, kind: "card", projectID: board.ProjectID, toolID: board.ID, title: card.Title, dueOn: *card.DueOn,
		placeTitle: board.Title, appURL: s.cardURL(board, card), project: project,
	}
}

// myEvents is "My calendar" between two days of the reader: the events they
// take part in and the open to-dos, cards and steps assigned to them that are
// due, across the projects they are a member of and that are not archived.
// What a calendar chose to show does not matter here.
func (s *Server) myEvents(w http.ResponseWriter, r *http.Request, person *Person) {
	from, to, ok := readSpan(w, r)
	if !ok {
		return
	}
	zoneName, zone := s.zoneOf(person)

	type inSpan struct {
		tool        *CalendarTool
		event       *CalendarEvent
		occurrences []eventTime
	}
	var listed []inSpan
	for _, tool := range s.Calendars {
		if project := s.visibleProject(tool.ProjectID, person); project == nil || project.Archived || s.visibleCalendar(tool.ID, person) == nil {
			continue
		}
		for _, event := range tool.Events {
			if !s.seesEvent(tool, event, person) || !slices.Contains(event.ParticipantIDs, person.ID) {
				continue
			}
			if occurrences := occurrencesBetween(event, from, to, zone); len(occurrences) > 0 {
				listed = append(listed, inSpan{tool, event, occurrences})
			}
		}
	}
	// The server reads the events in the order they were added and then
	// orders them by their first occurrence, keeping that order among equals.
	slices.SortStableFunc(listed, func(a, b inSpan) int {
		return cmp.Or(a.event.CreatedAt.Compare(b.event.CreatedAt), strings.Compare(a.event.ID, b.event.ID))
	})
	slices.SortStableFunc(listed, func(a, b inSpan) int {
		return startOf(a.occurrences[0]).Compare(startOf(b.occurrences[0]))
	})
	events := []any{}
	for _, one := range listed {
		line := s.eventLineResource(one.tool, one.event, one.occurrences)
		line["toolId"] = one.tool.ID
		line["project"] = s.projectReference(one.tool.ProjectID)
		events = append(events, line)
	}

	datedItems := []any{}
	for _, item := range s.myDatedItems(from, to, person) {
		datedItems = append(datedItems, item.resource())
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "timeZone": zoneName, "events": events, "datedItems": datedItems})
}

// myDatedItems are the open to-dos, cards and steps assigned to the person
// and due between two days. A to-do of which only a subtask is theirs stays
// out, and a subtask is never an entry.
func (s *Server) myDatedItems(from, to string, person *Person) []datedItem {
	var items []datedItem
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
				if todo.CompletedAt == nil && slices.Contains(todo.AssigneeIDs, person.ID) && dueIn(todo.DueOn, from, to) {
					items = append(items, s.datedTodo(tool, list, todo, project))
				}
			}
		}
	}
	for _, board := range s.Boards {
		project := s.visibleProject(board.ProjectID, person)
		if project == nil || project.Archived {
			continue
		}
		for _, column := range board.Columns {
			for _, card := range column.Cards {
				// The steps of a card no longer to be done go with it.
				if !openCard(column, card) {
					continue
				}
				if slices.Contains(card.AssigneeIDs, person.ID) && dueIn(card.DueOn, from, to) {
					items = append(items, s.datedCard(board, card, project))
				}
				for _, step := range card.Steps {
					if !step.Completed && slices.Contains(step.AssigneeIDs, person.ID) && dueIn(step.DueOn, from, to) {
						items = append(items, datedItem{
							id: step.ID, kind: "step", projectID: board.ProjectID, toolID: board.ID, cardID: card.ID, title: step.Title,
							dueOn: *step.DueOn, placeTitle: card.Title, appURL: s.cardURL(board, card), project: project,
						})
					}
				}
			}
		}
	}
	slices.SortFunc(items, byDueDayAndTitle)
	return items
}
