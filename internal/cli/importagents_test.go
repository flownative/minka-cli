package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportWritesAgentsAndCountsThem(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--agent", "101=102", "--out", out)
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["agents"] != float64(1) {
		t.Errorf("agents %v, want 1", data["agents"])
	}
	people, err := os.ReadFile(filepath.Join(out, "people.jsonl"))
	if err != nil || !strings.Contains(string(people), `"kind":"agent","principal":"102"`) {
		t.Errorf("people.jsonl holds no agent (%v):\n%s", err, people)
	}

	r = e.run("import", "basecamp", "--project", "7001", "--agent", "101=102", "--dry-run", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "People written as agents of a human: 1") {
		t.Errorf("the text does not count the agent:\n%s", r.stdout)
	}
}

// Whom the account holds as no user of its own is refused as a mistake of the
// call, once the people of the account are read and before anything else.
func TestImportRefusesAnAgentTheAccountHoldsAsNoUserAsAMistakeOfTheCall(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--agent", "801=101", "--out", filepath.Join(parent, "made", "bundle"))
	expectCode(t, r, ExitUsage)
	if env.Code != CodeUsage || !strings.Contains(env.Error, "801") || !strings.Contains(env.Hint, "basecamp people list") {
		t.Errorf("code %q, error %q, hint %q", env.Code, env.Error, env.Hint)
	}
	for _, request := range e.account.Requests {
		if !strings.HasPrefix(request, "people.json") {
			t.Errorf("went on to ask for %s", request)
		}
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("left %v", entries)
	}
}
