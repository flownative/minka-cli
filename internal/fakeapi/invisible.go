package fakeapi

import "unicode"

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
