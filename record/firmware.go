package record

import (
	"crypto/md5" //nolint:gosec // G501: the hub checks firmware images by MD5
	"encoding/binary"
	"fmt"
)

// A hub firmware update, as the Android app sent our X2 the image the
// vendor's server gave it (2026-10-30; docs/protocol/firmware-update.md):
// a start naming the image's size and MD5, then the image in numbered
// chunks, one at a time, each acknowledged.

// FirmwareChunkLen is the image bytes in each chunk but the last.
const FirmwareChunkLen = 236

// firmwareSelector leads every payload of an update: the hub itself, we
// guess, as against something it relays to.
var firmwareSelector = [6]byte{0, 0, 1, 0, 0, 0}

// FirmwareStart is the payload of the frame that starts the update of
// image: its size, big-endian, and its MD5, which our X2's image from
// the server matched.
func FirmwareStart(image []byte) []byte {
	sum := md5.Sum(image) //nolint:gosec // G401: the hub's check, not a security one
	p := append([]byte(nil), firmwareSelector[:]...)
	p = binary.BigEndian.AppendUint32(p, uint32(len(image)))
	return append(p, sum[:]...)
}

// FirmwareChunks are the payloads of the chunks of image, numbered from 1.
func FirmwareChunks(image []byte) ([][]byte, error) {
	n := (len(image) + FirmwareChunkLen - 1) / FirmwareChunkLen
	if len(image) == 0 || n > 0xFFFF {
		return nil, fmt.Errorf("record: a firmware image of %d bytes", len(image))
	}
	out := make([][]byte, 0, n)
	for i := range n {
		p := append([]byte(nil), firmwareSelector[:]...)
		p = append(p, 1)
		p = binary.BigEndian.AppendUint16(p, uint16(i+1))
		out = append(out, append(p, image[i*FirmwareChunkLen:min(len(image), (i+1)*FirmwareChunkLen)]...))
	}
	return out, nil
}

// ParseFirmwareAck is the number a FirmwareAck acknowledges: 0 for the
// start, then each chunk's.
func ParseFirmwareAck(p []byte) (int, error) {
	if len(p) != 8 || [6]byte(p[:6]) != firmwareSelector {
		return 0, fmt.Errorf("record: firmware ack % x", p)
	}
	return int(binary.BigEndian.Uint16(p[6:])), nil
}
