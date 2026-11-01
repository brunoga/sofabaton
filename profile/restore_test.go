package profile_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/dump"
	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

func emptyHub(t *testing.T) *hub.Client {
	t.Helper()
	m := &hubsim.Model{Info: &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 9, Name: "X2 HUB"}}
	c, err := hub.Open(context.Background(), hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// export is every profile of a hub, as YAML, by name.
func export(t *testing.T, c *hub.Client) map[string]string {
	t.Helper()
	ctx := context.Background()
	out := map[string]string{}
	devs, _ := c.Devices(ctx)
	for _, e := range devs {
		st, err := profile.Read(ctx, c, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		d, err := profile.FromHub(st.Entity, st)
		if err != nil {
			t.Fatal(err)
		}
		d.ID = 0
		b, _ := yaml.Marshal(d)
		out[d.Name] = string(b)
	}
	acts, _ := c.Activities(ctx)
	for _, e := range acts {
		st, err := profile.ReadActivity(ctx, c, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		a, err := profile.ActivityFromHub(st)
		if err != nil {
			t.Fatal(err)
		}
		a.ID = 0
		b, _ := yaml.Marshal(a)
		out[a.Name] = string(b)
	}
	return out
}

// A hub restored from another, directly and through a dump, has the same
// profiles; restoring again changes nothing.
func TestRestore(t *testing.T) {
	ctx := context.Background()
	src := emptyHub(t)
	for _, y := range []string{`device: TV
icon: TV
color: red
ir: {protocol: NEC, system: 0x00CE}
commands:
  - {name: Power On, code: 0x01}
  - {name: Power Off, code: 0x02}
  - {name: Menu, code: 0x10}
  - {name: Back, code: 0x11}
power: {on: Power On, off: Power Off}
buttons:
  OK: Menu
macros:
  - name: Menu Back
    steps: [Menu, {wait: 1}, Back]
`, `device: AVR
ir: {protocol: NEC, system: 0x00CE}
commands:
  - {name: Power On, code: 0x21}
  - {name: Power Off, code: 0x22}
  - {name: HDMI 1, code: 0x23}
  - {name: Volume Up, code: 0x24}
power: {on: Power On, off: Power Off}
inputs: [HDMI 1]
`} {
		d, err := profile.Load(strings.NewReader(y))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := profile.Create(ctx, src, d, nil); err != nil {
			t.Fatal(err)
		}
	}
	a, err := profile.LoadActivity(strings.NewReader(`activity: Watch TV
icon: Curved TV
color: green
devices:
  - {device: AVR, input: HDMI 1}
  - {device: TV}
buttons:
  Vol+: 'AVR: Volume Up'
  Red: 'TV: Menu Back'
macros:
  - name: Both Menus
    steps: [TV: Menu, AVR: HDMI 1]
favorites: ['TV: Back', 'AVR: HDMI 1']
`))
	if err != nil {
		t.Fatal(err)
	}
	first := *a
	first.Buttons = profile.Buttons{"Vol+": a.Buttons["Vol+"]}
	if _, err := profile.CreateActivity(ctx, src, &first, nil); err != nil {
		t.Fatal(err)
	}
	// The TV's macro, copied into the activity now that it has the TV.
	st, _ := profile.Read(ctx, src, 1)
	tv, _ := profile.FromHub(st.Entity, st)
	if tv.Macros == nil {
		tv.Macros = profile.MacroList{{Name: "Menu Back", Steps: []profile.DeviceStep{{Command: "Menu"}, {Seconds: 1}, {Command: "Back"}}}}
	}
	changes, _, err := profile.Plan(tv, st, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.Apply(ctx, src, changes, nil); err != nil {
		t.Fatal(err)
	}
	ast, _ := profile.ReadActivity(ctx, src, 101)
	a.ID = 101
	changes2, err := profile.PlanActivity(a, ast, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.ApplyActivity(ctx, src, changes2, nil); err != nil {
		t.Fatal(err)
	}
	want := export(t, src)
	if !strings.Contains(want["Watch TV"], "Red: 'TV: Menu Back'") || !strings.Contains(want["TV"], "Menu Back") {
		t.Fatalf("source:\n%s\n%s", want["Watch TV"], want["TV"])
	}

	check := func(name string, from *hub.Client) {
		t.Helper()
		dst := emptyHub(t)
		var log []string
		if err := profile.Restore(ctx, from, dst, false, false, func(s string) { log = append(log, s) }); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, strings.Join(log, "\n"))
		}
		got := export(t, dst)
		for k, w := range want {
			if got[k] != w {
				t.Errorf("%s: %s restored as:\n%s\nwant:\n%s", name, k, got[k], w)
			}
		}
		log = nil
		if err := profile.Restore(ctx, from, dst, false, true, func(s string) { log = append(log, s) }); err != nil {
			t.Fatal(err)
		}
		for _, l := range log {
			if !strings.HasSuffix(l, ": matches") {
				t.Errorf("%s: again: %s", name, l)
			}
		}
	}
	check("hub", src)

	var buf bytes.Buffer
	if err := dump.Read(ctx, src, "test", nil).Write(&buf); err != nil {
		t.Fatal(err)
	}
	d, err := dump.Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	from, err := d.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer from.Close()
	check("dump", from)
}
