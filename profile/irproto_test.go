package profile

import "testing"

// The protocols in irdb's terms render the descriptors upstream verified by
// the light the X2 sent (docs/protocol/ir-codes.md), and parse back.
func TestFieldProtocols(t *testing.T) {
	u8 := func(v uint8) *uint8 { return &v }
	for _, tc := range []struct {
		ir   IR
		f    uint16
		want string
	}{
		{IR{Protocol: "NECx", Device: u8(7), Subdevice: u8(7)}, 96, "P:NECx R:38400 D:7 S:7 F:96"},
		{IR{Protocol: "Sony12", Device: u8(1)}, 116, "P:Sony12 R:40000 D:1 F:116 MUL:2"},
		{IR{Protocol: "Sony15", Device: u8(1)}, 116, "P:Sony15 R:40000 D:1 F:116 MUL:2"},
		{IR{Protocol: "Sony20", Device: u8(26), Subdevice: u8(226)}, 59, "P:Sony20 R:40000 D:26 S:226 F:59 MUL:2"},
		{IR{Protocol: "JVC", Device: u8(3)}, 23, "P:JVC R:38000 D:3 F:23"},
		{IR{Protocol: "RC6", Device: u8(4)}, 90, "P:RC6 R:36000 D:4 F:90 MUL:1"},
		{IR{Protocol: "Samsung20", Device: u8(7), Subdevice: u8(7)}, 23, "P:Samsung20 R:38000 D:7 S:7 F:23"},
		{IR{Protocol: "Panasonic", Device: u8(128), Subdevice: u8(0)}, 74, "P:Panasonic R:37000 C0:2 C1:32 D:128 S:0 F:74 CHECKSUM:202"},
		{IR{Protocol: "Panasonic", Device: u8(128), Subdevice: u8(0)}, 79, "P:Panasonic R:37000 C0:2 C1:32 D:128 S:0 F:79 CHECKSUM:207"},
	} {
		p, err := protocolNamed(tc.ir.Protocol)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.check(&tc.ir); err != nil {
			t.Errorf("%s: %v", tc.want, err)
			continue
		}
		if got := p.render(&tc.ir, tc.f); got != tc.want {
			t.Errorf("%s renders %q", tc.want, got)
		}
		ir, f, ok := ParseCode(tc.want)
		if !ok || !ir.same(tc.ir) || f != tc.f {
			t.Errorf("%s parses as %+v %d %v", tc.want, ir, f, ok)
		}
	}
	// Forms the hub does not send right, or that are not ours, stay
	// descriptors.
	for _, desc := range []string{
		"P:Panasonic R:37000 C0:2 C1:32 D:128 S:0 F:74 CHECKSUM:202 MUL:1",
		"P:Panasonic R:37000 C0:2 C1:32 D:128 S:0 F:74 CHECKSUM:7",
		"P:Sony12 R:40000 D:40 F:116 MUL:2",
		"P:Samsung32 R:38000 D:7 S:7 F:2",
	} {
		if _, _, ok := ParseCode(desc); ok {
			t.Errorf("%s parses", desc)
		}
	}
	// Out of range.
	p, _ := protocolNamed("Sony12")
	if err := p.check(&IR{Protocol: "Sony12", Device: u8(32)}); err == nil {
		t.Error("Sony12 device 32 checks")
	}
	if err := p.check(&IR{Protocol: "Sony12", Device: u8(1), Subdevice: u8(1)}); err == nil {
		t.Error("Sony12 with a subdevice checks")
	}
}
