package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// signedInToTwoHosts signs in to the host of the test and then to another one,
// with nothing in the environment, so that the default host is the one of the
// configuration.
func signedInToTwoHosts(t *testing.T, loginFlags ...string) (*testEnv, *fakeapi.Server) {
	t.Helper()
	e := newTestEnv(t)
	other, otherFixture := fakeapi.New(t)
	delete(e.vars, "MINKA_TOKEN")
	delete(e.vars, "MINKA_HOST")
	expectCode(t, e.run(e.fixture.Token, "auth", "login", "--host", e.server.URL, "--with-token"), ExitOK)
	args := append([]string{"auth", "login", "--host", other.URL, "--with-token"}, loginFlags...)
	expectCode(t, e.run(otherFixture.Token, args...), ExitOK)
	return e, other
}

func (e *testEnv) defaultHost() string {
	e.t.Helper()
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		e.t.Fatal(err)
	}
	return file.DefaultHost
}

// On a fresh machine there is no default to keep, and the first host signed in
// to becomes it.
func TestTheFirstLoginSetsTheDefaultHost(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")

	env, r := e.runJSON(e.fixture.Token, "auth", "login", "--with-token")
	expectCode(t, r, ExitOK)
	if e.defaultHost() != e.server.URL {
		t.Errorf("default host %q, want %q", e.defaultHost(), e.server.URL)
	}
	if data := decode(t, env.Data); data["isDefault"] != true || data["defaultHost"] != e.server.URL {
		t.Errorf("data %s", env.Data)
	}
}

// A sign-in to another host leaves the default where it is: every command
// without --host, those of other sessions on the machine included, still goes
// there, and the answer says so and how to move it.
func TestASecondLoginKeepsTheDefaultHost(t *testing.T) {
	e, other := signedInToTwoHosts(t)
	if e.defaultHost() != e.server.URL {
		t.Fatalf("default host %q, want %q", e.defaultHost(), e.server.URL)
	}

	env, r := e.runJSON("minka_pat_bruno", "auth", "login", "--host", other.URL, "--with-token")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["isDefault"] != false || data["defaultHost"] != e.server.URL || data["host"] != other.URL {
		t.Errorf("data %s", env.Data)
	}
	if !strings.Contains(env.Summary, "default host stays "+e.server.URL) {
		t.Errorf("summary %q", env.Summary)
	}
	found := false
	for _, b := range env.Breadcrumbs {
		found = found || b.Cmd == "minka config set host "+other.URL
	}
	if !found {
		t.Errorf("no breadcrumb that makes %s the default: %+v", other.URL, env.Breadcrumbs)
	}

	r = e.run("minka_pat_bruno", "auth", "login", "--host", other.URL, "--with-token", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "The default host stays "+e.server.URL) || !strings.Contains(r.stdout, "minka config set host "+other.URL) {
		t.Errorf("the text does not say that the default stays:\n%s", r.stdout)
	}

	// The command without --host goes to the default host, not to the one
	// signed in to last.
	before := len(other.Requests())
	expectCode(t, e.run("", "me", "--json"), ExitOK)
	if len(other.Requests()) != before {
		t.Error("a command without --host went to the host signed in to last")
	}
}

func TestMakeDefaultMovesTheDefaultHost(t *testing.T) {
	e, other := signedInToTwoHosts(t, "--make-default")
	if e.defaultHost() != other.URL {
		t.Errorf("default host %q, want %q", e.defaultHost(), other.URL)
	}
}

// A default host no command can read is no host anything reaches, so a sign-in
// takes its place, and the hint that names the sign-in as the way out holds.
func TestALoginReplacesADefaultHostThatCannotBeRead(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost":"https://minka.example.com;t=x","hosts":{}}`)

	expectCode(t, e.run(e.fixture.Token, "auth", "login", "--with-token"), ExitOK)
	if e.defaultHost() != e.server.URL {
		t.Errorf("default host %q, want %q", e.defaultHost(), e.server.URL)
	}
}

// Only a host with a token can become the default: one without would make every
// later command without --host fail, far from the command that caused it.
func TestConfigSetHostTakesOnlyAHostSignedInTo(t *testing.T) {
	e, other := signedInToTwoHosts(t)

	env, r := e.runJSON("", "config", "set", "host", "https://minka.example.com")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeUsage || !strings.Contains(env.Hint, "minka auth login --host https://minka.example.com") {
		t.Errorf("code %q, hint %q", env.Code, env.Hint)
	}
	if e.defaultHost() != e.server.URL {
		t.Errorf("the refused host was written: default host %q", e.defaultHost())
	}

	// A host is read the way a sign-in reads it.
	env, r = e.runJSON("", "config", "set", "host", other.URL+"/api/v1/")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["value"] != other.URL {
		t.Errorf("data %s", env.Data)
	}
	if e.defaultHost() != other.URL {
		t.Errorf("default host %q, want %q", e.defaultHost(), other.URL)
	}
	env, r = e.runJSON("", "config", "get", "host")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["key"] != "host" || data["value"] != other.URL {
		t.Errorf("data %s", env.Data)
	}
}

func TestConfigGetHostWithoutADefaultSaysNotSet(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "config", "get", "host")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["value"] != nil {
		t.Errorf("data %s", env.Data)
	}
	if r := e.run("", "config", "get", "host", "--styled"); r.stdout != "not set\n" {
		t.Errorf("text %q", r.stdout)
	}
}

// Signing out of the default host leaves no default, rather than one picked
// among the hosts left over, and both the sign-out and the next command say so.
func TestLogoutOfTheDefaultHostLeavesNoDefault(t *testing.T) {
	e, other := signedInToTwoHosts(t)

	env, r := e.runJSON("", "auth", "logout")
	expectCode(t, r, ExitOK)
	if e.defaultHost() != "" {
		t.Errorf("default host %q, want none", e.defaultHost())
	}
	if !strings.Contains(env.Notice, "no host is the default now") || !strings.Contains(env.Notice, "minka config set host") {
		t.Errorf("notice %q", env.Notice)
	}

	before := len(other.Requests())
	env, r = e.runJSON("", "me")
	expectCode(t, r, ExitAuth)
	if env.Error != "no default host is set" || !strings.Contains(env.Hint, "minka config set host") {
		t.Errorf("error %q, hint %q", env.Error, env.Hint)
	}
	if len(other.Requests()) != before {
		t.Error("a command without --host went to a host left over")
	}

	// The next sign-in finds no default, and sets it.
	expectCode(t, e.run("minka_pat_bruno", "auth", "login", "--host", other.URL, "--with-token"), ExitOK)
	if e.defaultHost() != other.URL {
		t.Errorf("default host %q, want %q", e.defaultHost(), other.URL)
	}
}

func TestStatusNamesTheDefaultHostWhereTheHostOfTheCommandDiffers(t *testing.T) {
	e, other := signedInToTwoHosts(t)

	env, r := e.runJSON("", "auth", "status", "--host", other.URL)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["host"] != other.URL || data["defaultHost"] != e.server.URL || data["hostSource"] != hostFromFlag {
		t.Errorf("data %s", env.Data)
	}
	r = e.run("", "auth", "status", "--host", other.URL, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Signed in to "+other.URL+" ") || !strings.Contains(r.stdout, "the default host is "+e.server.URL+".") {
		t.Errorf("the text does not name both hosts:\n%s", r.stdout)
	}

	e.vars["MINKA_HOST"] = other.URL
	env, r = e.runJSON("", "auth", "status")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["hostSource"] != hostFromEnvironment || data["defaultHost"] != e.server.URL {
		t.Errorf("data %s", env.Data)
	}
	if !strings.Contains(env.Summary, "$MINKA_HOST") {
		t.Errorf("summary %q", env.Summary)
	}
}

func TestStatusNamesTheHostOnceWhereItIsTheDefault(t *testing.T) {
	e, _ := signedInToTwoHosts(t)

	env, r := e.runJSON("", "auth", "status")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["host"] != e.server.URL || data["defaultHost"] != e.server.URL || data["hostSource"] != hostFromDefault {
		t.Errorf("data %s", env.Data)
	}
	r = e.run("", "auth", "status", "--styled")
	expectCode(t, r, ExitOK)
	if strings.Count(r.stdout, e.server.URL) != 1 || !strings.Contains(r.stdout, "It is the default host.") {
		t.Errorf("the text names the host other than once:\n%s", r.stdout)
	}
}

func TestDoctorNamesTheDefaultHostWhereTheHostOfTheCommandDiffers(t *testing.T) {
	e, other := signedInToTwoHosts(t)

	e.vars["MINKA_HOST"] = other.URL
	report, _ := e.doctor()
	if report.Host == nil || *report.Host != other.URL || report.DefaultHost == nil || *report.DefaultHost != e.server.URL {
		t.Errorf("host %v, default host %v", report.Host, report.DefaultHost)
	}
	if detail := report.check(t, "sign-in").Detail; !strings.Contains(detail, other.URL) || !strings.Contains(detail, "the default host is "+e.server.URL) {
		t.Errorf("sign-in detail %q", detail)
	}

	delete(e.vars, "MINKA_HOST")
	report, _ = e.doctor()
	if report.Host == nil || *report.Host != e.server.URL || report.DefaultHost == nil || *report.DefaultHost != e.server.URL {
		t.Errorf("host %v, default host %v", report.Host, report.DefaultHost)
	}
	if detail := report.check(t, "sign-in").Detail; strings.Count(detail, e.server.URL) != 1 || !strings.Contains(detail, "it is the default host") {
		t.Errorf("sign-in detail %q", detail)
	}
}

// A default host that cannot be read is no missing default: every command
// without --host fails on it, and a report that called it none would hide that.
func TestStatusAndDoctorSayThatTheDefaultHostCannotBeRead(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	delete(e.vars, "MINKA_HOST")
	e.writeConfig(`{"defaultHost":"https://minka.example.com;t=x","hosts":{"` + e.server.URL + `":{"token":"` + e.fixture.Token + `"}}}`)

	env, r := e.runJSON("", "auth", "status", "--host", e.server.URL)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "the default host cannot be read: ") || strings.Contains(env.Summary, "no default host is set") {
		t.Errorf("summary %q", env.Summary)
	}

	e.vars["MINKA_HOST"] = e.server.URL
	report, _ := e.doctor()
	if detail := report.check(t, "sign-in").Detail; !strings.Contains(detail, "the default host cannot be read: ") {
		t.Errorf("sign-in detail %q", detail)
	}
}

// A configuration file others can read stops every command that reads it, so
// the report of a session that got past it through the environment says so.
func TestStatusSaysThatTheConfigurationCannotBeRead(t *testing.T) {
	e := newTestEnv(t)
	e.writeConfig(`{"hosts":{}}`)
	if err := os.Chmod(e.vars["MINKA_CONFIG"], 0o644); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "auth", "status")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "the default host cannot be read: ") || !strings.Contains(env.Summary, "accessible by other users") {
		t.Errorf("summary %q", env.Summary)
	}
}

// --host comes before $MINKA_HOST, and the report says which one named the
// host.
func TestStatusNamesTheFlagBeforeTheEnvironment(t *testing.T) {
	e, other := signedInToTwoHosts(t)
	e.vars["MINKA_HOST"] = e.server.URL

	env, r := e.runJSON("", "auth", "status", "--host", other.URL)
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["hostSource"] != hostFromFlag || data["host"] != other.URL {
		t.Errorf("data %s", env.Data)
	}
}

// A default host written by hand in another form is the same host, and a
// sign-in to it says it is the default.
func TestALoginToTheDefaultHostWrittenAnotherWaySaysItIsTheDefault(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost":"` + e.server.URL + `/api/v1/","hosts":{}}`)

	env, r := e.runJSON(e.fixture.Token, "auth", "login", "--with-token")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["isDefault"] != true || data["defaultHost"] != e.server.URL {
		t.Errorf("data %s", env.Data)
	}
}

// Without a sign-in, doctor still names the host the command was given.
func TestDoctorNamesTheHostOfTheCommandWithoutASession(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	e.vars["MINKA_HOST"] = "https://minka.example.com"

	report, _ := e.doctor()
	if report.check(t, "sign-in").Status != checkFail {
		t.Fatalf("sign-in %+v", report.check(t, "sign-in"))
	}
	if report.Host == nil || *report.Host != "https://minka.example.com" {
		t.Errorf("host %v", report.Host)
	}
}
