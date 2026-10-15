package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// commentedPlace is one kind of item comments stand under, with the commands
// that read and write them. A command left nil does not exist for the kind.
type commentedPlace struct {
	show   []string // shows the item with its appUrl, and with its comments where embeds is set
	embeds bool
	list   []string
	create []string // the text follows
	edit   func(commentID string) []string
	react  func(commentID string) []string // the content follows
}

func commentedPlaces(e *testEnv) map[string]commentedPlace {
	f := e.fixture
	card := f.CardInDoing.ID
	items := func(command, id string) commentedPlace {
		return commentedPlace{
			show: []string{command, "show", id}, embeds: command != "events",
			list:   []string{command, "comments", "list", id},
			create: []string{command, "comments", "create", id},
			edit:   func(c string) []string { return []string{command, "comments", "edit", id, c} },
			react:  func(c string) []string { return []string{command, "comments", "react", id, c} },
		}
	}
	entry := func(show []string, flag, id string) commentedPlace {
		return commentedPlace{
			show: show, embeds: true,
			create: []string{"docs", "comments", "create", flag, id},
			edit:   func(c string) []string { return []string{"docs", "comments", "edit", c, flag, id} },
			react:  func(c string) []string { return []string{"docs", "comments", "react", c, flag, id} },
		}
	}
	return map[string]commentedPlace{
		"a card": {
			show: []string{"cards", "show", card}, embeds: true,
			list:   []string{"comments", "list", card},
			create: []string{"comments", "create", card},
			edit:   func(c string) []string { return []string{"comments", "edit", card, c} },
			react:  func(c string) []string { return []string{"comments", "react", card, c} },
		},
		"a post":            items("posts", f.Post.ID),
		"a to-do":           items("todos", f.LooseTodo.ID),
		"a to-do list":      items("todolists", f.TodoList.ID),
		"an event":          items("events", f.OnceEvent.ID),
		"a document":        entry([]string{"docs", "show", f.Document.ID}, "--document", f.Document.ID),
		"an upload":         entry([]string{"docs", "uploads", "show", f.FilesUpload.ID}, "--upload", f.FilesUpload.ID),
		"a check-in answer": {show: []string{"checkins", "answers", "show", f.BrunosAnswer.ID}, embeds: true},
	}
}

// commentsRead are the comments a place hands over, from every command that
// reads them.
func commentsRead(t *testing.T, e *testEnv, place commentedPlace) map[string][]map[string]any {
	t.Helper()
	read := map[string][]map[string]any{}
	if place.embeds {
		var item struct {
			Comments []map[string]any `json:"comments"`
		}
		if err := json.Unmarshal(runOK(t, e, place.show...).Data, &item); err != nil {
			t.Fatal(err)
		}
		read[strings.Join(place.show, " ")] = item.Comments
	}
	if place.list != nil {
		read[strings.Join(place.list, " ")] = listOfObjects(t, runOK(t, e, place.list...).Data)
	}
	return read
}

func styled(t *testing.T, e *testEnv, args ...string) string {
	t.Helper()
	r := e.run("", append(args, "--styled")...)
	expectCode(t, r, ExitOK)
	return r.stdout
}

func TestEveryCommentCarriesThePageOfItsItemOpenedAtIt(t *testing.T) {
	for name := range commentedPlaces(newTestEnv(t)) {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			place := commentedPlaces(e)[name]
			itemURL, _ := decode(t, runOK(t, e, place.show...).Data)["appUrl"].(string)
			if itemURL == "" {
				t.Fatal("the item has no address to compare with")
			}

			if place.create != nil {
				written := decode(t, runOK(t, e, append(place.create, "A word")...).Data)
				id, _ := written["id"].(string)
				if want := itemURL + "#comment-" + id; written["appUrl"] != want {
					t.Errorf("the comment written: appUrl %v, want %s", written["appUrl"], want)
				}
				edited := decode(t, runOK(t, e, append(place.edit(id), "Another word")...).Data)
				if want := itemURL + "#comment-" + id; edited["appUrl"] != want {
					t.Errorf("the comment changed: appUrl %v, want %s", edited["appUrl"], want)
				}
				reacted := decode(t, runOK(t, e, append(place.react(id), "👍")...).Data)
				if want := itemURL + "#comment-" + id; reacted["appUrl"] != want {
					t.Errorf("the comment reacted to: appUrl %v, want %s", reacted["appUrl"], want)
				}
			}

			for where, comments := range commentsRead(t, e, place) {
				if len(comments) == 0 {
					t.Fatalf("%s reads no comments", where)
				}
				for _, comment := range comments {
					if want := itemURL + "#comment-" + comment["id"].(string); comment["appUrl"] != want {
						t.Errorf("%s: appUrl %v, want %s", where, comment["appUrl"], want)
					}
				}
			}
		})
	}
}

func TestTheTextViewsGiveTheAddressOfEveryComment(t *testing.T) {
	for name := range commentedPlaces(newTestEnv(t)) {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			place := commentedPlaces(e)[name]

			if place.create != nil {
				written := decode(t, runOK(t, e, append(place.create, "A word")...).Data)
				lines := strings.Split(strings.TrimSuffix(styled(t, e, append(place.create, "Once more")...), "\n"), "\n")
				if len(lines) != 2 || !strings.HasPrefix(lines[1], "http") || !strings.Contains(lines[1], "#comment-") {
					t.Errorf("writing a comment answers %q, want the address on the line below", lines)
				}
				lines = strings.Split(strings.TrimSuffix(styled(t, e, append(place.edit(written["id"].(string)), "Changed")...), "\n"), "\n")
				if want := written["appUrl"]; len(lines) != 2 || lines[1] != want {
					t.Errorf("changing a comment answers %q, want %v on the line below", lines, want)
				}
			}

			for where, comments := range commentsRead(t, e, place) {
				args := strings.Fields(where)
				text := styled(t, e, args...)
				for _, comment := range comments {
					if want := "URL: " + comment["appUrl"].(string); !strings.Contains(text, want+"\n") {
						t.Errorf("%s lacks %q:\n%s", where, want, text)
					}
				}
			}
		})
	}
}

// A Minka from before the address of a comment leaves the field out. Its
// comments are read and written all the same, passed on as served, and the
// text gives no address and no line for one.
func TestAMinkaWithoutCommentAddressesIsStillRead(t *testing.T) {
	for name := range commentedPlaces(newTestEnv(t)) {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			e.server.HandsOutNoCommentURL = true
			place := commentedPlaces(e)[name]

			if place.create != nil {
				written := decode(t, runOK(t, e, append(place.create, "A word")...).Data)
				if value, present := written["appUrl"]; present {
					t.Errorf("the comment written carries appUrl %v, which the server never sent", value)
				}
				for what, args := range map[string][]string{
					"writing":  append(place.create, "Once more"),
					"changing": append(place.edit(written["id"].(string)), "Changed"),
				} {
					if text := styled(t, e, args...); strings.Count(text, "\n") != 1 {
						t.Errorf("%s a comment answers %q, want the one line that says so", what, text)
					}
				}
			}

			for where, comments := range commentsRead(t, e, place) {
				for _, comment := range comments {
					if value, present := comment["appUrl"]; present {
						t.Errorf("%s: appUrl %v, which the server never sent", where, value)
					}
				}
				text := styled(t, e, strings.Fields(where)...)
				if strings.Contains(text, "#comment-") || strings.Contains(text, "URL: \n") || strings.HasSuffix(text, "URL: ") {
					t.Errorf("%s gives an address it does not have:\n%s", where, text)
				}
			}
		})
	}
}
