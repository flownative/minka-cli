package cli

import "testing"

// A card's front and a comment refused for a hidden character get the hint
// that names it; a harmless one comes back stripped.
func TestHiddenCharactersOnAFrontAreNamed(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "Plan\u200b the tent")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["title"] != "Plan the tent" {
		t.Errorf("data %s", env.Data)
	}
	env, r = e.runJSON("", "cards", "create", "--board", e.fixture.Board.ID, "--title", "Plan \u202eeht")
	expectRefusal(t, env, r, "cardTitleHasInvisibleCharacters", "direction override")
	env, r = e.runJSON("", "cards", "edit", e.fixture.CardInTriage.ID, "--description", "Go\U000e0041")
	expectRefusal(t, env, r, "cardDescriptionHasInvisibleCharacters", "Minka strips soft hyphens")
	env, r = e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, "Fine \u2066x", "--force")
	expectRefusal(t, env, r, "commentHasInvisibleCharacters", "Nothing was written")
}
