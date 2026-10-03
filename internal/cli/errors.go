package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/spf13/cobra"
)

// Error codes and exit codes. They are the ones basecamp-cli uses, so an
// agent that knows one of the two CLIs can read the other. What basecamp-cli
// has no class for starts at 11: it uses 5 for a rate limit and 10 for an
// account limit, which minka never meets.
const (
	CodeUsage      = "usage"
	CodeNotFound   = "not_found"
	CodeAuth       = "auth_required"
	CodeForbidden  = "forbidden"
	CodeNetwork    = "network"
	CodeAPI        = "api_error"
	CodeAmbiguous  = "ambiguous"
	CodeValidation = "validation"
	CodeConfig     = "config"
	// CodeUnread is a comment not written because its card has comments the
	// caller has not read. A script keys on it, so it stays as it is.
	CodeUnread = "unread"
)

const (
	ExitOK         = 0
	ExitUsage      = 1 // also for a config file that cannot be read or written
	ExitNotFound   = 2
	ExitAuth       = 3
	ExitForbidden  = 4
	ExitNetwork    = 6
	ExitAPI        = 7
	ExitAmbiguous  = 8
	ExitValidation = 9
	ExitUnread     = 11
)

var exitCodes = map[string]int{
	CodeUsage:      ExitUsage,
	CodeConfig:     ExitUsage,
	CodeNotFound:   ExitNotFound,
	CodeAuth:       ExitAuth,
	CodeForbidden:  ExitForbidden,
	CodeNetwork:    ExitNetwork,
	CodeAPI:        ExitAPI,
	CodeAmbiguous:  ExitAmbiguous,
	CodeValidation: ExitValidation,
	CodeUnread:     ExitUnread,
}

const writeMayHaveHappened = "The change may have reached the server. Read it back before trying again."

// Error is every failure the CLI reports. Code decides the exit code; the API
// code and HTTP status are kept for a client that needs the exact case, such
// as cardTitleTooLong.
type Error struct {
	Code       string
	Message    string
	Hint       string
	Retryable  bool
	APICode    string
	HTTPStatus int

	// Data is what a command got done before it failed, for a run that ends
	// incomplete: the caller still gets the result, and the exit code says
	// that something is missing from it. Text prints it for a person.
	Data    any
	Text    func(w io.Writer)
	Notices []string
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) ExitCode() int {
	if code, ok := exitCodes[e.Code]; ok {
		return code
	}
	return ExitAPI
}

func usageErrorf(cmd *cobra.Command, format string, args ...any) *Error {
	return &Error{
		Code:    CodeUsage,
		Message: fmt.Sprintf(format, args...),
		Hint:    fmt.Sprintf("Run '%s --help' for usage.", cmd.CommandPath()),
	}
}

// apiCodeProjectArchived is what Minka answers a member who writes into an
// archived project.
const apiCodeProjectArchived = "projectArchived"

// apiCodeColumnHasNoOnHoldArea is what Minka answers a move onto hold into a
// column of the card's board that carries no area for the cards on hold.
const apiCodeColumnHasNoOnHoldArea = "columnHasNoOnHoldArea"

// apiCodeVariantNotAvailable is what Minka answers for a rendering a file has
// not got, and for a name that is no rendering at all.
const apiCodeVariantNotAvailable = "variantNotAvailable"

// apiError classifies a failed request.
//
// Only a failed read is retryable. A write can have been applied before the
// connection broke or the server failed, and repeating it could create a card
// or a comment twice.
func apiError(err error, host string) *Error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		e := &Error{Code: CodeAPI, Message: apiErr.Message, APICode: apiErr.Code, HTTPStatus: apiErr.StatusCode}
		switch status := apiErr.StatusCode; {
		case status == http.StatusUnauthorized:
			e.Code = CodeAuth
			e.Hint = "The token is missing, unknown, revoked or expired, or the agent it belongs to stepped down. Run: minka auth login --host " + host
		case status == http.StatusForbidden:
			e.Code = CodeForbidden
			e.Hint = "The account of the token may not use this part of Minka."
		case status == http.StatusNotFound:
			e.Code = CodeNotFound
			e.Hint = "It does not exist, it belongs to a project you are not a member of, or its project is in the trash."
		case status == http.StatusBadRequest || status == http.StatusUnsupportedMediaType ||
			status == http.StatusRequestEntityTooLarge || status == http.StatusUnprocessableEntity ||
			status == http.StatusConflict:
			e.Code = CodeValidation
			if apiErr.Code == apiCodeProjectArchived {
				e.Hint = "The project can still be read. Nothing is written in it until it is put back in use, which is done in Minka on the web. List archived projects with: minka projects list --archived"
			}
		case status >= 500:
			if apiErr.Method == http.MethodGet {
				e.Retryable = true
			} else {
				e.Hint = writeMayHaveHappened
			}
		}
		return e
	}

	var networkErr *api.NetworkError
	if errors.As(err, &networkErr) {
		// The error of the request has the address in it with the password
		// replaced, the way the standard library prints one. A hint that named
		// the host as it stands would undo that in the same envelope.
		e := &Error{Code: CodeNetwork, Message: networkErr.Error(), Hint: "Is " + config.HostForOutput(host) + " reachable?"}
		switch {
		case errors.Is(err, context.Canceled):
			e.Message = "interrupted"
			e.Hint = ""
			if networkErr.Method != http.MethodGet {
				e.Hint = writeMayHaveHappened
			}
		case networkErr.Method == http.MethodGet:
			e.Retryable = true
		default:
			e.Hint = writeMayHaveHappened
		}
		return e
	}

	return &Error{Code: CodeAPI, Message: err.Error()}
}
