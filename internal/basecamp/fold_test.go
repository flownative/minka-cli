package basecamp

import (
	"slices"
	"testing"
)

// A title is folded the way Minka reads one: a control character it reads as a
// character of Windows-1252 stays, one it removes goes, one it refuses becomes a blank,
// bytes that are no UTF-8 become a blank, and only what PHP's trim takes comes
// off the ends.
func TestATitleIsFoldedTheWayMinkaReadsOne(t *testing.T) {
	for name, c := range map[string]struct {
		value, title string
		why          []string
	}{
		"a quote pasted from Word":                   {"Don\u0092t", "Don\u0092t", nil},
		"an ellipsis pasted from Word":               {"Wait\u0085", "Wait\u0085", nil},
		"a code point Windows-1252 lacks":            {"a\u0081b\u008dc\u008fd\u0090e\u009df", "abcdef", []string{"folded"}},
		"nothing but code points Windows-1252 lacks": {"\u0081 \u009d", "(no title)", []string{"none"}},
		"a delete":                          {"a\u007fb", "a b", []string{"folded"}},
		"bytes that are no UTF-8":           {"caf\xe9 au lait", "caf  au lait", []string{"folded"}},
		"nothing but bytes that are no UTF": {"\xff\xfe", "(no title)", []string{"none"}},
		"a no-break space around it":        {" Plan ", " Plan ", nil},
		"nothing but no-break spaces":       {" 　", " 　", nil},
		"a null and a vertical tab around":  {"\x00\x0bPlan\x0b\x00", "Plan", nil},
	} {
		title, why := aTitleOfAtMost(c.value, noTitle, maxName, foldedToOneLine)
		if title != c.title || !slices.Equal(why, c.why) {
			t.Errorf("%s: %q became %q %v, want %q %v", name, c.value, title, why, c.title, c.why)
		}
	}
}

// The name of a company stands inside another value of a record, where Minka
// reads no character of Windows-1252, so every control character is folded.
func TestANestedNameFoldsEveryControlCharacter(t *testing.T) {
	name, why := aTitleOfAtMost("North\u0092wind", noName, maxName, foldedAsANestedName)
	if name != "North wind" || !slices.Equal(why, []string{"folded"}) {
		t.Errorf("the name became %q %v", name, why)
	}
}
