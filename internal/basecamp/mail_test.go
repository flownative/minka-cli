package basecamp

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// The rule for the sender of a mail is kept here and asked of nobody, so every
// part of it is held down: what a bundle carries has to be an address Minka
// takes, and an address of a shape nobody writes is left out rather than
// risked. The quoted form is the one that matters most — another reader takes
// what stands in the quotes for the whole address.
func TestWhatAnAddressAMailCanCarryIs(t *testing.T) {
	taken := []string{
		"billing@supplier.test",
		"  billing@supplier.test  ",
		"a@b.co",
		"a.b.c@d.e.fg",
		"first+second@a-b.example.museum",
		"!#$%&'*+-/=?^_`{|}~@e.xy",
		"A@B.CO",
		strings.Repeat("a", 64) + "@e.xy",
		"a@" + strings.Repeat("b", 63) + ".xy",
	}
	for _, address := range taken {
		got, ok := anAddressAMailCanCarry(address)
		if !ok {
			t.Errorf("%q is no address a mail can carry", address)
			continue
		}
		if got != strings.TrimSpace(address) {
			t.Errorf("%q came out as %q", address, got)
		}
	}

	refused := map[string]string{
		"":                                      "nothing at all",
		"supplier.test":                         "no at sign",
		"@supplier.test":                        "no local part",
		"billing@":                              "no domain",
		"root@localhost":                        "a domain of one label",
		`"a b"@supplier.test`:                   "a local part in quotation marks, which another reader reads to the quote",
		"a,b@supplier.test":                     "a character no local part may hold",
		".a@supplier.test":                      "a local part beginning with a dot",
		"a.@supplier.test":                      "a local part ending with a dot",
		"a..b@supplier.test":                    "two dots in a row",
		"a b@supplier.test":                     "a blank",
		"a@supplier..test":                      "an empty label",
		"a@-supplier.test":                      "a label beginning with a hyphen",
		"a@supplier-.test":                      "a label ending with a hyphen",
		"a@supplier.t":                          "a top label of one character",
		"a@supplier.42":                         "a top label of digits",
		"a@supplier_two.test":                   "a character no label may hold",
		"müller@supplier.test":                  "a local part outside ASCII",
		"a@exämple.test":                        "a domain outside ASCII",
		"a••@supplier.test":                     "the glyph a source masks an address with",
		strings.Repeat("a", 65) + "@e.xy":       "a local part of more than 64 characters",
		"a@" + strings.Repeat("b", 64) + ".xy":  "a label of more than 63 characters",
		"a@" + strings.Repeat("b.", 130) + "xy": "an address of more than 254 characters",
	}
	for address, why := range refused {
		if got, ok := anAddressAMailCanCarry(address); ok {
			t.Errorf("%q was taken as %q, and it has %s", address, got, why)
		}
	}
}

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
