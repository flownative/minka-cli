package fakeapi

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"testing"
)

// storedBy puts an upload into the project of the fixture, as somebody stored
// it a moment before.
func storedBy(s *Server, f *Fixture, uploaderID, name string, content []byte, contentType string) *Upload {
	upload := &Upload{ID: newID(), Circle: f.Project.ID, Filename: name, ContentType: contentType, Content: content, Size: len(content), UploaderID: uploaderID}
	s.Uploads = append(s.Uploads, upload)
	return upload
}

func putAway(s *Server, f *Fixture, card *Card) {
	for _, column := range f.Board.Columns {
		for i, one := range column.Cards {
			if one == card {
				column.Cards = slices.Delete(column.Cards, i, i+1)
				s.Archive = append(s.Archive, &ArchivedCard{Card: card, Column: column})
				return
			}
		}
	}
}

func attachments(card *Card) string {
	return "/api/v1/cards/" + card.ID + "/attachments"
}

// Minka checks the card, then the field, then the card's state, and the
// upload last: a card that takes no changes is the reason whatever the upload
// is, and a field that names no upload is refused before the archive.
func TestTheFakeChecksAnAttachInMinkasOrder(t *testing.T) {
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	path := attachments(f.CardInTriage)
	mine := storedBy(s, f, f.Bruno.ID, "plan.md", []byte("one"), "text/markdown")
	ankes := storedBy(s, f, f.Anke.ID, "hers.md", []byte("hers"), "text/markdown")

	for _, step := range []struct{ token, body, want string }{
		{clientToken, `{"upload":"` + mine.ID + `"}`, "404"},
		{f.Token, `{"upload":1}`, "400 malformedBody"},
		{f.Token, `{}`, "422 uploadMissing"},
		{f.Token, `{"upload":"nonsense"}`, "422 uploadMissing"},
		{f.Token, `{"upload":"` + ankes.ID + `"}`, "422 uploadNotAttachable"},
		{f.Token, `{"upload":"` + newID() + `"}`, "422 uploadNotAttachable"},
	} {
		if got := sendAs(t, s, step.token, "POST", path, "application/json", step.body); got != step.want {
			t.Errorf("%s: %s, want %s", step.body, got, step.want)
		}
	}

	f.Project.Archived = true
	if got := sendAs(t, s, f.Token, "POST", path, "application/json", `{"upload":"`+ankes.ID+`"}`); got != "422 projectArchived" {
		t.Errorf("archived project: %s", got)
	}
	f.Project.Archived = false
	putAway(s, f, f.CardInTriage)
	if got := sendAs(t, s, f.Token, "POST", path, "application/json", `{"upload":"`+ankes.ID+`"}`); got != "422 cardArchived" {
		t.Errorf("archived card: %s", got)
	}
	if got := sendAs(t, s, f.Token, "POST", path, "application/json", `{}`); got != "422 uploadMissing" {
		t.Errorf("archived card without an upload: %s", got)
	}
	if len(f.CardInTriage.BackFiles) != 0 {
		t.Fatal("a refusal put a file on the back")
	}
}

// One name is one file; the newest version alone is listed and served, and
// an upload on a back goes nowhere else.
func TestTheFakeKeepsOneFilePerNameAndServesTheNewest(t *testing.T) {
	s, f := New(t)
	path := attachments(f.CardInTriage)
	first := storedBy(s, f, f.Bruno.ID, "plan.md", []byte("one"), "text/markdown")
	second := storedBy(s, f, f.Bruno.ID, "plan.md", []byte("two"), "text/markdown")
	picture := storedBy(s, f, f.Bruno.ID, "Stage.png", []byte("\x89PNG\r\n\x1a\nnot really"), "image/png")

	for _, upload := range []*Upload{first, second, picture} {
		if got := sendAs(t, s, f.Token, "POST", path, "application/json", `{"upload":"`+upload.ID+`"}`); got != "201" {
			t.Fatalf("%s: %s", upload.Filename, got)
		}
	}
	if got := sendAs(t, s, f.Token, "POST", path, "application/json", `{"upload":"`+first.ID+`"}`); got != "422 uploadNotAttachable" {
		t.Errorf("an upload held by the back again: %s", got)
	}
	if !s.fileIsInUse(second) {
		t.Error("an upload on the back counts as free")
	}

	var listed []map[string]any
	getJSON(t, s, f.Token, path, &listed)
	if len(listed) != 2 || listed[0]["filename"] != "plan.md" || listed[0]["version"] != float64(2) || listed[1]["filename"] != "Stage.png" {
		t.Fatalf("listed %v", listed)
	}

	content, mediaType := getBytes(t, s, f.Token, path+"/"+listed[0]["id"].(string)+"/content")
	if content != "two" || mediaType != "application/octet-stream" {
		t.Errorf("markdown served as %q, %q", content, mediaType)
	}
	if _, mediaType := getBytes(t, s, f.Token, path+"/"+listed[1]["id"].(string)+"/content"); mediaType != "image/png" {
		t.Errorf("a picture served as %q", mediaType)
	}

	if got := sendAs(t, s, f.Token, "DELETE", path+"/"+listed[0]["id"].(string), "", ""); got != "204" {
		t.Fatalf("delete: %s", got)
	}
	if !first.Swept || !second.Swept {
		t.Error("the uploads of a removed file were left")
	}
	if got := sendAs(t, s, f.Token, "GET", path+"/"+listed[0]["id"].(string), "", ""); got != "404" {
		t.Errorf("a removed file: %s", got)
	}
}

// The whole back is the team's: a client reads and writes none of it.
func TestTheFakeKeepsTheBackFromAClient(t *testing.T) {
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	card := f.CardInTriage
	upload := storedBy(s, f, f.Bruno.ID, "plan.md", []byte("one"), "text/markdown")
	if got := sendAs(t, s, f.Token, "POST", attachments(card), "application/json", `{"upload":"`+upload.ID+`"}`); got != "201" {
		t.Fatal(got)
	}
	file := attachments(card) + "/" + card.BackFiles[0].ID

	if got := sendAs(t, s, clientToken, "GET", "/api/v1/cards/"+card.ID, "", ""); got != "200" {
		t.Fatalf("the client does not see the front: %s", got)
	}
	for _, request := range [][3]string{
		{"GET", "/api/v1/cards/" + card.ID + "/notes", ""},
		{"PUT", "/api/v1/cards/" + card.ID + "/notes", `{"body":"Mine"}`},
		{"GET", attachments(card), ""},
		{"GET", file, ""},
		{"GET", file + "/content", ""},
		{"DELETE", file, ""},
	} {
		if got := sendAs(t, s, clientToken, request[0], request[1], "application/json", request[2]); got != "404" {
			t.Errorf("%s %s: %s", request[0], request[1], got)
		}
	}
}

func getJSON(t *testing.T, s *Server, token, path string, into any) {
	t.Helper()
	request, _ := http.NewRequest("GET", s.URL+path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		t.Fatal(err)
	}
}

func getBytes(t *testing.T, s *Server, token, path string) (string, string) {
	t.Helper()
	request, _ := http.NewRequest("GET", s.URL+path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return string(content), response.Header.Get("Content-Type")
}
