package fakeapi

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// A card hands its comments over oldest first by the moment they were written,
// and two of the same moment in the order they were written in: the newest is
// the last one, wherever in the list a comment was put.
func TestACardServesItsCommentsOldestFirst(t *testing.T) {
	s, f := New(t)
	card := f.CardInTriage
	first := card.Comments[0]
	earlier := &Comment{ID: "0199e7b0-0000-4000-8000-000000000001", AuthorID: f.Anke.ID, Body: "Earlier", CreatedAt: first.CreatedAt.Add(-time.Hour)}
	sameMoment := &Comment{ID: "0199e7b0-0000-4000-8000-000000000000", AuthorID: f.Anke.ID, Body: "Same moment", CreatedAt: first.CreatedAt}
	card.Comments = append(card.Comments, earlier, sameMoment)

	answer := readJSON(t, s, f.Token, "/api/v1/cards/"+card.ID).(map[string]any)
	var served []string
	for _, comment := range answer["comments"].([]any) {
		served = append(served, fmt.Sprint(comment.(map[string]any)["id"]))
	}
	if want := []string{earlier.ID, first.ID, sameMoment.ID}; !slices.Equal(served, want) {
		t.Fatalf("served %v, want %v", served, want)
	}
}
