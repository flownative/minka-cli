package fakeapi

import (
	"net/http"
	"testing"
)

// everyCommentAddress is one request at each of the six addresses of the
// comments under an item, about one comment under it.
func everyCommentAddress(item, commentID string) []struct{ method, path, body string } {
	comment := item + "/comments/" + commentID
	return []struct{ method, path, body string }{
		{http.MethodGet, item + "/comments", ``},
		{http.MethodPost, item + "/comments", `{"body":"Slipped in"}`},
		{http.MethodPatch, comment, `{"body":"Slipped in"}`},
		{http.MethodDelete, comment, ``},
		{http.MethodPost, comment + "/reactions", `{"content":"🎉"}`},
		{http.MethodDelete, comment + "/reactions?content=%F0%9F%91%8D", ``},
	}
}

func TestACommentOnAnEventIsWrittenReadAndCounted(t *testing.T) {
	s, f := New(t)
	event := "/api/v1/events/" + f.OnceEvent.ID

	got, written := answerTo(t, s, f.Token, http.MethodPost, event+"/comments", `{"body":"<p>Who helps?</p>"}`)
	if got != "201" {
		t.Fatalf("writing: %s", got)
	}
	if got, read := answerTo(t, s, f.Token, http.MethodGet, event, ``); got != "200" || read["commentCount"] != float64(3) {
		t.Errorf("the event counts %v comments", read["commentCount"])
	}
	comment := event + "/comments/" + written["id"].(string)
	if got, _ := answerTo(t, s, f.Token, http.MethodPost, comment+"/reactions", `{"content":"👍"}`); got != "201" {
		t.Errorf("reacting: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodDelete, comment, ``); got != "204" {
		t.Errorf("taking it back: %s", got)
	}
	if len(f.OnceEvent.Comments) != 2 {
		t.Errorf("comments %v", f.OnceEvent.Comments)
	}
}

// The event is looked up after the body is read and before what the body
// says; the comment under it before the reaction; the archive of the project
// before the author.
func TestACommentOnAnEventIsRefusedInTheOrderMinkaAsks(t *testing.T) {
	s, f := New(t)
	ofAnke, ofBruno := f.OnceEvent.Comments[0], f.OnceEvent.Comments[1]
	under := "/api/v1/events/" + f.OnceEvent.ID + "/comments"
	nowhere := "/api/v1/events/" + newID() + "/comments"
	ofAnotherEvent := "/api/v1/events/" + f.WeeklyEvent.ID + "/comments/" + ofBruno.ID

	for _, c := range []struct{ name, method, path, body, want string }{
		{"a comment that is no JSON on an event never written", http.MethodPost, nowhere, `{`, "400 malformedBody"},
		{"an empty comment on an event never written", http.MethodPost, nowhere, `{"body":""}`, "404"},
		{"a change that is no JSON of a comment never written", http.MethodPatch, under + "/" + newID(), `{`, "400 malformedBody"},
		{"an empty comment", http.MethodPost, under, `{"body":""}`, "422 commentMissing"},
		{"a change of a comment under another event", http.MethodPatch, ofAnotherEvent, `{"body":"Mine now"}`, "404"},
		{"an empty change of somebody else's comment", http.MethodPatch, under + "/" + ofAnke.ID, `{"body":""}`, "422 commentMissing"},
		{"a change of somebody else's comment", http.MethodPatch, under + "/" + ofAnke.ID, `{"body":"Mine now"}`, "404"},
		{"a removal of somebody else's comment", http.MethodDelete, under + "/" + ofAnke.ID, ``, "404"},
		{"an empty reaction", http.MethodPost, under + "/" + ofAnke.ID + "/reactions", `{"content":""}`, "422 reactionMissing"},
		{"an empty reaction on a comment never written", http.MethodPost, under + "/" + newID() + "/reactions", `{"content":""}`, "404"},
		{"a reaction on somebody else's comment", http.MethodPost, under + "/" + ofAnke.ID + "/reactions", `{"content":"👍"}`, "201"},
	} {
		if got, _ := answerTo(t, s, f.Token, c.method, c.path, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	if ofAnke.Body != "<p>The room is <strong>booked</strong>.</p>" || ofAnke.EditedAt != nil {
		t.Errorf("Anke's comment changed: %q", ofAnke.Body)
	}
}

// In an archived project every write under an event is refused as the
// archive, somebody else's comment included; reading stays open, and only a
// member hears why.
func TestAnEventOfAnArchivedProjectTakesNoWordAndSaysSo(t *testing.T) {
	s, f := New(t)
	ofAnke := f.OnceEvent.Comments[0]
	ofAnke.Reactions = []*Reaction{{Content: "👍", PersonIDs: []string{f.Bruno.ID}}}
	f.Project.Archived = true
	event := "/api/v1/events/" + f.OnceEvent.ID

	for _, request := range append(everyCommentAddress(event, f.OnceEvent.Comments[1].ID),
		struct{ method, path, body string }{http.MethodPatch, event + "/comments/" + ofAnke.ID, `{"body":"Mine now"}`},
		struct{ method, path, body string }{http.MethodDelete, event + "/comments/" + ofAnke.ID, ``},
	) {
		want := "422 projectArchived"
		if request.method == http.MethodGet {
			want = "200"
		}
		if got, _ := answerTo(t, s, f.Token, request.method, request.path, request.body); got != want {
			t.Errorf("%s %s: %s, want %s", request.method, request.path, got, want)
		}
		if got, _ := answerTo(t, s, f.StrangerToken, request.method, request.path, request.body); got != "404" {
			t.Errorf("a stranger at %s %s: %s", request.method, request.path, got)
		}
	}
	if len(f.OnceEvent.Comments) != 2 || f.OnceEvent.Comments[1].EditedAt != nil || len(ofAnke.Reactions) != 1 {
		t.Error("a comment under the event changed")
	}
}

// An event in the trash, and one a client is not shown, is at none of the
// addresses, with the byte-identical 404.
func TestAnEventInTheTrashOrHiddenFromAClientHasNoComments(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	event := "/api/v1/events/" + f.OnceEvent.ID
	for name, shown := range map[string]struct{ calendar, event bool }{
		"an event of the team":                       {calendar: true, event: false},
		"an event shown to clients, hidden calendar": {calendar: false, event: true},
	} {
		f.Calendar.ShownToClients, f.OnceEvent.ShownToClients = shown.calendar, shown.event
		for _, request := range everyCommentAddress(event, f.OnceEvent.Comments[1].ID) {
			if got, answer := answerTo(t, s, "minka_pat_client", request.method, request.path, request.body); got != "404" || answer["error"].(map[string]any)["code"] != "notFound" {
				t.Errorf("a client on %s, %s %s: %s", name, request.method, request.path, got)
			}
		}
	}

	f.Calendar.ShownToClients, f.OnceEvent.ShownToClients = true, true
	if got, _ := answerTo(t, s, "minka_pat_client", http.MethodPost, event+"/comments", `{"body":"Fine by us"}`); got != "201" {
		t.Errorf("a client on an event shown to clients: %s", got)
	}

	f.OnceEvent.Trashed = true
	for _, request := range everyCommentAddress(event, f.OnceEvent.Comments[1].ID) {
		if got, _ := answerTo(t, s, f.Token, request.method, request.path, request.body); got != "404" {
			t.Errorf("an event in the trash, %s %s: %s", request.method, request.path, got)
		}
	}
	if len(f.OnceEvent.Comments) != 3 || f.OnceEvent.Comments[1].EditedAt != nil {
		t.Error("a comment under the event changed")
	}
}

// A calendar no longer in use is written at its own address, and so are the
// comments under its events.
func TestAnEventOfACalendarOutOfUseTakesComments(t *testing.T) {
	s, f := New(t)
	event := &CalendarEvent{ID: newID(), Title: "Old fair", FirstDay: "2025-10-20", LastDay: "2025-10-20", CreatedAt: s.now, CreatorID: f.Bruno.ID}
	f.CalendarOutOfUse.Events = append(f.CalendarOutOfUse.Events, event)
	if got, _ := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/events/"+event.ID+"/comments", `{"body":"Still remembered"}`); got != "201" {
		t.Errorf("writing: %s", got)
	}
}
