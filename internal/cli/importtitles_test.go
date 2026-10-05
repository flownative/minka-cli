package cli

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// A title Minka would refuse and a to-do that starts after it is due cost the
// run nothing: the report names each record by its kind and ID, never by the
// title, in data, in a notice and in the text.
func TestImportBasecampNamesWhatItMadeToFit(t *testing.T) {
	e := newImportEnv(t)
	list := &e.account.Projects[7].Dock[0].TodoLists[0]
	list.Title = "Errands\x01of the week"
	todo := &list.Todos[0]
	todo.StartsOn, todo.DueOn = "2025-05-02", "2025-05-01"
	listID, todoID := strconv.FormatInt(list.ID, 10), strconv.FormatInt(todo.ID, 10)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7008", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	want := []any{map[string]any{"record": "to-do list", "sourceId": listID, "why": []any{"folded"}}}
	if got := data["titlesMadeToFit"]; !reflect.DeepEqual(got, want) {
		t.Errorf("titlesMadeToFit: got %v, want %v", got, want)
	}
	if got := data["todosStartingAfterTheyAreDue"]; !reflect.DeepEqual(got, []any{todoID}) {
		t.Errorf("todosStartingAfterTheyAreDue: got %v", got)
	}
	for _, said := range []string{"1 record carries a title or a name the run made into one Minka takes",
		"1 to-do comes without the day it starts on"} {
		if !strings.Contains(env.Notice, said) {
			t.Errorf("the notice does not say %q: %s", said, env.Notice)
		}
	}
	if strings.Contains(env.Notice, "Errands") {
		t.Errorf("the notice says the title: %s", env.Notice)
	}

	styled := e.run("import", "basecamp", "--project", "7008", "--out", out, "--styled")
	expectCode(t, styled, ExitOK)
	if !strings.Contains(styled.stdout, "to-do list "+listID+": folded") {
		t.Errorf("the text lacks the list:\n%s", styled.stdout)
	}
	if !strings.Contains(styled.stdout, "To-dos that start after they are due, without their start: 1 — "+todoID) {
		t.Errorf("the text lacks the to-do:\n%s", styled.stdout)
	}
}

// An event that ends before it starts is left out, and the report names it in
// data, in a notice and in the text.
func TestImportBasecampNamesTheEventsWhoseTimesMinkaRefuses(t *testing.T) {
	e := newImportEnv(t)
	e.account.AddCalendarProject()
	entry := &e.account.Calendar().Entries[2]
	entry.EndsAt = "2000-01-01T00:00:00.000Z"
	id := strconv.FormatInt(entry.ID, 10)
	project := strconv.Itoa(fakebasecamp.CalendarProject)

	env, r := e.runJSON("import", "basecamp", "--project", project, "--calendars", "--dry-run")
	expectCode(t, r, ExitOK)
	var data struct {
		Calendars struct {
			Calendars []struct {
				TimesMinkaRefuses []string `json:"timesMinkaRefuses"`
			} `json:"calendars"`
		} `json:"calendars"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	if got := data.Calendars.Calendars[0].TimesMinkaRefuses; !reflect.DeepEqual(got, []string{id}) {
		t.Errorf("timesMinkaRefuses: %v", got)
	}
	if !strings.Contains(env.Notice, "1 entry of a calendar is left out because it ends before it starts") {
		t.Errorf("the notice does not say so: %s", env.Notice)
	}

	styled := e.run("import", "basecamp", "--project", project, "--calendars", "--dry-run", "--styled")
	expectCode(t, styled, ExitOK)
	if !strings.Contains(styled.stdout, "entries that end before they start or start before the year 1, left out: 1 — "+id) {
		t.Errorf("the text lacks the entry:\n%s", styled.stdout)
	}
}

// What the run took out of a text or cut off a description is named in data,
// in a notice and in the text; a kind of post without an icon is no change
// the run made and is named nowhere.
func TestImportBasecampNamesTheTextsItCleaned(t *testing.T) {
	e := newImportEnv(t)
	e.account.Projects[0].Description = strings.Repeat("Plan the work. ", 200)
	e.account.Projects[3].Categories[1].Icon = ""
	e.account.Projects[3].Dock[3].Lines[0].Content = "Good\x07 morning"
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--project", "7004", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	for key, want := range map[string]any{
		"textsCleaned":    []any{map[string]any{"record": "line", "sourceId": "9801"}},
		"descriptionsCut": []any{"7001"},
		// Deprecated and kept empty, so that a reader of the report finds it.
		"categoriesGivenAnEmoji": []any{},
	} {
		if got := data[key]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", key, got, want)
		}
	}
	for _, said := range []string{"1 comment or line of a chat comes without the control characters",
		"1 project comes with its description cut"} {
		if !strings.Contains(env.Notice, said) {
			t.Errorf("the notice does not say %q: %s", said, env.Notice)
		}
	}

	styled := e.run("import", "basecamp", "--project", "7001", "--project", "7004", "--out", out, "--styled")
	expectCode(t, styled, ExitOK)
	for _, said := range []string{"Comments and lines without the control characters they held: 1\n  line       9801",
		"Projects whose descriptions were cut: 1 — 7001"} {
		if !strings.Contains(styled.stdout, said) {
			t.Errorf("the text lacks %q:\n%s", said, styled.stdout)
		}
	}
}
