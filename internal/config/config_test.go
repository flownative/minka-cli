package config

import (
	"errors"
	"os"
	"path/filepath"
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
