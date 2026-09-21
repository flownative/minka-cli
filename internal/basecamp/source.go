// Package basecamp reads a Basecamp account through the Basecamp CLI and
// turns what it finds into the records of an import bundle.
package basecamp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// Source answers GET requests to the Basecamp API.
type Source interface {
	// Get returns one page of a path as the JSON the API sent. The path is
	// relative to the account, such as "projects/1.json?page=2".
	Get(ctx context.Context, path string) ([]byte, error)
}

// Kind is the class of a failure, so that a caller can tell a missing project
// from a missing sign-in without reading the message.
type Kind string

const (
	NotInstalled Kind = "not_installed"
	NotFound     Kind = "not_found"
	AuthRequired Kind = "auth_required"
	Forbidden    Kind = "forbidden"
	RateLimited  Kind = "rate_limited"
	Network      Kind = "network"
	// Unavailable is a failure of Basecamp, or of a gateway in front of it,
	// that says nothing about the request and may be gone a moment later.
	Unavailable Kind = "unavailable"
	Interrupted Kind = "interrupted"
	API         Kind = "api"
)

type Error struct {
	Kind    Kind
	Message string
	// Hint is the next step, where it is another one than the class has.
	Hint string
}

func (e *Error) Error() string {
	return e.Message
}

// Transient tells that the failure may pass by itself, so that the same
// request is worth asking again.
func (e *Error) Transient() bool {
	return e.Kind == Network || e.Kind == RateLimited || e.Kind == Unavailable
}

// CLI is the Source that runs the Basecamp CLI. The Basecamp CLI brings the
// sign-in, the refreshing of the token, rate limiting and retries.
type CLI struct {
	// Binary is the name or path of the Basecamp CLI; "basecamp" when empty.
	Binary string
}

func (c CLI) Get(ctx context.Context, path string) ([]byte, error) {
	cmd := c.command(ctx, path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		if !json.Valid(stdout.Bytes()) {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("the Basecamp CLI answered %s with something that is no JSON", path)}
		}
		return stdout.Bytes(), nil
	}

	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return nil, &Error{Kind: Interrupted, Message: "interrupted"}
	case errors.As(err, &exitErr) && exitErr.ExitCode() == -1:
		return nil, endedBySignal(exitErr)
	case errors.As(err, &exitErr):
		return nil, failure(exitErr.ExitCode(), stdout.Bytes(), stderr.Bytes())
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return nil, &Error{Kind: NotInstalled, Message: "the Basecamp CLI was not found: " + err.Error()}
	default:
		return nil, &Error{Kind: API, Message: "the Basecamp CLI could not be run: " + err.Error()}
	}
}

// endedBySignal tells a run that was stopped from a Basecamp CLI that died.
// Ctrl-C and a termination reach the Basecamp CLI as well as minka, and it may
// be gone before the context says so. Any other signal, such as the one of a
// system out of memory, is a failure somebody has to see.
func endedBySignal(exitErr *exec.ExitError) *Error {
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if ok && status.Signaled() && (status.Signal() == syscall.SIGINT || status.Signal() == syscall.SIGTERM) {
		return &Error{Kind: Interrupted, Message: "interrupted: the Basecamp CLI was stopped by a signal"}
	}
	if ok && status.Signaled() {
		return &Error{Kind: API, Message: "the Basecamp CLI was ended by a signal: " + status.Signal().String()}
	}
	return &Error{Kind: API, Message: "the Basecamp CLI ended without an exit code"}
}

// command builds the call of the Basecamp CLI. The path stands behind "--", so
// that nothing in it is ever read as a flag, and the CLI is told not to ask
// anything: a question would hang a run nobody watches.
func (c CLI) command(ctx context.Context, path string) *exec.Cmd {
	binary := c.Binary
	if binary == "" {
		binary = "basecamp"
	}
	cmd := exec.CommandContext(ctx, binary, "api", "get", "--agent", "--", path)
	cmd.Env = append(os.Environ(), "BASECAMP_NONINTERACTIVE=1")
	return cmd
}

const maxFailureText = 500

// failureKinds are the classes the Basecamp CLI tells apart, by its exit code
// and by the code it prints.
var failureKinds = map[int]struct {
	code string
	kind Kind
}{
	2: {"not_found", NotFound},
	3: {"auth_required", AuthRequired},
	4: {"forbidden", Forbidden},
	5: {"rate_limit", RateLimited},
	6: {"network", Network},
}

// The Basecamp CLI ends a failure of the API with this exit code and code.
const (
	apiFailureExit = 7
	apiFailureCode = "api_error"
)

// statusPattern reads the HTTP status out of the messages the Basecamp CLI
// makes up itself for a failure of the API. It names the status nowhere else,
// and a message that Basecamp sent instead names none. The whole message has
// to match: a text that merely quotes a status proves nothing.
var statusPattern = regexp.MustCompile(`^(?:Request failed \(HTTP |Server error \(|Gateway error \()([0-9]{3})\)$`)

// passes tells whether a failure of the API may be gone a moment later: the
// Basecamp CLI says so itself, or the status is one of a server or a gateway
// that failed, of a request that timed out or of too many requests. The
// Basecamp CLI calls a status it does not know final, such as the one of a
// gateway that lost its connection to Basecamp, so its word alone is not
// enough.
func passes(retryable bool, message string) bool {
	if retryable {
		return true
	}
	match := statusPattern.FindStringSubmatch(message)
	if match == nil {
		return false
	}
	status, _ := strconv.Atoi(match[1])
	return (status >= 500 && status <= 599) || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests
}

// failure reads what the Basecamp CLI said when it exited with an error: an
// object {ok: false, error, code, retryable} on stdout, and an exit code per
// class.
//
// The exit code alone proves nothing: a program that crashes exits 2 as well,
// and a crash must not read as "not found". So a class counts only when the
// failure object stands on stdout and its code, where it has one, says the
// same. Everything else is an error that carries what stood on stderr.
func failure(exitCode int, stdout, stderr []byte) *Error {
	var answer struct {
		OK        *bool  `json:"ok"`
		Error     string `json:"error"`
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	}
	if json.Unmarshal(stdout, &answer) != nil || answer.OK == nil || *answer.OK {
		// What stands on stdout may be the data of a request that failed
		// halfway, names and addresses included, so it is never quoted.
		text := strings.TrimSpace(string(stderr))
		if text == "" {
			text = fmt.Sprintf("it printed %d bytes that are no failure object, and nothing on stderr", len(stdout))
		}
		if len(text) > maxFailureText {
			text = strings.ToValidUTF8(text[:maxFailureText], "") + " …"
		}
		return &Error{Kind: API, Message: fmt.Sprintf("the Basecamp CLI exited with code %d: %s", exitCode, text)}
	}

	message := answer.Error
	if message == "" {
		message = fmt.Sprintf("the Basecamp CLI exited with code %d", exitCode)
	}
	e := &Error{Kind: API, Message: "Basecamp: " + message}
	if class, known := failureKinds[exitCode]; known && (answer.Code == "" || answer.Code == class.code) {
		e.Kind = class.kind
	}
	if exitCode == apiFailureExit && (answer.Code == "" || answer.Code == apiFailureCode) && passes(answer.Retryable, answer.Error) {
		e.Kind = Unavailable
	}
	return e
}
