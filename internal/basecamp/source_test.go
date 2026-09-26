package basecamp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// pages answers a list from canned pages and an empty list behind them.
type pages struct {
	answers  []string
	requests []string
}

func (p *pages) Get(_ context.Context, path string) ([]byte, error) {
	p.requests = append(p.requests, path)
	if len(p.requests) > len(p.answers) {
		return []byte(`[]`), nil
	}
	return []byte(p.answers[len(p.requests)-1]), nil
}

func TestAListIsReadPageByPageUntilAPageIsEmpty(t *testing.T) {
	source := &pages{answers: []string{`[{"id":1},{"id":2}]`, `[{"id":3}]`}}

	people, err := getList[person](t.Context(), source, "people.json")
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, p := range people {
		ids = append(ids, p.ID)
	}
	if !slices.Equal(ids, []int64{1, 2, 3}) {
		t.Errorf("got the people %v", ids)
	}
	want := []string{"people.json?page=1", "people.json?page=2", "people.json?page=3"}
	if !slices.Equal(source.requests, want) {
		t.Errorf("asked for %v, want %v", source.requests, want)
	}
}

func TestAPageNumberJoinsAQueryThatIsAlreadyThere(t *testing.T) {
	source := &pages{}
	if _, err := getList[person](t.Context(), source, "recordings.json?type=Message"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"recordings.json?type=Message&page=1"}; !slices.Equal(source.requests, want) {
		t.Errorf("asked for %v, want %v", source.requests, want)
	}
}

func TestAListThatIgnoresThePageNumberEndsInAnError(t *testing.T) {
	same := `[{"id":1}]`
	source := &pages{answers: []string{same, same, same, same, same}}

	_, err := getList[person](t.Context(), source, "people.json")
	var failed *Error
	if !errors.As(err, &failed) || failed.Kind != API {
		t.Fatalf("got %v, want an API error", err)
	}
	if len(source.requests) != 2 {
		t.Errorf("asked %d times, want it to stop at the second page", len(source.requests))
	}
	// A chat that grows by a page between two requests looks the same.
	if !strings.Contains(failed.Message, "page 2 of people.json") || !strings.Contains(failed.Message, "moved under the walk") {
		t.Errorf("the message does not say which page, or that the list may have moved: %s", failed.Message)
	}
}

// reusing answers every page out of the same memory, as a source may.
type reusing struct {
	pages
	buffer []byte
}

func (r *reusing) Get(ctx context.Context, path string) ([]byte, error) {
	content, err := r.pages.Get(ctx, path)
	r.buffer = append(r.buffer[:0], content...)
	return r.buffer, err
}

// A chat has thousands of pages' worth: a page is read into its entries when
// it arrives, and nothing of what the source sent is held on to.
func TestAPageIsLetGoOfOnceItIsRead(t *testing.T) {
	source := &reusing{pages: pages{answers: []string{`[{"id":1}]`, `[{"id":2}]`, `[{"id":3}]`}}}
	people, err := getList[person](t.Context(), source, "people.json")
	if err != nil {
		t.Fatalf("pages that share their memory: %v", err)
	}
	if len(people) != 3 || people[0].ID != 1 || people[2].ID != 3 {
		t.Errorf("got the people %v", people)
	}

	// What cannot be read ends the walk at its page, not after the last one.
	broken := &pages{answers: []string{`[{"id":"one"}]`, `[{"id":2}]`}}
	_, err = getList[person](t.Context(), broken, "people.json")
	var failed *Error
	if !errors.As(err, &failed) || failed.Kind != API || len(broken.requests) != 1 {
		t.Errorf("got %v after %d requests, want an API error at the first page", err, len(broken.requests))
	}
}

func TestTheBasecampCLIIsCalledForOnePageWithoutQuestions(t *testing.T) {
	cmd := CLI{}.command(t.Context(), "projects/7001/people.json?page=2")

	want := []string{"basecamp", "api", "get", "--agent", "--", "projects/7001/people.json?page=2"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("argv %v, want %v", cmd.Args, want)
	}
	if !slices.Contains(cmd.Env, "BASECAMP_NONINTERACTIVE=1") {
		t.Error("the environment does not tell the Basecamp CLI to ask nothing")
	}

	named := CLI{Binary: "/opt/tools/basecamp"}.command(t.Context(), "people.json")
	if named.Args[0] != "/opt/tools/basecamp" {
		t.Errorf("ran %q, want the binary that was named", named.Args[0])
	}
}

func TestAFailureOfTheBasecampCLIKeepsItsClassAndMessage(t *testing.T) {
	answer := func(code string) []byte {
		return []byte(fmt.Sprintf(`{"ok":false,"error":"what went wrong","code":%q}`, code))
	}
	cases := []struct {
		exit int
		code string
		want Kind
	}{
		{2, "not_found", NotFound},
		{3, "auth_required", AuthRequired},
		{4, "forbidden", Forbidden},
		{5, "rate_limit", RateLimited},
		{6, "network", Network},
		{7, "api_error", API},
		{1, "usage", API},
	}
	for _, c := range cases {
		got := failure(c.exit, answer(c.code), nil)
		if got.Kind != c.want {
			t.Errorf("exit %d: kind %q, want %q", c.exit, got.Kind, c.want)
		}
		if got.Message != "Basecamp: what went wrong" {
			t.Errorf("exit %d: message %q", c.exit, got.Message)
		}
	}

	if got := failure(9, nil, nil); got.Kind != API || !strings.Contains(got.Message, "code 9") {
		t.Errorf("a failure without any output: %q, %q", got.Kind, got.Message)
	}
}

// A program that crashes exits 2 as well. Without the failure object on stdout
// an exit code says nothing about Basecamp.
func TestAnExitCodeAloneIsNoClass(t *testing.T) {
	trace := "panic: runtime error: index out of range\n\ngoroutine 1 [running]:\n" + strings.Repeat("main.walk(...)\n", 200)
	for name, c := range map[string]struct{ stdout, stderr string }{
		"a crash":                    {"", trace},
		"an answer that is no JSON":  {"Usage: basecamp api get <path>", ""},
		"an object that is a record": {`{"id": 1, "error": "a field of the record"}`, ""},
		"an object that says ok":     {`{"ok": true, "error": "", "code": "not_found"}`, ""},
		"a code of another class":    {`{"ok": false, "error": "what went wrong", "code": "api_error"}`, ""},
	} {
		for exit := 2; exit <= 6; exit++ {
			got := failure(exit, []byte(c.stdout), []byte(c.stderr))
			if got.Kind != API {
				t.Errorf("%s, exit %d: kind %q, want an API error", name, exit, got.Kind)
			}
		}
	}

	got := failure(2, nil, []byte(trace))
	if !strings.Contains(got.Message, "panic: runtime error") || !strings.Contains(got.Message, "code 2") {
		t.Errorf("the message does not carry what was printed: %q", got.Message)
	}
	if len(got.Message) > maxFailureText+100 {
		t.Errorf("the message is %d bytes long", len(got.Message))
	}

	// What stands on stdout may be the data of a request that failed halfway.
	for _, stderr := range []string{"", "  \n"} {
		data := `[{"id": 101, "name": "Ada Example", "email_address": "ada@example.test"}]`
		got := failure(1, []byte(data), []byte(stderr))
		for _, leaked := range []string{"Ada", "Example", "ada@", "101", "email"} {
			if strings.Contains(got.Message, leaked) {
				t.Errorf("the message quotes stdout (%q): %s", leaked, got.Message)
			}
		}
		if got.Kind != API || !strings.Contains(got.Message, "code 1") || !strings.Contains(got.Message, fmt.Sprintf("%d bytes", len(data))) {
			t.Errorf("the message does not say what happened: %q, %s", got.Kind, got.Message)
		}
	}

	// An object without a code still counts: the exit code says the class.
	if got := failure(2, []byte(`{"ok": false, "error": "Resource not found"}`), nil); got.Kind != NotFound {
		t.Errorf("a failure object without a code: kind %q", got.Kind)
	}
}

func TestAMissingBasecampCLIIsToldApartFromAFailedRequest(t *testing.T) {
	for _, binary := range []string{"minka-test-no-such-basecamp-cli", filepath.Join(t.TempDir(), "basecamp")} {
		_, err := CLI{Binary: binary}.Get(t.Context(), "people.json")
		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != NotInstalled {
			t.Errorf("%s: got %v, want not installed", binary, err)
		}
	}
}

// standIn writes a script that takes the place of the Basecamp CLI: it prints
// its last argument the way the real one prints an answer, or fails the way
// the real one does.
func standIn(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	path := filepath.Join(t.TempDir(), "basecamp")
	script := `#!/bin/sh
for last; do :; done
case "$last" in
	slow.json) exec sleep 5 ;;
	killed.json) kill -TERM $$ ;;
	interrupted.json) kill -INT $$ ;;
	oom.json) kill -KILL $$ ;;
	crash.json) echo "panic: runtime error" >&2; exit 2 ;;
	ok.json) printf '{"asked":"%s","quiet":"%s"}' "$*" "$BASECAMP_NONINTERACTIVE" ;;
	text.json) echo "Welcome to Basecamp" ;;
	missing.json) echo '{"ok":false,"error":"Resource not found","code":"not_found"}'; exit 2 ;;
	signin.json) echo '{"ok":false,"error":"Not authenticated","code":"auth_required"}'; exit 3 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnAnswerOfTheBasecampCLIIsJSONOrAFailure(t *testing.T) {
	cli := CLI{Binary: standIn(t)}

	content, err := cli.Get(t.Context(), "ok.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"asked":"api get --agent -- ok.json","quiet":"1"}`; string(content) != want {
		t.Errorf("got %s, want %s", content, want)
	}

	_, err = cli.Get(t.Context(), "text.json")
	expectKind(t, err, API)
	_, err = cli.Get(t.Context(), "missing.json")
	expectKind(t, err, NotFound)
	_, err = cli.Get(t.Context(), "signin.json")
	expectKind(t, err, AuthRequired)
	if err.Error() != "Basecamp: Not authenticated" {
		t.Errorf("message %q", err)
	}
}

func TestARunThatIsStoppedIsInterruptedNotFailed(t *testing.T) {
	cli := CLI{Binary: standIn(t)}

	// Stopped before the Basecamp CLI was started.
	stopped, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := cli.Get(stopped, "ok.json")
	expectKind(t, err, Interrupted)

	// Stopped while it runs.
	running, cancel := context.WithCancel(t.Context())
	time.AfterFunc(100*time.Millisecond, cancel)
	started := time.Now()
	_, err = cli.Get(running, "slow.json")
	expectKind(t, err, Interrupted)
	if time.Since(started) > 3*time.Second {
		t.Error("the Basecamp CLI was waited for instead of stopped")
	}

	// Ctrl-C and a termination reach the Basecamp CLI too, and it may be gone
	// first.
	_, err = cli.Get(t.Context(), "killed.json")
	expectKind(t, err, Interrupted)
	_, err = cli.Get(t.Context(), "interrupted.json")
	expectKind(t, err, Interrupted)
}

func TestABasecampCLIThatDiesIsAFailureNotAnInterruption(t *testing.T) {
	cli := CLI{Binary: standIn(t)}

	_, err := cli.Get(t.Context(), "oom.json")
	expectKind(t, err, API)
	if !strings.Contains(err.Error(), "killed") {
		t.Errorf("the message does not name the signal: %s", err)
	}

	_, err = cli.Get(t.Context(), "crash.json")
	expectKind(t, err, API)
	if !strings.Contains(err.Error(), "panic: runtime error") {
		t.Errorf("the message does not carry the crash: %s", err)
	}
}

func TestPlainTextBecomesParagraphsAndLineBreaks(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"  \n ":                      "",
		"One line":                   "<p>One line</p>",
		"a < b & \"c\"":              "<p>a &lt; b &amp; &#34;c&#34;</p>",
		"First\nsecond":              "<p>First<br>second</p>",
		"First\r\nsecond":            "<p>First<br>second</p>",
		"One\n\nTwo\n\n\n\nThree\n":  "<p>One</p><p>Two</p><p>Three</p>",
		"<script>alert(1)</script>":  "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>",
		"Trailing space \n\n  Two  ": "<p>Trailing space</p><p>Two</p>",
	}
	for text, want := range cases {
		if got := textToHTML(text); got != want {
			t.Errorf("textToHTML(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestTheShapeOfAnAddressIsCheckedPlainly(t *testing.T) {
	for address, want := range map[string]bool{
		"ada@example.test":                  true,
		"ada.example+tag@mail.example.test": true,
		"ädä@exämple.test":                  true,
		"ada":                               false,
		"@example.test":                     false,
		"ada@":                              false,
		"ada@example":                       false,
		"ada@.test":                         false,
		"ada@example.":                      false,
		"ada@@example.test":                 false,
		"ada@example@example.test":          false,
		"ada @example.test":                 false,
		"ada@example.test\n":                false,
		"ada@exam\tple.test":                false,
		"ada@example..test":                 false,
		"ada@sub.example.test":              true,
		"ada@my-example.test":               true,
		"ada@example.t3st":                  true,
		"a***@*******.****":                 false,
		"a\u25cf\u25cf@\u25cf\u25cf\u25cf.\u25cf\u25cf": false,
		"a\u2219\u2219@\u2219\u2219\u2219.\u2219\u2219": false,
		"a\u2022\u2022@\u2022\u2022\u2022.\u2022\u2022": false,
		"ada@example_.test":                             false,
		"ada@exam/ple.test":                             false,
	} {
		if got := looksLikeAnAddress(address); got != want {
			t.Errorf("looksLikeAnAddress(%q) = %v, want %v", address, got, want)
		}
	}
}

// goneAfterAPage answers one page and then says the list is not there, as a place
// does that Basecamp loses between two requests.
type goneAfterAPage struct {
	kind     Kind
	answered int
}

func (g *goneAfterAPage) Get(_ context.Context, _ string) ([]byte, error) {
	g.answered++
	if g.answered > 1 {
		return nil, &Error{Kind: g.kind, Message: "Basecamp says no"}
	}
	return []byte(`[{"id":1},{"id":2}]`), nil
}

// A list that is counted and not written keeps the pages that did arrive: a place
// Basecamp loses halfway would otherwise take the records it had already shown
// with it, and the number of what fell would be quietly too low.
func TestCountingKeepsWhatCameBeforeAPlaceWasGone(t *testing.T) {
	source := &goneAfterAPage{kind: NotFound}
	counted, err := listedForCounting[person](t.Context(), source, "buckets/1/todolists/2/todos.json")
	if err != nil {
		t.Fatalf("a place that is not there ended the counting: %v", err)
	}
	if len(counted) != 2 {
		t.Errorf("counted %d records, want the two that arrived before the place was gone", len(counted))
	}

	// Every other failure is what it is anywhere, and hands back what it read all
	// the same.
	source = &goneAfterAPage{kind: Forbidden}
	counted, err = listedForCounting[person](t.Context(), source, "buckets/1/todolists/2/todos.json")
	var failed *Error
	if !errors.As(err, &failed) || failed.Kind != Forbidden {
		t.Errorf("a refusal came back as %v, want forbidden", err)
	}
	if len(counted) != 2 {
		t.Errorf("counted %d records of a refusal, want the two that arrived", len(counted))
	}
}
