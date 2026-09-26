package basecamp

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// maxName is the length of a title and of a company's name Minka takes, in
	// characters.
	maxName = 200
	// ellipsis says that the rest of a title is not there.
	ellipsis = "…"
)

// itemTitle is the title of a record as a bundle carries it, without the
// whitespace around it that Minka takes off anyway. A title Minka refuses ends
// the run instead of reaching it, as it would end the applier's. No message
// quotes a title: it is what somebody wrote.
func itemTitle(what string, id int64, value string) (string, error) {
	title := strings.TrimSpace(value)
	switch {
	case title == "":
		return "", &Error{Kind: API, Message: fmt.Sprintf("%s %d has no title", what, id)}
	case utf8.RuneCountInString(title) > maxName:
		return "", &Error{Kind: API, Message: fmt.Sprintf("the title of %s %d is longer than the %d characters a title may have", what, id, maxName)}
	case strings.ContainsFunc(title, unicode.IsControl):
		return "", &Error{Kind: API, Message: fmt.Sprintf("the title of %s %d holds a character nobody can see", what, id)}
	}
	return title, nil
}

// aTitleMinkaTakes makes a title out of the subject of a mail, which is whatever
// a mail server let through and nobody's to put right at the source, and says
// whether there was nothing to make one of and whether the subject had to be
// changed. whenThereIsNone is the title for the first case.
//
// A title is one line, not empty, of at most maxName characters and free of
// characters nobody can see. So every character nobody can see becomes a blank,
// what is left is trimmed, and what is longer than a title holds is cut before
// the last character and ends in an ellipsis. The title of a card, a post, a
// folder or a document still ends the run: that one somebody can repair where it
// stands.
func aTitleMinkaTakes(value, whenThereIsNone string) (title string, withoutOne, changed bool) {
	folded, changed := foldedToOneLine(value)
	if folded == "" {
		return whenThereIsNone, true, false
	}
	if utf8.RuneCountInString(folded) <= maxName {
		return folded, false, changed
	}
	cut := []rune(folded)[:maxName-utf8.RuneCountInString(ellipsis)]
	return strings.TrimRightFunc(string(cut), unicode.IsSpace) + ellipsis, false, true
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
		return strings.TrimSpace(string(characters[:maxName])), false, true
	}
	stem := characters[:len(characters)-len(extension)]
	return strings.TrimSpace(string(stem[:room]) + string(extension)), false, true
}

// foldedToOneLine is what the source holds as the one line a title has to be:
// every character nobody can see a blank, bytes that are no UTF-8 a blank as
// well, and the whitespace around it off. It says whether anything had to be
// folded, which the whitespace around a value is not: Minka trims a name before
// it reads it, so a value that only needed trimming is a title as it stands.
func foldedToOneLine(value string) (folded string, changed bool) {
	// A value that is not UTF-8 is a title Minka reads as one long control
	// character, whatever else is in it.
	folded = strings.ToValidUTF8(value, " ")
	folded = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, folded)
	return strings.TrimSpace(folded), strings.TrimSpace(folded) != strings.TrimSpace(value)
}
