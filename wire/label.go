package wire

import (
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

// TextEncoding is how a record stores text in its fixed-width label slots.
type TextEncoding uint8

const (
	// ASCII slots (X1) hold one byte per character. Characters outside
	// ASCII are dropped on encoding; on decoding, valid UTF-8 is kept and
	// anything else is read as Latin-1.
	ASCII TextEncoding = iota + 1
	// UTF16BE slots (X1S, X2) hold big-endian UTF-16 code units.
	UTF16BE
)

func (e TextEncoding) String() string {
	switch e {
	case ASCII:
		return "ascii"
	case UTF16BE:
		return "utf-16be"
	}
	return fmt.Sprintf("TextEncoding(%d)", uint8(e))
}

// AppendSlot appends s as an n-byte slot: encoded, cut to fit and padded
// with zeros. A UTF-16 slot never ends in half a surrogate pair.
func (e TextEncoding) AppendSlot(b []byte, s string, n int) []byte {
	start := len(b)
	switch e {
	case UTF16BE:
		units := utf16.Encode([]rune(s))
		if len(units) > n/2 {
			units = units[:max(n, 0)/2]
			if k := len(units); k > 0 && utf16.IsSurrogate(rune(units[k-1])) && units[k-1] < 0xDC00 {
				units = units[:len(units)-1]
			}
		}
		for _, u := range units {
			b = append(b, byte(u>>8), byte(u))
		}
	default:
		for i := 0; i < len(s) && len(b)-start < n; i++ {
			if s[i] < utf8.RuneSelf {
				b = append(b, s[i])
			}
		}
	}
	for len(b)-start < n {
		b = append(b, 0)
	}
	return b
}

// Slot returns s as an n-byte slot.
func (e TextEncoding) Slot(s string, n int) []byte {
	return e.AppendSlot(make([]byte, 0, n), s, n)
}

// DecodeSlot reads a slot's text, which ends at the first NUL (a zero code
// unit in UTF-16). Unpaired UTF-16 surrogates are dropped. Whitespace is
// kept: trimming is the caller's choice, and the exact text round-trips.
func (e TextEncoding) DecodeSlot(b []byte) string {
	switch e {
	case UTF16BE:
		var r []rune
		for i := 0; i+1 < len(b); i += 2 {
			u := rune(b[i])<<8 | rune(b[i+1])
			if u == 0 {
				break
			}
			if !utf16.IsSurrogate(u) {
				r = append(r, u)
				continue
			}
			// A pair decodes to one rune; a surrogate without its other
			// half (a label cut mid-pair by some writer) is dropped.
			if u < 0xDC00 && i+3 < len(b) {
				lo := rune(b[i+2])<<8 | rune(b[i+3])
				if c := utf16.DecodeRune(u, lo); c != utf8.RuneError {
					r = append(r, c)
					i += 2
				}
			}
		}
		return string(r)
	default:
		end := len(b)
		for i, c := range b {
			if c == 0 {
				end = i
				break
			}
		}
		if utf8.Valid(b[:end]) {
			return string(b[:end])
		}
		r := make([]rune, end)
		for i, c := range b[:end] {
			r[i] = rune(c)
		}
		return string(r)
	}
}
