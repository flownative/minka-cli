package fakeapi

import (
	"html"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

// cardIDOfTheBody reads the identifier a body names a card by as Minka reads
// it there: an ID in any case, as in the address. What is no ID may still be
// a short identifier.
func cardIDOfTheBody(named string) (string, bool) {
	id := strings.ToLower(named)
	return id, uuidPattern.MatchString(id)
}

// seenCard finds a card the reader may see, on its board or in the archive of
// its board; a card in the trash is seen by nobody.
func (s *Server) seenCard(id string, reader *Person) (*Board, *Column, *Card, bool) {
	if board, column, card := s.visibleCard(id, reader); card != nil {
		return board, column, card, false
	}
	board, column, card := s.visibleArchivedCard(id, reader)
	return board, column, card, card != nil
}

// seenCardByShortIdentifier finds a card the reader may see by the name people
// call it by, in any case, on its board or in its archive.
func (s *Server) seenCardByShortIdentifier(named string, reader *Person) string {
	identifier := strings.ToUpper(named)
	for _, board := range s.Boards {
		if s.visibleProject(board.ProjectID, reader) == nil {
			continue
		}
		for _, column := range board.Columns {
			for _, card := range column.Cards {
				if goesBy(board, card, identifier) {
					return card.ID
				}
			}
		}
	}
	for _, archived := range s.Archive {
		if archived.Card.ShortID != "" && archived.Card.ShortID == identifier {
			if _, _, card, _ := s.seenCard(archived.Card.ID, reader); card != nil {
				return card.ID
			}
		}
	}
	return ""
}

// goesBy says whether a card goes by a short identifier in capitals: its own,
// or its suffix under a former prefix of its board.
func goesBy(board *Board, card *Card, identifier string) bool {
	if card.ShortID == "" {
		return false
	}
	_, suffix, _ := strings.Cut(card.ShortID, "-")
	return card.ShortID == identifier || slices.ContainsFunc(board.FormerPrefixes, func(prefix string) bool { return prefix+"-"+suffix == identifier })
}

// linkedCard is a card at either end of a card step as the reader sees it: in
// full, or only as a card where the reader may not see it. An archived card
// names nobody, as on the board.
func (s *Server) linkedCard(id string, reader *Person) (resource map[string]any, title string, done bool) {
	board, column, card, archived := s.seenCard(id, reader)
	if card == nil {
		return map[string]any{"kind": "card"}, "", false
	}
	assignees := []any{}
	if !archived {
		assignees = s.assigneeReferences(card)
	}
	return map[string]any{
		"kind": "card", "id": card.ID, "shortId": nullable(card.ShortID), "title": card.Title,
		"projectId": board.ProjectID, "boardId": board.ID,
		"column":      map[string]any{"id": column.ID, "type": column.Type, "title": columnTitle(column), "customTitle": column.CustomTitle},
		"columnColor": shownColor(column), "assignees": assignees, "appUrl": s.cardURL(board, card),
	}, card.Title, column.Type == "done"
}

// linkedTodo is the to-do of a to-do step as the reader sees it: in full, or
// only as a to-do where the reader may not see it or it lies in the trash.
func (s *Server) linkedTodo(id string, reader *Person) (resource map[string]any, title string, done bool) {
	tool, list, todo := s.visibleTodo(id, reader)
	if todo == nil {
		return map[string]any{"kind": "todo"}, "", false
	}
	return map[string]any{
		"kind": "todo", "id": todo.ID, "title": todo.Title, "projectId": tool.ProjectID, "toolId": tool.ID,
		"listId": list.ID, "completed": todo.CompletedAt != nil, "appUrl": s.todoURL(tool, todo),
	}, todo.Title, todo.CompletedAt != nil
}

// stepResources names everybody on a step, somebody who left the project
// included, as Minka reads them without asking about the project. A card step
// reads as its card stands for the reader, a to-do step as its to-do does.
func (s *Server) stepResources(card *Card, reader *Person) []any {
	list := []any{}
	for _, step := range card.Steps {
		assignees := []any{}
		for _, id := range step.AssigneeIDs {
			assignees = append(assignees, s.personReference(id))
		}
		resource := map[string]any{"id": step.ID, "title": step.Title, "completed": step.Completed, "assignees": assignees, "dueOn": step.DueOn}
		if !s.KnowsNoCardSteps {
			resource["card"] = nil
		}
		if !s.KnowsNoCardSteps && !s.KnowsNoTodoSteps {
			resource["todo"] = nil
		}
		switch {
		case step.CardID != "" && !s.KnowsNoCardSteps:
			resource["card"], resource["title"], resource["completed"] = s.linkedCard(step.CardID, reader)
			resource["assignees"], resource["dueOn"] = []any{}, nil
		case step.TodoID != "" && !s.KnowsNoCardSteps && !s.KnowsNoTodoSteps:
			resource["todo"], resource["title"], resource["completed"] = s.linkedTodo(step.TodoID, reader)
			resource["assignees"], resource["dueOn"] = []any{}, nil
		}
		list = append(list, resource)
	}
	return list
}

// completedStepCount counts the ticked steps in words, the card steps whose
// card the reader sees in Done and the to-do steps whose to-do the reader
// sees done.
func (s *Server) completedStepCount(card *Card, reader *Person) int {
	completed := 0
	for _, step := range card.Steps {
		done := step.Completed
		switch {
		case step.CardID != "":
			_, _, done = s.linkedCard(step.CardID, reader)
		case step.TodoID != "":
			_, _, done = s.linkedTodo(step.TodoID, reader)
		}
		if done {
			completed++
		}
	}
	return completed
}

// parentsOf lists the cards this card is a step of, only those the reader
// sees.
func (s *Server) parentsOf(card *Card, reader *Person) []any {
	var candidates []*Card
	for _, board := range s.Boards {
		for _, column := range board.Columns {
			candidates = append(candidates, column.Cards...)
		}
	}
	for _, archived := range s.Archive {
		candidates = append(candidates, archived.Card)
	}
	parents := []any{}
	for _, parent := range candidates {
		if !slices.ContainsFunc(parent.Steps, func(step *Step) bool { return step.CardID == card.ID }) {
			continue
		}
		if linked, _, _ := s.linkedCard(parent.ID, reader); linked["id"] != nil {
			parents = append(parents, linked)
		}
	}
	return parents
}

// addCardStep makes another card a step at the end of the list. Words beside
// the card are refused first, then the shape of the name, then a card the
// person may not see or the card itself; a card that is a step already is 200
// with that step and nothing written, unless nothing may be written on the
// card at all.
func (s *Server) addCardStep(w http.ResponseWriter, board *Board, column *Column, card *Card, archived bool, fields map[string]any, person *Person) {
	if _, namesTitle := fields["title"]; namesTitle {
		unprocessable(w, "stepTitleAndCard", "A step is either a title or a card, not both.")
		return
	}
	named, ok := stringField(w, fields, "cardId")
	if !ok {
		return
	}
	value := phpTrim(deref(named))
	childID := ""
	if id, isID := cardIDOfTheBody(value); isID {
		if _, _, child, _ := s.seenCard(id, person); child != nil {
			childID = child.ID
		}
	} else if value != "" {
		childID = s.seenCardByShortIdentifier(value, person)
	}
	if childID == "" {
		unprocessable(w, "stepCardUnknown", "No card you may see goes by that identifier.")
		return
	}
	if childID == card.ID {
		unprocessable(w, "stepCardIsThisCard", "A card cannot be a step of itself.")
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	if at := slices.IndexFunc(card.Steps, func(step *Step) bool { return step.CardID == childID }); at >= 0 {
		s.answerAStep(w, http.StatusOK, board, column, card, card.Steps[at], person)
		return
	}
	step := &Step{ID: newID(), CardID: childID}
	card.Steps = append(card.Steps, step)
	s.answerAStep(w, http.StatusCreated, board, column, card, step, person)
}

// addTodoStep makes a to-do a step at the end of the list. Words beside the
// to-do are refused first, then the shape of the name, then a to-do the person
// may not see; a to-do that is a step already is 200 with that step and
// nothing written, unless nothing may be written on the card at all. The
// to-do may be of any project the person sees it in.
func (s *Server) addTodoStep(w http.ResponseWriter, board *Board, column *Column, card *Card, archived bool, fields map[string]any, person *Person) {
	if _, namesTitle := fields["title"]; namesTitle {
		unprocessable(w, "stepTitleAndTodo", "A step is either a title or a to-do, not both.")
		return
	}
	named, ok := stringField(w, fields, "todoId")
	if !ok {
		return
	}
	_, _, todo := s.visibleTodo(strings.ToLower(phpTrim(deref(named))), person)
	if todo == nil {
		unprocessable(w, "stepTodoUnknown", "No to-do you may see goes by that identifier.")
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	if at := slices.IndexFunc(card.Steps, func(step *Step) bool { return step.TodoID == todo.ID }); at >= 0 {
		s.answerAStep(w, http.StatusOK, board, column, card, card.Steps[at], person)
		return
	}
	step := &Step{ID: newID(), TodoID: todo.ID}
	card.Steps = append(card.Steps, step)
	s.answerAStep(w, http.StatusCreated, board, column, card, step, person)
}

// makeStepIntoCard adds a card in Triage from the words, people and due day of
// a step and makes the step point at it, in its place. The body is never
// read. A card that takes no write refuses before a step that is not on it
// is 404; whatever is refused leaves everything as it was.
func (s *Server) makeStepIntoCard(w http.ResponseWriter, r *http.Request, person *Person) {
	if s.KnowsNoCardSteps {
		unknownAddress(w)
		return
	}
	board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person)
	stepID := r.PathValue("stepId")
	if card == nil || !uuidPattern.MatchString(stepID) {
		notFound(w)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	index := slices.IndexFunc(card.Steps, func(step *Step) bool { return step.ID == stepID })
	if index < 0 {
		notFound(w)
		return
	}
	step := card.Steps[index]
	if step.CardID != "" {
		unprocessable(w, "stepIsACard", "This step is a card already.")
		return
	}
	if step.TodoID != "" {
		unprocessable(w, "stepIsATodo", "This step is a to-do, which does not become a card.")
		return
	}
	if hasInvisibleCharacters(phpTrim(harmlessStripped(step.Title))) {
		unprocessable(w, "cardTitleHasInvisibleCharacters", "The title of a card must not contain invisible format characters such as direction overrides, isolates or tag characters; the zero-width joiner and non-joiner are fine.")
		return
	}
	words := phpTrim(step.Title)
	title, description := words, ""
	// Minka cuts between graphemes; the fake cuts between code points, which
	// is the same for every text without combining marks.
	if utf8.RuneCountInString(words) > maxTitleLength {
		title = phpTrim(string([]rune(words)[:maxTitleLength-1])) + "…"
		description = "<p>" + html.EscapeString(words) + "</p>"
	}
	if !validTitle(w, "card", "a card", title) {
		return
	}
	project := s.projectOf(board)
	for _, id := range step.AssigneeIDs {
		if !slices.Contains(project.MemberIDs, id) {
			notAProjectMember(w)
			return
		}
	}
	var triage *Column
	for _, column := range board.Columns {
		if column.Type == "triage" {
			triage = column
		}
	}

	made := &Card{ID: newID(), Title: title, Description: description, CreatedAt: s.now, CreatorID: person.ID, DueOn: step.DueOn}
	made.subscribe(person.ID)
	for _, id := range step.AssigneeIDs {
		made.AssigneeIDs = append(made.AssigneeIDs, id)
		made.subscribe(id)
	}
	triage.Cards = append([]*Card{made}, triage.Cards...)
	card.Steps[index] = &Step{ID: step.ID, CardID: made.ID}

	w.Header().Set("Location", s.URL+"/api/v1/cards/"+made.ID)
	writeJSON(w, http.StatusCreated, s.cardResource(board, triage, made, person))
}
