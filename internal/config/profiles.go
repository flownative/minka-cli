package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// MaxProfileName is the longest name a profile may have.
const MaxProfileName = 64

var profileNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// CheckProfileName says what is wrong with a name for a profile, or nil. A name
// is lower case letters, digits, "-" and ".", starts with a letter or a digit
// and is at most 64 characters long, so that it is typed without quotes and
// never reads as a host.
func CheckProfileName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("the name of a profile is empty")
	case len(name) > MaxProfileName:
		return fmt.Errorf("the profile name %q is longer than %d characters", name, MaxProfileName)
	case !profileNamePattern.MatchString(name):
		return fmt.Errorf("%q is no profile name: a name is lower case letters, digits, \"-\" and \".\", and starts with a letter or a digit", name)
	}
	return nil
}

// ProfileNameForHost is the name a profile gets that is named after its host:
// the host without the scheme, with a port joined by "-", and a number behind
// it where the name is taken already.
func (f *File) ProfileNameForHost(host string) string {
	name := nameOfAHost(host)
	if _, taken := f.Profiles[name]; !taken {
		return name
	}
	for i := 2; ; i++ {
		suffix := "-" + strconv.Itoa(i)
		candidate := name
		if len(candidate)+len(suffix) > MaxProfileName {
			candidate = candidate[:MaxProfileName-len(suffix)]
		}
		candidate += suffix
		if _, taken := f.Profiles[candidate]; !taken {
			return candidate
		}
	}
}

// nameOfAHost makes a profile name of a host. It reads the host as far as it
// may be printed, so that a password written into an address never becomes
// part of a name.
func nameOfAHost(host string) string {
	shown := HostForOutput(host)
	if _, rest, found := strings.Cut(shown, "://"); found {
		shown = rest
	}
	shown, _, _ = strings.Cut(shown, "/")
	// An address in brackets is one of IPv6, whose colons would leave a run of
	// dashes at the start of the name.
	prefix := ""
	if strings.HasPrefix(shown, "[") {
		prefix = "ipv6-"
	}
	shown = strings.ToLower(strings.NewReplacer("[", "", "]", "", ":", "-").Replace(shown))
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return '-'
	}, shown)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.TrimLeft(name, ".-")
	if name != "" {
		name = prefix + name
	}
	if len(name) > MaxProfileName {
		name = name[:MaxProfileName]
	}
	if name == "" {
		return "profile"
	}
	return name
}

// ProfilesOnHost are the names of the profiles on a host, sorted. A profile
// counts by its host read the way a sign-in writes it, so one written into the
// file by hand in another form counts as well.
func (f *File) ProfilesOnHost(host string) []string {
	var names []string
	for name, profile := range f.Profiles {
		if profile.Host == host {
			names = append(names, name)
			continue
		}
		if same, err := NormalizeHost(profile.Host); err == nil && same == host {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// ProfileNames are the names of all profiles, sorted.
func (f *File) ProfileNames() []string {
	return slices.Sorted(maps.Keys(f.Profiles))
}

// RemoveProfile forgets a profile and its token. If it was the default profile,
// there is no default afterwards: which account a command uses without being
// told is a choice, and a profile that is merely left over is not one.
func (f *File) RemoveProfile(name string) bool {
	if _, ok := f.Profiles[name]; !ok {
		return false
	}
	delete(f.Profiles, name)
	if f.DefaultProfile == name {
		f.DefaultProfile = ""
	}
	return true
}

// RenameProfile gives a profile another name and carries the default along.
func (f *File) RenameProfile(from, to string) error {
	profile, ok := f.Profiles[from]
	if !ok {
		return fmt.Errorf("there is no profile %q", from)
	}
	if _, taken := f.Profiles[to]; taken {
		return fmt.Errorf("there is a profile %q already", to)
	}
	delete(f.Profiles, from)
	f.Profiles[to] = profile
	if f.DefaultProfile == from {
		f.DefaultProfile = to
	}
	return nil
}

// HostProfile is the profile a host stands for where only a host is known: the
// default profile if it is on the host, else the first on it by name. It is
// what the file writes for older versions of minka under that host, and what
// state kept per host by those versions belongs to.
func (f *File) HostProfile(host string) (string, bool) {
	names := f.ProfilesOnHost(host)
	if len(names) == 0 {
		return "", false
	}
	if slices.Contains(names, f.DefaultProfile) {
		return f.DefaultProfile, true
	}
	return names[0], true
}

// onDisk is the file as it lies on disk. Next to the profiles it carries the
// hosts and the default host in the form of the versions before profiles, so
// that such a version still finds a sign-in for every host.
type onDisk struct {
	DefaultProfile string               `json:"defaultProfile,omitempty"`
	Profiles       *map[string]Profile  `json:"profiles,omitempty"`
	DefaultHost    string               `json:"defaultHost,omitempty"`
	ClientID       string               `json:"clientId,omitempty"`
	Hosts          map[string]HostEntry `json:"hosts"`
	Comments       CommentSettings      `json:"comments,omitzero"`
}

// onDisk writes every host of a profile with the account HostProfile names for
// it.
func (f *File) onDisk() onDisk {
	profiles := f.Profiles
	if profiles == nil {
		profiles = map[string]Profile{}
	}
	stored := onDisk{
		DefaultProfile: f.DefaultProfile,
		Profiles:       &profiles,
		ClientID:       f.ClientID,
		Hosts:          map[string]HostEntry{},
		Comments:       f.Comments,
	}
	for _, profile := range profiles {
		host := profile.Host
		if _, written := stored.Hosts[host]; written {
			continue
		}
		name, _ := f.hostProfileAsWritten(host)
		chosen := profiles[name]
		stored.Hosts[host] = HostEntry{Token: chosen.Token, PersonName: chosen.PersonName, EmailAddress: chosen.EmailAddress}
	}
	if profile, ok := profiles[f.DefaultProfile]; ok {
		stored.DefaultHost = profile.Host
	}
	return stored
}

// hostProfileAsWritten is HostProfile for a host as a profile writes it, which
// may be one no sign-in writes.
func (f *File) hostProfileAsWritten(host string) (string, bool) {
	if normalized, err := NormalizeHost(host); err == nil {
		return f.HostProfile(normalized)
	}
	var names []string
	for name, profile := range f.Profiles {
		if profile.Host == host {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	if len(names) == 0 {
		return "", false
	}
	if slices.Contains(names, f.DefaultProfile) {
		return f.DefaultProfile, true
	}
	return names[0], true
}

// file reads what lies on disk.
//
// Where the file has profiles, they are what holds, and the hosts beside them
// are what this version wrote for older ones. A token there that no profile
// holds was written by hand or by an older version and is taken over into a
// profile named after its host, so that no sign-in is lost.
//
// A file without profiles is one of an older version, or one an older version
// wrote last, which drops what it does not know. Every host in it becomes a
// profile named after the host, and the default host the default profile.
func (stored onDisk) file() *File {
	file := &File{
		DefaultProfile: stored.DefaultProfile,
		Profiles:       map[string]Profile{},
		ClientID:       stored.ClientID,
		Comments:       stored.Comments,
	}
	if stored.Profiles != nil {
		maps.Copy(file.Profiles, *stored.Profiles)
	} else {
		file.DefaultProfile = ""
	}

	// A token counts as held by the host it signs in to: one person may well
	// have given two hosts the same one.
	held := map[string]bool{}
	for _, profile := range file.Profiles {
		held[signIn(profile.Host, profile.Token)] = true
	}
	// Hosts in the form a sign-in writes come first, so that they keep the
	// plain name where another entry means the same host.
	keys := slices.SortedFunc(maps.Keys(stored.Hosts), func(a, b string) int {
		aCanonical, bCanonical := isCanonical(a), isCanonical(b)
		if aCanonical != bCanonical {
			if aCanonical {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	takenOver := map[string]string{}
	for _, key := range keys {
		entry := stored.Hosts[key]
		host := key
		if normalized, err := NormalizeHost(key); err == nil {
			host = normalized
		}
		if entry.Token == "" || held[signIn(host, entry.Token)] {
			continue
		}
		name := file.ProfileNameForHost(host)
		file.Profiles[name] = Profile{Host: host, Token: entry.Token, PersonName: entry.PersonName, EmailAddress: entry.EmailAddress}
		held[signIn(host, entry.Token)] = true
		takenOver[key] = name
	}

	if stored.Profiles == nil && stored.DefaultHost != "" {
		file.DefaultProfile = defaultOfAnOlderFile(stored.DefaultHost, keys, takenOver)
	}
	return file
}

// defaultOfAnOlderFile is the profile the default host of an older file became:
// the entry written letter for letter like it, else the first that means the
// same host.
func defaultOfAnOlderFile(defaultHost string, keys []string, takenOver map[string]string) string {
	if name, ok := takenOver[defaultHost]; ok {
		return name
	}
	wanted, err := NormalizeHost(defaultHost)
	if err != nil {
		return ""
	}
	for _, key := range keys {
		if same, err := NormalizeHost(key); err == nil && same == wanted && takenOver[key] != "" {
			return takenOver[key]
		}
	}
	return ""
}

func signIn(host, token string) string {
	if normalized, err := NormalizeHost(host); err == nil {
		host = normalized
	}
	return host + " " + token
}

func isCanonical(host string) bool {
	normalized, err := NormalizeHost(host)
	return err == nil && normalized == host
}
