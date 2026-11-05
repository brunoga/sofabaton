package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// Device categories and icons the app gave the IR devices it created.
// What else the remote does with them is not known.
const (
	defaultType = 2 // a TV, as the app made the Hisense
	defaultIcon = 1
)

// Create makes a new IR device on the hub from a profile, in the app's
// order: the device record, its commands, their order, its keys, its idle
// behavior and power macros, the update that commits it, then its
// inputs, the record's input mode and a sync to the remotes. done is
// called after each step. It returns the new device's id; if a step after
// the create fails, the device is left on the hub with that id for the
// caller to delete.
func Create(ctx context.Context, c *hub.Client, d *Device, done func(string)) (byte, error) {
	if err := d.Validate(); err != nil {
		return 0, err
	}
	if d.network() {
		return createNetwork(ctx, c, d, done)
	}
	for _, cmd := range d.Commands {
		if cmd.Data == "" {
			continue
		}
		if data, err := cmd.Signal(d); err != nil || !record.IsIR(data) {
			return 0, fmt.Errorf("profile: %q: a new device takes only IR commands (a code, a descriptor, or raw IR data)", cmd.Name)
		}
	}
	e, err := record.NewEntity(c.Variant())
	if err != nil {
		return 0, err
	}
	e.Name, e.Brand, e.Class, e.Type, e.Icon = d.Name, d.Brand, record.ClassIR, defaultType, defaultIcon
	if d.Icon != 0 {
		e.Icon = byte(d.Icon)
	}
	if d.Category != 0 {
		e.Type = byte(d.Category)
	}
	newTail := record.NewIRDeviceTail()
	if d.Color != 0 {
		newTail.Color = byte(d.Color)
	}
	if err := e.SetDeviceTail(newTail); err != nil {
		return 0, err
	}
	id, err := c.CreateDevice(ctx, e)
	if err != nil {
		return 0, err
	}
	report := func(s string) {
		if done != nil {
			done(s)
		}
	}
	report(fmt.Sprintf("created device %d %q", id, d.Name))

	// The commit: the record with its id, the marker and, with power
	// macros, the power fields the app sets once power is set up.
	committed := *e
	committed.ID = id
	committed.Tail = append([]byte(nil), e.Tail...)
	tail, err := committed.DeviceTail()
	if err != nil {
		return id, err
	}
	tail.Marker = 1
	if d.Power != nil {
		tail.PowerMode, tail.PowerStyle = powerMode, powerStyle
	}
	if err := committed.SetDeviceTail(tail); err != nil {
		return id, err
	}

	changes, _, err := Plan(d, &State{Variant: c.Variant(), Device: id, Data: map[byte][]byte{}, Entity: &committed}, false)
	if err != nil {
		return id, err
	}
	var before, seqs, after []Change
	for _, ch := range changes {
		switch ch.Kind {
		case PowerMacro:
			seqs = append(seqs, ch)
		case Inputs, Record, Sync:
			after = append(after, ch)
		default:
			before = append(before, ch)
		}
	}
	apply := func(chs []Change) error {
		return Apply(ctx, c, chs, func(ch Change) { report(ch.String()) })
	}
	if err := apply(before); err != nil {
		return id, err
	}
	if err := c.SetIdleBehavior(ctx, id, record.IdleDefault); err != nil {
		return id, err
	}
	if err := apply(seqs); err != nil {
		return id, err
	}
	if err := c.UpdateDevice(ctx, &committed); err != nil {
		return id, err
	}
	report(fmt.Sprintf("committed device %d", id))
	return id, apply(after)
}

// What upstream's create gives a generic network device: the app's
// Wi-Fi category, and in the last update the record's sort and channel
// bytes as below (what they mean is not known).
const (
	networkType    = 16
	networkChannel = 0x4D
)

// createNetwork makes a generic network device (record.ClassIP, a device
// of HTTP commands) in the order upstream's create does on the X1S and
// X2, which our X2 took (2026-11-04): the record, its commands, idle
// behavior 4, an empty inputs page, the update that commits it and a
// sync; with power macros, the power fields, idle behavior 1 and the
// macros; the keys; and a last update with a compact tail
// (record.NewIPDeviceTail), without which, upstream found, the hub does
// not take the device as configured.
func createNetwork(ctx context.Context, c *hub.Client, d *Device, done func(string)) (byte, error) {
	if c.Variant() == wire.X1 {
		return 0, errors.New("profile: the X1 has no devices of HTTP commands (its network devices are Roku ones)")
	}
	for _, cmd := range d.Commands {
		if cmd.HTTP == nil {
			return 0, fmt.Errorf("profile: %q: a new network device takes only HTTP commands", cmd.Name)
		}
	}
	report := func(s string) {
		if done != nil {
			done(s)
		}
	}
	e, err := record.NewEntity(c.Variant())
	if err != nil {
		return 0, err
	}
	e.Name, e.Brand, e.Class, e.Type, e.Icon = d.Name, d.Brand, record.ClassIP, networkType, defaultIcon
	if d.Icon != 0 {
		e.Icon = byte(d.Icon)
	}
	if d.Category != 0 {
		e.Type = byte(d.Category)
	}
	tail := record.DeviceTail{HasPollTime: true, InputMode: record.InputsNoSwitch}
	if err := e.SetDeviceTail(tail); err != nil {
		return 0, err
	}
	id, err := c.CreateDevice(ctx, e)
	if err != nil {
		return 0, err
	}
	report(fmt.Sprintf("created device %d %q", id, d.Name))

	committed := *e
	committed.ID = id
	committed.Tail = append([]byte(nil), e.Tail...)
	tail.Marker = 1
	if err := committed.SetDeviceTail(tail); err != nil {
		return id, err
	}
	changes, _, err := Plan(d, &State{Variant: c.Variant(), Device: id, Data: map[byte][]byte{}, Entity: &committed}, false)
	if err != nil {
		return id, err
	}
	var commands, keys, seqs []Change
	for _, ch := range changes {
		switch ch.Kind {
		case PowerMacro:
			seqs = append(seqs, ch)
		case Bind:
			keys = append(keys, ch)
		case Inputs, Record, Sync:
			// Written below, as upstream's create does.
		default:
			commands = append(commands, ch)
		}
	}
	apply := func(chs []Change) error {
		return Apply(ctx, c, chs, func(ch Change) { report(ch.String()) })
	}
	if err := apply(commands); err != nil {
		return id, err
	}
	if err := c.SetIdleBehavior(ctx, id, 4); err != nil {
		return id, err
	}
	in, err := record.NewInputs(c.Variant(), id, 0)
	if err != nil {
		return id, err
	}
	if err := c.WriteInputs(ctx, in); err != nil {
		return id, err
	}
	if err := c.UpdateDevice(ctx, &committed); err != nil {
		return id, err
	}
	if err := c.Sync(ctx, id); err != nil {
		return id, err
	}
	report(fmt.Sprintf("committed device %d", id))
	if d.Power != nil {
		// Upstream sends the record again, its power fields still zero.
		if err := c.UpdateDevice(ctx, &committed); err != nil {
			return id, err
		}
		if err := c.SetIdleBehavior(ctx, id, record.IdleDefault); err != nil {
			return id, err
		}
		if err := apply(seqs); err != nil {
			return id, err
		}
	}
	if err := apply(keys); err != nil {
		return id, err
	}
	last := committed
	last.Tail = append([]byte(nil), committed.Tail...)
	last.Sort, last.Channel = id, networkChannel
	if err := last.SetDeviceTail(record.NewIPDeviceTail()); err != nil {
		return id, err
	}
	if err := c.UpdateDevice(ctx, &last); err != nil {
		return id, err
	}
	report(fmt.Sprintf("configured device %d", id))
	return id, nil
}
