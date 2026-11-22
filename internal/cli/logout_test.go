package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// A 401 of something in front of Minka says nothing about the token, which
// may be valid behind it: the entry stays.
func TestLogoutKeepsTheEntryOnA401ThatIsNotMinkas(t *testing.T) {
	e := signedInByFile(t)
	e.server.FailNextWith(http.StatusUnauthorized)

	env, r := e.runJSON("", "auth", "logout", "--revoke")
	expectCode(t, r, ExitAuth)
	if env.Retryable || !strings.Contains(env.Hint, "did not come from Minka's sign-in") {
		t.Errorf("retryable %v, hint %q", env.Retryable, env.Hint)
	}
	if !e.entryStands() {
		t.Fatal("the entry was taken out on a 401 Minka did not send")
	}
}

// The token of the file entry is revoked, never the one in the environment,
// and the answer says that one still stands.
func TestLogoutRevokesTheEntryAndNotTheTokenOfTheEnvironment(t *testing.T) {
	e := newTestEnv(t)
	e.vars["MINKA_TOKEN"] = e.fixture.AgentToken
	e.writeConfig(`{"defaultHost":"` + e.server.URL + `","hosts":{"` + e.server.URL + `":{"token":"` + e.fixture.Token + `"}}}`)

	env, r := e.runJSON("", "auth", "logout", "--revoke")
	expectCode(t, r, ExitOK)
	if e.revocations() != 1 || e.server.WritingRequests()[0].Header.Get("Authorization") != "Bearer "+e.fixture.Token {
		t.Fatal("the revocation did not carry the token of the entry")
	}
	if _, known := e.server.Tokens[e.fixture.AgentToken]; !known {
		t.Error("the token of the environment was revoked")
	}
	if !strings.Contains(env.Notice, "$MINKA_TOKEN was not revoked") {
		t.Errorf("notice %q", env.Notice)
	}
}

// With the token in the environment alone there is nothing to take out, and
// the answer says that the token was not revoked.
func TestLogoutWithRevokeOfATokenInTheEnvironmentAloneSaysSo(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "auth", "logout", "--revoke")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["removed"] != false || data["revoked"] != false {
		t.Errorf("data %s", env.Data)
	}
	if !strings.Contains(env.Notice, "$MINKA_TOKEN was not revoked") {
		t.Errorf("notice %q", env.Notice)
	}
	if e.revocations() != 0 {
		t.Error("a revocation went out")
	}
}

// The entry --host names is the one revoked, at its own host, and the default
// host keeps its entry and hears nothing.
func TestLogoutRevokesTheTokenOfTheHostItIsTold(t *testing.T) {
	e := newTestEnv(t)
	other, _ := fakeapi.New(t)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost":"` + other.URL + `","hosts":{` +
		`"` + other.URL + `":{"token":"minka_pat_bruno"},` +
		`"` + e.server.URL + `":{"token":"minka_pat_bruno"}}}`)

	_, r := e.runJSON("", "auth", "logout", "--host", e.server.URL, "--revoke")
	expectCode(t, r, ExitOK)
	if e.revocations() != 1 || len(other.Requests()) != 0 {
		t.Fatalf("revocations here %d, requests at the default host %d", e.revocations(), len(other.Requests()))
	}
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if standing := len(file.ProfilesOnHost(other.URL)) > 0; !standing {
		t.Error("the entry of the default host was taken out")
	}
	if standing := len(file.ProfilesOnHost(e.server.URL)) > 0; standing {
		t.Error("the entry of the host told was left")
	}
}

// An interruption may have reached the server or not: it is not retryable,
// and the way on is the same command again.
func TestLogoutKeepsTheEntryWhenTheRevocationIsInterrupted(t *testing.T) {
	e := signedInByFile(t)
	interrupted, cancel := context.WithCancel(context.Background())
	cancel()

	r := e.runWith(func(a *app) { a.ctx = interrupted }, "", "auth", "logout", "--revoke", "--json")
	expectCode(t, r, ExitNetwork)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		t.Fatalf("output is not a JSON envelope: %v\n%s", err, r.stdout)
	}
	if env.Retryable || !strings.Contains(env.Hint, "with --revoke again") || strings.Contains(env.Hint, "without --revoke") {
		t.Errorf("retryable %v, hint %q", env.Retryable, env.Hint)
	}
	if !e.entryStands() {
		t.Fatal("the entry was taken out")
	}
}

// A host that does not answer leaves the entry, and running again is safe.
func TestLogoutKeepsTheEntryWhenTheHostDoesNotAnswer(t *testing.T) {
	e := newTestEnv(t)
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost":"` + gone.URL + `","hosts":{"` + gone.URL + `":{"token":"minka_pat_bruno"}}}`)

	env, r := e.runJSON("", "auth", "logout", "--revoke")
	expectCode(t, r, ExitNetwork)
	if !env.Retryable || !strings.Contains(env.Hint, "Run the same command with --revoke again") {
		t.Errorf("retryable %v, hint %q", env.Retryable, env.Hint)
	}
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if standing := len(file.ProfilesOnHost(gone.URL)) > 0; !standing {
		t.Error("the entry was taken out")
	}
}

// A token revoked whose entry cannot be written away says both, and hands
// over what it did.
func TestLogoutSaysTheTokenIsRevokedWhenTheEntryCannotBeRemoved(t *testing.T) {
	e := signedInByFile(t)
	dir := filepath.Dir(e.vars["MINKA_CONFIG"])
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	env, r := e.runJSON("", "auth", "logout", "--revoke")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeConfig || !strings.Contains(env.Error, "the token is revoked, but its profile could not be taken out") {
		t.Errorf("code %q, error %q", env.Code, env.Error)
	}
	if data := decode(t, env.Data); data["revoked"] != true || data["removed"] != false {
		t.Errorf("data %s", env.Data)
	}
}

// signedInByFile signs in through the config file alone, so that a sign-out
// has an entry to take out and nothing in the environment stands beside it.
func signedInByFile(t *testing.T) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	delete(e.vars, "MINKA_TOKEN")
	expectCode(t, e.run(e.fixture.Token, "auth", "login", "--with-token"), ExitOK)
	return e
}

func (e *testEnv) entryStands() bool {
	e.t.Helper()
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		e.t.Fatal(err)
	}
	standing := len(file.ProfilesOnHost(e.server.URL)) > 0
	return standing
}

func (e *testEnv) revocations() int {
	count := 0
	for _, request := range e.server.WritingRequests() {
		if request.Method == http.MethodDelete && request.Path == "/api/v1/me/token" {
			count++
		}
	}
	return count
}

func TestLogoutWithRevokeRevokesTheTokenAndRemovesTheEntry(t *testing.T) {
	e := signedInByFile(t)

	env, r := e.runJSON("", "auth", "logout", "--revoke")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["removed"] != true || data["revoked"] != true {
		t.Errorf("data %v, want removed and revoked", data)
	}
	if env.Notice != "" {
		t.Errorf("a token revoked now carries the notice %q", env.Notice)
	}
	if e.revocations() != 1 {
		t.Fatalf("%d revocations went out, want 1", e.revocations())
	}
	last := e.server.WritingRequests()[0]
	if last.Header.Get("Authorization") != "Bearer "+e.fixture.Token {
		t.Errorf("the revocation did not carry the token of the entry")
	}
	if _, known := e.server.Tokens[e.fixture.Token]; known {
		t.Error("the server still accepts the token")
	}
	if e.entryStands() {
		t.Error("the entry is still in the config file")
	}
}

// Without --revoke a sign-out stays on this machine.
func TestLogoutWithoutRevokeSendsNothing(t *testing.T) {
	e := signedInByFile(t)

	env, r := e.runJSON("", "auth", "logout")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["removed"] != true || data["revoked"] != false {
		t.Errorf("data %s", env.Data)
	}
	if e.revocations() != 0 {
		t.Error("a sign-out without --revoke revoked the token")
	}
	if _, known := e.server.Tokens[e.fixture.Token]; !known {
		t.Error("the server no longer accepts the token")
	}
}

// The entry is the one copy of the token on this machine: a revocation that
// did not go through leaves it standing, and the exit code is the failure's.
func TestLogoutKeepsTheEntryWhenTheRevocationFails(t *testing.T) {
	for _, c := range []struct {
		status    int
		exit      int
		retryable bool
		hint      string
	}{
		{status: http.StatusBadGateway, exit: ExitAPI, retryable: true, hint: "Run the same command with --revoke again"},
		{status: http.StatusNotFound, exit: ExitNotFound, hint: "v0.53.0"},
		{status: http.StatusForbidden, exit: ExitForbidden, hint: "revoke the token in Minka's account area"},
	} {
		t.Run(http.StatusText(c.status), func(t *testing.T) {
			e := signedInByFile(t)
			e.server.FailNextWith(c.status)

			env, r := e.runJSON("", "auth", "logout", "--revoke")
			expectCode(t, r, c.exit)
			if env.Retryable != c.retryable || !strings.Contains(env.Hint, c.hint) {
				t.Errorf("retryable %v, hint %q", env.Retryable, env.Hint)
			}
			if !strings.Contains(env.Error, "entry is kept") {
				t.Errorf("the error %q does not say the entry is kept", env.Error)
			}
			if !e.entryStands() {
				t.Fatal("the entry was taken out although the token was not revoked")
			}

			// Run again, it goes through.
			env, r = e.runJSON("", "auth", "logout", "--revoke")
			expectCode(t, r, ExitOK)
			if e.entryStands() {
				t.Error("the second run left the entry")
			}
		})
	}
}

// A token the server no longer accepts is what --revoke asks for: the entry
// comes out, and a notice says that there was nothing left to revoke.
func TestLogoutWithRevokeOfATokenTheServerNoLongerAccepts(t *testing.T) {
	e := signedInByFile(t)
	delete(e.server.Tokens, e.fixture.Token)

	env, r := e.runJSON("", "auth", "logout", "--revoke")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["removed"] != true || data["revoked"] != true {
		t.Errorf("data %s", env.Data)
	}
	if !strings.Contains(env.Notice, "nothing left to revoke") {
		t.Errorf("notice %q", env.Notice)
	}
	if e.entryStands() {
		t.Error("the entry is still in the config file")
	}
}

// An entry that reads as no host sends its token nowhere, and keeps it.
func TestLogoutWithRevokeOfAnEntryThatCannotBeReadSendsNothing(t *testing.T) {
	const written = "https://minka.example.com;t=x"
	e := newTestEnv(t)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")
	e.writeConfig(`{"defaultHost":"` + written + `","hosts":{"` + written + `":{"token":"minka_pat_written_by_hand"}}}`)

	env, r := e.runJSON("", "auth", "logout", "--revoke")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeConfig || !strings.Contains(env.Hint, "without --revoke") {
		t.Errorf("code %q, hint %q", env.Code, env.Hint)
	}
	if len(e.server.Requests()) != 0 {
		t.Error("a request went out")
	}
	file, err := config.Load(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if standing := len(file.ProfilesOnHost(written)) > 0; !standing {
		t.Error("the entry was taken out although its token was not revoked")
	}
}
