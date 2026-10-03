package fakeapi

import (
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// columnColors are the colours a column takes. White looks uncoloured, so it
// can be chosen but is never handed to a new column.
var columnColors = []string{"white", "red", "orange", "yellow", "green", "blue", "aqua", "purple", "gray", "pink", "brown"}

// TrashedColumn is a working column taken off its board, its cards with it.
// They stay in it untouched and stand on no board while it is in the trash.
type TrashedColumn struct {
	Column    *Column
	Board     *Board
	TrashedAt time.Time
	TrashedBy string // person ID

	seq int
}

func (s *Server) registerColumnsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/boards/{boardId}/columns", s.authenticated(s.addColumn))
	mux.HandleFunc("PATCH /api/v1/columns/{columnId}", s.authenticated(s.updateColumn))
	mux.HandleFunc("POST /api/v1/columns/{columnId}/moves", s.authenticated(s.moveColumn))
	mux.HandleFunc("DELETE /api/v1/columns/{columnId}", s.authenticated(s.removeColumn))
}

// addColumn puts a working column behind the others, in the colour the board
// holds least of. The title is judged before the write, and only the team may
// write: a client who sees the board is refused after the title.
func (s *Server) addColumn(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	if !validColumnTitle(w, deref(title)) {
		return
	}
	if s.columnWriteRefused(w, board, nil, person, "") {
		return
	}

	stored := phpTrim(*title)
	color := colorForANewColumn(board)
	column := &Column{ID: newID(), Type: "column", CustomTitle: &stored, Color: &color}
	at := len(board.Columns)
	for i, c := range board.Columns {
		if c.Type == "column" || c.Type == "triage" {
			at = i + 1
		}
	}
	board.Columns = slices.Insert(board.Columns, at, column)
	writeJSON(w, http.StatusCreated, s.columnResource(board, column))
}

// updateColumn renames and recolours a column. Every field is read and judged
// before anything is written; then the colour goes first, so that a system
// column refusing it keeps its title as well. A title the column bears already
// is no write at all and answers 200 even to a client, as in Minka.
func (s *Server) updateColumn(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column := s.visibleColumn(r.PathValue("columnId"), person)
	if column == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	_, titleNamed := fields["title"]
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	color, ok := stringField(w, fields, "color")
	if !ok {
		return
	}
	if !titleNamed && color == nil {
		unprocessable(w, "nothingToChange", "The body names neither a title nor a color.")
		return
	}
	// Only a system column may go without a title of its own: it is then
	// called by its type again.
	var newTitle *string
	if titleNamed && (column.Type == "column" || phpTrim(deref(title)) != "") {
		if !validColumnTitle(w, deref(title)) {
			return
		}
		trimmed := phpTrim(*title)
		newTitle = &trimmed
	}
	if color != nil && !slices.Contains(columnColors, *color) {
		unprocessable(w, "columnColorUnknown", "The color is none of the colors a column can carry.")
		return
	}

	if color != nil {
		if s.columnWriteRefused(w, board, column, person, "systemColumnHasNoColor") {
			return
		}
		chosen := *color
		column.Color = &chosen
	}
	unchanged := !s.projectOf(board).Archived && equalTitles(column.CustomTitle, newTitle)
	if titleNamed && !unchanged {
		if s.columnWriteRefused(w, board, column, person, "") {
			return
		}
		column.CustomTitle = newTitle
	}
	writeJSON(w, http.StatusOK, s.columnResource(board, column))
}

// moveColumn places a working column between its neighbours, or at the front
// without them. A neighbour that is no identifier is 404; one that is no other
// working column of the board is passed over, as the gap is looked for among
// the working columns alone. A column that stands where it is asked to go
// already is no write and answers 200 even to a client or in an archived
// project, as in Minka.
func (s *Server) moveColumn(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column := s.visibleColumn(r.PathValue("columnId"), person)
	if column == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	previousID, ok := stringField(w, fields, "previousColumnId")
	if !ok {
		return
	}
	nextID, ok := stringField(w, fields, "nextColumnId")
	if !ok {
		return
	}
	for _, id := range []*string{previousID, nextID} {
		if id != nil && *id != "" && !uuidPattern.MatchString(*id) {
			notFound(w)
			return
		}
	}

	var working []*Column
	from := -1
	for _, c := range board.Columns {
		if c.Type != "column" {
			continue
		}
		if c == column {
			from = len(working)
			continue
		}
		working = append(working, c)
	}
	to := 0
	if i := indexOfColumn(working, deref(previousID)); i >= 0 {
		to = i + 1
	} else if i := indexOfColumn(working, deref(nextID)); i >= 0 {
		to = i
	}
	if from == to {
		writeJSON(w, http.StatusOK, s.columnResource(board, column))
		return
	}
	if s.columnWriteRefused(w, board, column, person, "systemColumnCannotMove") {
		return
	}

	working = slices.Insert(working, to, column)
	next := 0
	for i, c := range board.Columns {
		if c.Type == "column" {
			board.Columns[i] = working[next]
			next++
		}
	}
	writeJSON(w, http.StatusOK, s.columnResource(board, column))
}

// removeColumn puts a working column into the trash of its project, and its
// cards go with it untouched: none of them is at its address until the column
// is back, which only the web does.
func (s *Server) removeColumn(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column := s.visibleColumn(r.PathValue("columnId"), person)
	if column == nil {
		notFound(w)
		return
	}
	if s.columnWriteRefused(w, board, column, person, "systemColumnCannotBeRemoved") {
		return
	}

	board.Columns = slices.DeleteFunc(board.Columns, func(c *Column) bool { return c == column })
	s.trashSeq++
	s.TrashedColumns = append(s.TrashedColumns, &TrashedColumn{Column: column, Board: board, TrashedAt: s.now, TrashedBy: person.ID, seq: s.trashSeq})

	// The server sends its JSON Content-Type along with the empty 204.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// columnWriteRefused answers a write Minka's statement does not let through,
// in the order Minka asks why once it failed: an archived project, told to
// whoever sees the project, a client among them; then Triage, Not now or Done
// where systemCode names what they refuse; then a client, who gets the same
// 404 as an outsider.
func (s *Server) columnWriteRefused(w http.ResponseWriter, board *Board, column *Column, person *Person, systemCode string) bool {
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return true
	}
	if systemCode != "" && column != nil && column.Type != "column" {
		unprocessable(w, systemCode, systemColumnRefusals[systemCode])
		return true
	}
	if s.isAClient(person.ID) {
		notFound(w)
		return true
	}
	return false
}

// systemColumnRefusals are the messages of what Triage, Not now and Done
// refuse.
var systemColumnRefusals = map[string]string{
	"systemColumnHasNoColor":      "Triage, Not now and Done carry no color of their own.",
	"systemColumnCannotMove":      "Triage, Not now and Done keep their places.",
	"systemColumnCannotBeRemoved": "Triage, Not now and Done belong to every board.",
}

// validColumnTitle refuses a title Minka would, in its order: the length of the
// trimmed title first, then a control character, then nothing at all.
func validColumnTitle(w http.ResponseWriter, title string) bool {
	title = phpTrim(title)
	switch {
	case utf8.RuneCountInString(title) > maxTitleLength:
		unprocessable(w, "columnTitleTooLong", "The title of a column is too long.")
	case strings.IndexFunc(title, unicode.IsControl) >= 0:
		unprocessable(w, "columnTitleHasControlCharacters", "The title of a column must not contain control characters.")
	case title == "":
		unprocessable(w, "columnTitleMissing", "A column needs a title.")
	default:
		return true
	}
	return false
}

// colorForANewColumn is the colour the working columns of a board carry least
// of, the earliest of the palette among equals. White is never handed out and
// not counted.
func colorForANewColumn(board *Board) string {
	palette := columnColors[1:]
	held := map[string]int{}
	for _, c := range board.Columns {
		if c.Type == "column" && c.Color != nil {
			held[*c.Color]++
		}
	}
	chosen := palette[0]
	for _, color := range palette {
		if held[color] < held[chosen] {
			chosen = color
		}
	}
	return chosen
}

// shownColor is the colour a column is drawn in: Done is always green, Triage
// and Not now carry none.
func shownColor(column *Column) *string {
	switch column.Type {
	case "column":
		return column.Color
	case "done":
		green := "green"
		return &green
	}
	return nil
}

func equalTitles(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func indexOfColumn(columns []*Column, id string) int {
	return slices.IndexFunc(columns, func(c *Column) bool { return c.ID == id })
}

// trashedColumn is the entry of the trash a column lies in, and nil for a
// column that stands on its board.
func (s *Server) trashedColumn(column *Column) *TrashedColumn {
	for _, trashed := range s.TrashedColumns {
		if trashed.Column == column {
			return trashed
		}
	}
	return nil
}
