package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// leakedSecret is what a Basecamp CLI that fails to hand out its token prints
// around the failure, which may be the token itself.
const leakedSecret = "FAKE-SECRET-4242"

// versionsThroughTheDownloader takes every file from the fake account and an
// earlier file of an upload through the real downloader, at the address the
// real account would give it, so that whatever the Basecamp CLI says when it
// is asked for its token reaches the run as it would.
type versionsThroughTheDownloader struct {
	*fakebasecamp.Account
	real basecamp.Downloader
}

func (v versionsThroughTheDownloader) DownloadVersion(ctx context.Context, address string, into io.Writer) error {
	return v.real.DownloadVersion(ctx, strings.Replace(address, "https://3.basecampapi.test/", "https://3.basecampapi.com/", 1), into)
}

// failingTokenStandIn is a Basecamp CLI that fails to hand out its token with
// the exit code and code given, printing the secret on both of its outputs.
func failingTokenStandIn(t *testing.T, exitCode, code string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	path := filepath.Join(t.TempDir(), "basecamp")
	script := `#!/bin/sh
echo "token ` + leakedSecret + `" >&2
printf '{"code": "` + code + `", "error": "token ` + leakedSecret + `", "ok": false}'
exit ` + exitCode + `
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// **A failure of the Basecamp CLI to hand out its token is never Basecamp's word
// on a file and never quoted.** Its "not found" and its "forbidden" say nothing
// about the earlier file, so the file is neither written off as gone nor as
// refused: it is missing as a failure nobody can place, and the run says so with
// its own words. Whatever the Basecamp CLI printed — and it may print the token —
// stands nowhere: not in the failure, the report or the text.
func TestATokenTheBasecampCLIWillNotHandOutIsNeitherBasecampsWordNorQuoted(t *testing.T) {
	for _, failing := range []struct{ exitCode, code string }{{"2", "not_found"}, {"4", "forbidden"}, {"1", "crash"}} {
		for _, format := range []string{"--json", "--styled"} {
			e := newImportEnv(t)
			e.files = versionsThroughTheDownloader{Account: e.account,
				real: basecamp.Downloader{Binary: failingTokenStandIn(t, failing.exitCode, failing.code)}}
			r := e.run("import", "basecamp", "--project", "7011", "--out", filepath.Join(t.TempDir(), "bundle"), "--upload-versions", format)

			expectCode(t, r, ExitAPI)
			if strings.Contains(r.stdout+r.stderr, leakedSecret) {
				t.Errorf("exit %s, %s: the output quotes what the Basecamp CLI printed:\n%s\n%s", failing.exitCode, format, r.stdout, r.stderr)
			}
			if format != "--json" {
				continue
			}
			var env envelope
			if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
				t.Fatalf("output is not a JSON envelope: %v", err)
			}
			missing := decode(t, env.Data)["missingFiles"].([]any)
			named := 0
			for _, entry := range missing {
				file := entry.(map[string]any)
				if strings.HasPrefix(file["attachmentSourceId"].(string), "upload-9277-version-") {
					named++
					if file["why"] != string(basecamp.NotUnderstood) {
						t.Errorf("exit %s: the earlier file is missing as %v, want a failure nobody can place", failing.exitCode, file["why"])
					}
				}
			}
			if named != 2 {
				t.Errorf("exit %s: %d earlier files are named among %v, want both", failing.exitCode, named, missing)
			}
			if gone := decode(t, env.Data)["filesGone"]; gone != float64(2) {
				t.Errorf("exit %s: %v files are gone, want the two the fake gives up on and no earlier one", failing.exitCode, gone)
			}
		}
	}
}
