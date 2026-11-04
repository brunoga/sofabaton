package irgen_test

import (
	"slices"
	"testing"

	"github.com/brunoga/sofabaton/irgen"
	"github.com/brunoga/sofabaton/record"
)

// decode reads pulse-distance bits back: n bits after a header of two
// timings, a 1 where the space is longer than twice the mark.
func decode(r record.RawIR, n int, msbFirst bool) uint64 {
	var v uint64
	for i := 0; i < n; i++ {
		mark, space := r.Timings[2+2*i], r.Timings[3+2*i]
		if space > 2*mark {
			if msbFirst {
				v |= 1 << (n - 1 - i)
			} else {
				v |= 1 << i
			}
		}
	}
	return v
}

func TestNEC(t *testing.T) {
	r := irgen.Pioneer(0xA5, 0x1C)
	if r.Carrier != 40000 || r.Timings[0] != 9024 || r.Timings[1] != 4512 || len(r.Timings) != 68 {
		t.Fatalf("carrier %d, header %v, %d timings", r.Carrier, r.Timings[:2], len(r.Timings))
	}
	if got := decode(r, 32, false); got != 0xE31C5AA5 {
		t.Errorf("bits %08X, want E31C5AA5", got)
	}
	var sum uint32
	for _, v := range r.Timings {
		sum += v
	}
	if sum != 108000 {
		t.Errorf("frame of %d µs, want 108000", sum)
	}
	if got := decode(irgen.NEC16(0x1234, 0xABCD), 32, false); got != 0xABCD1234 {
		t.Errorf("NEC16 bits %08X", got)
	}
	if got := decode(irgen.NEC42(0x1ABC, 0x42), 42, false); got != 0x1ABC|(0x0543<<13)|0x42<<26|0xBD<<34 {
		t.Errorf("NEC42 bits %X", got)
	}
}

func TestRCA(t *testing.T) {
	r := irgen.RCA(0xF, 0x54)
	if r.Carrier != 58000 || len(r.Timings) != 2+48+2 {
		t.Fatalf("carrier %d, %d timings", r.Carrier, len(r.Timings))
	}
	if got := decode(r, 24, true); got != 0xF54_0AB {
		t.Errorf("bits %06X, want F540AB", got)
	}
}

// RC5's Manchester halves merge: device 5, function 53 (a TV's volume
// up), toggle clear.
func TestRC5(t *testing.T) {
	r := irgen.RC5(5, 53, false)
	if r.Carrier != 36000 {
		t.Fatal(r.Carrier)
	}
	// Decode by halves: expand each timing into 889 µs halves.
	var halves []bool
	mark := true
	for _, v := range r.Timings[:len(r.Timings)/2] {
		n := int(v+444) / 889
		if n > 4 {
			break // the gap
		}
		for i := 0; i < n; i++ {
			halves = append(halves, mark)
		}
		mark = !mark
	}
	halves = append([]bool{false}, halves...) // the first bit's leading space
	for len(halves) < 28 {
		halves = append(halves, false)
	}
	var bits []int
	for i := 0; i < 28; i += 2 {
		if !halves[i] && halves[i+1] {
			bits = append(bits, 1)
		} else {
			bits = append(bits, 0)
		}
	}
	want := []int{1, 1, 0, 0, 0, 1, 0, 1, 1, 1, 0, 1, 0, 1}
	if !slices.Equal(bits, want) {
		t.Errorf("bits %v, want %v", bits, want)
	}
	if n := len(r.Timings); n%2 != 0 || !slices.Equal(r.Timings[:n/2], r.Timings[n/2:]) {
		t.Error("not two equal frames")
	}
}

func TestKaseikyo(t *testing.T) {
	// Panasonic's vendor has parity 0; the last byte is the parity of
	// the three before.
	b := irgen.KaseikyoFrame(0x2002, 0x8, 0x0, 0x3D, 0)
	if b != [6]byte{0x02, 0x20, 0x80, 0xD0, 0x03, 0x80 ^ 0xD0 ^ 0x03} {
		t.Errorf("frame % X", b)
	}
	r := irgen.Kaseikyo(b)
	if r.Carrier != 37000 || len(r.Timings) != 2+96+2 {
		t.Fatalf("carrier %d, %d timings", r.Carrier, len(r.Timings))
	}
	if got := decode(r, 48, false); got != 0x53_03_D0_80_20_02 {
		t.Errorf("bits %012X", got)
	}
	if _, err := r.Encode(); err != nil {
		t.Error(err)
	}
}
