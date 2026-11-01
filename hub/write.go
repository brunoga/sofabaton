package hub

import (
	"context"
	"fmt"
	"time"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// WriteCommand writes cmd, with its signal data (a record.Descriptor,
// say), as command cmd.ID of its device: a command burst of one (family
// 0x0E), every page acknowledged with a status ack. A free id adds the
// command (see NextCommandID); a used one overwrites it in place, which
// renames or recodes it and keeps the buttons that refer to it.
func (c *Client) WriteCommand(ctx context.Context, cmd *record.Command, data []byte) error {
	if err := c.Writable(); err != nil {
		return err
	}
	pages, err := cmd.WritePages(1, 1, data)
	if err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	for i, p := range pages {
		if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamWriteCommand, Payload: p}, session.Status()); err != nil {
			return fmt.Errorf("hub: writing command %q, page %d of %d: %w", cmd.Label, i+1, len(pages), err)
		}
	}
	return nil
}

// DeleteTimeout bounds a delete whose context has no deadline: the hub
// sweeps its configuration before acknowledging, which upstream timed at
// up to 12 s.
const DeleteTimeout = 60 * time.Second

// DeleteCommand deletes a command of a device (family 0x10, the frame that
// deletes an activity's key rows, scoped to a device). The hub drops the
// bindings that referred to it.
func (c *Client) DeleteCommand(ctx context.Context, device, id byte) error {
	if err := c.Writable(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DeleteTimeout)
		defer cancel()
	}
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamDeleteKey, Payload: []byte{device, id}}, session.Status()); err != nil {
		return fmt.Errorf("hub: deleting command %d of device %d: %w", id, device, err)
	}
	return nil
}

// NextCommandID is the lowest command id the commands do not use.
func NextCommandID(cmds []*record.Command) (byte, error) {
	used := map[byte]bool{}
	for _, c := range cmds {
		used[c.ID] = true
	}
	for id := 1; id < 0xFF; id++ {
		if !used[byte(id)] {
			return byte(id), nil
		}
	}
	return 0, fmt.Errorf("hub: no free command id")
}

// Run makes the hub carry out steps now, as a macro of entity that is not
// stored (family 0x5F; the app sends one to test a power command). A step
// names its command by device, id and code, as in record.Macro.
func (c *Client) Run(ctx context.Context, entity byte, steps []record.Step) error {
	if err := c.Writable(); err != nil {
		return err
	}
	m, err := record.NewMacro(c.variant, entity, 0, "")
	if err != nil {
		return err
	}
	m.Steps = steps
	pages, err := m.WritePages()
	if err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	for i, p := range pages {
		if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamRunMacro, Payload: p}, session.Status()); err != nil {
			return fmt.Errorf("hub: running %d steps, page %d of %d: %w", len(steps), i+1, len(pages), err)
		}
	}
	return nil
}

// StepFor is the macro step that sends cmd once.
func StepFor(cmd *record.Command) record.Step {
	return record.Step{Device: cmd.Device, Command: cmd.ID, Code: cmd.Code, Delay: 0xFF}
}

// WriteButton writes one key binding of a device or activity (family
// 0x3E), replacing the key's binding; the hub acks with the button. The
// app names the command by its code alone and the hub fills in the id.
func (c *Client) WriteButton(ctx context.Context, b record.Binding) error {
	if err := c.Writable(); err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	echo := byte(b.Button)
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamWriteButton, Payload: b.WritePayload()}, &session.Ack{Family: wire.FamButtonAck, Echo: &echo}); err != nil {
		return fmt.Errorf("hub: writing button %v of entity %d: %w", b.Button, b.Entity, err)
	}
	return nil
}

// WriteOrder writes the order of a device's commands on the remote's
// device page (family 0x61), replacing the device's order.
func (c *Client) WriteOrder(ctx context.Context, device byte, order []record.Place) error {
	if err := c.Writable(); err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	pages := record.OrderPages(device, order)
	for i, p := range pages {
		if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamKeySort, Payload: p}, session.Status()); err != nil {
			return fmt.Errorf("hub: writing the command order of device %d, page %d of %d: %w", device, i+1, len(pages), err)
		}
	}
	return nil
}

// CreateDevice creates a device from e (family 0x07; e's id is
// record.NewID) and returns the id the hub gives it. The device is not
// committed until UpdateDevice writes it with its tail's Marker set; the
// app writes its commands, keys and power macros in between.
func (c *Client) CreateDevice(ctx context.Context, e *record.Entity) (byte, error) {
	if err := c.Writable(); err != nil {
		return 0, err
	}
	p, err := e.WritePayload()
	if err != nil {
		return 0, err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	a := &session.Ack{Family: wire.FamCreateDeviceAck}
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamCreateDevice, Payload: p}, a); err != nil {
		return 0, fmt.Errorf("hub: creating device %q: %w", e.Name, err)
	}
	if len(a.Frame.Payload) == 0 || a.Frame.Payload[0] == 0 || a.Frame.Payload[0] == record.NewID {
		return 0, fmt.Errorf("hub: creating device %q: ack without an id", e.Name)
	}
	return a.Frame.Payload[0], nil
}

// UpdateDevice rewrites a device's record (family 0x08).
func (c *Client) UpdateDevice(ctx context.Context, e *record.Entity) error {
	if err := c.Writable(); err != nil {
		return err
	}
	p, err := e.WritePayload()
	if err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamUpdateDevice, Payload: p}, session.Status()); err != nil {
		return fmt.Errorf("hub: updating device %d: %w", e.ID, err)
	}
	return nil
}

// DeleteDevice deletes a device (family 0x09) and everything that refers
// to it. The hub took 13.5 s to acknowledge one.
func (c *Client) DeleteDevice(ctx context.Context, id byte) error {
	if err := c.Writable(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DeleteTimeout)
		defer cancel()
	}
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamDeleteEntity, Payload: []byte{id}}, session.Status()); err != nil {
		return fmt.Errorf("hub: deleting device %d: %w", id, err)
	}
	return nil
}

// SetIdleBehavior writes a device's idle behavior (family 0x41), such as
// record.IdleDefault.
func (c *Client) SetIdleBehavior(ctx context.Context, device, behavior byte) error {
	if err := c.Writable(); err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamSetIdleBehavior, Payload: []byte{device, behavior}}, session.Status()); err != nil {
		return fmt.Errorf("hub: setting the idle behavior of device %d: %w", device, err)
	}
	return nil
}

// WriteMacro writes a macro of a device or activity (family 0x12), such
// as a device's POWER_ON macro, replacing the one with its id. The hub
// acks the last page with family 0x12 and the others with a status ack.
func (c *Client) WriteMacro(ctx context.Context, m *record.Macro) error {
	if err := c.Writable(); err != nil {
		return err
	}
	pages, err := m.WritePages()
	if err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	for i, p := range pages {
		ack := session.Status()
		if i == len(pages)-1 {
			ack = &session.Ack{Family: wire.FamMacroAck}
		}
		if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamWriteMacro, Payload: p}, ack); err != nil {
			return fmt.Errorf("hub: writing macro %q of entity %d, page %d of %d: %w", m.Label, m.Entity, i+1, len(pages), err)
		}
	}
	return nil
}

// WriteInputs writes a device's inputs page (family 0x46), every page
// acknowledged with a status ack. The app follows it with an UpdateDevice
// whose tail's InputMode matches, then a Sync.
func (c *Client) WriteInputs(ctx context.Context, in *record.Inputs) error {
	if err := c.Writable(); err != nil {
		return err
	}
	pages, err := in.WritePages()
	if err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	for i, p := range pages {
		if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamWriteInputs, Payload: p}, session.Status()); err != nil {
			return fmt.Errorf("hub: writing the inputs of device %d, page %d of %d: %w", in.Device, i+1, len(pages), err)
		}
	}
	return nil
}

// Sync has the hub update the remotes' copy of one device or activity
// (family 0x65 [entity, 01], the X2's per-entity sync; upstream).
func (c *Client) Sync(ctx context.Context, entity byte) error {
	if err := c.Writable(); err != nil {
		return err
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamAssignCommit, Payload: []byte{entity, 0x01}}, session.Status()); err != nil {
		return fmt.Errorf("hub: syncing entity %d to the remotes: %w", entity, err)
	}
	return nil
}
