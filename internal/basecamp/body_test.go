package basecamp

import (
	"encoding/base64"
	"html"
	"strings"
	"testing"
	"time"
)

const (
	mentionOpen = `<bc-attachment content-type="application/vnd.basecamp.mention"`
	fileSGID    = "c2tldGNo--0a1b"
)

// rubyString is a short string as Ruby serialises it: a byte for its length.
func rubyString(text string) string {
	return "I\"" + string([]byte{byte(len(text) + 5)}) + text
}

// signed is a signed global ID as Basecamp writes them today: an envelope
// that Ruby serialised, as base64, and a signature.
func signed(globalID string) string {
	envelope := "\x04\x08{\x06I\"\x0b_rails\x06:\x06ET{\x07I\"\tdata\x06;\x00T" + rubyString(globalID) + "\x06;\x00TI\"\x08pur\x06;\x00TI\"\x0fattachable\x06;\x00T"
	return base64.StdEncoding.EncodeToString([]byte(envelope)) + "--0123456789abcdef0123456789abcdef01234567"
}

// enveloped is a signed global ID as older texts hold them: an envelope of
// JSON around the serialised ID, which is base64 in the given alphabet.
func enveloped(globalID string, alphabet *base64.Encoding) string {
	message := alphabet.EncodeToString([]byte("\x04\x08" + rubyString(globalID) + "\x06:\x06ET"))
	return base64.RawStdEncoding.EncodeToString([]byte(`{"_rails":{"message":"`+message+`","exp":null,"pur":"attachable"}}`)) + "--fedcba9876543210fedcba9876543210fedcba98"
}

func figure(personID, name string) string {
	return `<figure><img data-avatar-for-person-id="` + personID + `" src="https://avatars.example.test/a"><figcaption>` + name + `</figcaption></figure>`
}

// bodyExtractor knows Grace and Niklaus from the list of the account.
func bodyExtractor(t *testing.T) *Extractor {
	t.Helper()
	e := &Extractor{people: newRegistry()}
	err := e.people.listAccount([]person{
		{ID: 102, Name: "Grace Sample", EmailAddress: "grace@example.test"},
		{ID: 105, Name: "Niklaus <Nowhere>", EmailAddress: "niklaus@example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	timed(e.people)
	return e
}

func rewritten(t *testing.T, e *Extractor, text, rendered string, files ...bodyFile) string {
	t.Helper()
	out, _, err := e.rewriteBody(text, rendered, files)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestATextWithoutEmbedsStaysAsItIs(t *testing.T) {
	e := bodyExtractor(t)
	for _, text := range []string{
		"",
		"plain",
		`<DIV class='a'>Fish &amp; chips&nbsp;&lt;3 <BR/><a href="https://example.test/?a=1&b=2">it's</a></div><p>unclosed`,
		`<html><body>somebody wrote this</body></html>`,
	} {
		if got := rewritten(t, e, text, ""); got != text {
			t.Errorf("got %q, want %q", got, text)
		}
	}
}

func TestWhatStandsAroundAnEmbedPassesThroughByteForByte(t *testing.T) {
	e := bodyExtractor(t)
	before := `<DIV class='a'>Fish &amp; chips&nbsp;&lt;3 &#39;x&#39; <BR/><a href="https://example.test/?a=1&b=2">it's</a>`
	after := `<ul><li>one</LI></ul></div><p>unclosed`
	got := rewritten(t, e, before+`<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"></bc-attachment>`+after, "")
	if want := before + "<hr>" + after; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	for _, wrapper := range []string{"<html", "<head", "<body"} {
		if strings.Contains(got, wrapper) {
			t.Errorf("the text was wrapped in %s: %q", wrapper, got)
		}
	}
}

func TestAMentionBecomesAReferenceToSomebodyTheAccountKnows(t *testing.T) {
	opaque := signed("an opaque reference")
	for name, mention := range map[string]string{
		"the signed ID names the person": mentionOpen + ` sgid="` + signed("gid://bc3/Person/102") + `">` + figure("", "Older Name") + `</bc-attachment>`,
		"the signed ID in an envelope":   mentionOpen + ` sgid="` + enveloped("gid://bc3/Person/102?expires_in", base64.StdEncoding) + `">` + figure("", "Older Name") + `</bc-attachment>`,
		"an envelope safe for addresses": mentionOpen + ` sgid="` + enveloped("gid://bc3/Person/102?expires_in>>>", base64.RawURLEncoding) + `">` + figure("", "Older Name") + `</bc-attachment>`,
		"the avatar names the person":    mentionOpen + ` sgid="` + opaque + `">` + figure("102", "Older Name") + `</bc-attachment>`,
		"the editor named the person":    mentionOpen + ` sgid="` + opaque + `" content='&lt;bc-mention gid="gid://bc3/Person/102"&gt;&lt;/bc-mention&gt;'></bc-attachment>`,
		"written in capitals":            `<BC-ATTACHMENT CONTENT-TYPE="application/vnd.basecamp.mention" SGID="` + signed("gid://bc3/Person/102") + `"></BC-ATTACHMENT>`,
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, "<p>Hi "+mention+" there</p>", "")
		// The name is the one of the account's list, not the one in the text.
		if want := `<p>Hi <import-mention ref="102">Grace Sample</import-mention> there</p>`; got != want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
		if _, referred := e.people.referred[102]; !referred || e.bodies.MentionsUnresolved != 0 {
			t.Errorf("%s: the person mentioned did not come into the bundle, or the mention counts as unresolved", name)
		}
	}
}

func TestTheKindOfAnEmbedIsReadWithoutItsParametersAndInAnyCase(t *testing.T) {
	for _, contentType := range []string{
		"application/vnd.basecamp.mention; charset=utf-8", "Application/VND.Basecamp.Mention", " application/vnd.basecamp.mention ;x=y",
	} {
		e := bodyExtractor(t)
		mention := `<bc-attachment content-type="` + contentType + `" sgid="` + signed("gid://bc3/Person/102") + `">` + figure("102", "Older Name") + `</bc-attachment>`
		if got, want := rewritten(t, e, mention, ""), `<import-mention ref="102">Grace Sample</import-mention>`; got != want {
			t.Errorf("%q:\n got %q\nwant %q", contentType, got, want)
		}
		if e.bodies != (BodyCounts{}) {
			t.Errorf("%q: counted %+v", contentType, e.bodies)
		}
	}
	e := bodyExtractor(t)
	rule := `<bc-attachment content-type="Application/vnd.basecamp.horizontal-rule.HTML; charset=utf-8"><hr></bc-attachment>`
	if got := rewritten(t, e, rule, ""); got != "<hr>" {
		t.Errorf("got %q, want a rule", got)
	}
}

func TestOnlyAGlobalIDOfAPersonNamesOne(t *testing.T) {
	for globalID, want := range map[string]int64{
		"gid://bc3/Person/102":              102,
		"gid://bc3/Person/102?expires_in":   102,
		`"gid://bc3/Person/102?expires_in"`: 102,
		`{"gid":"gid://bc3/Person/102"}`:    102,
		// As Ruby serialises it: a byte for the length before, a field after.
		"\x04\x08I\")gid://bc3/Person/102?expires_in\x06:\x06ET": 102,
		"I\"\x1agid://bc3/Person/102\x06;\x00T":                  102,
		"gid://bc3/Person/102/avatar":                            0,
		"gid://bc3/Person/102x":                                  0,
		"bigid://bc3/Person/102":                                 0,
		"gid://bc3/Personality/102":                              0,
		"gid://bc3/Attachment/102":                               0,
		"gid://bc3/Person/":                                      0,
		"gid://bc3/Person/99999999999999999999":                  0,
	} {
		if got, found := idIn(globalID, personGID); found != (want != 0) || found && got != want {
			t.Errorf("%q names person %d (%v), want %d", globalID, got, found, want)
		}
	}

	// The signature stands behind the last "--": base64 that is safe for an
	// address may hold the same two letters.
	sgid := base64.URLEncoding.EncodeToString([]byte("\xfb\xef\xbe"+`"gid://bc3/Person/102"`)) + "--0a1b"
	if !strings.HasPrefix(sgid, "----") {
		t.Fatalf("the signed ID %q does not begin with what looks like a signature", sgid)
	}
	if got, found := gidIn(sgid, personGID); !found || got != 102 {
		t.Errorf("%q: person %d, found %v", sgid, got, found)
	}
}

func TestWhatBasecampSignedCountsBeforeWhatTheTextSays(t *testing.T) {
	e := bodyExtractor(t)
	mention := mentionOpen + ` sgid="` + signed("gid://bc3/Person/105") + `" content='&lt;bc-mention gid="gid://bc3/Person/102"&gt;'>` + figure("102", "Grace Sample") + `</bc-attachment>`
	if got, want := rewritten(t, e, mention, ""), `<import-mention ref="105">Niklaus &lt;Nowhere&gt;</import-mention>`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	e = bodyExtractor(t)
	mention = mentionOpen + ` sgid="opaque" content='&lt;bc-mention gid="gid://bc3/Person/102"&gt;'>` + figure("105", "Somebody") + `</bc-attachment>`
	if got := rewritten(t, e, mention, ""); !strings.Contains(got, `ref="105"`) {
		t.Errorf("what the editor wrote counted before what Basecamp rendered: %q", got)
	}
}

func TestAMentionThatNamesNobodyTheAccountKnowsStaysAName(t *testing.T) {
	for name, mention := range map[string]string{
		"nothing names a person":   mentionOpen + ` sgid="` + signed("an opaque reference") + `">` + figure("", "Gone &lt;Person&gt;") + `</bc-attachment>`,
		"somebody who has left":    mentionOpen + ` sgid="` + signed("gid://bc3/Person/777") + `">` + figure("777", "Gone &lt;Person&gt;") + `</bc-attachment>`,
		"no person but a to-do":    mentionOpen + ` sgid="` + signed("gid://bc3/Todo/102") + `">` + figure("", "Gone &lt;Person&gt;") + `</bc-attachment>`,
		"a number no ID can be":    mentionOpen + ` sgid="` + signed("gid://bc3/Person/99999999999999999999") + `">` + figure("99999999999999999999", "Gone &lt;Person&gt;") + `</bc-attachment>`,
		"a name without a caption": mentionOpen + ` sgid="x"><figure><img src="a"> Gone &lt;Person&gt; </figure></bc-attachment>`,
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, "<div>Ask "+mention+"!</div>", "")
		if want := "<div>Ask Gone &lt;Person&gt;!</div>"; got != want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
		if e.bodies.MentionsUnresolved != 1 || len(e.people.referred) != 0 {
			t.Errorf("%s: %d unresolved, %d people referred to", name, e.bodies.MentionsUnresolved, len(e.people.referred))
		}
	}
}

func TestAStoredTextTakesTheNameAndTheAvatarOfAMentionFromTheRenderedOne(t *testing.T) {
	gone, known := signed("an opaque reference"), signed("another opaque reference")
	stored := `<div>` + mentionOpen + ` sgid="` + gone + `"></bc-attachment> and ` + mentionOpen + ` sgid="` + known + `"></bc-attachment></div>`
	rendered := `<div>` + mentionOpen + ` sgid="` + gone + `">` + figure("777", "Gone Person") + `</bc-attachment> and ` +
		mentionOpen + ` sgid="` + known + `">` + figure("105", "Somebody") + `</bc-attachment></div>`

	e := bodyExtractor(t)
	got := rewritten(t, e, stored, rendered)
	if want := `<div>Gone Person and <import-mention ref="105">Niklaus &lt;Nowhere&gt;</import-mention></div>`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if e.bodies.MentionsUnresolved != 1 {
		t.Errorf("%d unresolved, want 1", e.bodies.MentionsUnresolved)
	}
}

func TestAFileBecomesAReferenceToItsAttachment(t *testing.T) {
	embed := `<bc-attachment sgid="` + fileSGID + `" content-type="image/png" url="https://storage.example.test/sketch.png" filename="sketch.png" caption="A &amp; B">` +
		"\n  <figure><img srcset=\"a 2x\" src=\"https://preview.example.test/sketch.png\">\n<figcaption>sketch.png &amp; <span>2 &lt;KB&gt;</span></figcaption></figure>\n</bc-attachment>"

	for name, file := range map[string]bodyFile{
		"by its signed ID":         {ID: 5001, SGID: fileSGID},
		"by the one it attaches":   {ID: 5001, SGID: "another", AttachableSGID: fileSGID},
		"among others of the text": {ID: 5001, SGID: fileSGID, AttachableSGID: fileSGID},
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, "<div>See "+embed+" above</div>", "", bodyFile{ID: 4000, SGID: "unrelated"}, file)
		if want := `<div>See <import-attachment ref="attachment-5001" caption="A &amp; B"></import-attachment> above</div>`; got != want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
		if e.bodies.Files != 1 || e.bodies.FilesDropped != 0 {
			t.Errorf("%s: counted %+v", name, e.bodies)
		}
	}

	// A file Basecamp lists nothing for cannot be named. What stays of it is
	// its caption, which somebody may have written.
	for name, files := range map[string][]bodyFile{
		"no file listed":         nil,
		"another file listed":    {{ID: 4000, SGID: "unrelated"}},
		"a file without its ID":  {{SGID: fileSGID}},
		"an empty signed ID too": {{ID: 4000}},
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, "<div>See "+embed+" above</div>", "", files...)
		if want := "<div>See sketch.png &amp; 2 &lt;KB&gt; above</div>"; got != want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
		if e.bodies.Files != 0 || e.bodies.FilesDropped != 1 {
			t.Errorf("%s: counted %+v", name, e.bodies)
		}
	}
}

// An older text carries the signed ID of its day, and the list the one of
// today. The key of the stored file and the ID of the attachment still say
// which listed file an embed is.
func TestAFileIsFoundByItsSignedIDThenItsKeyThenItsID(t *testing.T) {
	const href = "https://storage.example.test/4242/blobs/k-one/download/plan%20A.png?disposition=attachment"
	older := enveloped("gid://bc3/Attachment/5001?expires_in", base64.RawURLEncoding)
	today := signed("gid://bc3/Attachment/5001?expires_in")
	other := bodyFile{ID: 4000, SGID: signed("gid://bc3/Attachment/4000?expires_in"), Key: "k-other"}

	for name, c := range map[string]struct {
		embed string
		files []bodyFile
		want  int64
	}{
		"by the key in href":            {`sgid="` + older + `" href="` + href + `"`, []bodyFile{other, {ID: 7, SGID: today, Key: "k-one"}}, 7},
		"by the key in url":             {`sgid="` + older + `" url="` + href + `"`, []bodyFile{other, {ID: 7, SGID: today, Key: "k-one"}}, 7},
		"href counts before url":        {`sgid="x" url="https://s.example.test/blobs/k-other/download/a" href="` + href + `"`, []bodyFile{other, {ID: 7, Key: "k-one"}}, 7},
		"by the ID the older one names": {`sgid="` + older + `"`, []bodyFile{other, {ID: 5001, SGID: today, Key: "k-one"}}, 5001},
		"by the ID of today's form":     {`sgid="` + today + `"`, []bodyFile{other, {ID: 5001, SGID: "issued anew", Key: "k-one"}}, 5001},
		"the signed ID before the key":  {`sgid="` + older + `" href="` + href + `"`, []bodyFile{{ID: 7, Key: "k-one"}, {ID: 8, SGID: older}}, 8},
		"the key before the ID":         {`sgid="` + older + `" href="` + href + `"`, []bodyFile{{ID: 5001}, {ID: 7, Key: "k-one"}}, 7},
		// What an embed names counts only when a listed file confirms it.
		"an ID no listed file has":       {`sgid="` + today + `" href="` + href + `"`, []bodyFile{other}, 0},
		"a file without a key":           {`sgid="x" href="https://s.example.test/blobs//download/a"`, []bodyFile{{ID: 7}}, 0},
		"an address that names no key":   {`sgid="x" href="https://s.example.test/k-one/download/a"`, []bodyFile{{ID: 7, Key: "k-one"}}, 0},
		"the ID of something else":       {`sgid="` + signed("gid://bc3/Person/5001?expires_in") + `"`, []bodyFile{{ID: 5001}}, 0},
		"a listed file without its ID":   {`sgid="` + older + `" href="` + href + `"`, []bodyFile{{Key: "k-one", SGID: older}}, 0},
		"an address that is none at all": {`sgid="x" href="://%zz"`, []bodyFile{{ID: 7, Key: ""}}, 0},
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, `<div><bc-attachment content-type="image/png" `+c.embed+`><figure><figcaption>cap</figcaption></figure></bc-attachment></div>`, "", c.files...)
		want, counts := "<div>cap</div>", BodyCounts{FilesDropped: 1}
		if c.want != 0 {
			want, counts = `<div><import-attachment ref="attachment-`+sourceID(c.want)+`"></import-attachment></div>`, BodyCounts{Files: 1}
		}
		if got != want || e.bodies != counts {
			t.Errorf("%s:\n got %q %+v\nwant %q %+v", name, got, e.bodies, want, counts)
		}
	}
}

func TestAStoredTextTakesTheCaptionOfAFileThatIsDroppedFromTheRenderedOne(t *testing.T) {
	e := bodyExtractor(t)
	stored := `<div>See <bc-attachment sgid="` + fileSGID + `" content-type="image/png"></bc-attachment></div>`
	rendered := `<div>See <bc-attachment sgid="` + fileSGID + `" content-type="image/png"><figure><img src="a"><figcaption>A &amp; B</figcaption></figure></bc-attachment></div>`
	if got, want := rewritten(t, e, stored, rendered), "<div>See A &amp; B</div>"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestARuleBecomesARule(t *testing.T) {
	e := bodyExtractor(t)
	for _, embed := range []string{
		`<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"></bc-attachment>`,
		`<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"><hr></bc-attachment>`,
		`<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"> <figure><hr/></figure> </bc-attachment>`,
	} {
		if got, want := rewritten(t, e, "<div>a</div>"+embed+"<div>b</div>", ""), "<div>a</div><hr><div>b</div>"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestAnEmbedOfAnotherKindPassesThroughWithItsEnd(t *testing.T) {
	e := bodyExtractor(t)
	text := `<div><bc-attachment content-type="application/vnd.example.embed" href="https://example.test/"><figure>A caption</figure></bc-attachment> and <bc-attachment>bare</bc-attachment></div>`
	if got := rewritten(t, e, text, ""); got != text {
		t.Errorf("got %q, want %q", got, text)
	}
	if e.bodies != (BodyCounts{}) {
		t.Errorf("counted %+v", e.bodies)
	}
}

// The tokens are taken one by one, so an embed in a paragraph keeps its
// figure: a parser would move the figure out of the paragraph.
func TestAnEmbedKeepsItsFigureInsideAParagraph(t *testing.T) {
	e := bodyExtractor(t)
	text := `<p>See ` + mentionOpen + ` sgid="` + signed("gid://bc3/Person/102") + `">` + figure("102", "Grace Sample") + `</bc-attachment> and <bc-attachment sgid="` + fileSGID + `" content-type="image/png"><figure><img src="a"><figcaption>sketch.png</figcaption></figure></bc-attachment>.</p>`
	got := rewritten(t, e, text, "", bodyFile{ID: 5001, SGID: fileSGID})
	if want := `<p>See <import-mention ref="102">Grace Sample</import-mention> and <import-attachment ref="attachment-5001"></import-attachment>.</p>`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestABrokenEmbedCostsNoMoreThanItself(t *testing.T) {
	file := `<bc-attachment sgid="` + fileSGID + `" content-type="image/png">`
	for text, want := range map[string]string{
		// Never closed.
		"a " + file + " tail <b>bold</b>": "a  tail <b>bold</b>",
		// Closed twice.
		"a " + file + "</bc-attachment></bc-attachment> tail": "a  tail",
		// One in another.
		"a " + file + file + "</bc-attachment></bc-attachment> tail": "a  tail",
		// An end without a beginning.
		"a </bc-attachment> tail": "a  tail",
		// A figure that is never closed ends with its embed.
		"a " + file + "<figure><img src=x><figcaption>cap</bc-attachment> tail <b>bold</b>": "a cap tail <b>bold</b>",
		// A figure that is never closed ends where the next embed begins.
		"a " + file + "<figure>cap" + file + "</bc-attachment> tail <b>bold</b>": "a cap tail <b>bold</b>",
		// One that closes itself holds nothing: what follows is the text's own.
		`a <bc-attachment sgid="` + fileSGID + `" content-type="image/png"/> <hr> <figure>kept</figure></bc-attachment> tail`: "a  <hr> <figure>kept</figure> tail",
		`a ` + mentionOpen + `/> <figure><figcaption>kept</figcaption></figure> tail`:                                         "a  <figure><figcaption>kept</figcaption></figure> tail",
		// Cut off in the middle of the tag.
		`a <bc-attachment sgid="` + fileSGID: "a ",
		// What looks like an embed inside a text area is text.
		"<textarea>" + file + "</textarea>": "<textarea>" + file + "</textarea>",
		// An embed of another kind that closes itself has no end to let through.
		`a <bc-attachment content-type="other"/> b</bc-attachment> tail`: `a <bc-attachment content-type="other"/> b tail`,
		// A mention without anything.
		"a " + mentionOpen + "> tail": "a  tail",
		// A signed ID that is no base64, an envelope that holds none.
		"a " + mentionOpen + ` sgid="%%%--x"> tail`: "a  tail",
		"a " + mentionOpen + ` sgid="` + base64.StdEncoding.EncodeToString([]byte(`{"_rails":{"message":"%%%"}}`)) + `--x"> tail`: "a  tail",
		"a " + mentionOpen + ` sgid="` + base64.StdEncoding.EncodeToString([]byte(`{"_rails":17}`)) + `--x"> tail`:                "a  tail",
	} {
		e := bodyExtractor(t)
		got, _, err := e.rewriteBody(text, "", nil)
		if err != nil || got != want {
			t.Errorf("%q:\n got %q (%v)\nwant %q", text, got, err, want)
		}
	}
}

const previewOpen = `<bc-attachment content-type="application/vnd.basecamp.opengraph-embed"`

func TestThePreviewOfALinkBecomesTheLink(t *testing.T) {
	for name, c := range map[string]struct{ embed, want string }{
		"with a title":               {` url="https://example.test/a" href="https://example.test/a" title="A page" caption="example.test"></bc-attachment>`, `<a href="https://example.test/a">A page</a>`},
		"the address as the text":    {` href="https://example.test/a?b=1&amp;c=2"></bc-attachment>`, `<a href="https://example.test/a?b=1&amp;c=2">https://example.test/a?b=1&amp;c=2</a>`},
		"a title that is escaped":    {` href="http://example.test/" title="&lt;b&gt; &amp; &quot;more&quot;"></bc-attachment>`, `<a href="http://example.test/">&lt;b&gt; &amp; &#34;more&#34;</a>`},
		"an address with a quote":    {` href='https://example.test/"onmouseover="x' title="A page"></bc-attachment>`, `<a href="https://example.test/&#34;onmouseover=&#34;x">A page</a>`},
		"a title over several rows":  {` href="https://example.test/" title=" A&#10;  page "></bc-attachment>`, `<a href="https://example.test/">A page</a>`},
		"what Basecamp rendered":     {` href="https://example.test/" title="A page"><figure><img src="p"><figcaption>Shown</figcaption></figure></bc-attachment>`, `<a href="https://example.test/">A page</a>`},
		"one that closes itself":     {` href="https://example.test/" title="A page"/>`, `<a href="https://example.test/">A page</a>`},
		"what url says without href": {` url="https://example.test/b" title="A page"></bc-attachment>`, `<a href="https://example.test/b">A page</a>`},
		"href before url":            {` url="https://example.test/b" href="https://example.test/a" title="A page"></bc-attachment>`, `<a href="https://example.test/a">A page</a>`},
		"with a signed ID":           {` sgid="` + fileSGID + `" href="https://example.test/" title="A page"></bc-attachment>`, `<a href="https://example.test/">A page</a>`},
		"the kind in another case":   {``, ``},
		// Only an address of the web becomes a link: the title stays as text.
		"a script":                   {` href="javascript:alert(1)" title="Click &lt;me&gt;"></bc-attachment>`, `Click &lt;me&gt;`},
		"a script in capitals":       {` href=" JavaScript:alert(1)" title="Click"></bc-attachment>`, `Click`},
		"data":                       {` href="data:text/html,hello" title="Click"></bc-attachment>`, `Click`},
		"a path":                     {` href="/4242/projects" title="Click"></bc-attachment>`, `Click`},
		"an address without host":    {` href="https:///nowhere" title="Click"></bc-attachment>`, `Click`},
		"another protocol":           {` href="ftp://example.test/" title="Click"></bc-attachment>`, `Click`},
		"no address":                 {` title="Click"></bc-attachment>`, `Click`},
		"a script beside an address": {` url="https://example.test/" href="javascript:alert(1)" title="Click"></bc-attachment>`, `Click`},
		"nothing at all":             {`></bc-attachment>`, ``},
		"a script without title":     {` href="javascript:alert(1)"></bc-attachment>`, ``},
	} {
		e := bodyExtractor(t)
		embed := previewOpen + c.embed
		if c.embed == "" {
			embed = `<bc-attachment content-type="Application/VND.Basecamp.OpenGraph-Embed; charset=utf-8"></bc-attachment>`
		}
		got := rewritten(t, e, `<div>See `+embed+`!</div>`, "")
		if want := `<div>See ` + c.want + `!</div>`; got != want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
		// Only what became a link is counted as one.
		want := BodyCounts{}
		if strings.HasPrefix(c.want, "<a ") {
			want.LinkPreviews = 1
		}
		if e.bodies != want {
			t.Errorf("%s: counted %+v, want %+v", name, e.bodies, want)
		}
	}
}

const (
	frameAddress = "https://video.example.test/embed/42"
	frame        = `<iframe src="` + frameAddress + `"></iframe>`
	embed        = `<import-embed url="` + frameAddress + `"></import-embed>`
)

func TestAnEmbeddedFrameBecomesAnEmbedInItsPlace(t *testing.T) {
	for name, c := range map[string]struct {
		text, want string
		embeds     int
	}{
		"in its figure":               {`<div>Watch</div><figure>` + frame + `</figure><div>this</div>`, `<div>Watch</div>` + embed + `<div>this</div>`, 1},
		"whatever the figure carries": {`<figure class="attachment attachment--preview" data-trix-attachment='{"a":1}'>` + frame + `</figure>`, embed, 1},
		"outside a figure":            {`<div>Watch</div>` + frame + `<div>this</div>`, `<div>Watch</div>` + embed + `<div>this</div>`, 1},
		"in a paragraph":              {`<p>Watch ` + frame + ` now</p>`, `<p>Watch ` + embed + ` now</p>`, 1},
		"a figure in a block":         {`<div><figure>` + frame + `</figure></div>`, `<div>` + embed + `</div>`, 1},
		"a figure in a paragraph":     {`<p><figure>` + frame + `</figure></p>`, `<p>` + embed + `</p>`, 1},
		// Whatever the frame carries and holds goes with it.
		"attributes in any order": {`<figure><iframe width="560" height="315" src="` + frameAddress + `" title="A film" allowfullscreen></iframe></figure>`, embed, 1},
		"single quotes":           {`<iframe allow='autoplay' src='` + frameAddress + `'></iframe>`, embed, 1},
		"no quotes":               {`<iframe src=` + frameAddress + `></iframe>`, embed, 1},
		"in capitals":             {`<FIGURE><IFRAME SRC="` + frameAddress + `"></IFRAME></FIGURE>`, embed, 1},
		"what the frame holds":    {`<figure><iframe src="` + frameAddress + `">Your browser shows <b>no</b> frames. <p>Sorry</p></iframe></figure>`, embed, 1},
		// A caption becomes a paragraph, with all it holds as it was written.
		"with a caption":           {`<figure>` + frame + `<figcaption>A &amp; B &lt;3 <em>film</em> &#39;x&#39;<br></figcaption></figure>`, embed + `<p>A &amp; B &lt;3 <em>film</em> &#39;x&#39;<br></p>`, 1},
		"what the caption carries": {`<figure>` + frame + `<figcaption class="attachment__caption" data-x='y'> A film </figcaption></figure>`, embed + `<p> A film </p>`, 1},
		"an empty caption":         {`<figure>` + frame + `<figcaption></figcaption></figure>`, embed, 1},
		"a caption of whitespace":  {`<figure>` + frame + "<figcaption> \n\t&nbsp;\u200b</figcaption></figure>", embed, 1},
		"a caption in capitals":    {`<figure>` + frame + `<FIGCAPTION>A film</FIGCAPTION></figure>`, embed + `<p>A film</p>`, 1},
		"a caption before":         {`<figure><figcaption>A film</figcaption>` + frame + `</figure>`, `<p>A film</p>` + embed, 1},
		"a mention in the caption": {`<figure>` + frame + `<figcaption>by ` + mentionOpen + ` sgid="` + signed("gid://bc3/Person/102") + `">` + figure("102", "Grace Sample") + `</bc-attachment></figcaption></figure>`, embed + `<p>by <import-mention ref="102">Grace Sample</import-mention></p>`, 1},
		// What else stands in the figure stays where it stood.
		"a link beside the frame":  {`<figure>` + frame + `<a href="https://example.test/film?a=1&b=2" class='x'>The film</a></figure>`, embed + `<a href="https://example.test/film?a=1&b=2" class='x'>The film</a>`, 1},
		"whitespace in the figure": {"<figure>\n  " + frame + "\n  <figcaption>A film</figcaption>\n</figure>", "\n  " + embed + "\n  <p>A film</p>\n", 1},
		"a figure without a frame within": {`<figure>` + frame + `<figure class="in"><img src="x"><figcaption>inner</figcaption></figure><figcaption>outer</figcaption></figure>`,
			embed + `<figure class="in"><img src="x"><figcaption>inner</figcaption></figure><p>outer</p>`, 1},
		"a figure with a frame within": {`<figure><figure>` + frame + `</figure><figcaption>outer</figcaption></figure>`, embed + `<p>outer</p>`, 1},
		// Every frame of a text becomes an embed, in the order of the text.
		"two in one figure": {`<figure>` + frame + `<iframe src="https://sound.example.test/track/7"></iframe><figcaption>Both</figcaption></figure>`,
			embed + `<import-embed url="https://sound.example.test/track/7"></import-embed><p>Both</p>`, 2},
		"between the embeds of Basecamp": {`<figure>` + frame + `<figcaption>A film</figcaption></figure>` + previewOpen + ` href="https://example.test/" title="A page"></bc-attachment><figure><iframe src="https://video.example.test/embed/43"></iframe></figure>`,
			embed + `<p>A film</p><a href="https://example.test/">A page</a><import-embed url="https://video.example.test/embed/43"></import-embed>`, 2},
	} {
		e := bodyExtractor(t)
		want := BodyCounts{Embeds: c.embeds}
		if name == "between the embeds of Basecamp" {
			want.LinkPreviews = 1
		}
		if got := rewritten(t, e, c.text, ""); got != c.want || e.bodies != want {
			t.Errorf("%s:\n got %q %+v\nwant %q %+v", name, got, e.bodies, c.want, want)
		}
	}
}

// A table comes in a figure as well: only a frame makes a figure go.
func TestAFigureThatHoldsNoFrameStaysAsItIs(t *testing.T) {
	for _, untouched := range []string{
		`<figure class="table"><table><tr><td>Boxes &amp; tape</td></tr></table><figcaption> A table </figcaption></figure>`,
		`<FIGURE><img src="https://pictures.example.test/a.png"><figcaption></figcaption></FIGURE>`,
		`</figcaption></figure></iframe>`,
		// What looks like a frame and is text, or nothing anybody sees.
		`<figure><textarea>` + frame + `</textarea></figure>`,
		`<figure><!-- ` + frame + ` --><figcaption>A film</figcaption></figure>`,
	} {
		for _, text := range []string{untouched, untouched + frame, frame + untouched} {
			e := bodyExtractor(t)
			want := strings.ReplaceAll(text, frame, embed)
			if strings.HasPrefix(untouched, "<figure><textarea>") || strings.HasPrefix(untouched, "<figure><!--") {
				want = strings.Replace(text, untouched, "\x00", 1)
				want = strings.Replace(strings.ReplaceAll(want, frame, embed), "\x00", untouched, 1)
			}
			if got := rewritten(t, e, text, ""); got != want {
				t.Errorf("\n got %q\nwant %q", got, want)
			}
		}
	}
}

// The address is the one the frame stands for, read once out of the text and
// written once into it.
func TestTheAddressOfAnEmbedIsReadOnceAndEscapedOnce(t *testing.T) {
	for src, url := range map[string]string{
		`https://video.example.test/embed?a=1&amp;b=2`:     `https://video.example.test/embed?a=1&amp;b=2`,
		`https://video.example.test/embed?a=1&b=2`:         `https://video.example.test/embed?a=1&amp;b=2`,
		`https://video.example.test/embed?a=&amp;amp;`:     `https://video.example.test/embed?a=&amp;amp;`,
		`https://video.example.test/embed?a=&amp;quot;`:    `https://video.example.test/embed?a=&amp;quot;`,
		`https://video.example.test/&quot;onload=&quot;x`:  `https://video.example.test/&#34;onload=&#34;x`,
		`https://video.example.test/&lt;script&gt;&#x27;x`: `https://video.example.test/&lt;script&gt;&#39;x`,
		`https://video.example.test/&#252;ber`:             `https://video.example.test/über`,
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, `<figure><iframe src="`+src+`"></iframe></figure>`, "")
		if want := `<import-embed url="` + url + `"></import-embed>`; got != want {
			t.Errorf("%s:\n got %q\nwant %q", src, got, want)
		}
	}
	e := bodyExtractor(t)
	if got, want := rewritten(t, e, `<iframe src='https://video.example.test/"x'></iframe>`, ""), `<import-embed url="https://video.example.test/&#34;x"></import-embed>`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOnlyAnAddressMinkaTakesBecomesAnEmbed(t *testing.T) {
	const base = "https://video.example.test/"
	for name, c := range map[string]struct {
		address string
		taken   bool
	}{
		"https":                                {"https://video.example.test/embed/42", true},
		"http":                                 {"http://video.example.test/embed/42", true},
		"a scheme in capitals":                 {"HTTPS://video.example.test/embed/42", true},
		"a scheme in any case":                 {"hTtP://video.example.test/embed/42", true},
		"a host alone":                         {"https://video.example.test", true},
		"a host and a query":                   {"https://video.example.test?v=/@:x", true},
		"a host and a fragment":                {"https://video.example.test#t=/@:x", true},
		"somebody and a password":              {"https://ada:secret@video.example.test/embed", true},
		"an at sign in what names them":        {"https://ada@home@video.example.test/embed", true},
		"a password with an at sign before it": {"https://ada@home:secret@video.example.test/embed", true},
		"a colon and no port":                  {"https://video.example.test:/embed", true},
		"such a colon behind brackets":         {"https://[2001:db8::1]:/", true},
		"nobody before the at sign":            {"https://@video.example.test/embed", true},
		"a port":                               {"https://video.example.test:8443/embed", true},
		"the largest port":                     {"https://video.example.test:65535/embed", true},
		"a port with zeros before it":          {"https://video.example.test:00443/embed", true},
		"a host of another script":             {"https://bücher.example.test/embed", true},
		"a host of numbers and colons":         {"https://[2001:db8::1]/embed", true},
		"such a host with a port":              {"https://[2001:db8::1]:8443/embed", true},
		"exactly as long as may be":            {base + strings.Repeat("a", 2000-len(base)), true},
		"as long in characters":                {base + strings.Repeat("ü", 2000-len(base)), true},
		"a control character beyond":           {base + "a\u0085b\u009fc", true},
		"a space that does not break":          {base + "a\u00a0b", true},
		"a separator of lines":                 {base + "a\u2028b", true},
		"a character without a width":          {base + "a\u200bb\ufeff", true},
		"what HTML escapes":                    {base + `?a=1&b="2"&c=<d>&e='f'`, true},

		"a path":                                   {"/embed/42", false},
		"a relative path":                          {"embed/42", false},
		"a host without a scheme":                  {"//video.example.test/embed/42", false},
		"data":                                     {"data:text/html,<p>hello</p>", false},
		"about":                                    {"about:blank", false},
		"a script":                                 {"javascript:alert(1)", false},
		"another protocol":                         {"ftp://video.example.test/embed", false},
		"a longer scheme":                          {"httpsx://video.example.test/embed", false},
		"a scheme with one slash":                  {"https:/video.example.test/embed", false},
		"a scheme of another letter":               {"http\u017f://video.example.test/embed", false},
		"a scheme alone":                           {"https://", false},
		"a scheme and a path":                      {"https:///embed/42", false},
		"a scheme and a query":                     {"https://?v=video.example.test", false},
		"a scheme and a fragment":                  {"https://#video.example.test", false},
		"somebody and no host":                     {"https://ada@/embed", false},
		"a port and no host":                       {"https://:8443/embed", false},
		"a port that is no number":                 {"https://video.example.test:84a/embed", false},
		"a port with a sign":                       {"https://video.example.test:+8443/embed", false},
		"a port too large":                         {"https://video.example.test:65536/embed", false},
		"a port too long":                          {"https://video.example.test:008443/embed", false},
		"a colon and neither host nor port":        {"https://:/embed", false},
		"a port too large behind brackets":         {"https://[2001:db8::1]:65536/", false},
		"a port that is no number behind brackets": {"https://[2001:db8::1]:84a/", false},
		"something else behind brackets":           {"https://[2001:db8::1]x/embed", false},
		"a blank":                                  {base + "a b", false},
		"a blank in the scheme":                    {"java script://video.example.test/", false},
		"a tab":                                    {base + "a\tb", false},
		"a line break":                             {base + "a\nb", false},
		"a carriage return":                        {base + "a\rb", false},
		"a control character":                      {base + "a\u001fb", false},
		"a zero":                                   {base + "a\u0000b", false},
		"the character that deletes":               {base + "a\u007fb", false},
		"a backslash":                              {base + `a\b`, false},
		"a backslash before the host":              {`https://evil.example.test\@video.example.test/embed`, false},
		"a backslash for a slash":                  {`https:/\video.example.test/embed`, false},
		"one character too long":                   {base + strings.Repeat("a", 2001-len(base)), false},
		"too long in characters":                   {base + strings.Repeat("ü", 2001-len(base)), false},
		"no address":                               {"", false},
	} {
		if got := embeddable(c.address); got != c.taken {
			t.Errorf("%s: taken %v, want %v", name, got, c.taken)
		}

		// A carriage return is a line break by the time a text is read.
		if strings.Contains(c.address, "\r") || strings.Contains(c.address, "\x00") {
			continue
		}
		e := bodyExtractor(t)
		want, counts := `<import-embed url="`+html.EscapeString(c.address)+`"></import-embed>`, BodyCounts{Embeds: 1}
		if !c.taken {
			want, counts = "", BodyCounts{EmbedsLeftOut: 1}
		}
		got := rewritten(t, e, `<figure><iframe src="`+html.EscapeString(c.address)+`"></iframe></figure>`, "")
		if got != want || e.bodies != counts {
			t.Errorf("%s: a frame with it became %d characters and counted %+v, want %d and %+v", name, len(got), e.bodies, len(want), counts)
		}
	}
}

// A browser takes the whitespace around the address of a frame away, and
// Minka takes none away: the embed carries the address without it.
func TestTheWhitespaceAroundTheAddressOfAFrameGoes(t *testing.T) {
	for name, c := range map[string]struct {
		src   string
		taken bool
	}{
		"a blank before":         {" " + frameAddress, true},
		"a blank behind":         {frameAddress + " ", true},
		"every kind on each end": {"\n\t \f" + frameAddress + "\r\n\t ", true},
		"written as references":  {"&#32;&#9;&#10;" + frameAddress + "&#12;&#13;", true},
		"whitespace alone":       {" \t\n\f ", false},
		"a blank within":         {" https://video.example.test/embed /42 ", false},
		// Only what a browser takes away goes: any other character refuses or stays.
		"a vertical tab before":              {"\v" + frameAddress, false},
		"a space that does not break":        {"\u00a0" + frameAddress, false},
		"a character without a width":        {"\u200b" + frameAddress, false},
		"a space that does not break behind": {frameAddress + "\u00a0", true},
	} {
		if embeddable(" "+frameAddress) || embeddable(frameAddress+"\n") {
			t.Fatal("the rule itself takes an address with whitespace around it")
		}
		e := bodyExtractor(t)
		got := rewritten(t, e, `<figure><iframe src="`+c.src+`"></iframe></figure>`, "")
		want, counts := "", BodyCounts{EmbedsLeftOut: 1}
		if c.taken {
			want, counts = embed, BodyCounts{Embeds: 1}
			if strings.HasSuffix(c.src, "\u00a0") {
				want = `<import-embed url="` + frameAddress + "\u00a0" + `"></import-embed>`
			}
		}
		if got != want || e.bodies != counts {
			t.Errorf("%s:\n got %q %+v\nwant %q %+v", name, got, e.bodies, want, counts)
		}
	}
}

// A frame that becomes nothing takes the figure that held nothing else with
// it, and leaves what else stood there.
func TestAFrameWithoutAnAddressAnEmbedMayCarryIsLeftOut(t *testing.T) {
	const refused = `<iframe src="about:blank" width="1">nothing</iframe>`
	for name, c := range map[string]struct{ text, want string }{
		"no address at all":           {`<div>a</div><figure><iframe allowfullscreen></iframe></figure><div>b</div>`, `<div>a</div><div>b</div>`},
		"an empty address":            {`<div>a</div><figure><iframe src=""></iframe></figure><div>b</div>`, `<div>a</div><div>b</div>`},
		"in its figure":               {`<div>a</div><figure class="x">` + refused + `</figure><div>b</div>`, `<div>a</div><div>b</div>`},
		"outside a figure":            {`<p>a ` + refused + ` b</p>`, `<p>a  b</p>`},
		"an empty caption goes along": {`<figure>` + refused + `<figcaption> </figcaption></figure>`, ``},
		"a caption stays":             {`<figure>` + refused + `<figcaption>A <b>film</b></figcaption></figure>`, `<p>A <b>film</b></p>`},
		"a link stays":                {`<figure>` + refused + `<a href="https://example.test/">The film</a></figure>`, `<a href="https://example.test/">The film</a>`},
		"beside one that is taken":    {`<figure>` + refused + frame + `</figure>`, embed},
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, c.text, "")
		want := BodyCounts{EmbedsLeftOut: 1, Embeds: strings.Count(c.want, "<import-embed")}
		if got != c.want || e.bodies != want {
			t.Errorf("%s:\n got %q %+v\nwant %q %+v", name, got, e.bodies, c.want, want)
		}
	}
}

func TestABrokenFrameCostsNoMoreThanItself(t *testing.T) {
	rule := `<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"></bc-attachment>`
	for text, want := range map[string]string{
		// Never closed: what follows is the text's own, embeds included.
		`a <iframe src="` + frameAddress + `"> tail <b>bold</b>` + rule + `<figure><figcaption>kept</figcaption></figure>`: `a ` + embed + ` tail <b>bold</b><hr><figure><figcaption>kept</figcaption></figure>`,
		// A frame holds what stands up to the next end of one.
		`a <iframe src="` + frameAddress + `"> gone <figure>` + frame + `</figure> tail`: `a ` + embed + `</figure> tail`,
		`a <iframe src="` + frameAddress + `">`:                                          `a ` + embed,
		`a <figure><iframe src="about:blank"> tail <b>bold</b>`:                          `a  tail <b>bold</b>`,
		// One that closes itself holds what stands up to the next end of a frame, as one that does not.
		`a <iframe src="` + frameAddress + `"/> tail <b>bold</b>`:   `a ` + embed + ` tail <b>bold</b>`,
		`a <iframe src="` + frameAddress + `"/> gone</iframe> tail`: `a ` + embed + ` tail`,
		// Cut off in the middle of the tag.
		`a <iframe src="` + frameAddress: `a `,
		// A figure and a caption that are never closed.
		`a <figure>` + frame + ` tail <b>bold</b>`:                                                 `a ` + embed + ` tail <b>bold</b>`,
		`a <figure>` + frame + `<figcaption>A film</figure> tail`:                                  `a ` + embed + `<p>A film</p> tail`,
		`a <figure>` + frame + `<figcaption>A <b>film`:                                             `a ` + embed + `<p>A <b>film</p>`,
		`a <figure>` + frame + `<figcaption> `:                                                     `a ` + embed,
		`a <figure>` + frame + `<figcaption> </figure> tail`:                                       `a ` + embed + ` tail`,
		`a <figure>` + frame + `<figcaption>one<figcaption>two</figcaption></figcaption></figure>`: `a ` + embed + `<p>one<figcaption>two</p></figcaption>`,
		// What stands behind a frame that is never closed is read as any text: a figure there goes with its frame.
		`a <iframe src="` + frameAddress + `"> b <figure><iframe src="https://sound.example.test/track/7"> c<figcaption>cap</figcaption></figure>`: `a ` + embed + ` b <import-embed url="https://sound.example.test/track/7"></import-embed> c<p>cap</p>`,
		// Ends without a beginning are the text's own.
		`a </iframe></figure> tail ` + frame: `a </iframe></figure> tail ` + embed,
	} {
		e := bodyExtractor(t)
		got, _, err := e.rewriteBody(text, "", nil)
		if err != nil || got != want {
			t.Errorf("%q:\n got %q (%v)\nwant %q", text, got, err, want)
		}
	}
}

// What Basecamp rendered into an embed of its own goes with that embed. An
// embed of another kind passes through, and no frame with it: a frame reaches
// no bundle, wherever it stands.
func TestAFrameInAnEmbedOfBasecampGoesWithItOrBecomesAnEmbed(t *testing.T) {
	file := `<bc-attachment sgid="` + fileSGID + `" content-type="video/mp4"><figure>` + frame + `<figcaption>film.mp4</figcaption></figure></bc-attachment>`
	const other = `<bc-attachment content-type="application/vnd.example.embed">`
	for name, c := range map[string]struct {
		text, want string
		counts     BodyCounts
	}{
		"in an embed that is replaced": {`<div>` + file + `</div><figure>` + frame + `</figure>`,
			`<div><import-attachment ref="attachment-5001"></import-attachment></div>` + embed, BodyCounts{Files: 1, Embeds: 1}},
		"in an embed that passes": {other + `<figure class="x">` + frame + `<figcaption>A film</figcaption></figure></bc-attachment> tail`,
			other + embed + `<p>A film</p></bc-attachment> tail`, BodyCounts{Embeds: 1}},
		"one that is left out there": {other + `<figure><iframe src="about:blank"></iframe></figure></bc-attachment> tail`,
			other + `</bc-attachment> tail`, BodyCounts{EmbedsLeftOut: 1}},
		"behind an embed that never ends": {other + `<figure>` + frame + `</figure> and <p>` + frame + `</p><iframe src="//video.example.test/"></iframe>`,
			other + embed + ` and <p>` + embed + `</p>`, BodyCounts{Embeds: 2, EmbedsLeftOut: 1}},
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, c.text, "", bodyFile{ID: 5001, SGID: fileSGID})
		if got != c.want || e.bodies != c.counts {
			t.Errorf("%s:\n got %q %+v\nwant %q %+v", name, got, e.bodies, c.want, c.counts)
		}
		if strings.Contains(strings.ToLower(got), "<iframe") {
			t.Errorf("%s: a frame reached the text: %q", name, got)
		}
	}
}

// A text is read in a time that grows with its length, whatever it is made of:
// a text somebody pasted may hold thousands of frames nothing closes, or of
// figures in one another.
func TestATextOfManyBrokenFramesIsReadInATimeThatGrowsWithItsLength(t *testing.T) {
	// Reading such a text over for every frame in it takes many times the
	// bound, and reading it once a small part of it.
	const bound = 3 * time.Second
	for name, c := range map[string]struct {
		count                   int
		piece, becomes, closing string
	}{
		"frames that are never closed": {10000, `<iframe src="` + frameAddress + `"> tail `, embed + ` tail `, ""},
		"figures in one another":       {40000, `<figure><iframe src="` + frameAddress + `"></iframe>`, embed, ""},
		"with captions nothing closes": {10000, `<figure>` + frame + `<figcaption>c`, embed + `<p>c`, "</p>"},
	} {
		e := bodyExtractor(t)
		started := time.Now()
		got := rewritten(t, e, strings.Repeat(c.piece, c.count), "")
		if took := time.Since(started); took > bound {
			t.Errorf("%s: %d of them took %s", name, c.count, took)
		}
		if want := strings.Repeat(c.becomes, c.count) + strings.Repeat(c.closing, c.count); got != want || e.bodies.Embeds != c.count {
			t.Errorf("%s: the text became %d characters with %d embeds, want %d with %d", name, len(got), e.bodies.Embeds, len(want), c.count)
		}
	}
}

// A text that was a frame and nothing else is one: it says something where
// the frame became an embed, and nothing where it became nothing.
func TestATextThatIsAnEmbedAloneSaysSomething(t *testing.T) {
	for text, nothing := range map[string]bool{
		`<figure>` + frame + `</figure>`: false,
		frame:                            false,
		"<div>\n<figure>" + frame + "<figcaption></figcaption></figure></div>":                                  false,
		`<figure><iframe src="about:blank"></iframe></figure>`:                                                  true,
		"<div><figure>\n<iframe></iframe>\n<figcaption> </figcaption></figure><br></div>":                       true,
		`<figure><iframe src="//video.example.test/embed/42"></iframe><figcaption>A film</figcaption></figure>`: false,
	} {
		e := bodyExtractor(t)
		if got := saysNothing(rewritten(t, e, text, "")); got != nothing {
			t.Errorf("%q says nothing: %v, want %v", text, got, nothing)
		}
	}
}

func TestTheKeyOfAStoredFileStandsWhereItIsDownloadedOrPreviewed(t *testing.T) {
	for address, want := range map[string]string{
		"https://3.basecampapi.test/4242/blobs/k-one/download/a%20file.png": "k-one",
		"https://preview.basecamp.test/4242/blobs/k-one/previews/full":      "k-one",
		"https://preview.basecamp.test/4242/blobs/k-one/previews/full?x=1":  "k-one",
		"https://preview.basecamp.test/4242/blobs/k-one/thumbnails/full":    "",
		"https://preview.basecamp.test/4242/blobs/k-one":                    "",
		"https://preview.basecamp.test/4242/k-one/previews/full":            "",
		"https://preview.basecamp.test/4242/blobs//previews/full":           "",
		"://%zz": "",
		"":       "",
	} {
		if got := blobKey(address); got != want {
			t.Errorf("%s: the key is %q, want %q", address, got, want)
		}
	}
}

// A picture by an address of somebody else is no stored file: nothing is
// fetched, and it becomes a link to where it lies.
func TestAPictureAtAForeignAddressBecomesALinkToIt(t *testing.T) {
	const open = `<bc-attachment content-type="image"`
	for name, c := range map[string]struct {
		embed, want string
		pictures    int
	}{
		"with a caption":          {open + ` url="https://pictures.example.test/a.png" width="640" height="480" caption="A &amp; B"></bc-attachment>`, `<a href="https://pictures.example.test/a.png">A &amp; B</a>`, 1},
		"the address as the text": {open + ` url="https://pictures.example.test/a.png?w=1&amp;h=2"></bc-attachment>`, `<a href="https://pictures.example.test/a.png?w=1&amp;h=2">https://pictures.example.test/a.png?w=1&amp;h=2</a>`, 1},
		"what Basecamp rendered":  {open + ` url="https://pictures.example.test/a.png" caption="Seen"><figure><img src="https://pictures.example.test/a.png"><figcaption>Seen</figcaption></figure></bc-attachment>`, `<a href="https://pictures.example.test/a.png">Seen</a>`, 1},
		"the kind with its case":  {`<bc-attachment content-type="Image; x=y" url="https://pictures.example.test/a.png" caption="Seen"></bc-attachment>`, `<a href="https://pictures.example.test/a.png">Seen</a>`, 1},
		// Only what became a link is counted as one.
		"a script":               {open + ` url="javascript:alert(1)" caption="Click &lt;me&gt;"></bc-attachment>`, `Click &lt;me&gt;`, 0},
		"a path":                 {open + ` url="/4242/blobs/k/previews/full" caption="Seen"></bc-attachment>`, `Seen`, 0},
		"no address, no caption": {open + `></bc-attachment>`, ``, 0},
		// A stored file has a signed ID, and a kind such as image/png is none of this.
		"a stored picture":       {open + ` sgid="` + fileSGID + `" url="https://pictures.example.test/a.png"><figure><figcaption>sketch.png</figcaption></figure></bc-attachment>`, `<import-attachment ref="attachment-5001"></import-attachment>`, 0},
		"a picture of some kind": {`<bc-attachment content-type="image/png" url="https://pictures.example.test/a.png" caption="Seen"></bc-attachment>`, `<bc-attachment content-type="image/png" url="https://pictures.example.test/a.png" caption="Seen"></bc-attachment>`, 0},
	} {
		e := bodyExtractor(t)
		got := rewritten(t, e, `<div>See `+c.embed+`!</div>`, "", bodyFile{ID: 5001, SGID: fileSGID})
		if want := `<div>See ` + c.want + `!</div>`; got != want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
		if e.bodies.PicturesLinked != c.pictures || e.bodies.LinkPreviews != 0 {
			t.Errorf("%s: counted %+v", name, e.bodies)
		}
	}
}

func TestWhatAReaderSeesNothingOfSaysNothing(t *testing.T) {
	for body, want := range map[string]bool{
		"": true, " \n\t": true, "&nbsp; ": true, "\u200b": true, "<div>\u200b\u200c\u200d\u2060\ufeff &#8203;</div>": true, "\u200ba\u200b": false, "<br>": true, "<BR/>": true, "<div><p> <br></p></div>": true, "</div>": true, "<!-- x -->": true,
		"a": false, "&amp;": false, "<hr>": false, "<span></span>": false, "<img src=x>": false, `<a href="https://example.test/"></a>`: false,
		`<import-attachment ref="blob-k"></import-attachment>`: false, `<import-mention ref="102"></import-mention>`: false,
		`<import-embed url="https://video.example.test/"></import-embed>`: false, `<div><import-embed url="https://video.example.test/"></import-embed><br></div>`: false,
	} {
		if got := saysNothing(body); got != want {
			t.Errorf("%q says nothing: %v, want %v", body, got, want)
		}
	}
}
