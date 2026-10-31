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
