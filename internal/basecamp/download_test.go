package basecamp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// storageAddressOf is the storage address of an invented file, whose key ends
// in the number that tells the stand-in what to do.
func storageAddressOf(number int) string {
	return "https://storage.3.basecamp.com/4242/blobs/0a1b2c3d-0000-4000-8000-" + strconv.Itoa(100000000000 + number)[0:12] + "/download/some%20file.bin"
}

const (
	fileWhole = iota + 1
	fileBig
	fileGone
	fileCrash
	fileNetwork
	fileBrokenOff
	fileBrokenOffLong
	fileGoneHalfway
	fileLooksLikeAFailure
	fileHangs
	fileTrickles
	fileSlow
	fileKilled
	fileOutOfMemory
	fileForbidden
	fileComesSlowly
	fileSaysItIsFine
	fileNotSignedIn
)

// downloadStandIn takes the place of the Basecamp CLI for downloads. It keeps
// a line per call next to itself.
func downloadStandIn(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	path := filepath.Join(t.TempDir(), "basecamp")
	script := `#!/bin/sh
for last; do :; done
echo "$*" >> "$0.calls"
network='{
  "code": "network",
  "error": "Get \"https://storage.example.test/4242/blobs/key/download/secret%20name.pdf\": connection reset",
  "ok": false,
  "retryable": true
}'
gone='{
  "code": "not_found",
  "error": "resource not found",
  "meta": {"request_id": "abc"},
  "ok": false,
  "retryable": false
}'
case "$last" in
	*01/download/*) printf 'asked:%s quiet:%s' "$*" "$BASECAMP_NONINTERACTIVE" ;;
	*02/download/*) head -c 5242880 /dev/zero ;;
	*03/download/*) echo "$gone"; exit 2 ;;
	*04/download/*) echo "panic: runtime error near secret name.pdf" >&2; exit 2 ;;
	*05/download/*) echo "$network"; exit 6 ;;
	*06/download/*) printf 'half of a file'; echo "$network"; exit 6 ;;
	*07/download/*) head -c 100000 /dev/zero; echo "$network"; exit 6 ;;
	*08/download/*) head -c 100000 /dev/zero; echo "$gone"; exit 2 ;;
	*09/download/*) echo "$gone" ;;
	*10/download/*) printf 'a beginning'; exec sleep 30 ;;
	*11/download/*) while :; do printf 'x'; sleep 0.02; done ;;
	*12/download/*) exec sleep 30 ;;
	*13/download/*) kill -TERM $$ ;;
	*14/download/*) kill -KILL $$ ;;
	*15/download/*) echo '{"ok":false,"error":"no access to secret name.pdf","code":"forbidden"}'; exit 4 ;;
	*16/download/*) i=0; while [ $i -lt 40 ]; do printf 'x'; sleep 0.02; i=$((i+1)); done ;;
	*17/download/*) echo '{"ok": true, "note": "a file of JSON"}' ;;
	*18/download/*) echo '{"ok":false,"error":"Not authenticated","code":"auth_required"}'; exit 3 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func standInCalls(binary string) int {
	content, _ := os.ReadFile(binary + ".calls")
	return strings.Count(string(content), "\n")
}

// chunks notes how a file arrived.
type chunks struct {
	bytes.Buffer
	writes  int
	largest int
}

func (c *chunks) Write(content []byte) (int, error) {
	c.writes++
	c.largest = max(c.largest, len(content))
	return c.Buffer.Write(content)
}

func TestTheBasecampCLIIsCalledToStreamOneFileWithoutQuestions(t *testing.T) {
	address := storageAddressOf(fileWhole)
	cmd := Downloader{}.command(t.Context(), address)

	want := []string{"basecamp", "files", "download", "--out", "-", "--agent", "--", address}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("argv %v, want %v", cmd.Args, want)
	}
	if !slices.Contains(cmd.Env, "BASECAMP_NONINTERACTIVE=1") {
		t.Error("the environment does not tell the Basecamp CLI to ask nothing")
	}
	if named := (Downloader{Binary: "/opt/tools/basecamp"}).command(t.Context(), address); named.Args[0] != "/opt/tools/basecamp" {
		t.Errorf("ran %q, want the binary that was named", named.Args[0])
	}

	var got bytes.Buffer
	if err := (Downloader{Binary: downloadStandIn(t)}).Download(t.Context(), address, &got); err != nil {
		t.Fatal(err)
	}
	if want := "asked:files download --out - --agent -- " + address + " quiet:1"; got.String() != want {
		t.Errorf("got %q, want %q", got.String(), want)
	}
}

func TestAFileArrivesInPiecesAndIsNeverHeldAsAWhole(t *testing.T) {
	var got chunks
	if err := (Downloader{Binary: downloadStandIn(t)}).Download(t.Context(), storageAddressOf(fileBig), &got); err != nil {
		t.Fatal(err)
	}
	if got.Len() != 5<<20 {
		t.Fatalf("got %d bytes, want %d", got.Len(), 5<<20)
	}
	if got.writes < 5 || got.largest > 1<<20 {
		t.Errorf("the file came in %d pieces, the largest of %d bytes", got.writes, got.largest)
	}
}

func TestOnlyAStorageAddressOfBasecampReachesTheBasecampCLI(t *testing.T) {
	key := "0a1b2c3d-0000-4000-8000-000000000001"
	good := []string{
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png",
		"https://storage.app.basecamp.com/4242/blobs/" + key + "/download/plan%20A%0A.png",
		"https://storage.basecamp.com/1/blobs/" + strings.ToUpper(key) + "/download/a(1)+b,c;d=e@f!$&'~.png",
	}
	bad := []string{
		"http://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png",
		"https://3.basecampapi.com/4242/blobs/" + key + "/download/plan.png",
		"https://storage.3.basecamp.com.example.test/4242/blobs/" + key + "/download/plan.png",
		"https://storage.example.test/4242/blobs/" + key + "/download/plan.png",
		"https://mystorage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png",
		"https://storage.3.basecamp.com@example.test/4242/blobs/" + key + "/download/plan.png",
		"https://user@storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png",
		"https://storage.3.basecamp.com:8443/4242/blobs/" + key + "/download/plan.png",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png?token=1",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png#part",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/a/b.png",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/..",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/%2e%2e",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/.",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/bad%zzescape",
		// The Basecamp CLI asks for the path without its escapes.
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/..%2F..%2F..%2F..%2F999%2Fpeople.json",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/..%2f..%2f..%2fprojects.json",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan%20A%2Fb.png",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/..%5C..%5Cprojects.json",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan%00.png",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/a b.png",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/previews/full",
		"https://storage.3.basecamp.com/4242/blobs/../download/plan.png",
		"https://storage.3.basecamp.com/4242/blobs/..%2f..%2fpeople/download/plan.png",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "%2f../download/plan.png",
		"https://storage.3.basecamp.com/4242/blobs/not-a-key/download/plan.png",
		"https://storage.3.basecamp.com/4242/../1/blobs/" + key + "/download/plan.png",
		"https://storage.3.basecamp.com/account/blobs/" + key + "/download/plan.png",
		"https://storage.3.basecamp.com/4242/buckets/1/blobs/" + key + "/download/plan.png",
		"https://storage.3.basecamp.com/4242/blobs/" + key + "/download/plan.png\n--help",
		"--help",
		"",
	}

	binary := downloadStandIn(t)
	for _, address := range bad {
		var got bytes.Buffer
		err := Downloader{Binary: binary}.Download(t.Context(), address, &got)
		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != API || failed.Transient() {
			t.Errorf("%q: got %v, want a refusal", address, err)
			continue
		}
		if address != "" && strings.Contains(failed.Message, address) || strings.Contains(failed.Message, "plan") {
			t.Errorf("the refusal quotes the address: %s", failed.Message)
		}
	}
	if calls := standInCalls(binary); calls != 0 {
		t.Errorf("the Basecamp CLI was run %d times for addresses that are refused", calls)
	}
	for _, address := range good {
		if err := checkStorageAddress(address); err != nil {
			t.Errorf("%q is refused: %v", address, err)
		}
	}
}

func TestTheExitCodeDecidesWhetherADownloadFailedAndTheFailureObjectWhy(t *testing.T) {
	binary := downloadStandIn(t)
	cases := map[string]struct {
		file      int
		kind      Kind
		transient bool
	}{
		"gone":                                {fileGone, NotFound, false},
		"a crash with the exit code of gone":  {fileCrash, API, false},
		"the network":                         {fileNetwork, Network, true},
		"broken off after a few bytes":        {fileBrokenOff, Network, true},
		"broken off after many bytes":         {fileBrokenOffLong, Network, true},
		"called gone behind a part of a file": {fileGoneHalfway, API, false},
		"forbidden":                           {fileForbidden, Forbidden, false},
		"not signed in":                       {fileNotSignedIn, AuthRequired, false},
		"ended by a signal that is no stop":   {fileOutOfMemory, API, false},
	}
	for name, c := range cases {
		var got bytes.Buffer
		err := Downloader{Binary: binary}.Download(t.Context(), storageAddressOf(c.file), &got)
		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != c.kind || failed.Transient() != c.transient {
			t.Errorf("%s: got %v, want the kind %s", name, err, c.kind)
			continue
		}
		// What the Basecamp CLI printed may name the file, so none of it is said.
		for _, leaked := range []string{"secret", "name.pdf", "storage.", "blobs", "some%20file"} {
			if strings.Contains(failed.Message, leaked) {
				t.Errorf("%s: the reason quotes %q: %s", name, leaked, failed.Message)
			}
		}
	}

	// A failure object and nothing else, from a Basecamp CLI that ended well,
	// is marked as what it looks like and comes as a whole: whoever knows the
	// size of the file decides. It is no proof that the file is gone.
	var got bytes.Buffer
	err := Downloader{Binary: binary}.Download(t.Context(), storageAddressOf(fileLooksLikeAFailure), &got)
	if !errors.Is(err, ErrLooksLikeAFailure) || ErrLooksLikeAFailure.Kind != API || ErrLooksLikeAFailure.Transient() {
		t.Errorf("a failure object in place of a file: got %v", err)
	}
	if !strings.Contains(got.String(), `"not_found"`) || !strings.HasSuffix(got.String(), "}\n") {
		t.Errorf("not all of it was passed on: %q", got.String())
	}
	// Any other JSON is a file.
	got.Reset()
	if err := (Downloader{Binary: binary}).Download(t.Context(), storageAddressOf(fileSaysItIsFine), &got); err != nil || !strings.Contains(got.String(), "a file of JSON") {
		t.Errorf("a file of JSON: %q, %v", got.String(), err)
	}
}

// Every piece that arrives gives a download its time anew: one that takes
// longer than it may idle, and never idles that long, comes through.
func TestADownloadThatKeepsBringingBytesIsNotGivenUp(t *testing.T) {
	var got bytes.Buffer
	downloader := Downloader{Binary: downloadStandIn(t), IdleTimeout: 300 * time.Millisecond, TimeLimit: time.Minute}
	started := time.Now()
	if err := downloader.Download(t.Context(), storageAddressOf(fileComesSlowly), &got); err != nil {
		t.Fatalf("after %s: %v", time.Since(started), err)
	}
	if got.Len() != 40 || time.Since(started) < 2*downloader.IdleTimeout {
		t.Errorf("%d bytes in %s: the download did not outlast the time it may idle", got.Len(), time.Since(started))
	}
}

// stillRunning tells whether the Basecamp CLI of a download is still there. A
// download says which process it is as soon as there is one, so the test knows
// what to look for however far the CLI itself got before it was ended.
func stillRunning(t *testing.T, pid int) bool {
	t.Helper()
	if pid == 0 {
		t.Fatal("no Basecamp CLI was started")
	}
	return syscall.Kill(pid, 0) == nil
}

func TestADownloadThatHangsOrNeverEndsIsEndedWithTheBasecampCLI(t *testing.T) {
	for name, c := range map[string]struct {
		file       int
		downloader Downloader
		says       string
	}{
		"no byte for a while": {fileHangs, Downloader{IdleTimeout: 300 * time.Millisecond, TimeLimit: time.Minute}, "no byte for 300ms"},
		"nothing at all":      {fileSlow, Downloader{IdleTimeout: 300 * time.Millisecond, TimeLimit: time.Minute}, "no byte for 300ms"},
		"bytes without end":   {fileTrickles, Downloader{IdleTimeout: time.Minute, TimeLimit: 400 * time.Millisecond}, "did not end within 400ms"},
	} {
		t.Run(name, func(t *testing.T) {
			c.downloader.Binary = downloadStandIn(t)
			var got bytes.Buffer
			var cli int
			started := time.Now()
			err := c.downloader.download(t.Context(), c.downloader.commanding(storageAddressOf(c.file)), &got, func(pid int) { cli = pid })
			var failed *Error
			if !errors.As(err, &failed) || failed.Kind != Network || !strings.Contains(failed.Message, c.says) {
				t.Fatalf("got %v, want a failure of the network that says %q", err, c.says)
			}
			if time.Since(started) > 4*time.Second {
				t.Error("the Basecamp CLI was waited for instead of ended")
			}
			if stillRunning(t, cli) {
				t.Error("the Basecamp CLI is still running")
			}
		})
	}
}

func TestTheLimitsOfADownloadAreTheNamedOnes(t *testing.T) {
	if fileIdleTimeout != 60*time.Second || fileTimeLimit != time.Hour {
		t.Errorf("a download may idle for %s and take %s", fileIdleTimeout, fileTimeLimit)
	}
}

func TestADownloadThatIsStoppedIsInterruptedAndEndsTheBasecampCLI(t *testing.T) {
	binary := downloadStandIn(t)
	var got bytes.Buffer

	stopped, cancel := context.WithCancel(t.Context())
	cancel()
	err := Downloader{Binary: binary}.Download(stopped, storageAddressOf(fileWhole), &got)
	expectKind(t, err, Interrupted)
	if standInCalls(binary) != 0 {
		t.Error("the Basecamp CLI was run for a run that is stopped")
	}

	for _, file := range []int{fileSlow, fileHangs} {
		going, cancel := context.WithCancel(t.Context())
		var cli int
		started := time.Now()
		err = Downloader{Binary: binary}.download(going, Downloader{Binary: binary}.commanding(storageAddressOf(file)), &got, func(pid int) {
			// A stop that comes before the Basecamp CLI runs has no process to
			// end, and the test could not tell whether one was left behind.
			cli = pid
			cancel()
		})
		expectKind(t, err, Interrupted)
		if time.Since(started) > 3*time.Second {
			t.Error("the Basecamp CLI was waited for instead of stopped")
		}
		if stillRunning(t, cli) {
			t.Error("the Basecamp CLI is still running")
		}
	}

	// The signal may reach the Basecamp CLI first.
	err = Downloader{Binary: binary}.Download(t.Context(), storageAddressOf(fileKilled), &got)
	expectKind(t, err, Interrupted)
}

func TestAMissingBasecampCLIFailsADownloadAsNotInstalled(t *testing.T) {
	var got bytes.Buffer
	err := Downloader{Binary: filepath.Join(t.TempDir(), "basecamp")}.Download(t.Context(), storageAddressOf(fileWhole), &got)
	expectKind(t, err, NotInstalled)
}

type fullDisk struct{}

var errFullDisk = errors.New("no space left on device")

func (fullDisk) Write([]byte) (int, error) { return 0, errFullDisk }

// What cannot be written is a failure of the bundle, not of Basecamp: it ends
// a run instead of leaving one file out.
func TestABundleThatCannotTakeTheBytesIsNoFailureOfBasecamp(t *testing.T) {
	err := Downloader{Binary: downloadStandIn(t)}.Download(t.Context(), storageAddressOf(fileBig), fullDisk{})
	var failed *Error
	if !errors.Is(err, errFullDisk) || errors.As(err, &failed) {
		t.Errorf("got %v, want the failure of the writer as it is", err)
	}
}
