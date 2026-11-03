package irfile

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/brunoga/sofabaton/profile"
)

// ReadFlipper reads a Flipper Zero IR file (.ir): its signals, those in
// protocols profiles render as commands, the others (raw timings among
// them) left out, saying why.
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
	if t := f["type"]; t != "parsed" {
		if t == "raw" {
			s.Skip = "raw timings: not supported yet"
		} else {
			s.Skip = fmt.Sprintf("a signal of type %q", t)
		}
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
			s.Skip = "an NEC command of 16 bits: not supported yet"
			return s
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
	default:
		s.Skip = fmt.Sprintf("protocol %s: not one the X2 is known to send yet", proto)
	}
	return s
}

// flipperValue is a Flipper value: four bytes in hex, little-endian.
func flipperValue(v string) (uint32, error) {
	b, err := hex.DecodeString(strings.Join(strings.Fields(v), ""))
	if err != nil || len(b) != 4 {
		return 0, fmt.Errorf("irfile: %q is not four bytes", v)
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24, nil
}
