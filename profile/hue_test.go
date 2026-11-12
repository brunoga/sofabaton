package profile_test

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/capture"
	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// The bridge's user in testdata/x2/app-add-hue.jsonl, in place of ours.
const hueUser = "0123456789abcdefghijABCDEFGHIJ0123456789"

// appHue is the frames the app sent our X2 to add a Hue bridge, from its
// create to the update that commits it.
func appHue(t *testing.T) []wire.Frame {
	t.Helper()
	f, err := os.Open("../testdata/x2/app-add-hue.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := capture.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	var out []wire.Frame
	for _, e := range entries {
		if e.IsNote() || e.Dir != wire.ToHub {
			continue
		}
		fr, err := e.Frame()
		if err != nil {
			t.Fatal(err)
		}
		if fr.Family == wire.FamCreateDevice || len(out) > 0 {
			out = append(out, fr)
		}
		if fr.Family == wire.FamUpdateDevice {
			return out
		}
	}
	t.Fatal("no Hue bridge create in the capture")
	return nil
}

// A Hue bridge is created as the app created ours on our X2, from its
// profile: the same record, the commands' names and requests in the same
// order, idle behavior, inputs page and commit; it reads back as its
// profile.
func TestCreateHue(t *testing.T) {
	ctx := context.Background()
	app := appHue(t)

	// The profile of what the app wrote.
	created, err := record.ParseEntity(wire.X2, app[0].Payload[3:])
	if err != nil {
		t.Fatal(err)
	}
	d := &profile.Device{Name: created.Name, Brand: created.Brand, Hue: &profile.Hue{Bridge: "192.168.1.183", User: hueUser}}
	for _, f := range app {
		if f.Family != wire.FamWriteCommand {
			continue
		}
		_, c, data, err := record.ParseCommandWrite(wire.X2, f.Payload[3:])
		if err != nil {
			t.Fatal(err)
		}
		r, err := record.ParseHueRequest(data)
		if err != nil {
			t.Fatal(err)
		}
		path, ok := strings.CutPrefix(r.Path, "api/"+hueUser+"/")
		if !ok {
			t.Fatalf("%q: path %q", c.Label, r.Path)
		}
		d.Commands = append(d.Commands, profile.Command{Name: c.Label, Hue: &profile.HueCommand{Path: path, Body: r.Body, Length: r.Length}})
	}

	var sent []wire.Frame
	tap := session.WithTap(func(dir wire.Direction, f wire.Frame) {
		if dir == wire.ToHub {
			sent = append(sent, f)
		}
	})
	m := &hubsim.Model{Info: &record.HubInfo{Model: 0x03, Firmware: 9, Name: "X2 HUB"}}
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{Session: []session.Option{tap}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	sent = nil
	id, err := profile.Create(ctx, c, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("id %d", id)
	}
	families := func(fs []wire.Frame) []wire.Family {
		var out []wire.Family
		for _, f := range fs {
			out = append(out, f.Family)
		}
		return out
	}
	if !slices.Equal(families(sent), families(app)) {
		t.Fatalf("sent %v\nthe app %v", families(sent), families(app))
	}
	for i, f := range sent {
		a := app[i]
		switch f.Family {
		case wire.FamWriteCommand:
			// The app sends id 0 and its own codes in one burst, and the
			// hub numbers the commands; we number them, each a burst of
			// one, as for the network and Bluetooth devices the hub took.
			_, got, gdata, err1 := record.ParseCommandWrite(wire.X2, f.Payload[3:])
			_, want, wdata, err2 := record.ParseCommandWrite(wire.X2, a.Payload[3:])
			if err1 != nil || err2 != nil || got.Label != want.Label || got.Type != want.Type || got.Device != want.Device || !bytes.Equal(gdata, wdata) {
				t.Errorf("command %d: %+v %q, the app's %+v %q", i, got, gdata, want, wdata)
			}
		default:
			if !bytes.Equal(f.Payload, a.Payload) {
				t.Errorf("%v: % x\nthe app's % x", f.Family, f.Payload, a.Payload)
			}
		}
	}

	st, err := profile.Read(ctx, c, id)
	if err != nil {
		t.Fatal(err)
	}
	if changes, _, err := profile.Plan(d, st, true); err != nil || len(changes) != 0 {
		t.Fatalf("after the create: %v %v", changes, err)
	}
	back, err := profile.FromHub(st.Entity, st)
	if err != nil {
		t.Fatal(err)
	}
	if back.Hue == nil || *back.Hue != *d.Hue || len(back.Commands) != len(d.Commands) || *back.Commands[0].Hue != *d.Commands[0].Hue {
		t.Fatalf("read back %+v, %+v", back.Hue, back.Commands[0])
	}
	var y strings.Builder
	if err := back.Encode(&y); err != nil {
		t.Fatal(err)
	}
	if again, err := profile.Load(strings.NewReader(y.String())); err != nil || again.Commands[0].Hue.Path != "groups/3/action" {
		t.Fatalf("%v\n%s", err, y.String())
	}
}

func TestHueValidate(t *testing.T) {
	for _, bad := range []string{
		"device: X\nhue: {bridge: lights.local, user: k}\ncommands:\n  - {name: A, hue: {path: lights/1/state, body: '{}'}}\n",
		"device: X\nhue: {bridge: 10.0.0.2, user: ''}\ncommands:\n  - {name: A, hue: {path: lights/1/state, body: '{}'}}\n",
		"device: X\ncommands:\n  - {name: A, hue: {path: lights/1/state, body: '{}'}}\n",
		"device: X\nhue: {bridge: 10.0.0.2, user: k}\ncommands:\n  - {name: A, descriptor: 'P:NEC R:38400 D:0 S:206 F:1'}\n",
	} {
		if _, err := profile.Load(strings.NewReader(bad)); err == nil {
			t.Errorf("taken:\n%s", bad)
		}
	}
}
