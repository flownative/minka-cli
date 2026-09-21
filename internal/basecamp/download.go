package basecamp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

const (
	// fileIdleTimeout is how long a download may bring no byte before it is
	// given up.
	fileIdleTimeout = 60 * time.Second
	// fileTimeLimit is how long the download of one file may take at all.
	fileTimeLimit = time.Hour
	// childWaitDelay is how long a Basecamp CLI that was ended may hold on to
	// its output before it is let go of.
	childWaitDelay = 5 * time.Second
	// maxKeptOutput is how much of the end of what the Basecamp CLI printed
	// is kept, to read a failure from.
	maxKeptOutput = 8 << 10
	// maxEnvelopeStarts is how many places of the kept end are tried as the
	// beginning of a failure object.
	maxEnvelopeStarts = 64
)

// Downloader is the Files that runs the Basecamp CLI, which streams a stored
// file to its standard output. The Basecamp CLI brings the sign-in and sends
// its token to the API it is signed in at and nowhere else.
//
// It puts no limit of time on a download, so the limits are set here: a
// download that brings nothing for a while, or that never ends, is ended
// with the Basecamp CLI and counts as a failure of the network.
type Downloader struct {
	// Binary is the name or path of the Basecamp CLI; "basecamp" when empty.
	Binary string
	// IdleTimeout and TimeLimit replace fileIdleTimeout and fileTimeLimit.
	IdleTimeout time.Duration
	TimeLimit   time.Duration
}

func (d Downloader) Download(ctx context.Context, address string, into io.Writer) error {
	if err := checkStorageAddress(address); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return &Error{Kind: Interrupted, Message: "interrupted"}
	}
	idleTimeout, timeLimit := d.IdleTimeout, d.TimeLimit
	if idleTimeout == 0 {
		idleTimeout = fileIdleTimeout
	}
	if timeLimit == 0 {
		timeLimit = fileTimeLimit
	}

	running, end := context.WithCancel(ctx)
	defer end()
	var givenUp atomic.Pointer[string]
	giveUp := func(reason string) func() {
		return func() {
			givenUp.CompareAndSwap(nil, &reason)
			end()
		}
	}
	idle := time.AfterFunc(idleTimeout, giveUp(fmt.Sprintf("the download brought no byte for %s and was given up", idleTimeout)))
	defer idle.Stop()
	limit := time.AfterFunc(timeLimit, giveUp(fmt.Sprintf("the download did not end within %s and was given up", timeLimit)))
	defer limit.Stop()

	cmd := d.command(running, address)
	stdout := &watchedOutput{into: into, alive: func() { idle.Reset(idleTimeout) }}
	cmd.Stdout = stdout

	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case stdout.failed != nil:
		// The bundle did not take the bytes, which is no failure of Basecamp.
		return stdout.failed
	case err == nil:
		return failureInPlaceOfTheFile(stdout)
	case ctx.Err() != nil:
		return &Error{Kind: Interrupted, Message: "interrupted"}
	case givenUp.Load() != nil:
		return &Error{Kind: Network, Message: *givenUp.Load()}
	case errors.As(err, &exitErr) && exitErr.ExitCode() == -1:
		return endedBySignal(exitErr)
	case errors.As(err, &exitErr):
		return downloadFailure(exitErr.ExitCode(), stdout)
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return &Error{Kind: NotInstalled, Message: "the Basecamp CLI was not found: " + err.Error()}
	default:
		return &Error{Kind: API, Message: "the Basecamp CLI could not be run: " + err.Error()}
	}
}

// command builds the call of the Basecamp CLI. "--out -" makes it stream the
// file instead of choosing a name and a place for it. The address stands
// behind "--", and the CLI is told not to ask anything.
func (d Downloader) command(ctx context.Context, address string) *exec.Cmd {
	binary := d.Binary
	if binary == "" {
		binary = "basecamp"
	}
	cmd := exec.CommandContext(ctx, binary, "files", "download", "--out", "-", "--agent", "--", address)
	cmd.Env = append(os.Environ(), "BASECAMP_NONINTERACTIVE=1")
	cmd.WaitDelay = childWaitDelay
	return cmd
}

// watchedOutput passes on what the Basecamp CLI prints, says that the
// download is alive with every piece, and keeps the end of it: a failure
// stands there, instead of the file or behind a part of it.
type watchedOutput struct {
	into   io.Writer
	alive  func()
	failed error
	total  int64
	tail   []byte
}

func (o *watchedOutput) Write(content []byte) (int, error) {
	o.alive()
	o.tail = append(o.tail, content...)
	if len(o.tail) > maxKeptOutput {
		o.tail = append(o.tail[:0], o.tail[len(o.tail)-maxKeptOutput:]...)
	}
	o.total += int64(len(content))

	written, err := o.into.Write(content)
	if err != nil {
		o.failed = err
	}
	return written, err
}

// fileFailures is what a reason says per class of failure. It never says what
// the Basecamp CLI printed: a message of it may quote the address it asked
// for, which names the file.
var fileFailures = map[Kind]string{
	NotFound:     "Basecamp no longer has the file",
	AuthRequired: "the Basecamp CLI is not signed in",
	Forbidden:    "Basecamp forbids the download",
	RateLimited:  "Basecamp limits the requests of the account",
	Network:      "the network failed during the download",
	Unavailable:  "Basecamp, or a gateway in front of it, failed",
}

// downloadFailure reads why a download failed. The exit code decides that it
// did, whatever stands on standard output, and the failure object says the
// class, as for every request.
//
// A download that breaks off leaves a part of the file and the failure object
// behind it. Such an end counts for a failure that may pass only: that the
// file is gone is said before its first byte, and a run never asks for a file
// again that it has called gone.
func downloadFailure(exitCode int, stdout *watchedOutput) *Error {
	envelope := envelopeAtEnd(stdout.tail)
	failed := failure(exitCode, envelope, nil)
	if standsAlone := int64(len(envelope)) == stdout.total; !standsAlone && !failed.Transient() {
		failed.Kind = API
	}

	if reason, known := fileFailures[failed.Kind]; known {
		failed.Message = reason
	} else {
		failed.Message = fmt.Sprintf("the Basecamp CLI ended the download with exit code %d", exitCode)
	}
	return failed
}

// failureInPlaceOfTheFile marks a download that ended well and printed a
// failure object and nothing else: that is what the Basecamp CLI says instead
// of a file. It is no proof that a file is gone, which takes the exit code as
// well, and none that it is no file: whoever asked decides by its size.
func failureInPlaceOfTheFile(stdout *watchedOutput) error {
	envelope := envelopeAtEnd(stdout.tail)
	var answer struct {
		OK *bool `json:"ok"`
	}
	if int64(len(envelope)) != stdout.total || json.Unmarshal(envelope, &answer) != nil || answer.OK == nil || *answer.OK {
		return nil
	}
	return ErrLooksLikeAFailure
}

// envelopeAtEnd finds a failure object at the end of what was printed.
func envelopeAtEnd(tail []byte) []byte {
	end := len(tail)
	for tries := 0; tries < maxEnvelopeStarts; tries++ {
		start := bytes.LastIndexByte(tail[:end], '{')
		if start < 0 {
			return nil
		}
		if json.Valid(tail[start:]) {
			var answer struct {
				OK *bool `json:"ok"`
			}
			if json.Unmarshal(tail[start:], &answer) == nil && answer.OK != nil {
				return tail[start:]
			}
		}
		end = start
	}
	return nil
}
