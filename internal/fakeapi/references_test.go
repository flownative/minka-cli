package fakeapi

import (
	"net/http"
	"testing"
)

// A reference to a document or an upload is served as Minka serves it: with
// title and address, a document's at the section it names; one the reader
// cannot see, and every other kind, as stored.
func TestAReferenceIsServedWithItsTitleAndAddress(t *testing.T) {
	s, f := New(t)
	target, upload := f.DocumentInFolder, f.FilesUpload
	f.Document.Body = `<p><minka-ref kind="document" id="` + target.ID + `" anchor="h-prices"></minka-ref>` +
		`<minka-ref kind="document" id="` + target.ID + `" anchor="comment-1"></minka-ref>` +
		`<minka-ref kind="upload" id="` + upload.ID + `" anchor="h-prices"></minka-ref>` +
		`<minka-ref kind="document" id="` + f.OtherDocument.ID + `" anchor="h-x"></minka-ref>` +
		`<minka-ref kind="card" id="` + f.OtherCard.ID + `"></minka-ref></p>`

	status, answer := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/documents/"+f.Document.ID, "")
	if status != "200" {
		t.Fatalf("status %s", status)
	}
	page := s.URL + "/projects/" + f.Project.ID + "/files/" + f.Files.ID + "/" + target.ID
	want := `<p><minka-ref kind="document" id="` + target.ID + `" anchor="h-prices" title="What the tent costs" href="` + page + `#h-prices"></minka-ref>` +
		`<minka-ref kind="document" id="` + target.ID + `" title="What the tent costs" href="` + page + `"></minka-ref>` +
		`<minka-ref kind="upload" id="` + upload.ID + `" title="site-plan.pdf" href="` + s.URL + "/projects/" + f.Project.ID + "/files/" + f.Files.ID + "/uploads/" + upload.ID + `"></minka-ref>` +
		`<minka-ref kind="document" id="` + f.OtherDocument.ID + `" anchor="h-x"></minka-ref>` +
		`<minka-ref kind="card" id="` + f.OtherCard.ID + `"></minka-ref></p>`
	if answer["body"] != want {
		t.Fatalf("body\n got: %s\nwant: %s", answer["body"], want)
	}
}
