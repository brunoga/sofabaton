package profile_test

import (
	"context"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// A device the hub says has no commands has none when it has no keys,
// and is read again when it has: the hub answered wrongly.
func TestReadEmptyDevice(t *testing.T) {
	ctx := context.Background()
	dev := func(id byte, name string) *record.Entity {
		e, err := record.NewEntity(wire.X2)
		if err != nil {
			t.Fatal(err)
		}
		e.ID, e.Name, e.Class = id, name, record.ClassIR
		if err := e.SetDeviceTail(record.DeviceTail{InputMode: 1}); err != nil {
			t.Fatal(err)
		}
		return e
	}
	m := &hubsim.Model{
		Info:    &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 8, Name: "X2 HUB"},
		Devices: []*record.Entity{dev(1, "Empty"), dev(2, "Keyed")},
		// Keyed's commands, but the hub does not answer them.
		Keymaps: map[byte][]record.Binding{2: {{Entity: 2, Button: record.ButtonOK, Short: record.Press{Device: 2, Command: 1}}}},
	}
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	st, err := profile.Read(ctx, c, 1)
	if err != nil || len(st.Commands) != 0 {
		t.Fatalf("an empty device: %v, %d commands", err, len(st.Commands))
	}
	if _, err := profile.Read(ctx, c, 2); err == nil {
		t.Error("a device with keys and no commands read as empty")
	}
}
