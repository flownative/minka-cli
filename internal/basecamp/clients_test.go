package basecamp_test

import (
	"reflect"
	"testing"

	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

const clientProject = "7009"

// visibility is what a record of a bundle says about clients: absent where it
// says nothing.
func visibility(record map[string]any) any {
	value, said := record["visibleToClients"]
	if !said {
		return "nothing"
	}
	return value
}

// The safeguard: what a record says about clients is its own word. A post of a
// board the clients see can be one the source keeps from them, and taking the
// word of the tool would show it to them.
func TestARecordSaysWhetherClientsSeeItAndNeverTakesTheWordOfItsTool(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), clientProject)

	tools := records(t, dir, bundle.Tools)
	if got := visibility(recordWithID(t, tools, "9220")); got != true {
		t.Fatalf("the board the clients see says %v", got)
	}
	if got := visibility(recordWithID(t, tools, "9221")); got != false {
		t.Fatalf("the board they do not see says %v", got)
	}

	posts := records(t, dir, bundle.Posts)
	if got := visibility(recordWithID(t, posts, "9231")); got != false {
		t.Errorf("the post the source keeps from the clients says %v, want false in a board that says true", got)
	}
	if got := visibility(recordWithID(t, posts, "9232")); got != true {
		t.Errorf("the post of the same board that is shown says %v", got)
	}
	if got := visibility(recordWithID(t, posts, "9233")); got != true {
		t.Errorf("the post the clients see says %v, want true in a board that says false", got)
	}

	lists := records(t, dir, bundle.TodoLists)
	if got := visibility(recordWithID(t, lists, "9241")); got != false {
		t.Errorf("the list the source keeps from the clients says %v, want false in a tool that says true", got)
	}
	if got := visibility(recordWithID(t, lists, "9242")); got != true {
		t.Errorf("the list of the same tool that is shown says %v", got)
	}
	if got := visibility(recordWithID(t, records(t, dir, bundle.Questions), "9271")); got != true {
		t.Errorf("the question the clients see says %v, want true in a tool that says false", got)
	}
}

// Two tools of one kind in one project differ, which is why no tool may take
// another tool's word and no kind may be assumed.
func TestTwoToolsOfOneKindEachSayWhetherClientsSeeThem(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), clientProject)

	tools := records(t, dir, bundle.Tools)
	said := map[string]any{}
	for _, id := range []string{"9222", "9223"} {
		said[recordWithID(t, tools, id)["name"].(string)] = visibility(recordWithID(t, tools, id))
	}
	if want := map[string]any{"Chat": false, "Client Chat": true}; !reflect.DeepEqual(said, want) {
		t.Errorf("the chats say %v, want %v", said, want)
	}
}

// A record that follows what it hangs on carries no word of its own, and the run
// says how many words it left to a container that way.
func TestAWordAboutClientsOnARecordThatFollowsItsContainerIsLeftOutAndCounted(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), clientProject)

	for kind, id := range map[bundle.Kind]string{bundle.Todos: "9261", bundle.TodoGroups: "9251"} {
		record := recordWithID(t, records(t, dir, kind), id)
		if _, said := record["visibleToClients"]; said {
			t.Errorf("%s %s says %v about clients, and follows its list", kind, id, record["visibleToClients"])
		}
	}
	if result.VisibilityLeftToTheContainer != 2 {
		t.Errorf("%d words left to a container, want the group and the to-do", result.VisibilityLeftToTheContainer)
	}
}

// Where the source says nothing, nothing is written: Minka then leaves what it
// holds alone, and in doubt a record stays private.
func TestWhereTheSourceSaysNothingAboutClientsNothingIsWritten(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), "7001")

	for _, kind := range []bundle.Kind{bundle.Tools, bundle.Posts, bundle.TodoLists, bundle.Questions} {
		for _, record := range recordsOrNone(t, dir, kind) {
			if _, said := record["visibleToClients"]; said {
				t.Errorf("%s %s says %v about clients, and the source says nothing", kind, record["sourceId"], record["visibleToClients"])
			}
		}
	}
	if result.VisibilityLeftToTheContainer != 0 {
		t.Errorf("%d words left to a container, want none", result.VisibilityLeftToTheContainer)
	}
}

// A client comes as one, whatever kind of person the source calls them, and only
// a client comes with a company.
func TestAClientComesAsOneAndOnlyAClientComesWithACompany(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), clientProject)

	people := records(t, dir, bundle.People)
	want := map[string]map[string]any{
		"101": {"client": false, "company": "nothing"},
		"106": {"client": true, "company": map[string]any{"sourceId": "9281", "name": "Northwind Example"}},
		// The kind Basecamp had for a client before the flag, whose flag says no.
		"107": {"client": true, "company": nil},
		"108": {"client": true, "company": nil},
		// A company whose name is no name is left out, and the client comes
		// saying that the source names none.
		"109": {"client": true, "company": nil},
	}
	for id, wanted := range want {
		person := recordWithID(t, people, id)
		company, said := person["company"]
		if !said {
			company = "nothing"
		}
		got := map[string]any{"client": person["client"], "company": company}
		if !reflect.DeepEqual(got, wanted) {
			t.Errorf("person %s:\n got %v\nwant %v", id, got, wanted)
		}
	}
	if result.PeopleAsClients != 4 || result.CompaniesLeftOut != 1 {
		t.Errorf("%d clients and %d companies left out, want 4 and 1", result.PeopleAsClients, result.CompaniesLeftOut)
	}
	if result.ClientMemberships != 4 {
		t.Errorf("%d memberships of clients, want the four of the project", result.ClientMemberships)
	}
}

// A project says whether it works with clients and which company, and both
// together: Minka reads a company without the access as a project whose clients
// have none.
func TestAProjectSaysWhetherItWorksWithClientsAndWithWhichCompany(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, clientProject, "7001")

	projects := records(t, dir, bundle.Projects)
	got := recordWithID(t, projects, clientProject)
	want := map[string]any{"clientAccess": true, "clientCompany": map[string]any{"sourceId": "9281", "name": "Northwind Example"}}
	if !reflect.DeepEqual(map[string]any{"clientAccess": got["clientAccess"], "clientCompany": got["clientCompany"]}, want) {
		t.Errorf("the project with clients:\n got %v %v\nwant %v", got["clientAccess"], got["clientCompany"], want)
	}
	// A project that works with no clients says so, and names no company.
	without := recordWithID(t, projects, "7001")
	if without["clientAccess"] != false || without["clientCompany"] != nil {
		t.Errorf("the project without clients says %v and %v", without["clientAccess"], without["clientCompany"])
	}

	account = fakebasecamp.New()
	account.Projects[len(account.Projects)-1].SaysNothingAboutClients = true
	dir, _ = extract(t, account, clientProject)
	silent := recordWithID(t, records(t, dir, bundle.Projects), clientProject)
	if _, access := silent["clientAccess"]; access {
		t.Errorf("a source that says nothing of clients wrote clientAccess %v", silent["clientAccess"])
	}
	if _, company := silent["clientCompany"]; company {
		t.Errorf("a source that says nothing of clients wrote clientCompany %v", silent["clientCompany"])
	}
}

// The dock says nothing about clients, so the word is read where the tool itself
// is read and nowhere else.
func TestTheWordAboutClientsOfAToolIsReadFromTheToolAndNotFromTheDock(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, clientProject)

	if got := visibility(recordWithID(t, records(t, dir, bundle.Tools), "9220")); got != true {
		t.Fatalf("the board says %v", got)
	}
	if asked := asked(account, "message_boards/9220.json"); len(asked) != 1 {
		t.Errorf("the tool was asked for %d times, want once: %v", len(asked), asked)
	}
}
