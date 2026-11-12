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
	"strings"
	"unicode"
)

type File struct {
	DefaultHost string               `json:"defaultHost,omitempty"`
	ClientID    string               `json:"clientId,omitempty"`
	Hosts       map[string]HostEntry `json:"hosts"`
	Comments    CommentSettings      `json:"comments,omitzero"`
}

// CommentSettings are the settings of the comments on a card, for every host.
type CommentSettings struct {
	// RequireRead refuses a comment on a card that has comments this machine
	// has not shown.
	RequireRead bool `json:"requireRead,omitempty"`
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

// RemoveHost forgets the token of a host. If it was the default host, there is
// no default host afterwards: which host a command reaches without being told
// is a choice, and a host that is merely left over is not one.
func (f *File) RemoveHost(host string) bool {
	if _, ok := f.Hosts[host]; !ok {
		return false
	}
	delete(f.Hosts, host)
	if f.DefaultHost == host {
		f.DefaultHost = ""
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
	if !readsAsAHost(u) {
		if namesNothing(shown) {
			return "", errors.New("the address given names no machine; give the address of the Minka instance, such as https://minka.example.com")
		}
		return "", fmt.Errorf("%q names no machine; a host is the name or the address of one, with a port at most", shown)
	}
	path := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v1")
	if path != "" {
		return "", errors.New("the address given has a path; give the address of the Minka instance, such as https://minka.example.com")
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
// and path, and nothing that a credential can hide in, nor anything that is no
// part of an address at all.
//
// A host is said back to whoever gave it, and from there it travels into a
// transcript, a ticket and a log. A password or a token written into the
// address must not travel that far, and what is wrong with such a value is
// said in words instead. The host is shown as far as it reads as one: what
// stands behind the first character no name and no address is made of is
// dropped with the credential it could be.
func HostForOutput(raw string) string {
	value := strings.TrimSpace(raw)
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	// Where the value parses and no separator of user information is left in the
	// address, its parts say what is shown. Where one is left, the user
	// information did not end where a URL has it end: a password holding a
	// slash, a question mark or a hash moves that end into the path, and the
	// host and the path then carry the credential themselves.
	if u, err := url.Parse(value); err == nil && u.Host != "" && !holdsASeparator(u.Host+u.EscapedPath()) {
		return withoutControlCharacters(u.Scheme + "://" + hostAsFarAsItReads(u.Host) + u.EscapedPath())
	}

	return withoutControlCharacters(hostTextWithoutCredentials(value))
}

// readsAsAHost says whether the authority of an address can name a machine at
// all. A registered name may hold the sub-delimiters of a URL by the letter of
// the standard, so "minka.example.com;t=secret" parses as a host and would
// become a key of the configuration, of the store and of a lock, and be printed
// as an address somebody could reach. What answers for a Minka instance is a
// DNS name, an IP address, or an IP address in brackets, with a port at most.
func readsAsAHost(u *url.URL) bool {
	hostname := u.Hostname()
	if hostname == "" {
		return false
	}
	// In brackets stands an IP address, which the parser has read as one
	// already. Behind a percent stands the interface a link-local address is
	// reached over, and an empty one the parser refuses by itself.
	if strings.HasPrefix(u.Host, "[") {
		address, _, _ := strings.Cut(hostname, "%")
		return net.ParseIP(address) != nil
	}
	for _, r := range hostname {
		if !partOfAHostName(r) {
			return false
		}
	}

	return true
}

// namesNothing says whether what may be printed of a value is a scheme and
// nothing more, which happens where the very first character of the host is one
// no name is made of. Quoting that would name no host at all, so the refusal
// says that the address given is at fault instead.
func namesNothing(shown string) bool {
	_, host, found := strings.Cut(shown, "://")

	return !found || host == ""
}

// hostAsFarAsItReads is the authority up to the first character no name and no
// address is made of. What stands behind it is no part of the address, and
// whatever somebody wrote there has as little business in a transcript as a
// password has: the fault is named in words instead.
func hostAsFarAsItReads(host string) string {
	// An address in brackets is read as one by the parser or not at all, so
	// there is nothing in it to cut away.
	if strings.HasPrefix(host, "[") {
		return host
	}
	for i, r := range host {
		if !partOfAHostName(r) && r != ':' {
			return host[:i]
		}
	}

	return host
}

// partOfAHostName holds for the characters a DNS name and an IP address are
// written with. Letters are meant in every alphabet: an internationalised name
// reaches here as the text it was written in.
func partOfAHostName(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '.' || r == '_'
}

// withoutControlCharacters drops what no address is made of. What is left is
// printed, and a terminal, a transcript and a log read a control character as an
// order of their own instead of as text, so no caller has to quote the result to
// be safe.
func withoutControlCharacters(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}

		return r
	}, value)
}

// holdsASeparator says whether a part of an address still carries the sign that
// user information ends there, written plainly or percent-encoded.
func holdsASeparator(value string) bool {
	return strings.Contains(value, "@") || strings.Contains(value, "%40")
}

// hostTextWithoutCredentials renders a value whose address cannot be trusted to
// say where the user information ends. Such a value is refused as well and may
// carry just as much, so it is shown deliberately short: everything up to and
// including the last @ is replaced, and of what follows only as much as leads up
// to a path, a query or a fragment.
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

	return scheme + hostAsFarAsItReads(value)
}

func isLoopback(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)

	return ip != nil && ip.IsLoopback()
}
