// Package irgen renders IR protocols the X2 has no descriptor for as raw
// timings (record.RawIR), which it sends as they are: RC5 and RC5X, NEC
// with 16-bit commands, NEC42, Pioneer, RCA and Kaseikyo of any vendor.
// The timings follow the protocols' published definitions (IRP notation,
// as in IrpTransmogrifier's protocol list); each signal is one frame and
// its trailing gap, but for RC5's, sent twice with the same toggle as a
// remote does for a short press.
package irgen

import "github.com/brunoga/sofabaton/record"

// frame builds a signal: marks and spaces in µs, merging neighbors of the
// same kind, as Manchester codes produce.
type frame struct {
	t []uint32
	// mark is whether the last timing is a mark.
	mark bool
}

func (f *frame) add(mark bool, us uint32) {
	if len(f.t) > 0 && f.mark == mark {
		f.t[len(f.t)-1] += us
		return
	}
	if len(f.t) == 0 && !mark {
		return // a signal starts with a mark
	}
	f.t = append(f.t, us)
	f.mark = mark
}

// to ends the frame with a space bringing it to period µs, at least min.
func (f *frame) to(period, min uint32) {
	var sum uint32
	for _, v := range f.t {
		sum += v
	}
	gap := min
	if sum+min < period {
		gap = period - sum
	}
	f.add(false, gap)
}

// pulseDistance is the bits of v, n of them, least significant first
// unless msbFirst, as NEC-like protocols send them: a mark, then a short space for 0 or a
// long one for 1.
func (f *frame) pulseDistance(v uint64, n int, mark, zero, one uint32, msbFirst bool) {
	for i := 0; i < n; i++ {
		bit := v >> i & 1
		if msbFirst {
			bit = v >> (n - 1 - i) & 1
		}
		f.add(true, mark)
		if bit == 1 {
			f.add(false, one)
		} else {
			f.add(false, zero)
		}
	}
}

// RC5 is a Philips RC5 signal: 36 kHz, Manchester bits of 1,778 µs,
// most significant first: two start bits, the toggle, a 5-bit device and
// a 6-bit function; an RC5X function (7 bits) puts its top bit, inverted,
// in the second start bit. Frames repeat every 113,778 µs.
func RC5(device, function uint8, toggle bool) record.RawIR {
	const half = 889
	var f frame
	bits := []uint8{1, 1 &^ (function >> 6 & 1)}
	if toggle {
		bits = append(bits, 1)
	} else {
		bits = append(bits, 0)
	}
	for i := 4; i >= 0; i-- {
		bits = append(bits, device>>i&1)
	}
	for i := 5; i >= 0; i-- {
		bits = append(bits, function>>i&1)
	}
	var one frame
	for _, b := range bits {
		// 1 is a space then a mark; 0 a mark then a space.
		one.add(b == 0, half)
		one.add(b == 1, half)
	}
	one.to(113778, 10000)
	f.t = append(append(f.t, one.t...), one.t...)
	return record.RawIR{Carrier: 36000, Timings: f.t}
}

// nec is an NEC-like frame at carrier Hz, in units of 564 µs: a mark of
// 16 and space of 8, the bits (a mark of 1, a space of 1 for 0 or 3 for
// 1), a stop mark, and a gap to 108 ms.
func nec(carrier int, bits uint64, n int) record.RawIR {
	var f frame
	f.add(true, 16*564)
	f.add(false, 8*564)
	f.pulseDistance(bits, n, 564, 564, 3*564, false)
	f.add(true, 564)
	f.to(108000, 20000)
	return record.RawIR{Carrier: carrier, Timings: f.t}
}

// NEC16 is an NEC signal of a 16-bit address and a 16-bit command (what
// Flipper calls NECext with a command that is not a byte and its
// complement), both least significant byte first.
func NEC16(address, command uint16) record.RawIR {
	return nec(38000, uint64(address)|uint64(command)<<16, 32)
}

// NEC42 is the 42-bit NEC variant: a 13-bit address and its complement,
// then an 8-bit command and its complement.
func NEC42(address uint16, command uint8) record.RawIR {
	a := uint64(address & 0x1FFF)
	c := uint64(command)
	bits := a | (^a&0x1FFF)<<13 | c<<26 | (^c&0xFF)<<34
	return nec(38000, bits, 42)
}

// Pioneer is NEC at 40 kHz: an 8-bit address and command, each with its
// complement.
func Pioneer(address, command uint8) record.RawIR {
	a, c := uint64(address), uint64(command)
	return nec(40000, a|(^a&0xFF)<<8|c<<16|(^c&0xFF)<<24, 32)
}

// RCA is an RCA signal: 58 kHz, in units of 460 µs, a mark and space of
// 8, a 4-bit address and 8-bit command, then both inverted, most
// significant first, as a mark of 1 and a space of 2 (0) or 4 (1), a stop
// mark and a space of 16.
func RCA(address, command uint8) record.RawIR {
	var f frame
	f.add(true, 8*460)
	f.add(false, 8*460)
	v := uint64(address&0xF)<<8 | uint64(command)
	v = v<<12 | ^v&0xFFF
	f.pulseDistance(v, 24, 460, 2*460, 4*460, true)
	f.add(true, 460)
	f.add(false, 16*460)
	return record.RawIR{Carrier: 58000, Timings: f.t}
}

// Kaseikyo is a Kaseikyo (Japanese format) signal of six bytes, sent in
// order, least significant bit first: 37 kHz, a 3,456 µs mark and 1,728
// µs space, 432 µs marks and spaces of 432 (0) or 1,296 µs (1), a stop
// mark, and a space of 173 units of 432 µs.
func Kaseikyo(b [6]byte) record.RawIR {
	var f frame
	f.add(true, 3456)
	f.add(false, 1728)
	for _, x := range b {
		f.pulseDistance(uint64(x), 8, 432, 432, 1296, false)
	}
	f.add(true, 432)
	f.add(false, 173*432)
	return record.RawIR{Carrier: 37000, Timings: f.t}
}

// KaseikyoFrame is the six bytes of a Kaseikyo signal of a vendor, two
// 4-bit genres, a 10-bit command and a 2-bit id, as Flipper and IRP
// define them: the vendor (low byte first), genre 1 over the vendor's
// parity nibble, genre 2 under the command's low nibble, the command's
// high six bits under the id, and the parity of the three bytes before.
func KaseikyoFrame(vendor uint16, genre1, genre2 uint8, command uint16, id uint8) [6]byte {
	p := byte(vendor) ^ byte(vendor>>8)
	p = (p ^ p>>4) & 0xF
	b2 := genre1<<4 | p
	b3 := genre2&0xF | byte(command&0xF)<<4
	b4 := byte(command>>4&0x3F) | id<<6
	return [6]byte{byte(vendor), byte(vendor >> 8), b2, b3, b4, b2 ^ b3 ^ b4}
}
