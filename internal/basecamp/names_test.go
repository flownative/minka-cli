package basecamp

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/bundle"
)

func TestTheNameOfAFileBecomesOneABundleCanCarry(t *testing.T) {
	long := strings.Repeat("n", 300)
	for name, want := range map[string]string{
		"plan.png":             "plan.png",
		"holiday photo (1).ä":  "holiday photo (1).ä",
		".hidden":              ".hidden",
		"a..b":                 "a..b",
		"reports/2023/q1.pdf":  "reports_2023_q1.pdf",
		`c:\files\q1.pdf`:      "c:_files_q1.pdf",
		"line\nbreak\ttab.txt": "line_break_tab.txt",
		"nul\x00byte\x7f.txt":  "nul_byte_.txt",
		"c1\u0085control.txt":  "c1_control.txt",
		"bad\xff\xfeutf8.txt":  "bad_utf8.txt",
		"":                     "file",
		".":                    "file",
		"..":                   "file",
		"/":                    "_",
		long + ".jpeg":         strings.Repeat("n", 250) + ".jpeg",
		long:                   strings.Repeat("n", 255),
		"." + long:             "." + strings.Repeat("n", 254),
		long + "." + long:      strings.Repeat("n", 255),
		// Counted in bytes as well: a file system takes no more.
		strings.Repeat("ä", 200) + ".png": strings.Repeat("ä", 125) + ".png",
	} {
		got, changed := plainName(name)
		if got != want || changed != (name != want) {
			t.Errorf("plainName(%q) = %q, %v; want %q", name, got, changed, want)
		}
		if !bundle.PlainName(got) || len(got) > bundle.MaxNameLength || !utf8.ValidString(got) {
			t.Errorf("plainName(%q) = %q, which a bundle does not take", name, got)
		}
	}
}

func TestTheAddressOfAFileIsBuiltFromTheAccountTheKeyAndTheNameAsTheSourceEscapesIt(t *testing.T) {
	const key = "0a1b2c3d-0000-4000-8000-000000000001"
	e := &Extractor{account: "4242"}
	for name, c := range map[string]struct {
		file sourceFile
		want string
	}{
		"the name as the source escapes it": {
			sourceFile{key: key, filename: "plan A.png", downloadURL: "https://3.basecampapi.test/4242/blobs/" + key + "/download/plan-A.png?disposition=attachment"},
			"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan-A.png"},
		"a name with a slash, where the source writes none": {
			sourceFile{key: key, filename: "a/b.png", downloadURL: "https://3.basecampapi.test/4242/blobs/" + key + "/download/a-b.png"},
			"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/a-b.png"},
		"without an address the name is escaped": {
			sourceFile{key: key, filename: "plan A?\n.png"},
			"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan%20A%3F%0A.png"},
		"an address of another key says nothing": {
			sourceFile{key: key, filename: "plan.png", downloadURL: "https://3.basecampapi.test/4242/blobs/another/download/other.png"},
			"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png"},
		"nor one of another account": {
			sourceFile{key: key, filename: "plan.png", downloadURL: "https://3.basecampapi.test/1/blobs/" + key + "/download/other.png"},
			"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png"},
		"nor one that goes on behind the name": {
			sourceFile{key: key, filename: "plan.png", downloadURL: "https://3.basecampapi.test/4242/blobs/" + key + "/download/a/b.png"},
			"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png"},
		// The host of the source never becomes part of the address.
		"whatever host the source names": {
			sourceFile{key: key, filename: "plan.png", downloadURL: "https://elsewhere.example.test/4242/blobs/" + key + "/download/plan.png"},
			"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png"},
	} {
		got, err := e.storageAddress(c.file)
		if err != nil || got != c.want {
			t.Errorf("%s:\n got %q, %v\nwant %q", name, got, err, c.want)
		}
	}

	const aboutTheKey, aboutTheAddress = "the key of the stored file", "cannot be asked for safely"
	for name, c := range map[string]struct {
		file sourceFile
		says string
	}{
		"a key that is none":     {sourceFile{key: "k-one", filename: "plan.png"}, aboutTheKey},
		"a key that leads away":  {sourceFile{key: "..", filename: "plan.png"}, aboutTheKey},
		"no key":                 {sourceFile{filename: "plan.png"}, aboutTheKey},
		"no name":                {sourceFile{key: key}, aboutTheAddress},
		"a name that leads away": {sourceFile{key: key, filename: ".."}, aboutTheAddress},
		"a name that is a path":  {sourceFile{key: key, filename: "../../../projects.json"}, aboutTheAddress},
		"an address that leads away": {sourceFile{key: key, filename: "plan.png",
			downloadURL: "https://3.basecampapi.test/4242/blobs/" + key + "/download/%2E%2E"}, aboutTheAddress},
		"an address whose name is a path to another account": {sourceFile{key: key, filename: "plan.png",
			downloadURL: "https://3.basecampapi.test/4242/blobs/" + key + "/download/..%2F..%2F..%2F..%2F999%2Fpeople.json"}, aboutTheAddress},
		"an address whose name holds a slash": {sourceFile{key: key, filename: "a/b.png",
			downloadURL: "https://3.basecampapi.test/4242/blobs/" + key + "/download/a%2Fb.png"}, aboutTheAddress},
	} {
		address, err := e.storageAddress(c.file)
		var failed *Error
		if !errors.As(err, &failed) || address != "" || failed.Kind != API || failed.Transient() || !strings.Contains(failed.Message, c.says) {
			t.Errorf("%s: got %q, %v", name, address, err)
			continue
		}
		for _, leaked := range []string{"plan", "projects", "people", "999", "a/b", "k-one"} {
			if strings.Contains(failed.Message, leaked) {
				t.Errorf("%s: the reason quotes %q: %s", name, leaked, failed.Message)
			}
		}
	}
}

func TestAReferenceToAFileCarriesACaptionAndAGalleryAndNothingElse(t *testing.T) {
	longest := strings.Repeat("ä", maxCaptionLength)
	for name, c := range map[string]struct {
		seen    rendering
		want    string
		dropped int
	}{
		"nothing said":          {rendering{}, `<import-attachment ref="attachment-1"></import-attachment>`, 0},
		"a caption":             {rendering{fileCaption: `Tea & "biscuits" <3 'now'`}, `<import-attachment ref="attachment-1" caption="Tea &amp; &#34;biscuits&#34; &lt;3 &#39;now&#39;"></import-attachment>`, 0},
		"a gallery":             {rendering{presentation: "gallery"}, `<import-attachment ref="attachment-1" presentation="gallery"></import-attachment>`, 0},
		"both, in their order":  {rendering{presentation: "gallery", fileCaption: "Tea"}, `<import-attachment ref="attachment-1" caption="Tea" presentation="gallery"></import-attachment>`, 0},
		"another layout":        {rendering{presentation: "fullwidth"}, `<import-attachment ref="attachment-1"></import-attachment>`, 0},
		"a gallery in capitals": {rendering{presentation: "Gallery"}, `<import-attachment ref="attachment-1"></import-attachment>`, 0},
		"an empty caption":      {rendering{fileCaption: " \n "}, `<import-attachment ref="attachment-1"></import-attachment>`, 0},
		"the longest caption":   {rendering{fileCaption: longest}, `<import-attachment ref="attachment-1" caption="` + longest + `"></import-attachment>`, 0},
		"a caption too long":    {rendering{fileCaption: longest + "ä", presentation: "gallery"}, `<import-attachment ref="attachment-1" presentation="gallery"></import-attachment>`, 1},
		// What Basecamp renders under a file is its name and size, no caption.
		"a rendered caption": {rendering{caption: "plan.png 4 KB", text: "plan.png 4 KB"}, `<import-attachment ref="attachment-1"></import-attachment>`, 0},
	} {
		e := &Extractor{}
		if got := e.fileReference("attachment-1", c.seen); got != c.want || e.bodies.CaptionsDropped != c.dropped {
			t.Errorf("%s:\n got %q, %d dropped\nwant %q, %d dropped", name, got, e.bodies.CaptionsDropped, c.want, c.dropped)
		}
	}
}

func TestAStoredTextTakesTheCaptionOfAFileFromTheRenderedOne(t *testing.T) {
	e := bodyExtractor(t)
	stored := `<div><bc-attachment sgid="` + fileSGID + `" content-type="image/png" presentation="gallery"></bc-attachment></div>`
	rendered := `<div><bc-attachment sgid="` + fileSGID + `" content-type="image/png" caption="Tea &amp; biscuits" presentation="gallery"><figure><img src="a"><figcaption>Tea &amp; biscuits</figcaption></figure></bc-attachment></div>`

	got, files, err := e.rewriteBody(aBody, stored, rendered, []bodyFile{{ID: 5001, SGID: fileSGID}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `<div><import-attachment ref="attachment-5001" caption="Tea &amp; biscuits" presentation="gallery"></import-attachment></div>`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(files) != 1 || files[0].ref != "attachment-5001" {
		t.Errorf("the text refers to %+v", files)
	}
}
