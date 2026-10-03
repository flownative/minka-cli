package basecamp

import (
	"context"
	"testing"

	"github.com/flownative/minka-cli/internal/bundle"
)

func linkExtractor(t *testing.T) *Extractor {
	t.Helper()
	e := bodyExtractor(t)
	e.account = "4242"
	return e
}

func TestAnAddressShowsACardAPostAChatOrAnEventOfTheAccount(t *testing.T) {
	const bucket = "https://3.basecamp.com/4242/buckets/7/"
	shows := map[string]struct {
		kind bundle.Kind
		ref  string
	}{
		bucket + "card_tables/cards/2":                         {bundle.Cards, "2"},
		"https://app.basecamp.com/4242/buckets/7/messages/3":   {bundle.Posts, "3"},
		bucket + "chats/4":                                     {bundle.Tools, "4"},
		bucket + "chats/4@5":                                   {bundle.Tools, "4"},
		bucket + "chats/4/lines/5":                             {bundle.Tools, "4"},
		bucket + "chats/4/lines/5/options":                     {bundle.Tools, "4"},
		bucket + "card_tables/cards/2#__recording_9":           {bundle.Cards, "2"},
		bucket + "messages/3?page=2#__recording_9":             {bundle.Posts, "3"},
		bucket + "card_tables/cards/2/":                        {bundle.Cards, "2"},
		"HTTP://3.BaseCamp.COM/4242/buckets/7/messages/3":      {bundle.Posts, "3"},
		" \n" + bucket + "card_tables/cards/2\t":               {bundle.Cards, "2"},
		bucket + "card_tables/cards/002":                       {bundle.Cards, "2"},
		"https://3.basecamp.com/4242/buckets/8/messages/3":     {bundle.Posts, "3"},
		"https://app.basecamp.com/4242/buckets/7/chats/4@5#hi": {bundle.Tools, "4"},
		bucket + "messages/3/./":                               {bundle.Posts, "3"},
		bucket + "todos/5/../../card_tables/cards/2":           {bundle.Cards, "2"},
		// A day of an entry names the entry.
		bucket + "schedule_entries/6":                                      {bundle.Events, "6"},
		bucket + "schedule_entries/6/":                                     {bundle.Events, "6"},
		bucket + "schedule_entries/006":                                    {bundle.Events, "6"},
		"https://app.basecamp.com/4242/buckets/7/schedule_entries/6#x":     {bundle.Events, "6"},
		bucket + "schedule_entries/6/occurrences/20230512":                 {bundle.Events, "6"},
		bucket + "schedule_entries/6/occurrences/20240229#__recording_9":   {bundle.Events, "6"},
		bucket + "schedule_entries/6/occurrences/20230512?x=1":             {bundle.Events, "6"},
		bucket + "schedule_entries/6/occurrences/20230512/":                {bundle.Events, "6"},
		bucket + "todos/5/../../schedule_entries/6/occurrences/20230512":   {bundle.Events, "6"},
		bucket + "schedule_entries/6/edit/../occurrences/20230512":         {bundle.Events, "6"},
		"HTTPS://3.BASECAMP.COM/4242/buckets/7/schedule_entries/6":         {bundle.Events, "6"},
		bucket + "schedule_entries/6/occurrences/%32%30%32%33%30%35%31%32": {bundle.Events, "6"},
	}
	for address, want := range shows {
		kind, ref, found := linkExtractor(t).referenceTo(address)
		if !found || kind != want.kind || ref != want.ref {
			t.Errorf("%q: got %q %q %v, want %q %q", address, kind, ref, found, want.kind, want.ref)
		}
	}

	for _, address := range []string{
		"",
		// Another account.
		"https://3.basecamp.com/4243/buckets/7/card_tables/cards/2",
		"https://3.basecamp.com/42420/buckets/7/card_tables/cards/2",
		"https://3.basecamp.com/buckets/7/card_tables/cards/2",
		"https://3.basecamp.com/somewhere/4242/buckets/7/card_tables/cards/2",
		"https://3.basecamp.com//4242/buckets/7/card_tables/cards/2",
		"https://app.basecamp.com//4242/buckets/7/messages/3",
		// Another kind of record.
		bucket + "todos/2",
		bucket + "documents/2",
		bucket + "uploads/2",
		bucket + "question_answers/2",
		bucket + "recordings/2",
		bucket + "card_tables/2",
		bucket + "card_tables/lists/2",
		bucket + "message_boards/2",
		"https://3.basecamp.com/4242/projects/7",
		// Another host.
		"https://basecamp.com/4242/buckets/7/card_tables/cards/2",
		"https://storage.3.basecamp.com/4242/buckets/7/card_tables/cards/2",
		"https://3.basecampapi.com/4242/buckets/7/card_tables/cards/2",
		"https://3.basecamp.com.example.test/4242/buckets/7/card_tables/cards/2",
		"https://example.test/4242/buckets/7/card_tables/cards/2",
		"https://3.basecamp.com:8443/4242/buckets/7/card_tables/cards/2",
		"https://somebody@3.basecamp.com/4242/buckets/7/card_tables/cards/2",
		// No page of the web.
		"ftp://3.basecamp.com/4242/buckets/7/card_tables/cards/2",
		"//3.basecamp.com/4242/buckets/7/card_tables/cards/2",
		"/4242/buckets/7/card_tables/cards/2",
		// No record.
		bucket + "card_tables/cards/",
		bucket + "card_tables/cards/2x",
		bucket + "card_tables/cards/99999999999999999999",
		bucket + "card_tables/cards/2@5",
		bucket + "messages/3@5",
		"https://3.basecamp.com/4242/buckets/x/card_tables/cards/2",
		"https://3.basecamp.com/4242/buckets/7/card_tables/cards/2.json",
		// No entry, or no day of one.
		"https://3.basecamp.com/4243/buckets/7/schedule_entries/6",
		"https://3.basecampapi.com/4242/buckets/7/schedule_entries/6.json",
		bucket + "schedule_entries/",
		bucket + "schedule_entries/6x",
		bucket + "schedule_entries/6.json",
		bucket + "schedule_entries/6@5",
		bucket + "schedule_entries/99999999999999999999",
		bucket + "schedule_entries/6/edit",
		bucket + "schedule_entries/6/occurrences",
		bucket + "schedule_entries/6/occurrences/",
		bucket + "schedule_entries/6/occurrences/20230512.json",
		bucket + "schedule_entries/6/occurrences/20230512/subscription",
		bucket + "schedule_entries/6/occurrences/2023-05-12",
		bucket + "schedule_entries/6/occurrences/2023051",
		bucket + "schedule_entries/6/occurrences/202305120",
		bucket + "schedule_entries/6/occurrences/20230230",
		bucket + "schedule_entries/6/occurrences/20231301",
		bucket + "schedule_entries/6/occurrences/20230500",
		bucket + "schedule_entries/6/occurrences/20230229",
		bucket + "schedule_entries/6/occurrences/00000101",
		bucket + "schedule_entries/6/days/20230512",
		bucket + "schedule_entries/6/occurrences/20230512/../../../todos/5",
		// A path that climbs out of the page of a record.
		bucket + "card_tables/cards/2/../../../todos/5",
		bucket + "card_tables/cards/2/%2E%2E/%2e%2e/../todos/5",
		bucket + "chats/4/../../../../../4243/buckets/7/chats/4",
	} {
		if kind, ref, found := linkExtractor(t).referenceTo(address); found {
			t.Errorf("%q shows %s %s, want nothing", address, kind, ref)
		}
	}
}

// askedSource is a source that answers nothing and keeps what it was asked.
type askedSource struct{ paths []string }

func (s *askedSource) Get(_ context.Context, path string) ([]byte, error) {
	s.paths = append(s.paths, path)
	return []byte("{}"), nil
}

// A link to an entry or a day of it becomes a reference without a request:
// asking Basecamp for either writes at the source. The source here stands
// without the guard of a run, so that a request the guard would refuse shows
// as well.
func TestALinkToAnEntryIsReadNeverFollowed(t *testing.T) {
	e := linkExtractor(t)
	asked := &askedSource{}
	e.source = asked
	const entry = "https://3.basecamp.com/4242/buckets/7/schedule_entries/6"
	text := `<a href="` + entry + `">entry</a> <a href="` + entry + `/occurrences/20230512">day</a> <a href="` + entry + `/edit">no day</a>`
	want := `<import-reference kind="events" ref="6" href="` + entry + `">entry</import-reference> ` +
		`<import-reference kind="events" ref="6" href="` + entry + `/occurrences/20230512">day</import-reference> ` +
		`<a href="` + entry + `/edit">no day</a>`
	if got := rewritten(t, e, text, ""); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if len(asked.paths) != 0 {
		t.Errorf("the links were followed: %v", asked.paths)
	}
}

const (
	cardAddress = "https://3.basecamp.com/4242/buckets/7/card_tables/cards/2"
	postAddress = "https://app.basecamp.com/4242/buckets/7/messages/3"
	cardOpen    = `<import-reference kind="cards" ref="2" href="` + cardAddress + `">`
	postOpen    = `<import-reference kind="posts" ref="3" href="` + postAddress + `">`
)

func TestALinkToARecordBecomesAReferenceWithTheWordsOfTheLink(t *testing.T) {
	for name, c := range map[string]struct {
		text, want string
		cards      int
	}{
		"a link":                 {`<div>See <a href="` + cardAddress + `">the card</a>.</div>`, `<div>See ` + cardOpen + `the card</import-reference>.</div>`, 1},
		"what else it carries":   {`<a title="t" href="` + cardAddress + `" target="_blank" rel="noopener">the card</a>`, cardOpen + `the card</import-reference>`, 1},
		"in capitals":            {`<A HREF="` + cardAddress + `">the card</A>`, cardOpen + `the card</import-reference>`, 1},
		"quoted otherwise":       {`<a href='` + cardAddress + `'>x</a> <a href=` + cardAddress + `>y</a>`, cardOpen + `x</import-reference> ` + cardOpen + `y</import-reference>`, 2},
		"words with markup":      {`<a href="` + cardAddress + `"><strong>the</strong> <span class="x">card</span> &amp; more</a>`, cardOpen + `the card &amp; more</import-reference>`, 1},
		"no words":               {`<a href="` + cardAddress + `"></a>`, cardOpen + `</import-reference>`, 1},
		"one that closes itself": {`<a href="` + cardAddress + `"/>then`, cardOpen + `</import-reference>then`, 1},
		"the address as it stood": {`<a href="` + cardAddress + `?a=1&amp;b=&quot;x&quot;#__recording_9">x</a>`,
			`<import-reference kind="cards" ref="2" href="` + cardAddress + `?a=1&amp;b=&#34;x&#34;#__recording_9">x</import-reference>`, 1},
		"whitespace around the address": {`<a href=" ` + cardAddress + ` ">x</a>`, `<import-reference kind="cards" ref="2" href=" ` + cardAddress + ` ">x</import-reference>`, 1},
		// A link ends where the next one begins, and where the text ends.
		"never closed":            {`<p><a href="` + cardAddress + `">the card</p>`, `<p>` + cardOpen + `the card</import-reference></p>`, 1},
		"never closed at the end": {`<a href="` + cardAddress + `">the card`, cardOpen + `the card</import-reference>`, 1},
		"the next link begins":    {`<a href="` + cardAddress + `">one<a href="` + postAddress + `">two</a>`, cardOpen + `one</import-reference>` + postOpen + `two</import-reference>`, 1},
		"another link begins":     {`<a href="` + cardAddress + `">one<a href="https://example.test/">two</a>`, cardOpen + `one</import-reference><a href="https://example.test/">two</a>`, 1},
		"after another link":      {`<a href="https://example.test/">one<a href="` + cardAddress + `">two</a>`, `<a href="https://example.test/">one` + cardOpen + `two</import-reference>`, 1},
		"an end without a link":   {`<a href="https://example.test/">one</a></a> <a href="` + cardAddress + `">two</a></a>`, `<a href="https://example.test/">one</a></a> ` + cardOpen + `two</import-reference></a>`, 1},
		// The words are plain text.
		"a mention in the words":   {`<a href="` + cardAddress + `">ask ` + mentionOpen + ` sgid="` + signed("gid://bc3/Person/102") + `">` + figure("102", "Grace Sample") + `</bc-attachment></a>`, cardOpen + `ask Grace Sample</import-reference>`, 1},
		"escaped once":             {`<a href="` + cardAddress + `">&lt;b&gt; &amp;amp; x > y &quot;q&quot; &#39;s&#39; &nbsp;</a>`, cardOpen + "&lt;b&gt; &amp;amp; x &gt; y &#34;q&#34; &#39;s&#39; \u00a0</import-reference>", 1},
		"a line break":             {`<a href="` + cardAddress + `">one<br>two<br/>three</a>`, cardOpen + `one two three</import-reference>`, 1},
		"a comment":                {`<a href="` + cardAddress + `">one<!-- x -->two</a>`, cardOpen + `onetwo</import-reference>`, 1},
		"a frame":                  {`<a href="` + cardAddress + `">see <iframe src="` + frameAddress + `">no frames</iframe>it</a>`, cardOpen + `see it</import-reference>`, 1},
		"an embed of another kind": {`<a href="` + cardAddress + `">x <bc-attachment content-type="other"><b>y</b></bc-attachment> z</a>`, cardOpen + `x y z</import-reference>`, 1},
		// An element that opened before the link ends where it did, behind the
		// reference; one that opened within it gives its words.
		"an element around":  {`<b>x <a href="` + cardAddress + `">y</b> z</a> after`, `<b>x ` + cardOpen + `y</import-reference></b> z</a> after`, 1},
		"an element within":  {`<b>x <a href="` + cardAddress + `"><i>y<em>z</em></i> w</b> v</a>`, `<b>x ` + cardOpen + `yz w</import-reference></b> v</a>`, 1},
		"an end of a void":   {`<a href="` + cardAddress + `">x<img src="p"></img></br>y</a>`, cardOpen + `xy</import-reference>`, 1},
		"a second reference": {`<a href="` + cardAddress + `"><b>x</a> <b>y <a href="` + cardAddress + `">z</b> w</a>`, cardOpen + `x</import-reference> <b>y ` + cardOpen + `z</import-reference></b> w</a>`, 2},
		// A reference ends where a block begins or ends, and the block stays.
		"a block within":          {`<a href="` + cardAddress + `">one<div>two</div>three</a>`, cardOpen + `one</import-reference><div>two</div>three</a>`, 1},
		"a block around":          {`<li><a href="` + cardAddress + `">one</li><li>two</li>`, `<li>` + cardOpen + `one</import-reference></li><li>two</li>`, 1},
		"another link stays":      {`<a href="https://example.test/" class='x'>one</a> <a name="top">two</a> <a>three</a>`, `<a href="https://example.test/" class='x'>one</a> <a name="top">two</a> <a>three</a>`, 0},
		"a link to another kind":  {`<a href="https://3.basecamp.com/4242/buckets/7/todos/2">one</a>`, `<a href="https://3.basecamp.com/4242/buckets/7/todos/2">one</a>`, 0},
		"an address as the words": {`<a href="` + cardAddress + `">` + cardAddress + `</a>`, cardOpen + cardAddress + `</import-reference>`, 1},
	} {
		e := linkExtractor(t)
		if got := rewritten(t, e, c.text, ""); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, c.want)
		}
		posts := 0
		if name == "the next link begins" {
			posts = 1
		}
		if want := (ReferenceCounts{Cards: c.cards, Posts: posts}); e.bodies.References != want {
			t.Errorf("%s: counted %+v, want %+v", name, e.bodies.References, want)
		}
	}
}

func TestAPreviewOrAPictureOfARecordBecomesAReference(t *testing.T) {
	e := linkExtractor(t)
	text := `<div>` + previewOpen + ` href="` + postAddress + `" title="The &lt;plan&gt;"></bc-attachment> and ` +
		`<bc-attachment content-type="image" url="` + cardAddress + `" caption="A card"></bc-attachment> and ` +
		previewOpen + ` href="https://example.test/" title="Elsewhere"></bc-attachment></div>`
	want := `<div>` + postOpen + `The &lt;plan&gt;</import-reference> and ` + cardOpen + `A card</import-reference> and <a href="https://example.test/">Elsewhere</a></div>`
	if got := rewritten(t, e, text, ""); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	// Both are links all the same.
	if want := (BodyCounts{LinkPreviews: 2, PicturesLinked: 1, References: ReferenceCounts{Cards: 1, Posts: 1}}); e.bodies != want {
		t.Errorf("counted %+v, want %+v", e.bodies, want)
	}
}

func TestALinkToAChatOrALineOfItIsAReferenceToTheChat(t *testing.T) {
	e := linkExtractor(t)
	text := `<a href="https://3.basecamp.com/4242/buckets/7/chats/4">room</a>, <a href="https://3.basecamp.com/4242/buckets/7/chats/4@5">line</a>`
	want := `<import-reference kind="tools" ref="4" href="https://3.basecamp.com/4242/buckets/7/chats/4">room</import-reference>, ` +
		`<import-reference kind="tools" ref="4" href="https://3.basecamp.com/4242/buckets/7/chats/4@5">line</import-reference>`
	if got := rewritten(t, e, text, ""); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if want := (ReferenceCounts{Tools: 2}); e.bodies.References != want {
		t.Errorf("counted %+v, want %+v", e.bodies.References, want)
	}
}

func TestAnEmbedInTheWordsOfAReferenceGivesItsWordsAndRefersToNothing(t *testing.T) {
	e := linkExtractor(t)
	file := bodyFile{ID: 5001, SGID: fileSGID, Key: "5a1b2c3d-0001-4000-8000-000000005001", Filename: "sketch.png"}
	text := `<a href="` + cardAddress + `">` +
		mentionOpen + ` sgid="` + signed("gid://bc3/Person/105") + `">` + figure("105", "Rendered otherwise") + `</bc-attachment>, ` +
		mentionOpen + ` sgid="` + signed("gid://bc3/Person/102") + `"></bc-attachment>, ` +
		mentionOpen + ` sgid="` + signed("gid://bc3/Person/777") + `">` + figure("777", "Gone Person") + `</bc-attachment>, ` +
		previewOpen + ` href="` + postAddress + `" title="The &lt;plan&gt;"></bc-attachment>, ` +
		`<bc-attachment content-type="image" url="` + cardAddress + `" caption="A card"></bc-attachment>, ` +
		`<bc-attachment sgid="` + fileSGID + `" content-type="image/png" caption="A sketch"><figure><figcaption>A sketch</figcaption></figure></bc-attachment>` +
		`<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"><hr></bc-attachment>.</a>`
	got, files, err := e.rewriteBody(aBody, text, "", []bodyFile{file})
	if err != nil {
		t.Fatal(err)
	}
	if want := cardOpen + `Niklaus &lt;Nowhere&gt;, Grace Sample, Gone Person, The &lt;plan&gt;, A card, A sketch.</import-reference>`; got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if len(files) != 0 {
		t.Errorf("the words refer to files %v", files)
	}
	if want := (BodyCounts{References: ReferenceCounts{Cards: 1}}); e.bodies != want {
		t.Errorf("counted %+v, want %+v", e.bodies, want)
	}
	// Nobody comes into the bundle for a name in the words.
	if people := e.people.sorted(); len(people) != 0 {
		t.Errorf("the words refer to %v", people)
	}
}
