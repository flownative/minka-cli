package readmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/localfile"
)

const host = "https://minka.example.com"

var moment = time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)

// The safeguard: marks are read and added under a lock of the kernel, and a
// writer waits for the one before it. The test holds the lock itself, sees the
// writer find it taken, writes a mark of its own as the writer before it would,
// and lets go: the writer must not have finished before that, and must keep
// both marks after it. A writer that read the file before it took the lock
// would write over the mark of the test.
func TestAWriterWaitsForTheLockAndThenWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comments-read.json")
	held, err := localfile.Acquire(t.Context(), filepath.Join(filepath.Dir(path), "comments-read.lock"))
	if err != nil {
		t.Fatal(err)
	}
	busy := make(chan struct{}, 1)
	localfile.BusyTried = func(int) {
		select {
		case busy <- struct{}{}:
		default:
		}
	}
	t.Cleanup(func() { localfile.BusyTried = nil })

	done := make(chan error, 1)
	go func() { done <- Remember(t.Context(), path, host, "reader", "card", []string{"a"}, moment) }()

	select {
	case err := <-done:
		held.Release()
		t.Fatalf("the writer finished while the lock was held: %v", err)
	case <-busy:
	}
	select {
	case err := <-done:
		held.Release()
		t.Fatalf("the writer finished while the lock was held: %v", err)
	default:
	}
	before := &File{Hosts: map[string]map[string]map[string]*Card{
		host: {"reader": {"card": {Known: []string{"x"}, WrittenAt: moment}}},
	}}
	content, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := localfile.Write(path, ".comments-read-*.json", content); err != nil {
		t.Fatal(err)
	}
	held.Release()
	if err := <-done; err != nil {
		t.Fatalf("the writer failed after the lock was free: %v", err)
	}
	known, err := Known(path, host, "reader", "card")
	if err != nil {
		t.Fatal(err)
	}
	if !known["a"] || !known["x"] {
		t.Fatalf("known %v, want both the mark of the writer and the one before it", known)
	}
}

// Hosts, readers and cards each keep marks of their own.
func TestMarksAreKeptPerHostReaderAndCard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comments-read.json")
	if err := Remember(t.Context(), path, host, "reader", "card", []string{"a"}, moment); err != nil {
		t.Fatal(err)
	}
	for _, other := range [][3]string{
		{"https://other.example.com", "reader", "card"},
		{host, "somebody else", "card"},
		{host, "reader", "another card"},
	} {
		known, err := Known(path, other[0], other[1], other[2])
		if err != nil {
			t.Fatal(err)
		}
		if len(known) != 0 {
			t.Errorf("%v knows %v", other, known)
		}
	}
}

// A card not read for longer than ForgottenAfter is forgotten with the next
// write, and one read since is kept.
func TestTheMarkOfACardNotReadForLongIsForgotten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comments-read.json")
	if err := Remember(t.Context(), path, host, "reader", "old", []string{"a"}, moment); err != nil {
		t.Fatal(err)
	}
	if err := Remember(t.Context(), path, host, "reader", "recent", []string{"b"}, moment.Add(ForgottenAfter-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := Remember(t.Context(), path, host, "reader", "other", []string{"c"}, moment.Add(ForgottenAfter+time.Hour)); err != nil {
		t.Fatal(err)
	}
	for card, want := range map[string]int{"old": 0, "recent": 1, "other": 1} {
		known, err := Known(path, host, "reader", card)
		if err != nil {
			t.Fatal(err)
		}
		if len(known) != want {
			t.Errorf("%s knows %d comments, want %d", card, len(known), want)
		}
	}
}

// The file says which cards somebody reads, so only its owner may read it.
func TestTheMarksAreReadableByTheOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no permission bits")
	}
	path := filepath.Join(t.TempDir(), "minka", "comments-read.json")
	if err := Remember(t.Context(), path, host, "reader", "card", []string{"a"}, moment); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode %04o, want 0600", mode)
	}
}

// A reader is named without the token: nothing of it stands in the file.
func TestAReaderIsNotTheToken(t *testing.T) {
	token := "minka_secret_token_value"
	if Reader(token) == token || len(Reader(token)) != 32 || Reader(token) != Reader(token) || Reader(token) == Reader(token+"x") {
		t.Errorf("reader %q", Reader(token))
	}
}

// A read that adds nothing to a mark written less than a day ago leaves the
// file alone; a new comment, or a mark that old, is written.
func TestAMarkThatAddsNothingIsNotWrittenAgainWithinADay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comments-read.json")
	if err := Remember(t.Context(), path, host, "reader", "card", []string{"a"}, moment); err != nil {
		t.Fatal(err)
	}
	written := func() string {
		t.Helper()
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}
	first := written()

	if err := Remember(t.Context(), path, host, "reader", "card", []string{"a"}, moment.Add(RenewedAfter-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if written() != first {
		t.Error("a mark that added nothing was written again within a day")
	}
	if err := Remember(t.Context(), path, host, "reader", "card", []string{"a", "b"}, moment.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if known, _ := Known(path, host, "reader", "card"); !known["b"] {
		t.Error("a new comment was not written")
	}
	second := written()
	if err := Remember(t.Context(), path, host, "reader", "card", []string{"a"}, moment.Add(time.Minute+RenewedAfter)); err != nil {
		t.Fatal(err)
	}
	if written() == second {
		t.Error("a mark a day old was not written again")
	}
}
