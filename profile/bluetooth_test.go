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

// appCreate is the frames the app sent our X2 to create the AndroidBox,
// from its create to its pairing request.
func appCreate(t *testing.T) []wire.Frame {
	t.Helper()
	f, err := os.Open("../testdata/x2/app-add-bluetooth.jsonl")
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
		if fr.Family == wire.FamPair {
			return out
		}
	}
	t.Fatal("no Bluetooth create in the capture")
	return nil
}

// A Bluetooth device is created as the app created the AndroidBox on our
// X2, from its profile as exported after: the same record, its commands'
// names and HID usages in the same order, the same keys, power macros and
// inputs page, and the pairing request; it reads back as its profile.
func TestCreateBluetooth(t *testing.T) {
	ctx := context.Background()
	app := appCreate(t)
	var sent []wire.Frame
	tap := session.WithTap(func(d wire.Direction, f wire.Frame) {
		if d == wire.ToHub {
			sent = append(sent, f)
		}
	})
	m := &hubsim.Model{Info: &record.HubInfo{Model: 0x03, Firmware: 9, Name: "X2 HUB"}}
	for i := 1; i <= 2; i++ { // the hub's other devices, so the id is the app's
		e, _ := record.NewEntity(wire.X2)
		e.ID, e.Name, e.Class = byte(i), "Other"+string(rune('0'+i)), record.ClassIR
		m.Devices = append(m.Devices, e)
	}
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{Session: []session.Option{tap}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	y, err := os.ReadFile("../testdata/x2/bluetooth-androidbox.yaml")
	if err != nil {
		t.Fatal(err)
	}
	d, err := profile.Load(bytes.NewReader(y))
	if err != nil {
		t.Fatal(err)
	}
	d.ID = 0
	sent = nil
	id, err := profile.Create(ctx, c, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != 3 {
		t.Fatalf("id %d", id)
	}
	if err := c.Pair(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Paired, []byte{3}) {
		t.Errorf("paired %v", m.Paired)
	}

	// The app's commands by their code.
	names := map[string]string{}
	for _, f := range app {
		if f.Family == wire.FamWriteCommand {
			_, c, _, err := record.ParseCommandWrite(wire.X2, f.Payload[3:])
			if err != nil {
				t.Fatal(err)
			}
			names[string(c.Code[:])] = c.Label
		}
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
	keys := func(fs []wire.Frame) []record.Binding {
		var out []record.Binding
		for _, f := range fs {
			if f.Family == wire.FamWriteButton {
				b, err := record.ParseBinding(f.Payload[6 : 6+record.BindingLen])
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, b)
			}
		}
		slices.SortFunc(out, func(a, b record.Binding) int { return int(a.Button) - int(b.Button) })
		return out
	}
	// The keys, in another order: the app's names commands by their code,
	// ours by their id.
	gotKeys, appKeys := keys(sent), keys(app)
	for i := range gotKeys {
		g, a := gotKeys[i], appKeys[i]
		if g.Button != a.Button || d.Commands[g.Short.Command-1].Name != names[string(a.Short.Code[:])] {
			t.Errorf("key %v runs %d, the app's %v % x", g.Button, g.Short.Command, a.Button, a.Short.Code)
		}
	}
	for i, f := range sent {
		a := app[i]
		switch f.Family {
		case wire.FamWriteCommand:
			// The app sends id 0 and the catalog's function id as the
			// code, all in one burst, and the hub numbers the commands;
			// we number them and write each as a burst of one, which
			// paired and worked on our X2 as well (2026-11-06).
			_, got, gdata, err1 := record.ParseCommandWrite(wire.X2, f.Payload[3:])
			_, want, wdata, err2 := record.ParseCommandWrite(wire.X2, a.Payload[3:])
			if err1 != nil || err2 != nil || got.Label != want.Label || got.Type != want.Type || got.Device != want.Device || !bytes.Equal(gdata, wdata) {
				t.Errorf("command %d: %+v % x, the app's %+v % x", i, got, gdata, want, wdata)
			}
		case wire.FamWriteButton:
		case wire.FamWriteMacro:
			got, err1 := record.ParseMacro(wire.X2, f.Payload[3:])
			want, err2 := record.ParseMacro(wire.X2, a.Payload[3:])
			if err1 != nil || err2 != nil || got.ID != want.ID || got.Label != want.Label || len(got.Steps) != 1 || len(want.Steps) != 1 ||
				d.Commands[got.Steps[0].Command-1].Name != names[string(want.Steps[0].Code[:])] {
				t.Errorf("macro %d: %+v, the app's %+v", i, got, want)
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
	var out strings.Builder
	if err := back.Encode(&out); err != nil {
		t.Fatal(err)
	}
	if want := string(y[strings.Index(string(y), "device:"):]); out.String() != want {
		t.Fatalf("read back:\n%s\nwant:\n%s", out.String(), want)
	}
}

// ApplyConfig creates a Bluetooth device only with a way to pair it,
// which the hub needs to keep it.
func TestApplyConfigBluetooth(t *testing.T) {
	ctx := context.Background()
	c := emptyHub(t)
	d, err := profile.Load(strings.NewReader("device: ZZ Box\nbluetooth: {}\ncommands:\n  - {name: Play, data: 0c00b0}\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &profile.Config{Devices: []*profile.Device{d}}
	var log []string
	opts := profile.ApplyOptions{Report: func(s string) { log = append(log, s) }}
	if err := profile.ApplyConfig(ctx, c, cfg, opts); err != nil {
		t.Fatal(err)
	}
	if devs, _ := c.Devices(ctx); len(devs) != 0 || !strings.Contains(strings.Join(log, "\n"), "skipped: a Bluetooth device") {
		t.Fatalf("without pairing: %d devices\n%s", len(devs), strings.Join(log, "\n"))
	}
	var paired []string
	opts.Pair = func(ctx context.Context, c *hub.Client, id byte, name string) error {
		paired = append(paired, name)
		return c.Pair(ctx, id)
	}
	log = nil
	if err := profile.ApplyConfig(ctx, c, cfg, opts); err != nil {
		t.Fatal(err)
	}
	if devs, _ := c.Devices(ctx); len(devs) != 1 || !slices.Equal(paired, []string{"ZZ Box"}) {
		t.Fatalf("with pairing: %d devices, paired %v\n%s", len(devs), paired, strings.Join(log, "\n"))
	}
}
