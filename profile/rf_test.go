package profile_test

import (
	"context"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

const rfDevice = `device: ZZ Fan
commands:
  - {name: Power, descriptor: 'P:NEC R:38400 D:0 S:206 F:1'}
  - {name: Speed, data: 0a0b0c0d, rf: 433}
  - {name: Light, data: 01020304, rf: 315}
`

// RF commands are written with their frequency's class, beside IR ones,
// and read back as they were.
func TestRFCommands(t *testing.T) {
	ctx := context.Background()
	d, err := profile.Load(strings.NewReader(rfDevice))
	if err != nil {
		t.Fatal(err)
	}
	c := emptyHub(t)
	id, err := profile.Create(ctx, c, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	st, err := profile.Read(ctx, c, id)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]record.Class{}
	for _, cmd := range st.Commands {
		types[cmd.Label] = cmd.Type
	}
	if types["Speed"] != record.ClassRF433 || types["Light"] != record.ClassRF315 || types["Power"] != record.ClassIR {
		t.Fatalf("types %v", types)
	}
	if changes, _, err := profile.Plan(d, st, true); err != nil || len(changes) != 0 {
		t.Fatalf("after the create: %v %v", changes, err)
	}
	back, err := profile.FromHub(st.Entity, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range back.Commands {
		if want := map[string]int{"Speed": hub.RF433, "Light": hub.RF315}[cmd.Name]; cmd.RF != want {
			t.Errorf("%s read back at %d MHz, want %d", cmd.Name, cmd.RF, want)
		}
	}
}

func TestRFValidate(t *testing.T) {
	for _, bad := range []string{
		"device: X\ncommands:\n  - {name: A, data: 0102, rf: 868}\n",
		"device: X\ncommands:\n  - {name: A, descriptor: 'P:NEC R:38400 D:0 S:206 F:1', rf: 433}\n",
	} {
		if _, err := profile.Load(strings.NewReader(bad)); err == nil {
			t.Errorf("taken:\n%s", bad)
		}
	}
}
