package record

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// descriptorMagic follows the length of a descriptive IR code. Upstream
// calls it a magic marker; 0x9470 is 38 000.
var descriptorMagic = []byte{0x00, 0x00, 0x11, 0x00, 0x94, 0x70}

// descriptorPad is the padding the app writes after the text.
const descriptorPad = 4

// Descriptor returns the command data of a descriptive IR code (X2 only):
// text such as "P:NEC R:38400 D:0 S:206 F:217", which the hub renders into
// the signal itself (docs/protocol/ir-codes.md).
func Descriptor(text string) ([]byte, error) {
	if !strings.HasPrefix(text, "P:") {
		return nil, fmt.Errorf("record: IR descriptor %q lacks the P: prefix", text)
	}
	for _, r := range text {
		if r < 0x20 || r > 0x7E {
			return nil, fmt.Errorf("record: IR descriptor %q is not printable ASCII", text)
		}
	}
	b := binary.BigEndian.AppendUint16(nil, uint16(len(text)))
	b = append(b, descriptorMagic...)
	b = append(b, text...)
	return append(b, make([]byte, descriptorPad)...), nil
}

// ErrNotDescriptor reports command data that is not a descriptive IR code
// (a learned code, say).
var ErrNotDescriptor = errors.New("record: not a descriptive IR code")

// ParseDescriptor returns the text of descriptive IR command data.
func ParseDescriptor(data []byte) (string, error) {
	if len(data) < 2+len(descriptorMagic) || !bytes.Equal(data[2:2+len(descriptorMagic)], descriptorMagic) {
		return "", ErrNotDescriptor
	}
	n := int(binary.BigEndian.Uint16(data))
	start := 2 + len(descriptorMagic)
	if len(data) < start+n {
		return "", fmt.Errorf("record: IR descriptor of %d bytes in %d bytes of data", n, len(data))
	}
	return string(data[start : start+n]), nil
}
