//go:build unix

package export

import (
	"fmt"
	"os"
	"syscall"
)

// directoryIdentity names a directory by what the file system knows it as,
// whatever path leads there.
func directoryIdentity(dir string) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("no device and inode for %s", dir)
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
