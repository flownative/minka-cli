package basecamp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"
)

const (
	embedTag       = "bc-attachment"
	mentionType    = "application/vnd.basecamp.mention"
	horizontalRule = "application/vnd.basecamp.horizontal-rule.html"
)

// bodyFile is a file Basecamp lists next to the text it is embedded in. Its ID
// is the one of the attachment, which is no recording.
type bodyFile struct {
	ID             int64  `json:"id"`
	SGID           string `json:"sgid"`
	AttachableSGID string `json:"attachable_sgid"`
	// Key names the stored file, and stands in the address it is fetched from.
	Key string `json:"key"`
}

// BodyCounts says what became of the embeds in the texts of a run.
type BodyCounts struct {
	// MentionsUnresolved are mentions that stayed a name in the text.
	MentionsUnresolved int
	// Files are the references to a file that were written.
	Files int
	// FilesDropped are embedded files Basecamp lists no file for.
	FilesDropped int
}

// rewriteBody turns the HTML of Basecamp into the HTML of a bundle. A bundle
// never carries the embeds of its source: a mention becomes an
// <import-mention>, a file an <import-attachment>, a rule an <hr>, each in the
// place of the whole embed with what Basecamp rendered into it. Every other
// token passes through with the bytes it was read from, because the applier
// sanitises a body anyway and every change here would be a second opinion.
// What goes besides an embed is what belongs to one: whitespace between an
// embed and its rendering, an end tag of an embed without its beginning, and
// a tag the text breaks off in.
//
// The text is read token by token and never built into a tree: a tree moves a
// figure out of a paragraph, and the embed would lose what belongs to it.
//
// rendered is the same text as Basecamp renders it, where the text itself is
// the stored one and shows neither the name nor the avatar of a mention.
func (e *Extractor) rewriteBody(text, rendered string, files []bodyFile) (string, error) {
	if !strings.Contains(strings.ToLower(text), embedTag) {
		return text, nil
	}

	var renderings map[string]rendering
	renderingOf := func(sgid string, inline rendering) rendering {
		// A stored text shows neither the caption of a file nor who is mentioned.
		complete := inline.name() != "" && inline.personID != ""
		if complete || rendered == "" || sgid == "" {
			return inline
		}
		if renderings == nil {
			renderings = renderingsBySGID(rendered)
		}
		return inline.orElse(renderings[sgid])
	}

	var out strings.Builder
	reader := newBodyReader(text)
	// Embeds that pass through untouched, so that their end tags do as well.
	passing := 0
	for {
		token, more := reader.next()
		if !more {
			return out.String(), nil
		}
		if token.kind == nethtml.EndTagToken && token.name == embedTag {
			// An end tag without its embed is a leftover of one that was replaced.
			if passing > 0 {
				passing--
				out.WriteString(token.raw)
			}
			continue
		}
		if !token.opens(embedTag) {
			out.WriteString(token.raw)
			continue
		}

		sgid, contentType := token.attribute("sgid"), mediaType(token.attribute("content-type"))
		if contentType != mentionType && contentType != horizontalRule && sgid == "" {
			// One that closes itself has no end tag to let through.
			if token.kind == nethtml.StartTagToken {
				passing++
			}
			out.WriteString(token.raw)
			continue
		}
		// An embed that closes itself holds nothing: what follows it is text.
		var seen rendering
		if token.kind == nethtml.StartTagToken {
			seen = reader.skipRendering()
		}
		switch {
		case contentType == mentionType:
			replacement, err := e.mention(sgid, token.attribute("content"), renderingOf(sgid, seen))
			if err != nil {
				return "", err
			}
			out.WriteString(replacement)
		case contentType == horizontalRule:
			out.WriteString("<hr>")
		default:
			if file, listed := embeddedFile(files, token); listed {
				e.bodies.Files++
				fmt.Fprintf(&out, `<import-attachment ref="attachment-%d"></import-attachment>`, file.ID)
			} else {
				// The caption is all that is left of such a file.
				e.bodies.FilesDropped++
				out.WriteString(html.EscapeString(renderingOf(sgid, seen).name()))
			}
		}
	}
}

// mediaType is a content type without its parameters, in lower case.
func mediaType(contentType string) string {
	contentType, _, _ = strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(contentType))
}

// embeddedFile finds the file Basecamp lists for an embed. Only a listed file
// counts, because it is what says where the file is fetched from.
//
// The signed ID of an embed is the one of its day, and the list carries the
// one of today, so the two differ for an older text. Then the key of the
// stored file decides, which stands in the address of the embed, and last the
// ID of the attachment the signed ID names.
func embeddedFile(files []bodyFile, embed bodyToken) (bodyFile, bool) {
	sgid := embed.attribute("sgid")
	key := blobKey(embed.attribute("href"))
	if key == "" {
		key = blobKey(embed.attribute("url"))
	}
	attachment, named := gidIn(sgid, attachmentGID)

	matches := []func(bodyFile) bool{
		func(file bodyFile) bool { return file.SGID == sgid || file.AttachableSGID == sgid },
		func(file bodyFile) bool { return key != "" && file.Key == key },
		func(file bodyFile) bool { return named && file.ID == attachment },
	}
	for _, match := range matches {
		for _, file := range files {
			if file.ID != 0 && match(file) {
				return file, true
			}
		}
	}
	return bodyFile{}, false
}

// blobKey takes the key of a stored file out of the address it is downloaded
// from: …/blobs/<key>/download/<filename>.
func blobKey(address string) string {
	parsed, err := url.Parse(address)
	if err != nil {
		return ""
	}
	segments := strings.Split(parsed.Path, "/")
	for i := 0; i+2 < len(segments); i++ {
		if segments[i] == "blobs" && segments[i+2] == "download" {
			return segments[i+1]
		}
	}
	return ""
}

// mention writes the reference to somebody who is mentioned. It becomes one
// only for a person the list of the account knows: a mention names nothing but
// an ID, and of anybody else there is no record to write. Such a mention, and
// one that names nobody, stays the name that stood in the text.
func (e *Extractor) mention(sgid, content string, seen rendering) (string, error) {
	if id, found := mentionedPerson(sgid, content, seen); found {
		reference, name, err := e.people.referByID(id)
		if err != nil {
			return "", err
		}
		if reference != nil {
			return fmt.Sprintf(`<import-mention ref="%s">%s</import-mention>`, *reference, html.EscapeString(name)), nil
		}
	}
	e.bodies.MentionsUnresolved++
	return html.EscapeString(seen.name()), nil
}

// mentionedPerson finds the ID of the person a mention names. What Basecamp
// signed counts first, then what it rendered, and last what the editor of
// whoever wrote the text put next to it.
func mentionedPerson(sgid, content string, seen rendering) (int64, bool) {
	if id, found := gidIn(sgid, personGID); found {
		return id, true
	}
	if id, err := strconv.ParseInt(seen.personID, 10, 64); err == nil {
		return id, true
	}
	reader := newBodyReader(content)
	for {
		token, more := reader.next()
		if !more {
			return 0, false
		}
		if token.opens("bc-mention") {
			if id, found := idIn(token.attribute("gid"), personGID); found {
				return id, true
			}
		}
	}
}

// gidIn reads the ID of what a signed global ID names: base64 of what it
// names, then "--" and the signature. What it names is a global ID in an
// envelope, serialised the way Ruby does it. An older one is an envelope of
// JSON that holds the serialised ID as base64 again, in either alphabet. The
// signature is not checked, and cannot be: the ID only ever leads to what a
// list of Basecamp confirms.
func gidIn(sgid string, pattern *regexp.Regexp) (int64, bool) {
	payload := sgid
	if signature := strings.LastIndex(sgid, "--"); signature >= 0 {
		payload = sgid[:signature]
	}
	named, ok := decodeBase64(payload)
	if !ok {
		return 0, false
	}
	if id, found := idIn(string(named), pattern); found {
		return id, true
	}
	var envelope struct {
		Rails struct {
			Message string `json:"message"`
		} `json:"_rails"`
	}
	if json.Unmarshal(named, &envelope) != nil {
		return 0, false
	}
	message, ok := decodeBase64(envelope.Rails.Message)
	if !ok {
		return 0, false
	}
	return idIn(string(message), pattern)
}

func decodeBase64(value string) ([]byte, bool) {
	if value == "" {
		return nil, false
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, true
		}
	}
	return nil, false
}

// A global ID stands alone or in the middle of what was serialised: a byte
// that says how long it is before it, its parameters or the next field after
// it. So it may not go on as a word before, nor as a number or a path after.
var (
	personGID     = globalID("Person")
	attachmentGID = globalID("Attachment")
)

func globalID(model string) *regexp.Regexp {
	return regexp.MustCompile(`(?:^|[^A-Za-z])gid://bc3/` + model + `/([0-9]+)(?:$|[^0-9A-Za-z/])`)
}

func idIn(text string, pattern *regexp.Regexp) (int64, bool) {
	match := pattern.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	id, err := strconv.ParseInt(match[1], 10, 64)
	return id, err == nil
}

// rendering is what Basecamp rendered into an embed: for a mention the avatar,
// which names the person, and the name as a caption.
type rendering struct {
	personID string
	caption  string
	text     string
}

func (r rendering) name() string {
	if name := strings.Join(strings.Fields(r.caption), " "); name != "" {
		return name
	}
	return strings.Join(strings.Fields(r.text), " ")
}

func (r rendering) orElse(other rendering) rendering {
	if r.personID == "" {
		r.personID = other.personID
	}
	if r.name() == "" {
		r.caption, r.text = other.caption, other.text
	}
	return r
}

func renderingsBySGID(rendered string) map[string]rendering {
	renderings := map[string]rendering{}
	reader := newBodyReader(rendered)
	for {
		token, more := reader.next()
		if !more {
			return renderings
		}
		if sgid := token.attribute("sgid"); token.opens(embedTag) && sgid != "" {
			renderings[sgid] = reader.skipRendering()
		}
	}
}

type bodyToken struct {
	kind nethtml.TokenType
	raw  string
	name string
	attr []nethtml.Attribute
}

// opens tells whether the token is the start of such an element.
func (t bodyToken) opens(name string) bool {
	return (t.kind == nethtml.StartTagToken || t.kind == nethtml.SelfClosingTagToken) && t.name == name
}

func (t bodyToken) attribute(key string) string {
	for _, attribute := range t.attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

// bodyReader hands out the tokens of a text, each with the bytes it was read
// from, and lets a caller look at the next one before taking it.
type bodyReader struct {
	tokenizer *nethtml.Tokenizer
	peeked    *bodyToken
}

func newBodyReader(text string) *bodyReader {
	return &bodyReader{tokenizer: nethtml.NewTokenizer(strings.NewReader(text))}
}

func (r *bodyReader) next() (bodyToken, bool) {
	if r.peeked != nil {
		token := *r.peeked
		r.peeked = nil
		return token, true
	}
	kind := r.tokenizer.Next()
	if kind == nethtml.ErrorToken {
		return bodyToken{}, false
	}
	token := bodyToken{kind: kind, raw: string(r.tokenizer.Raw())}
	if kind == nethtml.StartTagToken || kind == nethtml.SelfClosingTagToken || kind == nethtml.EndTagToken {
		parsed := r.tokenizer.Token()
		token.name, token.attr = parsed.Data, parsed.Attr
	}
	return token, true
}

func (r *bodyReader) peek() (bodyToken, bool) {
	if r.peeked == nil {
		token, more := r.next()
		if !more {
			return bodyToken{}, false
		}
		r.peeked = &token
	}
	return *r.peeked, true
}

// skipRendering takes what Basecamp rendered into the embed that was just
// opened, and its end tag. That is a figure or a rule and nothing else, so an
// embed that is never closed costs no more of the text than that.
func (r *bodyReader) skipRendering() rendering {
	var seen rendering
	for {
		token, more := r.peek()
		switch {
		case !more:
			return seen
		case token.kind == nethtml.TextToken && strings.TrimSpace(token.raw) == "":
			r.next()
		case token.opens("hr"):
			r.next()
		case token.opens("figure"):
			r.next()
			r.skipFigure(&seen)
		case token.kind == nethtml.EndTagToken && token.name == embedTag:
			r.next()
			return seen
		default:
			return seen
		}
	}
}

// skipFigure takes a figure up to its end. A figure that is never closed ends
// where its embed ends or the next one begins.
func (r *bodyReader) skipFigure(seen *rendering) {
	depth, inCaption := 1, false
	for {
		token, more := r.peek()
		if !more || token.name == embedTag {
			return
		}
		r.next()
		switch {
		case token.opens("figure") && token.kind == nethtml.StartTagToken:
			depth++
		case token.kind == nethtml.EndTagToken && token.name == "figure":
			if depth--; depth == 0 {
				return
			}
		case token.opens("img") && seen.personID == "":
			seen.personID = token.attribute("data-avatar-for-person-id")
		case token.name == "figcaption":
			inCaption = token.kind == nethtml.StartTagToken
		case token.kind == nethtml.TextToken:
			text := html.UnescapeString(token.raw)
			seen.text += text
			if inCaption {
				seen.caption += text
			}
		}
	}
}
