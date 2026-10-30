package record

import (
	"encoding/binary"
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

// BindingLen is the length of a keymap row.
const BindingLen = 18

// Press is what a key press runs: a device's command.
type Press struct {
	Device byte
	// Code is the command's code (as in its Command record); Command its
	// id.
	Code    [CodeLen]byte
	Command byte
}

// IsZero reports whether the press is unset (no long press, for one).
func (p Press) IsZero() bool { return p == Press{} }

// Binding is one keymap row: a key or favorite slot of an activity (or a
// device) and what its short and long presses run. Rows read back from the
// hub (family 0x3D) have the same 18 bytes as the body of a binding write
// (family 0x3E).
type Binding struct {
	Entity byte
	Button Button
	Short  Press
	Long   Press
}

// ParseBinding decodes one 18-byte keymap row.
func ParseBinding(row []byte) (Binding, error) {
	if len(row) != BindingLen {
		return Binding{}, fmt.Errorf("record: keymap row is %d bytes, want %d", len(row), BindingLen)
	}
	r := [BindingLen]byte(row)
	b := Binding{Entity: r[0], Button: Button(r[1])}
	b.Short.Device, b.Short.Command = r[2], r[9]
	copy(b.Short.Code[:], r[3:9])
	b.Long.Device, b.Long.Command = r[10], r[17]
	copy(b.Long.Code[:], r[11:17])
	return b, nil
}

// AppendBinary appends the 18-byte row to buf.
func (b Binding) AppendBinary(buf []byte) ([]byte, error) {
	buf = append(buf, b.Entity, byte(b.Button), b.Short.Device)
	buf = append(buf, b.Short.Code[:]...)
	buf = append(buf, b.Short.Command, b.Long.Device)
	buf = append(buf, b.Long.Code[:]...)
	return append(buf, b.Long.Command), nil
}

// MarshalBinary returns the 18-byte row.
func (b Binding) MarshalBinary() ([]byte, error) { return b.AppendBinary(make([]byte, 0, BindingLen)) }

// WritePayload is the payload of a binding write (family 0x3E): the paging
// wrapper, a one-page body header, the row and the body's checksum. The
// hub acks with family 0x3E echoing the button.
func (b Binding) WritePayload() []byte {
	p := []byte{0x01, 0x00, 0x01, 0x01, 0x00, 0x01}
	p, _ = b.AppendBinary(p)
	return append(p, wire.Checksum(p[3:]))
}

// CodeFromUint packs a 48-bit integer into a command code, big-endian.
func CodeFromUint(v uint64) [CodeLen]byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return [CodeLen]byte(b[2:])
}

// ParseKeymap decodes an assembled keymap body (see wire.Assembler): a
// marker, the page count, the row count, then the rows. Rows span pages.
func ParseKeymap(body []byte) ([]Binding, error) {
	if len(body) < 4 {
		return nil, fmt.Errorf("record: keymap of %d bytes has no header", len(body))
	}
	count := int(body[3])
	rows := body[4:]
	if len(rows) != count*BindingLen {
		return nil, fmt.Errorf("record: keymap says %d rows but holds %d bytes", count, len(rows))
	}
	out := make([]Binding, 0, count)
	for len(rows) >= BindingLen {
		b, err := ParseBinding(rows[:BindingLen])
		if err != nil {
			return nil, err
		}
		out = append(out, b)
		rows = rows[BindingLen:]
	}
	return out, nil
}
