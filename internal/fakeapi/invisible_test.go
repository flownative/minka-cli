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
