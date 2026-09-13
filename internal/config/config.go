// Package config keeps the hosts the CLI is signed in to and their tokens.
//
// The file holds access tokens in plain text. It is written with permissions
// 0600 in a directory with 0700, and a file that others can read is refused
// instead of used, the way ssh treats a private key.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type File struct {
	DefaultHost string               `json:"defaultHost,omitempty"`
	Hosts       map[string]HostEntry `json:"hosts"`
}

type HostEntry struct {
	Token        string `json:"token"`
	PersonName   string `json:"personName,omitempty"`   // who the token acts as, at sign-in
	EmailAddress string `json:"emailAddress,omitempty"` // who the token acts as, at sign-in
}

// ErrInsecurePermissions is returned for a config file that users other than
// its owner may read or write.
var ErrInsecurePermissions = errors.New("config file is accessible by other users")

// Path is where the config file lives: $MINKA_CONFIG, else
// $XDG_CONFIG_HOME/minka/config.json, else ~/.config/minka/config.json.
func Path(getenv func(string) string) (string, error) {
	if path := getenv("MINKA_CONFIG"); path != "" {
		return path, nil
	}
	if base := getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "minka", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}

	return filepath.Join(home, ".config", "minka", "config.json"), nil
}

// Load reads the config file. A missing file is an empty configuration.
func Load(path string) (*File, error) {
	file := &File{Hosts: map[string]HostEntry{}}

	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: %s has permissions %04o, run: chmod 600 %s", ErrInsecurePermissions, path, info.Mode().Perm(), path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(content, file); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if file.Hosts == nil {
		file.Hosts = map[string]HostEntry{}
	}

	return file, nil
}

// Save writes the file with permissions 0600. It writes a temporary file next
// to the target and renames it, so an interrupted write cannot leave a
// truncated file with the tokens of the other hosts gone.
func (f *File) Save(path string) error {
	// A config file that is a symlink, for example into a dotfiles
	// repository, is written where the link points and stays a link.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	content, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the config: %w", err)
	}

	// CreateTemp creates the file with 0600, before anything is written to it.
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(content, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// RemoveHost forgets the token of a host. If it was the default host, another
// remaining host becomes the default.
func (f *File) RemoveHost(host string) bool {
	if _, ok := f.Hosts[host]; !ok {
		return false
	}
	delete(f.Hosts, host)
	if f.DefaultHost == host {
		f.DefaultHost = ""
		remaining := make([]string, 0, len(f.Hosts))
		for name := range f.Hosts {
			remaining = append(remaining, name)
		}
		sort.Strings(remaining)
		if len(remaining) > 0 {
			f.DefaultHost = remaining[0]
		}
	}

	return true
}

// NormalizeHost turns what a user types into the canonical form the config is
// keyed by: scheme and host, without a path. "minka.localbeach.net",
// "https://minka.localbeach.net/" and "https://minka.localbeach.net/api/v1"
// are the same host.
//
// Plain http is only accepted for loopback addresses, because the token would
// travel in clear text.
func NormalizeHost(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("the host is empty")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	u, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid host URL", raw)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("%q has to start with https://", raw)
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%q is not a valid host URL", raw)
	}
	path := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v1")
	if path != "" {
		return "", fmt.Errorf("%q has a path; give the address of the Minka instance, such as https://minka.example.com", raw)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return "", fmt.Errorf("%q uses plain http, which would send the token unencrypted; use https", raw)
	}

	host := strings.ToLower(u.Host)
	if u.Scheme == "https" && u.Port() == "443" || u.Scheme == "http" && u.Port() == "80" {
		host = strings.TrimSuffix(host, ":"+u.Port())
	}

	return u.Scheme + "://" + host, nil
}

func isLoopback(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)

	return ip != nil && ip.IsLoopback()
}
