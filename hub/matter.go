package hub

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// The X2 is a Matter device (firmware 9 and later): a Home app (Apple's,
// Google's, …) adds it from its setup code, and it offers switches, up to
// eight, whose presses the Home app's automations act on. The switches are
// the commands of a device of the hub's, which the Sofabaton app reads
// with family 0x72 and treats as device 51; remote keys and macros press
// them as they run any command. (The app's code, 2026-11-12; our X2 gave
// its setup code and answered 0x72 with status 0x07 before any Home app
// had added it.)

// MatterCode is the X2's Matter setup code: QR is the QR code's text
// ("MT:…"), Manual the code to type instead.
type MatterCode struct {
	QR     string
	Manual string
}

// MatterCode reads the hub's Matter setup code, to add the hub to a Home
// app (family 0x81, answered in 0x81 with the QR code's text and the
// manual code, split by a carriage return).
func (c *Client) MatterCode(ctx context.Context) (MatterCode, error) {
	if c.variant != wire.X2 {
		return MatterCode{}, fmt.Errorf("%w: Matter is the X2's", ErrUnsupported)
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	ack := &session.Ack{Family: wire.FamMatterCode}
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamReqMatterCode}, ack); err != nil {
		return MatterCode{}, fmt.Errorf("hub: Matter setup code: %w", err)
	}
	return ParseMatterCode(ack.Frame.Payload)
}

// ParseMatterCode decodes the hub's reply to a request for its Matter setup
// code: a byte our X2 sent first (the QR text's length), the QR text, a
// carriage return and the manual code (2026-11-12).
func ParseMatterCode(payload []byte) (MatterCode, error) {
	text := strings.TrimFunc(string(payload), func(r rune) bool { return r < ' ' })
	qr, manual, ok := strings.Cut(text, "\r")
	qr = strings.TrimFunc(qr, func(r rune) bool { return r <= ' ' })
	manual = strings.TrimSpace(manual)
	if !ok || !strings.HasPrefix(qr, "MT:") || manual == "" {
		return MatterCode{}, fmt.Errorf("hub: Matter setup code %q is not a QR text and a manual code", text)
	}
	return MatterCode{QR: qr, Manual: manual}, nil
}

// MatterDevice reads the record of the device whose commands are the
// hub's Matter switches (family 0x72, answered as a device catalog of one
// row, in the app's code). A hub that has none yet, as ours before a Home
// app added it, answers ErrNotFound.
func (c *Client) MatterDevice(ctx context.Context) (*record.Entity, error) {
	if c.variant != wire.X2 {
		return nil, fmt.Errorf("%w: Matter is the X2's", ErrUnsupported)
	}
	ents, err := c.entities(ctx, wire.Frame{Family: wire.FamReqMatterDevice}, wire.FamMatterDevice)
	if err != nil {
		return nil, notFound(err)
	}
	if len(ents) != 1 {
		return nil, errors.New("hub: the Matter device's reply has no single record")
	}
	return ents[0], nil
}
