package profile_test

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/govee"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/roku"
	"github.com/brunoga/sofabaton/sonos"
)

// Roku, Sonos and cloud (https) devices are created, read back as their
// profiles, and restored onto another hub.
func TestCreateWiFi(t *testing.T) {
	ctx := context.Background()
	devices := []*profile.Device{
		roku.Device("Den Roku", netip.MustParseAddr("192.168.1.50"), []roku.App{{ID: "12", Name: "Netflix"}}),
		sonos.Device("Kitchen", netip.MustParseAddr("192.168.1.60"), 30),
		govee.Device("Govee", "KEY", []govee.Light{{Device: "AA:BB", Model: "H6159", Name: "Lamp"}}, govee.Options{OnOff: true, Brightness: govee.Brightness}),
	}
	c := emptyHub(t)
	for _, d := range devices {
		id, err := profile.Create(ctx, c, d, nil)
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		st, err := profile.Read(ctx, c, id)
		if err != nil {
			t.Fatal(err)
		}
		if changes, _, err := profile.Plan(d, st, true); err != nil || len(changes) != 0 {
			t.Errorf("%s: after the create: %v %v", d.Name, changes, err)
		}
		back, err := profile.FromHub(st.Entity, st)
		if err != nil {
			t.Fatal(err)
		}
		var y strings.Builder
		if err := back.Encode(&y); err != nil {
			t.Fatal(err)
		}
		if _, err := profile.Load(strings.NewReader(y.String())); err != nil {
			t.Fatalf("%s: %v\n%s", d.Name, err, y.String())
		}
		t.Logf("%s:\n%s", d.Name, y.String())
		switch {
		case d.Roku != nil:
			tail, _ := st.Entity.DeviceTail()
			if st.Entity.Class != record.ClassRoku || st.Entity.Channel != 50 || tail.IP.String() != "192.168.1.50" || back.Roku == nil || *back.Roku != *d.Roku || back.Power == nil || back.Commands[1].Roku != "keypress/PowerOn" {
				t.Errorf("Roku read back as %+v, %+v", st.Entity, back)
			}
		case d.Sonos != nil:
			if st.Entity.Class != record.ClassSonos || back.Sonos == nil || *back.Sonos != *d.Sonos || *back.Commands[0].Sonos != *d.Commands[0].Sonos {
				t.Errorf("Sonos read back as %+v, %+v", st.Entity, back)
			}
		default:
			if st.Entity.Class != record.ClassIP || st.Commands[0].Type != record.ClassWeb || back.Commands[0].HTTP == nil || back.Commands[0].HTTP.URL != d.Commands[0].HTTP.URL {
				t.Errorf("Govee read back as %+v, %+v", st.Commands[0], back.Commands[0])
			}
		}
	}

	dst := emptyHub(t)
	var log []string
	if err := profile.Restore(ctx, c, dst, profile.ApplyOptions{Report: func(s string) { log = append(log, s) }}); err != nil {
		t.Fatal(err)
	}
	want, got := export(t, c), export(t, dst)
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s restored as:\n%s\nwant:\n%s\nlog:\n%s", k, got[k], w, strings.Join(log, "\n"))
		}
	}
}

// Requests in the newer form are for the X2 only, and a device is one
// kind.
func TestWiFiValidate(t *testing.T) {
	for _, bad := range []string{
		"device: X\nroku: {address: roku.local}\ncommands:\n  - {name: A, roku: keypress/Home}\n",
		"device: X\ncommands:\n  - {name: A, roku: keypress/Home}\n",
		"device: X\nroku: {address: 10.0.0.2}\ncommands:\n  - {name: A, descriptor: 'P:NEC R:38400 D:0 S:206 F:1'}\n",
		"device: X\nroku: {address: 10.0.0.2}\nsonos: {address: 10.0.0.3}\ncommands:\n  - {name: A, roku: keypress/Home}\n",
		"device: X\nsonos: {address: 10.0.0.3}\ncommands:\n  - {name: A, sonos: {path: a/Control, body: '1'}}\ninputs: [A]\n",
		"device: X\ncommands:\n  - {name: A, http: {url: 'ftp://x/y'}}\n",
	} {
		if _, err := profile.Load(strings.NewReader(bad)); err == nil {
			t.Errorf("taken:\n%s", bad)
		}
	}
	ok := "device: X\ncommands:\n  - {name: A, http: {url: 'https://example.com/a', method: PUT}}\n"
	d, err := profile.Load(strings.NewReader(ok))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Commands[0].HTTP.Newer() {
		t.Error("https is not the newer form")
	}
}
