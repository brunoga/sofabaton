package profile_test

import (
	"context"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// A device of HTTP commands, as our X2 took it (2026-11-04).
const network = `device: ZZ-test Wi-Fi
brand: Armrest
commands:
  - name: Hello
    http:
      method: POST
      url: http://192.168.1.87:8060/zz/hello
      content_type: application/json
      body: '{"a": 1}'
  - name: "On"
    http:
      url: http://192.168.1.87:8060/zz/on
  - name: "Off"
    http:
      url: http://192.168.1.87:8060/zz/off
      headers:
        - 'X-Token: t'
buttons:
  OK: Hello
power:
  "on": "On"
  "off": "Off"
`

// A device of HTTP commands is created in upstream's order, and reads
// back as its profile.
func TestCreateNetwork(t *testing.T) {
	ctx := context.Background()
	var sent []wire.Family
	tap := session.WithTap(func(d wire.Direction, f wire.Frame) {
		if d == wire.ToHub {
			sent = append(sent, f.Family)
		}
	})
	m := &hubsim.Model{Info: &record.HubInfo{Model: 0x03, Firmware: 9, Name: "X2 HUB"}}
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{Session: []session.Option{tap}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d, err := profile.Load(strings.NewReader(network))
	if err != nil {
		t.Fatal(err)
	}
	sent = nil
	id, err := profile.Create(ctx, c, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []wire.Family{
		wire.FamCreateDevice, wire.FamWriteCommand, wire.FamWriteCommand, wire.FamWriteCommand,
		wire.FamSetIdleBehavior, wire.FamWriteInputs, wire.FamUpdateDevice, wire.FamAssignCommit,
		wire.FamUpdateDevice, wire.FamSetIdleBehavior, wire.FamWriteMacro, wire.FamWriteMacro,
		wire.FamWriteButton, wire.FamUpdateDevice,
	}
	if len(sent) != len(want) {
		t.Fatalf("sent %v", sent)
	}
	for i := range want {
		if sent[i] != want[i] {
			t.Fatalf("sent %v, want %v", sent, want)
		}
	}

	st, err := profile.Read(ctx, c, id)
	if err != nil {
		t.Fatal(err)
	}
	tail, _ := st.Entity.DeviceTail()
	if st.Entity.Class != record.ClassIP || st.Entity.Type != 16 || tail != record.NewIPDeviceTail() {
		t.Fatalf("record %+v, tail %+v", st.Entity, tail)
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
	if got := strings.Replace(out.String(), "id: 1\nicon: Monitor\ncategory: Wi-Fi\n", "", 1); got != network {
		t.Fatalf("read back:\n%s", out.String())
	}
}

// HTTP commands take plain http to IPv4 addresses, and do not mix with IR.
func TestHTTPProfileErrors(t *testing.T) {
	for _, bad := range []string{
		"url: https://192.168.1.2/x",
		"url: http://lights.local/x",
		"url: http://[::1]/x",
		"url: http://192.168.1.2:0/x",
		"{url: http://192.168.1.2/x, method: PATCH}",
		"{url: http://192.168.1.2/x, headers: [Host: x]}",
	} {
		y := "device: X\ncommands:\n  - name: A\n    http: " + bad + "\n"
		if _, err := profile.Load(strings.NewReader(y)); err == nil {
			t.Errorf("%s: taken", bad)
		}
	}
	mixed := "device: X\ncommands:\n  - {name: A, http: {url: http://192.168.1.2/x}}\n  - {name: B, descriptor: \"P:NEC R:38400 D:0 S:206 F:1\"}\n"
	if _, err := profile.Load(strings.NewReader(mixed)); err == nil {
		t.Error("HTTP and IR commands on one device")
	}
	inputs := "device: X\ncommands:\n  - {name: A, http: {url: http://192.168.1.2/x}}\ninputs: [A]\n"
	if _, err := profile.Load(strings.NewReader(inputs)); err == nil {
		t.Error("inputs on a device of HTTP commands")
	}
}
