package profile_test

import (
	"bytes"
	"context"
	"fmt"
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
buttons:
  OK: OK
  Back: {short: ok, long: Focus}
  power: Power On
`

func TestLoad(t *testing.T) {
	d, err := profile.Load(strings.NewReader(aetherion))
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != 4 || d.IR.System != 0x00CE || len(d.Commands) != 4 || d.Commands[0].Code.Value != 0xE5 || d.Commands[0].Code.Decimal {
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
		"no such key":   "device: X\ncommands:\n  - {name: A, descriptor: \"P:X\"}\nbuttons:\n  Volume: A\n",
		"no such cmd":   "device: X\ncommands:\n  - {name: A, descriptor: \"P:X\"}\nbuttons:\n  OK: B\n",
		"same key":      "device: X\ncommands:\n  - {name: A, descriptor: \"P:X\"}\nbuttons:\n  OK: A\n  ok: A\n",
		"key field":     "device: X\ncommands:\n  - {name: A, descriptor: \"P:X\"}\nbuttons:\n  OK: {shrt: A}\n",
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
		`order       5 commands, as the profile lists them`,
		`button      Power -> "Power On"`,
		`button      OK -> "OK"`,
		`button      Back -> "ok", long "Focus"`,
		`sync        device 4 to the remotes`,
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
	for _, b := range st.Keymap {
		if b.Button == record.ButtonBack && (b.Short.Command != 2 || b.Long.Command != 5) {
			t.Fatalf("Back is bound to %+v", b)
		}
	}
	// Pruning removes what the profile lacks.
	changes, _, _ = profile.Plan(d, st, true)
	if len(changes) != 2 || changes[0].Kind != profile.Remove || changes[0].Command.Label != "Mute" || changes[1].Kind != profile.Sync {
		t.Fatalf("prune %v", changes)
	}

	// Export gives back the profile, codes and all.
	devs, _ := c.Devices(ctx)
	exported, err := profile.FromHub(devs[0], st)
	if err != nil {
		t.Fatal(err)
	}
	if exported.IR == nil || exported.IR.System != 0x00CE || len(exported.Commands) != 5 || exported.Commands[3].Name != "Focus" || exported.Commands[4].Name != "Mute" {
		t.Fatalf("exported %+v", exported)
	}
	if k := exported.Buttons["Back"]; len(exported.Buttons) != 3 || k.Short != "OK" || k.Long != "Focus" {
		t.Fatalf("exported buttons %+v", exported.Buttons)
	}
	var out bytes.Buffer
	if err := exported.Encode(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "buttons:\n  Power: Power On\n  OK: OK\n  Back: {short: OK, long: Focus}\n") {
		t.Fatalf("encoded:\n%s", out.String())
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

func TestCreate(t *testing.T) {
	ctx := context.Background()
	c := simulated(t)
	d, err := profile.Load(strings.NewReader(aetherion + "power: {on: Power On, off: Power On}\ninputs: [Disney+, Focus]\n"))
	if err != nil {
		t.Fatal(err)
	}
	d.Name, d.Power.Off = "ZZ Test", "OK"
	var steps []string
	id, err := profile.Create(ctx, c, d, func(s string) { steps = append(steps, s) })
	if err != nil || id != 5 {
		t.Fatalf("created %d: %v", id, err)
	}
	if steps[0] != `created device 5 "ZZ Test"` || steps[len(steps)-1] != "sync        device 5 to the remotes" || len(steps) != 1+4+3+2+1+3 || steps[len(steps)-3] != `inputs      ["Disney+" "Focus"]` {
		t.Fatalf("steps:\n%s", strings.Join(steps, "\n"))
	}
	devs, _ := c.Devices(ctx)
	dev := devs[len(devs)-1]
	if tail, _ := dev.DeviceTail(); dev.ID != 5 || dev.Name != "ZZ Test" || tail.Marker != 1 || tail.PowerMode != 1 || tail.InputMode != 1 {
		t.Fatalf("device %+v, tail %+v", dev, tail)
	}
	st, err := profile.Read(ctx, c, id)
	if err != nil {
		t.Fatal(err)
	}
	if changes, _, err := profile.Plan(d, st, true); err != nil || len(changes) != 0 {
		t.Fatalf("after the create: %v %v", changes, err)
	}
	back, err := profile.FromHub(dev, st)
	if err != nil {
		t.Fatal(err)
	}
	if back.Power == nil || back.Power.On != "Power On" || back.Power.Off != "OK" || len(back.Inputs) != 2 || back.Inputs[1] != "Focus" || len(back.Buttons) != 3 || len(back.Commands) != 4 {
		t.Fatalf("read back %+v", back)
	}
	if err := c.DeleteDevice(ctx, id); err != nil {
		t.Fatal(err)
	}
	if devs, _ := c.Devices(ctx); len(devs) != 1 {
		t.Fatalf("%d devices after the delete", len(devs))
	}
}

func TestRenameDevice(t *testing.T) {
	ctx := context.Background()
	c := simulated(t)
	st, err := profile.Read(ctx, c, 4)
	if err != nil {
		t.Fatal(err)
	}
	d, err := profile.FromHub(st.Entity, st)
	if err != nil {
		t.Fatal(err)
	}
	d.Name = "Aetherion"
	changes, _, err := profile.Plan(d, st, false)
	if err != nil || len(changes) != 2 || changes[0].Kind != profile.Record ||
		changes[0].String() != `record      rename device "Aetherion Max" -> "Aetherion" (brand "AWOL Vision"), power mode 0 style 0, input mode 0` {
		t.Fatalf("plan %v %v", changes, err)
	}
	if err := profile.Apply(ctx, c, changes, nil); err != nil {
		t.Fatal(err)
	}
	if devs, _ := c.Devices(ctx); devs[0].Name != "Aetherion" || devs[0].ID != 4 {
		t.Fatalf("after: %+v", devs[0])
	}
	st, _ = profile.Read(ctx, c, 4)
	if changes, _, _ := profile.Plan(d, st, false); len(changes) != 0 {
		t.Fatalf("again: %v", changes)
	}
}

// Every Denon code on our X2's receiver (Sofabaton's catalog, 2026-10-31)
// parses as DenonK and renders back to the same descriptor, checksum and
// all; its two Sharp power codes do not parse.
func TestDenonK(t *testing.T) {
	raw, err := os.ReadFile("../testdata/x2/receiver-descriptors.txt")
	if err != nil {
		t.Fatal(err)
	}
	four, one := uint8(4), uint8(1)
	d := &profile.Device{Name: "Receiver", IR: &profile.IR{Protocol: "DenonK", Device: &four, Subdevice: &one}}
	n := 0
	for _, desc := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var f uint16
		if _, err := fmt.Sscanf(desc[strings.Index(desc, " F:")+1:], "F:%d", &f); err != nil || !strings.HasPrefix(desc, "P:DenonK") {
			continue
		}
		n++
		sig, err := profile.Command{Name: "x", Code: &profile.Code{Value: f, Decimal: true}}.Signal(d)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := record.ParseDescriptor(sig); got != desc {
			t.Errorf("F:%d renders %q, the hub has %q", f, got, desc)
		}
	}
	if n != 69 {
		t.Fatalf("%d Denon codes", n)
	}
	var out bytes.Buffer
	d.Commands = []profile.Command{{Name: "Volume Up", Code: &profile.Code{Value: 368, Decimal: true}}}
	if err := d.Encode(&out); err != nil || !strings.Contains(out.String(), "code: 368") || !strings.Contains(out.String(), "device: 4") {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

// Denon's 15-bit codes, as the hub writes them: the catalog's Power Off is
// the chart's Main Zone Off, 2/122; 2/241 (volume up) is one we wrote and
// our receiver obeyed.
func TestDenon15(t *testing.T) {
	two := uint8(2)
	d := &profile.Device{Name: "Receiver", IR: &profile.IR{Protocol: "Denon", Device: &two}}
	for code, want := range map[uint16]string{
		122: "P:Sharp R:38000 D:1 F:61 C0:0 D_CHECKSUM:17 F_CHECKSUM:194 C1:1 D0:17",
		241: "P:Sharp R:38000 D:17 F:120 C0:0 D_CHECKSUM:1 F_CHECKSUM:135 C1:1 D0:1",
	} {
		sig, err := profile.Command{Name: "x", Code: &profile.Code{Value: code, Decimal: true}}.Signal(d)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := record.ParseDescriptor(sig); got != want {
			t.Errorf("2/%d renders %q, want %q", code, got, want)
		}
	}
	// A device in DenonK with one Denon command exports it with its own ir.
	four, one := uint8(4), uint8(1)
	cmds := []profile.Command{
		{Name: "Volume Up", Code: &profile.Code{Value: 368, Decimal: true}},
		{Name: "Mute", Code: &profile.Code{Value: 370, Decimal: true}},
		{Name: "Main Zone Off", Code: &profile.Code{Value: 122, Decimal: true}, IR: &profile.IR{Protocol: "Denon", Device: &two}},
	}
	src := &profile.Device{Name: "Receiver", IR: &profile.IR{Protocol: "DenonK", Device: &four, Subdevice: &one}, Commands: cmds}
	var out bytes.Buffer
	if err := src.Encode(&out); err != nil {
		t.Fatal(err)
	}
	back, err := profile.Load(&out)
	if err != nil || back.Commands[2].IR == nil || back.Commands[2].IR.Protocol != "Denon" || back.Commands[2].Code.Value != 122 {
		t.Fatalf("%v %+v", err, back)
	}
}

func TestActivityExport(t *testing.T) {
	tv, _ := record.NewEntity(wire.X2)
	tv.ID, tv.Name, tv.Class = 1, "TV", record.ClassIR
	avr, _ := record.NewEntity(wire.X2)
	avr.ID, avr.Name, avr.Class = 2, "AVR", record.ClassIR
	act, _ := record.NewEntity(wire.X2)
	act.ID, act.Name = 0x65, "Watch TV"
	command := func(dev, id byte, label string) *record.Command {
		c, _ := record.ParseCommand(wire.X2, make([]byte, 70))
		c.Device, c.ID, c.Label = dev, id, label
		return c
	}
	in, _ := record.NewInputs(wire.X2, 2, record.InputsDirect)
	in.Entries = []record.InputEntry{{Command: 8, Ordinal: 1, Label: "HDMI 1"}, {Command: 9, Ordinal: 2, Label: "HDMI 2"}}
	on, _ := record.NewMacro(wire.X2, 0x65, record.MacroPowerOn, "POWER_ON")
	wait := record.Step{Device: 0xFF, Command: 0xFF, Duration: 0xFF, Delay: 1}
	on.Steps = []record.Step{{Device: 2, Command: 0xC6, Duration: 1, Delay: 0xFF}, wait, {Device: 2, Command: 0xC5, Duration: 2, Delay: 0xFF}, {Device: 1, Command: 0xC5, Delay: 0xFF}}
	seq, _ := record.NewMacro(wire.X2, 0x65, 1, "Movie")
	seq.Steps = []record.Step{{Device: 1, Command: 3, Delay: 0xFF}, wait, {Device: 2, Command: 9, Delay: 0xFF}}
	m := &hubsim.Model{
		Info:       &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 8, Name: "X2 HUB"},
		Devices:    []*record.Entity{tv, avr},
		Activities: []*record.Entity{act},
		Commands:   map[byte][]*record.Command{1: {command(1, 3, "Menu")}, 2: {command(2, 5, "Volume Up"), command(2, 8, "HDMI 1"), command(2, 9, "HDMI 2")}},
		Keymaps: map[byte][]record.Binding{0x65: {
			{Entity: 0x65, Button: record.ButtonMenu, Short: record.Press{Device: 1, Command: 3}, Long: record.Press{Device: 0x65, Command: 1}},
		}},
		Macros:  map[byte][]*record.Macro{0x65: {on, seq}},
		Inputs:  map[byte]*record.Inputs{2: in},
		Members: map[byte][]byte{0x65: {2, 1}},
	}
	ctx := context.Background()
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	st, err := profile.ReadActivity(ctx, c, 0x65)
	if err != nil {
		t.Fatal(err)
	}
	a, err := profile.ActivityFromHub(st)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := a.Encode(&buf); err != nil {
		t.Fatal(err)
	}
	const want = `activity: Watch TV
id: 101
devices:
  - device: AVR
    input: HDMI 2
  - device: TV
buttons:
  Menu: {short: 'TV: Menu', long: Movie}
power_on:
  - power on: AVR
  - wait: 1
  - input: AVR
  - input: TV
sequences:
  - name: Movie
    steps:
      - TV: Menu
      - wait: 1
      - AVR: HDMI 2
`
	if buf.String() != want {
		t.Fatalf("exported\n%s\nwant\n%s", buf.String(), want)
	}
	back, err := profile.LoadActivity(strings.NewReader(want))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(back) != fmt.Sprint(a) {
		t.Fatalf("loaded %+v\nwant %+v", back, a)
	}
	for _, bad := range []string{
		"activity: A\ndevices: []\n",
		"activity: A\ndevices: [{device: TV}]\npower_on: [{power on: AVR}]\n",
		"activity: A\ndevices: [{device: TV}]\nbuttons: {OK: 'AVR: Menu'}\n",
		"activity: A\ndevices: [{device: TV}]\npower_on: [{wait: 0}]\n",
		"activity: A\ndevices: [{device: TV}]\npower_on: [{TV: Menu, wait: 1}]\n",
	} {
		if _, err := profile.LoadActivity(strings.NewReader(bad)); err == nil {
			t.Errorf("loaded %q", bad)
		}
	}
}

// inputs: [] says the device switches no inputs: an empty page and input
// mode 2, as the app wrote for the Ugoos; export gives it back.
func TestNoInputs(t *testing.T) {
	ctx := context.Background()
	c := simulated(t)
	d, err := profile.Load(strings.NewReader(aetherion + "inputs: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Inputs == nil || len(d.Inputs) != 0 {
		t.Fatalf("inputs %#v", d.Inputs)
	}
	st, err := profile.Read(ctx, c, 4)
	if err != nil {
		t.Fatal(err)
	}
	changes, _, err := profile.Plan(d, st, false)
	if err != nil {
		t.Fatal(err)
	}
	var plan []string
	for _, ch := range changes {
		plan = append(plan, ch.String())
	}
	got := strings.Join(plan, "\n")
	if !strings.Contains(got, "inputs      none: the device does not switch inputs") || !strings.Contains(got, "input mode 2") {
		t.Fatalf("plan:\n%s", got)
	}
	if err := profile.Apply(ctx, c, changes, nil); err != nil {
		t.Fatal(err)
	}
	if st, err = profile.Read(ctx, c, 4); err != nil {
		t.Fatal(err)
	}
	if changes, _, _ := profile.Plan(d, st, false); len(changes) != 0 {
		t.Fatalf("applied, still %v", changes)
	}
	if st.Inputs == nil || st.Inputs.Mode != record.InputsNone || len(st.Inputs.Entries) != 0 {
		t.Fatalf("inputs page %+v", st.Inputs)
	}
	exported, err := profile.FromHub(st.Entity, st)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := exported.Encode(&out); err != nil || !strings.Contains(out.String(), "\ninputs: []\n") {
		t.Fatalf("exported %v:\n%s", err, out.String())
	}
}
