package basecamp

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A comment and a line of a chat lose what Minka refuses in them, a control
// character other than a tab and a line break, and keep everything else; a
// code point between U+0080 and U+009F is Minka's reader's to read.
func TestATextLosesTheControlCharactersMinkaRefuses(t *testing.T) {
	for name, c := range map[string]struct {
		text, want string
		refused    bool
	}{
		"nothing to take":              {"<div>a\tb\nc\r\nd</div>", "<div>a\tb\nc\r\nd</div>", false},
		"a bell inside":                {"<div>a\x07b</div>", "<div>ab</div>", true},
		"a delete inside":              {"<div>a\x7fb</div>", "<div>ab</div>", true},
		"an escape in an attribute":    {`<a href="x` + "\x1b" + `">y</a>`, `<a href="x">y</a>`, true},
		"a null at the very end":       {"<div>a</div>\x00", "<div>a</div>", false},
		"a C1 code point":              {"<div>Don\u0092t\u0081</div>", "<div>Don\u0092t\u0081</div>", false},
		"a null behind a vertical tab": {"<div>a</div>\x0b\x00", "<div>a</div>", false},
	} {
		text, refused := withoutControlCharacters(c.text)
		if text != c.want || refused != c.refused {
			t.Errorf("%s: %q became %q, refused %v; want %q, %v", name, c.text, text, refused, c.want, c.refused)
		}
	}
}

// A description of a project is cut where the plain text Minka makes of it
// would be longer than 2000 characters: after the last whole word, ending in an
// ellipsis within the limit, and only then.
func TestADescriptionIsCutWhereMinkaWouldRefuseIt(t *testing.T) {
	fits := strings.Repeat("word ", 399) + "four"
	if got, cut := aDescriptionMinkaTakes(fits); cut || got != fits {
		t.Errorf("a description of %d characters was cut", utf8.RuneCountInString(fits))
	}

	// Whitespace Minka folds counts as it folds it: blanks run together, the
	// blanks around a line go, a line break is one character.
	folded := strings.Repeat("ab  \n  ", 500)
	if _, cut := aDescriptionMinkaTakes(folded); cut {
		t.Error("a description of 3500 characters that folds to 1499 was cut")
	}

	long := strings.Repeat("wörds ", 500)
	got, cut := aDescriptionMinkaTakes(long)
	switch {
	case !cut:
		t.Error("a description of 3000 characters was not cut")
	case utf8.RuneCountInString(got) > 2000:
		t.Errorf("the cut description has %d characters", utf8.RuneCountInString(got))
	case !strings.HasSuffix(got, " wörds…"):
		t.Errorf("the cut fell inside a word or left a blank: %q", got[len(got)-20:])
	}

	// Paragraphs count two characters between them, as Minka draws them.
	paragraphs := strings.Repeat(strings.Repeat("x", 99)+"\n\n", 20)
	if got, cut := aDescriptionMinkaTakes(paragraphs); !cut || utf8.RuneCountInString(got) > 2000 {
		t.Errorf("20 paragraphs of 99 characters, 2018 with their gaps, came as %d characters, cut %v", utf8.RuneCountInString(got), cut)
	}

	// One word longer than the limit is cut inside it.
	if got, cut := aDescriptionMinkaTakes(strings.Repeat("x", 2500)); !cut || utf8.RuneCountInString(got) != 2000 {
		t.Errorf("one long word came as %d characters", utf8.RuneCountInString(got))
	}
}

// A line of nothing but whitespace inside a paragraph is an empty line where
// Minka draws the text, and counts as one; more than one empty line in a row is
// drawn as one.
func TestADescriptionIsMeasuredAsMinkaDrawsIt(t *testing.T) {
	for text, want := range map[string]string{
		"ab\n \ncd":                "ab\n\ncd",
		"ab\n \n \ncd":             "ab\n\ncd",
		"  a   b\t\n c  \n\n\n d ": "a b\nc\n\nd",
		"one\n\ntwo\n \n\nthree":   "one\n\ntwo\n\nthree",
		"no break  end":            "no break  end",
	} {
		if got := asMinkaDrawsIt(text); got != want {
			t.Errorf("%q is drawn as %q, want %q", text, got, want)
		}
	}

	// Measured by the reviewer against Minka's own reading: 2398 characters,
	// which a measure that dropped the empty lines took for 1799.
	empty := strings.Repeat("ab\n \n", 600)
	got, cut := aDescriptionMinkaTakes(empty)
	if !cut {
		t.Fatal("a description Minka draws as 2398 characters was not cut")
	}
	if drawn := asMinkaDrawsIt(got); utf8.RuneCountInString(drawn) > 2000 {
		t.Errorf("the cut description is drawn as %d characters", utf8.RuneCountInString(drawn))
	}
}
