package record

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// RawIR is an IR signal as timings: what a hub learns, and what it sends
// for codes it has no descriptor for (docs/protocol/ir-codes.md).
type RawIR struct {
	// Carrier is the carrier's frequency, in Hz.
	Carrier int
	// Timings are the signal's marks and spaces, in µs, mark first.
	Timings []uint32
}

// rawGap is the space a signal ending on a mark gets: the hub's signals
// end on one (upstream's choice, hub-validated).
const rawGap = 40000

// ErrNotRaw reports command data that is not a raw IR signal.
var ErrNotRaw = errors.New("record: not a raw IR signal")

// Encode returns the command data of r: the timings' length in bytes, a
// zero format field, the carrier, the timings and a zero, all big-endian.
// A signal ending on a mark gets a 40 ms space.
func (r RawIR) Encode() ([]byte, error) {
	if len(r.Timings) == 0 {
		return nil, errors.New("record: a raw IR signal of no timings")
	}
	if r.Carrier <= 0 || r.Carrier > 0xFFFF {
		return nil, fmt.Errorf("record: carrier %d Hz out of range", r.Carrier)
	}
	t := r.Timings
	if len(t)%2 == 1 {
		t = append(t[:len(t):len(t)], rawGap)
	}
	if 4*len(t) > 0xFFFF {
		return nil, fmt.Errorf("record: a raw IR signal of %d timings is too long", len(t))
	}
	b := binary.BigEndian.AppendUint16(nil, uint16(4*len(t)))
	b = append(b, 0, 0, 0, 0)
	b = binary.BigEndian.AppendUint16(b, uint16(r.Carrier))
	for _, v := range t {
		if v == 0 {
			return nil, errors.New("record: a raw IR timing of zero")
		}
		b = binary.BigEndian.AppendUint32(b, v)
	}
	return append(b, 0, 0, 0, 0), nil
}

// ParseRawIR returns the raw IR signal of command data, or ErrNotRaw.
func ParseRawIR(data []byte) (RawIR, error) {
	if len(data) < 16 || data[2] != 0 || data[3] != 0 || data[4] != 0 || data[5] != 0 {
		return RawIR{}, ErrNotRaw
	}
	n := int(binary.BigEndian.Uint16(data))
	if n == 0 || n%4 != 0 || len(data) != 8+n+4 {
		return RawIR{}, ErrNotRaw
	}
	r := RawIR{Carrier: int(binary.BigEndian.Uint16(data[6:]))}
	if r.Carrier == 0 {
		return RawIR{}, ErrNotRaw
	}
	for i := 8; i < 8+n; i += 4 {
		r.Timings = append(r.Timings, binary.BigEndian.Uint32(data[i:]))
	}
	return r, nil
}

// IsIR reports whether command data is an IR signal: a descriptor or raw
// timings, as opposed to a Bluetooth or network command's data.
func IsIR(data []byte) bool {
	if _, err := ParseDescriptor(data); err == nil {
		return true
	}
	_, err := ParseRawIR(data)
	return err == nil
}
