package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

func simulated(t *testing.T) {
	t.Helper()
	tv, _ := record.NewEntity(wire.X2)
	tv.ID, tv.Name, tv.Class = 1, "TV", record.ClassIR
	_ = tv.SetDeviceTail(record.DeviceTail{InputMode: 1})
	act, _ := record.NewEntity(wire.X2)
	act.ID, act.Name = 0x65, "Watch TV"
	cmd, _ := record.ParseCommand(wire.X2, make([]byte, 70))
	cmd.Device, cmd.ID, cmd.Label = 1, 1, "Power"
	m := &hubsim.Model{
		Info:       &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 8, Name: "X2 HUB"},
		Devices:    []*record.Entity{tv},
		Activities: []*record.Entity{act},
		Commands:   map[byte][]*record.Command{1: {cmd}},
		Keymaps:    map[byte][]record.Binding{0x65: {{Entity: 0x65, Button: record.ButtonOK, Short: record.Press{Device: 1, Command: 1}}}},
		Members:    map[byte][]byte{0x65: {1}},
	}
	prev := dialHub
	dialHub = func(ctx context.Context, _ netip.Addr, opts hub.Options) (*hub.Client, error) {
		return hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), opts)
	}
	t.Cleanup(func() { dialHub = prev })
}

func TestInfo(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	if err := run(context.Background(), []string{"info", "-hub", "192.0.2.1"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "X2 HUB") || !strings.Contains(out.String(), "model     X2") {
		t.Fatalf("info:\n%s", out.String())
	}
}

func TestDump(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	if err := run(context.Background(), []string{"dump", "-hub", "192.0.2.1", "-v"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	var d dumpFile
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("dump is not JSON: %v\n%s", err, out.String())
	}
	if d.Info.Model != "X2" || len(d.Devices.Raw) != 1 || len(d.Activities.Raw) != 1 || len(d.Entities) != 2 {
		t.Fatalf("dump %+v", d)
	}
	tv := d.Entities[0]
	if tv.Commands == nil || len(tv.Commands.Raw) != 1 || tv.Commands.Error != "" {
		t.Fatalf("TV commands %+v", tv.Commands)
	}
	// The simulated hub holds no inputs page for the TV: the failure is
	// recorded and the dump goes on.
	if tv.Inputs == nil || tv.Inputs.Error == "" {
		t.Fatalf("TV inputs %+v", tv.Inputs)
	}
	if act := d.Entities[1]; act.Members == nil || len(act.Members.Raw) != 1 || act.Keymap.Error != "" {
		t.Fatalf("activity %+v", act)
	}
	if !strings.Contains(errs.String(), "A→H 0x0001 ReqBanner") {
		t.Fatalf("-v did not log frames:\n%s", errs.String())
	}
}

func TestUsage(t *testing.T) {
	var out, errs bytes.Buffer
	if err := run(context.Background(), nil, &out, &errs); err == nil {
		t.Fatal("no command accepted")
	}
	if err := run(context.Background(), []string{"bogus"}, &out, &errs); err == nil {
		t.Fatal("unknown command accepted")
	}
	if err := run(context.Background(), []string{"info"}, &out, &errs); err == nil || !strings.Contains(err.Error(), "-hub") {
		t.Fatalf("info without -hub: %v", err)
	}
	out.Reset()
	if err := run(context.Background(), []string{"version"}, &out, &errs); err != nil || strings.TrimSpace(out.String()) != "dev" {
		t.Fatalf("version: %q %v", out.String(), err)
	}
}
