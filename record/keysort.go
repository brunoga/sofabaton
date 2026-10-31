package record

import (
	"encoding/binary"
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

// Unplaced is the position of a command the remote's device page does not
// order.
const Unplaced = 0xFF

// Place is a command's position on the remote's device page: 1 is the
// first; 0 and Unplaced mean none.
type Place struct {
	Command  byte
	Position byte
}

// Placed reports whether p gives a position.
func (p Place) Placed() bool { return p.Position != 0 && p.Position != Unplaced }

// ParseOrder decodes a device's command order, the hub's reply to an order
// request (family 0x62 [device], answered in family 0x63): a marker, the
// page count, the device id, then (command, position) pairs. The reply
// has no checksum of its own.
func ParseOrder(body []byte) (device byte, order []Place, err error) {
	if len(body) < 4 || (len(body)-4)%2 != 0 {
		return 0, nil, fmt.Errorf("record: command order of %d bytes", len(body))
	}
	for p := body[4:]; len(p) >= 2; p = p[2:] {
		order = append(order, Place{Command: p[0], Position: p[1]})
	}
	return body[3], order, nil
}

// OrderPages returns the payloads of a write of a device's command order
// (family 0x61), one per page: the body of a reply (see ParseOrder) with
// a checksum. The hub acks every page with a status ack. It refuses a
// table that places nothing.
func OrderPages(device byte, order []Place) [][]byte {
	n := 4 + 2*len(order) + 1
	b := make([]byte, 0, n)
	b = append(b, 0x01)
	b = binary.BigEndian.AppendUint16(b, uint16(wire.PageCount(n)))
	b = append(b, device)
	for _, p := range order {
		b = append(b, p.Command, p.Position)
	}
	return wire.Pages(0x01, append(b, wire.Checksum(b)))
}
