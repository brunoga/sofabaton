package hub

import (
	"context"
	"fmt"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// UpdateFirmware sends the hub a firmware image, as the Android app sent
// our X2 the one the vendor's server gave it: a start the hub
// acknowledges, then the chunks one at a time, each acknowledged with its
// number but the last, which FirmwareDone answers. The hub then reboots
// without closing the connection, so the client is done with: connect
// again once the hub is back (a minute or so). progress, if set, is
// called after each chunk.
//
// The image must be one the vendor made for this hub's model: what the
// hub does with another is not known.
func (c *Client) UpdateFirmware(ctx context.Context, image []byte, progress func(done, total int)) error {
	if err := c.Writable(); err != nil {
		return err
	}
	chunks, err := record.FirmwareChunks(image)
	if err != nil {
		return err
	}
	ack := func(n int) session.Collector {
		return session.CollectorFunc(func(f wire.Frame) (bool, bool, error) {
			switch f.Family {
			case wire.FamFirmwareAck:
				got, err := record.ParseFirmwareAck(f.Payload)
				if err != nil || got != n {
					return true, true, fmt.Errorf("hub: firmware: acknowledged %d after %d (%v)", got, n, err)
				}
				return true, true, nil
			case wire.FamFirmwareDone:
				if n != len(chunks) {
					return true, true, fmt.Errorf("hub: firmware: done after chunk %d of %d", n, len(chunks))
				}
				return true, true, nil
			case wire.FamStatusAck:
				if len(f.Payload) > 0 && f.Payload[0] != 0 {
					return true, true, fmt.Errorf("hub: firmware: refused with status %#02x", f.Payload[0])
				}
			}
			return false, false, nil
		})
	}
	// Each exchange is bounded, not the whole: 217 chunks took our X2 a
	// minute.
	exchange := func(f wire.Frame, n int) error {
		ectx, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()
		return c.s.Exchange(ectx, f, ack(n))
	}
	if err := exchange(wire.Frame{Family: wire.FamFirmwareStart, Payload: record.FirmwareStart(image)}, 0); err != nil {
		return fmt.Errorf("hub: firmware start: %w", err)
	}
	for i, p := range chunks {
		if err := exchange(wire.Frame{Family: wire.FamFirmwareChunk, Payload: p}, i+1); err != nil {
			return fmt.Errorf("hub: firmware chunk %d of %d: %w", i+1, len(chunks), err)
		}
		if progress != nil {
			progress(i+1, len(chunks))
		}
	}
	return nil
}
