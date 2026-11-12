package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type doctorReport struct {
	Healthy     bool          `json:"healthy"`
	Checks      []doctorCheck `json:"checks"`
	Host        *string       `json:"host"`
	DefaultHost *string       `json:"defaultHost"`
}

func (e *testEnv) doctor() (doctorReport, envelope) {
	e.t.Helper()
	env, r := e.runJSON("", "doctor")
	expectCode(e.t, r, ExitOK)
	var report doctorReport
	if err := json.Unmarshal(env.Data, &report); err != nil {
		e.t.Fatal(err)
	}
	return report, env
}

func (r doctorReport) check(t *testing.T, name string) doctorCheck {
	t.Helper()
	for _, check := range r.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("no check named %q", name)
	return doctorCheck{}
}

func TestDoctorFindsNothingWrongWithAWorkingSetup(t *testing.T) {
	e := newTestEnv(t)
	e.run("", "skill", "install")

	report, env := e.doctor()
	if !report.Healthy {
		t.Fatalf("not healthy: %+v", report.Checks)
	}
	for _, check := range report.Checks {
		if check.Status != checkOK {
			t.Errorf("check %s is %s: %s", check.Name, check.Status, check.Detail)
		}
	}
	if strings.Contains(string(env.Data), e.fixture.Token) {
		t.Fatal("the report carries the token")
	}
}

func TestDoctorWarnsAboutAMissingSkillAndStaysHealthy(t *testing.T) {
	e := newTestEnv(t)

	report, _ := e.doctor()
	skill := report.check(t, "skill")
	if skill.Status != checkWarn || !strings.Contains(skill.Hint, "minka skill install") {
		t.Fatalf("skill check %+v", skill)
	}
	if !report.Healthy {
		t.Fatal("a warning made the report unhealthy")
	}
}

func TestDoctorSaysADifferingSkillOnce(t *testing.T) {
	e := newTestEnv(t)
	e.writeInstalledSkill("an older skill")

	report, env := e.doctor()
	if report.check(t, "skill").Status != checkWarn {
		t.Fatal("the differing skill is not reported")
	}
	if env.Notice != "" {
		t.Fatalf("notice %q repeats a check", env.Notice)
	}
}

func TestDoctorReportsATokenTheHostRefuses(t *testing.T) {
	e := newTestEnv(t)
	e.vars["MINKA_TOKEN"] = "minka_pat_wrong"

	report, _ := e.doctor()
	if report.Healthy {
		t.Fatal("healthy although the host refuses the token")
	}
	if host := report.check(t, "host"); host.Status != checkFail || host.Hint == "" {
		t.Fatalf("host check %+v", host)
	}
}

func TestDoctorPointsToSetupWhenNobodyIsSignedIn(t *testing.T) {
	e := newSetupEnv(t)

	report, _ := e.doctor()
	if report.Healthy {
		t.Fatal("healthy although nobody is signed in")
	}
	if signIn := report.check(t, "sign-in"); signIn.Status != checkFail || !strings.Contains(signIn.Hint, "minka setup") {
		t.Fatalf("sign-in check %+v", signIn)
	}
	if len(e.server.Requests()) != 0 {
		t.Fatal("a request was sent without a sign-in")
	}
}

func TestDoctorReportsAConfigFileOthersCanRead(t *testing.T) {
	e := newSetupEnv(t)
	path := e.vars["MINKA_CONFIG"]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hosts":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	report, _ := e.doctor()
	if config := report.check(t, "config"); config.Status != checkFail || !strings.Contains(config.Hint, "chmod 600") {
		t.Fatalf("config check %+v", config)
	}
	if signIn := report.check(t, "sign-in"); signIn.Status != checkSkip {
		t.Fatalf("the sign-in repeats the config error: %+v", signIn)
	}
}

func TestDoctorGivesAnAgentAHintItCanFollow(t *testing.T) {
	e := newSetupEnv(t)

	_, r := e.runJSON("", "doctor", "--host", "https://minka.example.com")
	expectCode(t, r, ExitOK)
	var env envelope
	_ = json.Unmarshal([]byte(r.stdout), &env)
	var report doctorReport
	_ = json.Unmarshal(env.Data, &report)
	if hint := report.check(t, "sign-in").Hint; !strings.Contains(hint, "minka auth login --host https://minka.example.com") {
		t.Fatalf("hint %q does not name the step that needs no terminal", hint)
	}
}
