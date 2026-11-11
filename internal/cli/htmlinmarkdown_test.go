package cli

import (
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
	"github.com/flownative/minka-cli/internal/markdown"
)

// htmlWrite is a write that takes a text as Markdown, with the flag that
// sends the same text as HTML instead.
type htmlWrite struct {
	args       func(f *fakeapi.Fixture, text string) []string
	formatFlag string
}

func htmlWrites() map[string]htmlWrite {
	return map[string]htmlWrite{
		"comments create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"comments", "create", f.CardInTriage.ID, text}
		}, "--format"},
		"posts comments create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"posts", "comments", "create", f.Post.ID, text}
		}, "--format"},
		"messages create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"messages", "create", f.Chat.ID, text}
		}, "--format"},
		"checkins answers create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"checkins", "answers", "create", f.DailyQuestion.ID, text}
		}, "--format"},
		"cards create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"cards", "create", "--board", f.Board.ID, "--title", "New", "--description", text}
		}, "--description-format"},
		"posts create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", "New", "--audience", "nobody", "--body", text}
		}, "--body-format"},
		"docs create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"docs", "create", "--tool", f.Files.ID, "--title", "A page", "--audience", "nobody", "--body", text}
		}, "--body-format"},
		"docs uploads edit": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"docs", "uploads", "edit", f.FilesUpload.ID, "--description", text, "--force"}
		}, "--description-format"},
		"todolists create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"todolists", "create", "--tool", f.Todos.ID, "--title", "A list", "--description", text}
		}, "--description-format"},
		"todos create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"todos", "create", "--list", f.TodoList.ID, "--title", "A to-do", "--notes", text}
		}, "--notes-format"},
		"decisions create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"decisions", "create", "--tool", f.Decisions.ID, "--title", "New", "--body", text}
		}, "--body-format"},
		"events create": {func(f *fakeapi.Fixture, text string) []string {
			return []string{"events", "create", "--tool", f.Calendar.ID, "--title", "New", "--first-day", "2026-10-20", "--description", text}
		}, "--description-format"},
	}
}

const textInHTML = "<p>Done.</p>\n<ul>\n<li>the first</li>\n<li>the second</li>\n</ul>"

func TestATextOfHTMLGivenAsMarkdownIsRefusedAndNothingIsSent(t *testing.T) {
	for name, write := range htmlWrites() {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON("", write.args(e.fixture, textInHTML)...)
			expectCode(t, r, ExitUsage)
			if env.Code != CodeUsage {
				t.Errorf("code %q", env.Code)
			}
			if !strings.Contains(env.Hint, write.formatFlag+" html") || !strings.Contains(env.Hint, "```") {
				t.Errorf("hint %q does not name %s html and the fenced code block", env.Hint, write.formatFlag)
			}
			if writes := e.server.WritingRequests(); len(writes) != 0 {
				t.Errorf("%d writes sent: %v", len(writes), writes)
			}
		})
	}
}

func TestATextOfHTMLGivenAsMarkdownOnStdinIsRefusedAndNothingIsSent(t *testing.T) {
	for name, write := range htmlWrites() {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON(textInHTML, write.args(e.fixture, "-")...)
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Hint, write.formatFlag+" html") {
				t.Errorf("hint %q does not name %s html", env.Hint, write.formatFlag)
			}
			if writes := e.server.WritingRequests(); len(writes) != 0 {
				t.Errorf("%d writes sent: %v", len(writes), writes)
			}
		})
	}
}

func TestATextOfHTMLGivenAsHTMLIsSent(t *testing.T) {
	for name, write := range htmlWrites() {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			_, r := e.runJSON("", append(write.args(e.fixture, textInHTML), write.formatFlag, "html")...)
			expectCode(t, r, ExitOK)
			if writes := e.server.WritingRequests(); len(writes) == 0 {
				t.Error("nothing was sent")
			}
		})
	}
}

// Below the threshold, HTML in Markdown is shown as text and a notice says so,
// and HTML in a fenced code block is code as any other.
func TestATextWithSomeHTMLIsSentAsMarkdown(t *testing.T) {
	for name, text := range map[string]string{
		"one HTML block in a longer text": "Done, with two changes:\n\n- the first\n- the second\n\n<details>open</details>",
		"HTML in a fenced code block":     "```html\n" + textInHTML + "\n```",
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, text)
			expectCode(t, r, ExitOK)
			if writes := e.server.WritingRequests(); len(writes) != 1 {
				t.Errorf("%d writes sent, want 1", len(writes))
			}
			if strings.Contains(text, "<details>") && !strings.Contains(env.Notice, markdown.WarningRawHTML) {
				t.Errorf("notice %q", env.Notice)
			}
		})
	}
}
