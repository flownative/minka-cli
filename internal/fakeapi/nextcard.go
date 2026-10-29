package fakeapi

import (
	"net/http"
	"slices"
)

func (s *Server) registerNextCardRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/boards/{boardId}/next-card", s.authenticated(s.nextCard))
}

// nextCard answers the card the person takes up next on a board: of the
// columns with the role ready from left to right, each read from the bottom
// as the board shows it, the first card assigned to them that is neither on
// hold nor completed. It is the card as the list of their cards gives it, or
// null where nothing is released to them.
//
// A board this person cannot see answers as the board itself does, before a
// board without a column with the role ready is refused. A Minka from before
// column roles has no such address either.
func (s *Server) nextCard(w http.ResponseWriter, r *http.Request, person *Person) {
	if s.KnowsNoNextCard || s.IgnoresColumnRoles {
		unknownAddress(w)
		return
	}
	board := s.visibleBoard(r.PathValue("boardId"), person)
	if board == nil {
		notFound(w)
		return
	}

	released := false
	for _, column := range board.Columns {
		if deref(roleOf(column)) != "ready" {
			continue
		}
		released = true
		for _, card := range slices.Backward(column.Cards) {
			if !card.OnHold && openCard(column, card) && slices.Contains(card.AssigneeIDs, person.ID) {
				writeJSON(w, http.StatusOK, s.cardResource(board, column, card))
				return
			}
		}
	}
	if !released {
		writeJSON(w, http.StatusConflict, errorBody("boardHasNoReadyColumn", `No column of this board has the role "ready", so no card is released to be taken up.`))
		return
	}
	writeRaw(w, http.StatusOK, "null")
}
