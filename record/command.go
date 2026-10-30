package record

import (
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

// CodeLen is the length of a command's code field.
const CodeLen = 6

const (
	cmdOffType  = 2
	cmdOffCode  = 3
	cmdOffLabel = 9
)

// Command is one record of a device's command list (the reply to a
// commands request, family 0x5D): 40 bytes on the X1, 70 on the X1S and
// X2. It names a command; the signal itself is a separate blob.
type Command struct {
	Device byte
	ID     byte
	// Type is the command's transport (IR, RF, a Wi-Fi class). It can
	// differ from the device's class.
	Type Class
	// Code identifies the command in its code library. Opaque.
	Code  [CodeLen]byte
	Label string
	// Sort is the record's last byte: the command id on X1S/X2 captures,
	// 0xFF on X1 ones.
	Sort byte

	variant wire.Variant
	raw     []byte
}

// ParseCommand decodes one command record of variant v.
func ParseCommand(v wire.Variant, rec []byte) (*Command, error) {
	s, err := v.Schema()
	if err != nil {
		return nil, err
	}
	if len(rec) != s.CommandStride {
		return nil, fmt.Errorf("record: %v command record is %d bytes, want %d", v, len(rec), s.CommandStride)
	}
	head := [cmdOffLabel]byte(rec)
	label := rec[cmdOffLabel : cmdOffLabel+s.CommandLabelLen]
	sort := rec[len(rec)-1]
	c := &Command{
		Device:  head[0],
		ID:      head[1],
		Type:    Class(head[cmdOffType]),
		Label:   s.Labels.DecodeSlot(label),
		Sort:    sort,
		variant: v,
		raw:     append([]byte(nil), rec...),
	}
	copy(c.Code[:], head[cmdOffCode:])
	return c, nil
}

// Variant is the hub variant the record belongs to.
func (c *Command) Variant() wire.Variant { return c.variant }

// AppendRecord appends the record to b. A decoded record keeps the bytes no
// field covers, and its label slot when the label is unchanged.
func (c *Command) AppendRecord(b []byte) ([]byte, error) {
	s, err := c.variant.Schema()
	if err != nil {
		return b, err
	}
	start := len(b)
	if c.raw != nil {
		b = append(b, c.raw...)
	} else {
		b = append(b, make([]byte, s.CommandStride)...)
	}
	rec := b[start:]
	rec[0], rec[1], rec[cmdOffType] = c.Device, c.ID, byte(c.Type)
	copy(rec[cmdOffCode:], c.Code[:])
	slot := rec[cmdOffLabel : cmdOffLabel+s.CommandLabelLen]
	if c.raw == nil || s.Labels.DecodeSlot(slot) != c.Label {
		copy(slot, s.Labels.Slot(c.Label, len(slot)))
	}
	rec[s.CommandStride-1] = c.Sort
	return b, nil
}

// MarshalBinary returns the record's bytes.
func (c *Command) MarshalBinary() ([]byte, error) { return c.AppendRecord(nil) }

// ParseCommandList decodes an assembled command-list body (see
// wire.Assembler): a marker, the page count, the command count, then the
// records back to back.
func ParseCommandList(v wire.Variant, body []byte) ([]*Command, error) {
	s, err := v.Schema()
	if err != nil {
		return nil, err
	}
	if len(body) < 4 {
		return nil, fmt.Errorf("record: command list of %d bytes has no header", len(body))
	}
	count := int(body[3])
	recs := body[4:]
	if len(recs) != count*s.CommandStride {
		return nil, fmt.Errorf("record: command list says %d commands but holds %d bytes of %d-byte records",
			count, len(recs), s.CommandStride)
	}
	out := make([]*Command, 0, count)
	for i := range count {
		c, err := ParseCommand(v, recs[i*s.CommandStride:(i+1)*s.CommandStride])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
