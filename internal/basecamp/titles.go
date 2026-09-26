package basecamp

import (
	"fmt"
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

// aTitleMinkaTakes makes a title out of what the source holds where what it
// holds is nobody's to put right, and says whether there was nothing to make one
// of and whether it had to be changed. whenThereIsNone is the title for the
// first case.
//
// A title is one line, not empty, of at most maxName characters and free of
// characters nobody can see. The subject of a mail is whatever a mail server let
// through and the name of a file is whatever a file system allowed, and neither
// is anybody's to repair at the source, so it is made into a title here rather
// than costing the record or the run: every character nobody can see becomes a
// blank, what is left is trimmed, and what is longer than a title holds is cut
// before the last character and ends in an ellipsis. The title of a card, a post
// or a document still ends the run: that one somebody can repair.
func aTitleMinkaTakes(value, whenThereIsNone string) (title string, withoutOne, changed bool) {
	// A value that is not UTF-8 is a title Minka reads as one long control
	// character, whatever else is in it.
	folded := strings.ToValidUTF8(value, " ")
	folded = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, folded)
	changed = folded != value
	folded = strings.TrimSpace(folded)
	if folded == "" {
		return whenThereIsNone, true, false
	}
	if utf8.RuneCountInString(folded) <= maxName {
		return folded, false, changed
	}
	cut := []rune(folded)[:maxName-utf8.RuneCountInString(ellipsis)]
	return strings.TrimRightFunc(string(cut), unicode.IsSpace) + ellipsis, false, true
}
