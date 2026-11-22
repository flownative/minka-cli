package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func load(t *testing.T, path string) *File {
	t.Helper()
	file, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

// The file of a version before profiles is taken over whole: every host is a
// profile named after it, the default host is the default profile, and nothing
// else is lost.
func TestAFileOfHostsBecomesOneOfProfiles(t *testing.T) {
	path := writeFile(t, `{"defaultHost": "https://minka.flownative.com", "clientId": "abc", "comments": {"requireRead": true}, "hosts": {
		"https://minka.flownative.com": {"token": "minka_pat_a", "personName": "Robert", "emailAddress": "robert@example.com"},
		"http://127.0.0.1:8081": {"token": "minka_pat_b"},
		"https://forgotten.example": {"token": ""}
	}}`)

	file := load(t, path)
	if len(file.Profiles) != 2 {
		t.Fatalf("profiles %v", file.Profiles)
	}
	want := Profile{Host: "https://minka.flownative.com", Token: "minka_pat_a", PersonName: "Robert", EmailAddress: "robert@example.com"}
	if file.Profiles["minka.flownative.com"] != want {
		t.Errorf("profile %+v", file.Profiles["minka.flownative.com"])
	}
	if file.Profiles["127.0.0.1-8081"].Token != "minka_pat_b" || file.Profiles["127.0.0.1-8081"].Host != "http://127.0.0.1:8081" {
		t.Errorf("profile with a port %+v", file.Profiles["127.0.0.1-8081"])
	}
	if file.DefaultProfile != "minka.flownative.com" || file.ClientID != "abc" || !file.Comments.RequireRead {
		t.Errorf("file %+v", file)
	}
}

// A plain read never writes: an older minka on the same machine keeps reading
// the file as it was until a command of this version changes something.
func TestLoadingAnOlderFileDoesNotRewriteIt(t *testing.T) {
	content := `{"defaultHost": "https://a.example", "hosts": {"https://a.example": {"token": "minka_pat_a"}}}`
	path := writeFile(t, content)
	load(t, path)
	if after, _ := os.ReadFile(path); string(after) != content {
		t.Errorf("the file changed: %s", after)
	}
}

// A file this version writes still reads to an older one: every host carries
// the token of the default profile where that one is on it, else of the first
// profile on it by name, and the default host is the host of the default
// profile.
func TestSaveWritesTheHostsForOlderVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	file := &File{DefaultProfile: "work", Profiles: map[string]Profile{
		"work":   {Host: "https://a.example", Token: "minka_pat_work", PersonName: "Work"},
		"agent":  {Host: "https://a.example", Token: "minka_pat_agent"},
		"family": {Host: "https://b.example", Token: "minka_pat_family"},
		"zoo":    {Host: "https://b.example", Token: "minka_pat_zoo"},
	}}
	if err := file.Save(path); err != nil {
		t.Fatal(err)
	}

	var older struct {
		DefaultHost string               `json:"defaultHost"`
		Hosts       map[string]HostEntry `json:"hosts"`
	}
	content, _ := os.ReadFile(path)
	if err := json.Unmarshal(content, &older); err != nil {
		t.Fatal(err)
	}
	if older.DefaultHost != "https://a.example" {
		t.Errorf("default host %q", older.DefaultHost)
	}
	if older.Hosts["https://a.example"].Token != "minka_pat_work" || older.Hosts["https://a.example"].PersonName != "Work" {
		t.Errorf("the default profile is not what its host carries: %+v", older.Hosts)
	}
	if older.Hosts["https://b.example"].Token != "minka_pat_family" {
		t.Errorf("the first profile by name is not what its host carries: %+v", older.Hosts)
	}

	// Read again by this version, the hosts are the profiles' own and add none.
	if again := load(t, path); len(again.Profiles) != 4 || again.DefaultProfile != "work" {
		t.Errorf("read again: %+v", again)
	}
}

// An older version drops the profiles when it writes the file, and keeps what
// it changed in the hosts. The next read takes those over, so that its sign-in
// is not lost; the names of the profiles are, and the account of a host that
// it never saw.
func TestAFileAnOlderVersionWroteLastKeepsItsSignIns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	file := &File{DefaultProfile: "flownative", Profiles: map[string]Profile{
		"flownative": {Host: "https://a.example", Token: "minka_pat_old"},
	}}
	if err := file.Save(path); err != nil {
		t.Fatal(err)
	}

	// What an older version writes after a sign-in of its own.
	var older struct {
		DefaultHost string               `json:"defaultHost,omitempty"`
		ClientID    string               `json:"clientId,omitempty"`
		Hosts       map[string]HostEntry `json:"hosts"`
	}
	content, _ := os.ReadFile(path)
	if err := json.Unmarshal(content, &older); err != nil {
		t.Fatal(err)
	}
	older.Hosts["https://a.example"] = HostEntry{Token: "minka_pat_new"}
	older.Hosts["https://c.example"] = HostEntry{Token: "minka_pat_c"}
	rewritten, _ := json.Marshal(older)
	if err := os.WriteFile(path, rewritten, 0o600); err != nil {
		t.Fatal(err)
	}

	again := load(t, path)
	if again.Profiles["a.example"].Token != "minka_pat_new" || again.Profiles["c.example"].Token != "minka_pat_c" {
		t.Errorf("profiles %+v", again.Profiles)
	}
	if again.DefaultProfile != "a.example" {
		t.Errorf("default profile %q", again.DefaultProfile)
	}
}

// Beside profiles, a host whose token no profile holds was written by hand,
// and it is taken over rather than lost.
func TestAHostBesideProfilesWithATokenOfItsOwnIsTakenOver(t *testing.T) {
	path := writeFile(t, `{"defaultProfile": "a.example", "profiles": {
		"a.example": {"host": "https://a.example", "token": "minka_pat_a"}
	}, "hosts": {
		"https://a.example": {"token": "minka_pat_a"},
		"https://b.example": {"token": "minka_pat_b"},
		"https://a.example/": {"token": "minka_pat_by_hand"}
	}}`)

	file := load(t, path)
	if len(file.Profiles) != 3 || file.Profiles["b.example"].Token != "minka_pat_b" || file.Profiles["a.example-2"].Token != "minka_pat_by_hand" {
		t.Errorf("profiles %+v", file.Profiles)
	}
	if file.DefaultProfile != "a.example" {
		t.Errorf("default profile %q", file.DefaultProfile)
	}
}

// A host written by hand in another form becomes a profile on the host as a
// sign-in writes it, and one that cannot be read keeps its form, under a name
// that carries no password written into it.
func TestAHostWrittenByHandBecomesAProfileWithoutItsCredentials(t *testing.T) {
	path := writeFile(t, `{"defaultHost": "https://Minka.Example.com/", "hosts": {
		"https://Minka.Example.com/": {"token": "minka_pat_a"},
		"https://robert:s3cret@minka.other.example": {"token": "minka_pat_b"}
	}}`)

	file := load(t, path)
	if file.Profiles["minka.example.com"].Host != "https://minka.example.com" || file.DefaultProfile != "minka.example.com" {
		t.Errorf("profiles %+v, default %q", file.Profiles, file.DefaultProfile)
	}
	for name, profile := range file.Profiles {
		if strings.Contains(name, "s3cret") || CheckProfileName(name) != nil {
			t.Errorf("profile name %q", name)
		}
		if profile.Token == "minka_pat_b" && profile.Host != "https://robert:s3cret@minka.other.example" {
			t.Errorf("the unreadable host was changed: %q", profile.Host)
		}
	}
}

func TestProfileNames(t *testing.T) {
	for _, name := range []string{"flownative", "familie", "a", "minka.flownative.com", "127.0.0.1-8081", "x-" + strings.Repeat("a", 62)} {
		if err := CheckProfileName(name); err != nil {
			t.Errorf("%q refused: %v", name, err)
		}
	}
	for _, name := range []string{"", "Flownative", "-a", ".a", "a b", "a_b", "https://a.example", "a:1", strings.Repeat("a", 65), "ä"} {
		if CheckProfileName(name) == nil {
			t.Errorf("%q taken", name)
		}
	}

	file := &File{Profiles: map[string]Profile{"a.example": {}}}
	if got := file.ProfileNameForHost("https://a.example"); got != "a.example-2" {
		t.Errorf("name for a taken host %q", got)
	}
	if got := file.ProfileNameForHost("http://[::1]:8080"); got != "ipv6-1-8080" {
		t.Errorf("name for an address in brackets %q", got)
	}
}

// A rename carries the default along and takes no name that is taken.
func TestRenameProfile(t *testing.T) {
	file := &File{DefaultProfile: "a", Profiles: map[string]Profile{"a": {Token: "1"}, "b": {Token: "2"}}}
	if err := file.RenameProfile("a", "b"); err == nil {
		t.Error("renamed onto a profile that exists")
	}
	if err := file.RenameProfile("a", "c"); err != nil {
		t.Fatal(err)
	}
	if file.DefaultProfile != "c" || file.Profiles["c"].Token != "1" {
		t.Errorf("file %+v", file)
	}
}
