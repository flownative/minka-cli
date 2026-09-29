package basecamp_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// followed is one record of each kind that carries `subscribers`, with the
// project it stands in.
type followed struct {
	kind    bundle.Kind
	record  string
	project string
}

var oneOfEachKind = []followed{
	{bundle.Cards, "9201", boardProject},
	{bundle.Posts, "9701", talkProject},
	{bundle.Todos, "9421", todoProject},
	{bundle.Documents, "9240", docsProject},
	{bundle.Uploads, "9260", docsProject},
}

func subscriptionOf(project, record string) string {
	return "buckets/" + project + "/recordings/" + record + "/subscription.json"
}

// Present, the field is the whole truth for Minka, so every kind has to carry
// it wherever the source said who follows the item, and it has to name exactly
// the members the source names, in the order of their IDs and each once.
func TestEveryKindOfItemSaysWhoFollowsIt(t *testing.T) {
	want := map[string][]any{
		"9201": {"101", "102"},
		"9701": {"102"},
		"9421": {"101", "104"},
		"9240": {"101"},
		"9260": {"101", "102"},
	}
	for _, one := range oneOfEachKind {
		dir, result := extract(t, fakebasecamp.New(), one.project)
		got, carried := recordWithID(t, records(t, dir, one.kind), one.record)["subscribers"]
		if !carried {
			t.Errorf("%s %s carries no subscribers", one.kind, one.record)
			continue
		}
		if !reflect.DeepEqual(got, want[one.record]) {
			t.Errorf("%s %s is followed by %v, want %v", one.kind, one.record, got, want[one.record])
		}
		if len(result.Subscribers.Unread) != 0 {
			t.Errorf("%s: a run that read every list names %v as unread", one.kind, result.Subscribers.Unread)
		}
	}
}

// An empty list is a list: nobody follows the item, and Minka takes everybody
// off it. It is written, not left out.
func TestAnItemNobodyFollowsSaysSo(t *testing.T) {
	account := fakebasecamp.New()
	busyPost(account).Subscribers = nil
	dir, result := extract(t, account, talkProject)

	got, carried := recordWithID(t, records(t, dir, bundle.Posts), "9701")["subscribers"]
	if !carried || !reflect.DeepEqual(got, []any{}) {
		t.Errorf("a post nobody follows carries %v (there: %v), want an empty list", got, carried)
	}
	if result.Subscribers.Lists != len(records(t, dir, bundle.Posts))-result.PostsByStatus[bundle.StatusTrashed] {
		t.Errorf("%d lists counted, want one per post that is not in the trash", result.Subscribers.Lists)
	}
}

// An item in the trash keeps whom it had in Minka, so nobody asks; one in the
// archive is followed there as well and is asked like any other. Basecamp answers
// the list of a trashed recording as of any other, and so does the fake, so only
// the status the run has read keeps the question from going out.
func TestAnItemInTheTrashIsNotAskedAndOneInTheArchiveIs(t *testing.T) {
	for _, one := range []followed{
		{bundle.Cards, "9208", boardProject},
		{bundle.Posts, "9704", talkProject},
		{bundle.Todos, "9423", todoProject},
		{bundle.Documents, "9247", docsProject},
		{bundle.Uploads, "9270", docsProject},
	} {
		account := fakebasecamp.New()
		dir, _ := extract(t, account, one.project)
		record := recordWithID(t, records(t, dir, one.kind), one.record)
		if record["status"] != bundle.StatusTrashed {
			t.Fatalf("%s %s is %v, want a record in the trash", one.kind, one.record, record["status"])
		}
		if slices.Contains(account.Requests, subscriptionOf(one.project, one.record)) {
			t.Errorf("%s %s in the trash was asked who follows it", one.kind, one.record)
		}
		if _, carried := record["subscribers"]; carried {
			t.Errorf("%s %s in the trash carries subscribers", one.kind, one.record)
		}
	}

	dir, _ := extract(t, fakebasecamp.New(), docsProject)
	if got := recordWithID(t, records(t, dir, bundle.Documents), "9255")["subscribers"]; !reflect.DeepEqual(got, []any{"102"}) {
		t.Errorf("an archived document is followed by %v, want [102]", got)
	}
}

// Where the source will not say who follows an item, the item comes without the
// field and Minka follows it as it always did. An empty list in its place would
// take everybody off the item, which is the one mistake this field can make
// silently. The run goes on, and the report names the record.
func TestAListTheSourceWillNotHandOutLeavesTheFieldOutAndIsNamed(t *testing.T) {
	for _, refusal := range []struct {
		kind basecamp.Kind
		why  string
	}{{basecamp.NotFound, basecamp.SubscribersNotFound}, {basecamp.Forbidden, basecamp.SubscribersForbidden}} {
		for _, one := range oneOfEachKind {
			account := fakebasecamp.New()
			account.Fail[subscriptionOf(one.project, one.record)] = &basecamp.Error{Kind: refusal.kind, Message: "Basecamp: refused"}
			dir, result := extract(t, account, one.project)

			if got, carried := recordWithID(t, records(t, dir, one.kind), one.record)["subscribers"]; carried {
				t.Errorf("%s %s answered %s and carries subscribers %v", one.kind, one.record, refusal.kind, got)
			}
			record := map[bundle.Kind]string{bundle.Cards: "card", bundle.Posts: "post", bundle.Todos: "to-do",
				bundle.Documents: "document", bundle.Uploads: "upload"}[one.kind]
			want := []basecamp.UnreadSubscribers{{Record: record, SourceID: one.record, ProjectSourceID: one.project, Why: refusal.why}}
			if !reflect.DeepEqual(result.Subscribers.Unread, want) {
				t.Errorf("%s %s answered %s: unread %v, want %v", one.kind, one.record, refusal.kind, result.Subscribers.Unread, want)
			}
		}
	}
}

// Every other failure ends the run as it does anywhere: a sign-in that ran out
// says nothing about the item, and neither does an answer that names nobody at
// all, which read as an empty list would take everybody off every item.
func TestAFailureThatIsNoRefusalOfTheListEndsTheRun(t *testing.T) {
	account := fakebasecamp.New()
	account.Fail[subscriptionOf(talkProject, "9701")] = &basecamp.Error{Kind: basecamp.AuthRequired, Message: "Basecamp: sign in"}
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
	_, err := extractor.Extract(t.Context(), []string{talkProject}, bundle.Discard())
	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.AuthRequired {
		t.Errorf("a sign-in that ran out ended the run with %v", err)
	}

}

// A list read from the source is the whole truth wherever it goes, for the
// followers of an item as for the people a question asks. An answer that is not
// plainly the whole list ends the run: read as a list, each of these shapes
// would name nobody and take everybody off the item.
func TestAnAnswerThatIsNotTheWholeListEndsTheRun(t *testing.T) {
	ada := map[string]any{"id": 101, "name": "Ada Example"}
	for name, answer := range map[string]map[string]any{
		"no list":                  {"subscribed": true, "count": 0},
		"a list that is null":      {"subscribed": true, "subscribers": nil},
		"an entry that is null":    {"subscribed": true, "count": 1, "subscribers": []any{nil}},
		"an entry without an ID":   {"subscribed": true, "count": 1, "subscribers": []any{map[string]any{"name": "x"}}},
		"an entry whose ID is 0":   {"subscribed": true, "count": 2, "subscribers": []any{ada, map[string]any{"id": 0, "name": "x"}}},
		"fewer than it counts":     {"subscribed": true, "count": 5, "subscribers": []any{}},
		"more than it counts":      {"subscribed": true, "count": 0, "subscribers": []any{ada}},
		"one of several it counts": {"subscribed": true, "count": 2, "subscribers": []any{ada}},
	} {
		for _, project := range []string{talkProject, checkinProject} {
			account := fakebasecamp.New()
			account.SubscriptionAnswer = answer
			extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Now: func() time.Time { return runDay }}
			_, err := extractor.Extract(t.Context(), []string{project}, bundle.Discard())
			var failed *basecamp.Error
			if !errors.As(err, &failed) || failed.Kind != basecamp.API {
				t.Errorf("%s in project %s: the run ended with %v, want an api_error", name, project, err)
			}
		}
	}

	// A list without a count and a list that names somebody twice are the
	// whole truth all the same.
	for name, answer := range map[string]map[string]any{
		"no count":         {"subscribed": true, "subscribers": []any{ada}},
		"one of two twice": {"subscribed": true, "count": 2, "subscribers": []any{ada, ada}},
	} {
		account := fakebasecamp.New()
		account.SubscriptionAnswer = answer
		dir, _ := extract(t, account, talkProject)
		if got := recordWithID(t, records(t, dir, bundle.Posts), "9701")["subscribers"]; !reflect.DeepEqual(got, []any{"101"}) {
			t.Errorf("%s: the post is followed by %v, want [101]", name, got)
		}
	}
}

// Minka puts only members of the project on an item. Whoever is none, is a
// client, has left or comes without an address is left off and counted under
// that reason, one reason each, as for the people a question asks.
func TestWhoMinkaCannotPutOnAnItemIsLeftOffAndCountedByReason(t *testing.T) {
	account := fakebasecamp.New()
	talk := &account.Projects[3]
	// Somebody without an address, somebody deleted and a client become members.
	talk.Members = append(talk.Members, 103, 802, 106)
	linus, deleted, iris := account.People[3], account.Deleted, account.People[7]
	ada, grace := account.People[1], account.People[0]
	busyPost(account).Subscribers = []fakebasecamp.Person{iris, grace, account.Former, linus, deleted, ada, iris}
	dir, result := extract(t, account, talkProject)

	if got := recordWithID(t, records(t, dir, bundle.Posts), "9701")["subscribers"]; !reflect.DeepEqual(got, []any{"101", "102"}) {
		t.Errorf("the post is followed by %v, want [101 102]", got)
	}
	want := basecamp.PeopleLeftOff{NotInTheProject: 1, Clients: 1, Retired: 1, WithoutAnAddress: 1}
	if result.Subscribers.LeftOff != want {
		t.Errorf("left off %+v, want %+v", result.Subscribers.LeftOff, want)
	}
	if result.AskedLeftOut != (basecamp.PeopleLeftOff{}) {
		t.Errorf("the people a question asks count what an item leaves off: %+v", result.AskedLeftOut)
	}
}

// Every person a list names is a member of the item's project in the bundle, so
// Minka never meets somebody the bundle does not carry.
func TestEverySubscriberIsAMemberOfTheProjectInTheBundle(t *testing.T) {
	projects := []string{boardProject, talkProject, todoProject, docsProject, clientProject}
	account := fakebasecamp.New()
	// A client project's post names its clients and an employee.
	for i := range account.Projects {
		if account.Projects[i].ID == 7009 {
			account.Projects[i].Dock[0].Posts[1].Subscribers = []fakebasecamp.Person{account.People[7], account.People[1], account.People[8]}
		}
	}
	dir, result := extract(t, account, projects...)

	members := map[string]map[string]bool{}
	for _, project := range records(t, dir, bundle.Projects) {
		members[project["sourceId"].(string)] = map[string]bool{}
		for _, member := range project["members"].([]any) {
			members[project["sourceId"].(string)][member.(string)] = true
		}
	}
	people := map[string]bool{}
	for _, person := range records(t, dir, bundle.People) {
		people[person["sourceId"].(string)] = person["client"] != true && person["retired"] != true && person["email"] != nil
	}
	seen := 0
	for _, kind := range []bundle.Kind{bundle.Cards, bundle.Posts, bundle.Todos, bundle.Documents, bundle.Uploads} {
		for _, record := range records(t, dir, kind) {
			subscribers, carried := record["subscribers"].([]any)
			if !carried {
				continue
			}
			for _, subscriber := range subscribers {
				seen++
				if !members[record["projectSourceId"].(string)][subscriber.(string)] || !people[subscriber.(string)] {
					t.Errorf("%s %v names %v, who is no member Minka takes", kind, record["sourceId"], subscriber)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no record names anybody who follows it")
	}
	if got := recordWithID(t, records(t, dir, bundle.Posts), "9232")["subscribers"]; !reflect.DeepEqual(got, []any{"101"}) {
		t.Errorf("the client project's post is followed by %v, want [101]", got)
	}
	if result.Subscribers.LeftOff.Clients != 2 {
		t.Errorf("%d clients left off, want 2", result.Subscribers.LeftOff.Clients)
	}
}
