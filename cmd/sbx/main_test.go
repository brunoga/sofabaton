package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"strings"
	"testing"

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
	if got := sbx("sequences"); !strings.Contains(got, "SEQUENCE") {
		t.Fatalf("list sequences:\n%s", got)
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
	if err := sbx("", "apply", "-f", file, "-create", "-n"); err != nil || !strings.Contains(out.String(), `create IR device "ZZ Test" with 3 commands, 1 buttons, powered by "Power On" and "Power Off"`) {
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
