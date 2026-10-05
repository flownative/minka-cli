package basecamp

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// TextCleaned names a record whose text the run took control characters out
// of. It says nothing of the text.
type TextCleaned struct {
	// Record is the kind of record, `comment` or `line`, SourceID which one.
	Record   string `json:"record"`
	SourceID string `json:"sourceId"`
}

// refusedInABody is a character Minka refuses in a comment and a line of a
// chat: a control character other than a tab and a line break. Its reader has
// read the code points between U+0080 and U+009F as Windows-1252 before, so
// what is left of them is none.
func refusedInABody(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f
}

// withoutControlCharacters is a text of a comment or a line with every
// character Minka refuses there taken out, and says whether Minka would have
// refused it: Minka looks at the text without what PHP's trim takes off its
// ends, so a null at the very end is taken out and refused by nobody.
func withoutControlCharacters(text string) (string, bool) {
	if !strings.ContainsFunc(text, refusedInABody) {
		return text, false
	}
	refused := strings.ContainsFunc(strings.Trim(text, minkaTrims), refusedInABody)
	return strings.Map(func(r rune) rune {
		if refusedInABody(r) {
			return -1
		}
		return r
	}, text), refused
}

// noteTextCleaned names a record whose text Minka would have refused for a
// control character.
func (e *Extractor) noteTextCleaned(record, id string) {
	e.textsCleaned = append(e.textsCleaned, TextCleaned{Record: record, SourceID: id})
}

// maxProjectDescription is how long the description of a project may be in
// Minka, in characters of the plain text it makes of it.
const maxProjectDescription = 2000

// aDescriptionMinkaTakes is the plain-text description of a project, cut where
// Minka would refuse it, and says whether it was cut.
//
// Minka reads the HTML a bundle carries back into plain text and measures
// that, so the text is measured as Minka draws the HTML textToHTML makes of it:
// a paragraph per block and a line per `<br>`, every run of whitespace within a
// line one blank, the blanks at either end of a line off — a line of nothing
// but whitespace stays as an empty line — two line breaks after a paragraph,
// never more than two in a row, and the line breaks around it all off. Only
// ASCII whitespace is run together here. Minka runs a no-break space and the
// other Unicode spaces together as well, so such a text counts longer here than
// there, never shorter.
//
// A text too long is cut after the last word that fits and ends in an ellipsis
// within the limit. What is cut is the plain text as measured, which textToHTML
// turns back into HTML Minka draws as no longer than that.
func aDescriptionMinkaTakes(text string) (string, bool) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	measured := asMinkaDrawsIt(text)
	if utf8.RuneCountInString(measured) <= maxProjectDescription {
		return text, false
	}
	cut := []rune(measured)[:maxProjectDescription-utf8.RuneCountInString(ellipsis)]
	// The word the cut fell into goes, unless there is no word boundary to
	// fall back on at all.
	if at := strings.LastIndexFunc(string(cut), asciiSpace); at > 0 && !asciiSpace([]rune(measured)[len(cut)]) {
		cut = []rune(string(cut)[:at])
	}
	return strings.TrimRightFunc(string(cut), asciiSpace) + ellipsis, true
}

// moreThanOneEmptyLine is what Minka draws as one empty line.
var moreThanOneEmptyLine = regexp.MustCompile(`\n{3,}`)

// asMinkaDrawsIt is the plain text Minka makes of the HTML textToHTML makes of
// text, with ASCII whitespace alone run together.
func asMinkaDrawsIt(text string) string {
	if text == "" {
		return ""
	}
	var drawn strings.Builder
	for _, paragraph := range paragraphBreak.Split(text, -1) {
		for i, line := range strings.Split(strings.TrimSpace(paragraph), "\n") {
			if i > 0 {
				drawn.WriteString("\n")
			}
			drawn.WriteString(strings.Join(strings.FieldsFunc(line, asciiSpace), " "))
		}
		drawn.WriteString("\n\n")
	}
	return strings.Trim(moreThanOneEmptyLine.ReplaceAllString(drawn.String(), "\n\n"), minkaTrims)
}

func asciiSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r'
}
