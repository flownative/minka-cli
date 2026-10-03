package basecamp_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

const referenceProject = "7006"

// anEntryOfTheTexts is an ID of an entry the texts of the reference project
// link to, as a whole number in a path.
var anEntryOfTheTexts = regexp.MustCompile(`(^|[^0-9])944[1-4]([^0-9]|$)`)

func reference(kind, ref, href, words string) string {
	return `<import-reference kind="` + kind + `" ref="` + ref + `" href="` + href + `">` + words + `</import-reference>`
}

func TestALinkToACardAPostAChatOrAnEventOfTheAccountBecomesAReferenceInEveryText(t *testing.T) {
	dir := t.TempDir() + "/bundle"
	account := fakebasecamp.New()
	result := extractInto(t, account, dir, referenceProject)
	bodies := bodiesOf(t, dir)

	page := func(path string) string { return "https://3.basecamp.com/4242/" + path }
	want := map[string]string{
		"cards 9961": `<div>Next: ` + reference("cards", "9962", page("buckets/7006/card_tables/cards/9962"), "the second card") +
			`, as said in ` + reference("cards", "9201", "https://app.basecamp.com/4242/buckets/7003/card_tables/cards/9201#__recording_9301", "a comment elsewhere") + `.</div>` +
			`<div>Not ours: ` + fakebasecamp.Link(page("buckets/7006/todos/1234"), "a to-do") + `, ` +
			fakebasecamp.Link(page("buckets/7006/card_tables/9060"), "the table") + `, ` +
			fakebasecamp.Link("https://3.basecamp.com/9999/buckets/7006/card_tables/cards/9962", "another account") + `, ` +
			fakebasecamp.Link("https://example.test/4242/buckets/7006/card_tables/cards/9962", "another host") + `.</div>`,
		"comments 9971": `<div>See ` + reference("posts", "9982", page("buckets/7006/messages/9982"), "the plan") + ` and ` +
			reference("posts", "9701", "http://app.basecamp.com/4242/buckets/7004/messages/9701#__recording_9751", "what was said") + `.</div>`,
		"posts 9981": `<div>In the chat: ` + reference("tools", "9062", page("buckets/7006/chats/9062"), "the room") + `, ` +
			reference("tools", "9062", page("buckets/7006/chats/9062@9993"), "a line") + `, ` +
			reference("tools", "9033", page("buckets/7004/chats/9033/lines/9801/options"), "a line elsewhere") + `.</div>` +
			`<div>Not ours: ` + fakebasecamp.Link(page("projects/7006"), "the project") + `, ` +
			fakebasecamp.Link(page("buckets/7006/documents/77"), "a document") + `, ` +
			fakebasecamp.Link(page("buckets/7006/message_boards/9061"), "the board") + `, ` +
			fakebasecamp.Link("https://storage.3.basecamp.com/4242/blobs/"+fakebasecamp.FileKey(1)+"/download/x.png", "a stored file") + `, ` +
			fakebasecamp.Link("https://3.basecamp.com.example.test/4242/buckets/7006/messages/9982", "a lookalike") + `, ` +
			fakebasecamp.Link(page("buckets/7006/messages/9982@1"), "a message with a line") + `.</div>`,
		"posts 9982": `<div>Plan.</div>`,
		"posts 9983": `<div>Meet at ` + reference("events", "9441", page("buckets/7006/schedule_entries/9441"), "the kick-off") + `, ` +
			reference("events", "9442", page("buckets/7006/schedule_entries/9442/occurrences/20230512"), "one Friday") + `, ` +
			reference("events", "9443", "https://app.basecamp.com/4242/buckets/7004/schedule_entries/9443/occurrences/20240229#__recording_9444", "a leap day elsewhere") + `.</div>` +
			`<div>Not ours: ` + fakebasecamp.Link("https://3.basecamp.com/9999/buckets/7006/schedule_entries/9441", "another account") + `, ` +
			fakebasecamp.Link(page("buckets/7006/schedule_entries/9441x"), "letters after the ID") + `, ` +
			fakebasecamp.Link(page("buckets/7006/schedule_entries/9441.json"), "the API") + `, ` +
			fakebasecamp.Link(page("buckets/7006/schedule_entries/9441/edit"), "a page below the entry") + `, ` +
			fakebasecamp.Link(page("buckets/7006/schedule_entries/9441@1"), "an entry with a line") + `, ` +
			fakebasecamp.Link(page("buckets/7006/schedule_entries/9442/occurrences/20230230"), "the thirtieth of February") + `, ` +
			fakebasecamp.Link(page("buckets/7006/schedule_entries/9442/occurrences/2023051"), "seven digits") + `, ` +
			fakebasecamp.Link(page("buckets/7006/schedule_entries/9442/occurrences/20230512/subscription"), "below a day") + `, ` +
			fakebasecamp.Link(page("buckets/7006/schedule_entries/9442/occurrences"), "no day") + `.</div>`,
		"messages 9991": `<div>` + reference("cards", "9961", page("buckets/7006/card_tables/cards/9961"), "Links") + ` and <a name="top">an anchor</a></div>`,
		"messages 9992": `see ` + reference("cards", "9962", "https://APP.basecamp.com/4242/buckets/7006/card_tables/cards/9962", "https://APP.basecamp.com/4242/buckets/7006/card_tables/cards/9962"),
		"messages 9993": `Here`,
	}
	for key, text := range want {
		if bodies[key] != text {
			t.Errorf("%s:\n got %q\nwant %q", key, bodies[key], text)
		}
	}
	if want := (basecamp.ReferenceCounts{Cards: 4, Posts: 2, Tools: 3, Events: 3}); result.Bodies.References != want {
		t.Errorf("references %+v, want %+v", result.Bodies.References, want)
	}
	// A link is read, never followed: asking for an entry or a day of it
	// writes at the source, at whatever address the entry is asked for.
	for _, path := range account.Requests {
		if strings.Contains(path, "schedule_entries") || anEntryOfTheTexts.MatchString(path) {
			t.Errorf("the run asked for %s", path)
		}
	}
	// The preview of a link to a card is a link all the same.
	if result.Bodies.LinkPreviews != 1 {
		t.Errorf("link previews %d, want 1", result.Bodies.LinkPreviews)
	}
}
