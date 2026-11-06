package fakeapi

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// A card becomes a step by its ID or its short identifier in any case, reads
// as it stands, and a second time is 200 without a second step.
func TestACardBecomesAStepAndReadsAsItStands(t *testing.T) {
	s, f := New(t)
	child := f.CardInDoing
	child.ShortID = "MNK-1A"
	child.AssigneeIDs = []string{f.Anke.ID}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps"

	got, answer := answerTo(t, s, f.Token, "POST", path, `{"cardId":" mnk-1a "}`)
	if got != "201" {
		t.Fatalf("by the short identifier: %s", got)
	}
	step := answer
	linked, _ := step["card"].(map[string]any)
	if step["title"] != child.Title || step["completed"] != false || len(step["assignees"].([]any)) != 0 || step["dueOn"] != nil ||
		linked["id"] != child.ID || linked["shortId"] != "MNK-1A" || linked["kind"] != "card" || len(linked["assignees"].([]any)) != 1 {
		t.Fatalf("the card step reads %v", step)
	}
	for _, absent := range []string{"shortUrl", "completed"} {
		if _, there := linked[absent]; there {
			t.Errorf("the linked card carries %s", absent)
		}
	}

	got, again := answerTo(t, s, f.Token, "POST", path, `{"cardId":"`+strings.ToUpper(child.ID)+`"}`)
	if got != "200" || !reflect.DeepEqual(again, step) || len(f.CardInTriage.Steps) != 1 {
		t.Fatalf("again: %s, %v", got, again)
	}

	_, read := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+child.ID, "")
	parents, _ := read["stepOf"].([]any)
	if len(parents) != 1 || parents[0].(map[string]any)["id"] != f.CardInTriage.ID {
		t.Fatalf("the child names its parents as %v", read["stepOf"])
	}

	f.Doing.Cards = slices.DeleteFunc(f.Doing.Cards, func(card *Card) bool { return card == child })
	f.Done.Cards = append(f.Done.Cards, child)
	_, board := answerTo(t, s, f.Token, "GET", "/api/v1/boards/"+f.Board.ID, "")
	if tile := tileOf(board, f.CardInTriage.ID); tile["stepCount"] != 1.0 || tile["completedStepCount"] != 1.0 {
		t.Errorf("the tile counts %v of %v", tile["completedStepCount"], tile["stepCount"])
	}
}

// A new step answers as an entry of the card's list of steps, in words and
// as a card, and so does a card step that stands already; a Minka from
// before that answers with the whole card.
func TestANewStepAnswersAsItselfOrWithItsCard(t *testing.T) {
	for _, olderMinka := range []bool{false, true} {
		s, f := New(t)
		s.AnswersANewStepWithItsCard = olderMinka
		path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps"

		for _, c := range []struct{ body, status string }{
			{`{"title":"Measure"}`, "201"},
			{`{"cardId":"` + f.CardInDoing.ID + `"}`, "201"},
			{`{"cardId":"` + f.CardInDoing.ID + `"}`, "200"},
		} {
			got, answer := answerTo(t, s, f.Token, "POST", path, c.body)
			_, read := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInTriage.ID, "")
			steps := read["steps"].([]any)
			if olderMinka {
				if got != c.status || answer["id"] != f.CardInTriage.ID || !reflect.DeepEqual(answer["steps"], read["steps"]) {
					t.Errorf("an older Minka, %s: %s %v", c.body, got, answer)
				}
				continue
			}
			if want := steps[len(steps)-1]; got != c.status || !reflect.DeepEqual(answer, want) {
				t.Errorf("%s: %s %v, want %v", c.body, got, answer, want)
			}
		}
	}
}

// A Minka without card steps or without renaming steps predates the step as
// the answer, so it answers a new step with the card.
func TestAnOlderMinkaAnswersANewStepWithTheCard(t *testing.T) {
	for name, older := range map[string]func(*Server){
		"without card steps":      func(s *Server) { s.KnowsNoCardSteps = true },
		"without renaming a step": func(s *Server) { s.NeitherRenamesNorMovesSteps = true },
		"answering with its card": func(s *Server) { s.AnswersANewStepWithItsCard = true },
	} {
		s, f := New(t)
		older(s)
		got, answer := answerTo(t, s, f.Token, "POST", "/api/v1/cards/"+f.CardInTriage.ID+"/steps", `{"title":"Measure"}`)
		if _, isACard := answer["steps"]; got != "201" || !isACard || answer["id"] != f.CardInTriage.ID {
			t.Errorf("%s: %s %v", name, got, answer)
		}
	}
}

// A short identifier under a former prefix of its board still names its
// card, which carries its current one.
func TestAFormerPrefixStillNamesTheCard(t *testing.T) {
	s, f := New(t)
	f.CardInDoing.ShortID = "NEW-12"
	f.Board.FormerPrefixes = []string{"OLD"}

	got, answer := answerTo(t, s, f.Token, "GET", "/api/v1/cards/by-short-id/old-12", "")
	if got != "200" || answer["id"] != f.CardInDoing.ID || answer["shortId"] != "NEW-12" {
		t.Errorf("the lookup: %s %v", got, answer)
	}
	got, answer = answerTo(t, s, f.Token, "POST", "/api/v1/cards/"+f.CardInTriage.ID+"/steps", `{"cardId":"OLD-12"}`)
	if linked, _ := answer["card"].(map[string]any); got != "201" || linked["id"] != f.CardInDoing.ID {
		t.Errorf("a card step: %s %v", got, answer)
	}
	if got, _ := answerTo(t, s, f.Token, "GET", "/api/v1/cards/by-short-id/OLD-13", ""); got != "404" {
		t.Errorf("another suffix: %s", got)
	}
}

// Minka refuses words beside a card, then a card nobody can see by that
// name, then the card itself; only then is the archive told.
func TestACardStepIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps"
	for body, want := range map[string]string{
		`{"cardId":"` + f.CardInDoing.ID + `","title":"x"}`: "422 stepTitleAndCard",
		`{"cardId":1}`:                           "400 malformedBody",
		`{"cardId":null}`:                        "422 stepCardUnknown",
		`{"cardId":""}`:                          "422 stepCardUnknown",
		`{"cardId":"#MNK-1"}`:                    "422 stepCardUnknown",
		`{"cardId":"` + f.OtherCard.ID + `"}`:    "422 stepCardUnknown",
		`{"cardId":"` + f.CardInTriage.ID + `"}`: "422 stepCardIsThisCard",
	} {
		if got, _ := answerTo(t, s, f.Token, "POST", path, body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	if len(f.CardInTriage.Steps) != 0 {
		t.Fatalf("a refusal wrote %v", f.CardInTriage.Steps)
	}

	s.PutInTheArchive(f.CardInTriage)
	if got, _ := answerTo(t, s, f.Token, "POST", path, `{"cardId":"`+f.CardInDoing.ID+`"}`); got != "422 cardArchived" {
		t.Errorf("on an archived parent: %s", got)
	}
}

// A child the reader may not see is only a card, and the reader misses a
// parent they may not see.
func TestAnUnseenChildIsOnlyACard(t *testing.T) {
	s, f := New(t)
	step := &Step{ID: newID(), CardID: f.OtherCard.ID}
	f.CardInTriage.Steps = []*Step{step}

	_, answer := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInTriage.ID, "")
	want := map[string]any{"id": step.ID, "title": "", "completed": false, "assignees": []any{}, "dueOn": nil, "card": map[string]any{"kind": "card"}}
	if shown := stepOnTheCard(t, answer); !reflect.DeepEqual(shown, want) {
		t.Errorf("the step reads %v", shown)
	}
	_, other := answerTo(t, s, f.StrangerToken, "GET", "/api/v1/cards/"+f.OtherCard.ID, "")
	if parents, _ := other["stepOf"].([]any); parents == nil || len(parents) != 0 {
		t.Errorf("the stranger reads the parents as %v", other["stepOf"])
	}
}

// A card step takes no change of its own, refused before the body is read;
// it moves and comes off as any step, and the child stays.
func TestACardStepIsNeitherTickedNorRenamed(t *testing.T) {
	s, f := New(t)
	words := &Step{ID: newID(), Title: "Measure"}
	linked := &Step{ID: newID(), CardID: f.CardInDoing.ID}
	f.CardInTriage.Steps = []*Step{words, linked}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + linked.ID

	for _, body := range []string{`{"completed":true}`, `not even JSON`} {
		if got, _ := answerTo(t, s, f.Token, "PATCH", path, body); got != "422 stepIsACard" {
			t.Errorf("%s: %s", body, got)
		}
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path+"/moves", `{}`); got != "200" || f.CardInTriage.Steps[0] != linked {
		t.Errorf("the move: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "DELETE", path, ""); got != "204" || len(f.CardInTriage.Steps) != 1 {
		t.Errorf("the removal: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInDoing.ID, ""); got != "200" {
		t.Errorf("the child went with the link: %s", got)
	}
}

// A step made into a card is a new card in Triage with the step's people and
// day, named by Location; the step keeps its place and points at it.
func TestAStepIsMadeIntoACard(t *testing.T) {
	s, f := New(t)
	due := "2026-12-01"
	step := &Step{ID: newID(), Title: "Measure the place", AssigneeIDs: []string{f.Anke.ID}, DueOn: &due, Completed: true}
	f.CardInTriage.Steps = []*Step{step}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + step.ID + "/card"

	got, made := answerTo(t, s, f.Token, "POST", path, "")
	column, _ := made["column"].(map[string]any)
	parents, _ := made["stepOf"].([]any)
	if got != "201" || made["title"] != "Measure the place" || made["dueOn"] != due || column["type"] != "triage" ||
		len(parents) != 1 || parents[0].(map[string]any)["id"] != f.CardInTriage.ID {
		t.Fatalf("%s: %v", got, made)
	}
	now := f.CardInTriage.Steps[0]
	if now.ID != step.ID || now.CardID != made["id"] || len(now.AssigneeIDs) != 0 {
		t.Fatalf("the step is now %+v", now)
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path, ""); got != "422 stepIsACard" {
		t.Errorf("again: %s", got)
	}
}

// Words too long for a title are cut with an ellipsis and kept whole in the
// description; words refused as a title leave everything as it was.
func TestTheWordsOfAStepAsACard(t *testing.T) {
	s, f := New(t)
	long := &Step{ID: newID(), Title: strings.Repeat("a", 250)}
	hidden := &Step{ID: newID(), Title: "Deploy ‮fix"}
	f.CardInTriage.Steps = []*Step{long, hidden}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/"

	_, made := answerTo(t, s, f.Token, "POST", path+long.ID+"/card", "")
	if made["title"] != strings.Repeat("a", 199)+"…" || made["description"] != "<p>"+strings.Repeat("a", 250)+"</p>" {
		t.Errorf("made %v / %v", made["title"], made["description"])
	}
	cards := len(f.Triage.Cards)
	if got, _ := answerTo(t, s, f.Token, "POST", path+hidden.ID+"/card", ""); got != "422 cardTitleHasInvisibleCharacters" || len(f.Triage.Cards) != cards || hidden.CardID != "" {
		t.Errorf("refused words: %s", got)
	}
}

// An older Minka passes over the card and asks for words, and has no address
// to make a step into a card.
func TestAMinkaWithoutCardSteps(t *testing.T) {
	s, f := New(t)
	s.KnowsNoCardSteps = true
	step := &Step{ID: newID(), Title: "Measure"}
	f.CardInTriage.Steps = []*Step{step}

	if got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/cards/"+f.CardInTriage.ID+"/steps", `{"cardId":"`+f.CardInDoing.ID+`"}`); got != "422 stepMissing" {
		t.Errorf("a card step: %s", got)
	}
	got, answer := answerTo(t, s, f.Token, "POST", "/api/v1/cards/"+f.CardInTriage.ID+"/steps/"+step.ID+"/card", "")
	if got != "404" || answer != nil {
		t.Errorf("make-card: %s %v", got, answer)
	}
	_, card := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInTriage.ID, "")
	if _, there := stepOnTheCard(t, card)["card"]; there {
		t.Error("a step carries card")
	}
	if _, there := card["stepOf"]; there {
		t.Error("the card carries stepOf")
	}
}

// The card itself is refused before the archive is told, so an archived
// parent answers it as any parent does.
func TestTheCardItselfIsRefusedBeforeTheArchive(t *testing.T) {
	s, f := New(t)
	s.PutInTheArchive(f.CardInTriage)
	got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/cards/"+f.CardInTriage.ID+"/steps", `{"cardId":"`+f.CardInTriage.ID+`"}`)
	if got != "422 stepCardIsThisCard" {
		t.Errorf("the card itself on an archived parent: %s", got)
	}
}

// A child in the archive names nobody, as on the board, and a step in words
// carries card as null.
func TestAnArchivedChildNamesNobodyAndWordsCarryNoCard(t *testing.T) {
	s, f := New(t)
	child := f.CardInDoing
	child.AssigneeIDs = []string{f.Anke.ID}
	f.CardInTriage.Steps = []*Step{{ID: newID(), Title: "Measure"}, {ID: newID(), CardID: child.ID}}
	s.PutInTheArchive(child)

	_, answer := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInTriage.ID, "")
	steps := answer["steps"].([]any)
	words, linked := steps[0].(map[string]any), steps[1].(map[string]any)["card"].(map[string]any)
	if card, there := words["card"]; !there || card != nil {
		t.Errorf("the step in words carries card as %v (present %v)", card, there)
	}
	if linked["id"] != child.ID || !reflect.DeepEqual(linked["assignees"], []any{}) {
		t.Errorf("the archived child reads %v", linked)
	}
}

// Making a step into a card names the new card in Location, and somebody on
// the step who may not be on a card of the project refuses it with nothing
// written.
func TestMakeCardNamesItsCardAndRefusesAStranger(t *testing.T) {
	s, f := New(t)
	step := &Step{ID: newID(), Title: "Measure"}
	stranger := &Step{ID: newID(), Title: "Lights", AssigneeIDs: []string{f.Stranger.ID}}
	f.CardInTriage.Steps = []*Step{step, stranger}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/"

	request, err := http.NewRequest("POST", s.URL+path+step.ID+"/card", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+f.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	made := f.CardInTriage.Steps[0].CardID
	if want := s.URL + "/api/v1/cards/" + made; response.StatusCode != 201 || made == "" || response.Header.Get("Location") != want {
		t.Errorf("%d, Location %q, want %q", response.StatusCode, response.Header.Get("Location"), want)
	}

	cards := len(f.Triage.Cards)
	got, _ := answerTo(t, s, f.Token, "POST", path+stranger.ID+"/card", "")
	if got != "422 personNotAProjectMember" || len(f.Triage.Cards) != cards || f.CardInTriage.Steps[1].CardID != "" {
		t.Errorf("a stranger on the step: %s", got)
	}
}

func tileOf(board map[string]any, cardID string) map[string]any {
	for _, column := range board["columns"].([]any) {
		for _, tile := range column.(map[string]any)["cards"].([]any) {
			if tile.(map[string]any)["id"] == cardID {
				return tile.(map[string]any)
			}
		}
	}
	return nil
}
