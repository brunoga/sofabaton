package record

import (
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

// Body offsets of the device/activity record.
const (
	offKind       = 3
	offID         = 4
	offIcon       = 5
	offSort       = 6
	offClass      = 7
	offType       = 8
	offCodeID     = 9
	offHide       = 25
	offInputFlag  = 26
	offChannel    = 27
	offPowerState = 28
	offName       = 29

	// CodeIDLen is the length of the opaque code id.
	CodeIDLen = 16
)

// Entity is a device or activity record: the fixed-size body of the
// catalog rows and of the create (0x07, 0x37) and update (0x08) writes,
// 120 bytes on the X1 and 210 on the X1S and X2.
type Entity struct {
	// Kind is the record-kind byte (0 on every observed record).
	Kind byte
	ID   byte
	Icon byte
	// Sort is the display position; the hub restamps it on reorders.
	Sort  byte
	Class Class
	// Type is the device category (2 is a TV) or the activity type.
	Type byte
	// CodeID identifies the IR-database code set or Bluetooth profile the
	// device came from. Opaque; all zeros for devices made from scratch.
	CodeID     [CodeIDLen]byte
	Hide       byte
	InputFlag  byte
	Channel    byte
	PowerState byte // the hub maintains it: 1 while the device is on
	Name       string
	Brand      string
	// Tail is the raw tail slot. Its layout depends on the entity: see
	// DeviceTail for devices. Activities keep it as it is.
	Tail []byte

	variant wire.Variant
	raw     []byte // the body as decoded, nil for a new record
}

// NewEntity returns an empty record for variant v, with a zeroed tail.
func NewEntity(v wire.Variant) (*Entity, error) {
	s, err := v.Schema()
	if err != nil {
		return nil, err
	}
	return &Entity{ID: NewID, Tail: make([]byte, s.DeviceSlotLen), variant: v}, nil
}

// ParseEntity decodes a record body of variant v. Only the length is
// checked: the hub does not keep the checksum of stored records current,
// so catalog rows routinely fail it.
func ParseEntity(v wire.Variant, body []byte) (*Entity, error) {
	s, err := v.Schema()
	if err != nil {
		return nil, err
	}
	if len(body) != s.DeviceBodyLen {
		return nil, fmt.Errorf("record: %v entity body is %d bytes, want %d", v, len(body), s.DeviceBodyLen)
	}
	w := s.DeviceSlotLen
	brand := offName + w
	tail := brand + w
	e := &Entity{
		Kind:       body[offKind],
		ID:         body[offID],
		Icon:       body[offIcon],
		Sort:       body[offSort],
		Class:      Class(body[offClass]),
		Type:       body[offType],
		Hide:       body[offHide],
		InputFlag:  body[offInputFlag],
		Channel:    body[offChannel],
		PowerState: body[offPowerState],
		Name:       s.Labels.DecodeSlot(body[offName:brand]),
		Brand:      s.Labels.DecodeSlot(body[brand:tail]),
		Tail:       append([]byte(nil), body[tail:tail+w]...),
		variant:    v,
		raw:        append([]byte(nil), body...),
	}
	copy(e.CodeID[:], body[offCodeID:offCodeID+CodeIDLen])
	return e, nil
}

// Variant is the hub variant the record belongs to.
func (e *Entity) Variant() wire.Variant { return e.variant }

// AppendBody appends the record's body in write form to b: the
// single-page header 01 00 01, every field, and a fresh checksum. Bytes of
// a decoded record that no field covers are kept, and a name or brand that
// was not changed keeps its slot's exact bytes.
func (e *Entity) AppendBody(b []byte) ([]byte, error) {
	s, err := e.variant.Schema()
	if err != nil {
		return b, err
	}
	w := s.DeviceSlotLen
	if e.Tail != nil && len(e.Tail) != w {
		return b, fmt.Errorf("record: tail is %d bytes, want %d", len(e.Tail), w)
	}

	start := len(b)
	if e.raw != nil {
		b = append(b, e.raw...)
	} else {
		b = append(b, make([]byte, s.DeviceBodyLen)...)
	}
	body := b[start:]

	body[0], body[1], body[2] = 0x01, 0x00, 0x01
	body[offKind] = e.Kind
	body[offID] = e.ID
	body[offIcon] = e.Icon
	body[offSort] = e.Sort
	body[offClass] = byte(e.Class)
	body[offType] = e.Type
	copy(body[offCodeID:], e.CodeID[:])
	body[offHide] = e.Hide
	body[offInputFlag] = e.InputFlag
	body[offChannel] = e.Channel
	body[offPowerState] = e.PowerState

	brand := offName + w
	tail := brand + w
	e.putLabel(s, body[offName:brand], e.Name)
	e.putLabel(s, body[brand:tail], e.Brand)
	if e.Tail != nil {
		copy(body[tail:tail+w], e.Tail)
	} else {
		clear(body[tail : tail+w])
	}
	last := len(body) - 1
	body[last] = wire.Checksum(body[:last])
	return b, nil
}

// putLabel writes text into slot unless the slot already decodes to it.
func (e *Entity) putLabel(s wire.Schema, slot []byte, text string) {
	if e.raw != nil && s.Labels.DecodeSlot(slot) == text {
		return
	}
	copy(slot, s.Labels.Slot(text, len(slot)))
}

// MarshalBinary returns the record's body in write form.
func (e *Entity) MarshalBinary() ([]byte, error) {
	return e.AppendBody(nil)
}

// WritePayload is the payload of a create or update write: the paging
// wrapper 01 00 01 and the body. The body always fits one page.
func (e *Entity) WritePayload() ([]byte, error) {
	return e.AppendBody([]byte{0x01, 0x00, 0x01})
}
