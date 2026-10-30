package wire

import (
	"errors"
	"fmt"
)

const (
	// Sync0 and Sync1 open every frame.
	Sync0 = 0xA5
	Sync1 = 0x5A

	// HeaderLen is the sync and opcode, the bytes before the payload.
	HeaderLen = 4
	// Overhead is the bytes a frame adds to its payload.
	Overhead = HeaderLen + 1
	// MaxPayload is the longest payload the one-byte length can carry.
	MaxPayload = 0xFF
	// MaxFrameLen is the longest frame on the wire.
	MaxFrameLen = Overhead + MaxPayload
)

var (
	// ErrShort reports a buffer that ends before the frame does.
	ErrShort = errors.New("wire: short frame")
	// ErrSync reports a buffer that does not start with A5 5A.
	ErrSync = errors.New("wire: missing sync")
	// ErrChecksum reports a frame whose last byte is not its checksum.
	ErrChecksum = errors.New("wire: bad checksum")
	// ErrPayloadTooLong reports a payload the length byte cannot carry.
	ErrPayloadTooLong = errors.New("wire: payload too long")
)

// Frame is one protocol message.
type Frame struct {
	Family  Family
	Payload []byte
}

// Opcode is the frame's 16-bit opcode: the payload length, then the family.
// It panics if the payload is longer than MaxPayload.
func (f Frame) Opcode() Opcode {
	op, err := NewOpcode(f.Family, len(f.Payload))
	if err != nil {
		panic(err)
	}
	return op
}

// Len is the frame's length on the wire.
func (f Frame) Len() int { return Overhead + len(f.Payload) }

// AppendBinary appends the frame's wire form to b.
func (f Frame) AppendBinary(b []byte) ([]byte, error) {
	if len(f.Payload) > MaxPayload {
		return b, fmt.Errorf("%w: %d bytes", ErrPayloadTooLong, len(f.Payload))
	}
	start := len(b)
	b = append(b, Sync0, Sync1, byte(len(f.Payload)), byte(f.Family))
	b = append(b, f.Payload...)
	return append(b, Checksum(b[start:])), nil
}

// MarshalBinary returns the frame's wire form.
func (f Frame) MarshalBinary() ([]byte, error) {
	return f.AppendBinary(make([]byte, 0, f.Len()))
}

// UnmarshalBinary decodes data, which must be exactly one frame. The payload
// is copied.
func (f *Frame) UnmarshalBinary(data []byte) error {
	fr, n, err := Parse(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return fmt.Errorf("wire: %d bytes after the frame", len(data)-n)
	}
	*f = fr
	return nil
}

// String describes the frame without naming its family, which needs a
// direction (see Family.Name).
func (f Frame) String() string {
	return fmt.Sprintf("%s [% x]", f.Opcode(), f.Payload)
}

// Parse decodes the frame at the start of data and reports how many bytes
// it used. The payload is copied, so data may be reused.
func Parse(data []byte) (Frame, int, error) {
	if len(data) < 2 {
		return Frame{}, 0, ErrShort
	}
	if data[0] != Sync0 || data[1] != Sync1 {
		return Frame{}, 0, ErrSync
	}
	if len(data) < HeaderLen {
		return Frame{}, 0, ErrShort
	}
	n := Overhead + int(data[2])
	if len(data) < n {
		return Frame{}, 0, ErrShort
	}
	if Checksum(data[:n-1]) != data[n-1] {
		return Frame{}, 0, ErrChecksum
	}
	return Frame{
		Family:  Family(data[3]),
		Payload: append([]byte(nil), data[HeaderLen:n-1]...),
	}, n, nil
}

// Checksum is the protocol's checksum: the sum of the bytes, modulo 256.
// Frames are checksummed from the first sync byte to the last payload byte;
// some record bodies carry the same checksum over their own bytes.
func Checksum(b []byte) byte {
	var sum byte
	for _, x := range b {
		sum += x
	}
	return sum
}
