package profile

import (
	"context"
	"fmt"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
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
	for _, cmd := range d.Commands {
		if cmd.Data != "" {
			return 0, fmt.Errorf("profile: %q: a new device takes only commands with a code or a descriptor", cmd.Name)
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
