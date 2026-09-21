package basecamp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// getAll collects every page of a list.
//
// The Basecamp CLI hands out one page per call and does not say whether
// another follows, so pages are asked for until one comes back empty.
func getAll(ctx context.Context, source Source, path string) ([]json.RawMessage, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}

	var all []json.RawMessage
	var previous []byte
	for page := 1; ; page++ {
		content, err := source.Get(ctx, fmt.Sprintf("%s%spage=%d", path, separator, page))
		if err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(content, &items); err != nil {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s with something that is no list", path)}
		}
		if len(items) == 0 {
			return all, nil
		}
		// A list that ignores the page number would never come to an end.
		if bytes.Equal(content, previous) {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered page %d of %s with the page before it", page, path)}
		}
		previous = content
		all = append(all, items...)
	}
}

func getList[T any](ctx context.Context, source Source, path string) ([]T, error) {
	raw, err := getAll(ctx, source, path)
	if err != nil {
		return nil, err
	}
	items := make([]T, len(raw))
	for i, item := range raw {
		if err := json.Unmarshal(item, &items[i]); err != nil {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s with an entry that cannot be read: %s", path, err)}
		}
	}
	return items, nil
}

func getOne[T any](ctx context.Context, source Source, path string) (T, error) {
	var value T
	content, err := source.Get(ctx, path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(content, &value); err != nil {
		return value, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s with something that cannot be read: %s", path, err)}
	}
	return value, nil
}
