package fakeapi

import (
	"html"
	"strings"
	"unicode"

	"github.com/flownative/minka-cli/internal/richtext"
)

// harmlessCharacters are what Minka strips without a word from a card's title
// and description and from a comment: the soft hyphen, the zero-width space,
// the word joiner, the byte order mark and the two direction marks. They
// cannot carry an instruction, and pasted text is full of them.
var harmlessCharacters = strings.NewReplacer("\u00ad", "", "\u200b", "", "\u2060", "", "\ufeff", "", "\u200e", "", "\u200f", "")

// harmlessStripped is a text without the harmless characters.
func harmlessStripped(text string) string {
	return harmlessCharacters.Replace(text)
}

// hasInvisibleCharactersInMarkup looks for invisible characters where Minka
// does, in the reduced markup, so that an entity of a text with markup counts
// as its character. The fake keeps markup as it was sent and reads its
// entities here instead; a text without markup is plain, and its entity is
// only text, as Minka escapes it.
func hasInvisibleCharactersInMarkup(text string) bool {
	if richtext.CarriesMarkup(text) {
		text = html.UnescapeString(text)
	}
	return hasInvisibleCharacters(harmlessStripped(text))
}

// hasInvisibleCharacters finds what Minka refuses in the texts agents read: a
// format character other than the joiners, a supplement variation selector,
// and a run of two or more joiners or of two or more variation selectors.
// A single joiner and a single variation selector stay for emoji and scripts.
func hasInvisibleCharacters(text string) bool {
	joiners, selectors := 0, 0
	for _, r := range text {
		isJoiner := r == '\u200c' || r == '\u200d'
		isSelector := r >= '\ufe00' && r <= '\ufe0f'
		switch {
		case isJoiner:
			joiners++
		case unicode.Is(unicode.Cf, r), r >= 0xe0100 && r <= 0xe01ef:
			return true
		}
		if !isJoiner {
			joiners = 0
		}
		if isSelector {
			selectors++
		} else {
			selectors = 0
		}
		if joiners >= 2 || selectors >= 2 {
			return true
		}
	}
	return false
}
