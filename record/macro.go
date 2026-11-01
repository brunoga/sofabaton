package record

import (
	"fmt"
	"strings"

	"github.com/brunoga/sofabaton/wire"
)

// StepLen is the length of a macro step.
const StepLen = 10

// Macro ids the hub uses for an entity's power macros; user macros
// take other ids.
const (
	MacroPowerOn  byte = 0xC6
	MacroPowerOff byte = 0xC7
)

// Step is one step of a macro: a device's command, or a pause (Command
// 0xFF), with a hold duration and the delay after it.
type Step struct {
	Device   byte
	Command  byte
	Code     [CodeLen]byte
	Duration byte
	Delay    byte
}

// IsDelay reports whether the step only waits.
func (s Step) IsDelay() bool { return s.Command == 0xFF }

// Macro is a sequence of commands bound to a key of an activity or a
// device: a user macro, or the power-on/off macro (MacroPowerOn,
// MacroPowerOff). The record read from the hub (family 0x13) and the body
// of a macro write (family 0x12) share a layout.
type Macro struct {
	Entity byte
	ID     byte
	Steps  []Step
	Label  string
	// Origin is the device a copy of a device's macro in an activity
	// belongs to, 0 for none. The remote names a device's own macros
	// none of its own way (it shows every one as one blank entry), so
	// device macros live as copies in the activities with the device.
	// The id is kept in the label slot after the label's terminator, as
	// "SB" and the id: the hub keeps the slot's bytes and the remote
	// shows only the label.
	Origin byte

	variant wire.Variant
	slot    []byte // the label slot as read; the hub keeps metadata in it
	origin  byte   // Origin as read
}

// originTag marks a label slot's Origin.
var originTag = [2]byte{'S', 'B'}

// labelEnd is the offset in a label slot just past the label's
// terminator, or the slot's length when the label fills it.
func labelEnd(enc wire.TextEncoding, slot []byte) int {
	if enc == wire.UTF16BE {
		for i := 0; i+1 < len(slot); i += 2 {
			if slot[i] == 0 && slot[i+1] == 0 {
				return i + 2
			}
		}
		return len(slot)
	}
	for i, c := range slot {
		if c == 0 {
			return i + 1
		}
	}
	return len(slot)
}

// slotOrigin reads a label slot's Origin.
func slotOrigin(enc wire.TextEncoding, slot []byte) byte {
	i := labelEnd(enc, slot)
	if i+3 > len(slot) || slot[i] != originTag[0] || slot[i+1] != originTag[1] {
		return 0
	}
	if d := slot[i+2]; d >= MinDeviceID && d <= MaxDeviceID {
		return d
	}
	return 0
}

// ParseMacro decodes one macro record body of variant v (one record of a
// macro burst, see wire.Burst).
func ParseMacro(v wire.Variant, body []byte) (*Macro, error) {
	s, err := v.Schema()
	if err != nil {
		return nil, err
	}
	if len(body) < 6 {
		return nil, fmt.Errorf("record: macro of %d bytes", len(body))
	}
	n := int(body[5])
	want := 6 + n*StepLen + s.MacroLabelLen + 1
	if len(body) != want {
		return nil, fmt.Errorf("record: macro with %d steps is %d bytes, want %d", n, len(body), want)
	}
	m := &Macro{Entity: body[3], ID: body[4], Steps: make([]Step, 0, n), variant: v}
	for i := range n {
		r := [StepLen]byte(body[6+i*StepLen:])
		st := Step{Device: r[0], Command: r[1], Duration: r[8], Delay: r[9]}
		copy(st.Code[:], r[2:8])
		m.Steps = append(m.Steps, st)
	}
	slot := body[6+n*StepLen : len(body)-1]
	m.slot = append([]byte(nil), slot...)
	m.Label = decodeMacroLabel(s.Labels, slot)
	if m.ID != MacroPowerOn && m.ID != MacroPowerOff {
		m.Origin = slotOrigin(s.Labels, slot)
		m.origin = m.Origin
	}
	return m, nil
}

// decodeMacroLabel reads a macro label. The hub's own power-macro labels
// can carry other bytes before the text, so those are found anywhere in
// the slot.
func decodeMacroLabel(enc wire.TextEncoding, slot []byte) string {
	all := enc.DecodeAll(slot)
	for _, name := range []string{"POWER_ON", "POWER_OFF"} {
		if i := strings.Index(all, name); i >= 0 {
			text, _, _ := strings.Cut(all[i:], "\x00")
			return strings.TrimSpace(text)
		}
	}
	return enc.DecodeSlot(slot)
}

// Variant is the hub variant the record belongs to.
func (m *Macro) Variant() wire.Variant { return m.variant }

// NewMacro returns an empty macro for variant v.
func NewMacro(v wire.Variant, entity, id byte, label string) (*Macro, error) {
	if _, err := v.Schema(); err != nil {
		return nil, err
	}
	return &Macro{Entity: entity, ID: id, Label: label, variant: v}, nil
}

// Body returns the macro's write body: marker, page count, entity, id, the
// steps, the label slot and the checksum. A decoded macro whose label is
// unchanged keeps its slot's exact bytes: the hub stores metadata after the
// label and rejects saves that change it.
func (m *Macro) Body() ([]byte, error) {
	s, err := m.variant.Schema()
	if err != nil {
		return nil, err
	}
	if len(m.Steps) > 0xFF {
		return nil, fmt.Errorf("record: macro has %d steps, at most 255 fit", len(m.Steps))
	}
	n := 6 + len(m.Steps)*StepLen + s.MacroLabelLen + 1
	pages := wire.PageCount(n)
	b := make([]byte, 0, n)
	b = append(b, 0x01, byte(pages>>8), byte(pages), m.Entity, m.ID, byte(len(m.Steps)))
	for _, st := range m.Steps {
		b = append(b, st.Device, st.Command)
		b = append(b, st.Code[:]...)
		b = append(b, st.Duration, st.Delay)
	}
	if m.slot != nil && decodeMacroLabel(s.Labels, m.slot) == m.Label && m.origin == m.Origin {
		b = append(b, m.slot...)
	} else {
		start := len(b)
		b = s.Labels.AppendSlot(b, m.Label, s.MacroLabelLen)
		if m.Origin != 0 {
			i := start + labelEnd(s.Labels, b[start:])
			if s.Labels.DecodeSlot(b[start:]) != m.Label || i+3 > len(b) {
				return nil, fmt.Errorf("record: macro %q is too long to carry its device", m.Label)
			}
			b[i], b[i+1], b[i+2] = originTag[0], originTag[1], m.Origin
		}
	}
	return append(b, wire.Checksum(b)), nil
}

// WritePages returns the payloads of the macro's write (family 0x12), one
// per page. The hub acks every page but the last with a status ack and the
// last with family 0x12.
func (m *Macro) WritePages() ([][]byte, error) {
	body, err := m.Body()
	if err != nil {
		return nil, err
	}
	return wire.Pages(0x01, body), nil
}
