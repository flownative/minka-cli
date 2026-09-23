//go:build unix

package work

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The safeguard: reading and writing the store happens under a lock of the
// kernel. Without it, every one of these starts reads the same list and the
// one that writes last drops what the others put there — a card of somebody
// who is working goes quiet.
func TestNoCardIsLostWhenSeveralSessionsStartAtOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.json")
	const sessions = 16

	var wg sync.WaitGroup
	for i := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle, err := Open(path, 10*time.Second)
			if err != nil {
				t.Errorf("session %d: %v", i, err)
				return
			}
			defer handle.Close()
			handle.File.Host(aHostOfTheStore).Start(Card{ID: fmt.Sprintf("card-%02d", i), StartedAt: time.Now()})
			if err := handle.Save(); err != nil {
				t.Errorf("session %d: %v", i, err)
			}
		}()
	}
	wg.Wait()

	handle, err := Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if cards := handle.File.Host(aHostOfTheStore).CardIDs(); len(cards) != sessions {
		t.Fatalf("%d of %d cards are left: %v", len(cards), sessions, cards)
	}
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
