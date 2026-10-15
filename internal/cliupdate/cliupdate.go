// Package cliupdate remembers which minka Minka names as the latest and as the
// oldest it still supports, and when a notice about it was last shown.
//
// What it keeps lies in one file beside the configuration. Nothing in it is
// worth failing a command for: a caller that cannot read it starts from
// nothing, and one that cannot write it shows no notice.
package cliupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/localfile"
)

// The headers every answer of the API carries where Minka names a version.
const (
	LatestHeader  = "Minka-CLI-Latest"
	MinimumHeader = "Minka-CLI-Minimum"
)

// NoticeEvery is how long a notice is kept back after it was shown on the
// same channel.
const NoticeEvery = 24 * time.Hour

// State is what the file holds.
type State struct {
	Latest       string    `json:"latest,omitempty"`
	Minimum      string    `json:"minimum,omitempty"`
	TextNoticeAt time.Time `json:"textNoticeAt,omitzero"` // last line on stderr
	JSONNoticeAt time.Time `json:"jsonNoticeAt,omitzero"` // last cliUpdate in an envelope
}

// Path is where the state lies: beside the configuration file.
func Path(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "cli-update.json")
}

// Read gives the state in the file. A missing file is an empty state; a file
// that cannot be read or holds a version that is none is an error, which a
// caller takes as an empty state as well.
func Read(path string) (State, error) {
	var state State
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return State{}, err
	}
	if err := json.Unmarshal(content, &state); err != nil {
		return State{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if !validOrEmpty(state.Latest) || !validOrEmpty(state.Minimum) {
		return State{}, fmt.Errorf("reading %s: a version in it is none", path)
	}
	return state, nil
}

// Write replaces the file whole, 0600 in a directory 0700. Two processes
// writing at once lose at most the moment of one notice, so there is no lock.
func Write(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return localfile.Write(path, ".cli-update-*.json", append(content, '\n'))
}

func validOrEmpty(value string) bool {
	_, ok := ParseRelease(value)
	return value == "" || ok
}

// Version is a release: major, minor and patch.
type Version [3]uint64

// releasePattern is a plain version x.y.z, the only kind Minka names and the
// only kind this minka compares itself as. It also keeps anything out that a
// terminal would read as an order, since a version taken from a header is
// printed.
var releasePattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

// ParseRelease reads a plain version x.y.z, as Minka names it in a header.
func ParseRelease(value string) (Version, bool) {
	match := releasePattern.FindStringSubmatch(value)
	if match == nil {
		return Version{}, false
	}
	var v Version
	for i := range v {
		v[i], _ = strconv.ParseUint(match[i+1], 10, 64)
	}
	return v, true
}

// ParseOwn reads the version of this minka, which may carry the v of its tag.
// Anything else is no release: a build of a checkout that is not a tag, such
// as 0.40.0-3-gabc1234 or 0.40.0-dirty, a prerelease or dev cannot say where
// it stands, and a developer is not told to upgrade what they build.
func ParseOwn(value string) (Version, bool) {
	return ParseRelease(strings.TrimPrefix(value, "v"))
}

// Compare orders two versions: below zero where v comes before other.
func (v Version) Compare(other Version) int {
	for i := range v {
		switch {
		case v[i] < other[i]:
			return -1
		case v[i] > other[i]:
			return 1
		}
	}
	return 0
}

// FromHeader gives a version a header names; ok is false where the header is
// missing or names no plain version.
func FromHeader(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if _, ok := ParseRelease(value); !ok {
		return "", false
	}
	return value, true
}
