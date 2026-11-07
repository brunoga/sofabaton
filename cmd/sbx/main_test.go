package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/dump"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/transport"
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
	simulate(t, m)
}

// simulate has sbx connect to a simulated hub of m.
func simulate(t *testing.T, m *hubsim.Model) {
	t.Helper()
	prev := dialHub
	dialHub = func(ctx context.Context, _ netip.Addr, opts hub.Options) (*hub.Client, error) {
		return hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), opts)
	}
	t.Cleanup(func() { dialHub = prev })
}

func TestInfo(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"info", "-hub", "192.0.2.1"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "X2 HUB") || !strings.Contains(out.String(), "model     X2") {
		t.Fatalf("info:\n%s", out.String())
	}
}

func TestDump(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"dump", "-hub", "192.0.2.1", "-v"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	var d dump.File
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
	if err := run(context.Background(), strings.NewReader(""), nil, &out, &errs); err == nil {
		t.Fatal("no command accepted")
	}
	if err := run(context.Background(), strings.NewReader(""), []string{"bogus"}, &out, &errs); err == nil {
		t.Fatal("unknown command accepted")
	}
	out.Reset()
	if err := run(context.Background(), strings.NewReader(""), []string{"--help"}, &out, &errs); err != nil || !strings.HasPrefix(out.String(), "sbx dev: ") || !strings.Contains(out.String(), "usage: sbx") {
		t.Fatalf("--help: %q %v", out.String(), err)
	}
	out.Reset()
	if err := run(context.Background(), strings.NewReader(""), []string{"version"}, &out, &errs); err != nil || strings.TrimSpace(out.String()) != "dev" {
		t.Fatalf("version: %q %v", out.String(), err)
	}
}

// discovering makes discovery find hubs at addrs.
func discovering(t *testing.T, addrs ...string) {
	t.Helper()
	t.Setenv(hubEnv, "")
	prev := discoverHubs
	discoverHubs = func(context.Context) ([]transport.Announcement, error) {
		var found []transport.Announcement
		for i, a := range addrs {
			info := &record.HubInfo{Variant: wire.X2, Name: "X2 HUB"}
			info.ID[0] = byte(i)
			found = append(found, transport.Announcement{Addr: netip.MustParseAddr(a), Info: info})
		}
		return found, nil
	}
	t.Cleanup(func() { discoverHubs = prev })
}

func TestHubDiscovery(t *testing.T) {
	simulated(t)
	var dialed netip.Addr
	sim := dialHub
	dialHub = func(ctx context.Context, a netip.Addr, opts hub.Options) (*hub.Client, error) {
		dialed = a
		return sim(ctx, a, opts)
	}
	info := func() (string, error) {
		var out, errs bytes.Buffer
		err := run(context.Background(), strings.NewReader(""), []string{"info"}, &out, &errs)
		return errs.String(), err
	}

	discovering(t, "192.0.2.7")
	if msg, err := info(); err != nil || dialed != netip.MustParseAddr("192.0.2.7") || !strings.Contains(msg, "using hub 192.0.2.7") {
		t.Fatalf("one hub: dialed %v, %q, %v", dialed, msg, err)
	}
	t.Setenv(hubEnv, "192.0.2.9")
	if _, err := info(); err != nil || dialed != netip.MustParseAddr("192.0.2.9") {
		t.Fatalf("$%s: dialed %v, %v", hubEnv, dialed, err)
	}

	discovering(t)
	if _, err := info(); err == nil || !strings.Contains(err.Error(), "-hub") {
		t.Fatalf("no hub: %v", err)
	}
	discovering(t, "192.0.2.7", "192.0.2.8")
	if _, err := info(); err == nil || !strings.Contains(err.Error(), "192.0.2.8") {
		t.Fatalf("two hubs: %v", err)
	}
	// A proxy on this machine is not a hub to connect to.
	prev := localAddrs
	localAddrs = func() map[netip.Addr]bool { return map[netip.Addr]bool{netip.MustParseAddr("192.0.2.8"): true} }
	t.Cleanup(func() { localAddrs = prev })
	if _, err := info(); err != nil || dialed != netip.MustParseAddr("192.0.2.7") {
		t.Fatalf("hub and local proxy: dialed %v, %v", dialed, err)
	}
	discovering(t, "192.0.2.8")
	if _, err := info(); err == nil || !strings.Contains(err.Error(), "proxy") {
		t.Fatalf("only the proxy: %v", err)
	}
}

func TestExportActivity(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"export", "-hub", "192.0.2.1", "-activity", "watch tv"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "activity: Watch TV") || !strings.Contains(got, "- device: TV") || !strings.Contains(got, "OK: 'TV: Power'") {
		t.Fatalf("exported %s", got)
	}
	if err := run(context.Background(), strings.NewReader(""), []string{"export", "-hub", "192.0.2.1", "-activity", "x", "-device", "1"}, &out, &errs); err == nil {
		t.Fatal("exported with -device and -activity")
	}
}

func TestApplyActivity(t *testing.T) {
	simulated(t)
	dir := t.TempDir()
	var out, errs bytes.Buffer
	sbx := func(stdin string, args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(stdin), append(args, "-hub", "192.0.2.1"), &out, &errs)
	}
	discovering(t) // every command must take the -hub after its arguments
	file := dir + "/watch.yaml"
	if err := sbx("", "export", "-activity", "Watch TV", "-o", file); err != nil {
		t.Fatal(err)
	}
	if err := sbx("", "apply", "-f", file); err != nil || !strings.Contains(out.String(), "already matches") {
		t.Fatalf("apply the export: %v\n%s", err, out.String())
	}
	data, _ := os.ReadFile(file)
	edited := strings.Replace(string(data), "activity: Watch TV", "activity: Watch Telly", 1)
	if err := os.WriteFile(file, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sbx("", "apply", "-f", file, "-n"); err != nil || !strings.Contains(out.String(), `rename activity "Watch TV" -> "Watch Telly"`) {
		t.Fatalf("plan: %v\n%s", err, out.String())
	}
	if err := sbx("", "apply", "-f", file, "-device", "1"); err == nil {
		t.Fatal("applied an activity's profile with -device")
	}
	if err := sbx("y\n", "apply", "-f", file); err != nil {
		t.Fatal(err)
	}
	newer := dir + "/new.yaml"
	if err := os.WriteFile(newer, []byte("activity: Late Show\ndevices: [{device: TV}]\nbuttons: {OK: 'TV: Power'}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sbx("", "apply", "-f", newer, "-create", "-y"); err != nil || !strings.Contains(out.String(), `created activity 102 "Late Show"`) {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	if err := sbx("", "apply", "-f", newer, "-create", "-y"); err == nil {
		t.Fatal("created the activity twice")
	}
	if err := sbx("", "remove-activity", "-activity", "late show", "-y"); err != nil {
		t.Fatal(err)
	}
	if err := sbx("", "list", "activities"); err != nil || strings.Contains(out.String(), "Late Show") || !strings.Contains(out.String(), "Watch Telly") {
		t.Fatalf("list: %v\n%s", err, out.String())
	}
}

func TestAddIR(t *testing.T) {
	simulated(t)
	code := "P:NEC R:38400 D:0 S:206 F:217"
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"add-ir", "-hub", "192.0.2.1", "-device", "1", "-label", "Focus", "-code", code, "-n"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "WriteCommand 01 00 01 01 00 01 01 02 0d") {
		t.Fatalf("dry run:\n%s", out.String())
	}
	out.Reset()
	if err := run(context.Background(), strings.NewReader(""), []string{"add-ir", "-hub", "192.0.2.1", "-device", "1", "-label", "Focus", "-code", code}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `added "Focus" to device 1: id 2`) {
		t.Fatalf("add:\n%s", out.String())
	}
	for _, bad := range [][]string{
		{"-device", "1", "-label", "power", "-code", code}, // label taken
		{"-device", "9", "-label", "X", "-code", code},     // no such device
		{"-device", "1", "-label", "X", "-code", "NEC 1"},  // not a descriptor
	} {
		if err := run(context.Background(), strings.NewReader(""), append([]string{"add-ir", "-hub", "192.0.2.1"}, bad...), &out, &errs); err == nil {
			t.Errorf("add-ir %v succeeded", bad)
		}
	}
}

func TestRemoveIR(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	run1 := func(args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(""), args, &out, &errs)
	}
	if err := run1("add-ir", "-hub", "192.0.2.1", "-device", "1", "-label", "Focus", "-code", "P:NEC R:38400 D:0 S:206 F:217"); err != nil {
		t.Fatal(err)
	}
	if err := run1("remove-ir", "-hub", "192.0.2.1", "-device", "1", "-label", "focus"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `removed "Focus" (id 2) from device 1`) {
		t.Fatalf("remove:\n%s", out.String())
	}
	if err := run1("remove-ir", "-hub", "192.0.2.1", "-device", "1", "-label", "Focus"); err == nil {
		t.Fatal("removed it twice")
	}
}

func TestExportApply(t *testing.T) {
	simulated(t)
	dir := t.TempDir()
	var out, errs bytes.Buffer
	sbx := func(stdin string, args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(stdin), args, &out, &errs)
	}
	if err := sbx("", "add-ir", "-hub", "192.0.2.1", "-device", "1", "-label", "Focus", "-code", "P:NEC R:38400 D:0 S:206 F:217"); err != nil {
		t.Fatal(err)
	}
	// The simulated TV's "Power" has no data: export needs every command's.
	if err := sbx("", "export", "-hub", "192.0.2.1", "-device", "1"); err == nil {
		t.Fatal("exported a command without data")
	}
	if err := sbx("", "remove-ir", "-hub", "192.0.2.1", "-device", "1", "-id", "1"); err != nil {
		t.Fatal(err)
	}
	file := dir + "/tv.yaml"
	if err := sbx("", "export", "-hub", "192.0.2.1", "-device", "1", "-o", file); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(data), "code: 0xD9") || !strings.Contains(string(data), "id: 1") {
		t.Fatalf("exported %s %v", data, err)
	}
	edited := strings.Replace(string(data), "name: Focus", "name: Lens Focus", 1) +
		"  - name: Menu\n    code: 0x5A\n    comment: from the IR list\n" +
		"buttons:\n  OK: {short: Menu, long: Lens Focus}\n"
	if err := os.WriteFile(file, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sbx("", "apply", "-hub", "192.0.2.1", "-f", file, "-n"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `rename "Focus" -> "Lens Focus"`) || !strings.Contains(out.String(), `add       1 "Menu"`) ||
		!strings.Contains(out.String(), `button      OK -> "Menu", long "Lens Focus"`) {
		t.Fatalf("plan:\n%s", out.String())
	}
	if err := sbx("n\n", "apply", "-hub", "192.0.2.1", "-f", file); err == nil {
		t.Fatal("applied without a yes")
	}
	if err := sbx("y\n", "apply", "-hub", "192.0.2.1", "-f", file); err != nil {
		t.Fatal(err)
	}
	if err := sbx("", "apply", "-hub", "192.0.2.1", "-f", file, "-n"); err != nil || !strings.Contains(out.String(), "already matches") {
		t.Fatalf("after apply: %v\n%s", err, out.String())
	}
}

func TestSend(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"send", "-hub", "192.0.2.1", "-device", "1", "-command", "power"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `sent "Power" (id 1) of device 1`) {
		t.Fatalf("send:\n%s", out.String())
	}
	if err := run(context.Background(), strings.NewReader(""), []string{"send", "-hub", "192.0.2.1", "-device", "1", "-command", "Nope"}, &out, &errs); err == nil {
		t.Fatal("sent a command that does not exist")
	}
}

func TestList(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	sbx := func(args ...string) string {
		t.Helper()
		out.Reset()
		if err := run(context.Background(), strings.NewReader(""), append([]string{"list", "-hub", "192.0.2.1"}, args...), &out, &errs); err != nil {
			t.Fatalf("list %v: %v", args, err)
		}
		return out.String()
	}
	if got := sbx(); !strings.Contains(got, "TV") || !strings.Contains(got, "Watch TV") {
		t.Fatalf("list:\n%s", got)
	}
	if got := sbx("commands", "tv"); !strings.Contains(got, "1   Power") {
		t.Fatalf("list commands:\n%s", got)
	}
	if got := sbx("buttons", "watch tv"); !strings.Contains(got, "LONG PRESS") {
		t.Fatalf("list buttons:\n%s", got)
	}
	if got := sbx("macros"); !strings.Contains(got, "MACRO") {
		t.Fatalf("list macros:\n%s", got)
	}
	if err := run(context.Background(), strings.NewReader(""), []string{"list", "-hub", "192.0.2.1", "commands", "Radio"}, &out, &errs); err == nil || !strings.Contains(err.Error(), `"TV"`) {
		t.Fatalf("unknown device: %v", err)
	}
}

func TestDeviceByName(t *testing.T) {
	simulated(t)
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"send", "-hub", "192.0.2.1", "-device", "tv", "-command", "Power"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAndRemoveDevice(t *testing.T) {
	simulated(t)
	file := t.TempDir() + "/new.yaml"
	if err := os.WriteFile(file, []byte(`device: ZZ Test
ir: {protocol: NEC, system: 0x00CE}
commands:
  - {name: Power On, code: 0xE5}
  - {name: Power Off, code: 0xE6}
  - {name: Focus, code: 0xD9}
power: {on: Power On, off: Power Off}
buttons:
  OK: Focus
`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	sbx := func(stdin string, args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(stdin), append(args[:1:1], append([]string{"-hub", "192.0.2.1"}, args[1:]...)...), &out, &errs)
	}
	if err := sbx("", "apply", "-f", file, "-create", "-n"); err != nil || !strings.Contains(out.String(), `create device "ZZ Test" with 3 commands, 1 buttons, powered by "Power On" and "Power Off"`) {
		t.Fatalf("dry run: %v\n%s", err, out.String())
	}
	if err := sbx("y\n", "apply", "-f", file, "-create"); err != nil || !strings.Contains(out.String(), "done: committed device") {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	if err := sbx("", "apply", "-f", file, "-create", "-n"); err == nil {
		t.Fatal("created it twice")
	}
	if err := sbx("", "apply", "-f", file, "-n"); err != nil || !strings.Contains(out.String(), "already matches") {
		t.Fatalf("after the create: %v\n%s", err, out.String())
	}
	if err := sbx("", "sync", "-device", "zz test"); err != nil || !strings.Contains(out.String(), `synced`) {
		t.Fatalf("sync: %v\n%s", err, out.String())
	}
	if err := sbx("n\n", "remove-device", "-device", "zz test"); err == nil {
		t.Fatal("deleted without a yes")
	}
	if err := sbx("y\n", "remove-device", "-device", "zz test"); err != nil || !strings.Contains(out.String(), `deleted device`) {
		t.Fatalf("remove: %v\n%s", err, out.String())
	}
}

// templates lists what sbx carries; new makes a profile of one, which
// apply takes and a template is refused by.
func TestTemplates(t *testing.T) {
	var out, errs bytes.Buffer
	sbx := func(args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(""), args, &out, &errs)
	}
	if err := sbx("templates"); err != nil || !strings.Contains(out.String(), "ir/denon/avr-x8500h") {
		t.Fatalf("templates: %q %v", out.String(), err)
	}
	dir := t.TempDir()
	file := dir + "/receiver.yaml"
	if err := sbx("new", "-template", "ir/denon/avr-x8500h", "-name", "Receiver", "-o", file); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(data), "\nfrom: ir/denon/avr-x8500h\ndevice: Receiver\n") || !strings.HasPrefix(string(data), "# Denon") {
		t.Fatalf("new: %s %v", data, err)
	}
	if err := sbx("new", "-template", "ir/denon/avr-x8500h", "-o", file); err == nil {
		t.Error("new overwrote a file")
	}
	if err := sbx("new", "-template", "ir/nobody/nothing"); err == nil {
		t.Error("new made a profile of no template")
	}
	if err := sbx("new", "-template", "../../templates/ir/ugoos/am6b-plus.yaml"); err != nil || !strings.Contains(out.String(), "\ndevice: Ugoos AM6B Plus\n") {
		t.Errorf("new from a file: %v\n%s", err, out.String())
	}
	if err := sbx("apply", "-f", "../../templates/ir/ugoos/am6b-plus.yaml", "-n"); err == nil || !strings.Contains(err.Error(), "is a template") {
		t.Errorf("applied a template: %v", err)
	}
}

// import-ir makes a profile of a Flipper file, raw signals included,
// saying what it left out.
func TestImportIR(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/My_Projector.ir"
	if err := os.WriteFile(file, []byte(`Filetype: IR signals file
Version: 1
#
name: Power
type: parsed
protocol: NECext
address: 00 CE 00 00
command: 12 ED 00 00
#
name: Ok
type: raw
frequency: 38000
duty_cycle: 0.330000
data: 9000 4500
#
name: Eject
type: parsed
protocol: Samsung36
address: 01 00 00 00
command: 02 00 00 00
`), 0o666); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"import-ir", "-f", file}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "device: My Projector") || !strings.Contains(out.String(), "name: Power") || !strings.Contains(out.String(), "name: OK") {
		t.Errorf("profile:\n%s", out.String())
	}
	if !strings.Contains(errs.String(), "left out Eject: protocol Samsung36") {
		t.Errorf("stderr: %s", errs.String())
	}
}

// export-ir writes a profile's codes in a format, told by -o's
// extension, saying what it left out.
func TestExportIR(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/tv.yaml"
	if err := os.WriteFile(file, []byte(`device: TV
ir:
  protocol: NEC
  system: 0x04FB
commands:
  - name: Power
    code: 0x08
  - name: Pair
    data: "0102"
`), 0o666); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"export-ir", "-f", file, "-o", dir + "/tv.ir"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dir + "/tv.ir")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "name: Power\ntype: parsed\nprotocol: NEC\naddress: 04 00 00 00\ncommand: 08 00 00 00") {
		t.Errorf("file:\n%s", b)
	}
	if !strings.Contains(errs.String(), "left out Pair: not an IR code") {
		t.Errorf("stderr: %s", errs.String())
	}
	out.Reset()
	if err := run(context.Background(), strings.NewReader(""), []string{"export-ir", "-f", file, "-format", "pronto"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Power: 0000 006C 0022 0000 ") {
		t.Errorf("pronto:\n%s", out.String())
	}
}

func TestOrder(t *testing.T) {
	var devs []*record.Entity
	for i, name := range []string{"TV", "AVR", "Player"} {
		e, _ := record.NewEntity(wire.X2)
		e.ID, e.Name, e.Class, e.Sort = byte(i+1), name, record.ClassIR, byte(i+1)
		devs = append(devs, e)
	}
	m := &hubsim.Model{Info: &record.HubInfo{Model: 0x03, Firmware: 9, Name: "X2 HUB"}, Devices: devs}
	prev := dialHub
	dialHub = func(ctx context.Context, _ netip.Addr, opts hub.Options) (*hub.Client, error) {
		return hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), opts)
	}
	t.Cleanup(func() { dialHub = prev })
	var out, errs bytes.Buffer
	if err := run(context.Background(), nil, []string{"order", "-hub", "192.0.2.1", "player", "2"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if want := " 1. Player (3)\n 2. AVR (2)\n 3. TV (1)\n"; out.String() != want {
		t.Fatalf("order:\n%s", out.String())
	}
	if devs[0].Sort != 3 || devs[1].Sort != 2 || devs[2].Sort != 1 || m.OrderSyncs != 1 {
		t.Fatalf("sorts %d %d %d, %d syncs", devs[0].Sort, devs[1].Sort, devs[2].Sort, m.OrderSyncs)
	}
	if err := run(context.Background(), nil, []string{"order", "-hub", "192.0.2.1", "TV", "TV"}, &out, &errs); err == nil {
		t.Error("a device named twice")
	}
}

// apply -create makes a Bluetooth device and has the hub pair with it;
// pair pairs it again.
func TestCreateBluetooth(t *testing.T) {
	simulated(t)
	file := t.TempDir() + "/bt.yaml"
	if err := os.WriteFile(file, []byte(`device: ZZ Box
bluetooth: {profile: d272ab7af5404b549eb260d367112165}
commands:
  - {name: Play, data: 0c00b0}
  - {name: Power, data: "070066"}
power: {on: Power, off: Power}
buttons:
  Play: Play
`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	sbx := func(stdin string, args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(stdin), append(args[:1:1], append([]string{"-hub", "192.0.2.1"}, args[1:]...)...), &out, &errs)
	}
	if err := sbx("y\n", "apply", "-f", file, "-create"); err != nil || !strings.Contains(out.String(), `paired "ZZ Box"`) {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	if err := sbx("", "apply", "-f", file, "-n"); err != nil || !strings.Contains(out.String(), "already matches") {
		t.Fatalf("after the create: %v\n%s", err, out.String())
	}
	if err := sbx("", "pair", "-device", "zz box"); err != nil || !strings.Contains(out.String(), `paired "ZZ Box"`) {
		t.Fatalf("pair: %v\n%s", err, out.String())
	}
	if err := sbx("", "pair", "-device", "TV"); err == nil {
		t.Error("paired an IR device")
	}
}

// restore creates a Bluetooth device the hub lacks and has it pair, unless
// told not to.
func TestRestoreBluetooth(t *testing.T) {
	empty := func() *hubsim.Model {
		return &hubsim.Model{Info: &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 8, Name: "X2 HUB"}}
	}
	simulate(t, empty())
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/bt.yaml", []byte(`device: ZZ Box
bluetooth: {profile: d272ab7af5404b549eb260d367112165}
commands:
  - {name: Play, data: 0c00b0}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	sbx := func(stdin string, args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(stdin), append(args[:1:1], append([]string{"-hub", "192.0.2.1"}, args[1:]...)...), &out, &errs)
	}
	if err := sbx("y\n", "apply", "-f", dir+"/bt.yaml", "-create"); err != nil {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	if err := sbx("", "dump"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/dump.json", out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	dst := empty() // a hub without it
	simulate(t, dst)
	if err := sbx("", "restore", "-f", dir+"/dump.json", "-no-pair", "-n"); err != nil || !strings.Contains(out.String(), `"ZZ Box": skipped`) {
		t.Fatalf("-no-pair: %v\n%s", err, out.String())
	}
	if err := sbx("", "restore", "-f", dir+"/dump.json", "-n"); err != nil || !strings.Contains(out.String(), `"ZZ Box": create`) {
		t.Fatalf("plan: %v\n%s", err, out.String())
	}
	if err := sbx("", "restore", "-f", dir+"/dump.json", "-y"); err != nil || !strings.Contains(out.String(), `paired "ZZ Box"`) {
		t.Fatalf("restore: %v\n%s", err, out.String())
	}
	if len(dst.Devices) != 1 || dst.Devices[0].Name != "ZZ Box" || len(dst.Paired) != 1 {
		t.Fatalf("devices %v, paired %v", dst.Devices, dst.Paired)
	}
}

func TestLearn(t *testing.T) {
	tv, _ := record.NewEntity(wire.X2)
	tv.ID, tv.Name, tv.Class = 1, "TV", record.ClassIR
	_ = tv.SetDeviceTail(record.DeviceTail{InputMode: 1})
	code, err := record.RawIR{Carrier: 38000, Timings: []uint32{9000, 4500, 560, 560, 560, 40000}}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	m := &hubsim.Model{
		Info:    &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 8, Name: "X2 HUB"},
		Devices: []*record.Entity{tv},
		Learned: code, LearnFailures: 1,
	}
	simulate(t, m)
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"learn", "-hub", "192.0.2.1", "-device", "TV", "-name", "Mute", "-key", "Mute"}, &out, &errs); err != nil {
		t.Fatalf("%v\n%s%s", err, out.String(), errs.String())
	}
	if !strings.Contains(out.String(), "could not read that") || !strings.Contains(out.String(), "learned: 6 timings at 38000 Hz") {
		t.Errorf("output:\n%s", out.String())
	}
	cmds := m.Commands[1]
	if len(cmds) != 1 || cmds[0].Label != "Mute" || cmds[0].Type != record.ClassIR || !bytes.Equal(m.Data[1][cmds[0].ID], code) {
		t.Fatalf("the hub holds %v", cmds)
	}
	bound := false
	for _, b := range m.Keymaps[1] {
		bound = bound || (b.Button == record.ButtonMute && b.Short.Command == cmds[0].ID)
	}
	if !bound || !slices.Contains(m.Synced, 1) {
		t.Errorf("Mute bound %v, synced %v", bound, m.Synced)
	}

	m.Learned = nil
	if err := run(context.Background(), strings.NewReader(""), []string{"learn", "-hub", "192.0.2.1", "-device", "TV", "-name", "Other", "-wait", "200ms"}, &out, &errs); err == nil || !strings.Contains(err.Error(), "learned no code") {
		t.Errorf("a window closed without a code: %v", err)
	}
}
