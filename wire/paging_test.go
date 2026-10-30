package wire

import (
	"bytes"
	"testing"
)

func TestPageCount(t *testing.T) {
	for _, tt := range []struct{ n, want int }{
		{0, 1}, {1, 1}, {PageChunk, 1}, {PageChunk + 1, 2}, {2 * PageChunk, 2}, {2*PageChunk + 1, 3},
	} {
		if got := PageCount(tt.n); got != tt.want {
			t.Errorf("PageCount(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
	if PageChunk != 247 {
		t.Fatalf("PageChunk = %d; upstream pages 247 body bytes", PageChunk)
	}
}

func TestPages(t *testing.T) {
	body := make([]byte, 2*PageChunk+10)
	for i := range body {
		body[i] = byte(i)
	}
	pages := Pages(0x01, body)
	if len(pages) != 3 {
		t.Fatalf("%d pages", len(pages))
	}
	var joined []byte
	for i, p := range pages {
		h, chunk, err := SplitPage(p)
		if err != nil {
			t.Fatal(err)
		}
		if h.Marker != 0x01 || int(h.Page) != i+1 {
			t.Fatalf("page %d header = %+v", i, h)
		}
		if len(p) > MaxPayload {
			t.Fatalf("page %d is %d bytes, more than a frame carries", i, len(p))
		}
		joined = append(joined, chunk...)
	}
	if !bytes.Equal(joined, body) {
		t.Fatal("pages do not join back into the body")
	}
	// Every page must fit a frame together with its header.
	for _, p := range pages {
		if _, err := (Frame{Family: FamWriteMacro, Payload: p}).MarshalBinary(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPagesEmptyAndMarker(t *testing.T) {
	pages := Pages(0x07, nil)
	if len(pages) != 1 || !bytes.Equal(pages[0], []byte{0x07, 0x00, 0x01}) {
		t.Fatalf("Pages(nil) = % x", pages)
	}
	if _, _, err := SplitPage([]byte{0x01, 0x00}); err == nil {
		t.Fatal("SplitPage accepted a 2-byte page")
	}
}
