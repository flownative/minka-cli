package work

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Beside the accounts, a host is taken over only where its account holds
// nothing: what this version writes per host is a copy of an account, and
// anything else under a host was written by somebody else.
func TestAHostNoAccountHoldsIsTakenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.json")
	stored := `{"accounts": {"work": {"sessions": {"": {"cards": [{"id": "of-work", "startedAt": "2099-01-01T00:00:00Z"}]}}}},
		"hosts": {
			"https://a.example": {"sessions": {"": {"cards": [{"id": "a-copy", "startedAt": "2099-01-01T00:00:00Z"}]}}},
			"https://b.example": {"sessions": {"": {"cards": [{"id": "of-b", "startedAt": "2099-01-01T00:00:00Z"}]}}}
		}}`
	if err := os.WriteFile(path, []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}
	accounts := Accounts{
		Of: func(host string) string {
			if host == "https://a.example" {
				return "work"
			}
			return "familie"
		},
		On: func(host string) []string {
			if host == "https://a.example" {
				return []string{"alpha", "work", host}
			}
			return []string{"familie", host}
		},
		Host: func(key string) (string, bool) { return "", false },
	}

	handle, err := OpenFor(path, time.Second, accounts)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if cards := handle.File.Accounts["work"].Sessions[NoSession].Cards; len(cards) != 1 || cards[0].ID != "of-work" {
		t.Errorf("the copy was taken over beside the account: %v", cards)
	}
	if cards := handle.File.Accounts["familie"].Sessions[NoSession].Cards; len(cards) != 1 || cards[0].ID != "of-b" {
		t.Errorf("the host no account holds was not taken over: %v", handle.File.Accounts)
	}
}
