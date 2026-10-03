package basecamp

import (
	"html"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/bundle"
	nethtml "golang.org/x/net/html"
)

// ReferenceCounts are the links of the texts of a run that became an
// <import-reference>, by the kind of record they point at.
type ReferenceCounts struct {
	Cards int `json:"cards"`
	Posts int `json:"posts"`
	// Tools are the links to a chat, and to a line of it.
	Tools int `json:"tools"`
	// Events are the links to an entry of a calendar, and to a day of it.
	Events int `json:"events"`
}

// webHosts are the hosts Basecamp shows its pages at. Links of one account
// stand at both.
var webHosts = map[string]bool{"3.basecamp.com": true, "app.basecamp.com": true}

// linkTag is the beginning of a link in a text of lower case: the tag name,
// then whitespace, the end of the tag or a slash.
var linkTag = regexp.MustCompile(`<a[\t\n\f\r />]`)

// recordPath is the path of the page of a card, a message, a chat or an entry
// of a calendar. A line of a chat stands at the page of its chat, as
// <chat>@<line> or below it at lines/<line>. What stands below the page of a
// record belongs to it, such as the menu of a line; below an entry only a day
// of it does, see belongsToTheEntry. It is asked of a path whose dot segments
// are resolved.
var recordPath = regexp.MustCompile(`^/([0-9]+)/buckets/[0-9]+/(card_tables/cards|messages|chats|schedule_entries)/([0-9]+)(@[0-9]+)?(/.*)?$`)

// recordKinds are the kinds of record a bundle has for the pages of recordPath.
var recordKinds = map[string]bundle.Kind{
	"card_tables/cards": bundle.Cards,
	"messages":          bundle.Posts,
	"chats":             bundle.Tools,
	"schedule_entries":  bundle.Events,
}

// dayOfAnEntry is the page of one day of an entry that repeats. A day is no
// record of its own, so a link to it names the entry.
var dayOfAnEntry = regexp.MustCompile(`^/occurrences/([0-9]{8})$`)

// belongsToTheEntry says whether what stands below the page of an entry is
// the page of one of its days: a day of the calendar, which the thirtieth of
// February and a day of the year 0 are not. Anything else is no page Basecamp
// writes, so it names no entry.
func belongsToTheEntry(below string) bool {
	if below == "" {
		return true
	}
	day := dayOfAnEntry.FindStringSubmatch(below)
	if day == nil {
		return false
	}
	parsed, err := time.Parse("20060102", day[1])
	return err == nil && parsed.Year() >= firstYear
}

// referenceTo says which record of the account an address shows: a card, a
// post, a chat or an event, the chat also for a line of it and the event for a
// day of it. The record is named by its kind in a bundle and its source ID,
// which is the ID of Basecamp that stands in the address. It need not be in
// the bundle: the applier keeps the link to what it does not know. An address
// of another account, another host or another kind of record shows none.
//
// It reads the address and nothing else: asking Basecamp for an entry or a
// day of it writes at the source.
func (e *Extractor) referenceTo(address string) (bundle.Kind, string, bool) {
	parsed, err := url.Parse(strings.Trim(address, addressPadding))
	if err != nil || parsed.User != nil || !webHosts[strings.ToLower(parsed.Host)] {
		return "", "", false
	}
	// Basecamp writes no address whose path begins with an empty segment.
	if strings.HasPrefix(parsed.Path, "//") {
		return "", "", false
	}
	if scheme := strings.ToLower(parsed.Scheme); scheme != "https" && scheme != "http" {
		return "", "", false
	}
	// A path that climbs out of a page with ".." shows what it climbs to.
	match := recordPath.FindStringSubmatch(path.Clean(parsed.Path))
	if match == nil || match[1] != e.account || match[4] != "" && match[2] != "chats" {
		return "", "", false
	}
	if match[2] == "schedule_entries" && !belongsToTheEntry(match[5]) {
		return "", "", false
	}
	// A number too large for an ID names no record.
	id, err := strconv.ParseInt(match[3], 10, 64)
	if err != nil {
		return "", "", false
	}
	return recordKinds[match[2]], sourceID(id), true
}

// openReference is the start tag of the reference a link to a record becomes,
// with the address as it stood; the words of the link follow it as plain
// text. The kind is counted.
func (e *Extractor) openReference(kind bundle.Kind, ref, address string) string {
	switch kind {
	case bundle.Cards:
		e.bodies.References.Cards++
	case bundle.Posts:
		e.bodies.References.Posts++
	case bundle.Tools:
		e.bodies.References.Tools++
	case bundle.Events:
		e.bodies.References.Events++
	}
	return `<import-reference kind="` + string(kind) + `" ref="` + html.EscapeString(ref) + `" href="` + html.EscapeString(address) + `">`
}

const closeReference = `</import-reference>`

// links turns the links of a text to a record of the account into references,
// with the words of the link between the tags as plain text, see words. Every
// other link passes as it stands. A link ends where the next one begins, as it
// does for a browser, and where the text ends.
type links struct {
	extractor *Extractor
	// referring says that the link that is open became a reference.
	referring bool
	// inWords are the elements that opened within the words of the reference,
	// the innermost last.
	inWords []string
}

// rewrite says what stands in the place of a token of a link, and whether it
// was one.
func (l *links) rewrite(token bodyToken) (string, bool) {
	if token.name != "a" {
		return "", false
	}
	closing := l.end()
	if token.kind == nethtml.EndTagToken {
		if closing == "" {
			closing = token.raw
		}
		return closing, true
	}
	address := token.attribute("href")
	kind, ref, found := l.extractor.referenceTo(address)
	if !found {
		return closing + token.raw, true
	}
	reference := l.extractor.openReference(kind, ref, address)
	if token.kind == nethtml.SelfClosingTagToken {
		return closing + reference + closeReference, true
	}
	l.referring, l.inWords = true, nil
	return closing + reference, true
}

// end closes the reference that is open, if one is.
func (l *links) end() string {
	if !l.referring {
		return ""
	}
	l.referring = false
	return closeReference
}

// words is what a token within the words of a reference gives: its text, and
// nothing of any tag, because a reference holds plain text. A line break
// gives a blank, so that the words on both sides stay apart, and a frame
// gives nothing.
func words(token bodyToken, reader *bodyReader) string {
	switch {
	case token.opens(frameTag):
		reader.skipFrame()
		return ""
	case token.opens("br"):
		return " "
	case token.kind == nethtml.TextToken:
		return html.EscapeString(html.UnescapeString(token.raw))
	}
	return ""
}

// blocks are the elements a link does not reach into. Words are inline text:
// where a block begins or ends, the reference ends before it, and the block
// stays as it was written.
var blocks = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "dd": true, "details": true,
	"div": true, "dl": true, "dt": true, "figcaption": true, "figure": true, "footer": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "header": true,
	"hr": true, "li": true, "main": true, "nav": true, "ol": true, "p": true, "pre": true,
	"section": true, "table": true, "tbody": true, "td": true, "tfoot": true, "th": true,
	"thead": true, "tr": true, "ul": true,
}

// voids are the elements that have no end tag: one written all the same ends
// nothing.
var voids = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

// endAt closes the reference that is open where a block begins or ends, and
// before the end of an element that opened before the link: that end tag
// stays, behind the reference, so that the element ends where it did.
func (l *links) endAt(token bodyToken) string {
	if !l.referring || token.name == "a" {
		return ""
	}
	switch {
	case (token.kind == nethtml.StartTagToken || token.kind == nethtml.SelfClosingTagToken || token.kind == nethtml.EndTagToken) && blocks[token.name]:
		return l.end()
	case voids[token.name]:
	case token.kind == nethtml.StartTagToken:
		l.inWords = append(l.inWords, token.name)
	case token.kind == nethtml.EndTagToken:
		for i := len(l.inWords) - 1; i >= 0; i-- {
			if l.inWords[i] == token.name {
				l.inWords = l.inWords[:i]
				return ""
			}
		}
		return l.end()
	}
	return ""
}

// wordsOf is what an embed within the words of a reference gives, unescaped:
// the name of whom it mentions, the title of a preview, the caption of a
// picture or a file, and nothing of a rule. It refers to nothing, so nobody
// and no file comes into the bundle for it.
//
// The name of a person is the one the list of the account has, which is the
// one a reference to them carries; what Basecamp rendered only where the list
// does not know them.
func (e *Extractor) wordsOf(contentType string, linkedPicture bool, embed bodyToken, seen rendering) string {
	switch {
	case contentType == mentionType:
		if id, found := mentionedPerson(embed.attribute("sgid"), embed.attribute("content"), seen); found {
			if name, listed := e.people.nameOf(id); listed {
				return name
			}
		}
	case contentType == horizontalRule:
		return ""
	case contentType == linkPreview:
		return strings.Join(strings.Fields(embed.attribute("title")), " ")
	case linkedPicture:
		return strings.Join(strings.Fields(embed.attribute("caption")), " ")
	}
	return seen.name()
}
