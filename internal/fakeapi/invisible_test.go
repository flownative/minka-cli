package fakeapi

import (
	"strings"
	"testing"
)

// The cases of Minka's own test: what hides something, and what is plain text.
// Its case of invalid UTF-8 is not here: at both endpoints the control check
// refuses such a text first, as Minka's does.
func TestTheFakeFindsInvisibleCharactersAsMinkaDoes(t *testing.T) {
	for name, text := range map[string]string{
		"a right-to-left override":                     "Do \u202esiht",
		"a left-to-right embedding":                    "\u202ax",
		"a first-strong isolate":                       "a\u2068b\u2069",
		"a tag character":                              "Hello\U000e0041\U000e0042",
		"a cancel tag":                                 "\U000e007f",
		"a zero-width space":                           "ignore\u200bprevious",
		"a byte order mark":                            "\ufeffText",
		"a word joiner":                                "a\u2060b",
		"a soft hyphen":                                "Silben\u00adtrennung",
		"a supplement variation selector":              "a\U000e0100b",
		"a run of variation selectors":                 "❤\ufe0f\ufe0f",
		"a variation selector before a supplement one": "x\ufe0e\U000e01ef",
		"a run of joiners":                             "a\u200d\u200cb",
	} {
		if !hasInvisibleCharacters(text) {
			t.Errorf("%s passes", name)
		}
	}
	for name, text := range map[string]string{
		"an empty text":                      "",
		"Markdown with umlauts":              "## Größe\n\n- Bug: kaputt\t!",
		"an emoji with a zero-width joiner":  "\U0001f469\u200d\U0001f4bb",
		"a zero-width non-joiner":            "م\u200cا",
		"an emoji with a variation selector": "❤\ufe0f",
		"right-to-left letters":              "שלום",
	} {
		if hasInvisibleCharacters(text) {
			t.Errorf("%s is refused", name)
		}
	}
}

// Notes and conventions refuse invisible characters after control characters
// and before the length, and write nothing.
func TestTheFakeRefusesInvisibleCharactersInTheirPlace(t *testing.T) {
	s, f := New(t)
	notes := "/api/v1/cards/" + f.CardInTriage.ID + "/notes"
	long := strings.Repeat("x", 100001)
	for body, want := range map[string]string{
		`{"body":"Plan \u202etxet"}`:           "422 cardNotesHasInvisibleCharacters",
		`{"body":"Plan\u0007 \u202e"}`:         "422 cardNotesHasControlCharacters",
		`{"body":"` + long + `\u200b"}`:        "422 cardNotesHasInvisibleCharacters",
		`{"body":"Pairing \u200d and \ufe0f"}`: "200",
	} {
		if got := sendAs(t, s, f.Token, "PUT", notes, "application/json", body); got != want {
			t.Errorf("notes %.40s: %s, want %s", body, got, want)
		}
	}

	conventions := "/api/v1/boards/" + f.Board.ID + "/conventions"
	longer := strings.Repeat("x", 20001)
	for body, want := range map[string]string{
		`{"text":"Be nice\u202e"}`:        "422 conventionsHaveInvisibleCharacters",
		`{"text":"Be\u0007 nice\u202e"}`:  "422 conventionsInvalid",
		`{"text":"` + longer + `\u200b"}`: "422 conventionsHaveInvisibleCharacters",
		`{"text":"` + longer + `"}`:       "422 conventionsInvalid",
	} {
		if got := sendAs(t, s, f.Token, "PUT", conventions, "application/json", body); got != want {
			t.Errorf("conventions %.40s: %s, want %s", body, got, want)
		}
	}
	if len(f.Board.Conventions) != 0 {
		t.Error("a refused text was written")
	}
}

// On a card's front and in a comment the harmless characters are stripped
// without a word and come back so; what may hide something is refused after
// every other refusal of the text, and nothing is written.
func TestTheFakeStripsTheHarmlessAndRefusesTheHiddenOnAFront(t *testing.T) {
	s, f := New(t)
	cards := "/api/v1/boards/" + f.Board.ID + "/cards"

	got, card := answerTo(t, s, f.Token, "POST", cards, `{"title":"Plan\u200b the\u00ad tent\u200e","description":"<p>Bring\u2060 it\ufeff</p>"}`)
	if got != "201" || card["title"] != "Plan the tent" || card["description"] != "<p>Bring it</p>" {
		t.Fatalf("stripped: %s %v %v", got, card["title"], card["description"])
	}
	cardPath := "/api/v1/cards/" + card["id"].(string)
	for _, step := range []struct{ name, method, path, body, want string }{
		{"an override in a new title", "POST", cards, `{"title":"Plan \u202eeht"}`, "422 cardTitleHasInvisibleCharacters"},
		{"a title of a mark alone", "POST", cards, `{"title":"\u200b"}`, "422 cardTitleMissing"},
		{"a control character before the override", "POST", cards, `{"title":"a\tb\u202e"}`, "422 cardTitleHasControlCharacters"},
		{"a tag in a new description", "POST", cards, `{"title":"Ok","description":"<p>Go\udb40\udc41</p>"}`, "422 cardDescriptionHasInvisibleCharacters"},
		{"an isolate as an entity", "POST", cards, `{"title":"Ok","description":"<p>Go &#x2066;on</p>"}`, "422 cardDescriptionHasInvisibleCharacters"},
		{"an override in an edited title", "PATCH", cardPath, `{"title":"Plan \u202eeht"}`, "422 cardTitleHasInvisibleCharacters"},
		{"a run of joiners in an edited description", "PATCH", cardPath, `{"description":"<p>a\u200d\u200db</p>"}`, "422 cardDescriptionHasInvisibleCharacters"},
		{"an override in a comment", "POST", cardPath + "/comments", `{"body":"<p>Fine \u202eenif</p>"}`, "422 commentHasInvisibleCharacters"},
		{"a plain entity of an override", "POST", cardPath + "/comments", `{"body":"x &#x202E; y"}`, "201"},
		{"an entity of an override in markup", "POST", cardPath + "/comments", `{"body":"<p>x &#x202E; y</p>"}`, "422 commentHasInvisibleCharacters"},
		{"a plain entity in a description", "PATCH", cardPath, `{"description":"x &#x2066; y"}`, "200"},
		{"an override in a comment on a document", "POST", "/api/v1/documents/" + f.Document.ID + "/comments", `{"body":"<p>\u2067x</p>"}`, "422 commentHasInvisibleCharacters"},
	} {
		if got := sendAs(t, s, f.Token, step.method, step.path, "application/json", step.body); got != step.want {
			t.Errorf("%s: %s, want %s", step.name, got, step.want)
		}
	}
	if len(s.Boards[0].Columns[0].Cards) != 2 {
		t.Errorf("a refused card was created")
	}

	got, comment := answerTo(t, s, f.Token, "POST", cardPath+"/comments", `{"body":"<p>Done\u200b, \ud83d\udc69\u200d\ud83d\udcbb</p>"}`)
	if got != "201" || comment["body"] != "<p>Done, \U0001f469\u200d\U0001f4bb</p>" {
		t.Errorf("comment: %s %v", got, comment["body"])
	}
}

// A comment is judged in the order of Minka's CommentBody: the length of what
// was sent, control characters, nothing at all, then invisible characters. A
// comment of a harmless character alone is kept, empty, with 201: that is how
// Minka answers it today, as its markup is not empty before the strip.
func TestTheFakeJudgesACommentInMinkasOrder(t *testing.T) {
	s, f := New(t)
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/comments"
	long := strings.Repeat("x", 10001)
	for _, step := range []struct{ name, body, want string }{
		{"too long with a control character", `{"body":"` + long + `\u0007"}`, "422 commentTooLong"},
		{"too long with harmless characters to strip", `{"body":"` + strings.Repeat("x", 9999) + `\u200b\u200b\u200b"}`, "422 commentTooLong"},
		{"a control character with an override", `{"body":"a\u0007\u202e"}`, "422 commentHasControlCharacters"},
		{"nothing", `{"body":"  "}`, "422 commentMissing"},
		{"a harmless character alone", `{"body":"\u200b"}`, "201"},
	} {
		if got := sendAs(t, s, f.Token, "POST", path, "application/json", step.body); got != step.want {
			t.Errorf("%s: %s, want %s", step.name, got, step.want)
		}
	}
	if last := f.CardInTriage.Comments[len(f.CardInTriage.Comments)-1]; last.Body != "" {
		t.Errorf("kept %q", last.Body)
	}
}
