package cli

import (
	"testing"

	"github.com/flownative/minka-cli/internal/api"
)

// What the trash says about a thing that is no card: the tool it hung in is
// named as a tool and not as a board, and what stands in its way is said in
// words, whatever kind Minka reports.
func TestTheTrashNamesToolsAsToolsAndBlockersInWords(t *testing.T) {
	board := &api.BoardReference{ID: "b", Title: "API-3 Abnahme"}
	column := &api.ColumnReference{ID: "c", Type: "column", Title: "Doing"}
	tool := &api.BoardReference{ID: "t", Title: "Fortschritt"}

	for _, c := range []struct {
		name  string
		entry api.TrashEntry
		where string
	}{
		{"a card", api.TrashEntry{Kind: "card", Board: board, Column: column}, "API-3 Abnahme / Doing"},
		{"a column", api.TrashEntry{Kind: "column", Board: board}, "API-3 Abnahme"},
		{"a post", api.TrashEntry{Kind: "post", Board: tool}, "Fortschritt (tool)"},
		{"a to-do", api.TrashEntry{Kind: "todo", Board: tool}, "Fortschritt (tool)"},
		{"a document", api.TrashEntry{Kind: "document", Board: tool}, "Fortschritt (tool)"},
		{"a tool", api.TrashEntry{Kind: "tool"}, ""},
	} {
		if where := whereItHung(c.entry); where != c.where {
			t.Errorf("%s hung in %q, want %q", c.name, where, c.where)
		}
	}

	for blocker, want := range map[string]string{
		"column":         "no, its column is in the trash",
		"board":          "no, its board is in the trash",
		"tool":           "no, its tool is in the trash",
		"todoList":       "no, its to-do list is in the trash",
		"todoGroup":      "no, its group is in the trash",
		"question":       "no, its check-in question is in the trash",
		"folder":         "no, its folder is in the trash",
		"archivedFolder": "no, its folder is in the archive",
		"somethingNew":   "no, somethingNew is in the way",
	} {
		if said := howItComesBack(api.TrashEntry{Kind: "card", BlockedBy: &blocker}); said != want {
			t.Errorf("blocked by %s: %q, want %q", blocker, said, want)
		}
	}
}
