// Package profile is a local, hand-editable description of a device's
// commands: a YAML file with comments, which can be exported from a hub,
// edited and applied back. Applying compares the file with the hub and
// makes only the changes needed (see Plan), so commands keep their ids and
// the buttons and macros that refer to them. The commands' order is
// the order of the device's page on the remote, and buttons binds the
// remote's keys to them.
//
//	# AWOL Vision Aetherion Max, from AWOL's IR list
//	device: Aetherion Max
//	brand: AWOL Vision
//	id: 4                          # the hub's device id
//	ir: {protocol: NEC, system: 0x00CE}
//	commands:
//	  - {name: Power On, code: 0xE5}
//	  - {name: Focus, code: 0xD9, comment: opens the focus screen}
//	  - {name: Other, descriptor: "P:NEC R:38400 D:4 S:251 F:114"}
//	  - {name: Learned, data: "00 a4 ..."}   # opaque bytes, as the hub holds them
//	power: {on: Power On, off: Power Off}
//	inputs: [Other]
//	buttons:
//	  Power: Power On
//	  OK: Other
//	  Back: {short: Other, long: Focus}
package profile

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/record"
)

// Device is a device's profile.
type Device struct {
	Name  string `yaml:"device"`
	Brand string `yaml:"brand,omitempty"`
	// From is the template the profile was made from (see FromTemplate).
	From string `yaml:"from,omitempty"`
	// ID is the device's id on the hub the profile belongs to.
	ID uint8 `yaml:"id,omitempty"`
	// IR, when set, lets commands give just a code.
	IR *IR `yaml:"ir,omitempty"`
	// Commands are in the order the remote lists them on the device's
	// page.
	Commands []Command `yaml:"commands"`
	Buttons  Buttons   `yaml:"buttons,omitempty"`
	// Power, when set, makes the device's power macros, which
	// activities run to switch it on and off, send one command each.
	Power *Power `yaml:"power,omitempty"`
	// Inputs, when set, are the commands that select each of the device's
	// inputs, in order: activities switch the device to one of them. An
	// empty list (inputs: []) says the device does not switch inputs.
	Inputs InputList `yaml:"inputs,omitempty"`
}

// InputList is a device's input commands. nil leaves the hub's inputs as
// they are; empty means the device has none to switch.
type InputList []string

// IsZero keeps an empty list in the YAML: only nil is left out.
func (l InputList) IsZero() bool { return l == nil }

// UnmarshalYAML reads a list of command names; [] is an empty list, not
// nil.
func (l *InputList) UnmarshalYAML(n *yaml.Node) error {
	var names []string
	if err := n.Decode(&names); err != nil {
		return err
	}
	if names == nil {
		names = []string{}
	}
	*l = names
	return nil
}

// Power names the commands of a device's POWER_ON and POWER_OFF
// macros.
type Power struct {
	On  string `yaml:"on"`
	Off string `yaml:"off"`
}

// IR is the IR protocol a device's codes share, and its parameters.
type IR struct {
	// Protocol is NEC, DenonK or Denon (Denon's 15-bit codes).
	Protocol string `yaml:"protocol"`
	// System is NEC's 16-bit system code, as manufacturers list it: its
	// high byte becomes the hub's D, its low byte S.
	System Hex16 `yaml:"system,omitempty"`
	// Device and Subdevice are DenonK's (4 and 1 for a Denon receiver's
	// main zone); Denon has a device (2 or 12 for a receiver) only.
	Device    *uint8 `yaml:"device,omitempty"`
	Subdevice *uint8 `yaml:"subdevice,omitempty"`
}

// Command is one command. Exactly one of Code, Descriptor and Data gives
// its signal.
type Command struct {
	Name string `yaml:"name"`
	// Code is the key value (NEC) or function (DenonK) in the device's IR
	// protocol.
	Code *Code `yaml:"code,omitempty"`
	// IR, when set, is the protocol of this command's code, for devices
	// that mix protocols.
	IR *IR `yaml:"ir,omitempty"`
	// Descriptor is a descriptive IR code (X2): "P:NEC R:38400 D:0 ...".
	Descriptor string `yaml:"descriptor,omitempty"`
	// Data is the signal as the hub holds it, in hex, for codes the other
	// fields cannot express (learned, RF, Bluetooth).
	Data    string `yaml:"data,omitempty"`
	Comment string `yaml:"comment,omitempty"`
}

// Signal returns the command's signal data as the hub stores it.
func (c Command) Signal(d *Device) ([]byte, error) {
	switch {
	case c.Code != nil:
		ir := d.IR
		if c.IR != nil {
			ir = c.IR
		}
		if ir == nil {
			return nil, fmt.Errorf("profile: %q has a code but no ir protocol", c.Name)
		}
		p, err := protocolNamed(ir.Protocol)
		if err != nil {
			return nil, err
		}
		if err := p.check(ir); err != nil {
			return nil, fmt.Errorf("%w (%q)", err, c.Name)
		}
		if c.Code.Value > p.maxCode {
			return nil, fmt.Errorf("profile: %q: code %d is beyond %s's %d", c.Name, c.Code.Value, p.name, p.maxCode)
		}
		return record.Descriptor(p.render(ir, c.Code.Value))
	case c.Descriptor != "":
		return record.Descriptor(c.Descriptor)
	case c.Data != "":
		b, err := hex.DecodeString(strings.Join(strings.Fields(c.Data), ""))
		if err != nil {
			return nil, fmt.Errorf("profile: %q data: %w", c.Name, err)
		}
		return b, nil
	}
	return nil, fmt.Errorf("profile: %q has no code, descriptor or data", c.Name)
}

// Validate checks that every command has a name, unique regardless of
// case, and exactly one valid signal, and that the buttons run commands of
// the profile.
func (d *Device) Validate() error {
	if d.Name == "" {
		return errors.New("profile: no device name")
	}
	seen := map[string]bool{}
	var errs []error
	for i, c := range d.Commands {
		if c.Name == "" {
			errs = append(errs, fmt.Errorf("profile: command %d has no name", i+1))
			continue
		}
		key := strings.ToLower(c.Name)
		if seen[key] {
			errs = append(errs, fmt.Errorf("profile: command %q appears twice", c.Name))
		}
		seen[key] = true
		n := 0
		for _, set := range []bool{c.Code != nil, c.Descriptor != "", c.Data != ""} {
			if set {
				n++
			}
		}
		if n != 1 {
			errs = append(errs, fmt.Errorf("profile: %q needs exactly one of code, descriptor and data", c.Name))
			continue
		}
		if _, err := c.Signal(d); err != nil {
			errs = append(errs, err)
		}
	}
	errs = append(errs, d.validateButtons()...)
	if p := d.Power; p != nil {
		for _, c := range []string{p.On, p.Off} {
			if d.find(c) < 0 {
				errs = append(errs, fmt.Errorf("profile: power: %q is not a command of the profile", c))
			}
		}
	}
	seenInput := map[int]bool{}
	for _, c := range d.Inputs {
		i := d.find(c)
		switch {
		case i < 0:
			errs = append(errs, fmt.Errorf("profile: inputs: %q is not a command of the profile", c))
		case seenInput[i]:
			errs = append(errs, fmt.Errorf("profile: inputs: %q appears twice", c))
		}
		seenInput[i] = true
	}
	return errors.Join(errs...)
}

// Load reads and validates a profile. Unknown fields are errors, so typos
// do not pass silently.
func Load(r io.Reader) (*Device, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	var d Device
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// Encode writes the profile as YAML.
func (d *Device) Encode(w io.Writer) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(d); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// Hex16 is a 16-bit value written in hex (0x00CE).
type Hex16 uint16

func (h Hex16) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("0x%04X", uint16(h))}, nil
}

func (h *Hex16) UnmarshalYAML(n *yaml.Node) error {
	v, err := strconv.ParseUint(n.Value, 0, 16)
	if err != nil {
		return fmt.Errorf("line %d: %q is not a 16-bit value", n.Line, n.Value)
	}
	*h = Hex16(v)
	return nil
}

// FromHub builds a device's profile from what the hub holds for it (see
// Read): its commands in the remote's order, with their signals, and its
// key bindings. Descriptive codes become descriptors, other signals data;
// when every command is an NEC code of one system code, they become codes
// of the device's IR protocol.
func FromHub(dev *record.Entity, st *State) (*Device, error) {
	d := &Device{Name: dev.Name, Brand: dev.Brand, ID: dev.ID}
	byID := map[byte]*record.Command{}
	for _, c := range st.Commands {
		byID[c.ID] = c
	}
	for _, id := range displayOrder(st.Commands, st.Order) {
		c := byID[id]
		raw, ok := st.Data[c.ID]
		if !ok {
			return nil, fmt.Errorf("profile: no data for command %d %q", c.ID, c.Label)
		}
		pc := Command{Name: c.Label}
		if desc, err := record.ParseDescriptor(raw); err == nil {
			if trailing := raw[len(raw)-4:]; !bytes.Equal(trailing, make([]byte, 4)) || len(raw) != 8+len(desc)+4 {
				pc.Data = hex.EncodeToString(raw) // keep anything unusual exactly
			} else {
				pc.Descriptor = desc
			}
		} else {
			pc.Data = hex.EncodeToString(raw)
		}
		d.Commands = append(d.Commands, pc)
	}
	// The protocol and parameters most commands share become the device's,
	// and those commands codes in it; commands in another protocol a
	// profile can express get codes with their own ir, and the rest keep
	// their descriptors.
	var best IR
	most := 0
	for _, c := range d.Commands {
		ir, _, ok := parseCode(c.Descriptor)
		if !ok {
			continue
		}
		n := 0
		for _, o := range d.Commands {
			if oir, _, ok := parseCode(o.Descriptor); ok && oir.same(ir) {
				n++
			}
		}
		if n > most {
			best, most = ir, n
		}
	}
	if most > 0 {
		d.IR = &best
		p, _ := protocolNamed(best.Protocol)
		for i, c := range d.Commands {
			ir, code, ok := parseCode(c.Descriptor)
			if !ok {
				continue
			}
			cp := p
			if !ir.same(best) {
				own := ir
				d.Commands[i].IR = &own
				cp, _ = protocolNamed(ir.Protocol)
			}
			d.Commands[i].Code, d.Commands[i].Descriptor = &Code{Value: code, Decimal: cp.decimal}, ""
		}
	}
	name := func(p record.Press) (string, bool) {
		if p.Device != dev.ID {
			return "", false
		}
		c, ok := byID[p.Command]
		if !ok {
			return "", false
		}
		return c.Label, true
	}
	for _, b := range st.Keymap {
		short, ok := name(b.Short)
		if !ok {
			continue // a key of another device's command: not this profile's
		}
		k := Key{Short: short}
		if !b.Long.IsZero() {
			if k.Long, ok = name(b.Long); !ok {
				continue
			}
		}
		if d.Buttons == nil {
			d.Buttons = Buttons{}
		}
		d.Buttons[b.Button.String()] = k
	}
	var power Power
	for _, m := range st.Macros {
		if len(m.Steps) != 1 {
			continue // more than a command: not what a profile describes
		}
		name, ok := name(record.Press{Device: m.Steps[0].Device, Command: m.Steps[0].Command})
		switch {
		case !ok:
		case m.ID == record.MacroPowerOn:
			power.On = name
		case m.ID == record.MacroPowerOff:
			power.Off = name
		}
	}
	if power.On != "" && power.Off != "" {
		d.Power = &power
	}
	if t, err := dev.DeviceTail(); err == nil && t.InputMode == record.InputsNoSwitch {
		d.Inputs = InputList{}
	}
	if in := st.Inputs; in != nil && in.Mode == record.InputsDirect {
		for _, e := range in.Entries {
			if n, ok := name(record.Press{Device: dev.ID, Command: e.Command}); ok {
				d.Inputs = append(d.Inputs, n)
			}
		}
	}
	return d, nil
}

// displayOrder returns the ids of a device's commands in the order the
// remote lists them: those the order places by position, then the others
// by id. A device without an order lists them by id.
func displayOrder(cmds []*record.Command, order []record.Place) []byte {
	pos := map[byte]int{}
	for _, p := range order {
		if p.Placed() {
			pos[p.Command] = int(p.Position)
		}
	}
	ids := make([]byte, 0, len(cmds))
	for _, c := range cmds {
		ids = append(ids, c.ID)
	}
	slices.SortFunc(ids, func(a, b byte) int {
		pa, oka := pos[a]
		pb, okb := pos[b]
		switch {
		case oka && okb && pa != pb:
			return pa - pb
		case oka != okb:
			if oka {
				return -1
			}
			return 1
		}
		return int(a) - int(b)
	})
	return ids
}
