package newsmark

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const host = "https://minka.example.com"

var moment = time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

func markOf(t *testing.T, path, reader, session string) (string, bool) {
	t.Helper()
	position, err := Read(path, host, reader, session, moment)
	if err != nil {
		t.Fatal(err)
	}
	return position.Cursor, position.Found
}

func moveOn(t *testing.T, path, reader, session, from, to string, now time.Time) bool {
	t.Helper()
	written, err := MoveOn(t.Context(), path, host, reader, session, from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	return written
}

// The safeguard: a mark moves on only from the position the process read. A
// process that read less never puts back a position another one has moved
// past, and a session that has a mark is not started anew over it.
func TestAMarkMovesOnOnlyFromWhereItStands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-read.json")

	if !moveOn(t, path, "reader", "s1", "", "c1", moment) {
		t.Fatal("the first mark of a session was not written")
	}
	if moveOn(t, path, "reader", "s1", "", "c0", moment) {
		t.Fatal("a session that has a mark was started anew over it")
	}
	if !moveOn(t, path, "reader", "s1", "c1", "c3", moment) {
		t.Fatal("the mark did not move on from where it stood")
	}
	if moveOn(t, path, "reader", "s1", "c1", "c2", moment) {
		t.Fatal("a process that read less put back a position another one had moved past")
	}
	if cursor, _ := markOf(t, path, "reader", "s1"); cursor != "c3" {
		t.Fatalf("mark %q, want c3", cursor)
	}
}

// Every session of every reader on every host has a mark of its own.
func TestEverySessionAndReaderHasAMarkOfItsOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-read.json")
	moveOn(t, path, "reader", "s1", "", "a", moment)
	moveOn(t, path, "reader", "", "", "b", moment)
	moveOn(t, path, "other", "s1", "", "c", moment)

	for _, c := range []struct{ reader, session, want string }{
		{"reader", "s1", "a"}, {"reader", "", "b"}, {"other", "s1", "c"},
	} {
		if cursor, found := markOf(t, path, c.reader, c.session); !found || cursor != c.want {
			t.Errorf("%s/%q: %q %v, want %q", c.reader, c.session, cursor, found, c.want)
		}
	}
	if _, found := markOf(t, path, "other", ""); found {
		t.Error("a session that never waited has a mark")
	}
}

// A mark nobody wrote for ForgottenAfter goes with the next write, and the
// file is readable by its user alone.
func TestAnOldMarkIsForgottenAndTheFileIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minka", "notifications-read.json")
	moveOn(t, path, "reader", "old", "", "a", moment)
	moveOn(t, path, "reader", "new", "", "b", moment.Add(ForgottenAfter+time.Hour))

	if _, found := markOf(t, path, "reader", "old"); found {
		t.Error("a mark older than ForgottenAfter was kept")
	}
	if _, found := markOf(t, path, "reader", "new"); !found {
		t.Error("the new mark is missing")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("the marks can be read by others: %04o", perm)
	}
}

// The safeguard: a mark older than ForgottenAfter counts as none on reading
// as well, and a session starts anew over it, as over no mark at all.
func TestAMarkTooOldIsReadAsForgotten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-read.json")
	moveOn(t, path, "reader", "s1", "", "old", moment)

	later := moment.Add(ForgottenAfter + time.Hour)
	position, err := Read(path, host, "reader", "s1", later)
	if err != nil {
		t.Fatal(err)
	}
	if position.Found || !position.Forgotten || position.Cursor != "" {
		t.Fatalf("position %+v, want a forgotten one", position)
	}
	if !moveOn(t, path, "reader", "s1", "", "new", later) {
		t.Fatal("a session did not start anew over a forgotten mark")
	}
}

// A file that holds no marks is read as unreadable, and the next write
// replaces it.
func TestAnUnreadableFileIsWrittenAnew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-read.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path, host, "reader", "s1", moment); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("error %v, want ErrUnreadable", err)
	}
	if !moveOn(t, path, "reader", "s1", "", "c1", moment) {
		t.Fatal("the unreadable file was not written anew")
	}
	if cursor, _ := markOf(t, path, "reader", "s1"); cursor != "c1" {
		t.Fatalf("mark %q", cursor)
	}
}

// The safeguard: a process that read a mark moves it on however old it has
// grown since, also where the write of another session pruned it meanwhile;
// only a mark another process moved stays.
func TestAMarkReadIsMovedOnHoweverOldItGrew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications-read.json")
	moveOn(t, path, "reader", "a", "", "c1", moment)

	days := moment.Add(ForgottenAfter + 24*time.Hour)
	if !moveOn(t, path, "reader", "a", "c1", "c2", days) {
		t.Fatal("the mark a process read was forgotten under it")
	}
	if moveOn(t, path, "reader", "a", "c1", "c3", days) {
		t.Fatal("a process that read less put back a position another one had moved past")
	}

	moveOn(t, path, "reader", "a", "c2", "c4", moment)
	moveOn(t, path, "reader", "b", "", "b1", days)
	if _, found := markOf(t, path, "reader", "a"); found {
		t.Fatal("the write of b did not prune the old mark of a; the test proves nothing")
	}
	if !moveOn(t, path, "reader", "a", "c4", "c5", days) {
		t.Fatal("a mark pruned under a running wait was not written again")
	}
}
