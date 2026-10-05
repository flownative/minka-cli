package fakeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// remindersOf reads the list of reminders as the person of the token sees it.
func remindersOf(t *testing.T, s *Server, token string) []map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, s.URL+"/api/v1/me/reminders", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("the list is no JSON array: %v: %s", err, raw)
	}
	return list
}

func reminderPath(kind, id string) string {
	return "/api/v1/me/reminders/" + kind + "/" + id
}

func TestAPresetSetsAReminderOnlyItsPersonSees(t *testing.T) {
	s, f := New(t)

	got, reminder := answerTo(t, s, f.Token, http.MethodPut, reminderPath("card", strings.ToUpper(f.CardInTriage.ID)), `{"preset":"in1Hour"}`)
	if got != "200" {
		t.Fatalf("setting: %s", got)
	}
	want := map[string]any{
		"kind": "card", "id": f.CardInTriage.ID, "title": f.CardInTriage.Title,
		"project":  map[string]any{"id": f.Project.ID, "name": f.Project.Name},
		"remindAt": timestamp(s.now.Add(time.Hour)), "isDue": false,
		"appUrl": s.cardURL(f.Board, f.CardInTriage),
	}
	if fmt.Sprint(reminder) != fmt.Sprint(want) {
		t.Fatalf("reminder %v\nwant     %v", reminder, want)
	}
	if list := remindersOf(t, s, f.Token); len(list) != 1 || fmt.Sprint(list[0]) != fmt.Sprint(want) {
		t.Fatalf("the list: %v", list)
	}
	if list := remindersOf(t, s, f.AgentToken); len(list) != 0 {
		t.Fatalf("somebody else sees %v", list)
	}
}

// Setting a reminder again moves the one that is there, due or not.
func TestASecondReminderOfTheSameThingMovesTheFirst(t *testing.T) {
	s, f := New(t)
	s.Reminders = []*Reminder{{PersonID: f.Bruno.ID, SubjectID: f.Post.ID, RemindAt: s.now.Add(-time.Hour)}}

	if got, _ := answerTo(t, s, f.Token, http.MethodPut, reminderPath("post", f.Post.ID), `{"remindAt":"2026-09-20T10:00:00+02:00"}`); got != "200" {
		t.Fatalf("moving: %s", got)
	}
	list := remindersOf(t, s, f.Token)
	if len(list) != 1 || list[0]["remindAt"] != "2026-09-20T08:00:00.000000Z" || list[0]["isDue"] != false {
		t.Fatalf("the list: %v", list)
	}
}

func TestTheDueRemindersComeFirstTheMostRecentlyDueFirst(t *testing.T) {
	s, f := New(t)
	at := func(hours int) time.Time { return s.now.Add(time.Duration(hours) * time.Hour) }
	// Two of the same moment stand in the order of their identifiers.
	first, second := f.CardInDoing.ID, f.CardInTriage.ID
	if first > second {
		first, second = second, first
	}
	s.Reminders = []*Reminder{
		{PersonID: f.Bruno.ID, SubjectID: f.Post.ID, RemindAt: at(5)},
		{PersonID: f.Bruno.ID, SubjectID: f.Document.ID, RemindAt: at(-5)},
		{PersonID: f.Bruno.ID, SubjectID: second, RemindAt: at(2)},
		{PersonID: f.Bruno.ID, SubjectID: f.LooseTodo.ID, RemindAt: s.now},
		{PersonID: f.Bruno.ID, SubjectID: first, RemindAt: at(2)},
		{PersonID: f.Anke.ID, SubjectID: f.OnceEvent.ID, RemindAt: at(1)},
	}

	var order []string
	for _, reminder := range remindersOf(t, s, f.Token) {
		order = append(order, fmt.Sprint(reminder["id"], " ", reminder["isDue"]))
	}
	want := []string{
		f.LooseTodo.ID + " true", f.Document.ID + " true",
		first + " false", second + " false", f.Post.ID + " false",
	}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("order %v\nwant  %v", order, want)
	}
}

// A reminder of something out of sight is left out, after each side was cut
// to 500: the cut counts what is then left out.
func TestAReminderOutOfSightIsLeftOutAfterTheCut(t *testing.T) {
	s, f := New(t)
	for i := range mostRemindersOnASide {
		s.Reminders = append(s.Reminders, &Reminder{PersonID: f.Bruno.ID, SubjectID: newID(), RemindAt: s.now.Add(time.Duration(i+1) * time.Minute)})
	}
	s.Reminders = append(s.Reminders,
		&Reminder{PersonID: f.Bruno.ID, SubjectID: f.Post.ID, RemindAt: s.now.Add(24 * time.Hour)},
		&Reminder{PersonID: f.Bruno.ID, SubjectID: f.OtherCard.ID, RemindAt: s.now.Add(-time.Minute)},
		&Reminder{PersonID: f.Bruno.ID, SubjectID: f.CardInTriage.ID, RemindAt: s.now.Add(-time.Hour)},
	)

	list := remindersOf(t, s, f.Token)
	if len(list) != 1 || list[0]["id"] != f.CardInTriage.ID {
		t.Fatalf("the list: %v", list)
	}
}

// The subject is looked up first, so a wrong kind, or one of nothing, is
// 404 whatever the body; then the body, and its fields in Minka's order.
func TestASetIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	card := reminderPath("card", f.CardInTriage.ID)
	for _, c := range []struct{ path, contentType, body, want string }{
		{reminderPath("post", f.CardInTriage.ID), "text/plain", "nonsense", "404"},
		{reminderPath("Card", f.CardInTriage.ID), "application/json", `{"preset":"tomorrow"}`, "404"},
		{reminderPath("card", newID()), "text/plain", "nonsense", "404"},
		{reminderPath("card", "not-an-id"), "application/json", `{"preset":"tomorrow"}`, "404"},
		{reminderPath("card", f.OtherCard.ID), "application/json", `{"preset":"tomorrow"}`, "404"},
		{reminderPath("chatMessage", f.Conversation.Messages[0].ID), "application/json", `{"preset":"tomorrow"}`, "404"},
		{reminderPath("post", f.TrashedPost.ID), "application/json", `{"preset":"tomorrow"}`, "404"},
		{card, "text/plain", `{"preset":"tomorrow"}`, "415 unsupportedMediaType"},
		{card, "application/json", `[]`, "400 malformedBody"},
		{card, "application/json", `{"preset":1,"remindAt":"x"}`, "400 malformedBody"},
		{card, "application/json", `{"preset":"tomorrow","remindAt":"2026-09-20T10:00:00Z"}`, "400 malformedBody"},
		{card, "application/json", `{"preset":"someday","remindAt":null}`, "422 reminderPresetUnknown"},
		{card, "application/json", `{"preset":""}`, "422 reminderPresetUnknown"},
		{card, "application/json", `{}`, "422 reminderTimeMissing"},
		{card, "application/json", `{"preset":null,"remindAt":null}`, "422 reminderTimeMissing"},
		{card, "application/json", `{"remindAt":"2026-09-20T10:00:00"}`, "422 reminderTimeNotAMoment"},
		{card, "application/json", `{"remindAt":"2027-02-30T10:00:00Z"}`, "422 reminderTimeNotAMoment"},
		{card, "application/json", `{"remindAt":"2026-09-20T10:00:00+0200"}`, "422 reminderTimeNotAMoment"},
		{card, "application/json", `{"remindAt":"2026-09-13T14:02:11.123456Z"}`, "422 reminderTimeGone"},
		{card, "application/json", `{"remindAt":"2026-09-13T14:02:11Z"}`, "422 reminderTimeGone"},
	} {
		request, err := http.NewRequest(http.MethodPut, s.URL+c.path, strings.NewReader(c.body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+f.Token)
		request.Header.Set("Content-Type", c.contentType)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		got := fmt.Sprint(response.StatusCode)
		var answer struct{ Error struct{ Code string } }
		if json.Unmarshal(raw, &answer) == nil && response.StatusCode != http.StatusNotFound {
			got += " " + answer.Error.Code
		}
		if got != c.want {
			t.Errorf("%s %s %s: %s, want %s", c.path, c.contentType, c.body, got, c.want)
		}
	}
	if len(s.Reminders) != 0 {
		t.Fatalf("a refusal set %d reminders", len(s.Reminders))
	}
}

// The moments Minka's own test of its reader takes and refuses, with what
// each stands for in UTC.
func TestAMomentIsReadInTheFormsMinkaReads(t *testing.T) {
	for written, want := range map[string]string{
		"2099-03-01T09:30:00Z":             "2099-03-01T09:30:00.000000Z",
		"2099-03-01T09:30:00+02:00":        "2099-03-01T07:30:00.000000Z",
		"2099-03-01T09:30:00-05:30":        "2099-03-01T15:00:00.000000Z",
		"2099-03-01T09:30:00+00:00":        "2099-03-01T09:30:00.000000Z",
		"2099-03-01T09:30:00+14:00":        "2099-02-28T19:30:00.000000Z",
		"2099-03-01T09:30:00-14:00":        "2099-03-01T23:30:00.000000Z",
		"2099-03-01T09:30:00.123Z":         "2099-03-01T09:30:00.123000Z",
		"2099-03-01T09:30:00.123456+01:00": "2099-03-01T08:30:00.123456Z",
		"2099-03-01T09:30:00.123456789Z":   "2099-03-01T09:30:00.123456Z",
		"2096-02-29T00:00:00Z":             "2096-02-29T00:00:00.000000Z",

		"":                                  "",
		"2099-03-01T09:30:00":               "",
		"2099-03-01T09:30:00 Europe/Berlin": "",
		"2099-03-01T09:30:00Europe/Berlin":  "",
		"2099-03-01T09:30:00CEST":           "",
		"2099-03-01T09:30:00UTC":            "",
		"2099-03-01T09:30:00z":              "",
		"2099-03-01t09:30:00Z":              "",
		"2099-03-01 09:30:00Z":              "",
		"2099-03-01T09:30:00+0200":          "",
		"2099-03-01T09:30:00+02":            "",
		"2099-03-01T09:30:00+25:00":         "",
		"2099-03-01T09:30:00+14:01":         "",
		"2099-03-01T09:30:00 +02:00":        "",
		"2099-03-01T9:30:00Z":               "",
		"2099-03-01T09:30Z":                 "",
		"99-03-01T09:30:00Z":                "",
		"2099-03-01T09:30:00.Z":             "",
		"2099-02-30T09:30:00Z":              "",
		"2099-03-01T24:00:00Z":              "",
		"2099-03-01T23:59:60Z":              "",
		"2099-03-01T09:30:00Z\n":            "",
		"2099-03-01":                        "",
		"tomorrow":                          "",
	} {
		moment, ok := parseReminderMoment(written)
		got := ""
		if ok {
			got = timestamp(moment)
		}
		if got != want {
			t.Errorf("%q: %q, want %q", written, got, want)
		}
	}
}

// Tomorrow and next Monday are the morning of a day in the person's zone; on
// a Monday the next Monday is a week on.
func TestAPresetIsCountedInThePersonsZone(t *testing.T) {
	s, f := New(t)
	f.Bruno.TimeZone = "America/New_York"
	// Monday, 14 September 2026, 01:30 in New York.
	s.now = time.Date(2026, 9, 14, 5, 30, 0, 0, time.UTC)

	for preset, want := range map[string]string{
		"in30Minutes": "2026-09-14T06:00:00.000000Z",
		"in3Hours":    "2026-09-14T08:30:00.000000Z",
		"tomorrow":    "2026-09-15T13:00:00.000000Z",
		"nextMonday":  "2026-09-21T13:00:00.000000Z",
	} {
		got, reminder := answerTo(t, s, f.Token, http.MethodPut, reminderPath("card", f.CardInTriage.ID), `{"preset":"`+preset+`"}`)
		if got != "200" || reminder["remindAt"] != want {
			t.Errorf("%s: %s %v, want %s", preset, got, reminder["remindAt"], want)
		}
	}

	f.Bruno.TimeZone = ""
	s.TimeZone = "Europe/Berlin"
	if _, reminder := answerTo(t, s, f.Token, http.MethodPut, reminderPath("card", f.CardInTriage.ID), `{"preset":"nextMonday"}`); reminder["remindAt"] != "2026-09-21T07:00:00.000000Z" {
		t.Errorf("in the installation's zone: %v", reminder["remindAt"])
	}
}

func TestACommentAndALineOfAProjectChatAreNamedByTheirOwnIdentifier(t *testing.T) {
	s, f := New(t)
	comment := f.Post.Comments[0]
	line := f.Chat.Messages[1]

	got, reminder := answerTo(t, s, f.Token, http.MethodPut, reminderPath("comment", comment.ID), `{"preset":"tomorrow"}`)
	if got != "200" || reminder["kind"] != "comment" || reminder["appUrl"] != s.postURL(f.Posts, f.Post)+"#comment-"+comment.ID ||
		!strings.HasPrefix(fmt.Sprint(reminder["title"]), s.People[comment.AuthorID].Name+": ") {
		t.Fatalf("a comment: %s %v", got, reminder)
	}
	got, reminder = answerTo(t, s, f.Token, http.MethodPut, reminderPath("chatMessage", line.ID), `{"preset":"tomorrow"}`)
	if got != "200" || reminder["kind"] != "chatMessage" || !strings.HasSuffix(fmt.Sprint(reminder["appUrl"]), "#chat-message-"+line.ID) {
		t.Fatalf("a chat line: %s %v", got, reminder)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPut, reminderPath("post", comment.ID), `{"preset":"tomorrow"}`); got != "404" {
		t.Fatalf("a comment as a post: %s", got)
	}
}

// The kind and the identifier come first, the outcome second, the reminder
// last; a reminder of something out of sight is still taken away.
func TestARemovalIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	s.Reminders = []*Reminder{{PersonID: f.Bruno.ID, SubjectID: f.OtherCard.ID, RemindAt: s.now.Add(time.Hour)}}
	other := reminderPath("card", f.OtherCard.ID)

	for path, want := range map[string]string{
		reminderPath("thing", f.OtherCard.ID) + "?outcome=later":   "404",
		reminderPath("Card", f.OtherCard.ID):                       "404",
		reminderPath("card", "not-an-id") + "?outcome=later":       "404",
		reminderPath("card", f.CardInTriage.ID) + "?outcome=later": "422 reminderOutcomeUnknown",
		reminderPath("card", f.CardInTriage.ID):                    "404",
		other + "?outcome=Done":                                    "422 reminderOutcomeUnknown",
	} {
		if got, _ := answerTo(t, s, f.Token, http.MethodDelete, path, ""); got != want {
			t.Errorf("%s: %s, want %s", path, got, want)
		}
	}
	if got, _ := answerTo(t, s, f.AgentToken, http.MethodDelete, other, ""); got != "404" {
		t.Errorf("somebody else's: %s", got)
	}

	if got, _ := answerTo(t, s, f.Token, http.MethodDelete, reminderPath("post", strings.ToUpper(f.OtherCard.ID)), ""); got != "204" {
		t.Fatalf("out of sight, under another kind: %s", got)
	}
	if len(s.Reminders) != 0 || len(s.RemovedReminders) != 1 || s.RemovedReminders[0].Outcome != "dismissed" {
		t.Fatalf("left %v, removed %v", s.Reminders, s.RemovedReminders)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodDelete, other, ""); got != "404" {
		t.Fatalf("a second time: %s", got)
	}

	s.Reminders = []*Reminder{{PersonID: f.Bruno.ID, SubjectID: f.Post.ID, RemindAt: s.now}}
	if got, _ := answerTo(t, s, f.Token, http.MethodDelete, reminderPath("post", f.Post.ID)+"?outcome=done", ""); got != "204" || s.RemovedReminders[1].Outcome != "done" {
		t.Fatalf("done: %s %v", got, s.RemovedReminders)
	}
}

// What a reminder can be of is looked up as the page of the item looks it up,
// for each kind with the rules of its own.
func TestEachKindIsFoundAsItsPageFindsIt(t *testing.T) {
	anke := func(s *Server, f *Fixture) string {
		s.Tokens["minka_pat_anke"] = f.Anke.ID
		return "minka_pat_anke"
	}
	client := func(s *Server, f *Fixture) string {
		s.Tokens["minka_pat_client"] = f.Client.ID
		return "minka_pat_client"
	}
	for _, c := range []struct {
		name    string
		prepare func(s *Server, f *Fixture) (token, kind, id string)
		want    string
	}{
		{"one's own draft post", func(s *Server, f *Fixture) (string, string, string) {
			return f.Token, "post", f.DraftPost.ID
		}, "200"},
		{"somebody else's draft post", func(s *Server, f *Fixture) (string, string, string) {
			return anke(s, f), "post", f.DraftPost.ID
		}, "404"},
		{"one's own draft document", func(s *Server, f *Fixture) (string, string, string) {
			return f.Token, "document", f.DraftDocument.ID
		}, "200"},
		{"somebody else's draft document", func(s *Server, f *Fixture) (string, string, string) {
			return anke(s, f), "document", f.DraftDocument.ID
		}, "404"},
		{"a card in the archive", func(s *Server, f *Fixture) (string, string, string) {
			s.PutInTheArchive(f.CardInDoing)
			return f.Token, "card", f.CardInDoing.ID
		}, "200"},
		{"a post shown to clients, to a client", func(s *Server, f *Fixture) (string, string, string) {
			f.Post.ShownToClients = true
			return client(s, f), "post", f.Post.ID
		}, "200"},
		{"a post not shown to clients, to a client", func(s *Server, f *Fixture) (string, string, string) {
			return client(s, f), "post", f.Post.ID
		}, "404"},
		{"a post shown to clients in a tool that is not, to a client", func(s *Server, f *Fixture) (string, string, string) {
			f.Post.ShownToClients = true
			f.Posts.ShownToClients = false
			return client(s, f), "post", f.Post.ID
		}, "404"},
		{"a document at the top of the tool, to a client", func(s *Server, f *Fixture) (string, string, string) {
			return client(s, f), "document", f.Document.ID
		}, "404"},
		{"a document in a folder shown to clients, to a client", func(s *Server, f *Fixture) (string, string, string) {
			f.Document.FolderID = f.FolderShownToClients.ID
			return client(s, f), "document", f.Document.ID
		}, "200"},
		{"an upload in a folder not shown to clients, to a client", func(s *Server, f *Fixture) (string, string, string) {
			return client(s, f), "upload", f.FilesUpload.ID
		}, "404"},
		{"an upload in a folder shown to clients, to a client", func(s *Server, f *Fixture) (string, string, string) {
			f.FilesUpload.FolderID = f.FolderShownToClients.ID
			return client(s, f), "upload", f.FilesUpload.ID
		}, "200"},
		{"an answer", func(s *Server, f *Fixture) (string, string, string) {
			return f.Token, "answer", f.AnkesAnswer.ID
		}, "200"},
		{"an answer in the trash", func(s *Server, f *Fixture) (string, string, string) {
			f.AnkesAnswer.Trashed = true
			return f.Token, "answer", f.AnkesAnswer.ID
		}, "404"},
		{"a line of a project chat", func(s *Server, f *Fixture) (string, string, string) {
			return f.Token, "chatMessage", f.Chat.Messages[0].ID
		}, "200"},
		{"a line taken back", func(s *Server, f *Fixture) (string, string, string) {
			f.Chat.Messages[0].Removed = true
			return f.Token, "chatMessage", f.Chat.Messages[0].ID
		}, "404"},
	} {
		s, f := New(t)
		token, kind, id := c.prepare(s, f)
		if got, _ := answerTo(t, s, token, http.MethodPut, reminderPath(kind, id), `{"preset":"tomorrow"}`); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// The title of a reminder of what somebody wrote is their name and the first
// 60 characters of the words, marked as cut.
func TestTheTitleOfACommentIsItsWriterAndTheStartOfItsWords(t *testing.T) {
	s, f := New(t)
	comment := f.Post.Comments[0]
	words := strings.Repeat("abcdefghij", 7)
	comment.Body = "<div>" + words + "</div>"

	_, reminder := answerTo(t, s, f.Token, http.MethodPut, reminderPath("comment", comment.ID), `{"preset":"tomorrow"}`)
	if want := "Anke: " + words[:60] + "…"; reminder["title"] != want {
		t.Fatalf("title %q, want %q", reminder["title"], want)
	}
}

// The title of an answer is cut from the words the server keeps beside its
// HTML: blocks apart, entities decoded.
func TestTheTitleOfAnAnswerIsCutFromItsWords(t *testing.T) {
	for body, want := range map[string]string{
		"<p>One</p><p>Two</p>":          "Anke: One Two",
		"<ul><li>a</li><li>b</li></ul>": "Anke: a b",
		"a &amp; b":                     "Anke: a & b",
		"One<br>Two":                    "Anke: One Two",
		"<p><strong>Bo</strong>ld</p>":  "Anke: Bold",
	} {
		s, f := New(t)
		f.AnkesAnswer.Body = body
		_, reminder := answerTo(t, s, f.Token, http.MethodPut, reminderPath("answer", f.AnkesAnswer.ID), `{"preset":"tomorrow"}`)
		if reminder["title"] != want {
			t.Errorf("%q: title %q, want %q", body, reminder["title"], want)
		}
	}
}

// Two due reminders of the same moment stand in the order of their subjects.
func TestTwoDueRemindersOfOneMomentStandByTheirSubjects(t *testing.T) {
	s, f := New(t)
	first, second := f.CardInDoing.ID, f.CardInTriage.ID
	if first > second {
		first, second = second, first
	}
	at := s.now.Add(-time.Hour)
	s.Reminders = []*Reminder{
		{PersonID: f.Bruno.ID, SubjectID: second, RemindAt: at},
		{PersonID: f.Bruno.ID, SubjectID: first, RemindAt: at},
	}
	list := remindersOf(t, s, f.Token)
	if len(list) != 2 || list[0]["id"] != first || list[1]["id"] != second {
		t.Fatalf("the list: %v", list)
	}
}
