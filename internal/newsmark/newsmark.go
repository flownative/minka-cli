// Package newsmark remembers how far a session has read the notifications of
// an account, so that the next wait goes on from there and nothing that came
// in between is missed.
//
// The marks lie in one file beside the configuration, per host, per reader and
// per session, a reader being the token a command signs in with. A mark is the
// position Minka handed out, kept as it came: it means nothing here, and two
// of them cannot be compared. A mark is therefore only ever moved on from the
// one a process read, so that a process that read less never puts back a
// position another one has moved past.
package newsmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/flownative/minka-cli/internal/localfile"
)

// ForgottenAfter is how long a mark is kept after it was last written. Every
// session of Claude Code has a name of its own, so without it the file would
// keep a mark for every session that ever waited. A session that comes back
// after that starts from the moment it comes back.
const ForgottenAfter = 30 * 24 * time.Hour

// File is the marks as they lie on disk.
type File struct {
	Hosts map[string]map[string]map[string]*Mark `json:"hosts"` // host, reader, session
}

// Mark is how far one session has read.
type Mark struct {
	Cursor    string    `json:"cursor"`
	WrittenAt time.Time `json:"writtenAt"`
}

// Path is where the marks lie: beside the configuration file, so that they
// belong to the machine and not to the directory a command was started in.
func Path(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "notifications-read.json")
}

// ErrUnreadable means that the file is there but holds no marks this package
// can read. MoveOn writes such a file anew.
var ErrUnreadable = errors.New("the marks cannot be read")

// Position is what is known of how far a session has read.
type Position struct {
	Cursor string
	Found  bool // the session has a mark
	// Forgotten says that the session had a mark, written longer than
	// ForgottenAfter ago, which counts as none.
	Forgotten bool
}

// Read gives the position a session has read up to. A mark older than
// ForgottenAfter counts as none, as it would be gone after the next write. It
// takes no lock: the file is only ever replaced whole.
func Read(path, host, reader, session string, now time.Time) (Position, error) {
	file, err := read(path)
	if err != nil {
		return Position{}, err
	}
	mark := file.Hosts[host][reader][session]
	switch {
	case mark == nil:
		return Position{}, nil
	case mark.WrittenAt.Before(now.Add(-ForgottenAfter)):
		return Position{Forgotten: true}, nil
	}
	return Position{Cursor: mark.Cursor, Found: true}, nil
}

// MoveOn writes the position to, where the session stands at from or has no
// mark at all, and says whether it wrote; an empty from stands for a session
// that has no mark yet, or one forgotten. Where another process has moved the
// mark meanwhile, that one's mark stays: it printed everything up to it, and
// of two positions nobody can tell here which is the later one.
//
// A mark that is not there is no such later position: another write took it
// out, pruning it as old or replacing a file that could not be read, and a
// process of the session that had written since would have left a mark that
// is recent. So the mark is written; dropping it would start the next wait
// from now, past the lines this one printed. For the same reason a mark is
// forgotten after ForgottenAfter only where nobody stands at it.
//
// A file it cannot read is written anew: what it held is lost either way.
//
// It waits for the lock as long as the context allows. A holder keeps it for
// one read and one write of this file and never across a request.
func MoveOn(ctx context.Context, path, host, reader, session, from, to string, now time.Time) (bool, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("creating %s: %w", dir, err)
	}
	held, err := localfile.Acquire(ctx, filepath.Join(dir, "notifications-read.lock"))
	if err != nil {
		return false, err
	}
	defer held.Release()

	file, err := read(path)
	if errors.Is(err, ErrUnreadable) {
		file, err = &File{Hosts: map[string]map[string]map[string]*Mark{}}, nil
	}
	if err != nil {
		return false, err
	}
	limit := now.Add(-ForgottenAfter)
	current := file.Hosts[host][reader][session]
	switch {
	case current == nil:
	case current.Cursor == from:
	case from == "" && current.WrittenAt.Before(limit):
	default:
		return false, nil
	}
	file.forgetWhatIsOlderThan(limit)

	readers := file.Hosts[host]
	if readers == nil {
		readers = map[string]map[string]*Mark{}
		file.Hosts[host] = readers
	}
	sessions := readers[reader]
	if sessions == nil {
		sessions = map[string]*Mark{}
		readers[reader] = sessions
	}
	sessions[session] = &Mark{Cursor: to, WrittenAt: now.UTC()}

	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encoding the marks: %w", err)
	}
	if err := localfile.Write(path, ".notifications-read-*.json", append(content, '\n')); err != nil {
		return false, err
	}
	return true, nil
}

func read(path string) (*File, error) {
	file := &File{Hosts: map[string]map[string]map[string]*Mark{}}
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(content, file); err != nil {
		return nil, fmt.Errorf("reading %s: %w: %w", path, ErrUnreadable, err)
	}
	if file.Hosts == nil {
		file.Hosts = map[string]map[string]map[string]*Mark{}
	}
	return file, nil
}

func (f *File) forgetWhatIsOlderThan(limit time.Time) {
	for host, readers := range f.Hosts {
		for reader, sessions := range readers {
			for session, mark := range sessions {
				if mark == nil || mark.WrittenAt.Before(limit) {
					delete(sessions, session)
				}
			}
			if len(sessions) == 0 {
				delete(readers, reader)
			}
		}
		if len(readers) == 0 {
			delete(f.Hosts, host)
		}
	}
}
