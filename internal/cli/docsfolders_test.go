package cli

import (
	"strings"
	"testing"
)

// A folder is shown by its ID alone, with its project and the folder it
// stands in; the data is Minka's answer as it came.
func TestAFolderIsShownByItsIDAlone(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	env := runOK(t, e, "docs", "folders", "show", f.ArchivedFolder.ID)
	data := decode(t, env.Data)
	if data["parent"].(map[string]any)["id"] != f.Folder.ID || data["projectId"] != f.Project.ID || data["archived"] != true {
		t.Fatalf("data %v", data)
	}
	if env.Summary != "Folder Last year in Documents of Test-Projekt: 0 folders, 1 document and 0 files, in the archive" {
		t.Fatalf("summary %q", env.Summary)
	}
	if requestsTo(e, "GET", "/api/v1/files-folders/"+f.ArchivedFolder.ID) != 1 {
		t.Fatal("the folder was not read by its address")
	}
	if text := e.run("", "docs", "folders", "show", f.Folder.ID, "--styled"); !strings.Contains(text.stdout, "Folder: "+f.Folder.ID) {
		t.Fatalf("text:\n%s", text.stdout)
	}

	f.Folder.Trashed = true
	_, r := e.runJSON("", "docs", "folders", "show", f.Folder.ID)
	expectCode(t, r, ExitNotFound)
}

// A Minka without the address answers with a page of its web: the message
// says that the host cannot, not that the folder is missing.
func TestAnOlderMinkaShowsNoFolderByItself(t *testing.T) {
	e := newTestEnv(t)
	e.server.KnowsNoTree = true
	env, r := e.runJSON("", "docs", "folders", "show", e.fixture.Folder.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != folderUnknownToTheHostHint || !strings.Contains(env.Error, "has no address for a folder by itself") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
}
