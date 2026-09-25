package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveWritesAFileOnlyItsOwnerCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minka", "config.json")
	file := &File{DefaultHost: "https://a.example", Hosts: map[string]HostEntry{"https://a.example": {Token: "minka_pat_x"}}}

	if err := file.Save(path); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions %04o, want 0600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("directory permissions %04o, want 0700", perm)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Hosts["https://a.example"].Token != "minka_pat_x" || loaded.DefaultHost != "https://a.example" {
		t.Errorf("loaded %+v", loaded)
	}
}

func TestSaveKeepsASymlinkedConfigFileALink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "minka.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"hosts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	file := &File{Hosts: map[string]HostEntry{"https://a.example": {Token: "minka_pat_x"}}}
	if err := file.Save(link); err != nil {
		t.Fatal(err)
	}

	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("config file is no longer a symlink: %v", err)
	}
	if loaded, err := Load(target); err != nil || loaded.Hosts["https://a.example"].Token != "minka_pat_x" {
		t.Errorf("target not written: %+v, %v", loaded, err)
	}
}

func TestAFileOthersCanReadIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"hosts":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); !errors.Is(err, ErrInsecurePermissions) {
		t.Errorf("got %v, want ErrInsecurePermissions", err)
	}
}

func TestAMissingFileIsAnEmptyConfig(t *testing.T) {
	file, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	if file.DefaultHost != "" || len(file.Hosts) != 0 {
		t.Errorf("got %+v", file)
	}
}

func TestRemoveHostPicksAnotherDefault(t *testing.T) {
	file := &File{DefaultHost: "https://b.example", Hosts: map[string]HostEntry{
		"https://b.example": {Token: "1"},
		"https://c.example": {Token: "2"},
		"https://a.example": {Token: "3"},
	}}

	if !file.RemoveHost("https://b.example") {
		t.Fatal("host not removed")
	}
	if file.DefaultHost != "https://a.example" {
		t.Errorf("default host %q", file.DefaultHost)
	}
	if file.RemoveHost("https://b.example") {
		t.Error("removing twice reported a removal")
	}
}

func TestNormalizeHost(t *testing.T) {
	valid := map[string]string{
		"minka.localbeach.net":                 "https://minka.localbeach.net",
		"https://Minka.LocalBeach.net/":        "https://minka.localbeach.net",
		"https://minka.localbeach.net/api/v1":  "https://minka.localbeach.net",
		"https://minka.localbeach.net/api/v1/": "https://minka.localbeach.net",
		"https://minka.example.com:8443":       "https://minka.example.com:8443",
		"https://minka.example.com:443":        "https://minka.example.com",
		"http://localhost:80":                  "http://localhost",
		"http://127.0.0.1:8080":                "http://127.0.0.1:8080",
		"http://localhost:8080":                "http://localhost:8080",
		"http://[::1]:8080":                    "http://[::1]:8080",
	}
	for input, want := range valid {
		got, err := NormalizeHost(input)
		if err != nil || got != want {
			t.Errorf("NormalizeHost(%q) = %q, %v; want %q", input, got, err, want)
		}
	}

	invalid := []string{
		"",
		"http://minka.localbeach.net",
		"ftp://minka.localbeach.net",
		"https://minka.localbeach.net/some/path",
		"https://user:pass@minka.localbeach.net",
		"https://minka.localbeach.net?x=1",
		"https://",
	}
	for _, input := range invalid {
		if got, err := NormalizeHost(input); err == nil {
			t.Errorf("NormalizeHost(%q) = %q, want an error", input, got)
		}
	}
}

// A refused host is said back to whoever gave it and ends up in a transcript
// and in a log. A password written into the address must not get there, while
// the refusal still has to say which host it is about and what is wrong with
// it — a message that said nothing would keep the secret too.
func TestNormalizeHostRefusesWithoutRepeatingACredential(t *testing.T) {
	const user = "robert"
	const password = "s3cr3t-password"
	const another = "other-s3cr3t"

	hosts := []string{
		"https://" + user + ":" + password + "@minka.localbeach.net",
		user + ":" + password + "@minka.localbeach.net",
		"https://" + user + ":" + password + "@minka.localbeach.net/api/v1",
		"http://" + user + ":" + password + "@minka.localbeach.net",
		"https://" + password + "@minka.localbeach.net",
		"ftp://" + user + ":" + password + "@minka.localbeach.net",
		// A password of its own making holds whatever characters it was given.
		// One with a slash, a question mark or a hash in it moves the end of the
		// user information out of reach of a search through the text, so what is
		// shown is built from the parts of the address instead.
		"https://" + user + ":pass/" + password + "@minka.localbeach.net",
		"https://" + user + ":pass?" + password + "@minka.localbeach.net",
		"https://" + user + ":pass#" + password + "@minka.localbeach.net",
		// A separator written percent-encoded leaves no literal @ or : to find.
		"https://" + user + ":" + password + "%40minka.localbeach.net",
		"https://" + user + "%3A" + password + "%40minka.localbeach.net",
		// A second @ belongs to the user information as well, so the last one
		// ends it — here where the address still parses, and below where a
		// control character keeps it from parsing at all.
		"https://a:" + password + "@b:" + another + "@minka.localbeach.net",
		"https://a:" + password + "@b:" + another + "@minka.localbeach.net\x7f",
		// A value with a control character in it cannot be parsed as a URL at
		// all and is refused before anything of it is understood.
		"https://" + user + ":" + password + "@minka.localbeach.net\x7f",
		"https://minka.localbeach.net?token=" + password,
		"https://minka.localbeach.net#" + password,
	}
	for _, host := range hosts {
		got, err := NormalizeHost(host)
		if err == nil {
			t.Errorf("NormalizeHost(%q) = %q, want an error", host, got)
			continue
		}
		message := err.Error()
		for _, secret := range []string{password, user, another} {
			if strings.Contains(message, secret) {
				t.Errorf("the refusal of %q hands on %q: %s", host, secret, message)
			}
		}
		if !strings.Contains(message, "minka.localbeach.net") {
			t.Errorf("the refusal of %q does not say which host it is about: %s", host, message)
		}
	}
}

// The refusal of a host that carries user information says what is wrong with
// it, so that whoever wrote the password into the address knows what to take
// out of it.
func TestNormalizeHostNamesUserInformationAsTheFault(t *testing.T) {
	_, err := NormalizeHost("https://robert:s3cr3t-password@minka.localbeach.net")
	if err == nil {
		t.Fatal("a host with user information was accepted")
	}
	if !strings.Contains(err.Error(), "user name and password before the @") {
		t.Errorf("the refusal does not name what is wrong: %s", err)
	}
}
