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

// frames are r's timings split at its gaps (spaces over 10 ms), each
// frame's gap left out.
func frames(r record.RawIR) [][]uint32 {
	var out [][]uint32
	start := 0
	for i, v := range r.Timings {
		if i%2 == 1 && v > 10000 {
			out = append(out, r.Timings[start:i])
			start = i + 1
		}
	}
	return out
}

func TestDescriptorProtocols(t *testing.T) {
	for _, c := range []struct {
		name    string
		r       record.RawIR
		carrier int
		lead    uint32
		n       int
		bits    uint64
		period  uint32
	}{
		{"NEC", irgen.NEC(0x04, 0xFB, 0x08), 38400, 9024, 32, 0xF708FB04, 108000},
		{"NECx", irgen.NECx(0x07, 0x07, 0x02), 38400, 4512, 32, 0xFD020707, 108000},
		{"Samsung20", irgen.Samsung20(0x28, 0x08, 0x05), 38400, 4512, 20, 0x05<<12 | 0x08<<6 | 0x28, 108000},
		{"JVC", irgen.JVC(0x03, 0x17), 38000, 8400, 16, 0x1703, 59080},
	} {
		r := c.r
		if r.Carrier != c.carrier || r.Timings[0] != c.lead || len(r.Timings) != 2+2*c.n+2 {
			t.Errorf("%s: carrier %d, lead %d, %d timings", c.name, r.Carrier, r.Timings[0], len(r.Timings))
			continue
		}
		if got := decode(r, c.n, false); got != c.bits {
			t.Errorf("%s: bits %X, want %X", c.name, got, c.bits)
		}
		var sum uint32
		for _, v := range r.Timings {
			sum += v
		}
		if sum != c.period {
			t.Errorf("%s: a frame of %d µs, want %d", c.name, sum, c.period)
		}
	}
}

func TestDenon(t *testing.T) {
	r := irgen.Denon(2, 122)
	f := frames(r)
	if r.Carrier != 38000 || len(f) != 2 || len(f[0]) != 31 || len(r.Timings) != 64 {
		t.Fatalf("carrier %d, %d frames, %d timings", r.Carrier, len(f), len(r.Timings))
	}
	// Each frame: 15 bits, least significant first, a 1 a space of 7
	// units; the second with the function and extension inverted.
	for i, want := range []uint64{2 | 122<<5, 2 | (^uint64(122)&0xFF)<<5 | 3<<13} {
		var got uint64
		for b := range 15 {
			if f[i][2*b+1] == 7*264 {
				got |= 1 << b
			}
		}
		if got != want {
			t.Errorf("frame %d: bits %04X, want %04X", i, got, want)
		}
	}
}

func TestDenonK(t *testing.T) {
	// The bytes are those of the hub's descriptor, its checksum the last.
	r := irgen.DenonK(4, 1, 369)
	sum := uint64((4<<4 ^ 1 ^ 369<<4 ^ 369>>4) & 0xFF)
	want := uint64(0x54) | 0x32<<8 | 4<<20 | (1|(369&0xF)<<4)<<24 | (369>>4)<<32 | sum<<40
	if got := decode(r, 48, false); got != want {
		t.Errorf("bits %012X, want %012X", got, want)
	}
}

func TestSonyRC6(t *testing.T) {
	r := irgen.Sony(12, 1, 0, 21)
	f := frames(r)
	if r.Carrier != 40000 || len(f) != 3 || !slices.Equal(f[0], f[1]) || !slices.Equal(f[1], f[2]) {
		t.Fatalf("carrier %d, %d frames", r.Carrier, len(f))
	}
	var got uint64
	for b := range 12 {
		if f[0][2+2*b] == 1200 {
			got |= 1 << b
		}
	}
	if got != 21|1<<7 {
		t.Errorf("Sony bits %03X", got)
	}
	if a, b := irgen.RC6(1, 12, false), irgen.RC6(1, 12, true); slices.Equal(a.Timings, b.Timings) || a.Carrier != 36000 {
		t.Error("RC6's toggle makes no difference")
	}
}
