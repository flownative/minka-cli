package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Locking answers with the document, which then refuses a new title, and
// unlocking gives the title back; each says where to go from there.
func TestADocumentIsLockedAndUnlocked(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	id := f.Document.ID

	env := runOK(t, e, "docs", "lock", id)
	var locked struct {
		ID            string
		Locked        bool
		LockedVersion *struct{ Number int }
	}
	if err := json.Unmarshal(env.Data, &locked); err != nil || locked.ID != id || !locked.Locked || locked.LockedVersion == nil {
		t.Fatalf("data %s: %v", env.Data, err)
	}
	if requests := e.server.WritingRequests(); len(requests) != 1 || requests[0].Method != http.MethodPost || requests[0].Path != "/api/v1/documents/"+id+"/lock" {
		t.Fatalf("requests %v", requests)
	}
	if crumbFor(t, env.Breadcrumbs, "unlock") != "minka docs unlock "+id {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}
	if f.Document.Lock == nil {
		t.Fatal("the fake holds no lock")
	}
	refused, r := e.runJSON("", "docs", "edit", id, "--title", "Renamed", "--force")
	expectCode(t, r, ExitValidation)
	if refused.Meta["apiCode"] != apiCodeDocumentLocked {
		t.Fatalf("an edit of the locked document answers %v", refused.Meta["apiCode"])
	}

	text := e.run("", "docs", "unlock", id, "--styled")
	expectCode(t, text, ExitOK)
	if !strings.Contains(text.stdout, "Unlocked document "+f.Document.Title) || !strings.Contains(text.stdout, "Locked: no") {
		t.Errorf("text:\n%s", text.stdout)
	}
	runOK(t, e, "docs", "edit", id, "--title", "Renamed", "--force")
	runOK(t, e, "docs", "lock", id)
	if crumbFor(t, runOK(t, e, "docs", "unlock", id).Breadcrumbs, "lock") != "minka docs lock "+id {
		t.Error("an unlocked document names no lock")
	}
	if text := e.run("", "docs", "lock", id, "--styled"); !strings.Contains(text.stdout, "Locked: yes, version ") || !strings.Contains(text.stdout, "by "+f.Bruno.Name) {
		t.Errorf("the text does not say what stands locked:\n%s", text.stdout)
	}
}

// Every refusal says what it means for a lock: locked already, nothing to
// unlock, not there or a client, the archive.
func TestTheRefusalsOfALockSayWhatToDo(t *testing.T) {
	for _, c := range []struct {
		name, verb string
		set        func(e *testEnv) string // the document, and the token where it is not the team's
		code       int
		apiCode    string
		hint       []string
	}{
		{"locked already", "lock", func(e *testEnv) string {
			e.server.LockDocument(e.fixture.Document.ID)
			return e.fixture.Document.ID
		}, ExitValidation, apiCodeDocumentLocked, []string{"locked already", "minka docs unlock " + "{id}"}},
		{"not locked", "unlock", func(e *testEnv) string { return e.fixture.Document.ID }, ExitValidation, apiCodeDocumentNotLocked, []string{"nothing to unlock"}},
		{"a client", "lock", func(e *testEnv) string {
			e.fixture.Folder.ShownToClients = true
			e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID
			e.vars["MINKA_TOKEN"] = "minka_pat_client"
			return e.fixture.DocumentInFolder.ID
		}, ExitNotFound, "", []string{"a client of the project meets the same", "it is a draft"}},
		{"a draft", "unlock", func(e *testEnv) string { return e.fixture.DraftDocument.ID }, ExitNotFound, "", []string{"Only the team locks"}},
		{"in the archive", "lock", func(e *testEnv) string { return e.fixture.ArchivedDocument.ID }, ExitValidation, apiCodeDocumentArchived, []string{"minka docs unarchive " + "{id}"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t)
			id := c.set(e)
			env, r := e.runJSON("", "docs", c.verb, id)
			expectCode(t, r, c.code)
			if c.apiCode != "" && env.Meta["apiCode"] != c.apiCode {
				t.Errorf("apiCode %v, want %s", env.Meta["apiCode"], c.apiCode)
			}
			for _, want := range c.hint {
				if want = strings.ReplaceAll(want, "{id}", id); !strings.Contains(env.Hint, want) {
					t.Errorf("hint %q does not say %q", env.Hint, want)
				}
			}
			if strings.Contains(env.Error, "cannot lock") {
				t.Errorf("Minka's own refusal reads as a Minka without the address: %q", env.Error)
			}
		})
	}
}

// A Minka without the address answers with a page of its web, and the
// document is then not the one at fault.
func TestALockAgainstAMinkaWithoutTheAddressSaysSo(t *testing.T) {
	for _, verb := range []string{"lock", "unlock"} {
		t.Run(verb, func(t *testing.T) {
			e := newTestEnv(t)
			older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=UTF-8")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Not found</body></html>"))
			}))
			t.Cleanup(older.Close)
			e.vars["MINKA_HOST"] = older.URL

			env, r := e.runJSON("", "docs", verb, e.fixture.Document.ID)
			expectCode(t, r, ExitNotFound)
			if env.Hint != documentLockUnknownHint || !strings.Contains(env.Error, "cannot lock or unlock a document") {
				t.Fatalf("error %q, hint %q", env.Error, env.Hint)
			}
		})
	}
}
