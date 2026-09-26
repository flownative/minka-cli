package basecamp_test

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// The to-dos of the fake are in its eighth project: two To-dos tools, lists in
// all three states, and a tool that is switched off whose list hangs on nothing
// a run extracts.
const todoProject = "7008"

// listsTheAccountShows are the lists of a tool in the order the account answers
// with, which is the order they stand in at the source. Read from the answer and
// not from the fixture: what a test holds against the bundle is what the account
// said.
func listsTheAccountShows(t *testing.T, account *fakebasecamp.Account, bucket, tool int64) []string {
	t.Helper()
	var ids []string
	for page := 1; ; page++ {
		content, err := account.Get(context.Background(),
			"buckets/"+strconv.FormatInt(bucket, 10)+"/todosets/"+strconv.FormatInt(tool, 10)+"/todolists.json?page="+strconv.Itoa(page))
		if err != nil {
			t.Fatal(err)
		}
		var listed []map[string]any
		if err := json.Unmarshal(content, &listed); err != nil {
			t.Fatal(err)
		}
		if len(listed) == 0 {
			return ids
		}
		for _, one := range listed {
			ids = append(ids, strconv.FormatInt(int64(one["id"].(float64)), 10))
		}
	}
}

// The lists of a To-dos tool come with the text and the state the source left
// them in, and the file holds the lists of a tool from the bottom up.
func TestTheListsOfAToDosToolComeWithTheirTextAndTheirState(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), todoProject)

	want := []map[string]any{
		{"sourceId": "9403", "createdAt": "2025-04-01T10:00:00.000Z", "updatedAt": "2025-04-03T10:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "title": "Finished", "description": nil, "status": "archived"},
		{"sourceId": "9404", "createdAt": "2025-04-01T11:00:00.000Z", "updatedAt": "2025-04-03T11:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "title": "Dropped", "description": nil, "status": "trashed"},
		{"sourceId": "9402", "createdAt": "2025-04-01T09:00:00.000Z", "updatedAt": "2025-04-01T09:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7008", "toolSourceId": "9080", "title": "Later", "description": nil, "status": "active"},
		{"sourceId": "9401", "createdAt": "2025-04-01T08:00:00.000Z", "updatedAt": "2025-04-02T08:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "title": "Launch", "status": "active",
			"description": `<div>Everything for the launch, with <import-mention ref="102">Grace Sample</import-mention></div>` +
				`<import-attachment ref="attachment-5061"></import-attachment>`},
		{"sourceId": "9405", "createdAt": "2025-04-01T12:00:00.000Z", "updatedAt": "2025-04-01T12:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7008", "toolSourceId": "9081", "title": "Every week", "description": nil, "status": "active"},
	}
	if got := records(t, dir, bundle.TodoLists); !reflect.DeepEqual(got, want) {
		t.Errorf("to-do lists:\n got %v\nwant %v", got, want)
	}
	if got := result.TodoListsByStatus; !reflect.DeepEqual(got, map[string]int{"active": 3, "archived": 1, "trashed": 1}) {
		t.Errorf("lists by status: %v", got)
	}
}

// Minka puts every new list on top of its tool, so the last line of a tool is
// the list on top. The account answers with the lists as they stand, the topmost
// first, which is what the order of the file is held against.
func TestTheListsOfAToolAreWrittenFromTheBottomUp(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, todoProject)

	shown := listsTheAccountShows(t, account, 7008, 9080)
	if !slices.Equal(shown, []string{"9401", "9402"}) {
		t.Fatalf("the account shows the lists as %v, and the test needs the order they stand in", shown)
	}

	var inUse []string
	for _, list := range records(t, dir, bundle.TodoLists) {
		if list["toolSourceId"] == "9080" && list["status"] == "active" {
			inUse = append(inUse, list["sourceId"].(string))
		}
	}
	want := slices.Clone(shown)
	slices.Reverse(want)
	if !slices.Equal(inUse, want) {
		t.Errorf("the file holds the lists in use as %v, want %v: the topmost list is the last line", inUse, want)
	}
}

// A list that hangs on a To-dos tool the run does not extract has nowhere to go:
// it is left out and counted, and the run goes on.
func TestAListOfAToolThatIsNotExtractedIsLeftOutAndCounted(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), todoProject)

	if ids := sourceIDs(records(t, dir, bundle.TodoLists)); slices.Contains(ids, "9406") {
		t.Errorf("the list of a tool that is switched off came along: %v", ids)
	}
	if result.TodosLeftOut != 1 {
		t.Errorf("%d records left out, want the one list of the tool that is switched off", result.TodosLeftOut)
	}
}

// What hangs on a list goes the way every other discussion goes: its comments,
// its boosts, the boosts of its comments and the files of its text.
func TestWhatHangsOnAListGoesTheWayEveryDiscussionGoes(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), todoProject)

	comment := recordWithID(t, records(t, dir, bundle.Comments), "9451")
	if comment["parentKind"] != bundle.ParentTodoList || comment["parentSourceId"] != "9401" {
		t.Errorf("the comment of a list: %v", comment)
	}
	boost := recordWithID(t, records(t, dir, bundle.Reactions), "9461")
	if boost["targetKind"] != bundle.TargetTodoList || boost["targetSourceId"] != "9401" {
		t.Errorf("the boost of a list: %v", boost)
	}
	onTheComment := recordWithID(t, records(t, dir, bundle.Reactions), "9462")
	if onTheComment["targetKind"] != bundle.TargetComment || onTheComment["targetSourceId"] != "9451" {
		t.Errorf("the boost of the comment of a list: %v", onTheComment)
	}
	file := recordWithID(t, records(t, dir, bundle.Attachments), "attachment-5061")
	if file["ownerKind"] != bundle.OwnerTodoList || file["ownerSourceId"] != "9401" || file["file"] != "attachment-5061/plan.png" {
		t.Errorf("the file of the text of a list: %v", file)
	}
}

// Every reference a record of a To-dos tool carries leads to a record of the
// bundle: the applier refuses one that leads nowhere.
func TestEveryRecordOfAToDosToolNamesRecordsOfTheBundle(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), todoProject)

	tools := sourceIDs(records(t, dir, bundle.Tools))
	projects := sourceIDs(records(t, dir, bundle.Projects))
	people := sourceIDs(records(t, dir, bundle.People))

	named := func(known []string, value any) bool {
		id, isText := value.(string)
		return isText && slices.Contains(known, id)
	}
	for _, list := range records(t, dir, bundle.TodoLists) {
		if !named(projects, list["projectSourceId"]) || !named(tools, list["toolSourceId"]) {
			t.Errorf("list %v names the project %v and the tool %v", list["sourceId"], list["projectSourceId"], list["toolSourceId"])
		}
		if by := list["createdBy"]; by != nil && !named(people, by) {
			t.Errorf("list %v was made by %v, who is not in the bundle", list["sourceId"], by)
		}
	}
}
