package irfile

import (
	"fmt"
	"math/bits"
	"regexp"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/irgen"
	"github.com/brunoga/sofabaton/profile"
)

// ucHexForm is an Unfolded Circle hex code: protocol;value;bits;repeat.
var ucHexForm = regexp.MustCompile(`(?i)^\s*([a-z_0-9]+)\s*;\s*(?:0x)?([0-9a-f]+)\s*;\s*(\d+)\s*;\s*(\d+)\s*$`)

// ucProtocols are IRremoteESP8266's protocol numbers (decode_type_t),
// which Unfolded Circle's codes use, by name.
var ucProtocols = map[string]int{
	"RC5": 1, "RC6": 2, "NEC": 3, "SONY": 4, "PANASONIC": 5, "JVC": 6, "SAMSUNG": 7,
	"SHARP": 14, "DENON": 17, "RC5X": 23, "PIONEER": 50, "SONY_38K": 74,
}

// ucHexSignal is s with the signal of an Unfolded Circle hex code, as its
// docks keep codes they learn: an IRremoteESP8266 protocol (a number, or
// its name), the frame's bits as that library sends them (most
// significant first, so the bytes of protocols sent least significant
// first are reversed), how many bits, and a repeat count. The unpacking
// follows upstream's (ir_uc_hex.py), checked against IRremoteESP8266's
// encoders: protocols the X2 has a descriptor for become descriptors, the
// others raw timings.
func ucHexSignal(s Signal, code string) Signal {
	m := ucHexForm.FindStringSubmatch(code)
	if m == nil {
		s.Skip = "not an Unfolded Circle hex code"
		return s
	}
	proto, err := strconv.Atoi(m[1])
	if err != nil {
		p, ok := ucProtocols[strings.ToUpper(m[1])]
		if !ok {
			s.Skip = fmt.Sprintf("Unfolded Circle protocol %s: not supported yet", m[1])
			return s
		}
		proto = p
	}
	n, _ := strconv.Atoi(m[3])
	v, err := strconv.ParseUint(m[2], 16, 64)
	if err != nil || n < 1 || n > 64 || (n < 64 && v>>n != 0) {
		s.Skip = "an Unfolded Circle code whose value does not fit its bits"
		return s
	}
	// bytes are the frame's bytes as sent, each back in its own order.
	byteOrder := func() []uint8 {
		var b []uint8
		for i := n/8 - 1; i >= 0; i-- {
			b = append(b, bits.Reverse8(uint8(v>>(8*i))))
		}
		return b
	}
	u8 := func(x uint8) *uint8 { return &x }
	unfit := func(why string) Signal {
		s.Skip = "Unfolded Circle " + why
		return s
	}
	switch proto {
	case 3: // NEC
		if n != 32 {
			return unfit(fmt.Sprintf("NEC of %d bits", n))
		}
		b := byteOrder()
		if b[3] != ^b[2] {
			// A 16-bit command: raw timings.
			return s.raw(irgen.NEC16(uint16(b[0])|uint16(b[1])<<8, uint16(b[2])|uint16(b[3])<<8))
		}
		s.IR = profile.IR{Protocol: "NEC", System: profile.Hex16(b[0])<<8 | profile.Hex16(b[1])}
		s.Code = uint16(b[2])
	case 7: // SAMSUNG
		if n != 32 {
			return unfit(fmt.Sprintf("Samsung of %d bits", n))
		}
		b := byteOrder()
		if b[3] != ^b[2] {
			return unfit("Samsung without an inverted command")
		}
		s.IR = profile.IR{Protocol: "NECx", Device: u8(b[0]), Subdevice: u8(b[1])}
		s.Code = uint16(b[2])
	case 4, 74: // SONY, SONY_38K
		if n != 12 && n != 15 && n != 20 {
			return unfit(fmt.Sprintf("Sony of %d bits", n))
		}
		logical := bits.Reverse64(v) >> (64 - n)
		cmd, addr := uint16(logical&0x7F), logical>>7
		switch n {
		case 12:
			s.IR = profile.IR{Protocol: "Sony12", Device: u8(uint8(addr))}
		case 15:
			s.IR = profile.IR{Protocol: "Sony15", Device: u8(uint8(addr))}
		default:
			s.IR = profile.IR{Protocol: "Sony20", Device: u8(uint8(addr & 0x1F)), Subdevice: u8(uint8(addr >> 5))}
		}
		s.Code = cmd
	case 1, 23: // RC5, RC5X
		if n != 12 && n != 13 {
			return unfit(fmt.Sprintf("RC5 of %d bits", n))
		}
		cmd, addr, toggle := uint8(v&0x3F), uint8(v>>6&0x1F), v>>11&1 == 1
		if n == 13 && v>>12&1 == 1 {
			cmd |= 0x40
		}
		return s.raw(irgen.RC5(addr, cmd, toggle))
	case 2: // RC6
		if n != 20 || v>>17&7 != 0 {
			return unfit("RC6 other than mode 0 of 20 bits")
		}
		s.IR = profile.IR{Protocol: "RC6", Device: u8(uint8(v >> 8))}
		s.Code = uint16(v & 0xFF)
	case 5: // PANASONIC (Kaseikyo)
		if n != 48 {
			return unfit(fmt.Sprintf("Panasonic of %d bits", n))
		}
		b := byteOrder()
		if b[0] == 0x02 && b[1] == 0x20 {
			s.IR = profile.IR{Protocol: "Panasonic", Device: u8(b[2]), Subdevice: u8(b[3])}
			s.Code = uint16(b[4])
			return s
		}
		return s.raw(irgen.Kaseikyo([6]byte(b)))
	case 50: // PIONEER: two NEC frames, a preamble and the code
		if n != 64 {
			return unfit(fmt.Sprintf("Pioneer of %d bits", n))
		}
		var frames [2][4]uint8
		for i := range 2 {
			w := v >> (32 * (1 - i))
			for j := range 4 {
				frames[i][j] = bits.Reverse8(uint8(w >> (8 * (3 - j))))
			}
			if frames[i][1] != ^frames[i][0] || frames[i][3] != ^frames[i][2] {
				return unfit("Pioneer frame that is not standard NEC")
			}
		}
		pre, code := irgen.Pioneer(frames[0][0], frames[0][2]), irgen.Pioneer(frames[1][0], frames[1][2])
		pre.Timings = append(pre.Timings, code.Timings...)
		return s.raw(pre)
	default:
		return unfit(fmt.Sprintf("protocol %s: not supported yet", m[1]))
	}
	return s
}
