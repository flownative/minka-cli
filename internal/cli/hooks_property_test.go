package cli

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// settingsMaker writes settings files of every shape the old hook can stand
// in, spaced and escaped at random, and remembers what has to come out of a
// removal unchanged. It is seeded, so every run makes the same files.
type settingsMaker struct {
	random *rand.Rand
	// kept are texts that have to stand in the result byte for byte.
	kept []string
	n    int
}

func (m *settingsMaker) space() string {
	spaces := []string{"", " ", "  ", "\n", "\r\n", "\t", "\n    ", "\r\n\t\t", " \n "}
	return spaces[m.random.Intn(len(spaces))]
}

// text writes a JSON string, escaping some of its letters where it need not.
func (m *settingsMaker) text(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c < 0x20:
			fmt.Fprintf(&b, `\u%04x`, c)
		case m.random.Intn(6) == 0 && c < 0x10000:
			fmt.Fprintf(&b, `\u%04x`, c)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (m *settingsMaker) object(members [][2]string) string {
	if len(members) == 0 {
		return "{" + m.space() + "}"
	}
	var b strings.Builder
	b.WriteString("{")
	for i, member := range members {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(m.space() + m.text(member[0]) + m.space() + ":" + m.space() + member[1] + m.space())
	}
	b.WriteString("}")
	return b.String()
}

func (m *settingsMaker) array(items []string) string {
	if len(items) == 0 {
		return "[" + m.space() + "]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, item := range items {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(m.space() + item + m.space())
	}
	b.WriteString("]")
	return b.String()
}

var (
	commandsLikeMinkas = []string{"minka heartbeat", "/opt/homebrew/bin/minka heartbeat", "/Users/a b/minka heartbeat", `"/x y/minka" heartbeat`, "  minka heartbeat  "}
	otherCommands      = []string{"format.sh", "minka work list", "echo \"minka heartbeat\" > /dev/null", "minka heartbeat --json", "ünïcödé ✓ \U0001F600", "minkactl heartbeat", "a\\b", "x\ny", "nice -n 5 /opt/homebrew/bin/minka heartbeat"}
)

func (m *settingsMaker) mark() string {
	m.n++
	return fmt.Sprintf("m%d", m.n)
}

func (m *settingsMaker) hook() string {
	if m.random.Intn(3) == 0 {
		command := commandsLikeMinkas[m.random.Intn(len(commandsLikeMinkas))]
		members := [][2]string{{"type", m.text("command")}, {"command", m.text(command)}}
		if m.random.Intn(2) == 0 {
			members = append(members, [2]string{"timeout", "10"})
		}
		if m.random.Intn(2) == 0 {
			members[0], members[1] = members[1], members[0]
		}
		raw := m.object(members)
		if !isTheHookOfMinka(command) {
			m.kept = append(m.kept, raw)
		}
		return raw
	}
	if m.random.Intn(8) == 0 {
		raw := m.array([]string{m.text("minka heartbeat")})
		m.kept = append(m.kept, raw)
		return raw
	}
	command := otherCommands[m.random.Intn(len(otherCommands))]
	kind := "command"
	if m.random.Intn(5) == 0 {
		kind, command = "prompt", "minka heartbeat"
	}
	raw := m.object([][2]string{{"type", m.text(kind)}, {"command", m.text(command)}, {"id", m.text(m.mark())}})
	m.kept = append(m.kept, raw)
	return raw
}

func (m *settingsMaker) group() string {
	var hooks []string
	for range m.random.Intn(4) {
		hooks = append(hooks, m.hook())
	}
	matcher := "*"
	if m.random.Intn(4) == 0 {
		matcher = "Bash"
	}
	members := [][2]string{{"matcher", m.text(matcher)}, {"hooks", m.array(hooks)}}
	if m.random.Intn(5) == 0 {
		members = append(members, [2]string{"description", m.text(m.mark())})
	}
	if m.random.Intn(2) == 0 {
		members[0], members[1] = members[1], members[0]
	}
	return m.object(members)
}

func (m *settingsMaker) settings() string {
	var top [][2]string
	for _, key := range []string{"model", "theme", "env", "permissions"} {
		if m.random.Intn(2) == 0 {
			value := m.text(m.mark() + " minka heartbeat")
			if m.random.Intn(3) == 0 {
				value = m.array([]string{m.text("minka heartbeat"), "1", "null", m.array([]string{"true", "[]"})})
			}
			top = append(top, [2]string{key, value})
			m.kept = append(m.kept, value)
		}
	}
	switch m.random.Intn(8) {
	case 0:
		top = append(top, [2]string{"hooks", "null"})
	case 1:
	default:
		var events [][2]string
		for _, event := range []string{"PreToolUse", "PostToolUse", "Stop"} {
			if event != "PostToolUse" && m.random.Intn(2) == 0 {
				continue
			}
			var groups []string
			for range m.random.Intn(4) {
				groups = append(groups, m.group())
			}
			events = append(events, [2]string{event, m.array(groups)})
		}
		m.random.Shuffle(len(events), func(i, j int) { events[i], events[j] = events[j], events[i] })
		top = append(top, [2]string{"hooks", m.object(events)})
	}
	m.random.Shuffle(len(top), func(i, j int) { top[i], top[j] = top[j], top[i] })
	return m.space() + m.object(top) + m.space()
}

// withoutTheOldHook is what the settings mean once the rules have been applied
// to them, worked out on the decoded settings rather than on the text.
func withoutTheOldHook(settings any) any {
	root, ok := settings.(map[string]any)
	if !ok {
		return settings
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return settings
	}
	event, ok := hooks["PostToolUse"].([]any)
	if !ok {
		return settings
	}
	groups := []any{}
	for _, entry := range event {
		group, ok := entry.(map[string]any)
		list, isList := group["hooks"].([]any)
		if !ok || !isList {
			groups = append(groups, entry)
			continue
		}
		kept, removed := []any{}, false
		for _, one := range list {
			hook, ok := one.(map[string]any)
			if command, isText := hook["command"].(string); ok && isText && hook["type"] == "command" && isTheHookOfMinka(command) {
				removed = true
				continue
			}
			kept = append(kept, one)
		}
		if !removed {
			groups = append(groups, entry)
			continue
		}
		if len(kept) == 0 && len(group) == 2 && group["matcher"] == "*" {
			continue
		}
		group["hooks"] = kept
		groups = append(groups, group)
	}
	if len(groups) == 0 && len(event) > 0 {
		delete(hooks, "PostToolUse")
		if len(hooks) == 0 {
			delete(root, "hooks")
		}
		return root
	}
	hooks["PostToolUse"] = groups
	return root
}

// Whatever the shape of the settings, taking the old hook out means what the
// rules say, leaves JSON, only ever deletes text, keeps what is not minka's
// byte for byte, and changes nothing the second time.
func TestTakingTheOldHookOutHoldsForSettingsOfEveryShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for seed := range 3000 {
		maker := &settingsMaker{random: rand.New(rand.NewSource(int64(seed)))}
		before := maker.settings()
		if !json.Valid([]byte(before)) {
			t.Fatalf("seed %d made no JSON:\n%s", seed, before)
		}
		if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
			t.Fatal(err)
		}

		hook, err := removeTheOldHook(path)
		if err != nil {
			t.Fatalf("seed %d: %v\n%s", seed, err, before)
		}
		after := readFile(t, path)
		if !json.Valid([]byte(after)) {
			t.Fatalf("seed %d left no JSON\nbefore:\n%s\nafter:\n%s", seed, before, after)
		}
		var meantBefore, meantAfter any
		_ = json.Unmarshal([]byte(before), &meantBefore)
		_ = json.Unmarshal([]byte(after), &meantAfter)
		if want := withoutTheOldHook(meantBefore); !reflect.DeepEqual(want, meantAfter) {
			t.Fatalf("seed %d means something else\nbefore:\n%s\nafter:\n%s", seed, before, after)
		}
		if !hook.found() && after != before {
			t.Fatalf("seed %d: no hook was found and the file changed", seed)
		}
		for _, kept := range maker.kept {
			if !strings.Contains(after, kept) {
				t.Fatalf("seed %d lost %q\nbefore:\n%s\nafter:\n%s", seed, kept, before, after)
			}
		}
		at := 0
		for i := 0; i < len(before) && at < len(after); i++ {
			if before[i] == after[at] {
				at++
			}
		}
		if at != len(after) {
			t.Fatalf("seed %d wrote something that was not there\nbefore:\n%s\nafter:\n%s", seed, before, after)
		}
		if again, err := removeTheOldHook(path); err != nil || again.found() {
			t.Fatalf("seed %d: a second run found %+v, %v", seed, again, err)
		}
	}
}

// Settings without the shape of an object of hooks are no settings to change.
func TestSettingsOfNoShapeForAHookAreLeftAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for name, c := range map[string]struct {
		before, after string
		refused       bool
	}{
		"empty":             {"", "", true},
		"with a BOM":        {"\xef\xbb\xbf" + theOldHookAlone, "\xef\xbb\xbf" + theOldHookAlone, true},
		"no members":        {"{}", "{}", false},
		"hooks as null":     {`{"hooks": null}`, `{"hooks": null}`, false},
		"an array":          {`[{"hooks": {}}]`, `[{"hooks": {}}]`, false},
		"a string":          {`"minka heartbeat"`, `"minka heartbeat"`, false},
		"space at the end":  {theOldHookAlone + "   \n\n", "{}   \n\n", false},
		"an escaped letter": {`{"hooks": {"PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "minka heartbeat"}]}]}, "x": 1}`, `{"x": 1}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(c.before), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := removeTheOldHook(path)
			if (err != nil) != c.refused {
				t.Errorf("refused: %v", err)
			}
			if got := readFile(t, path); got != c.after {
				t.Errorf("the settings are now %q, want %q", got, c.after)
			}
		})
	}
}
