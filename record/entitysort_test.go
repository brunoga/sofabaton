package record

import (
	"encoding/hex"
	"slices"
	"testing"
)

// The app's write of our X2's activity order (2026-11-05).
const appActivitySort = "010001010001006601006802006703006504a6"

func TestEntitySort(t *testing.T) {
	var ents []*Entity
	for _, id := range []byte{0x66, 0x68, 0x67, 0x65} {
		ents = append(ents, &Entity{ID: id})
	}
	pages := EntitySortPages(ents)
	if len(pages) != 1 || hex.EncodeToString(pages[0]) != appActivitySort {
		t.Fatalf("pages % x", pages)
	}
	ids, err := ParseEntitySort(pages[0][3:])
	if err != nil || !slices.Equal(ids, []byte{0x66, 0x68, 0x67, 0x65}) {
		t.Fatalf("parsed % x, %v", ids, err)
	}
	for _, bad := range []string{"010001006601006801a2", "0100010066010068", "010001006602a2"} {
		b, _ := hex.DecodeString(bad)
		if _, err := ParseEntitySort(b); err == nil {
			t.Errorf("%s taken", bad)
		}
	}
}
