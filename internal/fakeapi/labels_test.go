package fakeapi

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// sendFor sends a request and decodes what came back, for a test that looks
// at the answer and not only at its status.
func sendFor(t *testing.T, s *Server, token, method, path, body string) (int, any) {
	t.Helper()
	request, err := http.NewRequest(method, s.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	var answer any
	_ = json.Unmarshal(raw, &answer)
	return response.StatusCode, answer
}

func labelNames(answer any) []string {
	var names []string
	list, _ := answer.([]any)
	for _, entry := range list {
		names = append(names, entry.(map[string]any)["name"].(string))
	}
	return names
}

// A name is tidied before it is judged and kept, and refused in Minka's
// order: nothing, too long, the shape, then the colour, then a name the
// project has already, compared case-folded across all its places.
func TestTheFakeJudgesANewLabelInMinkasOrder(t *testing.T) {
	s, f := New(t)
	s.AddLabel(f.Project.ID, f.Board.ID, "Priority::High", "red")
	path := "/api/v1/projects/" + f.Project.ID + "/labels"
	for body, want := range map[string]string{
		`{}`:             "422 labelNameMissing",
		`{"name":"   "}`: "422 labelNameMissing",
		`{"name":"` + strings.Repeat("ä", 51) + `"}`: "422 labelNameTooLong",
		`{"name":"a::b::c"}`:                         "422 labelNameMalformed",
		`{"name":"a::"}`:                             "422 labelNameMalformed",
		`{"name":"a:b"}`:                             "422 labelNameMalformed",
		`{"name":"v1.2"}`:                            "422 labelNameMalformed",
		`{"name":"a::b::c","color":"Red"}`:           "422 labelNameMalformed",
		`{"name":"Fine","color":"Red"}`:              "422 labelColorUnknown",
		`{"name":"priority :: HIGH","color":"Red"}`:  "422 labelColorUnknown",
		`{"name":"priority :: HIGH"}`:                "422 labelNameTaken",
		`{"name":1}`:                                 "400 malformedBody",
	} {
		if got := sendAs(t, s, f.Token, "POST", path, "application/json", body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	if got := sendAs(t, s, f.Token, "POST", path, "text/plain", `{"name":"Fine"}`); got != "415 unsupportedMediaType" {
		t.Errorf("another media type: %s", got)
	}
	if len(s.Labels) != 1 {
		t.Fatalf("%d labels after refusals only", len(s.Labels))
	}

	status, answer := sendFor(t, s, f.Token, "POST", path, `{"name":"  Stage   ::  Ünder Way+  ","color":"teal"}`)
	label := answer.(map[string]any)
	if status != http.StatusCreated || label["name"] != "Stage::Ünder Way+" || label["scope"] != "Stage" ||
		label["value"] != "Ünder Way+" || label["toolId"] != nil || label["position"] != 1.0 || label["color"] != "teal" {
		t.Fatalf("%d %v", status, label)
	}
	_, answer = sendFor(t, s, f.Token, "POST", path, `{"name":"Plain"}`)
	if plain := answer.(map[string]any); plain["scope"] != nil || plain["value"] != "Plain" || plain["position"] != 2.0 {
		t.Fatalf("a plain label: %v", plain)
	}
}

// Without a colour, a label takes the one Minka suggests from the labels in
// reach: blue where there are none, else the least-used colour farthest from
// those used.
func TestTheFakeSuggestsAColourForANewLabel(t *testing.T) {
	s, f := New(t)
	path := "/api/v1/boards/" + f.Board.ID + "/labels"
	for _, step := range []struct{ name, want string }{{"One", "blue"}, {"Two", "orange"}} {
		_, answer := sendFor(t, s, f.Token, "POST", path, `{"name":"`+step.name+`"}`)
		if got := answer.(map[string]any)["color"]; got != step.want {
			t.Errorf("%s: colour %v, want %s", step.name, got, step.want)
		}
	}
	s.Labels = nil
	s.AddLabel(f.Project.ID, "", "Project", "red")
	s.AddLabel(f.Project.ID, f.Board.ID, "Board", "teal")
	_, answer := sendFor(t, s, f.Token, "POST", path, `{"name":"Three"}`)
	if got := answer.(map[string]any)["color"]; got != "purple" {
		t.Errorf("colour %v, want purple: a tool counts the project's labels as well", got)
	}
}

// A tool lists its own labels and then the project's, each counted from 1; a
// label is at the address of its own place only.
func TestTheFakeListsAToolsLabelsBeforeTheProjects(t *testing.T) {
	s, f := New(t)
	s.AddLabel(f.Project.ID, "", "Shared", "red")
	own := s.AddLabel(f.Project.ID, f.Board.ID, "Own", "blue")
	s.AddLabel(f.Project.ID, f.Files.ID, "Elsewhere", "blue")

	_, answer := sendFor(t, s, f.Token, "GET", "/api/v1/boards/"+f.Board.ID+"/labels", "")
	list := answer.([]any)
	if got := labelNames(answer); !slices.Equal(got, []string{"Own", "Shared"}) {
		t.Fatalf("labels %v", got)
	}
	for _, entry := range list {
		if entry.(map[string]any)["position"] != 1.0 {
			t.Errorf("position %v, want 1 in each group", entry.(map[string]any)["position"])
		}
	}
	if got := sendAs(t, s, f.Token, "PATCH", "/api/v1/projects/"+f.Project.ID+"/labels/"+own.ID, "application/json", `{"color":"red"}`); got != "404" {
		t.Errorf("a board's label under its project: %s", got)
	}
}

// An edit is judged before it writes, a case-only rename is allowed, and the
// same values again are no refusal.
func TestTheFakeJudgesALabelEdit(t *testing.T) {
	s, f := New(t)
	label := s.AddLabel(f.Project.ID, "", "Urgent", "red")
	s.AddLabel(f.Project.ID, f.Files.ID, "Later", "blue")
	path := "/api/v1/projects/" + f.Project.ID + "/labels/" + label.ID
	for body, want := range map[string]string{
		`{}`:                               "422 nothingToChange",
		`{"name":null,"color":null}`:       "422 nothingToChange",
		`{"name":"a::b::c","color":"Red"}`: "422 labelNameMalformed",
		`{"color":"Red"}`:                  "422 labelColorUnknown",
		`{"name":"LATER"}`:                 "422 labelNameTaken",
		`{"name":"urgent"}`:                "200",
		`{"color":"red"}`:                  "200",
	} {
		if got := sendAs(t, s, f.Token, "PATCH", path, "application/json", body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	if label.Name != "urgent" || label.Color != "red" {
		t.Fatalf("label %s %s", label.Name, label.Color)
	}
}

// A move goes behind previousLabelId, which alone decides where both are
// given, else in front of nextLabelId, else to the front; a neighbour of
// another place or the label itself is refused.
func TestTheFakeMovesALabelWithinItsPlace(t *testing.T) {
	s, f := New(t)
	a := s.AddLabel(f.Project.ID, "", "A", "red")
	b := s.AddLabel(f.Project.ID, "", "B", "red")
	c := s.AddLabel(f.Project.ID, "", "C", "red")
	onTheBoard := s.AddLabel(f.Project.ID, f.Board.ID, "D", "red")
	path := "/api/v1/projects/" + f.Project.ID + "/labels/"
	order := func() []string {
		_, answer := sendFor(t, s, f.Token, "GET", "/api/v1/projects/"+f.Project.ID+"/labels", "")
		return labelNames(answer)
	}

	for _, step := range []struct {
		label *Label
		body  string
		want  []string
	}{
		{c, `{}`, []string{"C", "A", "B"}},
		{c, `{"previousLabelId":"` + b.ID + `"}`, []string{"A", "B", "C"}},
		{c, `{"nextLabelId":"` + a.ID + `"}`, []string{"C", "A", "B"}},
		{a, `{"previousLabelId":"` + b.ID + `","nextLabelId":"` + c.ID + `"}`, []string{"C", "B", "A"}},
	} {
		status, answer := sendFor(t, s, f.Token, "POST", path+step.label.ID+"/moves", step.body)
		if status != http.StatusOK || answer.(map[string]any)["id"] != step.label.ID {
			t.Fatalf("%s: %d %v", step.body, status, answer)
		}
		if got := order(); !slices.Equal(got, step.want) {
			t.Fatalf("%s: order %v, want %v", step.body, got, step.want)
		}
	}
	for body, want := range map[string]string{
		`{"previousLabelId":"` + onTheBoard.ID + `"}`: "422 labelPlaceUnknown",
		`{"nextLabelId":"` + a.ID + `"}`:              "422 labelPlaceUnknown",
		`{"previousLabelId":""}`:                      "200",
	} {
		if got := sendAs(t, s, f.Token, "POST", path+a.ID+"/moves", "application/json", body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	if got := sendAs(t, s, f.Token, "POST", path+a.ID+"/moves", "application/json", ""); got != "400 malformedBody" {
		t.Errorf("a move without a body: %s", got)
	}
}

// Putting a scoped label on takes any other label of the same scope off, in
// any case; a plain label never displaces. 201 is new, 200 carried already,
// and the answer is every label the item carries, the tool's first.
func TestTheFakePutsLabelsOnAnItem(t *testing.T) {
	s, f := New(t)
	high := s.AddLabel(f.Project.ID, "", "Priority::High", "red")
	low := s.AddLabel(f.Project.ID, f.Board.ID, "priority::Low", "blue")
	s.AddLabel(f.Project.ID, "", "Plain", "gray")
	s.AddLabel(f.Project.ID, f.OtherBoard.ID, "Elsewhere", "gray")
	path := "/api/v1/cards/" + f.CardInDoing.ID + "/labels"

	for _, step := range []struct {
		body   string
		status int
		want   []string
	}{
		{`{"labelId":"` + high.ID + `"}`, 201, []string{"Priority::High"}},
		{`{"name":"plain"}`, 201, []string{"Priority::High", "Plain"}},
		{`{"name":"PRIORITY :: low"}`, 201, []string{"priority::Low", "Plain"}},
		{`{"labelId":"` + low.ID + `"}`, 200, []string{"priority::Low", "Plain"}},
		{`{"labelId":"` + high.ID + `"}`, 201, []string{"Priority::High", "Plain"}},
	} {
		status, answer := sendFor(t, s, f.Token, "POST", path, step.body)
		if status != step.status || !slices.Equal(labelNames(answer), step.want) {
			t.Fatalf("%s: %d %v, want %d %v", step.body, status, labelNames(answer), step.status, step.want)
		}
	}

	for body, want := range map[string]string{
		`{}`:                        "422 labelMissing",
		`{"labelId":"","name":"x"}`: "422 labelAmbiguous",
		`{"labelId":"` + high.ID + `","name":""}`:        "422 labelAmbiguous",
		`{"name":"a::b::c"}`:                             "422 labelUnknown",
		`{"name":"Elsewhere"}`:                           "422 labelUnknown",
		`{"labelId":"` + strings.ToUpper(high.ID) + `"}`: "422 labelUnknown",
	} {
		if got := sendAs(t, s, f.Token, "POST", path, "application/json", body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}

	_, answer := sendFor(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInDoing.ID, "")
	if got := labelNames(answer.(map[string]any)["labels"]); !slices.Equal(got, []string{"Priority::High", "Plain"}) {
		t.Fatalf("the card shows %v", got)
	}
	_, answer = sendFor(t, s, f.Token, "GET", "/api/v1/boards/"+f.Board.ID, "")
	for _, column := range answer.(map[string]any)["columns"].([]any) {
		for _, card := range column.(map[string]any)["cards"].([]any) {
			card := card.(map[string]any)
			if _, ok := card["labels"].([]any); !ok {
				t.Fatalf("a card of the board carries no list of labels: %v", card)
			}
			if card["id"] == f.CardInDoing.ID && len(card["labels"].([]any)) != 2 {
				t.Fatalf("the card on the board shows %v", card["labels"])
			}
		}
	}
}

// A label comes off by its ID or by its name, also where the item does not
// carry it; naming both is refused, as is naming none.
func TestTheFakeTakesLabelsOffAnItem(t *testing.T) {
	s, f := New(t)
	label := s.AddLabel(f.Project.ID, f.Files.ID, "Draft", "red")
	s.PutLabelOn(f.Document.ID, label)
	path := "/api/v1/documents/" + f.Document.ID + "/labels"
	for _, step := range []struct{ method, path, want string }{
		{"DELETE", path + "/" + label.ID + "?name=Draft", "422 labelAmbiguous"},
		{"DELETE", path + "/no-id?name=Draft", "422 labelAmbiguous"},
		{"DELETE", path + "/no-id", "404"},
		{"DELETE", path, "422 labelMissing"},
		{"DELETE", path + "?name=", "422 labelMissing"},
		{"DELETE", path + "?name=Other", "422 labelUnknown"},
		{"DELETE", path + "?name=draft", "204"},
		{"DELETE", path + "/" + label.ID, "204"},
	} {
		if got := sendAs(t, s, f.Token, step.method, step.path, "", ""); got != step.want {
			t.Errorf("%s %s: %s, want %s", step.method, step.path, got, step.want)
		}
	}
	if len(s.LabelsOn(f.Document.ID)) != 0 {
		t.Fatal("the label stayed on")
	}
}

// Deleting a label takes it off every item, and a second delete is 404.
func TestTheFakeDeletesALabelFromEveryItem(t *testing.T) {
	s, f := New(t)
	label := s.AddLabel(f.Project.ID, "", "Gone", "red")
	s.PutLabelOn(f.CardInDoing.ID, label)
	s.PutLabelOn(f.FilesUpload.ID, label)
	path := "/api/v1/projects/" + f.Project.ID + "/labels/" + label.ID
	for _, want := range []string{"204", "404"} {
		if got := sendAs(t, s, f.Token, "DELETE", path, "", ""); got != want {
			t.Fatalf("delete: %s, want %s", got, want)
		}
	}
	if len(s.LabelsOn(f.CardInDoing.ID))+len(s.LabelsOn(f.FilesUpload.ID)) != 0 {
		t.Fatal("an item still carries the deleted label")
	}
	_, answer := sendFor(t, s, f.Token, "GET", "/api/v1/files-tools/"+f.Files.ID+"/entries?folderId="+f.Folder.ID, "")
	for _, upload := range answer.(map[string]any)["uploads"].([]any) {
		if labels, ok := upload.(map[string]any)["labels"].([]any); !ok || len(labels) != 0 {
			t.Fatalf("an upload of the folder lists %v", upload.(map[string]any)["labels"])
		}
	}
}

// A client of the project, and somebody outside it, hear of no label at all.
func TestTheFakeAnswersAClientOfTheProject404AtEveryLabelAddress(t *testing.T) {
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	label := s.AddLabel(f.Project.ID, "", "Seen", "red")
	s.PutLabelOn(f.Document.ID, label)
	for _, token := range []string{clientToken, f.StrangerToken} {
		for _, c := range []struct{ method, path, body string }{
			{"GET", "/api/v1/projects/" + f.Project.ID + "/labels", ""},
			{"GET", "/api/v1/files-tools/" + f.Files.ID + "/labels", ""},
			{"POST", "/api/v1/boards/" + f.Board.ID + "/labels", `{"name":"x"}`},
			{"PATCH", "/api/v1/projects/" + f.Project.ID + "/labels/" + label.ID, `{}`},
			{"POST", "/api/v1/projects/" + f.Project.ID + "/labels/" + label.ID + "/moves", `{}`},
			{"DELETE", "/api/v1/projects/" + f.Project.ID + "/labels/" + label.ID, ""},
			{"POST", "/api/v1/documents/" + f.Document.ID + "/labels", `{}`},
			{"DELETE", "/api/v1/documents/" + f.Document.ID + "/labels?name=Seen", ""},
		} {
			if got := sendAs(t, s, token, c.method, c.path, "application/json", c.body); got != "404" {
				t.Errorf("%s %s %s: %s", token, c.method, c.path, got)
			}
		}
	}
	// Labels on items are not filtered by team: a client sees them where it
	// sees the item.
	_, answer := sendFor(t, s, f.Token, "GET", "/api/v1/documents/"+f.Document.ID, "")
	if got := labelNames(answer.(map[string]any)["labels"]); !slices.Equal(got, []string{"Seen"}) {
		t.Fatalf("the document shows %v", got)
	}
}

// An archived project still lists its labels and refuses every write, after
// what the request itself gets wrong; an archived item refuses after that,
// also for a label it carries already.
func TestTheFakeRefusesLabelWritesInAnArchivedProject(t *testing.T) {
	s, f := New(t)
	label := s.AddLabel(f.Project.ID, "", "Kept", "red")
	f.Project.Archived = true
	place := "/api/v1/projects/" + f.Project.ID + "/labels"
	for _, c := range []struct{ method, path, body, want string }{
		{"GET", place, "", "200"},
		{"POST", place, `{"name":"New"}`, "422 projectArchived"},
		{"POST", place, `{"name":"kept"}`, "422 labelNameTaken"},
		{"PATCH", place + "/" + label.ID, `{}`, "422 nothingToChange"},
		{"PATCH", place + "/" + label.ID, `{"color":"blue"}`, "422 projectArchived"},
		{"POST", place + "/" + label.ID + "/moves", `{}`, "422 projectArchived"},
		{"DELETE", place + "/" + label.ID, "", "422 projectArchived"},
		{"POST", "/api/v1/cards/" + f.CardInDoing.ID + "/labels", `{"name":"Other"}`, "422 labelUnknown"},
		{"POST", "/api/v1/cards/" + f.CardInDoing.ID + "/labels", `{"name":"Kept"}`, "422 projectArchived"},
		{"DELETE", "/api/v1/cards/" + f.CardInDoing.ID + "/labels/" + label.ID, "", "422 projectArchived"},
	} {
		if got := sendAs(t, s, f.Token, c.method, c.path, "application/json", c.body); got != c.want {
			t.Errorf("%s %s %s: %s, want %s", c.method, c.path, c.body, got, c.want)
		}
	}

	f.Project.Archived = false
	s.PutLabelOn(f.ArchivedDocument.ID, label)
	s.PutInTheArchive(f.CardInDoing)
	for _, c := range []struct{ path, want string }{
		{"/api/v1/documents/" + f.ArchivedDocument.ID + "/labels", "422 documentArchived"},
		{"/api/v1/documents/" + f.DocumentBelowTheArchivedFolder.ID + "/labels", "422 documentArchived"},
		{"/api/v1/cards/" + f.CardInDoing.ID + "/labels", "422 cardArchived"},
		{"/api/v1/documents/" + f.DraftDocument.ID + "/labels", "404"},
	} {
		if got := sendAs(t, s, f.Token, "POST", c.path, "application/json", `{"labelId":"`+label.ID+`"}`); got != c.want {
			t.Errorf("%s: %s, want %s", c.path, got, c.want)
		}
	}
}

// A new card takes labels of its board and of its project by their IDs, one
// named twice once; what refuses them is judged in Minka's order, and a
// refusal leaves no card behind.
func TestTheFakeCreatesACardWithItsLabels(t *testing.T) {
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	high := s.AddLabel(f.Project.ID, "", "Priority::High", "red")
	low := s.AddLabel(f.Project.ID, "", "Priority::Low", "blue")
	verify := s.AddLabel(f.Project.ID, f.Board.ID, "devops::verify", "teal")
	create := s.AddLabel(f.Project.ID, f.Board.ID, "DevOps::create", "teal")
	ofTheTool := s.AddLabel(f.Project.ID, f.Files.ID, "Draft", "gray")
	ofAnotherProject := s.AddLabel(f.OtherProject.ID, f.OtherBoard.ID, "Elsewhere", "gray")
	path := "/api/v1/boards/" + f.Board.ID + "/cards"
	cardsBefore := len(f.Triage.Cards) + len(f.Doing.Cards)

	for _, c := range []struct{ body, want string }{
		{`{"title":"","labelIds":["nowhere"]}`, "422 cardTitleMissing"},
		{`{"title":"T","assigneeIds":["nobody"],"labelIds":"` + high.ID + `"}`, "422 personNotAProjectMember"},
		{`{"title":"T","labelIds":"` + high.ID + `"}`, "400 malformedBody"},
		{`{"title":"T","labelIds":[1]}`, "400 malformedBody"},
		{`{"title":"T","labelIds":["nowhere"]}`, "422 labelUnknown"},
		{`{"title":"T","labelIds":["` + strings.ToUpper(high.ID) + `"]}`, "422 labelUnknown"},
		{`{"title":"T","labelIds":["` + ofTheTool.ID + `"]}`, "422 labelUnknown"},
		{`{"title":"T","labelIds":["` + ofAnotherProject.ID + `"]}`, "422 labelUnknown"},
		{`{"title":"T","columnId":"` + f.Done.ID + `","labelIds":["nowhere"]}`, "422 labelUnknown"},
		{`{"title":"T","columnId":"` + f.Done.ID + `","labelIds":["` + high.ID + `","` + low.ID + `"]}`, "404"},
		{`{"title":"T","assigneeIds":["` + f.Stranger.ID + `"],"labelIds":["` + high.ID + `","` + low.ID + `"]}`, "422 personNotAProjectMember"},
		{`{"title":"T","labelIds":["` + high.ID + `","` + low.ID + `"]}`, "422 labelsShareAScope"},
		{`{"title":"T","labelIds":["` + verify.ID + `","` + create.ID + `"]}`, "422 labelsShareAScope"},
	} {
		if got := sendAs(t, s, f.Token, "POST", path, "application/json", c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.body, got, c.want)
		}
	}
	if got := sendAs(t, s, clientToken, "POST", path, "application/json", `{"title":"T","labelIds":["`+high.ID+`"]}`); got != "404" {
		t.Errorf("a client sending labels: %s", got)
	}
	if after := len(f.Triage.Cards) + len(f.Doing.Cards); after != cardsBefore {
		t.Fatalf("%d cards after refusals only, %d before", after, cardsBefore)
	}

	status, answer := sendFor(t, s, f.Token, "POST", path, `{"title":"T","labelIds":["`+high.ID+`","`+verify.ID+`","`+high.ID+`"]}`)
	if status != http.StatusCreated {
		t.Fatalf("status %d: %v", status, answer)
	}
	if got := labelNames(answer.(map[string]any)["labels"]); !slices.Equal(got, []string{"devops::verify", "Priority::High"}) {
		t.Fatalf("labels %v", got)
	}
	if got := len(s.ItemLabels[answer.(map[string]any)["id"].(string)]); got != 2 {
		t.Fatalf("the card carries %d labels", got)
	}

	f.Project.Archived = true
	if got := sendAs(t, s, f.Token, "POST", path, "application/json", `{"title":"T","labelIds":["`+high.ID+`","`+low.ID+`"]}`); got != "422 projectArchived" {
		t.Errorf("an archived project: %s", got)
	}
}
