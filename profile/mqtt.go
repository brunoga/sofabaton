package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// MQTTData is the data of an MQTT command (record.ClassMQTT): its device's
// id and its own, which the hub publishes when it runs, as
// {"device_id":D,"key_id":K} to the topic of the hub's MAC and "/up"
// (the app's code; our X2, 2026-11-12).
func MQTTData(device, id byte) []byte { return []byte{device, id} }

// createMQTT makes a device of MQTT commands as the X2 app makes its "HA
// remote" (its code, 2026-11-12): the record, of class MQTT, then each
// command, numbered from 1, then the update that commits it. Keys and
// power macros, which the app's has none of, follow as an apply writes
// them. The hub must have a broker (hub.Client.SetMQTT).
func createMQTT(ctx context.Context, c *hub.Client, d *Device, done func(string)) (byte, error) {
	if c.Variant() != wire.X2 {
		return 0, errors.New("profile: MQTT devices are made on the X2 only")
	}
	ok, err := c.MQTTConfigured(ctx)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.New("profile: the hub has no MQTT broker: set one first (sbx mqtt -broker)")
	}
	if len(d.Commands) > 0xFE {
		return 0, fmt.Errorf("profile: %d commands, more than a device holds", len(d.Commands))
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
	e.Name, e.Brand, e.Class, e.Type, e.Icon = d.Name, d.Brand, record.ClassMQTT, hueType, defaultIcon
	if d.Icon != 0 {
		e.Icon = byte(d.Icon)
	}
	if d.Category != 0 {
		e.Type = byte(d.Category)
	}
	tail := record.NewMQTTDeviceTail()
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
	for i, pc := range d.Commands {
		cmd, err := record.NewCommand(c.Variant())
		if err != nil {
			return id, err
		}
		cmd.Device, cmd.ID, cmd.Type, cmd.Label = id, byte(i+1), record.ClassMQTT, pc.Name
		if err := c.WriteCommand(ctx, cmd, MQTTData(id, cmd.ID)); err != nil {
			return id, err
		}
		report(fmt.Sprintf("add     %3d %q", cmd.ID, pc.Name))
	}
	committed := *e
	committed.ID = id
	committed.Tail = append([]byte(nil), e.Tail...)
	tail.Marker = 1
	if err := committed.SetDeviceTail(tail); err != nil {
		return id, err
	}
	if err := c.UpdateDevice(ctx, &committed); err != nil {
		return id, err
	}
	report(fmt.Sprintf("committed device %d", id))
	if d.Power == nil && len(d.Buttons) == 0 {
		return id, nil
	}
	st, err := Read(ctx, c, id)
	if err != nil {
		return id, err
	}
	changes, _, err := Plan(d, st, false)
	if err != nil {
		return id, err
	}
	return id, Apply(ctx, c, changes, func(ch Change) { report(ch.String()) })
}
