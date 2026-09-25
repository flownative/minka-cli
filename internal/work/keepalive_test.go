package work

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The lock says whose it is where a host name allows it, and it says it in a
// name the file system takes whatever the host looks like.
func TestTheLockOfAKeepaliveIsNamedAfterTheHostItServes(t *testing.T) {
	store := filepath.Join(t.TempDir(), "minka", "work.json")

	path := KeepaliveLockPath(store, "https://minka.example.com")
	shaped := regexp.MustCompile(`^keepalive-minka\.example\.com-[0-9a-f]{16}\.lock$`)
	if name := filepath.Base(path); !shaped.MatchString(name) {
		t.Errorf("the lock is called %q, and nobody sees whose it is", name)
	}

	// Two hosts never share a lock, and none of them writes anywhere but beside
	// the store.
	taken := map[string]string{}
	for _, host := range []string{
		"https://minka.example.com",
		"http://minka.example.com",
		"https://minka.example.com:8443",
		"https://minka.example.com-8443",
		"https://" + strings.Repeat("a", 300) + ".example.com",
		"https://../../../etc/minka",
		"https://" + strings.Repeat(".", 20),
	} {
		path := KeepaliveLockPath(store, host)
		if dir := filepath.Dir(path); dir != filepath.Dir(store) {
			t.Errorf("the lock of %q lies in %s, the store in %s", host, dir, filepath.Dir(store))
		}
		name := filepath.Base(path)
		if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) || len(name) > 255 {
			t.Errorf("%q gives the file name %q", host, name)
		}
		if other, clash := taken[name]; clash {
			t.Errorf("%q and %q share the lock %q", host, other, name)
		}
		taken[name] = host
	}
}
