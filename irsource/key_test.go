package irsource

import (
	"strings"
	"testing"
)

func TestScrambleKey(t *testing.T) {
	for _, k := range []string{"0123456789abcdef", strings.Repeat("k", 70), ""} {
		s := ScrambleKey(k)
		if strings.Contains(s, k) && k != "" {
			t.Errorf("%q in %q", k, s)
		}
		if back := UnscrambleKey(s); back != k {
			t.Errorf("%q came back %q", k, back)
		}
	}
	if UnscrambleKey("not hex") != "" {
		t.Error("garbage unscrambled")
	}
}
