package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/cliupdate"
)

var updateMorning = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

// runAs runs a command as a minka of that version at that moment.
func (e *testEnv) runAs(own string, at time.Time, args ...string) result {
	e.t.Helper()
	return e.runWith(func(a *app) {
		a.ownVersion = own
		a.now = func() time.Time { return at }
	}, "", args...)
}

func (e *testEnv) updateState() cliupdate.State {
	e.t.Helper()
	state, err := cliupdate.Read(cliupdate.Path(e.vars["MINKA_CONFIG"]))
	if err != nil {
		e.t.Fatal(err)
	}
	return state
}

// pureEnvelope decodes stdout as one envelope and nothing beside it.
func pureEnvelope(t *testing.T, r result) map[string]json.RawMessage {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(r.stdout))
	var env map[string]json.RawMessage
	if err := decoder.Decode(&env); err != nil {
		t.Fatalf("stdout is no envelope: %v\n%s", err, r.stdout)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout carries more than the envelope:\n%s", r.stdout)
	}
	if r.stderr != "" {
		t.Fatalf("stderr is not empty: %q", r.stderr)
	}
	return env
}

func TestTheVersionsMinkaNamesAreRemembered(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest, e.server.CLIMinimum = "0.41.0", "0.40.0"

	expectCode(t, e.runAs("0.41.0", updateMorning, "projects", "list", "--json"), ExitOK)

	if got := e.updateState(); got.Latest != "0.41.0" || got.Minimum != "0.40.0" {
		t.Fatalf("remembered %+v", got)
	}
	info, err := os.Stat(cliupdate.Path(e.vars["MINKA_CONFIG"]))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("the state file has permissions %04o", info.Mode().Perm())
	}

	// Minka sets each header on its own, so one it leaves out beside the
	// other is unset.
	e.server.CLIMinimum = ""
	e.runAs("0.41.0", updateMorning, "projects", "list", "--json")
	if got := e.updateState(); got.Latest != "0.41.0" || got.Minimum != "" {
		t.Fatalf("remembered %+v", got)
	}
	e.server.CLILatest, e.server.CLIMinimum = "", "0.40.0"
	e.runAs("0.41.0", updateMorning, "projects", "list", "--json")
	if got := e.updateState(); got.Latest != "" || got.Minimum != "0.40.0" {
		t.Fatalf("remembered %+v", got)
	}
}

func TestAMinimumAloneBringsTheWarning(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLIMinimum = "0.40.0"

	r := e.runAs("0.39.0", updateMorning, "projects", "list", "--styled")
	want := "Warning: minka 0.39.0 is older than 0.40.0, the oldest version Minka supports; commands may fail or miss features.\n" +
		"Upgrade with: brew upgrade minka\n"
	if r.stderr != want {
		t.Fatalf("stderr %q", r.stderr)
	}
	env := pureEnvelope(t, e.runAs("0.39.0", updateMorning, "projects", "list", "--json"))
	if string(env["cliUpdate"]) != "{\n    \"current\": \"0.39.0\",\n    \"minimum\": \"0.40.0\"\n  }" {
		t.Fatalf("cliUpdate %s", env["cliUpdate"])
	}
}

func TestAnAnswerNamingNoVersionKeepsWhatWasRemembered(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest, e.server.CLIMinimum = "0.41.0", "0.40.0"
	e.runAs("0.41.0", updateMorning, "projects", "list", "--json")

	// Minka names a plain x.y.z or nothing; anything else names nothing here
	// either.
	for _, value := range []string{"", "not-a-version", "0.42", "01.2.3", "v0.42.0", "0.42.0-rc.1", "0.42.0+build"} {
		e.server.CLILatest, e.server.CLIMinimum = value, value
		expectCode(t, e.runAs("0.41.0", updateMorning, "projects", "list", "--json"), ExitOK)
		if got := e.updateState(); got.Latest != "0.41.0" || got.Minimum != "0.40.0" {
			t.Fatalf("after %q remembered %+v", value, got)
		}
	}
}

func TestAnErrorAnswerNamesTheVersionsAsWell(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest = "0.41.0"

	r := e.runAs("0.40.0", updateMorning, "cards", "show", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", "--json")
	expectCode(t, r, ExitNotFound)

	env := pureEnvelope(t, r)
	if string(env["code"]) != `"not_found"` {
		t.Fatalf("envelope %s", r.stdout)
	}
	var update cliUpdate
	if err := json.Unmarshal(env["cliUpdate"], &update); err != nil || update.Latest != "0.41.0" || update.Current != "0.40.0" {
		t.Fatalf("cliUpdate of the error %s", env["cliUpdate"])
	}
}

func TestAnUnreachableHostChangesNothing(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest, e.server.CLIMinimum = "0.41.0", "0.40.0"
	e.runAs("0.41.0", updateMorning, "projects", "list", "--json")

	closed := httptest.NewServer(http.NotFoundHandler())
	e.vars["MINKA_HOST"] = closed.URL
	closed.Close()
	expectCode(t, e.runAs("0.41.0", updateMorning, "projects", "list", "--json"), ExitNetwork)

	if got := e.updateState(); got.Latest != "0.41.0" || got.Minimum != "0.40.0" {
		t.Fatalf("remembered %+v", got)
	}
}

func TestTheNoticeOnATerminalComesOnceADay(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest = "0.41.0"

	r := e.runAs("0.40.0", updateMorning, "projects", "list", "--styled")
	expectCode(t, r, ExitOK)
	if r.stderr != "minka 0.41.0 is available (this is 0.40.0). Upgrade with: brew upgrade minka\n" {
		t.Fatalf("stderr %q", r.stderr)
	}
	if strings.Contains(r.stdout, "0.41.0") {
		t.Fatalf("the notice reached stdout: %s", r.stdout)
	}

	if r := e.runAs("0.40.0", updateMorning.Add(23*time.Hour), "projects", "list", "--styled"); r.stderr != "" {
		t.Fatalf("a second notice within a day: %q", r.stderr)
	}
	if r := e.runAs("0.40.0", updateMorning.Add(24*time.Hour), "projects", "list", "--styled"); !strings.Contains(r.stderr, "brew upgrade minka") {
		t.Fatalf("no notice a day later: %q", r.stderr)
	}
}

func TestTheEnvelopeCarriesTheUpdateOnceADayAndNothingElseIsPrinted(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest, e.server.CLIMinimum = "0.41.0", "0.39.0"

	// The terminal has a day of its own, so a line shown there takes nothing
	// from the envelope.
	e.runAs("0.40.0", updateMorning, "projects", "list", "--styled")

	env := pureEnvelope(t, e.runAs("0.40.0", updateMorning, "projects", "list", "--json"))
	var update map[string]string
	if err := json.Unmarshal(env["cliUpdate"], &update); err != nil {
		t.Fatalf("no cliUpdate: %v", err)
	}
	if update["current"] != "0.40.0" || update["latest"] != "0.41.0" || update["minimum"] != "0.39.0" {
		t.Fatalf("cliUpdate %v", update)
	}

	env = pureEnvelope(t, e.runAs("0.40.0", updateMorning.Add(time.Hour), "projects", "list", "--json"))
	if _, there := env["cliUpdate"]; there {
		t.Fatal("cliUpdate twice within a day")
	}
	env = pureEnvelope(t, e.runAs("0.40.0", updateMorning.Add(25*time.Hour), "projects", "list", "--json"))
	if _, there := env["cliUpdate"]; !there {
		t.Fatal("no cliUpdate a day later")
	}
}

func TestAnUnknownMinimumIsLeftOutOfTheEnvelope(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest = "0.41.0"

	env := pureEnvelope(t, e.runAs("0.40.0", updateMorning, "projects", "list", "--json"))
	if strings.Contains(string(env["cliUpdate"]), "minimum") {
		t.Fatalf("cliUpdate %s", env["cliUpdate"])
	}
}

func TestQuietPrintsTheDataAloneAndLeavesTheDayToTheEnvelope(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest = "0.41.0"

	r := e.runAs("0.40.0", updateMorning, "projects", "list", "-q")
	expectCode(t, r, ExitOK)
	if r.stderr != "" || strings.Contains(r.stdout, "0.41.0") || !strings.HasPrefix(r.stdout, "[") {
		t.Fatalf("stdout %s\nstderr %q", r.stdout, r.stderr)
	}

	env := pureEnvelope(t, e.runAs("0.40.0", updateMorning, "projects", "list", "--json"))
	if _, there := env["cliUpdate"]; !there {
		t.Fatal("--quiet took the day of the envelope")
	}
	if r := e.runAs("0.40.0", updateMorning, "projects", "list", "--styled"); !strings.Contains(r.stderr, "brew upgrade minka") {
		t.Fatalf("--quiet took the day of the terminal: %q", r.stderr)
	}
}

func TestBelowTheMinimumTheNoticeSaysSoAndTheCommandRuns(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest, e.server.CLIMinimum = "0.41.0", "0.40.0"

	r := e.runAs("0.39.2", updateMorning, "projects", "list", "--styled")
	expectCode(t, r, ExitOK)
	want := "Warning: minka 0.39.2 is older than 0.40.0, the oldest version Minka supports; commands may fail or miss features.\n" +
		"Upgrade to 0.41.0 with: brew upgrade minka\n"
	if r.stderr != want {
		t.Fatalf("stderr %q", r.stderr)
	}
	if !strings.Contains(r.stdout, e.fixture.Project.Name) {
		t.Fatalf("the command did not run: %s", r.stdout)
	}
}

func TestTheEnvironmentSwitchesTheNoticeOff(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest, e.server.CLIMinimum = "0.41.0", "0.40.0"
	e.vars["MINKA_NO_UPDATE_NOTICE"] = "1"

	if r := e.runAs("0.39.0", updateMorning, "projects", "list", "--styled"); r.stderr != "" {
		t.Fatalf("stderr %q", r.stderr)
	}
	if env := pureEnvelope(t, e.runAs("0.39.0", updateMorning, "projects", "list", "--json")); env["cliUpdate"] != nil {
		t.Fatal("cliUpdate despite the switch")
	}
	if got := e.updateState(); got.Latest != "0.41.0" {
		t.Fatalf("the switch kept the versions from being remembered: %+v", got)
	}

	// Switched off, nothing is shown, so nothing of the day is spent.
	e.vars["MINKA_NO_UPDATE_NOTICE"] = "0"
	if r := e.runAs("0.39.0", updateMorning, "projects", "list", "--styled"); !strings.Contains(r.stderr, "brew upgrade minka") {
		t.Fatalf("0 did not switch it on: %q", r.stderr)
	}
}

func TestADeveloperBuildIsNotToldToUpgrade(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest, e.server.CLIMinimum = "0.41.0", "0.41.0"

	for _, own := range []string{"dev", "v0.40.0-3-gabc1234", "0.40.0-dirty", "abc1234", "0.40.0+local", "0.41.0-rc.1"} {
		if r := e.runAs(own, updateMorning, "projects", "list", "--styled"); r.stderr != "" {
			t.Errorf("%s: stderr %q", own, r.stderr)
		}
		if env := pureEnvelope(t, e.runAs(own, updateMorning, "projects", "list", "--json")); env["cliUpdate"] != nil {
			t.Errorf("%s: cliUpdate %s", own, env["cliUpdate"])
		}
	}
}

func TestAReleaseWithTheVOfItsTagIsNamedTheWayMinkaNamesVersions(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest, e.server.CLIMinimum = "0.41.0", "0.40.0"

	r := e.runAs("v0.39.0", updateMorning, "projects", "list", "--styled")
	if !strings.HasPrefix(r.stderr, "Warning: minka 0.39.0 is older than 0.40.0,") {
		t.Fatalf("stderr %q", r.stderr)
	}
	env := pureEnvelope(t, e.runAs("v0.39.0", updateMorning, "projects", "list", "--json"))
	var update cliUpdate
	if err := json.Unmarshal(env["cliUpdate"], &update); err != nil || update.Current != "0.39.0" {
		t.Fatalf("cliUpdate %s", env["cliUpdate"])
	}
	if r := e.runAs("v0.39.0", updateMorning, "doctor", "--styled"); !strings.Contains(r.stdout, "minka 0.39.0 is older than 0.40.0") {
		t.Fatalf("doctor below the minimum:\n%s", r.stdout)
	}

	e.server.CLIMinimum = ""
	r = e.runAs("v0.40.0", updateMorning.Add(25*time.Hour), "projects", "list", "--styled")
	if r.stderr != "minka 0.41.0 is available (this is 0.40.0). Upgrade with: brew upgrade minka\n" {
		t.Fatalf("stderr %q", r.stderr)
	}
	if r := e.runAs("v0.40.0", updateMorning, "doctor", "--styled"); !strings.Contains(r.stdout, "minka 0.41.0 is available, this is 0.40.0") {
		t.Fatalf("doctor behind:\n%s", r.stdout)
	}
}

func TestACorruptStateFileIsIgnoredAndWrittenAnew(t *testing.T) {
	e := newTestEnv(t)
	e.server.CLILatest = "0.41.0"
	path := cliupdate.Path(e.vars["MINKA_CONFIG"])
	for _, content := range []string{"{not json", `{"latest": "\u001b[31mred"}`, `{"textNoticeAt": "yesterday"}`} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		r := e.runAs("0.40.0", updateMorning, "projects", "list", "--styled")
		expectCode(t, r, ExitOK)
		if !strings.Contains(r.stderr, "minka 0.41.0 is available") || strings.Contains(r.stderr, "red") {
			t.Fatalf("%s: stderr %q", content, r.stderr)
		}
		if got := e.updateState(); got.Latest != "0.41.0" {
			t.Fatalf("%s: remembered %+v", content, got)
		}
	}
}

func TestADirectoryThatCannotBeWrittenShowsNoNoticeAndFailsNothing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions do not hold here")
	}
	e := newTestEnv(t)
	e.server.CLILatest = "0.41.0"
	parent := filepath.Dir(filepath.Dir(e.vars["MINKA_CONFIG"]))
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	for range 2 {
		r := e.runAs("0.40.0", updateMorning, "projects", "list", "--styled")
		expectCode(t, r, ExitOK)
		if r.stderr != "" {
			t.Fatalf("a notice whose day cannot be kept: %q", r.stderr)
		}
	}
}

func TestDoctorShowsHowThisMinkaStands(t *testing.T) {
	e := newTestEnv(t)
	e.run("", "skill", "install")
	doctorAs := func(own string) (doctorReport, result) {
		t.Helper()
		r := e.runAs(own, updateMorning, "doctor", "--json")
		expectCode(t, r, ExitOK)
		env := pureEnvelope(t, r)
		if env["cliUpdate"] != nil {
			t.Fatal("doctor carries the notice beside its own check")
		}
		var report doctorReport
		if err := json.Unmarshal(env["data"], &report); err != nil {
			t.Fatal(err)
		}
		return report, r
	}

	report, _ := doctorAs("0.40.0")
	if got := report.check(t, "update"); got.Status != checkOK || got.Detail != "no Minka has named a version of minka yet" {
		t.Fatalf("nothing known: %+v", got)
	}

	// The answer to doctor's own request counts already.
	e.server.CLILatest = "0.41.0"
	report, _ = doctorAs("0.40.0")
	got := report.check(t, "update")
	if got.Status != checkOK || got.Detail != "minka 0.41.0 is available, this is 0.40.0" || got.Hint != "Run: brew upgrade minka" || !report.Healthy {
		t.Fatalf("behind: %+v", got)
	}

	report, _ = doctorAs("0.41.0")
	if got := report.check(t, "update"); got.Status != checkOK || got.Hint != "" || got.Detail != "minka 0.41.0 is the latest Minka names" {
		t.Fatalf("current: %+v", got)
	}

	report, _ = doctorAs("dev")
	if got := report.check(t, "update"); got.Status != checkOK || got.Hint != "" || !strings.Contains(got.Detail, "is no release") {
		t.Fatalf("dev: %+v", got)
	}

	e.server.CLIMinimum = "0.40.0"
	e.vars["MINKA_NO_UPDATE_NOTICE"] = "1"
	report, _ = doctorAs("0.39.0")
	got = report.check(t, "update")
	if got.Status != checkWarn || !report.Healthy || !strings.Contains(got.Detail, "older than 0.40.0") || !strings.Contains(got.Detail, "switched off") {
		t.Fatalf("below the minimum, switched off: %+v", got)
	}

	// Doctor reports the state itself and spends no day of the notice.
	delete(e.vars, "MINKA_NO_UPDATE_NOTICE")
	r := e.runAs("0.39.0", updateMorning, "doctor", "--styled")
	if !bytes.Contains([]byte(r.stdout), []byte("warn  update")) || r.stderr != "" {
		t.Fatalf("doctor as text:\n%s\nstderr %q", r.stdout, r.stderr)
	}
	if r := e.runAs("0.39.0", updateMorning, "projects", "list", "--styled"); !strings.Contains(r.stderr, "older than 0.40.0") {
		t.Fatalf("doctor spent the day of the notice: %q", r.stderr)
	}
}
