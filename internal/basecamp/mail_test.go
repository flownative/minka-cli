package basecamp

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// A title is one line, not empty, of at most 200 characters and free of
// characters nobody can see. A subject is none of that, and nobody can put one
// right at the source, so it is made to fit here. Every case is one Minka would
// have refused.
func TestWhatASubjectBecomes(t *testing.T) {
	r := func(code rune) string { return string(code) }
	long := strings.Repeat("ab ", 100)

	for name, given := range map[string]struct{ subject, title string }{
		"a subject a title can carry":           {"Invoice for March", "Invoice for March"},
		"whitespace around it":                  {"  Invoice  ", "Invoice"},
		"nothing at all":                        {"", "(no subject)"},
		"nothing but whitespace":                {"  \t\n ", "(no subject)"},
		"nothing but characters nobody can see": {r(0) + r(7) + r(0x9f), "(no subject)"},
		"a line break":                          {"Re:\nFwd: it", "Re: Fwd: it"},
		"a carriage return and a tab":           {"Re:\r\n\tFwd: it", "Re:   Fwd: it"},
		"a control character of the second set": {"Re:" + r(0x85) + "it", "Re: it"},
		"a delete":                              {"Re:" + r(0x7f) + "it", "Re: it"},
		"bytes that are no UTF-8":               {string([]byte{0xff, 0xfe}) + "it", "it"},
	} {
		title, _, _ := aSubjectAsATitle(given.subject)
		if title != given.title {
			t.Errorf("%s: %q became %q, want %q", name, given.subject, title, given.title)
		}
	}

	// What is said about the two the run counts.
	if _, withoutASubject, changed := aSubjectAsATitle(""); !withoutASubject || changed {
		t.Errorf("a mail without a subject is said to be %v and %v", withoutASubject, changed)
	}
	if _, withoutASubject, changed := aSubjectAsATitle("Invoice"); withoutASubject || changed {
		t.Errorf("a subject a title can carry is said to be %v and %v", withoutASubject, changed)
	}
	if _, _, changed := aSubjectAsATitle("Re:\nit"); !changed {
		t.Error("a subject folded into one line is not said to have changed")
	}

	// Longer than a title holds: cut before the last character, ending in an
	// ellipsis and never in whitespace, and counted as changed.
	title, _, changed := aSubjectAsATitle(long)
	switch {
	case utf8.RuneCountInString(title) > maxTitleLength:
		t.Errorf("a long subject became a title of %d characters", utf8.RuneCountInString(title))
	case !strings.HasSuffix(title, ellipsis):
		t.Errorf("a long subject was cut without saying so: %q", title)
	case strings.HasSuffix(strings.TrimSuffix(title, ellipsis), " "):
		t.Errorf("a long subject was cut on a blank: %q", title)
	case !changed:
		t.Error("a subject that was cut is not said to have changed")
	}

	// A cut that lands on whitespace does not leave it before the ellipsis.
	onABlank, _, _ := aSubjectAsATitle(strings.Repeat("a", 198) + "  " + strings.Repeat("b", 50))
	if want := strings.Repeat("a", 198) + ellipsis; onABlank != want {
		t.Errorf("a subject cut on a blank became %q", onABlank)
	}

	// Counted in characters and not in bytes, or a subject of umlauts would be
	// cut at half the length.
	if title, _, _ := aSubjectAsATitle(strings.Repeat("ä", 200)); utf8.RuneCountInString(title) != 200 {
		t.Errorf("a subject of 200 umlauts became a title of %d characters", utf8.RuneCountInString(title))
	}

	// Whatever comes out is a title Minka takes.
	for _, subject := range []string{"", "  ", r(0) + r(7), "Re:\nit", long, strings.Repeat("ä", 500),
		string([]byte{0xff, 0xfe, 'a'}), strings.Repeat("a", 199) + " " + strings.Repeat("b", 50)} {
		title, _, _ := aSubjectAsATitle(subject)
		switch {
		case strings.TrimSpace(title) == "":
			t.Errorf("%q became a title of nothing", subject)
		case utf8.RuneCountInString(title) > maxTitleLength:
			t.Errorf("%q became a title of %d characters", subject, utf8.RuneCountInString(title))
		case strings.ContainsFunc(title, unicode.IsControl):
			t.Errorf("%q became a title holding a character nobody can see", subject)
		case !utf8.ValidString(title):
			t.Errorf("%q became a title that is no UTF-8", subject)
		}
	}
}
