// Package irgen renders IR protocols as raw timings (record.RawIR): those
// the X2 has no descriptor for, which it sends as they are (RC5 and RC5X,
// NEC with 16-bit commands, NEC42, Pioneer, RCA, Sharp and Kaseikyo of
// any vendor), and those it has, for files of timings (NEC, NECx, Sony, JVC,
// RC6, Samsung20, Panasonic, Denon and Denon-K). The timings follow the
// protocols' published definitions (IRP notation, as in
// IrpTransmogrifier's protocol list); each signal is one frame and its
// trailing gap, but for RC5's, sent twice with the same toggle, and
// Sony's, three times, as remotes do for a short press.
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
	return necLead(carrier, 16, bits, n)
}

// necLead is nec with a leading mark of lead units: 16 for NEC's, 8 for
// Samsung's.
func necLead(carrier int, lead uint32, bits uint64, n int) record.RawIR {
	var f frame
	f.add(true, lead*564)
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

// NEC is an NEC1 signal, as the X2's NEC descriptor (38.4 kHz): an 8-bit
// device and subdevice (a system code's high and low bytes), an 8-bit
// function and its complement.
func NEC(device, subdevice, function uint8) record.RawIR {
	f := uint64(function)
	return nec(38400, uint64(device)|uint64(subdevice)<<8|f<<16|(^f&0xFF)<<24, 32)
}

// NECx is Samsung's NEC (NECx2): NEC with a leading mark of half the
// length.
func NECx(device, subdevice, function uint8) record.RawIR {
	f := uint64(function)
	return necLead(38400, 8, uint64(device)|uint64(subdevice)<<8|f<<16|(^f&0xFF)<<24, 32)
}

// Samsung20 is Samsung's 20-bit code: NECx's lead, a 6-bit device and
// subdevice and an 8-bit function, without a complement.
func Samsung20(device, subdevice, function uint8) record.RawIR {
	v := uint64(device&0x3F) | uint64(subdevice&0x3F)<<6 | uint64(function)<<12
	return necLead(38400, 8, v, 20)
}

// Sony is a Sony (SIRC) signal of n bits, 12, 15 or 20: 40 kHz, in units
// of 600 µs, a mark of 4, then the 7-bit function and the device (5 bits;
// 8 for 15 bits; 5 and an 8-bit subdevice for 20), least significant
// first, each bit a space of 1 and a mark of 1 (0) or 2 (1), framed to
// 45 ms; sent three times.
func Sony(n int, device, subdevice, function uint8) record.RawIR {
	v := uint64(function & 0x7F)
	switch n {
	case 12:
		v |= uint64(device&0x1F) << 7
	case 15:
		v |= uint64(device) << 7
	default:
		v |= uint64(device&0x1F)<<7 | uint64(subdevice)<<12
	}
	var one frame
	one.add(true, 4*600)
	for i := range n {
		one.add(false, 600)
		one.add(true, uint32(600*(1+v>>i&1)))
	}
	one.to(45000, 600)
	var t []uint32
	for range 3 {
		t = append(t, one.t...)
	}
	return record.RawIR{Carrier: 40000, Timings: t}
}

// JVC is a JVC signal: 38 kHz, in units of 525 µs, a mark of 16 and space
// of 8, an 8-bit device and function as NEC's bits, a stop mark, framed
// to 59.08 ms.
func JVC(device, function uint8) record.RawIR {
	var f frame
	f.add(true, 16*525)
	f.add(false, 8*525)
	f.pulseDistance(uint64(device)|uint64(function)<<8, 16, 525, 525, 3*525, false)
	f.add(true, 525)
	f.to(59080, 10000)
	return record.RawIR{Carrier: 38000, Timings: f.t}
}

// RC6 is an RC6 mode 0 signal: 36 kHz, in units of 444 µs, a mark of 6
// and space of 2, a start bit of 1, mode 0 in three bits, the toggle in a
// bit twice as long, then the 8-bit device and function, most
// significant first, as Manchester bits (1 a mark then a space), framed
// to 107 ms.
func RC6(device, function uint8, toggle bool) record.RawIR {
	const u = 444
	var f frame
	f.add(true, 6*u)
	f.add(false, 2*u)
	bit := func(b uint8, w uint32) {
		f.add(b == 1, w)
		f.add(b == 0, w)
	}
	bit(1, u)
	for range 3 {
		bit(0, u)
	}
	t := uint8(0)
	if toggle {
		t = 1
	}
	bit(t, 2*u)
	v := uint16(device)<<8 | uint16(function)
	for i := 15; i >= 0; i-- {
		bit(uint8(v>>i&1), u)
	}
	f.to(107000, 2*u)
	return record.RawIR{Carrier: 36000, Timings: f.t}
}

// Panasonic is Panasonic's Kaseikyo: vendor 0x2002, then the device,
// subdevice and function, and their parity.
func Panasonic(device, subdevice, function uint8) record.RawIR {
	return Kaseikyo([6]byte{0x02, 0x20, device, subdevice, function, device ^ subdevice ^ function})
}

// DenonK is Denon's Kaseikyo: vendor 0x3254, a 4-bit device and
// subdevice (genres) and a 12-bit function (the command and id).
func DenonK(device, subdevice uint8, function uint16) record.RawIR {
	return Kaseikyo(KaseikyoFrame(0x3254, device&0xF, subdevice&0xF, function&0x3FF, uint8(function>>10&3)))
}

// Denon is a Denon 15-bit signal (Sharp's format): 38 kHz, in units of
// 264 µs, a 5-bit address, an 8-bit function and two extension bits of 0,
// least significant first, as a mark of 1 and a space of 3 (0) or 7 (1),
// a stop mark and a space of 165; then the same with the function and
// the extension inverted.
func Denon(address, function uint8) record.RawIR {
	return sharp(address, function, 0)
}

// Sharp is a Sharp signal: Denon's, but for its extension bits, 1 in the
// first frame (2 in the second).
func Sharp(address, function uint8) record.RawIR {
	return sharp(address, function, 1)
}

// sharp is a frame of Sharp's format with the extension bits ext, then
// one with the function and extension inverted.
func sharp(address, function uint8, ext uint64) record.RawIR {
	const u = 264
	var f frame
	half := func(fn uint8, ext uint64) {
		v := uint64(address&0x1F) | uint64(fn)<<5 | ext<<13
		f.pulseDistance(v, 15, u, 3*u, 7*u, false)
		f.add(true, u)
		f.add(false, 165*u)
	}
	half(function, ext)
	half(^function, ^ext&3)
	return record.RawIR{Carrier: 38000, Timings: f.t}
}
