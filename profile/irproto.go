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
}

// denonK renders a DenonK descriptor. Its checksum is the Kaseikyo
// check byte (D·16 ^ S ^ F·16 ^ F>>4), which matched all 69 of the hub's
// Denon codes.
func denonK(d, s uint8, f uint16) string {
	sum := (uint16(d)<<4 ^ uint16(s) ^ f<<4 ^ f>>4) & 0xFF
	return fmt.Sprintf("P:DenonK R:37000 C0:84 C1:50 C2:0 D:%d S:%d F:%d CHECKSUM:%d", d, s, f, sum)
}

var (
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
