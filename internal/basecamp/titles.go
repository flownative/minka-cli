package basecamp

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// maxName is the length of a title and of a name Minka takes, in
	// characters.
	maxName = 200
	// ellipsis says that the rest of a title is not there.
	ellipsis = "…"
)

// noTitle is what a record is called whose title is nothing at all, nothing
// but whitespace or nothing but characters nobody can see.
const noTitle = "(no title)"

// maxStep is the length of a step Minka takes, in characters: a step is a
// sentence and not a name, and gets the room of one.
const maxStep = 500

// TitleMadeToFit names a record whose title the run had to make into one Minka
// takes. It says nothing of the title itself: that is what somebody wrote.
type TitleMadeToFit struct {
	// Record is the kind of record, SourceID which one.
	Record   string `json:"record"`
	SourceID string `json:"sourceId"`
	// Why is what had to be done: `none` where there was nothing to make a
	// title of, `folded` where a character nobody can see became a blank and
	// `cut` where it was longer than a title holds; the last two can come
	// together.
	Why []string `json:"why"`
}

// titleOf is the title of a record as a bundle carries it, made into one Minka
// takes and named in the report where it had to be. A title is somebody's to
// put right at the source, but a walk of hours that ended on one character
// nobody can see would never bring an account across; the record comes, and
// the report says which one to look at.
func (e *Extractor) titleOf(what string, id int64, value string) string {
	return e.aTitleThatFits(what, id, value, noTitle, maxName)
}

// nameOf is titleOf for what Minka calls a name: of a person, a company, a
// project or a category of posts.
func (e *Extractor) nameOf(what string, id int64, value string) string {
	return e.aTitleThatFits(what, id, value, noName, maxName)
}

// stepTitleOf is titleOf for a step, which holds more than a title does.
func (e *Extractor) stepTitleOf(id int64, value string) string {
	return e.aTitleThatFits("step", id, value, noTitle, maxStep)
}

func (e *Extractor) aTitleThatFits(what string, id int64, value, whenThereIsNone string, longest int) string {
	title, why := aTitleOfAtMost(value, whenThereIsNone, longest, foldedToOneLine)
	e.noteMadeToFit(what, id, why)
	return title
}

// noteMadeToFit names a record whose title took what why says, once however
// many records carry it: a company stands on every client of it.
func (e *Extractor) noteMadeToFit(what string, id int64, why []string) {
	if len(why) == 0 {
		return
	}
	for _, one := range e.titlesMadeToFit {
		if one.Record == what && one.SourceID == sourceID(id) {
			return
		}
	}
	e.titlesMadeToFit = append(e.titlesMadeToFit, TitleMadeToFit{Record: what, SourceID: sourceID(id), Why: why})
}

// aTitleOfAtMost makes a value into a title of at most longest characters,
// folded to one line Minka takes by fold, and cut before the last character
// with an ellipsis where it is longer. It says what it had to do — `none`,
// `folded`, `cut` — and nothing where the value was a title as it stands.
func aTitleOfAtMost(value, whenThereIsNone string, longest int, fold func(string) (string, bool)) (string, []string) {
	folded, changed := fold(value)
	if folded == "" {
		return whenThereIsNone, []string{"none"}
	}
	var why []string
	if changed {
		why = append(why, "folded")
	}
	if utf8.RuneCountInString(folded) <= longest {
		return folded, why
	}
	cut := []rune(folded)[:longest-utf8.RuneCountInString(ellipsis)]
	return strings.TrimRightFunc(string(cut), unicode.IsSpace) + ellipsis, append(why, "cut")
}

// aTitleMinkaTakes makes a title out of the subject of a mail, as
// aTitleOfAtMost makes one, and says whether there was nothing to make one of
// and whether the subject had to be changed. whenThereIsNone is the title for
// the first case.
func aTitleMinkaTakes(value, whenThereIsNone string) (title string, withoutOne, changed bool) {
	title, why := aTitleOfAtMost(value, whenThereIsNone, maxName, foldedToOneLine)
	if len(why) == 1 && why[0] == "none" {
		return title, true, false
	}
	return title, false, len(why) > 0
}

// theExtensionOfAName is what Minka counts as the extension of a file name when
// it shortens one: a dot and up to eight letters or digits at the end.
var theExtensionOfAName = regexp.MustCompile(`\.[A-Za-z0-9]{1,8}$`)

// aFileNameAsATitle makes a title out of the name of a file, which is whatever a
// file system allowed and nobody's to put right at the source either, and says
// the same two things aTitleMinkaTakes says.
//
// A name too long is shortened **the way Minka shortens one** for an upload
// somebody makes there (`ItemTitle::fromFilename`): the extension stays and the
// stem before it is cut to make room, with no ellipsis, and a name that is
// nothing but an extension is cut like any other text. So one file carries the
// same title whether somebody uploaded it there or a run brought it here, which
// a rule of our own would break for every name over two hundred characters.
func aFileNameAsATitle(name, whenThereIsNone string) (title string, withoutOne, changed bool) {
	folded, changed := foldedToOneLine(name)
	if folded == "" {
		return whenThereIsNone, true, false
	}
	if utf8.RuneCountInString(folded) <= maxName {
		return folded, false, changed
	}
	characters := []rune(folded)
	extension := []rune(theExtensionOfAName.FindString(folded))
	room := maxName - len(extension)
	if room <= 0 {
		return strings.Trim(string(characters[:maxName]), minkaTrims), false, true
	}
	stem := characters[:len(characters)-len(extension)]
	return strings.Trim(string(stem[:room])+string(extension), minkaTrims), false, true
}

// minkaTrims is what Minka takes off both ends of a name before it reads it:
// what PHP's trim takes, and no other whitespace, so a name of nothing but
// no-break spaces is a name there.
const minkaTrims = " \t\n\r\x00\x0b"

// undefinedInWindows1252 are the five code points between U+0080 and U+009F
// Windows-1252 has no character for. Minka removes them from a text of a bundle
// and reads the other 27 as the characters of Windows-1252, a dash or a quote
// pasted from Word, before it holds the text to any rule.
var undefinedInWindows1252 = map[rune]bool{0x81: true, 0x8d: true, 0x8f: true, 0x90: true, 0x9d: true}

// unseenInAText is a character Minka refuses in a name that stands as a text of
// its own in a bundle: a control character it does not read as one of
// Windows-1252.
func unseenInAText(r rune) bool {
	return r < 0x20 || r == 0x7f || undefinedInWindows1252[r]
}

// foldedToOneLine is what the source holds as the one line a title has to be:
// every character Minka would refuse a blank, bytes that are no UTF-8 a blank as
// well, and what Minka trims off. A character of Windows-1252 written as a
// control character stays: Minka reads it as that character, one for one, so
// the length holds; one Windows-1252 has none for goes, as Minka removes it.
// It says whether anything had to be folded, which what Minka trims is not: a
// value that only needed trimming is a title as it stands.
func foldedToOneLine(value string) (folded string, changed bool) {
	return foldedBy(value, func(r rune) rune {
		switch {
		case undefinedInWindows1252[r]:
			return -1
		case unseenInAText(r):
			return ' '
		}
		return r
	})
}

// foldedAsANestedName is foldedToOneLine for a name that stands inside another
// value of a record, the name of a company: Minka reads no character of
// Windows-1252 there, so every control character is one it refuses.
func foldedAsANestedName(value string) (folded string, changed bool) {
	return foldedBy(value, func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	})
}

// foldedBy folds a value by fold, which says of every character what becomes
// of it: itself, a blank, or nothing.
func foldedBy(value string, fold func(rune) rune) (folded string, changed bool) {
	// A value that is not UTF-8 is a title Minka reads as one long control
	// character, whatever else is in it.
	folded = strings.Map(fold, strings.ToValidUTF8(value, " "))
	folded = strings.Trim(folded, minkaTrims)
	return folded, folded != strings.Trim(value, minkaTrims)
}
