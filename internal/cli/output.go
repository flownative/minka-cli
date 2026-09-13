package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Response is the JSON envelope of a success, the same shape basecamp-cli
// prints. Data is the resource exactly as the API sent it.
type Response struct {
	OK          bool         `json:"ok"`
	Data        any          `json:"data,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Notice      string       `json:"notice,omitempty"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs,omitempty"`
}

// Breadcrumb suggests a command that commonly follows.
type Breadcrumb struct {
	Action      string `json:"action"`
	Cmd         string `json:"cmd"`
	Description string `json:"description"`
}

// ErrorResponse is the JSON envelope of a failure. It is printed for --quiet
// as well, so a client always finds the code.
type ErrorResponse struct {
	OK        bool           `json:"ok"`
	Error     string         `json:"error"`
	Code      string         `json:"code"`
	Retryable bool           `json:"retryable"`
	Hint      string         `json:"hint,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"` // apiCode and httpStatus when the server answered
}

type format int

const (
	formatText format = iota
	formatJSON
	formatQuiet
)

// format decides between text and JSON. Without a flag, a terminal gets text
// and anything else, such as an agent reading the output, gets JSON.
func (a *app) format() format {
	switch {
	case a.quietFlag:
		return formatQuiet
	case a.jsonFlag:
		return formatJSON
	case a.styledFlag || a.stdoutIsTerminal():
		return formatText
	default:
		return formatJSON
	}
}

type success struct {
	data        any
	summary     string
	notices     []string
	breadcrumbs []Breadcrumb
	text        func(w io.Writer)
}

// ok prints a success. Notices go into the envelope for --json and to stderr
// otherwise, so that --quiet keeps stdout to the bare data without losing them.
func (a *app) ok(s success) error {
	switch a.format() {
	case formatQuiet:
		a.warn(s.notices)
		a.writeJSON(s.data)
	case formatJSON:
		a.writeJSON(Response{
			OK:          true,
			Data:        s.data,
			Summary:     s.summary,
			Notice:      strings.Join(s.notices, "; "),
			Breadcrumbs: s.breadcrumbs,
		})
	default:
		a.warn(s.notices)
		s.text(a.stdout)
	}
	return nil
}

func (a *app) warn(notices []string) {
	for _, notice := range notices {
		fmt.Fprintf(a.stderr, "Warning: %s\n", notice)
	}
}

func (a *app) fail(e *Error) {
	if a.format() != formatText {
		response := ErrorResponse{OK: false, Error: e.Message, Code: e.Code, Retryable: e.Retryable, Hint: e.Hint}
		if e.APICode != "" || e.HTTPStatus != 0 {
			response.Meta = map[string]any{"apiCode": e.APICode, "httpStatus": e.HTTPStatus}
		}
		a.writeJSON(response)
		return
	}

	message := e.Message
	if e.APICode != "" && e.Code == CodeValidation {
		message += " (" + e.APICode + ")"
	}
	fmt.Fprintf(a.stderr, "Error: %s\n", message)
	if e.Hint != "" {
		fmt.Fprintf(a.stderr, "Hint: %s\n", e.Hint)
	}
}

// writeJSON prints indented JSON. HTML stays unescaped, so a description
// reads as the server stores it.
func (a *app) writeJSON(value any) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(a.stderr, "Error: encoding the output: %s\n", err)
		return
	}
	_, _ = a.stdout.Write(buf.Bytes())
}
