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
// holds least of. The title is judged before the write.
func (s *Server) addColumn(w http.ResponseWriter, r *http.Request, person *Person) {
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil || s.isAClient(person.ID) {
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
	if s.columnWriteRefused(w, board, nil, "") {
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

// updateColumn renames, recolours and gives a role to a column. Every field is
// read and judged before anything is written, the role before the title and
// the colour; then the role and the colour go first, so that a system column
// refusing the colour keeps its title as well. A null colour makes a working
// column white, which is how a column without a colour looks, and a null role
// takes it away. A title the column bears already is no write while the
// project is in use; a role or colour it bears already is.
func (s *Server) updateColumn(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column := s.visibleColumnOfTheTeam(r.PathValue("columnId"), person)
	if column == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	_, titleNamed := fields["title"]
	_, colorNamed := fields["color"]
	if s.KeepsColumnColorOnNull && fields["color"] == nil {
		colorNamed = false
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	color, ok := stringField(w, fields, "color")
	if !ok {
		return
	}
	if s.IgnoresColumnRoles {
		delete(fields, "role")
	}
	_, roleNamed := fields["role"]
	role, ok := stringField(w, fields, "role")
	if !ok {
		return
	}
	if !titleNamed && !colorNamed && !roleNamed {
		unprocessable(w, "nothingToChange", "The body names neither a title, a color nor a role.")
		return
	}
	if role != nil && !slices.Contains(workingColumnRoles, *role) {
		unprocessable(w, "columnRoleUnknown", "The role is none of ready, in_progress and waiting_for_decision.")
		return
	}
	if roleNamed && column.Type != "column" {
		unprocessable(w, "systemColumnHasNoRole", "Triage and Done carry their role by type, Not now carries none.")
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

	if roleNamed {
		if s.columnWriteRefused(w, board, column, "") {
			return
		}
		column.Role = role
	}
	if colorNamed {
		if s.columnWriteRefused(w, board, column, "systemColumnHasNoColor") {
			return
		}
		chosen := "white"
		if color != nil {
			chosen = *color
		}
		column.Color = &chosen
	}
	unchanged := !s.projectOf(board).Archived && equalTitles(column.CustomTitle, newTitle)
	if titleNamed && !unchanged {
		if s.columnWriteRefused(w, board, column, "") {
			return
		}
		column.CustomTitle = newTitle
	}
	writeJSON(w, http.StatusOK, s.columnResource(board, column))
}

// moveColumn places a working column between its neighbours, or at the front
// without them. A neighbour that is no identifier is 404, and one on another
// board is refused. One the person cannot see, a system column of the board
// and the column itself are passed over, as the gap is looked for among the
// working columns alone. A column that stands where it is asked to go already
// is no write while the project is in use.
func (s *Server) moveColumn(w http.ResponseWriter, r *http.Request, person *Person) {
	board, column := s.visibleColumnOfTheTeam(r.PathValue("columnId"), person)
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
	for _, id := range []*string{previousID, nextID} {
		if id == nil || *id == "" {
			continue
		}
		if neighboursBoard, _ := s.visibleColumn(strings.ToLower(*id), person); neighboursBoard != nil && neighboursBoard != board {
			unprocessable(w, "neighbourColumnOnAnotherBoard", "A column moves only between columns of its own board.")
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
	if from == to && !s.projectOf(board).Archived {
		writeJSON(w, http.StatusOK, s.columnResource(board, column))
		return
	}
	if s.columnWriteRefused(w, board, column, "systemColumnCannotMove") {
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
	board, column := s.visibleColumnOfTheTeam(r.PathValue("columnId"), person)
	if column == nil {
		notFound(w)
		return
	}
	if s.columnWriteRefused(w, board, column, "systemColumnCannotBeRemoved") {
		return
	}

	board.Columns = slices.DeleteFunc(board.Columns, func(c *Column) bool { return c == column })
	s.trashSeq++
	s.TrashedColumns = append(s.TrashedColumns, &TrashedColumn{Column: column, Board: board, TrashedAt: s.now, TrashedBy: person.ID, seq: s.trashSeq})

	// The server sends its JSON Content-Type along with the empty 204.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// visibleColumnOfTheTeam finds a column for a write on it. A client of the
// project gets nothing, so that it hears of no field, system column or archive
// what it would not hear of a column that does not exist.
func (s *Server) visibleColumnOfTheTeam(id string, person *Person) (*Board, *Column) {
	if s.isAClient(person.ID) {
		return nil, nil
	}
	return s.visibleColumn(id, person)
}

// columnWriteRefused answers a write Minka's statement does not let through,
// in the order Minka asks why once it failed: an archived project, then
// Triage, Not now or Done where systemCode names what they refuse.
func (s *Server) columnWriteRefused(w http.ResponseWriter, board *Board, column *Column, systemCode string) bool {
	if s.refusedAsArchived(w, s.projectOf(board)) {
		return true
	}
	if systemCode != "" && column != nil && column.Type != "column" {
		unprocessable(w, systemCode, systemColumnRefusals[systemCode])
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

// workingColumnRoles are the roles a working column takes by hand. Triage is
// the board's inbox and Done is done, by type alone.
var workingColumnRoles = []string{"ready", "in_progress", "waiting_for_decision"}

// roleOf is the role a column reads with: Triage inbox, Done done, Not now
// none, and a working column the one it was given.
func roleOf(column *Column) *string {
	var role string
	switch column.Type {
	case "column":
		return column.Role
	case "triage":
		role = "inbox"
	case "done":
		role = "done"
	default:
		return nil
	}
	return &role
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
