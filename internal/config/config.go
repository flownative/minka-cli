// Package config keeps the hosts the CLI is signed in to and their tokens.
//
// The file holds access tokens in plain text. It is written with permissions
// 0600 in a directory with 0700, and a file that others can read is refused
// instead of used, the way ssh treats a private key.
package config

import (
	"crypto/rand"
	"encoding/hex"
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
	ClientID    string               `json:"clientId,omitempty"`
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

// NewClientID makes the name this installation of minka goes by where the
// server keeps one set of things per client.
//
// It is random and means nothing on purpose: the name travels to the server,
// into its database and into its logs, and a host name or a user name there
// would say who sits at which machine, for as long as those logs are kept.
func NewClientID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("making a client id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
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
	shown := HostForOutput(raw)

	u, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid host URL", shown)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("%q has to start with https://", shown)
	}
	if u.User != nil {
		return "", fmt.Errorf("%q was given with a user name and password before the @; leave them out and sign in with a token", shown)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%q was given with a query or a fragment; give the address of the Minka instance alone", shown)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%q is not a valid host URL", shown)
	}
	path := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v1")
	if path != "" {
		return "", fmt.Errorf("%q has a path; give the address of the Minka instance, such as https://minka.example.com", shown)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return "", fmt.Errorf("%q uses plain http, which would send the token unencrypted; use https", shown)
	}

	host := strings.ToLower(u.Host)
	if u.Scheme == "https" && u.Port() == "443" || u.Scheme == "http" && u.Port() == "80" {
		host = strings.TrimSuffix(host, ":"+u.Port())
	}

	return u.Scheme + "://" + host, nil
}

// HostForOutput renders a host the way it may be printed: scheme, host, port
// and path, and nothing that a credential can hide in.
//
// A host is said back to whoever gave it, and from there it travels into a
// transcript, a ticket and a log. A password or a token written into the
// address must not travel that far, and what is wrong with such a value is
// said in words instead. Inside the CLI the value stays as it was given: it
// is a key of a store and part of the name of a lock, and only what is
// printed is rendered this way.
func HostForOutput(raw string) string {
	value := strings.TrimSpace(raw)
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	// Where the value parses, the parts decide what is shown, because a
	// password may hold a slash, a question mark or a hash and would take the
	// end of the user information out of reach of any search through the text.
	if u, err := url.Parse(value); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host + u.EscapedPath()
	}

	return hostTextWithoutCredentials(value)
}

// hostTextWithoutCredentials renders a value that cannot be parsed as a URL at
// all. Such a value is refused as well and may carry just as much, so it is
// shown deliberately short: everything up to and including the last @ is
// replaced, and of what follows only as much as leads up to a path, a query or
// a fragment.
func hostTextWithoutCredentials(value string) string {
	scheme := ""
	if i := strings.Index(value, "://"); i >= 0 {
		scheme, value = value[:i+3], value[i+3:]
	}
	// A separator written percent-encoded separates just as well once whoever
	// reads the address decodes it.
	value = strings.NewReplacer("%40", "@", "%3A", ":", "%3a", ":").Replace(value)
	if i := strings.LastIndex(value, "@"); i >= 0 {
		scheme, value = scheme+"***@", value[i+1:]
	}
	if i := strings.IndexAny(value, "/?#"); i >= 0 {
		value = value[:i]
	}

	return scheme + value
}

func isLoopback(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)

	return ip != nil && ip.IsLoopback()
}
