package basecamp_test

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
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

// groupsTheAccountShows are the groups of a list as the account answers them,
// each in full: what a test holds against the bundle is what the account said.
func groupsTheAccountShows(t *testing.T, account *fakebasecamp.Account, bucket, list int64) []map[string]any {
	t.Helper()
	var groups []map[string]any
	for page := 1; ; page++ {
		content, err := account.Get(context.Background(),
			"buckets/"+strconv.FormatInt(bucket, 10)+"/todolists/"+strconv.FormatInt(list, 10)+"/groups.json?page="+strconv.Itoa(page))
		if err != nil {
			t.Fatal(err)
		}
		var listed []map[string]any
		if err := json.Unmarshal(content, &listed); err != nil {
			t.Fatal(err)
		}
		if len(listed) == 0 {
			return groups
		}
		groups = append(groups, listed...)
	}
}

// todosTheAccountShows are the to-dos of a list or a group as the account answers
// them, those that are open or those that are done.
func todosTheAccountShows(t *testing.T, account *fakebasecamp.Account, bucket, in int64, done bool) []map[string]any {
	t.Helper()
	completed := ""
	if done {
		completed = "?completed=true"
	}
	var todos []map[string]any
	for page := 1; ; page++ {
		content, err := account.Get(context.Background(),
			"buckets/"+strconv.FormatInt(bucket, 10)+"/todolists/"+strconv.FormatInt(in, 10)+"/todos.json"+completed+
				map[bool]string{true: "&page=", false: "?page="}[done]+strconv.Itoa(page))
		if err != nil {
			t.Fatal(err)
		}
		var listed []map[string]any
		if err := json.Unmarshal(content, &listed); err != nil {
			t.Fatal(err)
		}
		if len(listed) == 0 {
			return todos
		}
		todos = append(todos, listed...)
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

// A list that hangs on a To-dos tool the run does not extract has nowhere to go,
// and neither has a group of such a list: both are left out and counted, and the
// run goes on.
func TestWhatHangsOnNoExtractedToolIsLeftOutAndCounted(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), todoProject)

	if ids := sourceIDs(records(t, dir, bundle.TodoLists)); slices.Contains(ids, "9406") {
		t.Errorf("the list of a tool that is switched off came along: %v", ids)
	}
	if ids := sourceIDs(records(t, dir, bundle.TodoGroups)); slices.Contains(ids, "9415") {
		t.Errorf("the group of a list nobody extracts came along: %v", ids)
	}
	if ids := sourceIDs(records(t, dir, bundle.Todos)); slices.Contains(ids, "9428") {
		t.Errorf("the to-do of a list nobody extracts came along: %v", ids)
	}
	if result.TodosLeftOut != 3 {
		t.Errorf("%d records left out, want the list, the group and the to-do of the tool that is switched off", result.TodosLeftOut)
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
	lists := sourceIDs(records(t, dir, bundle.TodoLists))
	groups := sourceIDs(records(t, dir, bundle.TodoGroups))
	for _, one := range records(t, dir, bundle.Todos) {
		if !named(projects, one["projectSourceId"]) || !named(tools, one["toolSourceId"]) || !named(lists, one["listSourceId"]) {
			t.Errorf("to-do %v names the project %v, the tool %v and the list %v",
				one["sourceId"], one["projectSourceId"], one["toolSourceId"], one["listSourceId"])
		}
		if in := one["groupSourceId"]; in != nil && !named(groups, in) {
			t.Errorf("to-do %v stands in the group %v, which is not in the bundle", one["sourceId"], in)
		}
		for _, field := range []string{"createdBy", "completedBy"} {
			if who := one[field]; who != nil && !named(people, who) {
				t.Errorf("the %s of to-do %v is %v, who is not in the bundle", field, one["sourceId"], who)
			}
		}
		for _, field := range []string{"assignees", "notifyWhenDone"} {
			for _, who := range one[field].([]any) {
				if !named(people, who) {
					t.Errorf("to-do %v names %v in %s, who is not in the bundle", one["sourceId"], who, field)
				}
			}
		}
	}
	for _, group := range records(t, dir, bundle.TodoGroups) {
		if !named(projects, group["projectSourceId"]) || !named(tools, group["toolSourceId"]) || !named(lists, group["listSourceId"]) {
			t.Errorf("group %v names the project %v, the tool %v and the list %v",
				group["sourceId"], group["projectSourceId"], group["toolSourceId"], group["listSourceId"])
		}
		if by := group["createdBy"]; by != nil && !named(people, by) {
			t.Errorf("group %v was made by %v, who is not in the bundle", group["sourceId"], by)
		}
	}
}

// A group goes to the end of its list, so the order of the file is the order of
// the list: the groups in use as the source has them, then the one it has thrown
// away, which comes with its status rather than being left out.
func TestTheGroupsOfAListComeInTheOrderOfTheListAndKeepTheirStatus(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), todoProject)

	want := []map[string]any{
		{"sourceId": "9411", "createdAt": "2025-04-01T14:00:00.000Z", "updatedAt": "2025-04-01T14:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "title": "Before the launch", "status": "active"},
		{"sourceId": "9412", "createdAt": "2025-04-01T15:00:00.000Z", "updatedAt": "2025-04-01T15:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "title": "After the launch", "status": "active"},
		{"sourceId": "9413", "createdAt": "2025-04-01T16:00:00.000Z", "updatedAt": "2025-04-03T16:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "title": "Given up", "status": "trashed"},
	}
	if got := records(t, dir, bundle.TodoGroups); !reflect.DeepEqual(got, want) {
		t.Errorf("groups:\n got %v\nwant %v", got, want)
	}
}

// A group knows the trash and no shelf, so a bundle has no word for one the
// source has put on the shelf: it is left out, counted with the word that made
// it impossible, and the run goes on.
func TestAGroupOnTheShelfIsLeftOutAndCounted(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), todoProject)

	if ids := sourceIDs(records(t, dir, bundle.TodoGroups)); slices.Contains(ids, "9414") {
		t.Errorf("a group on the shelf came along: %v", ids)
	}
	want := []basecamp.LeftOutRecords{{Record: "group", Field: "status", Value: "archived", In: "project", InSourceID: "7008", LeftOut: 1}}
	if got := result.RecordsLeftOut; !slices.Equal(got, want) {
		t.Errorf("records left out:\n got %v\nwant %v", got, want)
	}
}

// The format has no word for a comment, a reaction or a file of a group, so such
// a record would wait in Minka for ever: none is written, and none is asked for.
// The account holds all three on a group and answers for it as it does for a
// list, so a run that took them would be seen taking them.
func TestNothingEverHangsOnAGroup(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, todoProject)

	shown := groupsTheAccountShows(t, account, 7008, 9401)
	if len(shown) == 0 {
		t.Fatal("the account shows no group of the list, and the test needs one that carries something")
	}
	group := shown[0]
	if group["id"].(float64) != 9411 || group["comments_count"].(float64) == 0 || group["boosts_count"].(float64) == 0 ||
		len(group["description_attachments"].([]any)) == 0 {
		t.Fatalf("the account says of the group %v, and the test needs one with a comment, a boost and a file", group)
	}

	for _, comment := range records(t, dir, bundle.Comments) {
		if comment["parentSourceId"] == "9411" || comment["sourceId"] == "9455" {
			t.Errorf("a comment of a group was written: %v", comment)
		}
	}
	for _, reaction := range records(t, dir, bundle.Reactions) {
		if reaction["targetSourceId"] == "9411" || reaction["sourceId"] == "9465" {
			t.Errorf("a reaction of a group was written: %v", reaction)
		}
	}
	for _, file := range records(t, dir, bundle.Attachments) {
		if file["ownerSourceId"] == "9411" || file["sourceId"] == "attachment-5062" {
			t.Errorf("a file of a group was written: %v", file)
		}
	}
	if asked := asked(account, "recordings/9411/"); len(asked) > 0 {
		t.Errorf("a run asked for what hangs on a group: %v", asked)
	}
}

// A to-do goes to the end of its list or its group, so the order of the file is
// the order of the list: what is open as the source lists it, then what is done,
// then what it has thrown away, which comes with its status. The days, the people
// and the tick come as the source has them, and a group names a group.
func TestTheToDosOfAListComeInItsOrderWithTheirDaysAndPeople(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), todoProject)

	notes := `<div>On thick paper, says <import-mention ref="104">Edsger Invented</import-mention></div>` +
		`<import-attachment ref="attachment-5063"></import-attachment>`
	want := []map[string]any{
		{"sourceId": "9421", "createdAt": "2025-04-02T08:00:00.000Z", "updatedAt": "2025-04-02T08:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "groupSourceId": nil,
			"title": "Print the invitations", "notes": notes, "startsOn": "2025-04-20", "dueOn": "2025-05-01",
			"assignees": []any{"101", "102"}, "notifyWhenDone": []any{"104"},
			"completedAt": nil, "completedBy": nil, "status": "active"},
		{"sourceId": "9422", "createdAt": "2025-04-02T09:00:00.000Z", "updatedAt": "2025-04-03T09:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "groupSourceId": nil,
			"title": "Book the room", "notes": nil, "startsOn": nil, "dueOn": nil,
			"assignees": []any{}, "notifyWhenDone": []any{},
			"completedAt": "2025-04-03T09:00:00.000Z", "completedBy": "102", "status": "active"},
		{"sourceId": "9423", "createdAt": "2025-04-02T10:00:00.000Z", "updatedAt": "2025-04-03T10:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "groupSourceId": nil,
			"title": "Cancelled", "notes": nil, "startsOn": nil, "dueOn": nil,
			"assignees": []any{}, "notifyWhenDone": []any{},
			"completedAt": nil, "completedBy": nil, "status": "trashed"},
		{"sourceId": "9424", "createdAt": "2025-04-02T18:00:00.000Z", "updatedAt": "2025-04-02T18:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "groupSourceId": "9411",
			"title": "Choose a date", "notes": nil, "startsOn": nil, "dueOn": nil,
			"assignees": []any{"99"}, "notifyWhenDone": []any{},
			"completedAt": nil, "completedBy": nil, "status": "active"},
		{"sourceId": "9425", "createdAt": "2025-04-02T19:00:00.000Z", "updatedAt": "2025-04-03T19:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "groupSourceId": "9412",
			"title": "Send thanks", "notes": nil, "startsOn": nil, "dueOn": nil,
			"assignees": []any{}, "notifyWhenDone": []any{},
			"completedAt": "2025-04-02T20:00:00.000Z", "completedBy": "102", "status": "active"},
		{"sourceId": "9426", "createdAt": "2025-04-02T21:00:00.000Z", "updatedAt": "2025-04-02T21:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7008", "toolSourceId": "9080", "listSourceId": "9401", "groupSourceId": "9413",
			"title": "Of a group in the trash", "notes": nil, "startsOn": nil, "dueOn": nil,
			"assignees": []any{}, "notifyWhenDone": []any{},
			"completedAt": nil, "completedBy": nil, "status": "active"},
		{"sourceId": "9427", "createdAt": "2025-04-02T22:00:00.000Z", "updatedAt": "2025-04-02T22:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7008", "toolSourceId": "9081", "listSourceId": "9405", "groupSourceId": nil,
			"title": "Water the plants", "notes": nil, "startsOn": nil, "dueOn": nil,
			"assignees": []any{}, "notifyWhenDone": []any{},
			"completedAt": nil, "completedBy": nil, "status": "active"},
	}
	if got := records(t, dir, bundle.Todos); !reflect.DeepEqual(got, want) {
		t.Errorf("to-dos:\n got %v\nwant %v", got, want)
	}
	if got := result.TodosByStatus; !reflect.DeepEqual(got, map[string]int{"active": 6, "trashed": 1}) {
		t.Errorf("to-dos by status: %v", got)
	}
	// Somebody who has left the account comes into the bundle for being on a
	// to-do, as retired as anywhere else.
	if person := recordWithID(t, records(t, dir, bundle.People), "99"); person["retired"] != true {
		t.Errorf("the assignee who has left: %v", person)
	}
}

// Minka passes a record over whole whose updatedAt has not moved since the run
// before, so a to-do whose completion is newer than the moment the source says it
// was touched would never get its tick: it is written as touched when it was
// done. One done earlier keeps the moment the source gives it.
func TestATodoIsNeverSaidToBeOlderThanItsCompletion(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, todoProject)

	done := todosTheAccountShows(t, account, 7008, 9401, true)
	if len(done) != 1 || done[0]["id"].(float64) != 9422 {
		t.Fatalf("the account shows %v as done, and the test needs the one to-do it was built with", done)
	}
	completion := done[0]["completion"].(map[string]any)
	if completion["created_at"].(string) <= done[0]["updated_at"].(string) {
		t.Fatalf("the account says the to-do was touched at %v and done at %v, and the test needs a completion that is the newer of the two",
			done[0]["updated_at"], completion["created_at"])
	}

	later := recordWithID(t, records(t, dir, bundle.Todos), "9422")
	if later["updatedAt"] != completion["created_at"] || later["completedAt"] != completion["created_at"] {
		t.Errorf("a to-do done after it was last touched: %v", later)
	}
	// The other way round the source decides: a completion that is older changes
	// nothing.
	earlier := recordWithID(t, records(t, dir, bundle.Todos), "9425")
	if earlier["updatedAt"] != "2025-04-03T19:00:00.000Z" || earlier["completedAt"] != "2025-04-02T20:00:00.000Z" {
		t.Errorf("a to-do done before it was last touched: %v", earlier)
	}
}

// A groupSourceId names a group of the to-do's own list. Minka disguises a group
// of another list as "the list is not in use here", so such a to-do would wait
// for ever without anybody knowing why.
func TestAGroupOfAToDoIsAlwaysAGroupOfItsOwnList(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), todoProject)

	groups := map[string]map[string]any{}
	for _, group := range records(t, dir, bundle.TodoGroups) {
		groups[group["sourceId"].(string)] = group
	}
	named := 0
	for _, one := range records(t, dir, bundle.Todos) {
		id, has := one["groupSourceId"].(string)
		if !has {
			continue
		}
		named++
		group, known := groups[id]
		if !known {
			t.Errorf("to-do %v names the group %v, which is not in the bundle", one["sourceId"], id)
			continue
		}
		if group["listSourceId"] != one["listSourceId"] || group["toolSourceId"] != one["toolSourceId"] {
			t.Errorf("to-do %v stands in the list %v and names the group %v of the list %v",
				one["sourceId"], one["listSourceId"], id, group["listSourceId"])
		}
	}
	if named == 0 {
		t.Fatal("no to-do of the bundle names a group, so the safeguard is held by nothing")
	}
}

// A bundle has no word for the subtasks of a to-do, so all that is left of them
// is how many there were. None is asked for either: a run fetches nothing it
// cannot carry.
func TestTheSubtasksOfAToDoAreCountedAndNeverAskedFor(t *testing.T) {
	account := fakebasecamp.New()
	_, result := extract(t, account, todoProject)

	open := todosTheAccountShows(t, account, 7008, 9401, false)
	if len(open) == 0 || open[0]["subtasks_count"].(float64) != 2 {
		t.Fatalf("the account shows %v, and the test needs a to-do with subtasks", open)
	}
	if result.TodosWithSubtasks != 1 || result.SubtasksLeftOut != 2 {
		t.Errorf("%d to-dos with %d subtasks, want 1 with 2", result.TodosWithSubtasks, result.SubtasksLeftOut)
	}
	if asked := asked(account, "subtasks"); len(asked) > 0 {
		t.Errorf("a run asked for subtasks it cannot carry: %v", asked)
	}
}

// What hangs on a to-do goes the way every other discussion goes.
func TestWhatHangsOnAToDoGoesTheWayEveryDiscussionGoes(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), todoProject)

	comment := recordWithID(t, records(t, dir, bundle.Comments), "9471")
	if comment["parentKind"] != bundle.ParentTodo || comment["parentSourceId"] != "9421" {
		t.Errorf("the comment of a to-do: %v", comment)
	}
	boost := recordWithID(t, records(t, dir, bundle.Reactions), "9481")
	if boost["targetKind"] != bundle.TargetTodo || boost["targetSourceId"] != "9421" {
		t.Errorf("the boost of a to-do: %v", boost)
	}
	onTheComment := recordWithID(t, records(t, dir, bundle.Reactions), "9482")
	if onTheComment["targetKind"] != bundle.TargetComment || onTheComment["targetSourceId"] != "9471" {
		t.Errorf("the boost of the comment of a to-do: %v", onTheComment)
	}
	file := recordWithID(t, records(t, dir, bundle.Attachments), "attachment-5063")
	if file["ownerKind"] != bundle.OwnerTodo || file["ownerSourceId"] != "9421" || file["file"] != "attachment-5063/card.png" {
		t.Errorf("the file of the notes of a to-do: %v", file)
	}
}
