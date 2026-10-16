package fakeapi

import (
	"html"
	"regexp"

	"github.com/flownative/minka-cli/internal/richtext"
)

// referenceElement is a reference to another record as a stored text carries
// it: its attributes and nothing inside.
var (
	referenceElement   = regexp.MustCompile(`<minka-ref\b([^>]*)>\s*</minka-ref>`)
	referenceAttribute = regexp.MustCompile(`\b(kind|id|anchor)="([^"]*)"`)
)

// servedReferences gives every reference to a document or an upload what the
// API gives it to be shown by: title and address, a document's at the
// section it names. A target the reader cannot see keeps what is stored.
// References to other kinds of record stay as they are stored; a test that
// needs one served writes it so.
func (s *Server) servedReferences(text string, person *Person) string {
	return referenceElement.ReplaceAllStringFunc(text, func(element string) string {
		attributes := map[string]string{}
		for _, match := range referenceAttribute.FindAllStringSubmatch(element, -1) {
			attributes[match[1]] = html.UnescapeString(match[2])
		}
		var title, address, anchor string
		switch attributes["kind"] {
		case "document":
			tool, document := s.visibleDocument(attributes["id"], person)
			if document == nil {
				return element
			}
			title, address = document.Title, s.documentURL(tool, document)
			if richtext.IsAnchor(attributes["anchor"]) {
				anchor = attributes["anchor"]
				address += "#" + anchor
			}
		case "upload":
			tool, upload := s.visibleFilesUpload(attributes["id"], person)
			if upload == nil {
				return element
			}
			title, address = upload.Title, s.filesUploadURL(tool, upload)
		default:
			return element
		}
		served := `<minka-ref kind="` + attributes["kind"] + `" id="` + html.EscapeString(attributes["id"]) + `"`
		if anchor != "" {
			served += ` anchor="` + anchor + `"`
		}
		return served + ` title="` + html.EscapeString(title) + `" href="` + html.EscapeString(address) + `"></minka-ref>`
	})
}
