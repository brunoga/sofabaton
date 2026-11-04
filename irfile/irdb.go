package irfile

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/irgen"
	"github.com/brunoga/sofabaton/profile"
)

// ReadIRDB reads one of irdb's files (https://github.com/probonopd/irdb):
// a CSV of functionname, protocol, device, subdevice and function, in
// decimal, a subdevice of -1 standing for none (or the protocol's
// default). Its signals in protocols profiles render become commands;
// the others are left out, saying why.
func ReadIRDB(r io.Reader) ([]Signal, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("irfile: %w", err)
	}
	if len(rows) == 0 {
		return nil, errors.New("irfile: an empty file")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, h := range []string{"functionname", "protocol", "device", "subdevice", "function"} {
		if _, ok := col[h]; !ok {
			return nil, fmt.Errorf("irfile: not an irdb file (no %s column)", h)
		}
	}
	var out []Signal
	for _, row := range rows[1:] {
		field := func(h string) string {
			if i := col[h]; i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		name := field("functionname")
		if name == "" {
			continue
		}
		out = append(out, irdbSignal(name, field("protocol"), field("device"), field("subdevice"), field("function")))
	}
	return out, nil
}

// irdbSignal is one row of an irdb file.
func irdbSignal(name, proto, device, subdevice, function string) Signal {
	s := Signal{Name: commandName(name), Source: name}
	d, err1 := strconv.Atoi(device)
	sub, err2 := strconv.Atoi(subdevice)
	f, err3 := strconv.Atoi(function)
	if err1 != nil || err2 != nil || err3 != nil || d < 0 || d > 255 || sub < -1 || sub > 255 || f < 0 || f > 0xFFFF {
		s.Skip = "its device, subdevice or function is not a number in range"
		return s
	}
	dv, sv := uint8(d), uint8(sub)
	noSub := sub == -1
	// one is a protocol of a device and a function, and no subdevice.
	one := func(p string) {
		if !noSub {
			s.Skip = fmt.Sprintf("%s with a subdevice: not supported yet", proto)
			return
		}
		s.IR = profile.IR{Protocol: p, Device: &dv}
	}
	// two is a protocol of a device, a subdevice and a function, the
	// subdevice def when the file has none.
	two := func(p string, def uint8) {
		if noSub {
			sv = def
		}
		s.IR = profile.IR{Protocol: p, Device: &dv, Subdevice: &sv}
	}
	switch strings.ToUpper(proto) {
	case "NEC", "NEC1", "NEC2":
		// The subdevice is the address's second byte, the device's
		// complement when there is none.
		if noSub {
			sv = ^dv
		}
		s.IR = profile.IR{Protocol: "NEC", System: profile.Hex16(dv)<<8 | profile.Hex16(sv)}
	case "NECX", "NECX1", "NECX2":
		two("NECx", dv)
	case "SONY12":
		one("Sony12")
	case "SONY15":
		one("Sony15")
	case "SONY20":
		if noSub {
			s.Skip = "Sony20 without a subdevice"
			return s
		}
		two("Sony20", 0)
	case "JVC":
		one("JVC")
	case "RC6":
		one("RC6")
	case "SAMSUNG20":
		if noSub {
			s.Skip = "Samsung20 without a subdevice"
			return s
		}
		two("Samsung20", 0)
	case "PANASONIC":
		two("Panasonic", 0)
	case "DENON":
		one("Denon")
	case "DENON-K", "DENONK":
		if noSub {
			s.Skip = "Denon-K without a subdevice"
			return s
		}
		two("DenonK", 0)
	case "RC5":
		if !noSub || d > 31 || f > 127 {
			s.Skip = "RC5 beyond a 5-bit device and 7-bit function, or with a subdevice"
			return s
		}
		return s.raw(irgen.RC5(dv, uint8(f), false))
	case "RCA":
		if !noSub || d > 15 || f > 255 {
			s.Skip = "RCA beyond a 4-bit device and 8-bit function, or with a subdevice"
			return s
		}
		return s.raw(irgen.RCA(dv, uint8(f)))
	case "PIONEER":
		// The subdevice, when given, must be the device's complement:
		// Pioneer's second byte.
		if (!noSub && sv != ^dv) || f > 255 {
			s.Skip = "Pioneer with a subdevice other than the device's complement"
			return s
		}
		return s.raw(irgen.Pioneer(dv, uint8(f)))
	default:
		s.Skip = fmt.Sprintf("protocol %s: not one the X2 is known to send yet", proto)
		return s
	}
	s.Code = uint16(f)
	return s
}
