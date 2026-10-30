package hub

import (
	"context"
	"fmt"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// Requests of the readers.
var (
	reqDevices    = wire.Frame{Family: wire.FamReqDevices}
	reqActivities = wire.Frame{Family: wire.FamReqActivities}
)

func reqCommands(dev byte) wire.Frame {
	return wire.Frame{Family: wire.FamReqCommands, Payload: []byte{dev, 0xFF}}
}

func reqKeymap(entity byte) wire.Frame {
	return wire.Frame{Family: wire.FamReqButtons, Payload: []byte{entity, 0xFF}}
}

func reqMacros(entity byte) wire.Frame {
	return wire.Frame{Family: wire.FamReqMacros, Payload: []byte{entity, 0xFF}}
}

func reqMembers(activity byte) wire.Frame {
	return wire.Frame{Family: wire.FamReqActivityMap, Payload: []byte{activity}}
}

func reqInputs(dev byte) wire.Frame {
	return wire.Frame{Family: wire.FamReqActivityInputs, Payload: []byte{dev}}
}

// Devices reads the device catalog.
func (c *Client) Devices(ctx context.Context) ([]*record.Entity, error) {
	return c.entities(ctx, reqDevices, wire.FamDeviceRow)
}

// Activities reads the activity catalog.
func (c *Client) Activities(ctx context.Context) ([]*record.Entity, error) {
	return c.entities(ctx, reqActivities, wire.FamActivityRow)
}

// Members reads the devices that belong to an activity.
func (c *Client) Members(ctx context.Context, activity byte) ([]*record.Entity, error) {
	return c.entities(ctx, reqMembers(activity), wire.FamActivityMapPage)
}

func (c *Client) entities(ctx context.Context, req wire.Frame, fam wire.Family) ([]*record.Entity, error) {
	bodies, err := c.Records(ctx, req, fam)
	if err != nil {
		return nil, err
	}
	out := make([]*record.Entity, 0, len(bodies))
	for i, body := range bodies {
		e, err := record.ParseEntity(c.variant, body)
		if err != nil {
			return nil, fmt.Errorf("hub: %v row %d: %w", fam.Name(wire.FromHub), i+1, err)
		}
		out = append(out, e)
	}
	return out, nil
}

// Commands reads a device's command list.
func (c *Client) Commands(ctx context.Context, device byte) ([]*record.Command, error) {
	body, err := c.single(ctx, reqCommands(device), wire.FamCommandPage)
	if err != nil {
		return nil, err
	}
	return record.ParseCommandList(c.variant, body)
}

// Keymap reads the key bindings and favorites of an activity or device.
func (c *Client) Keymap(ctx context.Context, entity byte) ([]record.Binding, error) {
	body, err := c.single(ctx, reqKeymap(entity), wire.FamKeymapPage)
	if err != nil {
		return nil, err
	}
	return record.ParseKeymap(body)
}

// Macros reads the macros of an activity or device, power sequences
// included.
func (c *Client) Macros(ctx context.Context, entity byte) ([]*record.Macro, error) {
	bodies, err := c.Records(ctx, reqMacros(entity), wire.FamMacroPage)
	if err != nil {
		return nil, err
	}
	out := make([]*record.Macro, 0, len(bodies))
	for i, body := range bodies {
		m, err := record.ParseMacro(c.variant, body)
		if err != nil {
			return nil, fmt.Errorf("hub: macro %d: %w", i+1, err)
		}
		out = append(out, m)
	}
	return out, nil
}

// Inputs reads a device's inputs page. A device without one is
// ErrNotFound.
func (c *Client) Inputs(ctx context.Context, device byte) (*record.Inputs, error) {
	body, err := c.single(ctx, reqInputs(device), wire.FamActivityInputs)
	if err != nil {
		return nil, notFound(err)
	}
	return record.ParseInputs(c.variant, body)
}

func (c *Client) single(ctx context.Context, req wire.Frame, fam wire.Family) ([]byte, error) {
	bodies, err := c.Records(ctx, req, fam)
	if err != nil {
		return nil, err
	}
	if len(bodies) != 1 {
		return nil, fmt.Errorf("hub: %v reply has %d records, want 1", fam.Name(wire.FromHub), len(bodies))
	}
	return bodies[0], nil
}
