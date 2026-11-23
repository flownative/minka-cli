package fakeapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func blockOf(toolID, filters string) string {
	return `<minka-decisions tool="` + toolID + `"` + filters + `></minka-decisions>`
}

func bodyField(html string) string {
	encoded, _ := json.Marshal(map[string]string{"body": html})
	return string(encoded)
}

// A document lists the decisions of each block where its element stands,
// and a block of a tool the reader cannot see lists nothing.
func TestADocumentListsTheRowsOfItsBlocks(t *testing.T) {
	s, f := New(t)
	f.Document.Body = "<p>It holds:</p>" + blockOf(f.Decisions.ID, ` status="`+f.Decisions.StatusOfRole("holds").ID+`"`) + blockOf(f.OtherDecisions.ID, "")

	got, answer := answerTo(t, s, f.Token, "GET", "/api/v1/documents/"+f.Document.ID, "")
	if got != "200" {
		t.Fatal(got)
	}
	blocks := answer["decisionBlocks"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("blocks %v", blocks)
	}
	live := blocks[0].(map[string]any)
	if live["position"] != float64(0) || live["frozen"] != false || live["summary"] != "Decisions · status In force" {
		t.Errorf("block %v", live)
	}
	var titles []string
	for _, decision := range live["decisions"].([]any) {
		titles = append(titles, decision.(map[string]any)["title"].(string))
	}
	if strings.Join(titles, "|") != "We met on Tuesdays|We meet on Mondays" {
		t.Errorf("titles %v: in force, archived ones included, in the order of their numbers", titles)
	}
	if !strings.HasPrefix(live["markdown"].(string), "**Decisions · status In force**\n\n- DEC-1 We met on Tuesdays — In force") {
		t.Errorf("markdown %q", live["markdown"])
	}
	unseen := blocks[1].(map[string]any)
	if unseen["position"] != float64(1) || unseen["tool"] != nil || len(unseen["decisions"].([]any)) != 0 || unseen["summary"] != decisionBlockSummaryTitle {
		t.Errorf("block of a tool of another project %v", unseen)
	}
}

// A write checks its blocks in Minka's order: the length of the text first,
// a lock before any block, and a block the stored text holds passes.
func TestBlocksAreCheckedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	statusID := f.Decisions.StatusOfRole("holds").ID
	unknown := "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	address := "/api/v1/documents/" + f.Document.ID

	cases := []struct {
		name, body, want string
	}{
		{"too long before the block", strings.Repeat("x", maxDocumentBodyLength) + blockOf(unknown, ""), "422 documentBodyTooLong"},
		{"no tool", `<minka-decisions label="` + unknown + `"></minka-decisions>`, "422 decisionBlockInvalid"},
		{"a filter that is no identifier", blockOf(f.Decisions.ID, ` status="holds"`), "422 decisionBlockInvalid"},
		{"a tool of another project", blockOf(f.OtherDecisions.ID, ""), "422 decisionBlockInvalid"},
		{"an unknown status", blockOf(f.Decisions.ID, ` status="`+unknown+`"`), "422 decisionBlockInvalid"},
		{"an unknown label", blockOf(f.Decisions.ID, ` label="`+unknown+`"`), "422 decisionBlockInvalid"},
		{"an event of another project", blockOf(f.Decisions.ID, ` decided-in="`+f.OtherEvent.ID+`"`), "422 decisionBlockInvalid"},
		{"a card of another project", blockOf(f.Decisions.ID, ` applies-to="`+f.OtherCard.ID+`"`), "422 decisionBlockInvalid"},
		{"twenty-one blocks", strings.Repeat(blockOf(f.Decisions.ID, ""), 21), "422 decisionBlockInvalid"},
		{"twenty blocks", strings.Repeat(blockOf(f.Decisions.ID, ""), 20), "200"},
		{"an event and a card of the project", blockOf(f.Decisions.ID, ` status="`+statusID+`" decided-in="`+f.OnceEvent.ID+`" applies-to="`+f.CardInDoing.ID+`"`), "200"},
	}
	for _, c := range cases {
		if got, answer := answerTo(t, s, f.Token, "PATCH", address, bodyField(c.body)); got != c.want {
			t.Errorf("%s: %s %v, want %s", c.name, got, answer["error"], c.want)
		}
	}

	f.Document.Body = blockOf(f.Decisions.ID, ` label="`+unknown+`"`)
	if got, _ := answerTo(t, s, f.Token, "PATCH", address, bodyField("<p>More</p>"+f.Document.Body)); got != "200" {
		t.Errorf("a block the text holds already is checked again: %s", got)
	}

	s.LockDocument(f.Document.ID)
	if got, _ := answerTo(t, s, f.Token, "PATCH", address, bodyField(blockOf(unknown, ""))); got != "422 documentLocked" {
		t.Errorf("a locked document with a block it cannot take answers %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "PATCH", address, bodyField(strings.Repeat("x", maxDocumentBodyLength+1))); got != "422 documentBodyTooLong" {
		t.Errorf("a locked document with a text too long answers %s", got)
	}
	earliest := f.Document.versions()[0]
	if got, _ := answerTo(t, s, f.Token, "POST", address+"/versions/"+earliest.ID+"/restore", ""); got != "422 documentLocked" {
		t.Errorf("restoring a version of a locked document answers %s", got)
	}
}

// Minka keeps the five attributes with an identifier each and nothing inside
// the element.
func TestABlockIsKeptAsItsQueryAlone(t *testing.T) {
	s, f := New(t)
	sent := `<minka-decisions tool="` + f.Decisions.ID + `" summary="&lt;b&gt;x&lt;/b&gt;" label=""><b>Inside</b></minka-decisions>`
	got, answer := answerTo(t, s, f.Token, "PATCH", "/api/v1/documents/"+f.Document.ID, bodyField(sent))
	if got != "200" || answer["body"] != blockOf(f.Decisions.ID, "") {
		t.Fatalf("%s: body %q", got, answer["body"])
	}
}

// A locked document shows the decisions it copied, each with what changed
// since, and none written after the lock.
func TestALockedDocumentShowsItsCopies(t *testing.T) {
	s, f := New(t)
	f.Document.Body = blockOf(f.Decisions.ID, "")
	s.LockDocument(f.Document.ID)
	f.Decision.Title = "We meet on Mondays at ten"
	f.ProposedDecision.StatusID = f.Decisions.StatusOfRole("rejected").ID
	f.Decisions.Decisions = append(f.Decisions.Decisions, &Decision{ID: newID(), Title: "After the lock", StatusID: f.Decisions.StatusOfRole("holds").ID, ShortID: "DEC-9"})

	_, answer := answerTo(t, s, f.Token, "GET", "/api/v1/documents/"+f.Document.ID, "")
	if answer["locked"] != true {
		t.Fatal("the document does not say it is locked")
	}
	block := answer["decisionBlocks"].([]any)[0].(map[string]any)
	if block["frozen"] != true {
		t.Fatal("the block is not frozen")
	}
	byTitle := map[string]map[string]any{}
	for _, decision := range block["decisions"].([]any) {
		byTitle[decision.(map[string]any)["title"].(string)] = decision.(map[string]any)
	}
	if _, found := byTitle["After the lock"]; found {
		t.Error("a decision written after the lock is listed")
	}
	if today := byTitle["We meet on Mondays"]["today"].(map[string]any); today["reworded"] != true {
		t.Errorf("today %v of a reworded decision", today)
	}
	if status, _ := byTitle["We buy a tent"]["today"].(map[string]any)["status"].(map[string]any); status["name"] != "Rejected" {
		t.Errorf("today's status %v", status)
	}
	if !strings.Contains(block["markdown"].(string), `\(as locked\)`) {
		t.Errorf("markdown %q", block["markdown"])
	}
}

// A post carries its blocks like a document, and a block alone says nothing.
func TestAPostCarriesItsBlocks(t *testing.T) {
	s, f := New(t)
	address := "/api/v1/posts/" + f.Post.ID
	if got, _ := answerTo(t, s, f.Token, "PATCH", address, bodyField(blockOf(f.Decisions.ID, ""))); got != "422 postBodyMissing" {
		t.Errorf("a block alone answers %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "PATCH", address, bodyField(blockOf(f.OtherDecisions.ID, ""))); got != "422 decisionBlockInvalid" {
		t.Errorf("a block of another project answers %s, before the body that says nothing", got)
	}
	got, answer := answerTo(t, s, f.Token, "PATCH", address, bodyField("<p>It holds:</p>"+blockOf(f.Decisions.ID, "")))
	if got != "200" || len(answer["decisionBlocks"].([]any)) != 1 {
		t.Fatalf("%s %v", got, answer["decisionBlocks"])
	}
}
