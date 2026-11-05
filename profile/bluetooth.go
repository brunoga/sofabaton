package profile

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// Bluetooth is what makes a device a Bluetooth one, which the hub sends
// HID usages to as a keyboard and remote would.
type Bluetooth struct {
	// Profile is the id of the Bluetooth profile of Sofabaton's catalog
	// the device was made from (its code_id, 32 hex digits), which the
	// app keeps in the device's record; empty for none.
	Profile string `yaml:"profile,omitempty"`
	// Classic is set for a device that pairs over classic Bluetooth
	// rather than BLE (the catalog's mode 0, as older Apple TVs and the
	// PS4).
	Classic bool `yaml:"classic,omitempty"`
}

func (b *Bluetooth) check() error {
	if b.Profile == "" {
		return nil
	}
	if id, err := hex.DecodeString(b.Profile); err != nil || len(id) != record.CodeIDLen {
		return fmt.Errorf("profile: bluetooth: profile %q is not a catalog profile's id (32 hex digits)", b.Profile)
	}
	return nil
}

// The app's category for Bluetooth devices.
const bluetoothType = 15

// createBluetooth makes a Bluetooth device (record.ClassBluetooth) as the
// app did on our X2 (2026-11-05): the record, its commands, its keys,
// idle behavior 1 and the power macros, and an empty inputs page. The
// app leaves the record uncommitted (Marker 0) and sends no sync; it
// then has the hub pair (hub.Client.Pair), which the caller does. Our X2
// took every write and listed no such device after them (2026-11-05):
// it keeps a new Bluetooth device only once it has paired.
func createBluetooth(ctx context.Context, c *hub.Client, d *Device, done func(string)) (byte, error) {
	if c.Variant() != wire.X2 {
		return 0, errors.New("profile: Bluetooth devices are made on the X2 only")
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
	e.Name, e.Brand, e.Class, e.Type, e.Icon = d.Name, d.Brand, record.ClassBluetooth, bluetoothType, defaultIcon
	if d.Icon != 0 {
		e.Icon = byte(d.Icon)
	}
	if d.Category != 0 {
		e.Type = byte(d.Category)
	}
	if p := d.Bluetooth.Profile; p != "" {
		id, _ := hex.DecodeString(p) // validated
		copy(e.CodeID[:], id)
	}
	tail := record.NewBluetoothDeviceTail(d.Bluetooth.Classic)
	if d.Color != 0 {
		tail.Color = byte(d.Color)
	}
	if err := e.SetDeviceTail(tail); err != nil {
		return 0, err
	}
	id, err := c.CreateDevice(ctx, e)
	if err != nil {
		return 0, err
	}
	report(fmt.Sprintf("created device %d %q", id, d.Name))

	created := *e
	created.ID = id
	changes, _, err := Plan(d, &State{Variant: c.Variant(), Device: id, Data: map[byte][]byte{}, Entity: &created}, false)
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
			// The record stays as created, and the inputs page is
			// written below.
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
	if err := apply(keys); err != nil {
		return id, err
	}
	if err := c.SetIdleBehavior(ctx, id, record.IdleDefault); err != nil {
		return id, err
	}
	if err := apply(seqs); err != nil {
		return id, err
	}
	in, err := record.NewInputs(c.Variant(), id, 0)
	if err != nil {
		return id, err
	}
	if err := c.WriteInputs(ctx, in); err != nil {
		return id, err
	}
	report(fmt.Sprintf("wrote device %d; it pairs next", id))
	return id, nil
}
