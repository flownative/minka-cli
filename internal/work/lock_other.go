//go:build !unix

package work

import "time"

// lock is nothing where the kernel has no advisory lock that ends with the
// process. minka is released for unix only; elsewhere it builds, and two
// sessions writing the store at the same moment can lose each other's cards.
type lock struct{}

func acquire(string, time.Duration) (*lock, error) { return &lock{}, nil }

func (l *lock) release() {}
