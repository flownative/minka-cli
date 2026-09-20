//go:build unix

package bundle

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const (
	fileLimitEnv  = "MINKA_TEST_FILE_LIMIT_TARGET"
	somebodysName = "what somebody called it.bin"
	noLimit       = "NO LIMIT:"
)

// The file system itself has to fail here: a failure somebody made up carries
// no path. So the test runs once more as a process that may write no file of
// more than a few bytes, and reads what that one says.
func TestAFailureToWriteAFileDoesNotSayWhatTheFileIsCalled(t *testing.T) {
	if target := os.Getenv(fileLimitEnv); target != "" {
		writeBeyondTheLimit(target)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestAFailureToWriteAFileDoesNotSayWhatTheFileIsCalled$")
	cmd.Env = append(os.Environ(), fileLimitEnv+"="+filepath.Join(t.TempDir(), "bundle"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the process with the limit: %v\n%s", err, output)
	}
	said := string(output)
	if strings.Contains(said, noLimit) {
		t.Skip("this system lets a process set no limit on the size of its files")
	}
	if !strings.Contains(said, "STORED: false") || !strings.Contains(said, "FAILURE: ") || strings.Contains(said, "FAILURE: <nil>") {
		t.Fatalf("the file system did not fail:\n%s", said)
	}
	if strings.Contains(said, somebodysName) || strings.Contains(said, "somebody") {
		t.Errorf("the failure says what the file is called:\n%s", said)
	}
}

func writeBeyondTheLimit(target string) {
	// Beyond the limit the kernel sends a signal first, which would end the
	// process, and fails the write only when nobody listens.
	signal.Ignore(syscall.SIGXFSZ)
	limit := syscall.Rlimit{Cur: 4096, Max: 4096}
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		fmt.Println(noLimit, err)
		return
	}
	w, err := Create(target, "basecamp")
	if err != nil {
		fmt.Println("no bundle could be made:", err)
		return
	}
	defer w.Abort()
	_, err = w.StoreFile("attachment-1", somebodysName, func(into io.Writer) error {
		_, err := io.Copy(into, repeated(1<<20))
		return err
	})
	_, statErr := os.Stat(filepath.Join(w.staging, filesDir, "attachment-1"))
	fmt.Printf("STORED: %v\nFAILURE: %v\n", statErr == nil, err)
}

// closedTo makes a directory one nothing can be removed from, until the test
// ends.
func closedTo(t *testing.T, dir string) {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("nothing is closed to root")
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

func TestAFileThatCannotBeRemovedIsNotNamed(t *testing.T) {
	w := mustCreate(t, filepath.Join(t.TempDir(), "bundle"))
	if _, err := w.AddFile("attachment-1", somebodysName, []byte("x")); err != nil {
		t.Fatal(err)
	}
	closedTo(t, filepath.Join(w.staging, filesDir, "attachment-1"))

	err := w.DropFile("attachment-1")
	if err == nil {
		t.Fatal("the file was removed out of a directory that is closed")
	}
	if strings.Contains(err.Error(), "somebody") || !strings.Contains(err.Error(), "attachment-1") {
		t.Errorf("the failure says what the file is called, or not whose it is: %s", err)
	}
}

func TestWhatAKilledRunLeftAndCannotBeRemovedIsNotNamed(t *testing.T) {
	parent := t.TempDir()
	leftover := filepath.Join(parent, ".bundle.staging-111")
	dir := filepath.Join(leftover, filesDir, "attachment-1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, somebodysName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	closedTo(t, dir)

	_, err := Create(filepath.Join(parent, "bundle"), "basecamp")
	if err == nil {
		t.Fatal("a run began although it could not tidy up")
	}
	if strings.Contains(err.Error(), "somebody") || !strings.Contains(err.Error(), leftover) {
		t.Errorf("the failure says what the file is called, or not where the leftover lies: %s", err)
	}
}
