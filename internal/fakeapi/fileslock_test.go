package fakeapi

import (
	"net/http"
	"testing"
)

// Locking and unlocking refuse in Minka's order: the document and the client
// first, then the project, then the archive, and only then the lock itself.
func TestLockAndUnlockRefuseInMinkasOrder(t *testing.T) {
	for _, c := range []struct {
		name   string
		set    func(*Server, *Fixture)
		token  string
		action string
		of     func(*Fixture) *Document
		want   string
	}{
		{"the team locks", nil, "", "lock", nil, "200"},
		{"locked already", func(s *Server, f *Fixture) { s.LockDocument(f.DocumentInFolder.ID) }, "", "lock", nil, "422 documentLocked"},
		{"the team unlocks", func(s *Server, f *Fixture) { s.LockDocument(f.DocumentInFolder.ID) }, "", "unlock", nil, "200"},
		{"not locked", nil, "", "unlock", nil, "422 documentNotLocked"},
		{"a client, shown the document", func(_ *Server, f *Fixture) { f.Folder.ShownToClients = true }, "minka_pat_client", "lock", nil, "404"},
		{"a client unlocking", func(s *Server, f *Fixture) { f.Folder.ShownToClients = true; s.LockDocument(f.DocumentInFolder.ID) }, "minka_pat_client", "unlock", nil, "404"},
		{"a client in an archived project", func(_ *Server, f *Fixture) { f.Folder.ShownToClients = true; f.Project.Archived = true }, "minka_pat_client", "lock", nil, "404"},
		{"an outsider", nil, "stranger", "lock", nil, "404"},
		{"a draft", nil, "", "lock", func(f *Fixture) *Document { return f.DraftDocument }, "404"},
		{"an archived project, locked already", func(s *Server, f *Fixture) { s.LockDocument(f.DocumentInFolder.ID); f.Project.Archived = true }, "", "lock", nil, "422 projectArchived"},
		{"an archived project, not locked", func(_ *Server, f *Fixture) { f.Project.Archived = true }, "", "unlock", nil, "422 projectArchived"},
		{"in the archive, locked already", func(s *Server, f *Fixture) { s.LockDocument(f.ArchivedDocument.ID) }, "", "lock", func(f *Fixture) *Document { return f.ArchivedDocument }, "422 documentArchived"},
		{"in the archive, not locked", nil, "", "unlock", func(f *Fixture) *Document { return f.ArchivedDocument }, "422 documentArchived"},
		{"below an archived folder", nil, "", "lock", func(f *Fixture) *Document { return f.DocumentBelowTheArchivedFolder }, "422 documentArchived"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := New(t)
			s.Tokens["minka_pat_client"] = f.Client.ID
			if c.set != nil {
				c.set(s, f)
			}
			token := f.Token
			switch c.token {
			case "stranger":
				token = f.StrangerToken
			case "minka_pat_client":
				token = c.token
			}
			document := f.DocumentInFolder
			if c.of != nil {
				document = c.of(f)
			}
			if got, answer := answerTo(t, s, token, http.MethodPost, "/api/v1/documents/"+document.ID+"/"+c.action, ""); got != c.want {
				t.Fatalf("answered %s %v, want %s", got, answer["error"], c.want)
			}
		})
	}
}

// A lock names who locked it and the version it holds, and its blocks keep
// the rows of the moment; unlocking lists them afresh, and the document takes
// a new title again.
func TestALockHoldsTheNewestVersionUntilItIsUnlocked(t *testing.T) {
	s, f := New(t)
	f.Document.Body = blockOf(f.Decisions.ID, "")
	address := "/api/v1/documents/" + f.Document.ID
	updatedAt := f.Document.UpdatedAt

	_, locked := answerTo(t, s, f.Token, http.MethodPost, address+"/lock", "")
	by, _ := locked["lockedBy"].(map[string]any)
	version, _ := locked["lockedVersion"].(map[string]any)
	if locked["locked"] != true || locked["lockedAt"] == nil || by["id"] != f.Bruno.ID || version["number"] != float64(len(f.Document.Versions)) {
		t.Fatalf("locked %v at %v by %v, version %v", locked["locked"], locked["lockedAt"], by, version)
	}
	if !f.Document.UpdatedAt.Equal(updatedAt) {
		t.Error("the lock changed the moment of the document")
	}
	if block := locked["decisionBlocks"].([]any)[0].(map[string]any); block["frozen"] != true {
		t.Errorf("the block of a locked document is %v", block["frozen"])
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"title":"Another"}`); got != "422 documentLocked" {
		t.Errorf("a new title for a locked document answers %s", got)
	}

	_, unlocked := answerTo(t, s, f.Token, http.MethodPost, address+"/unlock", "")
	if unlocked["locked"] != false || unlocked["lockedAt"] != nil || unlocked["lockedBy"] != nil || unlocked["lockedVersion"] != nil {
		t.Fatalf("unlocked %v", unlocked)
	}
	if block := unlocked["decisionBlocks"].([]any)[0].(map[string]any); block["frozen"] != false {
		t.Errorf("the block of an unlocked document is %v", block["frozen"])
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"title":"Another"}`); got != "200" {
		t.Errorf("a new title for an unlocked document answers %s", got)
	}
}
