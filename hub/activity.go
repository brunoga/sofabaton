package hub

import (
	"context"
	"fmt"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// ConfirmMember tells the hub a device is to be a member of the activity
// the app is about to create or change (family 0x4F [device, 00]); the
// app sends it for every member before writing the activity.
func (c *Client) ConfirmMember(ctx context.Context, device byte) error {
	if err := c.Writable(); err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamConfirmMember, Payload: []byte{device, 0x00}}, session.Status()); err != nil {
		return fmt.Errorf("hub: confirming member %d: %w", device, err)
	}
	return nil
}

// CreateActivity creates an activity from e (family 0x37; see
// record.NewActivity) and returns the id the hub gives it. The hub takes
// its members from the ConfirmMember calls before it and from its
// POWER_ON sequence, written next.
func (c *Client) CreateActivity(ctx context.Context, e *record.Entity) (byte, error) {
	if err := c.Writable(); err != nil {
		return 0, err
	}
	p, err := e.WritePayload()
	if err != nil {
		return 0, err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	a := &session.Ack{Family: wire.FamCreateActAck}
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamCreateActivity, Payload: p}, a); err != nil {
		return 0, fmt.Errorf("hub: creating activity %q: %w", e.Name, err)
	}
	id := byte(0)
	if len(a.Frame.Payload) > 0 {
		id = a.Frame.Payload[0]
	}
	if !record.IsActivityID(id) {
		return 0, fmt.Errorf("hub: creating activity %q: ack with id %d", e.Name, id)
	}
	return id, nil
}

// UpdateActivity rewrites an activity's record (family 0x38), as the hub
// holds it but for the changes: the app renames activities this way, and
// writes the record after changing the members.
func (c *Client) UpdateActivity(ctx context.Context, e *record.Entity) error {
	if err := c.Writable(); err != nil {
		return err
	}
	p, err := e.WritePayload()
	if err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamWriteActivity, Payload: p}, session.Status()); err != nil {
		return fmt.Errorf("hub: updating activity %d: %w", e.ID, err)
	}
	return nil
}

// DeleteActivity deletes an activity (family 0x39 [activity]) with its
// keys and sequences.
func (c *Client) DeleteActivity(ctx context.Context, id byte) error {
	if err := c.Writable(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DeleteTimeout)
		defer cancel()
	}
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamDeleteActivity, Payload: []byte{id}}, session.Status()); err != nil {
		return fmt.Errorf("hub: deleting activity %d: %w", id, err)
	}
	return nil
}

// CreateMacro writes a new sequence of an activity, m with ID 0, and
// returns the id the hub gives it (the last page's ack). The app then
// places it on the remote's list with WriteOrder, entity for device, and
// syncs.
func (c *Client) CreateMacro(ctx context.Context, m *record.Macro) (byte, error) {
	if err := c.Writable(); err != nil {
		return 0, err
	}
	if m.ID != 0 {
		return 0, fmt.Errorf("hub: creating macro %q: id %d is not 0", m.Label, m.ID)
	}
	pages, err := m.WritePages()
	if err != nil {
		return 0, err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	var last *session.Ack
	for i, p := range pages {
		ack := session.Status()
		if i == len(pages)-1 {
			last = &session.Ack{Family: wire.FamMacroAck}
			ack = last
		}
		if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamWriteMacro, Payload: p}, ack); err != nil {
			return 0, fmt.Errorf("hub: creating macro %q of entity %d, page %d of %d: %w", m.Label, m.Entity, i+1, len(pages), err)
		}
	}
	if len(last.Frame.Payload) == 0 || last.Frame.Payload[0] == 0 {
		return 0, fmt.Errorf("hub: creating macro %q: ack without an id", m.Label)
	}
	return last.Frame.Payload[0], nil
}

// DeleteMacro deletes a sequence of an activity (family 0x10 [entity,
// id], the frame that deletes a device's command; upstream, X1 and X1S).
func (c *Client) DeleteMacro(ctx context.Context, entity, id byte) error {
	if err := c.Writable(); err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamDeleteKey, Payload: []byte{entity, id}}, session.Status()); err != nil {
		return fmt.Errorf("hub: deleting macro %d of entity %d: %w", id, entity, err)
	}
	return nil
}
