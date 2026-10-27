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
	CLIUpdate   *cliUpdate   `json:"cliUpdate,omitempty"` // at most once a day, where a newer minka exists
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
	Meta      map[string]any `json:"meta,omitempty"` // apiCode and httpStatus when the server answered, reference when it named its log entry
	Data      any            `json:"data,omitempty"` // what an incomplete run got done
	Notice    string         `json:"notice,omitempty"`
	CLIUpdate *cliUpdate     `json:"cliUpdate,omitempty"`
}

type format int

const (
	formatText format = iota
	formatJSON
	formatQuiet
)

// format decides between text and JSON. Without a flag, a terminal gets text
// and anything else, such as an agent reading the output, gets JSON.
//
// Where the bytes of a file are the output, nothing may be written beside
// them: the answer is then a line of text, and the command sends it to
// standard error itself.
func (a *app) format() format {
	switch {
	case a.bytesAreTheOutput:
		return formatText
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
	if !a.skillIsTheTopic {
		if notice := a.staleSkillNotice(); notice != "" {
			s.notices = append(s.notices, notice)
		}
	}

	_, update := a.settleCLIUpdate(a.noticeChannel())
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
			CLIUpdate:   update,
		})
	default:
		a.warn(s.notices)
		s.text(a.stdout)
		if update != nil {
			writeUpdateNotice(a.stderr, update)
		}
	}
	return nil
}

// noticeChannel is where the notice of a newer minka goes in the format of
// this run. --quiet has no envelope to carry it and keeps stderr for warnings
// about the data, so it gets none.
func (a *app) noticeChannel() noticeChannel {
	switch a.format() {
	case formatJSON:
		return jsonChannel
	case formatText:
		return textChannel
	}
	return noChannel
}

func (a *app) warn(notices []string) {
	for _, notice := range notices {
		fmt.Fprintf(a.stderr, "Warning: %s\n", notice)
	}
}

func (a *app) fail(e *Error) {
	_, update := a.settleCLIUpdate(a.noticeChannel())
	if a.format() != formatText {
		response := ErrorResponse{OK: false, Error: e.Message, Code: e.Code, Retryable: e.Retryable, Hint: e.fullHint(), Data: e.Data, Notice: strings.Join(e.Notices, "; "), CLIUpdate: update}
		if e.APICode != "" || e.HTTPStatus != 0 {
			response.Meta = map[string]any{"apiCode": e.APICode, "httpStatus": e.HTTPStatus}
			if e.Reference != "" {
				response.Meta["reference"] = e.Reference
			}
		}
		a.writeJSON(response)
		return
	}

	a.warn(e.Notices)
	if e.Text != nil && !a.bytesAreTheOutput {
		e.Text(a.stdout)
	}
	message := e.Message
	if e.APICode != "" && e.Code == CodeValidation {
		message += " (" + e.APICode + ")"
	}
	fmt.Fprintf(a.stderr, "Error: %s\n", message)
	if hint := e.fullHint(); hint != "" {
		fmt.Fprintf(a.stderr, "Hint: %s\n", hint)
	}
	if update != nil {
		writeUpdateNotice(a.stderr, update)
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
