package basecamp_test

import (
	"slices"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// The first project of the fake was created at 09:00:00 UTC. Its tools 9001 to
// 9004 are extracted; 9005 to 9009 are not, two of them being switched off.
const withTheProject = "2022-05-01T09:00:00.150Z"

func creatorOfProject(t *testing.T, account *fakebasecamp.Account, project string) any {
	t.Helper()
	dir, _ := extract(t, account, project)
	return recordWithID(t, records(t, dir, bundle.Projects), project)["createdBy"]
}

// toolRequests are the requests for tools the run does not extract.
func toolRequests(account *fakebasecamp.Account) []string {
	var requests []string
	for _, kind := range []string{"todosets", "schedules", "vaults", "card_tables/9007", "card_tables/9021"} {
		requests = append(requests, asked(account, kind)...)
	}
	slices.Sort(requests)
	return requests
}

func TestWhoMadeAToolTogetherWithAProjectCreatedTheProject(t *testing.T) {
	// A tool that is extracted answers, and nothing else is asked for.
	account := fakebasecamp.New()
	account.Projects[0].Dock[3].CreatedAt = withTheProject
	if got := creatorOfProject(t, account, "7001"); got != "102" {
		t.Errorf("created by %v, want the creator of the tool made with the project", got)
	}
	if requests := toolRequests(account); len(requests) != 0 {
		t.Errorf("asked for %v although a tool of the bundle answers", requests)
	}

	// The project with the boards has such a tool as it stands.
	account = fakebasecamp.New()
	if got := creatorOfProject(t, account, boardProject); got != "101" || len(toolRequests(account)) != 0 {
		t.Errorf("created by %v, asked for %v", got, toolRequests(account))
	}
}

func TestAToolAddedLaterNeverNamesTheCreatorOfAProject(t *testing.T) {
	for what, createdAt := range map[string]string{
		"days later":                     "",
		"just outside the window":        "2022-05-01T09:00:05.001Z",
		"before the project":             "2022-05-01T08:59:59.999Z",
		"the same letters, an hour away": "2022-05-01T09:00:00.150+01:00",
		"no time at all":                 "in the morning",
	} {
		account := fakebasecamp.New()
		if createdAt != "" {
			for i := range account.Projects[0].Dock {
				account.Projects[0].Dock[i].CreatedAt = createdAt
			}
		}
		if got := creatorOfProject(t, account, "7001"); got != nil {
			t.Errorf("%s: created by %v, want nobody", what, got)
		}
	}

	// The window is closed at its end and at its beginning.
	for _, createdAt := range []string{"2022-05-01T09:00:00.000Z", "2022-05-01T09:00:05.000Z"} {
		account := fakebasecamp.New()
		account.Projects[0].Dock[0].CreatedAt = createdAt
		if got := creatorOfProject(t, account, "7001"); got != "101" {
			t.Errorf("%s: created by %v, want 101", createdAt, got)
		}
	}
}

func TestTheWindowComparesMomentsAndNotTheLettersOfATime(t *testing.T) {
	account := fakebasecamp.New()
	account.Projects[0].CreatedAt = "2022-05-01T11:00:00+02:00"
	account.Projects[0].Dock[0].CreatedAt = "2022-05-01T09:00:00.150Z"
	if got := creatorOfProject(t, account, "7001"); got != "101" {
		t.Errorf("created by %v, want 101: 11:00 +02:00 is 09:00 UTC", got)
	}
}

func TestOnlyAFewOtherToolsAreAskedAndOnlyUntilOneAnswers(t *testing.T) {
	grace := fakebasecamp.Person{ID: 102, Name: "Grace Sample", Email: "grace@example.test"}

	// The second of the others answers, one that is switched off.
	account := fakebasecamp.New()
	slices.Reverse(account.Projects[0].Dock)
	for i := range account.Projects[0].Dock {
		if tool := &account.Projects[0].Dock[i]; tool.ID == 9006 {
			tool.CreatedAt, tool.Creator = withTheProject, &grace
		}
	}
	if got := creatorOfProject(t, account, "7001"); got != "102" {
		t.Errorf("created by %v, want 102", got)
	}
	want := []string{"buckets/7001/schedules/9006.json", "buckets/7001/todosets/9005.json"}
	if got := toolRequests(account); !slices.Equal(got, want) {
		t.Errorf("asked for %v, want the others in the order they were made in, up to the one that answers: %v", got, want)
	}

	// Only the fifth would answer: that is more than a run asks for.
	account = fakebasecamp.New()
	account.Projects[0].Dock[8].CreatedAt = withTheProject
	if got := creatorOfProject(t, account, "7001"); got != nil {
		t.Errorf("created by %v, want nobody", got)
	}
	want = []string{"buckets/7001/card_tables/9007.json", "buckets/7001/schedules/9006.json", "buckets/7001/todosets/9005.json"}
	if got := toolRequests(account); !slices.Equal(got, want) {
		t.Errorf("asked for %v, want %v and no more", got, want)
	}
}

func TestAToolThatCannotAnswerDoesNotEndTheRun(t *testing.T) {
	// One that names nobody, one that is not there, one at an address outside
	// the account, which is not asked for and does not count.
	account := fakebasecamp.New()
	dock := account.Projects[0].Dock
	dock[0].CreatedAt, dock[0].Creator = withTheProject, nil
	account.Fail["buckets/7001/todosets/9005.json"] = &basecamp.Error{Kind: basecamp.NotFound, Message: "gone"}
	dock[5].URL = "https://elsewhere.example.test/4242/buckets/7001/schedules/9006.json"
	dock[7].CreatedAt = withTheProject
	if got := creatorOfProject(t, account, "7001"); got != "101" {
		t.Errorf("created by %v, want 101", got)
	}
	if requests := asked(account, "elsewhere"); len(requests) != 0 || len(asked(account, "schedules")) != 0 {
		t.Errorf("asked for a tool outside the account: %v", account.Requests)
	}

	// Anything else that goes wrong is what it is for every request.
	account = fakebasecamp.New()
	account.Fail["buckets/7001/todosets/9005.json"] = &basecamp.Error{Kind: basecamp.Network, Message: "connection reset"}
	extractor := basecamp.Extractor{Source: account, Avatars: account}
	if _, err := extractor.Extract(t.Context(), []string{"7001"}, bundle.Discard()); err == nil {
		t.Error("a request that failed was passed over")
	}
}

func TestTheCreatorOfAProjectComesIntoTheBundle(t *testing.T) {
	account := fakebasecamp.New()
	stranger := fakebasecamp.Person{ID: 555, Name: "A Stranger", Email: "stranger@example.test", CreatedAt: "2021-01-01T08:00:00.000Z", UpdatedAt: "2021-01-01T08:00:00.000Z"}
	account.Projects[0].Dock[5].CreatedAt, account.Projects[0].Dock[5].Creator = withTheProject, &stranger
	dir, _ := extract(t, account, "7001")

	if got := recordWithID(t, records(t, dir, bundle.Projects), "7001")["createdBy"]; got != "555" {
		t.Fatalf("created by %v, want 555", got)
	}
	person := recordWithID(t, records(t, dir, bundle.People), "555")
	if person["retired"] != true {
		t.Errorf("the person: %v, want somebody who has left", person)
	}
}
