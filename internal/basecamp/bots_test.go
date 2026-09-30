package basecamp_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// personOf is the person of the account's list with the given ID, as a record
// names them.
func personOf(t *testing.T, account *fakebasecamp.Account, id int64) *fakebasecamp.Person {
	t.Helper()
	for i := range account.People {
		if account.People[i].ID == id {
			return &account.People[i]
		}
	}
	t.Fatalf("person %d is not in the list of the account", id)
	return nil
}

// lineAt is a line of the quiet room, written at the given minute past ten.
func lineAt(id int64, kind string, creator *fakebasecamp.Person, minute int, content string) fakebasecamp.Line {
	at := fmt.Sprintf("2023-04-20T10:%02d:00.000Z", minute)
	return fakebasecamp.Line{ID: id, Type: kind, Creator: creator, CreatedAt: at, UpdatedAt: at, Content: content}
}

// linesOfIntegrations puts into the quiet room a line of the deleted chatbot,
// and lines of an integration that a user, somebody of no kind and somebody of
// the legacy kind of client wrote.
func linesOfIntegrations(t *testing.T, account *fakebasecamp.Account) {
	t.Helper()
	kindless := fakebasecamp.Person{ID: 804, Name: "Nobody Kind", NoKind: true, Email: "kindless@example.test",
		CreatedAt: "2020-07-01T08:00:00.000Z", UpdatedAt: "2023-07-01T08:00:00.000Z"}
	legacyClient := fakebasecamp.Person{ID: 805, Name: "Old Client", Kind: "Client", Email: "old-client@example.test",
		CreatedAt: "2020-08-01T08:00:00.000Z", UpdatedAt: "2023-08-01T08:00:00.000Z"}
	quiet(account).Lines = []fakebasecamp.Line{
		lineAt(9821, fakebasecamp.IntegrationLine, &account.DeletedBot, 1, `<b>Build 41</b> failed`),
		lineAt(9822, fakebasecamp.IntegrationLine, personOf(t, account, 101), 2, `Deployed by hand`),
		lineAt(9823, fakebasecamp.IntegrationLine, &kindless, 3, `Deployed by nobody`),
		lineAt(9824, fakebasecamp.IntegrationLine, &legacyClient, 4, `Deployed by a client`),
	}
}

// A chatbot comes as a bot, the one Basecamp still has and the one it deleted
// alike, whether the list of the account holds the deleted one or not. A bot
// carries no address and no word about being a client, and only the deleted one
// says it is retired. Somebody deleted who wrote no line of an integration
// stays a person, and so does a user, a client of the legacy kind and somebody
// of no kind, whatever line names them.
func TestChatbotsAndIntegrationsComeAsBots(t *testing.T) {
	for view, change := range map[string]func(*fakebasecamp.Account){
		"in no list":            func(*fakebasecamp.Account) {},
		"in the account's list": func(a *fakebasecamp.Account) { a.People = append(a.People, a.DeletedBot) },
	} {
		account := fakebasecamp.New()
		change(account)
		linesOfIntegrations(t, account)
		dir, result := extract(t, account, talkProject)
		people := records(t, dir, bundle.People)

		for id, name := range map[string]string{"801": "Build Robot", "803": "xbuildrobot"} {
			bot := recordWithID(t, people, id)
			want := map[string]any{"sourceId": id, "createdAt": bot["createdAt"], "updatedAt": bot["updatedAt"], "createdBy": nil,
				"name": name, "avatar": "avatar-" + id, "kind": "bot"}
			if id == "803" {
				want["retired"] = true
			}
			if !reflect.DeepEqual(bot, want) {
				t.Errorf("%s: the bot %s:\n got %v\nwant %v", view, id, bot, want)
			}
		}
		for _, id := range []string{"101", "802", "804", "805"} {
			person := recordWithID(t, people, id)
			if _, has := person["kind"]; has || person["email"] == nil {
				t.Errorf("%s: person %s comes as %v", view, id, person)
			}
		}
		if deleted := recordWithID(t, people, "802"); deleted["retired"] != true {
			t.Errorf("%s: somebody deleted comes in service: %v", view, deleted)
		}
		if result.Bots != 2 || len(result.BotsAlsoNamedElsewhere) != 0 {
			t.Errorf("%s: %d bots counted, want 2; named as people too: %v", view, result.Bots, result.BotsAlsoNamedElsewhere)
		}
		if got := sourceIDs(messages(t, dir)); !slices.Contains(got, "9821") || !slices.Contains(got, "9824") {
			t.Errorf("%s: the lines of integrations did not come: %v", view, got)
		}
	}
}

// What Basecamp keeps of a chatbot's address is never written, so nothing is
// reported of it either; and a bot is never a client, whatever Basecamp says,
// so a company a bundle could not carry does not end the run on a bot.
func TestABotIsNoClientAndCarriesNoAddressWhateverTheSourceSays(t *testing.T) {
	for _, address := range []string{`"buildrobot@example.test"`, "no address at all"} {
		account := fakebasecamp.New()
		account.DeletedBot.Email = address
		account.DeletedBot.Client, account.DeletedBot.Company = true, &fakebasecamp.Company{ID: 9284, Name: "  "}
		linesOfIntegrations(t, account)
		bot := personOf(t, account, 801)
		bot.Client, bot.Company = true, &fakebasecamp.Company{ID: 9290, Name: "Robots Inc."}
		*campfire(account).Lines[3].Creator = *bot

		dir, result := extract(t, account, talkProject)
		people := records(t, dir, bundle.People)
		for _, id := range []string{"801", "803"} {
			person := recordWithID(t, people, id)
			for _, field := range []string{"email", "client", "company"} {
				if value, has := person[field]; has {
					t.Errorf("%s: the bot %s carries %s %v", address, id, field, value)
				}
			}
		}
		if len(result.InvalidAddresses) != 0 || result.AddressesUnquoted != 0 {
			t.Errorf("%s: the address of a bot is reported: invalid %v, unquoted %d",
				address, result.InvalidAddresses, result.AddressesUnquoted)
		}
		// The legacy client of the quiet room is the one client.
		if result.PeopleAsClients != 1 {
			t.Errorf("%d clients counted, want the one who is no bot", result.PeopleAsClients)
		}
	}
}

// Somebody deleted who wrote as a person and as an integration comes as a bot,
// which Minka never turns back into a human: the report names them. A deleted
// chatbot that wrote nothing but lines of an integration, however many, is not
// named.
func TestADeletedPersonWhoAlsoWroteAsAnIntegrationIsNamed(t *testing.T) {
	account := fakebasecamp.New()
	linesOfIntegrations(t, account)
	quiet(account).Lines = append(quiet(account).Lines,
		lineAt(9825, fakebasecamp.IntegrationLine, &account.DeletedBot, 5, `<b>Build 42</b> passed`),
		lineAt(9826, fakebasecamp.IntegrationLine, personOf(t, account, 802), 6, `Deployed on the way out`))
	dir, result := extract(t, account, talkProject)

	if !slices.Equal(result.BotsAlsoNamedElsewhere, []string{"802"}) {
		t.Errorf("bots named as people too: %v, want the deleted person who wrote a text", result.BotsAlsoNamedElsewhere)
	}
	if person := recordWithID(t, records(t, dir, bundle.People), "802"); person["kind"] != "bot" {
		t.Errorf("the deleted person who wrote as an integration comes as %v", person)
	}
}

// A line of an integration that says nothing is no line of the bundle, and it
// makes nobody a bot.
func TestALineOfAnIntegrationThatSaysNothingMakesNoBot(t *testing.T) {
	account := fakebasecamp.New()
	quiet(account).Lines = []fakebasecamp.Line{
		lineAt(9821, fakebasecamp.TextLine, &account.DeletedBot, 1, `Hello`),
		lineAt(9822, fakebasecamp.IntegrationLine, &account.DeletedBot, 2, ` `),
	}
	dir, result := extract(t, account, talkProject)

	person := recordWithID(t, records(t, dir, bundle.People), "803")
	if _, has := person["kind"]; has || person["email"] == nil || person["retired"] != true {
		t.Errorf("the deleted person comes as %v", person)
	}
	if result.Bots != 1 {
		t.Errorf("%d bots counted, want the one chatbot of the account", result.Bots)
	}
}
