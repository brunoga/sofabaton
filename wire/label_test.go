package wire

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func TestSlot(t *testing.T) {
	tests := []struct {
		name string
		enc  TextEncoding
		in   string
		n    int
		want []byte
		back string
	}{
		{"ascii padded", ASCII, "TV", 4, []byte{'T', 'V', 0, 0}, "TV"},
		{"ascii cut", ASCII, "Projector", 4, []byte("Proj"), "Proj"},
		{"ascii drops non-ascii", ASCII, "Télé", 4, []byte{'T', 'l', 0, 0}, "Tl"},
		{"utf16 padded", UTF16BE, "TV", 6, []byte{0, 'T', 0, 'V', 0, 0}, "TV"},
		{"utf16 non-ascii", UTF16BE, "é", 2, []byte{0x00, 0xE9}, "é"},
		{"utf16 cut", UTF16BE, "abc", 4, []byte{0, 'a', 0, 'b'}, "ab"},
		{"utf16 odd slot", UTF16BE, "abc", 5, []byte{0, 'a', 0, 'b', 0}, "ab"},
		// U+1F4FA is a surrogate pair; a 6-byte slot fits "a" and the high
		// half only, so the pair is dropped whole.
		{"utf16 no half pair", UTF16BE, "a\U0001F4FA", 4, []byte{0, 'a', 0, 0}, "a"},
		{"utf16 whole pair", UTF16BE, "a\U0001F4FA", 6, []byte{0, 'a', 0xD8, 0x3D, 0xDC, 0xFA}, "a\U0001F4FA"},
		{"empty", UTF16BE, "", 2, []byte{0, 0}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.enc.Slot(tt.in, tt.n)
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("Slot = % x, want % x", got, tt.want)
			}
			if back := tt.enc.DecodeSlot(got); back != tt.back {
				t.Fatalf("DecodeSlot = %q, want %q", back, tt.back)
			}
		})
	}
}

func TestDecodeSlotASCII(t *testing.T) {
	tests := []struct {
		in   []byte
		want string
	}{
		{[]byte("kantoor on\x00\x00junk"), "kantoor on"},
		{[]byte("caf\xc3\xa9"), "café"}, // valid UTF-8 is kept
		{[]byte("caf\xe9"), "café"},     // anything else is Latin-1
		{nil, ""},
	}
	for _, tt := range tests {
		if got := ASCII.DecodeSlot(tt.in); got != tt.want {
			t.Errorf("DecodeSlot(% x) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func FuzzSlot(f *testing.F) {
	f.Add("Living room TV", 60)
	f.Add("\U0001F4FA🎮", 3)
	f.Fuzz(func(t *testing.T, s string, n int) {
		if n < 0 || n > 512 {
			return
		}
		for _, enc := range []TextEncoding{ASCII, UTF16BE} {
			slot := enc.Slot(s, n)
			if len(slot) != n {
				t.Fatalf("%v: slot of %d bytes, want %d", enc, len(slot), n)
			}
			back := enc.DecodeSlot(slot)
			if !utf8.ValidString(back) {
				t.Fatalf("%v: decoded invalid UTF-8 %q", enc, back)
			}
			// Re-encoding what was decoded gives the same slot when the
			// text survived intact (no NUL inside, valid input).
			if utf8.ValidString(s) && !bytes.Contains([]byte(s), []byte{0}) {
				if again := enc.Slot(back, n); !bytes.Equal(again, slot) {
					t.Fatalf("%v: %q -> % x -> %q -> % x", enc, s, slot, back, again)
				}
			}
		}
	})
}
