package basecamp_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

func campfire(a *fakebasecamp.Account) *fakebasecamp.Tool { return &a.Projects[3].Dock[3] }
func quiet(a *fakebasecamp.Account) *fakebasecamp.Tool    { return &a.Projects[3].Dock[4] }
func side(a *fakebasecamp.Account) *fakebasecamp.Tool     { return &a.Projects[3].Dock[5] }

// partsOf are the files the manifest of a bundle names for its messages. A
// bundle carries the one file or parts, and never both.
func partsOf(t *testing.T, dir string) []string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Parts map[string][]string `json:"parts"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}

	lying, err := filepath.Glob(filepath.Join(dir, "messages*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range lying {
		lying[i] = filepath.Base(lying[i])
	}
	named := manifest.Parts["messages"]
	if named == nil && len(lying) > 0 {
		named = []string{"messages.jsonl"}
	}
	if !slices.Equal(lying, named) || len(manifest.Parts["messages"]) == 1 {
		t.Fatalf("the bundle holds %v and its manifest names the parts %v", lying, manifest.Parts["messages"])
	}
	return manifest.Parts["messages"]
}

// messages are the lines of a bundle in their order, whether it carries them
// in one file or in parts.
func messages(t *testing.T, dir string) []map[string]any {
	t.Helper()
	parts := partsOf(t, dir)
	if parts == nil {
		return recordsOrNone(t, dir, bundle.Messages)
	}
	var all []map[string]any
	for _, part := range parts {
		all = append(all, recordsIn(t, filepath.Join(dir, part))...)
	}
	return all
}

func TestALineOfEveryKindBecomesAMessage(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), talkProject)
	lines := messages(t, dir)

	want := map[string]any{
		"sourceId": "9802", "createdAt": "2023-04-20T09:02:00.000Z", "updatedAt": "2023-04-20T09:03:00.000Z", "createdBy": "102",
		"projectSourceId": "7004", "toolSourceId": "9033",
		"body": `<div><import-mention ref="101">Ada Example</import-mention> look: <a href="https://example.test/news?a=1&amp;b=2">News &amp; weather</a></div>` +
			`<import-embed url="https://video.example.test/embed/42"></import-embed>`,
	}
	if got := recordWithID(t, lines, "9802"); !reflect.DeepEqual(got, want) {
		t.Errorf("message:\n got %v\nwant %v", got, want)
	}

	for id, body := range map[string]string{
		// A plain line and the line of a bot hold no embed and pass as they are.
		"9801": `Good morning<br>all, see <a href="https://example.test/agenda">https://example.test/agenda</a>`,
		"9804": `<b>Build 42</b> passed`,
		// Files alone: a reference each, in the order of the line.
		"9803": `<import-attachment ref="blob-0a1b2c3d-0001-4000-8000-00000000000a"></import-attachment><import-attachment ref="blob-0a1b2c3d-0002-4000-8000-00000000000b"></import-attachment>`,
		// A preview without a title, and one whose address is none of the web.
		"9806": `<div><a href="https://example.test/plain">https://example.test/plain</a> or Click &lt;me&gt;</div>`,
	} {
		if got := recordWithID(t, lines, id)["body"]; got != body {
			t.Errorf("message %s:\n got %s\nwant %s", id, got, body)
		}
	}

	if want := map[string]int{"text": 4, "richText": 2, "upload": 2, "integration": 1}; !reflect.DeepEqual(result.MessagesByType, want) {
		t.Errorf("messages by type %v, want %v", result.MessagesByType, want)
	}
	if want := (basecamp.BodyCounts{Files: 4, FilesDropped: 2, LinkPreviews: 3, PicturesLinked: 1, Embeds: 1}); result.Bodies != want {
		t.Errorf("bodies %+v, want %+v", result.Bodies, want)
	}
}

// A line a bundle cannot carry costs that line and nothing else: one exotic line
// of one chat would otherwise cost the extraction of a whole account.
func TestALineABundleCannotCarryIsLeftOutAndTheRestOfItsChatIsWritten(t *testing.T) {
	account := fakebasecamp.New()
	// Nobody but the lines that are left out names this person.
	campfire(account).Lines[8].Creator = stranger()
	campfire(account).Lines[9].Creator = stranger()
	campfire(account).Lines[5].Status = "trashed"
	dir, result := extract(t, account, talkProject)
	written := sourceIDs(messages(t, dir))

	for _, id := range []string{"9809", "9810", "9806"} {
		if slices.Contains(written, id) {
			t.Errorf("line %s, which a bundle cannot carry, was written", id)
		}
	}
	for _, id := range []string{"9801", "9802", "9803", "9804", "9805", "9807"} {
		if !slices.Contains(written, id) {
			t.Errorf("line %s of the chat was not written: %v", id, written)
		}
	}
	if slices.Contains(sourceIDs(records(t, dir, bundle.People)), "555") {
		t.Error("the writer of a line that is left out came into the bundle")
	}
	for _, line := range []string{"9809", "9810", "9806"} {
		if asked := asked(account, "recordings/"+line+"/"); len(asked) != 0 {
			t.Errorf("the boosts of line %s, which is left out, were asked for: %v", line, asked)
		}
	}
	if want := map[string]int{"text": 4, "richText": 1, "upload": 2, "integration": 1}; !reflect.DeepEqual(result.MessagesByType, want) {
		t.Errorf("messages by type %v, want %v", result.MessagesByType, want)
	}
}

// Whoever reads a run has to be able to tell a sound effect from something that
// matters, and only the word the source used says which it was. The kind of a
// line is looked at before its status and its times, so a line nobody has a word
// for is left out for that and counted once.
func TestTheWordALineIsLeftOutForIsCountedWithTheChatItStoodIn(t *testing.T) {
	account := fakebasecamp.New()
	campfire(account).Lines[3].Type = fakebasecamp.UnknownLine
	campfire(account).Lines[4].Type = "Chat::Lines::Poll"
	campfire(account).Lines[5].Status = "trashed"
	side(account).Lines[0].Type = "Chat::Lines::Poll"
	_, result := extract(t, account, talkProject)

	// The line the fake holds thrown away and without a time is one of an
	// unknown kind: it counts under its kind and never reaches the other two
	// guards.
	want := []basecamp.LeftOutRecords{
		{Record: "line", Field: "type", Value: fakebasecamp.UnknownLine, In: "chat", InSourceID: "9033", LeftOut: 3},
		{Record: "line", Field: "type", Value: "Chat::Lines::Poll", In: "chat", InSourceID: "9033", LeftOut: 1},
		{Record: "line", Field: "status", Value: "trashed", In: "chat", InSourceID: "9033", LeftOut: 1},
		{Record: "line", Field: "type", Value: "Chat::Lines::Poll", In: "chat", InSourceID: "9035", LeftOut: 1},
	}
	if len(result.RecordsLeftOut) != len(want) {
		t.Fatalf("the records left out are %v, want %v", result.RecordsLeftOut, want)
	}
	for _, one := range want {
		if !slices.Contains(result.RecordsLeftOut, one) {
			t.Errorf("the records left out are %v, want %v among them", result.RecordsLeftOut, one)
		}
	}
	// What is left of the chats comes, and what is left out counts nowhere else.
	if want := map[string]int{"text": 2, "richText": 1, "upload": 2, "integration": 0}; !reflect.DeepEqual(result.MessagesByType, want) {
		t.Errorf("messages by type %v, want %v", result.MessagesByType, want)
	}
}

// A run that left nothing out says so with an empty list, not with nothing.
func TestARunThatLeftNoRecordOutReportsNone(t *testing.T) {
	_, result := extract(t, fakebasecamp.New(), boardProject)
	if got := result.RecordsLeftOut; len(got) != 0 || got == nil {
		t.Errorf("the records left out are %v, want an empty list", got)
	}
}

func TestAFileOfALineIsNamedByTheKeyOfItsStoredFileOrDropped(t *testing.T) {
	account := fakebasecamp.New()
	// Nobody but the line that is left out names this person.
	campfire(account).Lines[7].Creator = stranger()
	// Where a file is downloaded counts before where it is previewed.
	campfire(account).Lines[2].Uploads[1].URL = "https://preview.basecamp.test/4242/blobs/another-key/previews/full"
	dir, result := extract(t, account, talkProject)
	lines := messages(t, dir)

	// The key stands where the file is previewed when it does not where it is
	// downloaded. A file without a key that can be part of a name is dropped.
	if got, want := recordWithID(t, lines, "9807")["body"], `<import-attachment ref="blob-0a1b2c3d-0003-4000-8000-00000000000c"></import-attachment>`; got != want {
		t.Errorf("message 9807:\n got %s\nwant %s", got, want)
	}
	if body := recordWithID(t, lines, "9803")["body"].(string); !strings.HasSuffix(body, `<import-attachment ref="blob-0a1b2c3d-0002-4000-8000-00000000000b"></import-attachment>`) {
		t.Errorf("message 9803: %s", body)
	}
	if slices.Contains(sourceIDs(lines), "9808") {
		t.Error("a line of which no file can be named was written")
	}
	if result.UploadLinesLeftOut != 1 || result.Bodies.FilesDropped != 2 {
		t.Errorf("%d lines left out and %d files dropped, want 1 and 2", result.UploadLinesLeftOut, result.Bodies.FilesDropped)
	}
	if slices.Contains(sourceIDs(records(t, dir, bundle.People)), "555") {
		t.Error("the writer of a line that is left out came into the bundle")
	}
	if len(asked(account, "recordings/9808/")) != 0 {
		t.Error("the boosts of a line that is left out were asked for")
	}
}

func TestOnlyAKeyThatCanBePartOfANameNamesAFile(t *testing.T) {
	for key, usable := range map[string]bool{
		"0a1b2c3d-0001-4000-8000-00000000000a": true, "Key_with-every_KIND-of-letter-09": true,
		"": false, "..": false, "a.b": false, "a%2Fb": false, "a b": false, "a\"b": false, "ä": false,
	} {
		account := fakebasecamp.New()
		campfire(account).Lines = []fakebasecamp.Line{{ID: 9803, Type: fakebasecamp.UploadLine, Creator: campfire(account).Lines[0].Creator,
			CreatedAt: "2023-04-20T09:04:00.000Z", UpdatedAt: "2023-04-20T09:04:00.000Z", Uploads: []fakebasecamp.Upload{{
				Filename: "a.png", URL: "https://preview.basecamp.test/4242/blobs/" + key + "/previews/full",
				DownloadURL: "https://storage.basecamp.test/4242/blobs/" + key + "/download/a.png"}}}}
		side(account).Lines = nil
		dir, _ := extract(t, account, talkProject)

		lines := messages(t, dir)
		if usable != (len(lines) == 1) {
			t.Errorf("the key %q: %d lines were written", key, len(lines))
		}
		if usable && lines[0]["body"] != `<import-attachment ref="blob-`+key+`"></import-attachment>` {
			t.Errorf("the key %q: the body is %v", key, lines[0]["body"])
		}
	}
}

func TestTheLinesOfAChatAreWrittenOldestFirstHoweverBasecampListsThem(t *testing.T) {
	first, _ := extract(t, fakebasecamp.New(), talkProject)
	want := []string{"9801", "9802", "9803", "9804", "9805", "9806", "9807", "9811", "9812"}
	if got := sourceIDs(messages(t, first)); !slices.Equal(got, want) {
		t.Errorf("messages %v, want %v", got, want)
	}

	// The moment counts, not the letters of a time, and the number of an ID
	// where two lines are of one moment, not its letters.
	account := fakebasecamp.New()
	lines := side(account).Lines
	lines[0].CreatedAt = "2023-04-20T11:29:00.000+02:00"
	lines = append(lines, fakebasecamp.Line{ID: 10000, Type: fakebasecamp.TextLine, Creator: lines[1].Creator, CreatedAt: lines[1].CreatedAt, UpdatedAt: lines[1].UpdatedAt, Content: "Pang"})
	side(account).Lines = lines
	dir, _ := extract(t, account, talkProject)
	if got, want := sourceIDs(messages(t, dir))[7:], []string{"9812", "9811", "10000"}; !slices.Equal(got, want) {
		t.Errorf("messages %v, want %v", got, want)
	}
}

func TestALineMetTwiceIsWrittenOnce(t *testing.T) {
	account := fakebasecamp.New()
	side(account).Lines = append(side(account).Lines, side(account).Lines[0])
	dir, _ := extract(t, account, talkProject)
	if got, want := sourceIDs(messages(t, dir))[7:], []string{"9811", "9812"}; !slices.Equal(got, want) {
		t.Errorf("messages %v, want %v", got, want)
	}
}

// talkFiles reads what a run over message boards and chats wrote.
func talkFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, name := range append([]string{"categories.jsonl", "posts.jsonl", "comments.jsonl", "reactions.jsonl", "people.jsonl", "tools.jsonl", "manifest.json"}, partsOf(t, dir)...) {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(content)
	}
	return files
}

func TestABundleOfTalksDoesNotDependOnTheSizeOfAPage(t *testing.T) {
	var first map[string]string
	for _, size := range []int{1, 2, 100} {
		account := fakebasecamp.New()
		account.PageSize = size
		dir, _ := extractInParts(t, account, 3, talkProject)

		files := talkFiles(t, dir)
		if first == nil {
			first = files
			// With one entry on the first page, every list there is has a second.
			for _, list := range []string{"chats/9033/lines.json", "message_boards/9030/messages.json", "type=Message&bucket=7004&status=archived", "recordings/9806/boosts.json"} {
				if requests := asked(account, list); len(requests) < 3 {
					t.Errorf("%s was asked for %d times, want page after page until one is empty", list, len(requests))
				}
			}
			continue
		}
		if !reflect.DeepEqual(files, first) {
			t.Errorf("pages of %d give another bundle than pages of 1", size)
		}
	}
	if strings.Count(first["posts.jsonl"], "\n") != 6 || strings.Count(first["messages-0001.jsonl"], "\n") != 3 || len(first) != 11 {
		t.Errorf("pages of 1 lost records: %d files", len(first))
	}
}

func TestTwoRunsOverTheSameTalksWriteTheSameBytes(t *testing.T) {
	for _, perPart := range []int{0, 3} {
		first, _ := extractInParts(t, fakebasecamp.New(), perPart, talkProject)

		// The order a source lists things in is not what a bundle depends on.
		shuffled := fakebasecamp.New()
		project := &shuffled.Projects[3]
		slices.Reverse(project.Dock)
		slices.Reverse(project.Categories)
		for i := range project.Dock {
			tool := &project.Dock[i]
			slices.Reverse(tool.Posts)
			slices.Reverse(tool.Lines)
			for j := range tool.Posts {
				slices.Reverse(tool.Posts[j].Comments)
			}
			for j := range tool.Lines {
				slices.Reverse(tool.Lines[j].Boosts)
			}
		}
		second, _ := extractInParts(t, shuffled, perPart, talkProject)

		a, b := talkFiles(t, first), talkFiles(t, second)
		if len(a) != len(b) {
			t.Errorf("parts of %d: the runs wrote %d and %d files", perPart, len(a), len(b))
		}
		for name := range a {
			if a[name] != b[name] {
				t.Errorf("parts of %d: %s differs between two runs:\n%s\n%s", perPart, name, a[name], b[name])
			}
		}
	}
}

func TestADiscardedRunOverTalksAsksTheSameCountsTheSameAndNamesTheSameParts(t *testing.T) {
	written := fakebasecamp.New()
	dir, want := extractInParts(t, written, 3, talkProject)

	dry := fakebasecamp.New()
	writer := bundle.Discard()
	extractor := basecamp.Extractor{Source: dry, Avatars: dry, Files: dry, MessagesPerPart: 3}
	got, err := extractor.Extract(t.Context(), []string{talkProject}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(dry.Requests, written.Requests) {
		t.Errorf("a dry run asked for\n%v\na real one for\n%v", dry.Requests, written.Requests)
	}
	if want = asADryRunKnowsIt(want); !reflect.DeepEqual(got, want) {
		t.Errorf("a dry run reports\n%+v\na real one\n%+v", got, want)
	}
	wantCounts := map[bundle.Kind]int{bundle.People: 7, bundle.Projects: 1, bundle.Tools: 5, bundle.Categories: 4, bundle.Posts: 6,
		bundle.Messages: 9, bundle.Comments: 3, bundle.Reactions: 4, bundle.Attachments: 10}
	if counts := writer.Counts(); !reflect.DeepEqual(counts, wantCounts) {
		t.Errorf("counts %v, want %v", counts, wantCounts)
	}
	if len(messages(t, dir)) != 9 || len(records(t, dir, bundle.Posts)) != 6 || len(records(t, dir, bundle.Categories)) != 4 {
		t.Error("the real run wrote other numbers than the dry one counted")
	}
	if parts := writer.Parts()[bundle.Messages]; !slices.Equal(parts, partsOf(t, dir)) || len(parts) != 4 {
		t.Errorf("a dry run names the parts %v, a real one %v", parts, partsOf(t, dir))
	}
}

// chatOf makes so many plain lines for a chat, numbered from an ID on.
func chatOf(account *fakebasecamp.Account, from int64, count int) []fakebasecamp.Line {
	lines := make([]fakebasecamp.Line, count)
	for i := range lines {
		at := fmt.Sprintf("2023-05-01T%02d:%02d:%02d.000Z", 10+i/3600, i/60%60, i%60)
		lines[i] = fakebasecamp.Line{ID: from + int64(i), Type: fakebasecamp.TextLine, Creator: &account.People[1], CreatedAt: at, UpdatedAt: at, Content: "A line"}
	}
	return lines
}

func TestAPartHoldsLinesOfOneChatAndIsCutAtItsSize(t *testing.T) {
	const perPart = 4
	for name, c := range map[string]struct {
		chats [3]int
		want  [][2]any
	}{
		"two chats under the size":       {[3]int{2, 0, 3}, [][2]any{{"9033", 2}, {"9035", 3}}},
		"no line anywhere but the last":  {[3]int{0, 0, 9}, [][2]any{{"9035", 4}, {"9035", 4}, {"9035", 1}}},
		"a chat of one more than a part": {[3]int{5, 1, 0}, [][2]any{{"9033", 4}, {"9033", 1}, {"9034", 1}}},
		"a chat of exactly a part":       {[3]int{4, 1, 0}, [][2]any{{"9033", 4}, {"9034", 1}}},
		"a chat without a line between":  {[3]int{1, 0, 1}, [][2]any{{"9033", 1}, {"9035", 1}}},
		"every chat has its parts":       {[3]int{8, 5, 4}, [][2]any{{"9033", 4}, {"9033", 4}, {"9034", 4}, {"9034", 1}, {"9035", 4}}},
	} {
		account := fakebasecamp.New()
		campfire(account).Lines = chatOf(account, 20000, c.chats[0])
		quiet(account).Lines = chatOf(account, 30000, c.chats[1])
		side(account).Lines = chatOf(account, 40000, c.chats[2])
		dir, _ := extractInParts(t, account, perPart, talkProject)

		parts := partsOf(t, dir)
		if len(parts) != len(c.want) {
			t.Errorf("%s: the parts are %v, want %d of them", name, parts, len(c.want))
			continue
		}
		var all []string
		for i, part := range parts {
			// Numbered through the bundle, without a gap.
			if want := fmt.Sprintf("messages-%04d.jsonl", i+1); part != want {
				t.Errorf("%s: part %d is called %s, want %s", name, i+1, part, want)
			}
			lines := recordsIn(t, filepath.Join(dir, part))
			if len(lines) != c.want[i][1] {
				t.Errorf("%s: %s holds %d lines, want %d", name, part, len(lines), c.want[i][1])
			}
			for _, line := range lines {
				if line["toolSourceId"] != c.want[i][0] {
					t.Errorf("%s: %s holds a line of chat %v, want only lines of %v", name, part, line["toolSourceId"], c.want[i][0])
				}
			}
			all = append(all, sourceIDs(lines)...)
		}
		// Every line of a part comes after every line of the part before it.
		if !slices.IsSorted(all) || len(all) != c.chats[0]+c.chats[1]+c.chats[2] {
			t.Errorf("%s: the parts hold %v", name, all)
		}
	}
}

func TestOnlyABundleWithOneChatOfNoMoreThanAPartCarriesTheOneFile(t *testing.T) {
	const perPart = 4
	for name, c := range map[string]struct {
		lines int
		files []string
	}{
		"fewer lines than a part holds": {3, []string{"messages.jsonl"}},
		"exactly what a part holds":     {4, []string{"messages.jsonl"}},
		"one line more":                 {5, []string{"messages-0001.jsonl", "messages-0002.jsonl"}},
		"no line at all":                {0, nil},
	} {
		account := fakebasecamp.New()
		campfire(account).Lines, side(account).Lines = nil, nil
		quiet(account).Lines = chatOf(account, 30000, c.lines)
		dir, _ := extractInParts(t, account, perPart, talkProject)

		// partsOf holds what lies in the bundle against its manifest.
		parts := partsOf(t, dir)
		lying, _ := filepath.Glob(filepath.Join(dir, "messages*"))
		for i := range lying {
			lying[i] = filepath.Base(lying[i])
		}
		if !slices.Equal(lying, c.files) || (len(c.files) > 1) != (parts != nil) {
			t.Errorf("%s: the bundle holds %v and names the parts %v, want %v", name, lying, parts, c.files)
		}
		if got := len(messages(t, dir)); got != c.lines {
			t.Errorf("%s: %d lines were written", name, got)
		}
	}
}

func TestWithoutANumberAPartHoldsTenThousandLines(t *testing.T) {
	account := fakebasecamp.New()
	account.PageSize = 1000
	campfire(account).Lines, side(account).Lines = nil, nil
	quiet(account).Lines = chatOf(account, 100000, 10001)
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
	writer := bundle.Discard()
	if _, err := extractor.Extract(t.Context(), []string{talkProject}, writer); err != nil {
		t.Fatal(err)
	}
	if got, want := writer.Parts()[bundle.Messages], []string{"messages-0001.jsonl", "messages-0002.jsonl"}; !slices.Equal(got, want) {
		t.Errorf("10,001 lines come in the parts %v, want %v", got, want)
	}

	quiet(account).Lines = quiet(account).Lines[:10000]
	writer = bundle.Discard()
	if _, err := extractor.Extract(t.Context(), []string{talkProject}, writer); err != nil {
		t.Fatal(err)
	}
	if got := writer.Parts(); got != nil || writer.Counts()[bundle.Messages] != 10000 {
		t.Errorf("10,000 lines come in the parts %v", got)
	}
}

// The list of an account holds every kind of person for an admin and users
// alone for anybody else: the people of a bundle are the same under both.
func TestABotAndSomebodyDeletedAreRetiredWhicheverListTheAccountShows(t *testing.T) {
	var first string
	for view, change := range map[string]func(*fakebasecamp.Account){
		"what an admin sees":     func(*fakebasecamp.Account) {},
		"what anybody else sees": func(a *fakebasecamp.Account) { a.UsersOnly() },
	} {
		account := fakebasecamp.New()
		change(account)
		dir, result := extract(t, account, talkProject)
		people := records(t, dir, bundle.People)

		// A bot has no address; of somebody deleted Basecamp keeps theirs.
		want := map[string]any{"sourceId": "801", "createdAt": "2021-06-01T08:00:00.000Z", "updatedAt": "2021-06-01T08:00:00.000Z", "createdBy": nil,
			"name": "Build Robot", "email": nil, "avatar": "avatar-801", "retired": true}
		if got := recordWithID(t, people, "801"); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the bot:\n got %v\nwant %v", view, got, want)
		}
		want = map[string]any{"sourceId": "802", "createdAt": "2019-06-01T08:00:00.000Z", "updatedAt": "2022-06-01T08:00:00.000Z", "createdBy": nil,
			"name": "Deleted Somebody", "email": "deleted@example.test", "avatar": "avatar-802", "retired": true}
		if got := recordWithID(t, people, "802"); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: somebody deleted:\n got %v\nwant %v", view, got, want)
		}
		for _, person := range people {
			user := !slices.Contains([]any{"99", "801", "802"}, person["sourceId"])
			if person["retired"] == user {
				t.Errorf("%s: person %v comes with retired %v", view, person["sourceId"], person["retired"])
			}
		}
		if got := asked(account, "people/80"); len(got) != 0 {
			t.Errorf("%s: somebody who cannot be asked for was asked for: %v", view, got)
		}
		// Whoever is in the list and in no record is left out, of any kind.
		if result.PeopleLeftOut != 1 {
			t.Errorf("%s: %d people left out, want the one user no record names", view, result.PeopleLeftOut)
		}

		content, err := os.ReadFile(filepath.Join(dir, "people.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = string(content)
		}
		if string(content) != first {
			t.Errorf("%s: the people differ from those of the other view:\n%s\n%s", view, content, first)
		}
	}
}

// A bot and somebody deleted who are in the list and wrote nothing are people
// of the account nothing refers to.
func TestABotNoRecordNamesStaysOutOfTheBundle(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), boardProject)
	for _, id := range []string{"801", "802"} {
		if slices.Contains(sourceIDs(records(t, dir, bundle.People)), id) {
			t.Errorf("person %s came into a bundle no record of which names them", id)
		}
	}
	if result.PeopleLeftOut != 2 {
		t.Errorf("%d people left out, want the bot and who was deleted", result.PeopleLeftOut)
	}
}

func TestTheAddressOfSomebodyDeletedIsHeldToWhatEveryAddressIs(t *testing.T) {
	account := fakebasecamp.New()
	account.UsersOnly()
	campfire(account).Lines[4].Creator.Email = "no address at all"
	dir, result := extract(t, account, talkProject)
	if email := recordWithID(t, records(t, dir, bundle.People), "802")["email"]; email != nil {
		t.Errorf("an address that is none came as %v", email)
	}
	if !slices.Equal(result.InvalidAddresses, []string{"802"}) {
		t.Errorf("invalid addresses %v, want the ID of who was deleted", result.InvalidAddresses)
	}

	account = fakebasecamp.New()
	account.UsersOnly()
	campfire(account).Lines[4].Creator.Email = maskedAddress
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
	_, err := extractor.Extract(t.Context(), []string{talkProject}, bundle.Discard())
	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.Forbidden {
		t.Errorf("a masked address of somebody deleted: got %v, want the run to end as forbidden", err)
	}
}

func TestEveryPersonARecordOfATalkNamesIsInTheBundle(t *testing.T) {
	// Linus and Edsger are no members and come through a boost alone, the former
	// colleague through a comment, the bot and who was deleted through a line.
	people := everyReferenceLeadsToAPerson(t, first(extract(t, fakebasecamp.New(), talkProject)))
	for _, id := range []string{"99", "101", "102", "103", "104", "801", "802"} {
		if !people[id] {
			t.Errorf("person %s is not in the bundle", id)
		}
	}

	// Somebody no list knows, in each place a record can name a person.
	for place, put := range map[string]func(*fakebasecamp.Account, *fakebasecamp.Person){
		"the creator of a post":              func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyPost(a).Creator = p },
		"the creator of an archived post":    func(a *fakebasecamp.Account, p *fakebasecamp.Person) { news(a).Posts[2].Creator = p },
		"the creator of a comment on a post": func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyPost(a).Comments[0].Creator = p },
		"who boosted a post":                 func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyPost(a).Boosts[0].Booster = p },
		"who boosted a comment on a post":    func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyPost(a).Comments[1].Boosts[0].Booster = p },
		"the writer of a line":               func(a *fakebasecamp.Account, p *fakebasecamp.Person) { campfire(a).Lines[0].Creator = p },
		"who dropped a file":                 func(a *fakebasecamp.Account, p *fakebasecamp.Person) { campfire(a).Lines[2].Creator = p },
		"who boosted a line":                 func(a *fakebasecamp.Account, p *fakebasecamp.Person) { campfire(a).Lines[5].Boosts[0].Booster = p },
	} {
		account := fakebasecamp.New()
		put(account, stranger())
		dir, _ := extractInParts(t, account, 3, talkProject)
		if people := everyReferenceLeadsToAPerson(t, dir); !people["555"] {
			t.Errorf("%s did not come into the bundle", place)
		}
		named := false
		for name, content := range talkFiles(t, dir) {
			named = named || name != "people.jsonl" && strings.Contains(content, `"555"`)
		}
		if !named {
			t.Errorf("%s: no record names the stranger", place)
		}
	}

	// Somebody the account lists and no project knows, mentioned in a text.
	for place, put := range map[string]func(*fakebasecamp.Account, string){
		"a post":              func(a *fakebasecamp.Account, mention string) { busyPost(a).Content += mention },
		"a comment on a post": func(a *fakebasecamp.Account, mention string) { busyPost(a).Comments[0].Content += mention },
		"a line":              func(a *fakebasecamp.Account, mention string) { campfire(a).Lines[1].Content += mention },
		"the line of a bot":   func(a *fakebasecamp.Account, mention string) { campfire(a).Lines[3].Content += mention },
	} {
		account := fakebasecamp.New()
		put(account, fakebasecamp.Mention(fakebasecamp.SGID("gid://bc3/Person/105?expires_in"), "", 105, "Niklaus Nowhere"))
		dir, _ := extract(t, account, talkProject)
		if people := everyReferenceLeadsToAPerson(t, dir); !people["105"] {
			t.Errorf("who is mentioned in %s did not come into the bundle", place)
		}
	}
}

// talkTexts is text of the fake that no message may carry.
var talkTexts = []string{"News", "Pitches", "Campfire", "Office move", "Reminder", "tea club", "Announcement", "faq", "Count me in", "Good morning",
	"Build 42", "Goodbye", "Whatever", "minutes.pdf", "route.png", "Grace", "Ada", "Robot", "Deleted Somebody", "example.test"}

func TestWhatABundleCannotSayAboutATalkEndsTheRunWithoutNamingAnything(t *testing.T) {
	for what, change := range map[string]func(*fakebasecamp.Account){
		"a line without createdAt":           func(a *fakebasecamp.Account) { campfire(a).Lines[0].CreatedAt = "" },
		"a line without updatedAt":           func(a *fakebasecamp.Account) { campfire(a).Lines[0].UpdatedAt = "" },
		"a line written at no time":          func(a *fakebasecamp.Account) { campfire(a).Lines[0].CreatedAt = "Good morning" },
		"a boost of a line, of nobody":       func(a *fakebasecamp.Account) { campfire(a).Lines[5].Boosts[0].Booster = nil },
		"a post without a time":              func(a *fakebasecamp.Account) { busyPost(a).UpdatedAt = "" },
		"a comment on a post without a time": func(a *fakebasecamp.Account) { busyPost(a).Comments[0].CreatedAt = "" },
		"a boost of a post, of nobody":       func(a *fakebasecamp.Account) { busyPost(a).Boosts[0].Booster = nil },
		"a category without a time":          func(a *fakebasecamp.Account) { a.Projects[3].Categories[2].CreatedAt = "" },
		"lines that cannot be read":          func(a *fakebasecamp.Account) { a.Fail["buckets/7004/chats/9033/lines.json"] = failure },
		"messages that cannot be read":       func(a *fakebasecamp.Account) { a.Fail["buckets/7004/message_boards/9032/messages.json"] = failure },
		"categories that cannot be read":     func(a *fakebasecamp.Account) { a.Fail["buckets/7004/categories.json"] = failure },
		"an archive of posts that fails": func(a *fakebasecamp.Account) {
			a.Fail["projects/recordings.json?type=Message&bucket=7004&status=trashed"] = failure
		},
		"comments on a post that fail": func(a *fakebasecamp.Account) { a.Fail["buckets/7004/recordings/9701/comments.json"] = failure },
		"boosts of a line that fail":   func(a *fakebasecamp.Account) { a.Fail["buckets/7004/recordings/9806/boosts.json"] = failure },
		"a bot with a masked address": func(a *fakebasecamp.Account) {
			a.UsersOnly()
			campfire(a).Lines[3].Creator.Kind, campfire(a).Lines[3].Creator.Email = "Tombstone", maskedAddress
		},
		"somebody masked as the writer of post": func(a *fakebasecamp.Account) { news(a).Posts[0].Comments[0].Creator.Email = maskedAddress },
	} {
		account := fakebasecamp.New()
		change(account)
		extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
		_, err := extractor.Extract(t.Context(), []string{talkProject}, bundle.Discard())
		var failed *basecamp.Error
		if !errors.As(err, &failed) {
			t.Errorf("%s: got %v, want the run to end", what, err)
			continue
		}
		for _, text := range talkTexts {
			if strings.Contains(failed.Message+failed.Hint, text) {
				t.Errorf("%s: the message names %q: %s", what, text, failed.Message)
			}
		}
	}
}

// A chat is held to what a bundle can carry before its first line is written
// or a boost is asked for.
func TestALineThatCannotBeCarriedEndsTheRunBeforeItsChatIsWritten(t *testing.T) {
	for what, c := range map[string]struct {
		change  func(*fakebasecamp.Account)
		message string
	}{
		"a line without a time":     {func(a *fakebasecamp.Account) { campfire(a).Lines[6].UpdatedAt = "" }, "when line 9807 was updated"},
		"a line written at no time": {func(a *fakebasecamp.Account) { campfire(a).Lines[6].CreatedAt = "at dawn" }, "line 9807 was created at something that is no time"},
	} {
		account := fakebasecamp.New()
		c.change(account)
		writer := bundle.Discard()
		extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
		_, err := extractor.Extract(t.Context(), []string{talkProject}, writer)
		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != basecamp.API || !strings.HasSuffix(failed.Message, c.message) {
			t.Errorf("%s: got %v, want an error of the API that ends in %q", what, err, c.message)
		}
		if writer.Counts()[bundle.Messages] != 0 || len(asked(account, "recordings/9806/boosts.json")) != 0 {
			t.Errorf("%s: %d lines were written and %v asked for", what, writer.Counts()[bundle.Messages], asked(account, "recordings/9806/boosts.json"))
		}
	}
}

func TestTheLongestTextOfAKindIsMeasuredInCharacters(t *testing.T) {
	account := fakebasecamp.New()
	busyCard(account).Comments[0].Content = strings.Repeat("ä", 500)
	busyPost(account).Comments[0].Content = strings.Repeat("ö", 400)
	pitches(account).Posts[0].Content = strings.Repeat("ü", 300)
	side(account).Lines[0].Content = strings.Repeat("ß", 250)
	doing(account).Cards[1].Content = strings.Repeat("€", 600)
	_, result := extract(t, account, boardProject, talkProject)

	want := map[bundle.Kind]int{bundle.Cards: 600, bundle.Comments: 500, bundle.Posts: 300, bundle.Messages: 250, bundle.Answers: 0}
	if !reflect.DeepEqual(result.LongestBody, want) {
		t.Errorf("the longest texts are %v, want %v", result.LongestBody, want)
	}
}

func TestEveryRecordOfATalkNamesRecordsOfTheBundle(t *testing.T) {
	dir, _ := extractInParts(t, fakebasecamp.New(), 3, talkProject)

	tools := map[string]string{}
	for _, tool := range records(t, dir, bundle.Tools) {
		tools[tool["sourceId"].(string)] = tool["kind"].(string)
	}
	parents := map[string]bool{}
	for _, post := range records(t, dir, bundle.Posts) {
		parents["post "+post["sourceId"].(string)] = true
		if tools[post["toolSourceId"].(string)] != "posts" || post["projectSourceId"] != "7004" {
			t.Errorf("post %v names the tool %v of project %v", post["sourceId"], post["toolSourceId"], post["projectSourceId"])
		}
	}
	for _, line := range messages(t, dir) {
		parents["message "+line["sourceId"].(string)] = true
		if tools[line["toolSourceId"].(string)] != "chat" || line["projectSourceId"] != "7004" {
			t.Errorf("message %v names the tool %v of project %v", line["sourceId"], line["toolSourceId"], line["projectSourceId"])
		}
	}
	for _, comment := range records(t, dir, bundle.Comments) {
		parents["comment "+comment["sourceId"].(string)] = true
		if !parents[comment["parentKind"].(string)+" "+comment["parentSourceId"].(string)] {
			t.Errorf("comment %v names a %v that is not in the bundle before it", comment["sourceId"], comment["parentKind"])
		}
	}
	for _, reaction := range records(t, dir, bundle.Reactions) {
		if !parents[reaction["targetKind"].(string)+" "+reaction["targetSourceId"].(string)] {
			t.Errorf("reaction %v names a %v that is not in the bundle", reaction["sourceId"], reaction["targetKind"])
		}
	}
}

func TestThePartsOfABundleAreForTheirOwnerOnly(t *testing.T) {
	dir, _ := extractInParts(t, fakebasecamp.New(), 3, talkProject)
	for _, name := range append([]string{"categories.jsonl", "posts.jsonl"}, partsOf(t, dir)...) {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s has the mode %v, want 0600", name, info.Mode().Perm())
		}
	}
}

func TestABotThatDoesNotSayWhenItWasMadeEndsTheRun(t *testing.T) {
	for what, c := range map[string]struct {
		change  func(*fakebasecamp.Person)
		message string
	}{
		"neither time": {func(p *fakebasecamp.Person) { p.CreatedAt, p.UpdatedAt = "", "" }, "Basecamp does not say when person 801 was created and updated"},
		"no updatedAt": {func(p *fakebasecamp.Person) { p.UpdatedAt = "" }, "Basecamp does not say when person 801 was updated"},
	} {
		account := fakebasecamp.New()
		account.UsersOnly()
		c.change(campfire(account).Lines[3].Creator)
		dir := filepath.Join(t.TempDir(), "bundle")
		writer, err := bundle.Create(dir, basecamp.SourceName)
		if err != nil {
			t.Fatal(err)
		}
		extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
		_, err = extractor.Extract(t.Context(), []string{talkProject}, writer)
		writer.Abort()

		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != basecamp.API || failed.Message != c.message {
			t.Errorf("%s: got %v, want the error %q", what, err, c.message)
		}
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("%s: a bundle was written with somebody in it who has no times", what)
		}
	}
}

// chatAt makes a plain line for each time, numbered from an ID on.
func chatAt(account *fakebasecamp.Account, from int64, times ...string) []fakebasecamp.Line {
	lines := make([]fakebasecamp.Line, len(times))
	for i, at := range times {
		lines[i] = fakebasecamp.Line{ID: from + int64(i), Type: fakebasecamp.TextLine, Creator: &account.People[1], CreatedAt: at, UpdatedAt: at, Content: "A line"}
	}
	return lines
}

// The applier keeps the lines of one second in the order of their file by
// counting them, anew in every part: so no second is cut in two.
func TestAPartEndsOnlyWhereTheSecondChanges(t *testing.T) {
	const perPart = 4
	at := func(second string) string { return "2023-05-01T10:00:" + second + "Z" }
	for name, c := range map[string]struct {
		times []string
		want  []int
	}{
		"a second a line to itself": {[]string{at("01"), at("02"), at("03"), at("04"), at("05"), at("06")}, []int{4, 2}},
		"a second across the size":  {[]string{at("01"), at("02"), at("03"), at("04"), at("04"), at("04"), at("05")}, []int{6, 1}},
		"the first change after it": {[]string{at("01"), at("02"), at("02"), at("02"), at("02"), at("03"), at("04"), at("04")}, []int{5, 3}},
		"a change before the size":  {[]string{at("01"), at("01"), at("02"), at("02"), at("02"), at("03")}, []int{5, 1}},
		"fractions of one second":   {[]string{at("01"), at("02"), at("03"), at("04.100"), at("04.500"), at("04.900"), at("05.000")}, []int{6, 1}},
		// One second, written in two zones, and two seconds that read alike.
		"a zone that hides a second": {[]string{at("01"), at("02"), at("03"), at("04.100"), "2023-05-01T12:00:04.900+02:00", at("05")}, []int{5, 1}},
		"a zone that fakes a second": {[]string{"2023-05-01T07:00:01Z", "2023-05-01T07:00:02Z", "2023-05-01T07:00:03Z", "2023-05-01T10:00:00.000+02:00", "2023-05-01T10:00:00.000Z"}, []int{4, 1}},
		"one second to the end":      {[]string{at("01"), at("02"), at("03"), at("03"), at("03"), at("03"), at("03"), at("03"), at("03"), at("03"), at("03")}, []int{11}},
	} {
		account := fakebasecamp.New()
		campfire(account).Lines = chatAt(account, 20000, c.times...)
		quiet(account).Lines = nil
		side(account).Lines = chatAt(account, 40000, at("30"))
		dir, _ := extractInParts(t, account, perPart, talkProject)

		var got []int
		for _, part := range partsOf(t, dir) {
			got = append(got, len(recordsIn(t, filepath.Join(dir, part))))
		}
		if want := append(c.want, 1); !slices.Equal(got, want) {
			t.Errorf("%s: the parts hold %v lines, want %v", name, got, want)
		}
		if lines := messages(t, dir); len(lines) != len(c.times)+1 || !slices.IsSorted(sourceIDs(lines)) {
			t.Errorf("%s: the lines are %v", name, sourceIDs(lines))
		}
	}
}

func TestALineThatSaysNothingIsLeftOutAndCounted(t *testing.T) {
	account := fakebasecamp.New()
	lines := campfire(account).Lines
	lines[0].Content = ""
	lines[3].Content = " \n&nbsp;<div><br></div><p> </p><BR/>"
	// Nobody but these two lines names the bot and this person, and one of
	// them has boosts.
	lines[0].Creator = stranger()
	lines[0].Boosts = lines[5].Boosts
	dir, result := extract(t, account, talkProject)

	if result.MessagesWithEmptyBody != 2 {
		t.Errorf("%d lines that say nothing, want 2", result.MessagesWithEmptyBody)
	}
	if got, want := sourceIDs(messages(t, dir)), []string{"9802", "9803", "9805", "9806", "9807", "9811", "9812"}; !slices.Equal(got, want) {
		t.Errorf("messages %v, want %v", got, want)
	}
	if want := map[string]int{"text": 3, "richText": 2, "upload": 2, "integration": 0}; !reflect.DeepEqual(result.MessagesByType, want) {
		t.Errorf("messages by type %v, want %v", result.MessagesByType, want)
	}
	for _, id := range []string{"555", "801"} {
		if slices.Contains(sourceIDs(records(t, dir, bundle.People)), id) {
			t.Errorf("person %s came into the bundle for a line that is left out", id)
		}
	}
	if got := asked(account, "recordings/9801/boosts.json"); len(got) != 0 {
		t.Errorf("the boosts of a line that is left out were asked for: %v", got)
	}
}

// What a reader sees nothing of is nothing; a reference, a link, a rule and an
// embed are something, and so is a line however long. A frame that is left out
// leaves nothing.
func TestOnlyWhitespaceBreaksAndEmptyBlocksSayNothing(t *testing.T) {
	long := strings.Repeat("a", 10001)
	for content, written := range map[string]bool{
		"": false, " \t\n": false, "\u200b\ufeff": false, "<br>": false, "<div></div>": false, "<div><br/></div><p>&nbsp;</p>": false, "<!-- nothing -->": false,
		".": true, "<div>&amp;</div>": true, "<hr>": true, `<a href="https://example.test/"></a>`: true, "<span></span>": true,
		`<figure><iframe src="https://video.example.test/embed/1"></iframe></figure>`:                          true,
		"<figure>\n<iframe src=\"//video.example.test/embed/1\"></iframe><figcaption> </figcaption></figure>":  false,
		`<figure><iframe src="//video.example.test/embed/1"></iframe><figcaption>A film</figcaption></figure>`: true,
		fakebasecamp.LinkPreview("https://example.test/", ""):                                                  true,
		fakebasecamp.Mention(fakebasecamp.SGID("gid://bc3/Person/102"), "", 102, "Grace Sample"):               true,
		`<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"><hr></bc-attachment>`:     true,
		long: true,
	} {
		account := fakebasecamp.New()
		campfire(account).Lines = chatAt(account, 20000, "2023-05-01T10:00:00Z")
		campfire(account).Lines[0].Type, campfire(account).Lines[0].Content = fakebasecamp.RichTextLine, content
		side(account).Lines = nil
		dir, result := extract(t, account, talkProject)

		lines := messages(t, dir)
		if written != (len(lines) == 1) || written == (result.MessagesWithEmptyBody == 1) {
			t.Errorf("%.40q: %d lines were written and %d counted as saying nothing", content, len(lines), result.MessagesWithEmptyBody)
		}
		if strings.HasPrefix(content, "<figure>") {
			leftOut := strings.Count(content, `"//`)
			if result.Bodies.Embeds != 1-leftOut || result.Bodies.EmbedsLeftOut != leftOut {
				t.Errorf("%.40q: counted %+v, want %d left out", content, result.Bodies, leftOut)
			}
		}
		if strings.Contains(content, `"https://video`) && lines[0]["body"] != `<import-embed url="https://video.example.test/embed/1"></import-embed>` {
			t.Errorf("a line that was a video alone became %q", lines[0]["body"])
		}
		if content == long && (lines[0]["body"] != long || result.LongestBody[bundle.Messages] != 10001) {
			t.Errorf("a long line was changed, or measured as %d", result.LongestBody[bundle.Messages])
		}
	}
}
