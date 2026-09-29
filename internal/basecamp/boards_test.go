package basecamp_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// The boards of the fake are in its third project: the first board of the
// dock holds every case, the second is switched off, the third is small.
const boardProject = "7003"

func roadmap(a *fakebasecamp.Account) *fakebasecamp.Tool { return &a.Projects[2].Dock[0] }

// doing is the column of the first board that has cards set aside.
func doing(a *fakebasecamp.Account) *fakebasecamp.List { return &roadmap(a).Lists[2] }

// busyCard is the card that has steps, comments, boosts and a note.
func busyCard(a *fakebasecamp.Account) *fakebasecamp.Card { return &doing(a).Cards[0] }

func sourceIDs(records []map[string]any) []string {
	ids := make([]string, len(records))
	for i, record := range records {
		ids[i] = record["sourceId"].(string)
	}
	return ids
}

func recordWithID(t *testing.T, records []map[string]any, id string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["sourceId"] == id {
			return record
		}
	}
	t.Fatalf("no record %s among %v", id, sourceIDs(records))
	return nil
}

func asked(account *fakebasecamp.Account, part string) []string {
	var requests []string
	for _, request := range account.Requests {
		if strings.Contains(request, part) {
			requests = append(requests, request)
		}
	}
	return requests
}

func TestTheColumnsOfABoardComeInItsOrderWithARunningPosition(t *testing.T) {
	account := fakebasecamp.New()
	slices.Reverse(roadmap(account).Lists)
	dir, _ := extract(t, account, boardProject)

	type column struct {
		id, tool, name, role string
		position             float64
	}
	var got []column
	for _, record := range records(t, dir, bundle.Columns) {
		got = append(got, column{record["sourceId"].(string), record["toolSourceId"].(string), record["name"].(string), record["role"].(string), record["position"].(float64)})
	}
	want := []column{
		{"9103", "9020", "Inbox", "triage", 1},
		{"9105", "9020", "Not now", "notnow", 2},
		{"9104", "9020", "Doing", "work", 3},
		{"9101", "9020", "Review", "work", 4},
		{"9102", "9020", "Done", "done", 5},
		{"9131", "9022", "Triage", "triage", 1},
		{"9132", "9022", "Not now", "notnow", 2},
		{"9133", "9022", "Done", "done", 3},
	}
	if !slices.Equal(got, want) {
		t.Errorf("columns:\n got %v\nwant %v", got, want)
	}

	whole := map[string]any{
		"sourceId": "9104", "createdAt": "2023-03-01T10:00:00.000Z", "updatedAt": "2023-03-01T11:00:00.000Z", "createdBy": "102",
		"projectSourceId": "7003", "toolSourceId": "9020", "name": "Doing", "position": float64(3), "role": "work",
	}
	if got := recordWithID(t, records(t, dir, bundle.Columns), "9104"); !reflect.DeepEqual(got, whole) {
		t.Errorf("a column:\n got %v\nwant %v", got, whole)
	}
}

func TestACardComesWithItsColumnItsPeopleAndItsNote(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), boardProject)
	cards := records(t, dir, bundle.Cards)

	busy := map[string]any{
		"sourceId": "9201", "createdAt": "2023-03-05T08:00:00.000Z", "updatedAt": "2023-03-06T08:00:00.000Z", "createdBy": "101",
		"projectSourceId": "7003", "toolSourceId": "9020", "columnSourceId": "9104", "title": "Draw the map",
		"description": `<div>Ask <import-mention ref="102">Grace Sample</import-mention> about tea &amp; biscuits.</div><hr>` +
			`<div><import-attachment ref="attachment-5001"></import-attachment></div>`,
		"position": float64(1), "dueOn": "2023-04-01", "completedAt": nil, "completedBy": nil,
		"assignees": []any{"101", "102"}, "status": "active",
		// In the order of their IDs and each once, and nobody who is no member.
		"subscribers": []any{"101", "102"},
	}
	if got := recordWithID(t, cards, "9201"); !reflect.DeepEqual(got, busy) {
		t.Errorf("a card with everything:\n got %v\nwant %v", got, busy)
	}

	completed := map[string]any{
		"sourceId": "9206", "createdAt": "2023-03-01T14:00:00.000Z", "updatedAt": "2023-03-03T14:00:00.000Z", "createdBy": "101",
		"projectSourceId": "7003", "toolSourceId": "9020", "columnSourceId": "9102", "title": "Buy paper",
		"description": nil, "position": float64(1), "dueOn": nil,
		"completedAt": "2023-03-03T14:00:00.000Z", "completedBy": "104", "assignees": []any{}, "status": "active",
		// Nobody follows it at the source, which is said and not left out.
		"subscribers": []any{},
	}
	if got := recordWithID(t, cards, "9206"); !reflect.DeepEqual(got, completed) {
		t.Errorf("a completed card:\n got %v\nwant %v", got, completed)
	}
}

func TestANoteOfNothingButWhitespaceIsStillANoteAndAnEmptyOneIsNone(t *testing.T) {
	account := fakebasecamp.New()
	doing(account).Cards[1].Content = "<div><br></div>"
	dir, _ := extract(t, account, boardProject)
	cards := records(t, dir, bundle.Cards)

	if got := recordWithID(t, cards, "9202")["description"]; got != "<div><br></div>" {
		t.Errorf("description %v, want the note as it is", got)
	}
	if got, has := recordWithID(t, cards, "9204")["description"]; got != nil || !has {
		t.Errorf("description %v (there: %v), want null", got, has)
	}
}

func TestTheCardsOfABoardAreWrittenOldestFirstAndBoardsInTheOrderOfTheTools(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), boardProject)
	// 9205 and 9206 were created in the same moment.
	want := []string{"9205", "9206", "9204", "9207", "9202", "9203", "9208", "9201", "9210", "9211"}
	if got := sourceIDs(records(t, dir, bundle.Cards)); !slices.Equal(got, want) {
		t.Errorf("cards in the order\n got %v\nwant %v", got, want)
	}
}

func TestPositionsCountThroughAColumnWithoutAGap(t *testing.T) {
	account := fakebasecamp.New()
	// Basecamp's own positions decide, not the order it lists cards in.
	slices.Reverse(doing(account).Cards)
	dir, result := extract(t, account, boardProject)

	columns := map[string][]string{}
	for _, card := range records(t, dir, bundle.Cards) {
		column := card["columnSourceId"].(string)
		position := int(card["position"].(float64))
		for len(columns[column]) < position {
			columns[column] = append(columns[column], "")
		}
		if columns[column][position-1] != "" {
			t.Errorf("column %s has two cards at position %d", column, position)
		}
		columns[column][position-1] = card["sourceId"].(string)
	}
	want := map[string][]string{
		"9103": {"9204"},
		// By their positions 1 and 3, then the one set aside, the archived one,
		// the trashed one.
		"9104": {"9201", "9202", "9203", "9207", "9208"},
		// Done has no positions: as Basecamp lists them.
		"9102": {"9206", "9205"},
		"9131": {"9211", "9210"},
	}
	if !reflect.DeepEqual(columns, want) {
		t.Errorf("the columns hold\n got %v\nwant %v", columns, want)
	}
	if result.CardsOnHold != 2 {
		t.Errorf("%d cards on hold, want 2", result.CardsOnHold)
	}
}

func TestACardThatWasSetAsideBelongsToItsColumnAndSaysSo(t *testing.T) {
	var cardsOfTheFirstRun string
	// Wherever Basecamp lists such a card: in the list of the cards set aside,
	// among the cards of the column as well, or only there.
	for _, listed := range []struct{ inColumn, notInItsOwnList bool }{{false, false}, {true, false}, {true, true}} {
		account := fakebasecamp.New()
		account.OnHoldInColumn, account.OnHoldListEmpty = listed.inColumn, listed.notInItsOwnList
		dir, result := extract(t, account, boardProject)

		var onHold []string
		for _, card := range records(t, dir, bundle.Cards) {
			flag, has := card["onHold"]
			if has && flag != true {
				t.Errorf("card %v carries onHold %v: the key is left out unless true", card["sourceId"], flag)
			}
			if has {
				onHold = append(onHold, card["sourceId"].(string))
				if card["columnSourceId"] != "9104" {
					t.Errorf("card %v is in column %v, want the column that owns the cards set aside", card["sourceId"], card["columnSourceId"])
				}
			}
		}
		if !slices.Equal(onHold, []string{"9203", "9208"}) || result.CardsOnHold != 2 {
			t.Errorf("%+v: on hold %v, counted %d", listed, onHold, result.CardsOnHold)
		}
		if got := len(records(t, dir, bundle.Cards)); got != 10 {
			t.Errorf("%+v: %d cards, want each of the 10 once", listed, got)
		}
		// It stands behind the other cards of the column in every case.
		written := boardFiles(t, dir)["cards.jsonl"]
		if cardsOfTheFirstRun != "" && written != cardsOfTheFirstRun {
			t.Errorf("%+v: where a card on hold is listed changes the bundle:\n%s\n%s", listed, cardsOfTheFirstRun, written)
		}
		cardsOfTheFirstRun = written
	}
}

func TestArchivedAndTrashedCardsComeWithTheBoardTheirColumnIsOf(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, boardProject)
	cards := records(t, dir, bundle.Cards)

	for id, want := range map[string][3]string{
		"9207": {"9020", "9104", "archived"},
		"9208": {"9020", "9104", "trashed"},
		"9210": {"9022", "9131", "archived"},
	} {
		card := recordWithID(t, cards, id)
		if got := [3]string{card["toolSourceId"].(string), card["columnSourceId"].(string), card["status"].(string)}; got != want {
			t.Errorf("card %s: %v, want %v", id, got, want)
		}
	}
	// The card of the board that is switched off belongs to no board here.
	if slices.Contains(sourceIDs(cards), "9209") || result.CardsLeftOut != 1 {
		t.Errorf("cards %v, %d left out", sourceIDs(cards), result.CardsLeftOut)
	}
	wantStatuses := map[string]int{"active": 7, "archived": 2, "trashed": 1}
	if !reflect.DeepEqual(result.CardsByStatus, wantStatuses) {
		t.Errorf("cards by status %v, want %v", result.CardsByStatus, wantStatuses)
	}
	if len(asked(account, "status=archived")) == 0 || len(asked(account, "status=trashed")) == 0 {
		t.Errorf("asked for %v", asked(account, "recordings.json"))
	}

	// A project without a board costs no such request.
	account = fakebasecamp.New()
	_, result = extract(t, account, "7002")
	if requests := asked(account, "recordings.json"); len(requests) != 0 {
		t.Errorf("a project without a board was asked for %v", requests)
	}
	if !reflect.DeepEqual(result.CardsByStatus, map[string]int{"active": 0, "archived": 0, "trashed": 0}) {
		t.Errorf("cards by status %v, want every status with 0", result.CardsByStatus)
	}
}

func TestStepsComeInTheOrderOfTheirPositionsWithoutAssigneesOrDueDates(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, boardProject)

	want := []map[string]any{
		{"sourceId": "9401", "createdAt": "2023-03-05T09:00:00.000Z", "updatedAt": "2023-03-07T09:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7003", "toolSourceId": "9020", "cardSourceId": "9201", "title": "Outline the coast", "position": float64(1),
			"completedAt": "2023-03-07T09:00:00.000Z", "completedBy": "102"},
		{"sourceId": "9402", "createdAt": "2023-03-05T10:00:00.000Z", "updatedAt": "2023-03-05T10:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7003", "toolSourceId": "9020", "cardSourceId": "9201", "title": "Colour the sea", "position": float64(2),
			"completedAt": nil, "completedBy": nil},
		{"sourceId": "9403", "createdAt": "2023-03-05T11:00:00.000Z", "updatedAt": "2023-03-05T11:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7003", "toolSourceId": "9020", "cardSourceId": "9201", "title": "Name the towns", "position": float64(3),
			"completedAt": nil, "completedBy": nil},
	}
	if got := records(t, dir, bundle.Steps); !reflect.DeepEqual(got, want) {
		t.Errorf("steps:\n got %v\nwant %v", got, want)
	}
	if requests := asked(account, "subtasks.json"); len(requests) != 0 {
		t.Errorf("the card shows all its steps, and they were asked for all the same: %v", requests)
	}
}

func TestStepsACardDoesNotShowAreAskedFor(t *testing.T) {
	whole, _ := extract(t, fakebasecamp.New(), boardProject)

	account := fakebasecamp.New()
	account.InlineSteps = 1
	dir, _ := extract(t, account, boardProject)
	if got, want := records(t, dir, bundle.Steps), records(t, whole, bundle.Steps); !reflect.DeepEqual(got, want) {
		t.Errorf("steps:\n got %v\nwant %v", got, want)
	}
	if requests := asked(account, "recordings/9201/subtasks.json"); len(requests) != 3 {
		t.Errorf("asked for %v, want the two pages and the empty one", requests)
	}
}

func TestTheStepsOfACardAreNotFetchedFromOutsideTheAccount(t *testing.T) {
	account := fakebasecamp.New()
	account.InlineSteps = 1
	account.SubtasksURL[9201] = "https://elsewhere.example.test/4242/recordings/9201/subtasks.json"
	expectAPIError(t, account, "steps at another host", boardProject)
	for _, request := range account.Requests {
		if strings.Contains(request, "://") || strings.Contains(request, "subtasks") {
			t.Errorf("asked the Basecamp CLI for %q", request)
		}
	}
}

func TestCommentsComeOldestFirstAndOnlyWhereACardHasAny(t *testing.T) {
	account := fakebasecamp.New()
	slices.Reverse(busyCard(account).Comments)
	dir, result := extract(t, account, boardProject)

	want := []map[string]any{
		{"sourceId": "9301", "createdAt": "2023-03-05T12:00:00.000Z", "updatedAt": "2023-03-05T13:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7003", "parentKind": "card", "parentSourceId": "9201",
			"body": `<div>Fish &amp; chips for <import-mention ref="105">Niklaus Nowhere</import-mention>?</div>`},
		{"sourceId": "9302", "createdAt": "2023-03-05T14:00:00.000Z", "updatedAt": "2023-03-05T14:00:00.000Z", "createdBy": "99",
			"projectSourceId": "7003", "parentKind": "card", "parentSourceId": "9201", "body": `<div>Gone Person had one: lost.png</div>`},
		{"sourceId": "9303", "createdAt": "2023-03-05T16:00:00.000Z", "updatedAt": "2023-03-05T16:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7003", "parentKind": "card", "parentSourceId": "9201",
			// An embed of an earlier day, found by the key of its stored file.
			"body": `<div>Settled.<import-attachment ref="attachment-5003"></import-attachment></div>`},
	}
	if got := records(t, dir, bundle.Comments); !reflect.DeepEqual(got, want) {
		t.Errorf("comments:\n got %v\nwant %v", got, want)
	}
	if want := (basecamp.BodyCounts{MentionsUnresolved: 1, Files: 2, FilesDropped: 1}); result.Bodies != want {
		t.Errorf("bodies %+v, want %+v", result.Bodies, want)
	}

	wantRequests := []string{
		"buckets/7003/recordings/9201/comments.json?page=1",
		"buckets/7003/recordings/9201/comments.json?page=2",
		"buckets/7003/recordings/9201/comments.json?page=3",
	}
	if got := asked(account, "comments.json"); !slices.Equal(got, wantRequests) {
		t.Errorf("asked for %v, want the comments of the one card that has any", got)
	}
}

func TestBoostsBecomeReactionsToWhatTheyWereAskedOf(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, boardProject)

	want := []map[string]any{
		{"sourceId": "9503", "createdAt": "2023-03-06T08:00:00.000Z", "updatedAt": "2023-03-06T08:00:00.000Z", "createdBy": "103",
			"targetKind": "card", "targetSourceId": "9201", "content": "👍"},
		{"sourceId": "9501", "createdAt": "2023-03-05T14:00:00.000Z", "updatedAt": "2023-03-05T14:00:00.000Z", "createdBy": "102",
			"targetKind": "comment", "targetSourceId": "9301", "content": "🎉"},
		{"sourceId": "9502", "createdAt": "2023-03-05T15:00:00.000Z", "updatedAt": "2023-03-05T15:00:00.000Z", "createdBy": "101",
			"targetKind": "comment", "targetSourceId": "9301", "content": "Well done, all of you"},
	}
	if got := records(t, dir, bundle.Reactions); !reflect.DeepEqual(got, want) {
		t.Errorf("reactions:\n got %v\nwant %v", got, want)
	}

	var targets []string
	for _, request := range asked(account, "boosts.json") {
		if strings.HasSuffix(request, "page=1") {
			targets = append(targets, request)
		}
	}
	if !slices.Equal(targets, []string{"buckets/7003/recordings/9201/boosts.json?page=1", "buckets/7003/recordings/9301/boosts.json?page=1"}) {
		t.Errorf("asked for the boosts of %v, want only what has any", targets)
	}
}

func TestACardIsReadFromItsListAndNeverAskedForByItself(t *testing.T) {
	account := fakebasecamp.New()
	extract(t, account, boardProject)
	single := regexp.MustCompile(`card_tables/cards/|recordings/[0-9]+\.json`)
	for _, request := range account.Requests {
		if single.MatchString(request) {
			t.Errorf("asked for %s", request)
		}
	}
	// Each board is asked for once: what it says as a tool holds its lists.
	if requests := asked(account, "card_tables/9020.json"); len(requests) != 1 {
		t.Errorf("asked for the board %d times", len(requests))
	}
}

// boardFiles reads what a run over the boards wrote, apart from the people and
// their pictures.
func boardFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, name := range []string{"columns.jsonl", "cards.jsonl", "steps.jsonl", "comments.jsonl", "reactions.jsonl", "people.jsonl", "manifest.json"} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(content)
	}
	return files
}

func TestABundleDoesNotDependOnTheSizeOfAPage(t *testing.T) {
	var first map[string]string
	for _, size := range []int{1, 2, 100} {
		account := fakebasecamp.New()
		account.PageSize = size
		account.InlineSteps = 1
		dir, _ := extract(t, account, boardProject)

		files := boardFiles(t, dir)
		if first == nil {
			first = files
			// With one entry per page, every list there is has a second page.
			for _, list := range []string{
				"card_tables/lists/9104/cards.json", "card_tables/lists/9113/cards.json", "card_tables/lists/9102/cards.json",
				"status=archived", "status=trashed", "recordings/9201/comments.json", "recordings/9301/boosts.json",
				"recordings/9201/boosts.json", "recordings/9201/subtasks.json",
			} {
				if requests := asked(account, list); len(requests) < 2 {
					t.Errorf("%s was asked for %d times, want page after page until one is empty", list, len(requests))
				}
			}
			continue
		}
		if !reflect.DeepEqual(files, first) {
			t.Errorf("pages of %d give another bundle than pages of 1", size)
		}
	}
	if strings.Count(first["cards.jsonl"], "\n") != 10 || strings.Count(first["comments.jsonl"], "\n") != 3 || strings.Count(first["reactions.jsonl"], "\n") != 3 {
		t.Errorf("pages of 1 lost records:\n%s", first["cards.jsonl"])
	}
}

func TestTwoRunsOverTheSameBoardsWriteTheSameBytes(t *testing.T) {
	first, _ := extract(t, fakebasecamp.New(), boardProject)

	// The order a source lists things in is not what a bundle depends on, save
	// where a list has no other order: Done, and the archive.
	shuffled := fakebasecamp.New()
	slices.Reverse(shuffled.Projects[2].Dock)
	for i := range shuffled.Projects[2].Dock {
		slices.Reverse(shuffled.Projects[2].Dock[i].Lists)
	}
	var busy *fakebasecamp.Card
	for i := range shuffled.Projects[2].Dock {
		for j := range shuffled.Projects[2].Dock[i].Lists {
			if list := &shuffled.Projects[2].Dock[i].Lists[j]; list.ID == 9104 {
				slices.Reverse(list.Cards)
				busy = &list.Cards[len(list.Cards)-1]
			}
		}
	}
	slices.Reverse(busy.Steps)
	slices.Reverse(busy.Comments)
	slices.Reverse(busy.Assignees)
	slices.Reverse(busy.Boosts)
	for i := range busy.Comments {
		slices.Reverse(busy.Comments[i].Boosts)
	}
	second, _ := extract(t, shuffled, boardProject)

	a, b := boardFiles(t, first), boardFiles(t, second)
	for name := range a {
		if a[name] != b[name] {
			t.Errorf("%s differs between two runs:\n%s\n%s", name, a[name], b[name])
		}
	}
}

func TestADiscardedRunOverBoardsAsksTheSameAndCountsTheSame(t *testing.T) {
	written := fakebasecamp.New()
	dir, want := extract(t, written, boardProject)

	dry := fakebasecamp.New()
	writer := bundle.Discard()
	extractor := basecamp.Extractor{Source: dry, Avatars: dry, Files: dry}
	got, err := extractor.Extract(t.Context(), []string{boardProject}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(dry.Requests, written.Requests) {
		t.Errorf("a dry run asked for\n%v\na real one for\n%v", dry.Requests, written.Requests)
	}
	if want = asADryRunKnowsIt(want); !reflect.DeepEqual(got, want) {
		t.Errorf("a dry run reports\n%+v\na real one\n%+v", got, want)
	}
	if len(dry.Downloads) != 0 || len(written.Downloads) != 2 {
		t.Errorf("a dry run downloaded %d files, a real one %d", len(dry.Downloads), len(written.Downloads))
	}
	wantCounts := map[bundle.Kind]int{bundle.People: 6, bundle.Projects: 1, bundle.Tools: 2, bundle.Columns: 8, bundle.Cards: 10,
		bundle.Steps: 3, bundle.Comments: 3, bundle.Reactions: 3, bundle.Attachments: 7}
	if counts := writer.Counts(); !reflect.DeepEqual(counts, wantCounts) {
		t.Errorf("counts %v, want %v", counts, wantCounts)
	}
	for kind, count := range wantCounts {
		if len(records(t, dir, kind)) != count {
			t.Errorf("the real run wrote %d %s, the dry one counted %d", len(records(t, dir, kind)), kind, count)
		}
	}
}

var mentionRef = regexp.MustCompile(`<import-mention ref="([^"]*)"`)

// everyReferenceLeadsToAPerson holds every reference to a person, in every
// file of a bundle, against the people of the bundle.
func everyReferenceLeadsToAPerson(t *testing.T, dir string) map[string]bool {
	t.Helper()
	people := map[string]bool{}
	for _, record := range records(t, dir, bundle.People) {
		people[record["sourceId"].(string)] = true
	}
	for _, kind := range []bundle.Kind{bundle.Tools, bundle.Categories, bundle.Columns, bundle.Cards, bundle.Steps, bundle.Posts, bundle.Messages, bundle.Comments, bundle.Reactions} {
		found := recordsOrNone(t, dir, kind)
		if kind == bundle.Messages {
			found = messages(t, dir)
		}
		for _, record := range found {
			var named []string
			for _, field := range []string{"createdBy", "completedBy"} {
				if reference, ok := record[field].(string); ok {
					named = append(named, reference)
				}
			}
			if assignees, ok := record["assignees"].([]any); ok {
				for _, assignee := range assignees {
					named = append(named, assignee.(string))
				}
			}
			for _, field := range []string{"description", "body"} {
				if text, ok := record[field].(string); ok {
					for _, match := range mentionRef.FindAllStringSubmatch(text, -1) {
						named = append(named, match[1])
					}
				}
			}
			for _, reference := range named {
				if !people[reference] {
					t.Errorf("%s %v names person %s, who is not in the bundle", kind, record["sourceId"], reference)
				}
			}
			if kind == bundle.Reactions && record["createdBy"] == nil {
				t.Errorf("reaction %v does not say whose it is", record["sourceId"])
			}
		}
	}
	return people
}

func TestEveryPersonARecordOfABoardNamesIsInTheBundle(t *testing.T) {
	// Niklaus is no member and comes through a mention alone, Linus through a
	// boost, Edsger as the one who completed a card.
	people := everyReferenceLeadsToAPerson(t, first(extract(t, fakebasecamp.New(), boardProject)))
	for _, id := range []string{"99", "101", "102", "103", "104", "105"} {
		if !people[id] {
			t.Errorf("person %s is not in the bundle", id)
		}
	}

	// Somebody no list knows, in each place a record can name a person.
	for place, put := range map[string]func(*fakebasecamp.Account, *fakebasecamp.Person){
		"the creator of a column": func(a *fakebasecamp.Account, p *fakebasecamp.Person) { doing(a).Creator = p },
		"the creator of a card":   func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyCard(a).Creator = p },
		"an assignee": func(a *fakebasecamp.Account, p *fakebasecamp.Person) {
			busyCard(a).Assignees = []fakebasecamp.Person{*p}
		},
		"who completed a card":       func(a *fakebasecamp.Account, p *fakebasecamp.Person) { roadmap(a).Lists[4].Cards[0].Completion.By = p },
		"the creator of a step":      func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyCard(a).Steps[0].Creator = p },
		"who completed a step":       func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyCard(a).Steps[1].Completion.By = p },
		"the creator of a comment":   func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyCard(a).Comments[2].Creator = p },
		"who boosted a card":         func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyCard(a).Boosts[0].Booster = p },
		"who boosted a comment":      func(a *fakebasecamp.Account, p *fakebasecamp.Person) { busyCard(a).Comments[0].Boosts[0].Booster = p },
		"the creator of an archived": func(a *fakebasecamp.Account, p *fakebasecamp.Person) { doing(a).Cards[2].Creator = p },
	} {
		account := fakebasecamp.New()
		stranger := fakebasecamp.Person{ID: 555, Name: "A Stranger", Email: "stranger@example.test", CreatedAt: "2021-01-01T08:00:00.000Z", UpdatedAt: "2021-01-01T08:00:00.000Z"}
		put(account, &stranger)
		dir, _ := extract(t, account, boardProject)
		if people := everyReferenceLeadsToAPerson(t, dir); !people["555"] {
			t.Errorf("%s did not come into the bundle", place)
		}
		if !strings.Contains(boardFiles(t, dir)["columns.jsonl"]+boardFiles(t, dir)["cards.jsonl"]+boardFiles(t, dir)["steps.jsonl"]+
			boardFiles(t, dir)["comments.jsonl"]+boardFiles(t, dir)["reactions.jsonl"], `"555"`) {
			t.Errorf("%s: no record names the stranger", place)
		}
	}
}

func first(dir string, _ basecamp.Result) string { return dir }

// boardTitles is text of the fake that no message may carry.
var boardTitles = []string{"Roadmap", "Doing", "Inbox", "Draw the map", "Outline", "Fish", "Settled", "Well done", "Grace", "Ada", "Former"}

// A column of a kind a bundle has no role for is left out and counted, and the
// board comes with the columns that are left. It takes the cards standing in it
// with it, which are never asked for; the archived and the trashed ones among
// them count as cards whose column is in no extracted board.
func TestAColumnOfAKindNobodyKnowsIsLeftOutWithTheCardsInIt(t *testing.T) {
	account := fakebasecamp.New()
	doing(account).Type = "Kanban::Swimlane"
	dir, result := extract(t, account, boardProject)

	want := []basecamp.LeftOutRecords{{Record: "column", Field: "type", Value: "Kanban::Swimlane", In: "board", InSourceID: "9020", LeftOut: 1}}
	if !reflect.DeepEqual(result.RecordsLeftOut, want) {
		t.Errorf("the records left out are %v, want %v", result.RecordsLeftOut, want)
	}
	if got, want := sourceIDs(records(t, dir, bundle.Columns)), []string{"9103", "9105", "9101", "9102", "9131", "9132", "9133"}; !slices.Equal(got, want) {
		t.Errorf("columns %v, want %v", got, want)
	}
	cards := sourceIDs(records(t, dir, bundle.Cards))
	for _, card := range []string{"9201", "9202", "9203", "9207", "9208"} {
		if slices.Contains(cards, card) {
			t.Errorf("card %s of a column a bundle has no role for came into the bundle: %v", card, cards)
		}
	}
	for _, list := range []string{"9104", "9113"} {
		if asked := asked(account, "lists/"+list+"/cards.json"); len(asked) != 0 {
			t.Errorf("the cards of list %s were asked for: %v", list, asked)
		}
	}
	// The one of the board that is switched off, and the archived and the
	// trashed card of the column that is gone.
	if result.CardsLeftOut != 3 {
		t.Errorf("%d cards left out, want 3", result.CardsLeftOut)
	}
}

// A status a bundle has no word for leaves the record out and is counted, before
// anything that hangs on it is asked for.
func TestARecordWhoseStatusABundleDoesNotKnowIsLeftOutAndCounted(t *testing.T) {
	account := fakebasecamp.New()
	doing(account).Cards[2].ShownStatus = "drafted"
	busyCard(account).Comments[0].Status = "drafted"
	dir, result := extract(t, account, boardProject)

	want := []basecamp.LeftOutRecords{
		{Record: "card", Field: "status", Value: "drafted", In: "project", InSourceID: "7003", LeftOut: 1},
		{Record: "comment", Field: "status", Value: "drafted", In: "project", InSourceID: "7003", LeftOut: 1},
	}
	if !reflect.DeepEqual(result.RecordsLeftOut, want) {
		t.Errorf("the records left out are %v, want %v", result.RecordsLeftOut, want)
	}
	if cards := sourceIDs(records(t, dir, bundle.Cards)); slices.Contains(cards, "9207") {
		t.Errorf("the card of a status nobody knows came into the bundle: %v", cards)
	}
	if comments := sourceIDs(records(t, dir, bundle.Comments)); slices.Contains(comments, "9301") {
		t.Errorf("the comment of a status nobody knows came into the bundle: %v", comments)
	}
	if asked := asked(account, "recordings/9301/boosts.json"); len(asked) != 0 {
		t.Errorf("the boosts of a comment that is left out were asked for: %v", asked)
	}
}

func TestWhatABundleCannotSayAboutABoardEndsTheRunWithoutNamingAnything(t *testing.T) {
	for what, change := range map[string]func(*fakebasecamp.Account){
		"a second Triage":                 func(a *fakebasecamp.Account) { roadmap(a).Lists[1].Type = fakebasecamp.Triage },
		"a second Done":                   func(a *fakebasecamp.Account) { roadmap(a).Lists[1].Type = fakebasecamp.Done },
		"a second Not now":                func(a *fakebasecamp.Account) { roadmap(a).Lists[0].Type = fakebasecamp.NotNow },
		"a column without createdAt":      func(a *fakebasecamp.Account) { doing(a).CreatedAt = "" },
		"a column without updatedAt":      func(a *fakebasecamp.Account) { doing(a).UpdatedAt = "" },
		"a card without createdAt":        func(a *fakebasecamp.Account) { busyCard(a).CreatedAt = "" },
		"a card without updatedAt":        func(a *fakebasecamp.Account) { busyCard(a).UpdatedAt = "" },
		"a card created at no time":       func(a *fakebasecamp.Account) { busyCard(a).CreatedAt = "the other day" },
		"an archived card without a time": func(a *fakebasecamp.Account) { doing(a).Cards[2].UpdatedAt = "" },
		"a step without createdAt":        func(a *fakebasecamp.Account) { busyCard(a).Steps[0].CreatedAt = "" },
		"a step without updatedAt":        func(a *fakebasecamp.Account) { busyCard(a).Steps[0].UpdatedAt = "" },
		"a comment without createdAt":     func(a *fakebasecamp.Account) { busyCard(a).Comments[0].CreatedAt = "" },
		"a comment without updatedAt":     func(a *fakebasecamp.Account) { busyCard(a).Comments[0].UpdatedAt = "" },
		"a comment created at no time":    func(a *fakebasecamp.Account) { busyCard(a).Comments[0].CreatedAt = "yesterday" },
		"a boost without a time":          func(a *fakebasecamp.Account) { busyCard(a).Boosts[0].CreatedAt = "" },
		"a boost given at no time":        func(a *fakebasecamp.Account) { busyCard(a).Boosts[0].CreatedAt = "just now" },
		"a boost of nobody":               func(a *fakebasecamp.Account) { busyCard(a).Boosts[0].Booster = nil },
		"a boost of a comment, of nobody": func(a *fakebasecamp.Account) { busyCard(a).Comments[0].Boosts[1].Booster = nil },
		"a board that cannot be read":     func(a *fakebasecamp.Account) { a.Fail["buckets/7003/card_tables/lists/9104/cards.json"] = failure },
		"an archive that cannot be read": func(a *fakebasecamp.Account) {
			a.Fail["projects/recordings.json?type=Kanban::Card&bucket=7003&status=trashed"] = failure
		},
		"comments that cannot be read": func(a *fakebasecamp.Account) { a.Fail["buckets/7003/recordings/9201/comments.json"] = failure },
		"boosts that cannot be read":   func(a *fakebasecamp.Account) { a.Fail["buckets/7003/recordings/9301/boosts.json"] = failure },
		"steps at an address that is a text": func(a *fakebasecamp.Account) {
			a.InlineSteps = 1
			a.SubtasksURL[9201] = "https://elsewhere.example.test/Draw the map/subtasks.json"
		},
		"somebody masked among the people": func(a *fakebasecamp.Account) { busyCard(a).Comments[1].Creator.Email = maskedAddress },
	} {
		account := fakebasecamp.New()
		change(account)
		extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
		_, err := extractor.Extract(t.Context(), []string{boardProject}, bundle.Discard())
		var failed *basecamp.Error
		if !errors.As(err, &failed) {
			t.Errorf("%s: got %v, want the run to end", what, err)
			continue
		}
		for _, text := range boardTitles {
			if strings.Contains(failed.Message+failed.Hint, text) {
				t.Errorf("%s: the message names %q: %s", what, text, failed.Message)
			}
		}
	}
}

// What is wrong with a record is said where the walk first meets it, before
// anything that hangs on it is asked for or written.
func TestARecordThatCannotBeCarriedEndsTheRunWhereItIsFirstMet(t *testing.T) {
	for what, c := range map[string]struct {
		change  func(*fakebasecamp.Account)
		message string
	}{
		"a card without createdAt":    {func(a *fakebasecamp.Account) { doing(a).Cards[1].CreatedAt = "" }, "when card 9202 was created"},
		"a card without updatedAt":    {func(a *fakebasecamp.Account) { doing(a).Cards[1].UpdatedAt = "" }, "when card 9202 was updated"},
		"a card without either":       {func(a *fakebasecamp.Account) { doing(a).Cards[1].CreatedAt, doing(a).Cards[1].UpdatedAt = "", "" }, "when card 9202 was created and updated"},
		"a comment without createdAt": {func(a *fakebasecamp.Account) { busyCard(a).Comments[2].CreatedAt = "" }, "when comment 9303 was created"},
		"a comment without updatedAt": {func(a *fakebasecamp.Account) { busyCard(a).Comments[2].UpdatedAt = "" }, "when comment 9303 was updated"},
	} {
		account := fakebasecamp.New()
		c.change(account)
		writer := bundle.Discard()
		extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
		_, err := extractor.Extract(t.Context(), []string{boardProject}, writer)
		if err == nil || !strings.HasSuffix(err.Error(), c.message) && !strings.Contains(err.Error(), c.message+",") {
			t.Errorf("%s: got %v, want a message that ends in %q", what, err, c.message)
		}
		counts := writer.Counts()
		if strings.Contains(what, "card") && (counts[bundle.Cards] != 0 || len(asked(account, "comments.json")) != 0) {
			t.Errorf("%s: %d cards were written and %v asked for", what, counts[bundle.Cards], asked(account, "comments.json"))
		}
		if counts[bundle.Comments] != 0 || len(asked(account, "boosts.json")) != 0 {
			t.Errorf("%s: %d comments were written and %v asked for", what, counts[bundle.Comments], asked(account, "boosts.json"))
		}
	}
}

var failure = &basecamp.Error{Kind: basecamp.API, Message: "the source gave way"}

func TestASecondColumnOfARoleEndsTheRunBeforeACardIsAskedFor(t *testing.T) {
	account := fakebasecamp.New()
	roadmap(account).Lists[1].Type = fakebasecamp.Done
	expectAPIError(t, account, "two columns of the role done", boardProject)
	if requests := asked(account, "cards.json"); len(requests) != 0 {
		t.Errorf("went on to ask for %v", requests)
	}

	// Several columns somebody made are what a board is for.
	account = fakebasecamp.New()
	dir, _ := extract(t, account, boardProject)
	work := 0
	for _, column := range records(t, dir, bundle.Columns) {
		if column["toolSourceId"] == "9020" && column["role"] == "work" {
			work++
		}
	}
	if work != 2 {
		t.Errorf("%d work columns, want 2", work)
	}
}

func TestTimesComeAsPreciseAsBasecampGivesThem(t *testing.T) {
	account := fakebasecamp.New()
	busyCard(account).CreatedAt = "2023-03-05T09:00:00.123+01:00"
	busyCard(account).Completion = &fakebasecamp.Completion{At: "2023-03-06T09:30:00.456789Z"}
	dir, _ := extract(t, account, boardProject)

	card := recordWithID(t, records(t, dir, bundle.Cards), "9201")
	if card["createdAt"] != "2023-03-05T09:00:00.123+01:00" || card["completedAt"] != "2023-03-06T09:30:00.456789Z" || card["completedBy"] != nil {
		t.Errorf("createdAt %v, completedAt %v, completedBy %v", card["createdAt"], card["completedAt"], card["completedBy"])
	}
}

// The letters of two times say nothing about which came first: a zone moves
// the hour, and the same moment is written with and without its fraction.
func TestRecordsAreOrderedByTheMomentAndNotByTheLettersOfATime(t *testing.T) {
	account := fakebasecamp.New()
	busy := busyCard(account)
	// 08:00:00.123 UTC, which is before 08:30 UTC and sorts behind it as text.
	busy.CreatedAt = "2023-03-05T09:00:00.123+01:00"
	doing(account).Cards[1].CreatedAt = "2023-03-05T08:30:00Z"
	// The same moment twice, where the ID decides and the text says otherwise.
	doing(account).OnHold.Cards[0].CreatedAt = "2023-03-05T10:00:00Z"
	roadmap(account).Lists[0].Cards[0].CreatedAt = "2023-03-05T10:00:00.000Z"

	busy.Comments[0].CreatedAt, busy.Comments[1].CreatedAt = "2023-03-05T15:00:00.5+02:00", "2023-03-05T13:30:00Z"
	busy.Comments[2].CreatedAt, busy.Comments[2].ID = "2023-03-05T13:30:00.000Z", 9300
	boosts := busy.Comments[0].Boosts
	boosts[0].CreatedAt, boosts[1].CreatedAt = "2023-03-05T16:00:00.000Z", "2023-03-05T18:59:00+03:00"
	dir, _ := extract(t, account, boardProject)

	ids := sourceIDs(records(t, dir, bundle.Cards))
	if got, want := ids[len(ids)-6:len(ids)-2], []string{"9201", "9202", "9203", "9204"}; !slices.Equal(got, want) {
		t.Errorf("cards in the order %v, want %v among %v", got, want, ids)
	}
	if got, want := sourceIDs(records(t, dir, bundle.Comments)), []string{"9301", "9300", "9302"}; !slices.Equal(got, want) {
		t.Errorf("comments in the order %v, want %v", got, want)
	}
	if got, want := sourceIDs(records(t, dir, bundle.Reactions)), []string{"9503", "9501", "9502"}; !slices.Equal(got, want) {
		t.Errorf("reactions in the order %v, want %v", got, want)
	}
}

func TestACommentInTheTrashIsLeftOutAndCounted(t *testing.T) {
	account := fakebasecamp.New()
	busyCard(account).Comments[0].Status = "trashed"
	busyCard(account).Comments[2].Status = "archived"
	dir, result := extract(t, account, boardProject)

	if got := sourceIDs(records(t, dir, bundle.Comments)); !slices.Equal(got, []string{"9302", "9303"}) || result.CommentsTrashed != 1 {
		t.Errorf("comments %v, %d counted as trashed; want the active and the archived one, and 1", got, result.CommentsTrashed)
	}
	// Nothing is asked of what is not written, and nobody comes in through it.
	if requests := asked(account, "recordings/9301/"); len(requests) != 0 {
		t.Errorf("asked for %v", requests)
	}
	everyReferenceLeadsToAPerson(t, dir)
	for _, reaction := range records(t, dir, bundle.Reactions) {
		if reaction["targetSourceId"] == "9301" {
			t.Errorf("reaction %v answers a comment that is not in the bundle", reaction["sourceId"])
		}
	}
}

func TestWhatIsCompletedWithoutATimeIsNotCompleted(t *testing.T) {
	account := fakebasecamp.New()
	busyCard(account).Steps[1].Completion.At = ""
	roadmap(account).Lists[4].Cards[0].Completion.At = ""
	dir, _ := extract(t, account, boardProject)

	step := recordWithID(t, records(t, dir, bundle.Steps), "9401")
	card := recordWithID(t, records(t, dir, bundle.Cards), "9206")
	for what, record := range map[string]map[string]any{"step": step, "card": card} {
		if record["completedAt"] != nil || record["completedBy"] != nil {
			t.Errorf("the %s was completed at %v by %v, want neither", what, record["completedAt"], record["completedBy"])
		}
	}
}

func TestEveryRecordOfABoardNamesRecordsOfTheBundle(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), boardProject)
	known := map[bundle.Kind]map[string]bool{}
	for _, kind := range []bundle.Kind{bundle.Projects, bundle.Tools, bundle.Columns, bundle.Cards, bundle.Comments} {
		known[kind] = map[string]bool{}
		for _, record := range records(t, dir, kind) {
			known[kind][record["sourceId"].(string)] = true
		}
	}
	for kind, fields := range map[bundle.Kind]map[string]bundle.Kind{
		bundle.Columns:  {"projectSourceId": bundle.Projects, "toolSourceId": bundle.Tools},
		bundle.Cards:    {"projectSourceId": bundle.Projects, "toolSourceId": bundle.Tools, "columnSourceId": bundle.Columns},
		bundle.Steps:    {"projectSourceId": bundle.Projects, "toolSourceId": bundle.Tools, "cardSourceId": bundle.Cards},
		bundle.Comments: {"projectSourceId": bundle.Projects, "parentSourceId": bundle.Cards},
	} {
		for _, record := range records(t, dir, kind) {
			for field, target := range fields {
				if !known[target][record[field].(string)] {
					t.Errorf("%s %v: %s names %v, which is not among the %s", kind, record["sourceId"], field, record[field], target)
				}
			}
		}
	}
	for _, reaction := range records(t, dir, bundle.Reactions) {
		target := map[string]bundle.Kind{"card": bundle.Cards, "comment": bundle.Comments}[reaction["targetKind"].(string)]
		if !known[target][reaction["targetSourceId"].(string)] {
			t.Errorf("reaction %v answers %v %v, which is not in the bundle", reaction["sourceId"], reaction["targetKind"], reaction["targetSourceId"])
		}
	}
}

func TestTheBoardFilesOfABundleAreForTheirOwnerOnly(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), boardProject)
	for _, name := range []string{"columns.jsonl", "cards.jsonl", "steps.jsonl", "comments.jsonl", "reactions.jsonl"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v, %v", name, info, err)
		}
	}
	if content, _ := os.ReadFile(filepath.Join(dir, "cards.jsonl")); bytes.Contains(content, []byte("bc-attachment")) {
		t.Errorf("cards.jsonl carries the markup of the source:\n%s", content)
	}
}
