//go:build unix

package work

import (
	"errors"
	"path/filepath"
	"testing"
)

// The safeguard: one process per host reports on a timer, and a second of the
// same host does not get the lock. Without it two of them would report the same
// list on timers of their own, neither hearing when the other stopped, and the
// one that outlived its session would keep reporting for the other's silence.
func TestASecondKeepaliveOfTheSameHostDoesNotGetTheLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minka", "work.json")

	first, err := HoldKeepalive(path, aHostOfTheStore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HoldKeepalive(path, aHostOfTheStore); !errors.Is(err, ErrBusy) {
		t.Fatalf("the second keepalive got the lock: %v", err)
	}
	if reporting, err := SomebodyKeepsAlive(path, aHostOfTheStore); err != nil || !reporting {
		t.Errorf("somebody keeps alive: %v, %v", reporting, err)
	}

	// Another host is another lock: its cards are its own, and one lock for the
	// machine would let them go dark without a word.
	elsewhere, err := HoldKeepalive(path, "https://minka.example.org")
	if err != nil {
		t.Fatalf("the keepalive of another host did not get its lock: %v", err)
	}
	elsewhere.Release()

	// And the lock is there for the taking once the one that held it is gone.
	first.Release()
	if reporting, err := SomebodyKeepsAlive(path, aHostOfTheStore); err != nil || reporting {
		t.Errorf("somebody keeps alive after the one that did gave up: %v, %v", reporting, err)
	}
	again, err := HoldKeepalive(path, aHostOfTheStore)
	if err != nil {
		t.Fatalf("the lock was not given up: %v", err)
	}
	again.Release()
}
