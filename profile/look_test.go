package profile

import (
	"strings"
	"testing"
)

func TestIconPathsAndColors(t *testing.T) {
	for i := Icon(1); i <= 45; i++ {
		if p := i.Path(); !strings.HasPrefix(p, "M") {
			t.Errorf("icon %d (%v): path %.20q", i, i, p)
		}
	}
	if Icon(0).Path() != "" || Icon(46).Path() != "" {
		t.Error("paths for ids the X2 has no icon for")
	}
	if r, g, b := Color(7).RGB(); r != 0x00 || g != 0xA8 || b != 0xE0 {
		t.Errorf("blue: %02X%02X%02X", r, g, b)
	}
	if r, g, b := Color(0).RGB(); r != 0x7F || g != 0x7F || b != 0x7F {
		t.Errorf("no color: %02X%02X%02X", r, g, b)
	}
}
