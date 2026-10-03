//go:build !unix

package localfile

import "context"

// Lock is nothing where the kernel has no advisory lock that ends with the
// process. minka is released for unix only; elsewhere it builds, and two
// processes writing the same file at the same moment can lose each other's
// changes.
type Lock struct{}

func Acquire(context.Context, string) (*Lock, error) { return &Lock{}, nil }

func (l *Lock) Release() {}
