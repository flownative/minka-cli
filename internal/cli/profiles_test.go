package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/fakeapi"
	"github.com/flownative/minka-cli/internal/work"
)

// profileNamedAfter is the name a sign-in without --profile gives the first
// profile on a host.
func profileNamedAfter(host string) string {
	return (&config.File{}).ProfileNameForHost(host)
}

func (e *testEnv) config() *config.File {
	e.t.Helper()
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		e.t.Fatal(err)
	}
	return file
}

// withProfiles signs in with two accounts on the host of the test, "work" as
// the person and "agent" as the agent, and with "familie" on another host,
// with nothing in the environment. "work" is the default.
func withProfiles(t *testing.T) (*testEnv, *fakeapi.Server, *fakeapi.Fixture) {
	t.Helper()
	e := newTestEnv(t)
	other, otherFixture := fakeapi.New(t)
	delete(e.vars, "MINKA_TOKEN")
	delete(e.vars, "MINKA_HOST")
	expectCode(t, e.run(e.fixture.Token, "auth", "login", "--profile", "work", "--host", e.server.URL, "--with-token"), ExitOK)
	expectCode(t, e.run(e.fixture.AgentToken, "auth", "login", "--profile", "agent", "--host", e.server.URL, "--with-token"), ExitOK)
	expectCode(t, e.run(otherFixture.AgentToken, "auth", "login", "--profile", "familie", "--host", other.URL, "--with-token"), ExitOK)
	return e, other, otherFixture
}

// lastToken is the token the last request to a server carried.
func lastToken(t *testing.T, server *fakeapi.Server) string {
	t.Helper()
	requests := server.Requests()
	if len(requests) == 0 {
		t.Fatal("no request reached the server")
	}
	return strings.TrimPrefix(requests[len(requests)-1].Header.Get("Authorization"), "Bearer ")
}

func TestAProfileIsChosenByFlagEnvironmentOrDefault(t *testing.T) {
	e, other, otherFixture := withProfiles(t)
	if file := e.config(); file.DefaultProfile != "work" || len(file.Profiles) != 3 {
		t.Fatalf("config %+v", file)
	}

	cases := []struct {
		name   string
		args   []string
		env    map[string]string
		server *fakeapi.Server
		token  string
		source string
	}{
		{"default", nil, nil, e.server, e.fixture.Token, chosenByDefault},
		{"--profile", []string{"--profile", "agent"}, nil, e.server, e.fixture.AgentToken, chosenByProfileFlag},
		{"$MINKA_PROFILE", nil, map[string]string{"MINKA_PROFILE": "familie"}, other, otherFixture.AgentToken, chosenByProfileEnv},
		{"--profile over $MINKA_PROFILE", []string{"--profile", "agent"}, map[string]string{"MINKA_PROFILE": "familie"}, e.server, e.fixture.AgentToken, chosenByProfileFlag},
		{"--host over $MINKA_PROFILE", []string{"--host", other.URL}, map[string]string{"MINKA_PROFILE": "agent"}, other, otherFixture.AgentToken, chosenByHostFlag},
		{"--profile over $MINKA_HOST", []string{"--profile", "familie"}, map[string]string{"MINKA_HOST": e.server.URL}, other, otherFixture.AgentToken, chosenByProfileFlag},
		{"$MINKA_PROFILE over $MINKA_HOST", nil, map[string]string{"MINKA_PROFILE": "familie", "MINKA_HOST": e.server.URL}, other, otherFixture.AgentToken, chosenByProfileEnv},
		// Of the two profiles on the host, the default one.
		{"$MINKA_HOST", nil, map[string]string{"MINKA_HOST": e.server.URL}, e.server, e.fixture.Token, chosenByHostEnv},
	}
	for _, c := range cases {
		for name, value := range c.env {
			e.vars[name] = value
		}
		env, r := e.runJSON("", append([]string{"auth", "status"}, c.args...)...)
		for name := range c.env {
			delete(e.vars, name)
		}
		expectCode(t, r, ExitOK)
		if got := lastToken(t, c.server); got != c.token {
			t.Errorf("%s: the request carried %q, want %q", c.name, got, c.token)
		}
		if data := decode(t, env.Data); data["profileSource"] != c.source || data["defaultProfile"] != "work" {
			t.Errorf("%s: data %s", c.name, env.Data)
		}
	}
}

func TestBothFlagsAndAnUnknownOrMisspeltProfileAreRefused(t *testing.T) {
	e, other, _ := withProfiles(t)

	_, r := e.runJSON("", "me", "--profile", "work", "--host", other.URL)
	expectCode(t, r, ExitUsage)

	env, r := e.runJSON("", "me", "--profile", "nobody")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "agent, familie, work") {
		t.Errorf("the hint does not list the profiles: %q", env.Hint)
	}

	e.vars["MINKA_PROFILE"] = "Not A Name"
	env, r = e.runJSON("", "me")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "$MINKA_PROFILE") {
		t.Errorf("error %q", env.Error)
	}
	if len(e.server.Requests())+len(other.Requests()) != 3 {
		t.Errorf("a refused choice sent a request")
	}
}

// A host with several profiles stands for the default one only; without it,
// which account writes is not to be guessed.
func TestAHostWithSeveralProfilesAndNoDefaultAmongThemIsAmbiguous(t *testing.T) {
	e, other, _ := withProfiles(t)
	expectCode(t, e.run("", "profiles", "default", "familie"), ExitOK)
	before := len(e.server.Requests())

	env, r := e.runJSON("", "me", "--host", e.server.URL)
	expectCode(t, r, ExitAmbiguous)
	if !strings.Contains(env.Error, "agent, work") || !strings.Contains(env.Hint, "--profile") {
		t.Errorf("error %q, hint %q", env.Error, env.Hint)
	}
	if len(e.server.Requests()) != before {
		t.Error("a request was sent")
	}

	// A sign-in and the default by host refuse alike.
	_, r = e.runJSON(e.fixture.Token, "auth", "login", "--host", e.server.URL, "--with-token")
	expectCode(t, r, ExitAmbiguous)
	_, r = e.runJSON("", "config", "set", "host", e.server.URL)
	expectCode(t, r, ExitAmbiguous)

	// The one profile on a host is that host's.
	expectCode(t, e.run("", "me", "--host", other.URL, "--json"), ExitOK)
}

// $MINKA_TOKEN belongs to $MINKA_HOST and to no profile: a profile named
// signs in with its own token, even on that very host.
func TestTheEnvironmentTokenGoesToNoProfile(t *testing.T) {
	e, _, _ := withProfiles(t)
	e.vars["MINKA_HOST"] = e.server.URL
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken

	env, r := e.runJSON("", "auth", "status")
	expectCode(t, r, ExitOK)
	if got := lastToken(t, e.server); got != e.fixture.StrangerToken {
		t.Errorf("the token of $MINKA_HOST was not used for it: %q", got)
	}
	if data := decode(t, env.Data); data["profile"] != nil || data["tokenSource"] != tokenFromEnvironment {
		t.Errorf("data %s", env.Data)
	}

	for _, choose := range []func(){
		func() { e.vars["MINKA_PROFILE"] = "agent" },
		func() { delete(e.vars, "MINKA_PROFILE") },
	} {
		choose()
		args := []string{"auth", "status"}
		if e.vars["MINKA_PROFILE"] == "" {
			args = append(args, "--profile", "agent")
		}
		env, r = e.runJSON("", args...)
		expectCode(t, r, ExitOK)
		if got := lastToken(t, e.server); got != e.fixture.AgentToken {
			t.Errorf("the profile signed in with %q, want its own token", got)
		}
		if data := decode(t, env.Data); data["profile"] != "agent" {
			t.Errorf("data %s", env.Data)
		}
		if !strings.Contains(env.Notice, "$MINKA_TOKEN was not used") {
			t.Errorf("nothing says the token of the environment was passed over: %q", env.Notice)
		}
	}
}

// A sign-out by host follows the choice of every command: of two profiles on
// the host the default one, and none where the default is elsewhere. It never
// takes, nor revokes, one of them by chance.
func TestASignOutByHostOfSeveralProfilesTakesNoneByChance(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		e, _, _ := withProfiles(t)
		args := []string{"auth", "logout", "--host", e.server.URL}
		if revoke {
			args = append(args, "--revoke")
		}
		expectCode(t, e.run("", "profiles", "default", "familie"), ExitOK)

		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitAmbiguous)
		if file := e.config(); len(file.Profiles) != 3 {
			t.Errorf("revoke %v: a profile was removed: %v", revoke, file.ProfileNames())
		}
		if e.revocations() != 0 {
			t.Errorf("revoke %v: a token was revoked", revoke)
		}

		expectCode(t, e.run("", "profiles", "default", "work"), ExitOK)
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitOK)
		if data := decode(t, env.Data); data["profile"] != "work" {
			t.Errorf("revoke %v: data %s", revoke, env.Data)
		}
		if _, left := e.config().Profiles["agent"]; !left {
			t.Errorf("revoke %v: the other profile on the host was removed", revoke)
		}
		if revoke {
			revocations := 0
			for _, request := range e.server.WritingRequests() {
				if request.Method == http.MethodDelete && request.Path == "/api/v1/me/token" {
					revocations++
					if got := request.Header.Get("Authorization"); got != "Bearer "+e.fixture.Token {
						t.Errorf("the revocation carried %q", got)
					}
				}
			}
			if revocations != 1 {
				t.Errorf("%d revocations", revocations)
			}
		}
	}
}

// The cards of a profile go with it. A later profile of the same name, on
// another host, must not report them there with its token.
func TestARemovedProfileLeavesNoCardsBehind(t *testing.T) {
	e, other, otherFixture := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--profile", "agent", "--json"), ExitOK)

	expectCode(t, e.run("", "profiles", "remove", "agent", "--json"), ExitOK)
	if _, left := e.storedHosts()["agent"]; left {
		t.Fatalf("the cards of the removed profile are still in the store: %v", e.storedHosts())
	}

	expectCode(t, e.run(otherFixture.AgentToken, "auth", "login", "--profile", "agent", "--host", other.URL, "--with-token"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
	for _, request := range other.Requests() {
		if request.Method == http.MethodPut && request.Path == "/api/v1/me/work" && strings.Contains(string(request.Body), e.fixture.CardInDoing.ID) {
			t.Error("a card of the removed profile was reported to the other host")
		}
	}
}

// A rename onto a name whose lists outlived their profile takes none of them.
func TestARenameMixesNoListsLeftOver(t *testing.T) {
	e, _, _ := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--profile", "agent", "--json"), ExitOK)
	handle, err := work.Open(e.storePath(), waitForTheStore)
	if err != nil {
		t.Fatal(err)
	}
	handle.File.Account("bot").List("a-session").Start(work.Card{ID: "left-over", StartedAt: time.Now()})
	if err := handle.Save(); err != nil {
		t.Fatal(err)
	}
	handle.Close()

	expectCode(t, e.run("", "profiles", "rename", "agent", "bot", "--json"), ExitOK)
	env, r := e.runJSON("", "work", "list", "--profile", "bot")
	expectCode(t, r, ExitOK)
	cards := decode(t, env.Data)["cards"].([]any)
	if len(cards) != 1 || cards[0].(map[string]any)["id"] != e.fixture.CardInDoing.ID {
		t.Errorf("cards %v", cards)
	}
}

// An older minka reads the store per host, so the store keeps per host the
// lists of the profile that host carries in the configuration.
func TestTheStoreKeepsTheHostsForOlderVersions(t *testing.T) {
	e, _, _ := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	content, err := os.ReadFile(e.storePath())
	if err != nil {
		t.Fatal(err)
	}
	var older struct {
		Hosts map[string]*work.Account `json:"hosts"`
	}
	if err := json.Unmarshal(content, &older); err != nil {
		t.Fatal(err)
	}
	list, held := older.Hosts[e.server.URL].Sessions["a-session"]
	if !held || len(list.Cards) != 1 || list.Cards[0].ID != e.fixture.CardInDoing.ID {
		t.Errorf("the host does not carry the card of its profile: %s", content)
	}
}

// A host that cannot be read finds the profile written letter for letter like
// it, and of two such profiles neither.
func TestAProfileWrittenByHandIsSignedOutOfByItsHost(t *testing.T) {
	const written = "https://minka.example.com;t=x"
	e := newTestEnv(t)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"profiles": {
		"one": {"host": "` + written + `", "token": "minka_pat_one"},
		"two": {"host": "` + written + `", "token": "minka_pat_two"}}}`)

	_, r := e.runJSON("", "auth", "logout", "--host", written)
	expectCode(t, r, ExitAmbiguous)
	if len(e.config().Profiles) != 2 {
		t.Fatal("a profile was removed")
	}

	expectCode(t, e.run("", "profiles", "remove", "two", "--json"), ExitOK)
	env, r := e.runJSON("", "auth", "logout", "--host", written)
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["profile"] != "one" {
		t.Errorf("data %s", env.Data)
	}
}

// hostsOfTheStore is what the store writes per host for an older version.
func (e *testEnv) hostsOfTheStore() map[string]*work.Account {
	e.t.Helper()
	content, err := os.ReadFile(e.storePath())
	if err != nil {
		e.t.Fatal(err)
	}
	var older struct {
		Hosts map[string]*work.Account `json:"hosts"`
	}
	if err := json.Unmarshal(content, &older); err != nil {
		e.t.Fatal(err)
	}
	return older.Hosts
}

// What the store writes per host is a copy of the profile the host stood for
// then. Once another profile on the host is the default, that copy is still
// no card of the new one.
func TestACopyForOlderVersionsGoesToNoOtherProfile(t *testing.T) {
	e, _, _ := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	expectCode(t, e.run("", "profiles", "default", "agent"), ExitOK)
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	if _, copied := e.hostsOfTheStore()[e.server.URL]; !copied {
		t.Fatal("the store keeps no copy for the host")
	}

	expectCode(t, e.run("", "profiles", "default", "work"), ExitOK)
	env, r := e.runJSON("", "work", "list", "--profile", "work")
	expectCode(t, r, ExitOK)
	if cards := decode(t, env.Data)["cards"].([]any); len(cards) != 0 {
		t.Errorf("the card of agent is listed under work: %s", env.Data)
	}
}

// A rename writes the copy for older versions under the new name at once.
func TestARenameKeepsTheCopyForOlderVersions(t *testing.T) {
	e, _, _ := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "profiles", "rename", "work", "office", "--json"), ExitOK)

	list, held := e.hostsOfTheStore()[e.server.URL].Sessions["a-session"]
	if !held || len(list.Cards) != 1 || list.Cards[0].ID != e.fixture.CardInDoing.ID {
		t.Errorf("no copy for the host after the rename: %v", e.hostsOfTheStore())
	}
}

// storeFails makes every opening of the store fail, as one held by another
// minka for longer than a command waits.
func storeFails(a *app) {
	a.openTheStore = func(string, time.Duration, work.Accounts) (*work.Handle, error) {
		return nil, work.ErrBusy
	}
}

// reportedTo says whether a signal to a server named a card.
func reportedTo(server *fakeapi.Server, cardID string) bool {
	for _, request := range server.Requests() {
		if request.Method == http.MethodPut && request.Path == "/api/v1/me/work" && strings.Contains(string(request.Body), cardID) {
			return true
		}
	}
	return false
}

// Where the store could not follow a rename or a removal, the lists stay under
// a name no profile has. Nothing reports them, and a new profile of that name
// starts without them.
func TestListsUnderANameNoProfileHasAreDead(t *testing.T) {
	for _, how := range []string{"rename", "remove"} {
		e, other, otherFixture := withProfiles(t)
		e.vars[sessionVariable] = "a-session"
		expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--profile", "agent", "--json"), ExitOK)

		args := []string{"profiles", "remove", "agent", "--json"}
		if how == "rename" {
			args = []string{"profiles", "rename", "agent", "bot", "--json"}
		}
		r := e.runWith(storeFails, "", args...)
		expectCode(t, r, ExitOK)
		if _, left := e.storedHosts()["agent"]; !left {
			t.Fatalf("%s: the failure was not forced", how)
		}

		expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
		if reportedTo(e.server, e.fixture.CardInDoing.ID) {
			t.Errorf("%s: the dead list was reported", how)
		}

		expectCode(t, e.run(otherFixture.AgentToken, "auth", "login", "--profile", "agent", "--host", other.URL, "--with-token"), ExitOK)
		expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
		if reportedTo(other, e.fixture.CardInDoing.ID) {
			t.Errorf("%s: the card of the old profile was reported to the other host", how)
		}
	}
}

// A new profile clears whatever the store holds under its name before it is
// saved, and a store that cannot be cleared refuses the sign-in.
func TestANewProfileStartsWithoutCards(t *testing.T) {
	e, other, otherFixture := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	handle, err := work.Open(e.storePath(), waitForTheStore)
	if err != nil {
		t.Fatal(err)
	}
	handle.File.Account("ghost").List("a-session").Start(work.Card{ID: e.fixture.CardInDoing.ID, StartedAt: time.Now()})
	if err := handle.Save(); err != nil {
		t.Fatal(err)
	}
	handle.Close()

	r := e.runWith(storeFails, otherFixture.AgentToken, "auth", "login", "--profile", "ghost", "--host", other.URL, "--with-token", "--json")
	expectCode(t, r, ExitUsage)
	if _, saved := e.config().Profiles["ghost"]; saved {
		t.Fatal("the sign-in was saved although the store could not be cleared")
	}

	expectCode(t, e.run(otherFixture.AgentToken, "auth", "login", "--profile", "ghost", "--host", other.URL, "--with-token"), ExitOK)
	if _, left := e.storedHosts()["ghost"]; left {
		t.Error("the sign-in left the lists under its name")
	}
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
	if reportedTo(other, e.fixture.CardInDoing.ID) {
		t.Error("a card under the name was reported with the new profile")
	}
}

// The configuration may be read in the middle of a rename, between the file and
// the store, so a name it lacks is no reason to delete the lists under it.
func TestAStoreKeepsTheListsOfANameTheConfigurationLacks(t *testing.T) {
	e, _, _ := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	handle, err := work.Open(e.storePath(), waitForTheStore)
	if err != nil {
		t.Fatal(err)
	}
	handle.File.Account("in-the-middle").List("a-session").Start(work.Card{ID: e.fixture.CardInDoing.ID, StartedAt: time.Now()})
	if err := handle.Save(); err != nil {
		t.Fatal(err)
	}
	handle.Close()

	expectCode(t, e.run("", "work", "start", e.fixture.CardInTriage.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
	if _, kept := e.storedHosts()["in-the-middle"]; !kept {
		t.Error("the store deleted the lists of a name the configuration lacks")
	}
	if reportedTo(e.server, e.fixture.CardInDoing.ID) {
		t.Error("the lists of a name without a profile were reported")
	}
}

func TestProfilesRemoveAndDefaultRefuseANameThatIsNone(t *testing.T) {
	e, _, _ := withProfiles(t)
	for _, command := range []string{"remove", "default"} {
		env, r := e.runJSON("", "profiles", command, "Not/A Name")
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "is no profile name") {
			t.Errorf("%s: error %q", command, env.Error)
		}
	}
}

func TestASignInOfAProfileKeepsItsHost(t *testing.T) {
	e, other, _ := withProfiles(t)

	env, r := e.runJSON(e.fixture.Token, "auth", "login", "--profile", "work", "--host", other.URL, "--with-token")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "minka profiles remove work") {
		t.Errorf("hint %q", env.Hint)
	}

	// Signed in again without a host, it keeps its own and stays the default.
	env, r = e.runJSON(e.fixture.Token, "auth", "login", "--profile", "work", "--with-token")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["host"] != e.server.URL || data["isDefault"] != true || data["profile"] != "work" {
		t.Errorf("data %s", env.Data)
	}

	// A new name needs a host.
	_, r = e.runJSON(e.fixture.Token, "auth", "login", "--profile", "new", "--with-token")
	expectCode(t, r, ExitUsage)
}

func TestProfilesListNamesTheDefaultAndTheActiveOne(t *testing.T) {
	e, _, _ := withProfiles(t)
	e.vars["MINKA_PROFILE"] = "familie"

	env, r := e.runJSON("", "profiles", "list")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	rows := data["profiles"].([]any)
	if len(rows) != 3 || data["defaultProfile"] != "work" {
		t.Fatalf("data %s", env.Data)
	}
	active := data["active"].(map[string]any)
	if active["profile"] != "familie" || active["source"] != chosenByProfileEnv {
		t.Errorf("active %v", active)
	}
	for _, row := range rows {
		profile := row.(map[string]any)
		if (profile["name"] == "work") != (profile["isDefault"] == true) || (profile["name"] == "familie") != (profile["isActive"] == true) {
			t.Errorf("row %v", profile)
		}
		if profile["name"] == "work" && profile["personName"] != "Bruno" {
			t.Errorf("row %v", profile)
		}
	}
	if strings.Contains(r.stdout, e.fixture.Token) {
		t.Error("the list carries a token")
	}
}

// A rename carries the default and the cards a session holds with the profile.
func TestARenameCarriesTheDefaultAndTheCards(t *testing.T) {
	e, _, _ := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--profile", "agent", "--json"), ExitOK)
	expectCode(t, e.run("", "profiles", "rename", "agent", "bot", "--json"), ExitOK)
	expectCode(t, e.run("", "profiles", "rename", "work", "office", "--json"), ExitOK)

	if file := e.config(); file.DefaultProfile != "office" || file.Profiles["bot"].Token != e.fixture.AgentToken {
		t.Errorf("config %+v", file)
	}
	if _, kept := e.storedHosts()["bot"]; !kept {
		t.Errorf("the cards stayed behind: %v", e.storedHosts())
	}
	env, r := e.runJSON("", "work", "list", "--profile", "bot")
	expectCode(t, r, ExitOK)
	if cards := decode(t, env.Data)["cards"].([]any); len(cards) != 1 {
		t.Errorf("data %s", env.Data)
	}

	_, r = e.runJSON("", "profiles", "rename", "bot", "familie")
	expectCode(t, r, ExitUsage)
	_, r = e.runJSON("", "profiles", "rename", "bot", "Bot")
	expectCode(t, r, ExitUsage)
}

// Removing the default profile leaves none, as a sign-out of it does.
func TestRemovingTheDefaultProfileLeavesNoDefault(t *testing.T) {
	e, other, _ := withProfiles(t)

	env, r := e.runJSON("", "profiles", "remove", "work")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "no profile is the default now") {
		t.Errorf("notice %q", env.Notice)
	}
	file := e.config()
	if file.DefaultProfile != "" || len(file.Profiles) != 2 {
		t.Errorf("config %+v", file)
	}

	env, r = e.runJSON("", "me")
	expectCode(t, r, ExitAuth)
	if env.Error != "no default profile is set" {
		t.Errorf("error %q", env.Error)
	}
	// Of the two on the host, one is left, and the host stands for it now.
	expectCode(t, e.run("", "me", "--host", e.server.URL, "--json"), ExitOK)

	expectCode(t, e.run("", "profiles", "default", "familie", "--json"), ExitOK)
	expectCode(t, e.run("", "me", "--json"), ExitOK)
	if got := lastToken(t, other); got == "" {
		t.Error("the default profile was not used")
	}
}

// A card belongs to the account it was taken up with, and the beat of the
// session reports it with that account, whatever the mod's environment names.
func TestTheHeartbeatReportsEveryProfileWithItsOwnToken(t *testing.T) {
	e, other, otherFixture := withProfiles(t)
	e.vars[sessionVariable] = "a-session"
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--profile", "agent", "--json"), ExitOK)
	expectCode(t, e.run("", "work", "start", otherFixture.CardInDoing.ID, "--profile", "familie", "--json"), ExitOK)

	// The mod's environment names the default profile, a person's.
	e.vars["MINKA_PROFILE"] = "work"
	env, r := e.runJSON("", "heartbeat", "--from-mod")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["sent"] != true || len(data["accounts"].([]any)) != 2 {
		t.Fatalf("data %s", env.Data)
	}
	for server, token := range map[*fakeapi.Server]string{e.server: e.fixture.AgentToken, other: otherFixture.AgentToken} {
		signalled := false
		for _, request := range server.Requests() {
			if request.Method == http.MethodPut && request.Path == "/api/v1/me/work" {
				signalled = true
				if got := request.Header.Get("Authorization"); got != "Bearer "+token {
					t.Errorf("a signal carried %q", got)
				}
			}
		}
		if !signalled {
			t.Errorf("no signal reached %s", server.URL)
		}
	}
	if got := e.server.CardsAtWork(); len(got) != 1 || got[0] != e.fixture.CardInDoing.ID {
		t.Errorf("at work here: %v", got)
	}
	if got := other.CardsAtWork(); len(got) != 1 || got[0] != otherFixture.CardInDoing.ID {
		t.Errorf("at work there: %v", got)
	}
}

// A store written per host is handed to the profile its host became, so that a
// card somebody has in hand at the update stays reported.
func TestAStoreKeptPerHostGoesToTheProfileOfTheHost(t *testing.T) {
	e := newAgentEnv(t)
	host := e.server.URL
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost": "` + host + `", "hosts": {"` + host + `": {"token": "` + e.fixture.AgentToken + `"}}}`)
	if err := os.MkdirAll(dirOf(e.storePath()), 0o700); err != nil {
		t.Fatal(err)
	}
	stored := `{"hosts": {"` + host + `": {"sessions": {"": {"cards": [{"id": "` + e.fixture.CardInDoing.ID + `", "startedAt": "2099-01-01T00:00:00Z"}]}}}}}`
	if err := os.WriteFile(e.storePath(), []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "work", "list")
	expectCode(t, r, ExitOK)
	if cards := decode(t, env.Data)["cards"].([]any); len(cards) != 1 {
		t.Errorf("data %s", env.Data)
	}
}
