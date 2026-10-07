package basecamp_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// extractWithAgents is an extraction that writes the given people as agents.
func extractWithAgents(t *testing.T, account *fakebasecamp.Account, agents basecamp.Agents, projects ...string) (string, basecamp.Result) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	writer, err := bundle.Create(dir, basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Agents: agents,
		Now: func() time.Time { return runDay }}
	result, err := extractor.Extract(t.Context(), projects, writer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Commit(bundle.Manifest{SourceInstance: result.SourceInstance}); err != nil {
		t.Fatal(err)
	}
	return dir, result
}

func sourceIDsOf(people []map[string]any) []string {
	ids := make([]string, len(people))
	for i, person := range people {
		ids[i] = person["sourceId"].(string)
	}
	return ids
}

// An agent comes with its kind and its human and without an address, after
// the human, whose ID is the higher one here; everybody else comes as before.
func TestAnAgentComesWithItsHumanAndWithoutAnAddressAfterTheHuman(t *testing.T) {
	dir, result := extractWithAgents(t, fakebasecamp.New(), basecamp.Agents{101: 102}, "7001")
	people := records(t, dir, bundle.People)

	if got := sourceIDsOf(people); !slices.Equal(got, []string{"99", "102", "103", "104", "101"}) {
		t.Errorf("people in the order %v, want the agent last", got)
	}
	want := map[string]any{"sourceId": "101", "createdAt": "2020-01-01T08:00:00.000Z", "updatedAt": "2024-01-01T08:00:00.000Z", "createdBy": nil,
		"name": "Ada Example", "avatar": "avatar-101", "retired": false, "client": false, "kind": "agent", "principal": "102"}
	if agent := recordWithID(t, people, "101"); !reflect.DeepEqual(agent, want) {
		t.Errorf("the agent:\n got %v\nwant %v", agent, want)
	}
	if human := recordWithID(t, people, "102"); human["email"] != "grace@example.test" || human["kind"] != nil {
		t.Errorf("the human: %v", human)
	}
	if result.Agents != 1 || result.Bots != 0 || result.Avatars != 4 {
		t.Errorf("%d agents, %d bots, %d avatars", result.Agents, result.Bots, result.Avatars)
	}
}

// The agent refers to its human, so a run whose records name the agent alone
// writes the human too. An agent no record names stays out, and so does its
// human.
func TestTheHumanOfAnAgentComesWhereNoRecordNamesThem(t *testing.T) {
	dir, result := extractWithAgents(t, fakebasecamp.New(), basecamp.Agents{101: 105}, "7002")
	if got := sourceIDsOf(records(t, dir, bundle.People)); !slices.Equal(got, []string{"105", "101"}) || result.PeopleLeftOut != 10 {
		t.Errorf("wrote %v and left %d out, want the human and the agent and 10 left out", got, result.PeopleLeftOut)
	}

	dir, result = extractWithAgents(t, fakebasecamp.New(), basecamp.Agents{105: 104}, "7002")
	if got := sourceIDsOf(records(t, dir, bundle.People)); !slices.Equal(got, []string{"101"}) || result.PeopleLeftOut != 11 || result.Agents != 0 {
		t.Errorf("wrote %v, left %d out, %d agents: want the one member alone", got, result.PeopleLeftOut, result.Agents)
	}
}

// The address of an agent is not written, so nothing is reported of it.
func TestTheAddressOfAnAgentIsNeitherWrittenNorReported(t *testing.T) {
	for _, address := range []string{`"ada@example.test"`, "no address at all"} {
		account := fakebasecamp.New()
		account.People[1].Email = address // 101
		dir, result := extractWithAgents(t, account, basecamp.Agents{101: 102}, "7001")
		if _, has := recordWithID(t, records(t, dir, bundle.People), "101")["email"]; has {
			t.Errorf("%s: the agent carries an address", address)
		}
		if len(result.InvalidAddresses) != 0 || result.AddressesUnquoted != 0 {
			t.Errorf("%s: the address of an agent is reported: invalid %v, unquoted %d", address, result.InvalidAddresses, result.AddressesUnquoted)
		}
	}
}

// Agent and human are held against the list of the account before anything
// else is asked for, in a dry run as in a real one. Somebody the list does not
// hold is named as such, not as somebody of another kind.
func TestWhomTheAccountHoldsAsNoUserEndsTheRunBeforeAnythingElse(t *testing.T) {
	for name, c := range map[string]struct {
		agents basecamp.Agents
		says   string
	}{
		"an agent not in the list":   {basecamp.Agents{555: 101}, "555 as an agent, who is not in the people list"},
		"somebody who left as agent": {basecamp.Agents{99: 101}, "99 as an agent, who is not in the people list"},
		"a bot as agent":             {basecamp.Agents{801: 101}, "801 as an agent, who is no user"},
		"somebody deleted as agent":  {basecamp.Agents{802: 101}, "802 as an agent, who is no user"},
		"a legacy client as agent":   {basecamp.Agents{107: 101}, "107 as an agent, who is no user"},
		"a client as agent":          {basecamp.Agents{106: 101}, "106 as an agent, who is a client"},
		"a human not in the list":    {basecamp.Agents{101: 555}, "555 as the human of 101, who is not in the people list"},
		"a bot as human":             {basecamp.Agents{101: 801}, "801 as the human of 101, who is no user"},
		"somebody deleted as human":  {basecamp.Agents{101: 802}, "802 as the human of 101, who is no user"},
		"a client as human":          {basecamp.Agents{101: 108}, "108 as the human of 101, who is a client"},
		"a second agent is wrong":    {basecamp.Agents{102: 101, 104: 801}, "801 as the human of 104"},
	} {
		for _, dry := range []bool{false, true} {
			account := fakebasecamp.New()
			parent := t.TempDir()
			writer := bundle.Discard()
			if !dry {
				var err error
				if writer, err = bundle.Create(filepath.Join(parent, "bundle"), basecamp.SourceName); err != nil {
					t.Fatal(err)
				}
			}
			extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Agents: c.agents}
			_, err := extractor.Extract(t.Context(), []string{"7001"}, writer)
			writer.Abort()

			var failed *basecamp.Error
			if !errors.As(err, &failed) || failed.Kind != basecamp.Usage || failed.Hint == "" || !strings.Contains(failed.Message, c.says) {
				t.Fatalf("%s, dry %v: got %v, want a usage error with a hint", name, dry, err)
			}
			for _, request := range account.Requests {
				if !strings.HasPrefix(request, "people.json") {
					t.Errorf("%s, dry %v: went on to ask for %s", name, dry, request)
				}
			}
			if entries, _ := os.ReadDir(parent); len(entries) != 0 {
				t.Errorf("%s, dry %v: left %v", name, dry, entries)
			}
		}
	}
}

// Each refusal says what is wrong: an agent of themselves would otherwise be
// refused as the human of an agent.
func TestAgentsAreReadStrictly(t *testing.T) {
	notAPair := "is not <agent ID>=<human ID>"
	for _, c := range []struct {
		values []string
		says   string
	}{
		{[]string{"101"}, notAPair}, {[]string{"101="}, notAPair}, {[]string{"=102"}, notAPair},
		{[]string{"101=102=103"}, notAPair}, {[]string{"101:102"}, notAPair}, {[]string{" 101=102"}, notAPair},
		{[]string{"101=102 "}, notAPair}, {[]string{"0101=102"}, notAPair}, {[]string{"-101=102"}, notAPair},
		{[]string{"+101=102"}, notAPair}, {[]string{"0=102"}, notAPair}, {[]string{"101=Ada"}, notAPair},
		{[]string{"1234567890123456789=102"}, notAPair},
		{[]string{"101=101"}, "101 as their own agent"},
		{[]string{"101=102", "101=103"}, "101 as the agent of 102 and of 103"},
		{[]string{"101=102", "102=103"}, "102 as the human of 101 and as an agent"},
		{[]string{"102=103", "101=102"}, "102 as the human of 101 and as an agent"},
	} {
		agents, err := basecamp.ParseAgents(c.values)
		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != basecamp.Usage || failed.Hint == "" || !strings.Contains(failed.Message, c.says) {
			t.Errorf("%q: got %v and %v, want a usage error that says %q", c.values, agents, err, c.says)
		}
	}

	agents, err := basecamp.ParseAgents([]string{"101=102", "103=102", "101=102", "123456789012345678=104"})
	want := basecamp.Agents{101: 102, 103: 102, 123456789012345678: 104}
	if err != nil || !reflect.DeepEqual(agents, want) {
		t.Errorf("got %v, %v; want %v", agents, err, want)
	}
}

// Minka gives an agent an address of its own, so an agent whose source has
// none, or none that is one, is put on an item and asked a question like
// anybody with an address.
func TestAnAgentWithoutAnAddressAtTheSourceIsStillFollowingAndAsked(t *testing.T) {
	for _, address := range []string{"", "no address at all"} {
		account := fakebasecamp.New()
		account.People[3].Email = address // 103, a member of both projects
		talk := &account.Projects[3]
		talk.Members = append(talk.Members, 103)
		busyPost(account).Subscribers = []fakebasecamp.Person{account.People[3], account.People[1]}

		dir, result := extractWithAgents(t, account, basecamp.Agents{103: 101}, talkProject, checkinProject)
		if got := recordWithID(t, records(t, dir, bundle.Posts), "9701")["subscribers"]; !reflect.DeepEqual(got, []any{"101", "103"}) {
			t.Errorf("%q: the post is followed by %v, want the agent too", address, got)
		}
		if got := recordWithID(t, records(t, dir, bundle.Questions), "9310")["asked"]; !reflect.DeepEqual(got, []any{"101", "102", "103", "104"}) {
			t.Errorf("%q: asked %v, want the agent too", address, got)
		}
		if result.Subscribers.LeftOff.WithoutAnAddress != 0 || result.AskedLeftOut.WithoutAnAddress != 0 {
			t.Errorf("%q: an agent is left off for its address: %+v, %+v", address, result.Subscribers.LeftOff, result.AskedLeftOut)
		}
	}
}

// A human whose times Basecamp does not say may come in only where the people
// are written, so they are asked for with the list.
func TestAnAgentOrHumanWithoutTimesEndsTheRunBeforeAnythingElse(t *testing.T) {
	for name, agents := range map[string]basecamp.Agents{"agent": {105: 101}, "human": {101: 105}} {
		account := fakebasecamp.New()
		account.People[4].CreatedAt = "" // 105
		extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Agents: agents}
		_, err := extractor.Extract(t.Context(), []string{"7002"}, bundle.Discard())
		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != basecamp.API || !strings.Contains(failed.Message, "person 105") {
			t.Fatalf("%s: got %v, want an api_error naming person 105", name, err)
		}
		for _, request := range account.Requests {
			if !strings.HasPrefix(request, "people.json") {
				t.Errorf("%s: went on to ask for %s", name, request)
			}
		}
	}
}
