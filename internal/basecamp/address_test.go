package basecamp

import (
	"strings"
	"testing"
)

// minkaTakes is what Minka makes of each address, measured against its own
// reader and not guessed: PHP's filter_var with the plain local part of
// EmailAddress on top of it. The rule here has to agree with every line of it
// in both directions. An address ours takes that Minka refuses stops the
// applier after a walk of hours; one Minka takes that ours refuses loses
// somebody for nothing.
var minkaTakes = map[string]bool{
	"ada@example.test":                  true,
	"ada.example+tag@mail.example.test": true,
	"a@b.co":                            true,
	"a.b.c@d.e.fg":                      true,
	"first+second@a-b.example.museum":   true,
	"!#$%&'*+-/=?^_`{|}~@e.xy":          true,
	"a|b@c.test":                        true,
	"a_b@c.test":                        true,
	"A@B.CO":                            true,
	"ada@sub.example.test":              true,
	"ada@my-example.test":               true,
	"ada@example.t3st":                  true,
	// A top label of one letter.
	"ada@example.t": true,
	// Hyphens by the pair, inside a label and inside the top label alike.
	"ada@my--example.test": true,
	"ada@example.co--uk":   true,
	"ada@example.co-uk":    true,
	"ada@example.c1":       true,
	"ada@example.a-1":      true,
	// A domain written the way a name outside ASCII is carried.
	"ada@xn--bcher-kva.test": true,
	"ada@example.xn--p1ai":   true,
	"ada@xn--e.test":         true,
	// A label that is nothing but digits, where it is not the top one.
	"ada@1.test": true,

	"":                     false,
	"ada":                  false,
	"example.test":         false,
	"@example.test":        false,
	"ada@":                 false,
	"ada@example":          false,
	"ada@.test":            false,
	"ada@example.":         false,
	"ada@@example.test":    false,
	"ada@example@e.test":   false,
	"ada@example..test":    false,
	"ada b@example.test":   false,
	"ada@exam ple.test":    false,
	"ada@exam\tple.test":   false,
	"ada@-example.test":    false,
	"ada@example-.test":    false,
	"ada@example_two.test": false,
	"ada@exam/ple.test":    false,
	// A top label has to begin with a letter and end with a letter or a digit.
	"ada@example.42":   false,
	"ada@example.1st":  false,
	"ada@example.co-":  false,
	"ada@example.-co":  false,
	"ada@example.xn--": false,
	// One label is no domain, however it is written.
	"ada@xn--p1ai": false,
	// A local part that another reader would read to the quotation mark.
	`"ada b"@example.test`: false,
	`"ada"@example.test`:   false,
	`"ada.b"@example.test`: false,
	`ada"b@example.test`:   false,
	// And the whole of an address inside a pair of them, which is an address
	// for nobody until the pair comes off.
	`"ada@example.test"`:  false,
	"ada,b@example.test":  false,
	".ada@example.test":   false,
	"ada.@example.test":   false,
	"ada..b@example.test": false,
	// Outside ASCII on either side of the at sign.
	"müller@example.test": false,
	"ada@exämple.test":    false,
	"ada@example.tëst":    false,
	// The glyphs a source may put in the place of an address it hides.
	"a••@example.test":  false,
	"a●●@●●●.●●":        false,
	"a∙∙@∙∙∙.∙∙":        false,
	"a***@*******.****": false,
}

// TestAnAddressIsReadAsMinkaReadsIt holds the rule against Minka's in both
// directions, line by line.
func TestAnAddressIsReadAsMinkaReadsIt(t *testing.T) {
	for address, want := range minkaTakes {
		if got := minkaTakesTheAddress(address); got != want {
			t.Errorf("minkaTakesTheAddress(%q) = %v, and Minka answers %v", address, got, want)
		}
	}
}

// The lengths are Minka's too, measured at the character that decides.
func TestAnAddressIsAsLongAsMinkaAllows(t *testing.T) {
	label63 := strings.Repeat("b", 63)
	for address, want := range map[string]bool{
		strings.Repeat("a", 64) + "@e.xy":      true,
		strings.Repeat("a", 65) + "@e.xy":      false,
		"a@" + label63 + ".xy":                 true,
		"a@" + strings.Repeat("b", 64) + ".xy": false,
		// 254 characters and one more: the whole of an address, in labels a
		// domain can hold.
		strings.Repeat("a", 64) + "@" + label63 + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 57) + ".com": true,
		strings.Repeat("a", 64) + "@" + label63 + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 58) + ".com": false,
	} {
		if got := minkaTakesTheAddress(address); got != want {
			t.Errorf("an address of %d characters: %v, want %v", len(address), got, want)
		}
	}
}

// The one shape Minka takes and a run leaves out, on purpose: a domain written
// as a bracketed literal address of a machine, in either of its two spellings.
// Nobody's mailbox is written that way, and reading one exactly as PHP reads it
// is a risk in the direction that stops a run.
func TestAnAddressAtABracketedMachineIsLeftOutAlthoughMinkaTakesIt(t *testing.T) {
	for _, address := range []string{"ada@[192.168.1.1]", "ada@[IPv6:::1]"} {
		if minkaTakesTheAddress(address) {
			t.Errorf("%q came through, and the rule says it does not", address)
		}
	}
}

// A pair of quotation marks around the whole of an address is the one thing a
// run repairs rather than passes on, and it comes off before the address is
// read: the reading alone would turn away every one of them.
func TestAPairOfQuotationMarksAroundAWholeAddressComesOff(t *testing.T) {
	for given, want := range map[string]string{
		`"ada@example.test"`:     "ada@example.test",
		`  "ada@example.test"  `: "ada@example.test",
	} {
		address, unquoted, anAddress := anAddressMinkaTakes(given)
		if !anAddress || address != want || !unquoted {
			t.Errorf("%q came out as %q (unquoted %v, an address %v), want %q", given, address, unquoted, anAddress, want)
		}
	}

	// The whitespace around an address comes off here as it comes off at the
	// far end, where Minka trims before it reads.
	for _, given := range []string{" ada@example.test", "ada@example.test\n", "\tada@example.test "} {
		if address, _, anAddress := anAddressMinkaTakes(given); !anAddress || address != "ada@example.test" {
			t.Errorf("%q came out as %q (an address %v)", given, address, anAddress)
		}
	}

	// Nothing else is taken off, and nothing else is said to have been.
	for _, given := range []string{"ada@example.test", "  ada@example.test  ", `"ada b"@example.test`, `"ada"@example.test`} {
		address, unquoted, _ := anAddressMinkaTakes(given)
		if unquoted {
			t.Errorf("%q was said to have come out of quotation marks, and it carries none around the whole of it", given)
		}
		if address != "" && address != strings.TrimSpace(given) {
			t.Errorf("%q came out as %q, and nothing but the whitespace comes off it", given, address)
		}
	}

	// A pair that saves nothing saves nothing: what is inside has to be an
	// address of its own, and a run that took the marks off and wrote the rest
	// would put a sender into the bundle that Minka refuses all the same.
	//
	// And it is one pair, around the whole of it: a mark at one end only is no
	// pair, and a second pair is not the one a header writes. Both of those are
	// addresses Minka refuses and a wider repair would carry.
	for _, given := range []string{`"ada at example"`, `"ada@localhost"`, `""`, `"`,
		`ada@example.test"`, `"ada@example.test`, `""ada@example.test""`} {
		address, unquoted, anAddress := anAddressMinkaTakes(given)
		if anAddress || address != "" || unquoted {
			t.Errorf("%q came out as %q (unquoted %v, an address %v)", given, address, unquoted, anAddress)
		}
	}
}
