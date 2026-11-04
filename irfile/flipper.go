package irfile

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/irgen"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// ReadFlipper reads a Flipper Zero IR file (.ir): its signals, parsed
// ones in protocols profiles render, or as raw IR; raw ones as they are.
// Those in other protocols are left out, saying why.
//
// A Flipper file is blocks of "key: value" lines, one per signal, after a
// "Filetype: IR signals file" header; a parsed signal has a protocol, an
// address and a command, each as four little-endian bytes in hex.
func ReadFlipper(r io.Reader) ([]Signal, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024) // raw signals are long lines
	var out []Signal
	var cur map[string]string
	flush := func() {
		if cur != nil && cur["name"] != "" {
			out = append(out, flipperSignal(cur))
		}
		cur = nil
	}
	header := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("irfile: %q is not a key: value line", line)
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "filetype":
			if v != "IR signals file" {
				return nil, fmt.Errorf("irfile: a %q, not an IR signals file", v)
			}
			header = true
			continue
		case "version":
			continue
		case "name":
			flush()
			cur = map[string]string{}
		}
		if cur != nil {
			cur[k] = v
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !header {
		return nil, fmt.Errorf("irfile: not a Flipper IR file (no Filetype line)")
	}
	flush()
	return out, nil
}

// flipperSignal is one signal of a Flipper file.
func flipperSignal(f map[string]string) Signal {
	s := Signal{Name: commandName(f["name"]), Source: f["name"]}
	switch t := f["type"]; t {
	case "parsed":
	case "raw":
		return flipperRaw(s, f)
	default:
		s.Skip = fmt.Sprintf("a signal of type %q", t)
		return s
	}
	a, err1 := flipperValue(f["address"])
	c, err2 := flipperValue(f["command"])
	if err1 != nil || err2 != nil {
		s.Skip = "its address or command is not four bytes in hex"
		return s
	}
	u8 := func(v uint32) *uint8 {
		b := uint8(v)
		return &b
	}
	proto := f["protocol"]
	switch strings.ToUpper(proto) {
	case "NEC":
		// An 8-bit address, sent with its complement.
		s.IR = profile.IR{Protocol: "NEC", System: profile.Hex16(a&0xFF)<<8 | profile.Hex16(^a&0xFF)}
		s.Code = uint16(c & 0xFF)
	case "NECEXT":
		// A 16-bit address, low byte first; the command is a byte and its
		// complement, else a 16-bit command the hub's NEC cannot send.
		if c>>8&0xFF != ^c&0xFF {
			// A 16-bit command: no descriptor sends it, raw timings do.
			return s.raw(irgen.NEC16(uint16(a), uint16(c)))
		}
		s.IR = profile.IR{Protocol: "NEC", System: profile.Hex16(a&0xFF)<<8 | profile.Hex16(a>>8&0xFF)}
		s.Code = uint16(c & 0xFF)
	case "SAMSUNG32":
		// NECx with the subdevice the device.
		s.IR = profile.IR{Protocol: "NECx", Device: u8(a), Subdevice: u8(a)}
		s.Code = uint16(c & 0xFF)
	case "SIRC":
		s.IR = profile.IR{Protocol: "Sony12", Device: u8(a & 0x1F)}
		s.Code = uint16(c & 0x7F)
	case "SIRC15":
		s.IR = profile.IR{Protocol: "Sony15", Device: u8(a)}
		s.Code = uint16(c & 0x7F)
	case "SIRC20":
		// A 13-bit address: the device in its low 5 bits, the subdevice
		// above.
		s.IR = profile.IR{Protocol: "Sony20", Device: u8(a & 0x1F), Subdevice: u8(a >> 5)}
		s.Code = uint16(c & 0x7F)
	case "RC6":
		s.IR = profile.IR{Protocol: "RC6", Device: u8(a)}
		s.Code = uint16(c & 0xFF)
	case "KASEIKYO":
		return flipperKaseikyo(s, a, c)
	case "NEC42":
		return s.raw(irgen.NEC42(uint16(a&0x1FFF), uint8(c)))
	case "RC5":
		return s.raw(irgen.RC5(uint8(a&0x1F), uint8(c&0x3F), false))
	case "RC5X":
		return s.raw(irgen.RC5(uint8(a&0x1F), uint8(c&0x7F), false))
	case "RCA":
		return s.raw(irgen.RCA(uint8(a&0xF), uint8(c)))
	case "PIONEER":
		return s.raw(irgen.Pioneer(uint8(a), uint8(c)))
	default:
		s.Skip = fmt.Sprintf("protocol %s: not one the X2 is known to send yet", proto)
	}
	return s
}

// flipperKaseikyo is a Kaseikyo signal of a Flipper file. Flipper splits
// the frame's six bytes (b0 to b5, sent in that order) into a vendor (b0,
// b1), two 4-bit genres (b2's high nibble, b3's low), a 10-bit command
// (b3's high nibble, b4's low six bits) and a 2-bit id (b4's top), and
// keeps the address as id<<24 | vendor<<8 | genre1<<4 | genre2. Two
// vendors are protocols profiles render: Panasonic's (0x2002), whose
// device, subdevice and function are b2, b3 and b4, and Denon's (0x3254),
// Denon-K, whose device and subdevice are the genres and whose function
// is the command and the id (12 bits: Flipper's Denon files give the
// codes of Denon's charts, 368 for a receiver's volume up).
func flipperKaseikyo(s Signal, a, c uint32) Signal {
	id, vendor := a>>24&0x3, a>>8&0xFFFF
	g1, g2 := uint8(a>>4&0xF), uint8(a&0xF)
	cmd := c & 0x3FF
	switch vendor {
	case 0x2002:
		// b2's low nibble is the vendor's parity, 0 for 0x2002.
		d, sub := g1<<4, g2|uint8(cmd&0xF)<<4
		s.IR = profile.IR{Protocol: "Panasonic", Device: &d, Subdevice: &sub}
		s.Code = uint16(cmd>>4 | id<<6)
	case 0x3254:
		s.IR = profile.IR{Protocol: "DenonK", Device: &g1, Subdevice: &g2}
		s.Code = uint16(cmd | id<<10)
	default:
		// Other vendors' have no descriptor: raw timings.
		return s.raw(irgen.Kaseikyo(irgen.KaseikyoFrame(uint16(vendor), g1, g2, uint16(cmd), uint8(id))))
	}
	return s
}

// flipperRaw is a raw signal of a Flipper file: its carrier frequency in
// Hz, and its timings in µs, mark first.
func flipperRaw(s Signal, f map[string]string) Signal {
	freq, err := strconv.Atoi(f["frequency"])
	if err != nil {
		s.Skip = "a raw signal without a frequency"
		return s
	}
	var t []uint32
	for _, w := range strings.Fields(f["data"]) {
		v, err := strconv.ParseUint(w, 10, 32)
		if err != nil {
			s.Skip = fmt.Sprintf("a raw signal's timing %q is not a number", w)
			return s
		}
		t = append(t, uint32(v))
	}
	return s.raw(record.RawIR{Carrier: freq, Timings: t})
}

// flipperValue is a Flipper value: four bytes in hex, little-endian.
func flipperValue(v string) (uint32, error) {
	b, err := hex.DecodeString(strings.Join(strings.Fields(v), ""))
	if err != nil || len(b) != 4 {
		return 0, fmt.Errorf("irfile: %q is not four bytes", v)
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24, nil
}

// writeFlipper writes a Flipper Zero IR file: codes in protocols Flipper
// parses as parsed signals, the others as raw ones.
func writeFlipper(w io.Writer, _ string, signals []Signal) ([]Skipped, error) {
	var b strings.Builder
	b.WriteString("Filetype: IR signals file\nVersion: 1\n")
	var skipped []Skipped
	for _, s := range signals {
		if proto, a, c, ok := flipperParsed(s); ok {
			fmt.Fprintf(&b, "#\nname: %s\ntype: parsed\nprotocol: %s\naddress: %s\ncommand: %s\n", s.Name, proto, flipperHex(a), flipperHex(c))
			continue
		}
		r, err := s.Timings()
		if err != nil {
			skipped = append(skipped, Skipped{Name: s.Source, Why: err.Error()})
			continue
		}
		t := make([]string, len(r.Timings))
		for i, v := range r.Timings {
			t[i] = strconv.FormatUint(uint64(v), 10)
		}
		fmt.Fprintf(&b, "#\nname: %s\ntype: raw\nfrequency: %d\nduty_cycle: 0.330000\ndata: %s\n", s.Name, r.Carrier, strings.Join(t, " "))
	}
	_, err := io.WriteString(w, b.String())
	return skipped, err
}

// flipperParsed is a code as Flipper's parsed signal, the reverse of
// flipperSignal's reading, if Flipper has its protocol.
func flipperParsed(s Signal) (proto string, a, c uint32, ok bool) {
	if s.Raw != nil {
		return "", 0, 0, false
	}
	ir := s.IR
	var d, sub uint32
	if ir.Device != nil {
		d = uint32(*ir.Device)
	}
	if ir.Subdevice != nil {
		sub = uint32(*ir.Subdevice)
	}
	code := uint32(s.Code)
	switch strings.ToLower(ir.Protocol) {
	case "nec":
		hi, lo := uint32(ir.System>>8), uint32(ir.System&0xFF)
		if lo == ^hi&0xFF {
			return "NEC", hi, code, true
		}
		return "NECext", hi | lo<<8, code | (^code&0xFF)<<8, true
	case "necx":
		if d != sub {
			return "", 0, 0, false // Flipper's Samsung32 has one address byte
		}
		return "Samsung32", d, code, true
	case "sony12":
		return "SIRC", d, code, true
	case "sony15":
		return "SIRC15", d, code, true
	case "sony20":
		return "SIRC20", d | sub<<5, code, true
	case "rc6":
		return "RC6", d, code, true
	case "panasonic":
		if d&0xF != 0 {
			return "", 0, 0, false // not a genre over the vendor's parity
		}
		cmd := sub>>4 | (code&0x3F)<<4
		return "Kaseikyo", (code>>6&3)<<24 | 0x2002<<8 | d | sub&0xF, cmd, true
	case "denonk":
		return "Kaseikyo", (code>>10&3)<<24 | 0x3254<<8 | d<<4 | sub, code & 0x3FF, true
	}
	return "", 0, 0, false
}

// flipperHex is a Flipper value: four bytes in hex, little-endian.
func flipperHex(v uint32) string {
	return fmt.Sprintf("%02X %02X %02X %02X", byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
