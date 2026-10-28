package fakeapi

import (
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// maxConventionsLength is the most characters the conventions of a board
// hold, counted after the line breaks are made "\n".
const maxConventionsLength = 20000

// ConventionsVersion is one saved text on how the team works on a board.
type ConventionsVersion struct {
	Text     string // Markdown, "" for an emptied version
	At       time.Time
	WriterID string
}

func (s *Server) registerConventionsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/v1/boards/{boardId}/conventions", s.authenticated(s.writeConventions))
}

// conventionsResource is the version in force, nil while nobody wrote one.
func conventionsResource(board *Board, s *Server) any {
	count := len(board.Conventions)
	if count == 0 {
		return nil
	}
	current := board.Conventions[count-1]
	writer := s.People[current.WriterID]
	return map[string]any{
		"text": current.Text, "updatedAt": timestamp(current.At), "version": count,
		"updatedBy": map[string]any{"id": writer.ID, "name": writer.Name, "kind": kindOf(writer)},
	}
}

// mayWriteConventions says whether a person writes the conventions of a
// board: a human owner or admin of the organization who is on the project's
// team. An agent never is, whatever role it was given.
func (s *Server) mayWriteConventions(board *Board, person *Person) bool {
	if kindOf(person) != "human" || !s.seesOnTheTeam(board, person) {
		return false
	}
	return person.OrganizationRole == "owner" || person.OrganizationRole == "admin"
}

// writeConventions saves a new version of how the team works on a board, in
// the order of the controller: the board and a client (404), the writer (403)
// before the body is read at all, the text (422: control characters, then
// invisible ones, then the length), then the text in force,
// which writes nothing; an archived project is refused only by a write.
// It answers the version in force, and 204 while there is none.
func (s *Server) writeConventions(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	if !s.mayWriteConventions(board, person) {
		writeJSON(w, http.StatusForbidden, errorBody("notAllowedToWriteConventions", "Only a human owner or admin of the organization on the project's team writes the conventions of a board."))
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	text, ok := stringField(w, fields, "text")
	if !ok {
		return
	}
	if text == nil {
		unprocessable(w, "conventionsMissing", "The body names no text; an empty text clears the conventions.")
		return
	}
	normalized := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(*text)
	invalid := "The conventions are too long or hold control characters other than line breaks and tabs."
	switch {
	case !utf8.ValidString(*text) || strings.ContainsFunc(*text, func(r rune) bool { return unicode.Is(unicode.Cc, r) && r != '\t' && r != '\n' && r != '\r' }):
		unprocessable(w, "conventionsInvalid", invalid)
		return
	case hasInvisibleCharacters(*text):
		unprocessable(w, "conventionsHaveInvisibleCharacters", "The conventions must not hold invisible format characters such as direction marks, tags or zero-width spaces; the zero-width joiner and non-joiner are fine.")
		return
	case utf8.RuneCountInString(normalized) > maxConventionsLength:
		unprocessable(w, "conventionsInvalid", invalid)
		return
	}

	inForce := ""
	if count := len(board.Conventions); count > 0 {
		inForce = board.Conventions[count-1].Text
	}
	if normalized != inForce {
		if s.refusedAsArchived(w, s.projectOf(board)) {
			return
		}
		board.Conventions = append(board.Conventions, &ConventionsVersion{Text: normalized, At: s.now, WriterID: person.ID})
		s.tellTheCoordinators(board, person)
	}

	if len(board.Conventions) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, conventionsResource(board, s))
}

// tellTheCoordinators writes a line into the bell of everybody who holds a
// live entry on the board, the writer left out. The line is about the board,
// not a card.
func (s *Server) tellTheCoordinators(board *Board, writer *Person) {
	ids := make([]string, 0, len(board.Coordinators))
	for id := range board.Coordinators {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		coordinator := s.People[id]
		if id == writer.ID || coordinator == nil || s.coordinatorOf(board, coordinator) == nil {
			continue
		}
		s.notify(&Notification{
			PersonID: id, Occasion: "board.conventionsChanged", ActorID: writer.ID, ProjectID: board.ProjectID,
			SubjectType: "board", SubjectID: board.ID, SubjectTitle: board.Title,
			Summary: "“How we work here” changed on " + board.Title,
		})
	}
}

// claimResource is the answer to a claim: the entry, the columns in board
// order with their roles, and the conventions in force.
func (s *Server) claimResource(board *Board, person *Person) any {
	entry, _ := s.coordinatorResource(board, person).(map[string]any)
	columns := []any{}
	for _, column := range board.Columns {
		named := map[string]any{"id": column.ID, "type": column.Type, "title": columnTitle(column)}
		if !s.IgnoresColumnRoles {
			named["role"] = roleOf(column)
		}
		columns = append(columns, named)
	}
	entry["columns"] = columns
	entry["conventions"] = conventionsResource(board, s)
	return entry
}
