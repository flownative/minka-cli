//go:build unix

package work

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/localfile"
)

// The safeguard: reading and writing the store happens under a lock of the
// kernel. Without it, every one of these starts reads the same list and the
// one that writes last drops what the others put there — a card of somebody
// who is working goes quiet.
//
// The test holds the lock until every session has found it taken, so that all
// of them stand at the store at the same moment whatever the pace of the
// machine, and then lets go.
func TestNoCardIsLostWhenSeveralSessionsStartAtOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.json")
	const sessions = 16

	held, err := Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	waiting := make(chan struct{}, sessions)
	localfile.BusyTried = func(tries int) {
		if tries == 1 {
			waiting <- struct{}{}
		}
	}
	t.Cleanup(func() { localfile.BusyTried = nil })

	done := make(chan error, sessions)
	for i := range sessions {
		go func() { done <- startACard(path, i) }()
	}

	finished := 0
	for waited := 0; waited < sessions && finished == 0; {
		select {
		case <-waiting:
			waited++
		case err := <-done:
			finished++
			t.Errorf("a session finished while the store was held: %v", err)
		}
	}
	held.Close()
	for range sessions - finished {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}

	handle, err := Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	var cards []string
	for _, list := range handle.File.Host(aHostOfTheStore).Sessions {
		cards = append(cards, list.CardIDs()...)
	}
	if len(cards) != sessions {
		t.Fatalf("%d of %d cards are left: %v", len(cards), sessions, cards)
	}
}

// startACard is one session taking up a card of its own.
func startACard(path string, session int) error {
	handle, err := Open(path, time.Minute)
	if err != nil {
		return fmt.Errorf("session %d: %w", session, err)
	}
	defer handle.Close()
	handle.File.Host(aHostOfTheStore).List(fmt.Sprintf("session-%02d", session)).Start(Card{ID: fmt.Sprintf("card-%02d", session), StartedAt: time.Now()})
	if err := handle.Save(); err != nil {
		return fmt.Errorf("session %d: %w", session, err)
	}
	return nil
}

// A signal waits for nobody: where another minka is at the store, it says so
// at once rather than holding the tool call it runs in.
func TestAStoreSomebodyElseHoldsIsBusyAndNotWaitedFor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.json")
	held, err := Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	began := time.Now()
	if _, err := Open(path, 0); err != ErrBusy {
		t.Fatalf("error %v, want %v", err, ErrBusy)
	}
	if waited := time.Since(began); waited > time.Second {
		t.Errorf("it waited %s", waited)
	}

	held.Close()
	next, err := Open(path, time.Second)
	if err != nil {
		t.Fatalf("the lock was not given up: %v", err)
	}
	next.Close()
}
