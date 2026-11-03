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
				if card.ShortID != "" && card.ShortID == identifier {
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

// stepResources names everybody on a step, somebody who left the project
// included, as Minka reads them without asking about the project. A card step
// reads as its card stands for the reader.
func (s *Server) stepResources(card *Card, reader *Person) []any {
	list := []any{}
	for _, step := range card.Steps {
		if step.CardID != "" && !s.KnowsNoCardSteps {
			linked, title, done := s.linkedCard(step.CardID, reader)
			list = append(list, map[string]any{"id": step.ID, "title": title, "completed": done, "assignees": []any{}, "dueOn": nil, "card": linked})
			continue
		}
		assignees := []any{}
		for _, id := range step.AssigneeIDs {
			assignees = append(assignees, s.personReference(id))
		}
		resource := map[string]any{"id": step.ID, "title": step.Title, "completed": step.Completed, "assignees": assignees, "dueOn": step.DueOn}
		if !s.KnowsNoCardSteps {
			resource["card"] = nil
		}
		list = append(list, resource)
	}
	return list
}

// completedStepCount counts the ticked steps in words and the card steps whose
// card the reader sees in Done.
func (s *Server) completedStepCount(card *Card, reader *Person) int {
	completed := 0
	for _, step := range card.Steps {
		if step.CardID == "" {
			if step.Completed {
				completed++
			}
			continue
		}
		if _, _, done := s.linkedCard(step.CardID, reader); done {
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
// with nothing written, unless nothing may be written on the card at all.
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
	if !slices.ContainsFunc(card.Steps, func(step *Step) bool { return step.CardID == childID }) {
		card.Steps = append(card.Steps, &Step{ID: newID(), CardID: childID})
		writeJSON(w, http.StatusCreated, s.cardResource(board, column, card, person))
		return
	}
	writeJSON(w, http.StatusOK, s.cardResource(board, column, card, person))
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
