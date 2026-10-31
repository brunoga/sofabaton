package profile_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

const aetherion = `# AWOL Vision, from the IR list
device: Aetherion Max
brand: AWOL Vision
id: 4
ir: {protocol: NEC, system: 0x00CE}
commands:
  - {name: Power On, code: 0xE5}
  - {name: OK, code: 0x28, comment: was "Ok"}
  - {name: Disney+, code: 0xDE}
  - {name: Focus, code: 0xD9, comment: opens the focus screen}
`

func TestLoad(t *testing.T) {
	d, err := profile.Load(strings.NewReader(aetherion))
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != 4 || d.IR.System != 0x00CE || len(d.Commands) != 4 || *d.Commands[0].Code != 0xE5 {
		t.Fatalf("%+v", d)
	}
	sig, err := d.Commands[3].Signal(d)
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := record.ParseDescriptor(sig); text != "P:NEC R:38400 D:0 S:206 F:217" {
		t.Fatalf("Focus is %q", text)
	}
	var out bytes.Buffer
	if err := d.Encode(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "system: 0x00CE") || !strings.Contains(out.String(), "code: 0xE5") {
		t.Fatalf("encoded:\n%s", out.String())
	}
	again, err := profile.Load(&out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if again.Commands[3].Comment != "opens the focus screen" {
		t.Fatalf("round trip %+v", again.Commands[3])
	}
}

func TestLoadRejects(t *testing.T) {
	for name, text := range map[string]string{
		"unknown field": "device: X\ncommands:\n  - {name: A, cod: 0x01}\n",
		"no signal":     "device: X\ncommands:\n  - {name: A}\n",
		"two signals":   "device: X\nir: {protocol: NEC, system: 0x1}\ncommands:\n  - {name: A, code: 0x01, descriptor: \"P:NEC R:38400 D:0 S:1 F:1\"}\n",
		"code, no ir":   "device: X\ncommands:\n  - {name: A, code: 0x01}\n",
		"duplicate":     "device: X\ncommands:\n  - {name: A, descriptor: \"P:X\"}\n  - {name: a, descriptor: \"P:Y\"}\n",
		"not a byte":    "device: X\nir: {protocol: NEC, system: 0x1}\ncommands:\n  - {name: A, code: 0x100}\n",
		"other proto":   "device: X\nir: {protocol: RC5, system: 0x1}\ncommands:\n  - {name: A, code: 0x01}\n",
	} {
		if _, err := profile.Load(strings.NewReader(text)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// simulated is a hub with the Aetherion's device 4 holding three of its
// commands, as Sofabaton's profile named them.
func simulated(t *testing.T) *hub.Client {
	t.Helper()
	dev, _ := record.NewEntity(wire.X2)
	dev.ID, dev.Name, dev.Brand, dev.Class = 4, "Aetherion Max", "AWOL Vision", record.ClassIR
	m := &hubsim.Model{
		Info:    &record.HubInfo{Model: 0x03, Firmware: 9, Name: "X2 HUB"},
		Devices: []*record.Entity{dev},
		// Commands come in through the write handler, as on a hub.
	}
	c, err := hub.Open(context.Background(), hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	for i, x := range []struct {
		label string
		f     int
	}{{"Power on", 229}, {"Ok", 40}, {"disnesy", 222}, {"Mute", 27}} {
		cmd, _ := record.NewCommand(wire.X2)
		cmd.Device, cmd.ID, cmd.Type, cmd.Label = 4, byte(i+1), record.ClassIR, x.label
		data, _ := record.Descriptor("P:NEC R:38400 D:0 S:206 F:" + itoa(x.f))
		if err := c.WriteCommand(context.Background(), cmd, data); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestPlanAndApply(t *testing.T) {
	ctx := context.Background()
	c := simulated(t)
	d, err := profile.Load(strings.NewReader(aetherion))
	if err != nil {
		t.Fatal(err)
	}
	st, err := profile.Read(ctx, c, 4)
	if err != nil {
		t.Fatal(err)
	}
	changes, kept, err := profile.Plan(d, st, false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, ch := range changes {
		got = append(got, ch.String())
	}
	want := []string{
		`update    1 rename "Power on" -> "Power On"`,
		`update    2 rename "Ok" -> "OK"`,
		`update    3 rename "disnesy" -> "Disney+"`,
		`add       5 "Focus"`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("plan:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(kept) != 1 || kept[0].Label != "Mute" {
		t.Fatalf("kept %v", kept)
	}
	if err := profile.Apply(ctx, c, changes, nil); err != nil {
		t.Fatal(err)
	}
	st, err = profile.Read(ctx, c, 4)
	if err != nil {
		t.Fatal(err)
	}
	if changes, _, _ := profile.Plan(d, st, false); len(changes) != 0 {
		t.Fatalf("applied, still %v", changes)
	}
	// Pruning removes what the profile lacks.
	changes, _, _ = profile.Plan(d, st, true)
	if len(changes) != 1 || changes[0].Kind != profile.Remove || changes[0].Command.Label != "Mute" {
		t.Fatalf("prune %v", changes)
	}

	// Export gives back the profile, codes and all.
	devs, _ := c.Devices(ctx)
	exported, err := profile.FromHub(devs[0], st.Commands, st.Data)
	if err != nil {
		t.Fatal(err)
	}
	if exported.IR == nil || exported.IR.System != 0x00CE || len(exported.Commands) != 5 || exported.Commands[3].Name != "Mute" {
		t.Fatalf("exported %+v", exported)
	}
}

// The profiles we maintain all load, and the Aetherion's has every key of
// AWOL Vision's IR list.
func TestMaintainedProfiles(t *testing.T) {
	paths, err := filepath.Glob("../profiles/*.yaml")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no profiles: %v", err)
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		d, err := profile.Load(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if filepath.Base(p) == "awol-vision-aetherion.yaml" && len(d.Commands) != 35 {
			t.Errorf("%s: %d commands, the IR list has 35", p, len(d.Commands))
		}
	}
}
