package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The hook minka once wrote into the settings of Claude Code: after every tool
// call, a group of every tool holding one command that runs minka heartbeat.
const (
	oldHookEvent   = "PostToolUse"
	oldHookMatcher = "*"
)

// oldHook is a hook of minka found in the settings of Claude Code, and what
// became of it.
type oldHook struct {
	path    string
	command string
	removed bool
	// why says why it is still there where it could not be removed.
	why string
}

func (h oldHook) found() bool { return h.command != "" }

func (h oldHook) data() any {
	if !h.found() {
		return nil
	}
	data := map[string]any{"path": h.path, "command": h.command, "removed": h.removed}
	if h.why != "" {
		data["reason"] = h.why
	}
	return data
}

// isTheHookOfMinka says whether a command line is the one minka wrote: minka
// on the PATH, or a program called minka by its absolute path, spaces and all,
// then heartbeat and nothing else. A command line that does anything more is
// somebody else's, even where it runs minka: one that runs another program
// first, sets a variable or changes the directory.
func isTheHookOfMinka(command string) bool {
	program, ok := strings.CutSuffix(command, " heartbeat")
	if !ok {
		return false
	}
	if program == "minka" {
		return true
	}
	if unquoted := strings.TrimSuffix(strings.TrimPrefix(program, `"`), `"`); len(unquoted) == len(program)-2 {
		program = unquoted
	}
	return strings.HasPrefix(program, "/") && !strings.Contains(program, " /") && filepath.Base(program) == "minka"
}

// findTheOldHook looks for the hook of minka in the settings at path. A file
// that is not there holds none; one that cannot be read as JSON, or that names
// a key twice on the way to the hook, says so in the error, since then nobody
// can tell.
func findTheOldHook(path string) (oldHook, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return oldHook{path: path}, nil
	}
	if err != nil {
		return oldHook{path: path}, err
	}
	where, err := theOldHookIn(path, content)
	if err != nil || where == nil {
		return oldHook{path: path}, err
	}
	return oldHook{path: path, command: where.command}, nil
}

func theOldHookIn(path string, content []byte) (*hookInTheText, error) {
	root, err := parseSpans(content)
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read as JSON", path)
	}
	where, err := root.theOldHook()
	if err != nil {
		return nil, fmt.Errorf("%s %w", path, err)
	}
	return where, nil
}

// removeTheOldHook takes every hook of minka out of the settings at path and
// leaves everything else as it was written, byte for byte: the other hooks,
// every other key, the order, the indentation. What only held the hook goes
// with it — the group of every tool around it, and a list and an object of
// hooks left empty — so that the file reads as it would without the hook.
//
// The file is written whole or not at all, where a link points if it is one,
// and keeps its permissions. It is read right before, so that what Claude Code
// wrote into it a moment ago is kept.
func removeTheOldHook(path string) (oldHook, error) {
	hook, err := findTheOldHook(path)
	if err != nil || !hook.found() {
		return hook, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return hook, err
	}
	for {
		where, err := theOldHookIn(path, content)
		if err != nil {
			return hook, err
		}
		if where == nil {
			break
		}
		if content, err = cutOut(content, where.from, where.to); err != nil {
			return hook, err
		}
	}
	if err := writeSettings(path, content); err != nil {
		return hook, err
	}
	hook.removed = true
	return hook, nil
}

// writeSettings replaces the settings where they lie, keeping the permissions
// of the file. A file behind a symbolic link is written where the link points
// and stays a link.
func writeSettings(path string, content []byte) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer os.Remove(temp.Name())

	_, err = temp.Write(content)
	if err == nil {
		err = temp.Chmod(info.Mode().Perm())
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// span is one JSON value and where it stands in the text: [start, end). An
// object knows its members, an array its items.
type span struct {
	start, end int
	kind       byte
	members    []member
	items      []span
	// text is the value of a string.
	text string
}

// member is one key of an object, from its opening quote to the end of its
// value.
type member struct {
	start int
	key   string
	value span
}

func (m member) end() int { return m.value.end }

// member is the value of a key, the last one where a key stands twice, as
// Claude Code reads it.
func (s *span) member(key string) (*span, int) {
	for i := len(s.members) - 1; i >= 0; i-- {
		if s.members[i].key == key {
			return &s.members[i].value, i
		}
	}
	return nil, -1
}

// hookInTheText is where the first hook of minka stands, and what has to go
// with it.
type hookInTheText struct {
	command string
	// cut is the range of the text to take out.
	from, to int
}

// cutOut takes a range out of the settings, and refuses where what is left is
// no JSON: a file Claude Code cannot read any more would cost it every
// setting, and the hook is not worth that.
func cutOut(content []byte, from, to int) ([]byte, error) {
	left := append(content[:from:from], content[to:]...)
	if !json.Valid(left) {
		return nil, errors.New("taking the hook out would leave no JSON behind")
	}
	return left, nil
}

// twice names a key that stands twice in the object, of those named, and
// nothing where none does.
func (s *span) twice(keys ...string) string {
	for _, key := range keys {
		seen := 0
		for _, m := range s.members {
			if m.key == key {
				seen++
			}
		}
		if seen > 1 {
			return key
		}
	}
	return ""
}

// theOldHook finds the first hook of minka in the settings, and the range to
// take out with it, nothing where there is none. A shape other than the one
// Claude Code reads is passed over: what is not where Claude Code looks is
// not run, and not minka's to touch.
//
// A key that stands twice on the way to the hook is refused: which of the two
// a reader takes is its own business, and cutting by a guess could take out
// what somebody else wrote.
func (s *span) theOldHook() (*hookInTheText, error) {
	doubled := func(key string) error {
		return fmt.Errorf("names %q twice in one object, so which one counts cannot be told", key)
	}
	if s.kind != '{' {
		return nil, nil
	}
	if key := s.twice("hooks"); key != "" {
		return nil, doubled(key)
	}
	hooks, hooksAt := s.member("hooks")
	if hooks == nil || hooks.kind != '{' {
		return nil, nil
	}
	if key := hooks.twice(oldHookEvent); key != "" {
		return nil, doubled(key)
	}
	event, eventAt := hooks.member(oldHookEvent)
	if event == nil || event.kind != '[' {
		return nil, nil
	}
	for g := range event.items {
		group := &event.items[g]
		if group.kind != '{' {
			continue
		}
		if key := group.twice("hooks", "matcher"); key != "" {
			return nil, doubled(key)
		}
		list, _ := group.member("hooks")
		if list == nil || list.kind != '[' {
			continue
		}
		for h := range list.items {
			hook := &list.items[h]
			if hook.kind != '{' {
				continue
			}
			if key := hook.twice("type", "command"); key != "" {
				return nil, doubled(key)
			}
			kind, _ := hook.member("type")
			command, _ := hook.member("command")
			if kind == nil || kind.kind != '"' || kind.text != "command" || command == nil || command.kind != '"' || !isTheHookOfMinka(command.text) {
				continue
			}
			found := &hookInTheText{command: command.text}
			switch {
			case len(list.items) > 1 || !group.onlyHoldsHooks():
				found.from, found.to = list.itemRange(h)
			case len(event.items) > 1:
				found.from, found.to = event.itemRange(g)
			case len(hooks.members) > 1:
				found.from, found.to = hooks.memberRange(eventAt)
			default:
				found.from, found.to = s.memberRange(hooksAt)
			}
			return found, nil
		}
	}
	return nil, nil
}

// onlyHoldsHooks says whether a group is the one minka wrote around its hook:
// every tool, and the hooks.
func (s *span) onlyHoldsHooks() bool {
	if len(s.members) != 2 {
		return false
	}
	matcher, _ := s.member("matcher")
	list, _ := s.member("hooks")
	return matcher != nil && matcher.kind == '"' && matcher.text == oldHookMatcher && list != nil
}

// itemRange is what to take out of the text for the item at i of an array:
// the item with the comma after it and the space up to the next, the comma
// before it where it is the last, everything between the brackets where it is
// the only one.
func (s *span) itemRange(i int) (int, int) {
	bounds := make([][2]int, len(s.items))
	for k, item := range s.items {
		bounds[k] = [2]int{item.start, item.end}
	}
	return s.rangeOf(bounds, i)
}

func (s *span) memberRange(i int) (int, int) {
	bounds := make([][2]int, len(s.members))
	for k, m := range s.members {
		bounds[k] = [2]int{m.start, m.end()}
	}
	return s.rangeOf(bounds, i)
}

func (s *span) rangeOf(bounds [][2]int, i int) (int, int) {
	switch {
	case len(bounds) == 1:
		return s.start + 1, s.end - 1
	case i < len(bounds)-1:
		return bounds[i][0], bounds[i+1][0]
	default:
		return bounds[i-1][1], bounds[i][1]
	}
}

// parseSpans reads JSON and says where each value stands. Only valid JSON is
// read: a text that is not is no settings file to change.
func parseSpans(content []byte) (*span, error) {
	if !json.Valid(content) {
		return nil, errors.New("not JSON")
	}
	p := spanParser{text: content}
	value, err := p.value()
	if err != nil {
		return nil, err
	}
	return &value, nil
}

type spanParser struct {
	text []byte
	at   int
}

func (p *spanParser) skipSpace() {
	for p.at < len(p.text) && strings.IndexByte(" \t\r\n", p.text[p.at]) >= 0 {
		p.at++
	}
}

func (p *spanParser) value() (span, error) {
	p.skipSpace()
	if p.at >= len(p.text) {
		return span{}, errors.New("the text ends early")
	}
	start := p.at
	switch p.text[p.at] {
	case '{':
		return p.object(start)
	case '[':
		return p.array(start)
	case '"':
		end, err := p.stringEnd()
		if err != nil {
			return span{}, err
		}
		var text string
		if err := json.Unmarshal(p.text[start:end], &text); err != nil {
			return span{}, err
		}
		return span{start: start, end: end, kind: '"', text: text}, nil
	}
	for p.at < len(p.text) && strings.IndexByte(",]} \t\r\n", p.text[p.at]) < 0 {
		p.at++
	}
	return span{start: start, end: p.at}, nil
}

func (p *spanParser) stringEnd() (int, error) {
	for p.at++; p.at < len(p.text); p.at++ {
		switch p.text[p.at] {
		case '\\':
			p.at++
		case '"':
			p.at++
			return p.at, nil
		}
	}
	return 0, errors.New("a string does not end")
}

func (p *spanParser) object(start int) (span, error) {
	s := span{start: start, kind: '{'}
	p.at++
	for {
		p.skipSpace()
		if p.text[p.at] == '}' {
			p.at++
			s.end = p.at
			return s, nil
		}
		if p.text[p.at] == ',' {
			p.at++
			p.skipSpace()
		}
		key, err := p.value()
		if err != nil {
			return span{}, err
		}
		p.skipSpace()
		p.at++ // the colon
		value, err := p.value()
		if err != nil {
			return span{}, err
		}
		s.members = append(s.members, member{start: key.start, key: key.text, value: value})
	}
}

func (p *spanParser) array(start int) (span, error) {
	s := span{start: start, kind: '['}
	p.at++
	for {
		p.skipSpace()
		if p.text[p.at] == ']' {
			p.at++
			s.end = p.at
			return s, nil
		}
		if p.text[p.at] == ',' {
			p.at++
		}
		item, err := p.value()
		if err != nil {
			return span{}, err
		}
		s.items = append(s.items, item)
	}
}
