//go:build !unix

package export

import "path/filepath"

// directoryIdentity names a directory by its path with the links resolved:
// minka is released for unix only, and elsewhere two spellings of one
// directory may take two locks.
func directoryIdentity(dir string) (string, error) {
	return filepath.EvalSymlinks(dir)
}
