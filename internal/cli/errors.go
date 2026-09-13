package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// Error codes and exit codes. They are the ones basecamp-cli uses, so an
// agent that knows one of the two CLIs can read the other.
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
			e.Hint = "The token is missing, unknown, revoked or expired. Run: minka auth login --host " + host
		case status == http.StatusForbidden:
			e.Code = CodeForbidden
			e.Hint = "The account of the token may not use this part of Minka."
		case status == http.StatusNotFound:
			e.Code = CodeNotFound
			e.Hint = "It does not exist, or it belongs to a project you are not a member of."
		case status == http.StatusBadRequest || status == http.StatusUnsupportedMediaType || status == http.StatusUnprocessableEntity:
			e.Code = CodeValidation
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
		e := &Error{Code: CodeNetwork, Message: networkErr.Error(), Hint: "Is " + host + " reachable?"}
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
