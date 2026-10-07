package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const minkasRefusal = `{"error":{"code":"unauthenticated","message":"This address needs a valid access token in the Authorization header."}}`

// Only a 401 with both marks of Minka's sign-in is Minka refusing the token:
// a front door answers 401 as well, in words of its own.
func TestOnlyMinkasOwn401SaysTheTokenIsRefused(t *testing.T) {
	for _, c := range []struct {
		name      string
		challenge string
		body      string
		want      bool
	}{
		{"Minka's sign-in", `Bearer realm="Minka"`, minkasRefusal, true},
		{"a password page in front", `Basic realm="staging"`, "<html>Who are you?</html>", false},
		{"Minka's body without its challenge", "", minkasRefusal, false},
		{"Minka's challenge without its body", `Bearer realm="Minka"`, `{"error":{"code":"forbidden"}}`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if c.challenge != "" {
					w.Header().Set("WWW-Authenticate", c.challenge)
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(c.body))
			}))
			defer server.Close()

			err := New(server.URL, "minka_pat_x", "").RevokeOwnToken(context.Background())
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
				t.Fatalf("error %v", err)
			}
			if apiErr.RefusedByMinka != c.want {
				t.Errorf("RefusedByMinka %v, want %v", apiErr.RefusedByMinka, c.want)
			}
		})
	}
}
