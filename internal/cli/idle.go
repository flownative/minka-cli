package cli

import (
	"fmt"
	"time"

	"github.com/flownative/minka-cli/internal/work"
)

// nobodyIsAtWorkAfter is how long a Claude Code process that sits idle keeps
// the cards of its session alight after the last step. A session at work makes
// a step with every turn and every tool call, so one that has made none for
// this long is a window left open, and an open window is nobody at work.
const nobodyIsAtWorkAfter = 25 * time.Minute

// nobodyIsAtWorkOn says why a beat that only says the process is alive reports
// nothing, and nothing where it reports. A process left open over night keeps
// its cards alight for as long as it was at work a moment ago, and not for as
// long as it runs.
//
// Nor does it light again what the server forgot while nobody was at work: the
// cards may have been let go of meanwhile, and the next step lights them again
// where they were not. An empty list is always reported, so that the tiles go
// out at once.
func nobodyIsAtWorkOn(entry *work.List, now time.Time) string {
	if len(entry.Cards) == 0 {
		return ""
	}
	if now.Sub(entry.BeatAt) >= nobodyIsAtWorkAfter {
		return fmt.Sprintf("nobody has been at work in this session for %.0f minutes", nobodyIsAtWorkAfter.Minutes())
	}
	if now.Sub(entry.SentAt) >= work.HeldFor && !entry.BeatAt.After(entry.SentAt) {
		return "the board has forgotten these cards, and nobody has been at work in this session since"
	}
	return ""
}
