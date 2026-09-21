package basecamp

import (
	"encoding/base64"
	"strings"
	"testing"
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
	return e
}

func rewritten(t *testing.T, e *Extractor, text, rendered string, files ...bodyFile) string {
	t.Helper()
	out, err := e.rewriteBody(text, rendered, files)
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
		if want := `<div>See <import-attachment ref="attachment-5001"></import-attachment> above</div>`; got != want {
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
		got, err := e.rewriteBody(text, "", nil)
		if err != nil || got != want {
			t.Errorf("%q:\n got %q (%v)\nwant %q", text, got, err, want)
		}
	}
}
