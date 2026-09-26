package basecamp_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
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
// holds alone, and in doubt a record stays private. The projects are the ones
// that hold posts, to-do lists and questions, so every kind that has a word of
// its own is looked at.
func TestWhereTheSourceSaysNothingAboutClientsNothingIsWritten(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), talkProject, checkinProject, todoProject)

	seen := map[bundle.Kind]int{}
	for _, kind := range []bundle.Kind{bundle.Tools, bundle.Posts, bundle.TodoLists, bundle.Questions} {
		for _, record := range recordsOrNone(t, dir, kind) {
			seen[kind]++
			if _, said := record["visibleToClients"]; said {
				t.Errorf("%s %s says %v about clients, and the source says nothing", kind, record["sourceId"], record["visibleToClients"])
			}
		}
		if seen[kind] == 0 {
			t.Errorf("the bundle holds no %s to look at", kind)
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
		// A client of a second company: a company is the person's own.
		"109": {"client": true, "company": map[string]any{"sourceId": "9282", "name": "Southwind Example"}},
		"110": {"client": true, "company": nil},
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
	if result.PeopleAsClients != 5 {
		t.Errorf("%d clients, want the five of the project", result.PeopleAsClients)
	}
	if result.ClientMemberships != 5 {
		t.Errorf("%d memberships of clients, want the five of the project", result.ClientMemberships)
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

// theClient is the client of the client project whose company a test changes,
// and theClientProject the project itself: a company a bundle cannot carry ends
// a run, so the fake holds none by itself.
func theClient(t *testing.T, a *fakebasecamp.Account) *fakebasecamp.Person {
	t.Helper()
	for i := range a.People {
		if a.People[i].ID == 109 {
			return &a.People[i]
		}
	}
	t.Fatal("the account holds no client 109")
	return nil
}

func theClientProject(t *testing.T, a *fakebasecamp.Account) *fakebasecamp.Project {
	t.Helper()
	for i := range a.Projects {
		if a.Projects[i].ID == 7009 {
			return &a.Projects[i]
		}
	}
	t.Fatal("the account holds no project 7009")
	return nil
}

// A company Minka would refuse ends the run rather than reaching it. Writing the
// record without the company the source names would tell Minka to drop the
// company it holds, which is a lie about a person that no later run takes back.
func TestACompanyMinkaWouldRefuseEndsTheRun(t *testing.T) {
	for name, company := range map[string]fakebasecamp.Company{
		"without an identifier":                {Name: "Northwind Example"},
		"without a name":                       {ID: 9284, Name: "  "},
		"with a name too long":                 {ID: 9284, Name: strings.Repeat("a", 201)},
		"with a control character in its name": {ID: 9284, Name: "North\u0007wind"},
	} {
		for _, where := range []string{"person", "project"} {
			account := fakebasecamp.New()
			if where == "person" {
				theClient(t, account).Company = &company
			} else {
				theClientProject(t, account).ClientCompany = &company
			}
			extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account,
				Now: func() time.Time { return runDay }}
			_, err := extractor.Extract(t.Context(), []string{clientProject}, bundle.Discard())

			var failed *basecamp.Error
			if !errors.As(err, &failed) || failed.Kind != basecamp.API {
				t.Fatalf("%s, a company %s: got %v, want an api error", where, name, err)
			}
			// Never the name itself: a field that holds no name holds something.
			for _, secret := range []string{company.Name, "Cora", "Southward"} {
				if strings.TrimSpace(secret) != "" && strings.Contains(failed.Message, secret) {
					t.Errorf("%s, a company %s: the message names %q: %s", where, name, secret, failed.Message)
				}
			}
			if company.ID != 0 && !strings.Contains(failed.Message, "9284") {
				t.Errorf("%s, a company %s: the message does not say which company: %s", where, name, failed.Message)
			}
		}
	}
}

// The run ends where the client comes in. An extraction of an account walks for
// hours, and a company it read in the first minutes must not end it in the last.
func TestACompanyMinkaWouldRefuseEndsTheRunWhereTheClientComesIn(t *testing.T) {
	account := fakebasecamp.New()
	theClient(t, account).Company = &fakebasecamp.Company{ID: 9284, Name: "  "}
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account,
		Now: func() time.Time { return runDay }}
	_, err := extractor.Extract(t.Context(), []string{clientProject}, bundle.Discard())

	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.API {
		t.Fatalf("got %v, want an api error", err)
	}
	// The members of the project are the last thing the run asked for.
	if last := account.Requests[len(account.Requests)-1]; !strings.Contains(last, "projects/7009/people.json") {
		t.Errorf("the run went on and asked for %s", last)
	}
	for _, part := range []string{"message_boards", "chats", "questionnaires", "todosets", "card_tables"} {
		if went := asked(account, part); len(went) != 0 {
			t.Errorf("the run walked the dock: %v", went)
		}
	}
	if len(account.AvatarRequests) != 0 {
		t.Errorf("the run fetched %d avatars", len(account.AvatarRequests))
	}
}

// A client is no member of a project in Minka, so a question is never said to be
// asked of one: Minka drops such a person without a word, and the counters would
// say a question is asked of somebody who never gets it.
func TestAClientIsNeverSaidToBeAskedAQuestion(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), clientProject)

	question := recordWithID(t, records(t, dir, bundle.Questions), "9271")
	if got := question["asked"]; !reflect.DeepEqual(got, []any{"101"}) {
		t.Errorf("the question is asked of %v, want the one member who is no client", got)
	}
	if result.AskedLeftOut.Clients != 4 {
		t.Errorf("%d clients left out of a question, want the ones subscribed to the two", result.AskedLeftOut.Clients)
	}
}

// Being a client is the first reason somebody cannot be asked, as it is in
// Minka: a client is out of the project's memory before membership is asked at
// all, so a client who has left and comes without an address is counted as a
// client and nowhere else.
func TestSomebodyWhoIsAClientAndMoreIsLeftOutOfAQuestionAsAClient(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), clientProject)

	person := recordWithID(t, records(t, dir, bundle.People), "110")
	if person["client"] != true || person["retired"] != true || person["email"] != nil {
		t.Fatalf("the client who has left and has no address comes as %v", person)
	}
	want := basecamp.AskedLeftOut{Clients: 4}
	if result.AskedLeftOut != want {
		t.Errorf("people left out of asked %+v, want %+v", result.AskedLeftOut, want)
	}
}

// A question whose every subscriber is a client comes asking nobody: the run
// names it, because a later run never touches asked again.
func TestAQuestionAskedOfClientsAloneComesAskingNobody(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), clientProject)

	question := recordWithID(t, records(t, dir, bundle.Questions), "9272")
	if got := question["asked"]; !reflect.DeepEqual(got, []any{}) {
		t.Errorf("the question is asked of %v, want nobody", got)
	}
	want := []basecamp.NobodyAsked{{QuestionSourceID: "9272", LeftOut: 2}}
	if !reflect.DeepEqual(result.NobodyAsked, want) {
		t.Errorf("questions asking nobody %+v, want %+v", result.NobodyAsked, want)
	}
}

// A kind of tool is either extracted or silent about clients, never both: what a
// run extracts is asked first, so the day Docs & Files is extracted this test
// fails and the gap in the report goes with it.
func TestAKindOfToolIsEitherExtractedOrSilentAboutClients(t *testing.T) {
	silent := map[string]bool{
		"vault": true, "inbox": true,
		"kanban_board": false, "chat": false, "message_board": false, "questionnaire": false, "todoset": false,
		"schedule": false,
	}
	for kind, want := range silent {
		if got := basecamp.SaysNothingAboutClients(kind); got != want {
			t.Errorf("%s says nothing about clients: %v, want %v", kind, got, want)
		}
	}
}
