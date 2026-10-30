package hub_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

func model(t *testing.T) *hubsim.Model {
	t.Helper()
	info := &record.HubInfo{ID: [6]byte{0xfc, 0x01, 0x2c, 0x39, 0xd3, 0x90}, Marker: 0x64, Model: 0x03,
		Batch: [4]byte{0x20, 0x22, 0x11, 0x20}, Firmware: 8, Flags: [2]byte{1, 0}, Name: "X2 HUB"}
	dev := func(id byte, name string) *record.Entity {
		e, err := record.NewEntity(wire.X2)
		if err != nil {
			t.Fatal(err)
		}
		e.ID, e.Name, e.Brand, e.Class, e.Type = id, name, "Brand", record.ClassIR, 2
		if err := e.SetDeviceTail(record.DeviceTail{HasPollTime: true, InputMode: 1}); err != nil {
			t.Fatal(err)
		}
		return e
	}
	act, _ := record.NewEntity(wire.X2)
	act.ID, act.Name = 0x65, "Watch TV"
	tv, avr := dev(1, "TV"), dev(2, "AVR")

	var cmds []*record.Command
	for i := range 7 { // three pages of commands
		c, err := record.ParseCommand(wire.X2, make([]byte, 70))
		if err != nil {
			t.Fatal(err)
		}
		c.Device, c.ID, c.Type, c.Label, c.Sort = 1, byte(i+1), record.ClassIR, "Command", byte(i+1)
		cmds = append(cmds, c)
	}
	var keys []record.Binding
	for i := range 20 {
		keys = append(keys, record.Binding{Entity: 0x65, Button: record.Button(0xAE + i), Short: record.Press{Device: 1, Command: byte(i + 1)}})
	}
	mac, _ := record.NewMacro(wire.X2, 0x65, record.MacroPowerOn, "POWER_ON")
	mac.Steps = []record.Step{{Device: 1, Command: 1}, {Device: 2, Command: 1, Delay: 5}}
	in, _ := record.NewInputs(wire.X2, 2, record.InputsDirect)
	in.Entries = []record.InputEntry{{Command: 9, Ordinal: 1, Label: "HDMI 1"}}

	return &hubsim.Model{
		Info:       info,
		Devices:    []*record.Entity{tv, avr},
		Activities: []*record.Entity{act},
		Commands:   map[byte][]*record.Command{1: cmds},
		Keymaps:    map[byte][]record.Binding{0x65: keys},
		Macros:     map[byte][]*record.Macro{0x65: {mac}},
		Inputs:     map[byte]*record.Inputs{2: in},
		Members:    map[byte][]byte{0x65: {1, 2}},
	}
}

func TestClientReads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(model(t))), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if c.Variant() != wire.X2 || c.Info().Name != "X2 HUB" {
		t.Fatalf("info %+v", c.Info())
	}
	devs, err := c.Devices(ctx)
	if err != nil || len(devs) != 2 || devs[1].Name != "AVR" {
		t.Fatalf("devices %v %v", devs, err)
	}
	acts, err := c.Activities(ctx)
	if err != nil || len(acts) != 1 || acts[0].Name != "Watch TV" {
		t.Fatalf("activities %v %v", acts, err)
	}
	members, err := c.Members(ctx, 0x65)
	if err != nil || len(members) != 2 || members[0].Name != "TV" {
		t.Fatalf("members %v %v", members, err)
	}
	cmds, err := c.Commands(ctx, 1)
	if err != nil || len(cmds) != 7 || cmds[6].ID != 7 {
		t.Fatalf("commands %d %v", len(cmds), err)
	}
	keys, err := c.Keymap(ctx, 0x65)
	if err != nil || len(keys) != 20 || keys[19].Short.Command != 20 {
		t.Fatalf("keymap %d %v", len(keys), err)
	}
	macros, err := c.Macros(ctx, 0x65)
	if err != nil || len(macros) != 1 || macros[0].Label != "POWER_ON" || len(macros[0].Steps) != 2 {
		t.Fatalf("macros %v %v", macros, err)
	}
	in, err := c.Inputs(ctx, 2)
	if err != nil || len(in.Entries) != 1 || in.Entries[0].Label != "HDMI 1" {
		t.Fatalf("inputs %v %v", in, err)
	}
	if _, err := c.Inputs(ctx, 1); !errors.Is(err, hub.ErrNotFound) {
		t.Fatalf("inputs of a device without a page: %v", err)
	}
}

func TestOpenUnknownModel(t *testing.T) {
	m := model(t)
	m.Info.Model = 0x09
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{}); !errors.Is(err, wire.ErrUnknownVariant) {
		t.Fatalf("Open: %v", err)
	}
}

func TestDefaultTimeout(t *testing.T) {
	h := hubsim.FromModel(model(t))
	h.Handle(wire.FamReqDevices, func(wire.Frame) []wire.Frame { return nil }) // never answers
	c, err := hub.Open(context.Background(), hubsim.Pipe(h), hub.Options{Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Devices(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Devices: %v", err)
	}
}
