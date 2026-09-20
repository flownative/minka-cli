package basecamp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

// getList collects every page of a list.
//
// The Basecamp CLI hands out one page per call and does not say whether
// another follows, so pages are asked for until one comes back empty. A page
// is read into its entries when it arrives and let go of: a chat has
// thousands of pages' worth, and only what a run needs of them is held.
func getList[T any](ctx context.Context, source Source, path string) ([]T, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}

	var all []T
	var previous [sha256.Size]byte
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
		// A list that ignores the page number would never come to an end. A list
		// that grows by exactly a page between two requests looks the same, and
		// giving up is the safe answer to both.
		digest := sha256.Sum256(content)
		if digest == previous {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered page %d of %s with the page before it: the list ignores the page, or it moved under the walk", page, path)}
		}
		previous = digest

		for _, item := range items {
			var entry T
			if err := json.Unmarshal(item, &entry); err != nil {
				return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s with an entry that cannot be read: %s", path, err)}
			}
			all = append(all, entry)
		}
	}
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
