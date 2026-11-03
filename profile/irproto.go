package profile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// irProtocol turns a device's IR parameters and a command's code into a
// descriptor in Sofabaton's form, and back.
type irProtocol struct {
	name string
	// maxCode is the largest code; decimal tells whether manufacturers
	// list codes in decimal (Denon) rather than hex (NEC).
	maxCode uint16
	decimal bool
	check   func(ir *IR) error
	render  func(ir *IR, code uint16) string
	parse   func(desc string) (IR, uint16, bool)
}

var protocols = []irProtocol{
	{
		name: "NEC", maxCode: 0xFF,
		check: func(ir *IR) error { return nil },
		render: func(ir *IR, code uint16) string {
			return fmt.Sprintf("P:NEC R:38400 D:%d S:%d F:%d", ir.System>>8, ir.System&0xFF, code)
		},
		parse: func(desc string) (IR, uint16, bool) {
			v, ok := match(necPattern, desc, 8, 8, 8)
			if !ok {
				return IR{}, 0, false
			}
			return IR{Protocol: "NEC", System: Hex16(v[0]<<8 | v[1])}, uint16(v[2]), true
		},
	},
	{
		// Denon's Kaseikyo variant (manufacturer code 0x3254, which the
		// hub writes as C0:84 C1:50): a 4-bit device, a 4-bit subdevice
		// (the receiver's zone and remote id) and a 12-bit function, as
		// Denon's IR command charts list them.
		name: "DenonK", maxCode: 0xFFF, decimal: true,
		check: func(ir *IR) error {
			if ir.Device == nil || ir.Subdevice == nil || *ir.Device > 0xF || *ir.Subdevice > 0xF {
				return fmt.Errorf("profile: DenonK needs a device and a subdevice, 0 to 15")
			}
			return nil
		},
		render: func(ir *IR, code uint16) string { return denonK(*ir.Device, *ir.Subdevice, code) },
		parse: func(desc string) (IR, uint16, bool) {
			v, ok := match(denonKPattern, desc, 4, 4, 12, 8)
			if !ok {
				return IR{}, 0, false
			}
			d, s := uint8(v[0]), uint8(v[1])
			ir := IR{Protocol: "DenonK", Device: &d, Subdevice: &s}
			if denonK(d, s, uint16(v[2])) != desc {
				return IR{}, 0, false // a checksum we would not reproduce
			}
			return ir, uint16(v[2]), true
		},
	},
	{
		// Denon's 15-bit codes (Sharp's format): a 5-bit address (the
		// chart's device, 2 or 12 for a receiver) and an 8-bit function.
		name: "Denon", maxCode: 0xFF, decimal: true,
		check: func(ir *IR) error {
			if ir.Device == nil || *ir.Device > 0x1F || ir.Subdevice != nil {
				return fmt.Errorf("profile: Denon needs a device, 0 to 31, and no subdevice")
			}
			return nil
		},
		render: func(ir *IR, code uint16) string { return denonSharp(*ir.Device, uint8(code)) },
		parse: func(desc string) (IR, uint16, bool) {
			v, ok := match(sharpPattern, desc, 5, 8, 1)
			if !ok {
				return IR{}, 0, false
			}
			a := uint8(v[2]) | uint8(v[0]&0xF)<<1
			f := uint16(v[0]>>4) | uint16(v[1]&0x7F)<<1
			if denonSharp(a, uint8(f)) != desc {
				return IR{}, 0, false
			}
			return IR{Protocol: "Denon", Device: &a}, f, true
		},
	},
	// The protocols below are in irdb's terms (device, subdevice,
	// function), as the hub's descriptors are; upstream verified each form
	// by the light the X2 sends (docs/protocol/ir-codes.md). NECx is
	// Samsung's NEC, an 8-bit device and subdevice (Samsung32 is NECx with
	// both the same: the hub's own Samsung32 is silent).
	fields("NECx", "P:NECx R:38400", 8, 8, 8, ""),
	fields("Sony12", "P:Sony12 R:40000", 5, 0, 7, " MUL:2"),
	fields("Sony15", "P:Sony15 R:40000", 8, 0, 7, " MUL:2"),
	fields("Sony20", "P:Sony20 R:40000", 5, 8, 7, " MUL:2"),
	fields("JVC", "P:JVC R:38000", 8, 0, 8, ""),
	fields("RC6", "P:RC6 R:36000", 8, 0, 8, " MUL:1"),
	// Samsung20's F:2 comes out as 222 (upstream, open).
	fields("Samsung20", "P:Samsung20 R:38000", 6, 6, 8, ""),
	panasonic(),
}

// fields is a protocol whose descriptor is head, then its device (D),
// subdevice (S, when it has one: sBits not 0) and function (F) in decimal,
// then tail: the form of most of the hub's protocols. The bit sizes bound
// each.
func fields(name, head string, dBits, sBits, fBits int, tail string) irProtocol {
	render := func(ir *IR, code uint16) string {
		if sBits == 0 {
			return fmt.Sprintf("%s D:%d F:%d%s", head, *ir.Device, code, tail)
		}
		return fmt.Sprintf("%s D:%d S:%d F:%d%s", head, *ir.Device, *ir.Subdevice, code, tail)
	}
	pattern := `^` + regexp.QuoteMeta(head) + ` D:(\d+)`
	bits := []int{dBits}
	if sBits != 0 {
		pattern += ` S:(\d+)`
		bits = append(bits, sBits)
	}
	pattern += ` F:(\d+)` + regexp.QuoteMeta(tail) + `$`
	re := regexp.MustCompile(pattern)
	return irProtocol{
		name: name, maxCode: 1<<fBits - 1, decimal: true,
		check: func(ir *IR) error {
			ok := ir.Device != nil && int(*ir.Device) < 1<<dBits && ir.System == 0
			if sBits == 0 {
				ok = ok && ir.Subdevice == nil
			} else {
				ok = ok && ir.Subdevice != nil && int(*ir.Subdevice) < 1<<sBits
			}
			switch {
			case ok:
				return nil
			case sBits == 0:
				return fmt.Errorf("profile: %s needs a device, 0 to %d, and no subdevice", name, 1<<dBits-1)
			}
			return fmt.Errorf("profile: %s needs a device, 0 to %d, and a subdevice, 0 to %d", name, 1<<dBits-1, 1<<sBits-1)
		},
		render: render,
		parse: func(desc string) (IR, uint16, bool) {
			v, ok := match(re, desc, append(bits, fBits)...)
			if !ok {
				return IR{}, 0, false
			}
			d := uint8(v[0])
			ir := IR{Protocol: name, Device: &d}
			if sBits != 0 {
				s := uint8(v[1])
				ir.Subdevice = &s
			}
			f := uint16(v[len(v)-1])
			if render(&ir, f) != desc {
				return IR{}, 0, false
			}
			return ir, f, true
		},
	}
}

// panasonic is Panasonic's Kaseikyo (manufacturer code 0x2002, which the
// hub writes as C0:2 C1:32), its check byte D^S^F. The catalog's form ends
// in MUL:1, which upstream found the hub never sends: it is left out.
func panasonic() irProtocol {
	p := fields("Panasonic", "P:Panasonic R:37000 C0:2 C1:32", 8, 8, 8, "")
	plain, parse := p.render, p.parse
	p.render = func(ir *IR, code uint16) string {
		return fmt.Sprintf("%s CHECKSUM:%d", plain(ir, code), *ir.Device^*ir.Subdevice^uint8(code))
	}
	re := regexp.MustCompile(`^(.*) CHECKSUM:\d+$`)
	p.parse = func(desc string) (IR, uint16, bool) {
		m := re.FindStringSubmatch(desc)
		if m == nil {
			return IR{}, 0, false
		}
		ir, f, ok := parse(m[1])
		if !ok || p.render(&ir, f) != desc {
			return IR{}, 0, false
		}
		return ir, f, true
	}
	return p
}

// denonK renders a DenonK descriptor. Its checksum is the Kaseikyo
// check byte (D·16 ^ S ^ F·16 ^ F>>4), which matched all 69 of the hub's
// Denon codes.
func denonK(d, s uint8, f uint16) string {
	sum := (uint16(d)<<4 ^ uint16(s) ^ f<<4 ^ f>>4) & 0xFF
	return fmt.Sprintf("P:DenonK R:37000 C0:84 C1:50 C2:0 D:%d S:%d F:%d CHECKSUM:%d", d, s, f, sum)
}

// denonSharp renders a Denon 15-bit code as the hub's Sharp descriptor.
// The hub reads Denon's frame (5 address bits, 8 function bits, 2
// extension bits, LSB first, then a repeat with the function and
// extension inverted) one bit later than Denon numbers it: C0 is the
// address's low bit, D the rest of the address and the function's low
// bit, F the rest of the function; D_CHECKSUM (and D0) and F_CHECKSUM are
// the same for the repeat. Worked out from the catalog's Power Off (2/122)
// and confirmed on our receiver with 2/121, 2/240, 2/241 and 2/242.
func denonSharp(a, f uint8) string {
	r := f ^ 0xFF
	d, fd := a>>1|(f&1)<<4, f>>1
	dk, fk := a>>1|(r&1)<<4, r>>1|0x80
	return fmt.Sprintf("P:Sharp R:38000 D:%d F:%d C0:%d D_CHECKSUM:%d F_CHECKSUM:%d C1:1 D0:%d", d, fd, a&1, dk, fk, dk)
}

var (
	sharpPattern  = regexp.MustCompile(`^P:Sharp R:38000 D:(\d+) F:(\d+) C0:(\d+) D_CHECKSUM:\d+ F_CHECKSUM:\d+ C1:1 D0:\d+$`)
	necPattern    = regexp.MustCompile(`^P:NEC R:38400 D:(\d+) S:(\d+) F:(\d+)$`)
	denonKPattern = regexp.MustCompile(`^P:DenonK R:37000 C0:84 C1:50 C2:0 D:(\d+) S:(\d+) F:(\d+) CHECKSUM:(\d+)$`)
)

// match returns the decimal fields of desc, each within its bit size.
func match(re *regexp.Regexp, desc string, bits ...int) ([]uint64, bool) {
	m := re.FindStringSubmatch(desc)
	if m == nil {
		return nil, false
	}
	out := make([]uint64, len(bits))
	for i, b := range bits {
		v, err := strconv.ParseUint(m[i+1], 10, b)
		if err != nil {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func protocolNamed(name string) (*irProtocol, error) {
	for i := range protocols {
		if strings.EqualFold(protocols[i].name, name) {
			return &protocols[i], nil
		}
	}
	var names []string
	for _, p := range protocols {
		names = append(names, p.name)
	}
	return nil, fmt.Errorf("profile: ir protocol %q is not supported (%s)", name, strings.Join(names, ", "))
}

// parseCode returns the device IR parameters and code a descriptor stands
// for, in any protocol a profile can express.
func parseCode(desc string) (IR, uint16, bool) {
	for _, p := range protocols {
		if ir, code, ok := p.parse(desc); ok {
			return ir, code, true
		}
	}
	return IR{}, 0, false
}

// same reports whether two devices' IR parameters are the same.
func (ir IR) same(o IR) bool {
	eq := func(a, b *uint8) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	return strings.EqualFold(ir.Protocol, o.Protocol) && ir.System == o.System && eq(ir.Device, o.Device) && eq(ir.Subdevice, o.Subdevice)
}

// Code is a command's code in its device's IR protocol. It keeps the base
// it was written in: hex (0xE5) or decimal (368).
type Code struct {
	Value   uint16
	Decimal bool
}

func (c Code) MarshalYAML() (any, error) {
	v := fmt.Sprintf("0x%02X", c.Value)
	if c.Decimal {
		v = strconv.Itoa(int(c.Value))
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: v}, nil
}

func (c *Code) UnmarshalYAML(n *yaml.Node) error {
	v, err := strconv.ParseUint(n.Value, 0, 16)
	if err != nil {
		return fmt.Errorf("line %d: %q is not a code", n.Line, n.Value)
	}
	lower := strings.ToLower(n.Value)
	*c = Code{Value: uint16(v), Decimal: !strings.HasPrefix(lower, "0x") && !strings.HasPrefix(lower, "0o") && !strings.HasPrefix(lower, "0b")}
	return nil
}
