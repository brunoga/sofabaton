package record

import (
	"encoding/binary"
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

// EntitySortPages returns the payloads of a write of the display order of
// the hub's devices (family 0x11) or activities (0x51), one per page: a
// marker, the page count, a row per entity of its record kind, its id and
// its 1-based place, and a checksum. The order names every device or
// every activity; the hub acks with a status and restamps the records'
// Sort. Our X2 took the app's write of four activities (2026-11-05):
//
//	01 00 01 | 00 66 01 00 68 02 00 67 03 00 65 04 | a6
func EntitySortPages(ents []*Entity) [][]byte {
	n := 3 + 3*len(ents) + 1
	b := make([]byte, 0, n)
	b = append(b, 0x01)
	b = binary.BigEndian.AppendUint16(b, uint16(wire.PageCount(n)))
	for i, e := range ents {
		b = append(b, e.Kind, e.ID, byte(i+1))
	}
	return wire.Pages(0x01, append(b, wire.Checksum(b)))
}

// ParseEntitySort decodes an assembled display-order write (see
// EntitySortPages) into the ids it orders, first place first.
func ParseEntitySort(body []byte) ([]byte, error) {
	if len(body) < 4 || (len(body)-4)%3 != 0 {
		return nil, fmt.Errorf("record: display order of %d bytes", len(body))
	}
	rows := body[3 : len(body)-1]
	ids := make([]byte, len(rows)/3)
	for i := range ids {
		place := int(rows[3*i+2])
		if place < 1 || place > len(ids) || ids[place-1] != 0 {
			return nil, fmt.Errorf("record: display order places %d at %d", rows[3*i+1], place)
		}
		ids[place-1] = rows[3*i+1]
	}
	return ids, nil
}
