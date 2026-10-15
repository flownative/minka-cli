package cliupdate

import "testing"

func TestReleasesAreOrderedByTheirNumbers(t *testing.T) {
	ordered := []string{"0.9.9", "0.10.0", "0.10.1", "1.0.0", "10.0.0"}
	for i := range ordered {
		for j := range ordered {
			a, _ := ParseRelease(ordered[i])
			b, _ := ParseRelease(ordered[j])
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := a.Compare(b); got != want {
				t.Errorf("%s against %s: %d", ordered[i], ordered[j], got)
			}
		}
	}
}

func TestOnlyAPlainVersionIsARelease(t *testing.T) {
	for value, want := range map[string]bool{
		"0.40.0": true, "v0.40.0": true, "dev": false, "": false, "abc1234": false,
		"0.40.0-dirty": false, "v0.40.0-3-gabc1234": false, "0.40.0+local": false,
		"0.41.0-rc.1": false, "01.2.3": false, "0.40": false, "0.40.0\n": false,
	} {
		if _, got := ParseOwn(value); got != want {
			t.Errorf("%q: %v", value, got)
		}
	}
	if _, ok := FromHeader("v0.40.0"); ok {
		t.Error("a header with the v of a tag names no version")
	}
}
