//go:build !unix

package bundle

// lock is nothing where the kernel has no advisory lock that ends with the
// process. minka is released for unix only; elsewhere it builds, and keeping
// two runs off one target is up to whoever starts them.
type lock struct{}

func acquireLock(string) (*lock, error) { return &lock{}, nil }

func (l *lock) release() {}
