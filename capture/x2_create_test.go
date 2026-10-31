package capture

import (
	"bytes"
	"os"
	"testing"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// The frames of the app's device creates that are not commands or keys
// rebuild byte for byte from records made from scratch: the create (a new
// record with the catalog's fields), the idle behavior, the power
// sequences and the closing update (the create with the hub's id and the
// commit marker).
func TestX2CreateFrames(t *testing.T) {
	for _, path := range []string{"../testdata/x2/app-add-ir-device.jsonl", "../testdata/x2/app-add-aetherion.jsonl"} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := Read(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		var created *record.Entity
		var id byte
		macros, done := 0, false
		for _, e := range entries {
			if e.IsNote() || done {
				continue
			}
			fr, err := e.Frame()
			if err != nil {
				t.Fatal(err)
			}
			p := fr.Payload
			if e.Dir == wire.FromHub {
				if fr.Family == wire.FamCreateDeviceAck && created != nil {
					id = p[0]
				}
				continue
			}
			switch fr.Family {
			case wire.FamCreateDevice:
				got, err := record.ParseEntity(wire.X2, p[3:])
				if err != nil {
					t.Fatal(err)
				}
				tail, err := got.DeviceTail()
				if err != nil {
					t.Fatal(err)
				}
				created, _ = record.NewEntity(wire.X2)
				created.Icon, created.Class, created.Type, created.CodeID = got.Icon, record.ClassIR, got.Type, got.CodeID
				created.Name, created.Brand = got.Name, got.Brand
				if err := created.SetDeviceTail(record.NewIRDeviceTail()); err != nil {
					t.Fatal(err)
				}
				if tail != record.NewIRDeviceTail() {
					t.Errorf("%s: create tail %+v", path, tail)
				}
				if b, _ := created.WritePayload(); !bytes.Equal(b, p) {
					t.Errorf("%s: create\n% x\nwant\n% x", path, b, p)
				}
			case wire.FamSetIdleBehavior:
				if !bytes.Equal(p, []byte{id, record.IdleDefault}) {
					t.Errorf("%s: idle behavior % x", path, p)
				}
			case wire.FamWriteMacro:
				macros++
				key, label, cmd := record.MacroPowerOn, "POWER_ON", p[10]
				if p[7] == record.MacroPowerOff {
					key, label = record.MacroPowerOff, "POWER_OFF"
				}
				m, _ := record.NewMacro(wire.X2, id, key, label)
				s := record.Step{Device: id, Command: cmd, Delay: 0xFF}
				copy(s.Code[:], p[11:17])
				m.Steps = []record.Step{s}
				if pages, _ := m.WritePages(); len(pages) != 1 || !bytes.Equal(pages[0], p) {
					t.Errorf("%s: macro\n% x\nwant\n% x", path, pages, p)
				}
			case wire.FamUpdateDevice:
				done = true // what follows is the user's configuration
				up := *created
				up.ID = id
				tail, _ := up.DeviceTail()
				tail.Marker = 1
				up.Tail = append([]byte(nil), created.Tail...)
				if err := up.SetDeviceTail(tail); err != nil {
					t.Fatal(err)
				}
				if b, _ := up.WritePayload(); !bytes.Equal(b, p) {
					t.Errorf("%s: update\n% x\nwant\n% x", path, b, p)
				}
			}
		}
		if created == nil || id == 0 || macros != 2 || !done {
			t.Errorf("%s: create %v, id %d, %d macros, committed %v", path, created != nil, id, macros, done)
		}
	}
}

// The app's inputs page for the Aetherion (HDMI 1-3 and Home, each its
// own command) rebuilds byte for byte from a new record.Inputs, followed
// by the device's update with input mode 1 and the commit 65 [device, 01].
func TestX2InputsWrite(t *testing.T) {
	f, err := os.Open("../testdata/x2/app-add-aetherion.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := Read(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	var pages [][]byte
	var commit []byte
	for _, e := range entries {
		if e.IsNote() || e.Dir != wire.ToHub {
			continue
		}
		fr, _ := e.Frame()
		switch fr.Family {
		case wire.FamWriteInputs:
			pages = append(pages, fr.Payload)
		case wire.FamAssignCommit:
			commit = fr.Payload
		}
	}
	in, _ := record.NewInputs(wire.X2, 4, record.InputsDirect)
	for i, x := range []struct {
		cmd   byte
		code  uint64
		label string
	}{{7, 0x085E, "Hdmi1"}, {8, 0x0863, "Hdmi2"}, {9, 0x0868, "Hdmi3"}, {0x11, 0x07C7, "Home"}} {
		in.Entries = append(in.Entries, record.InputEntry{Command: x.cmd, Code: record.CodeFromUint(x.code), Ordinal: byte(i + 1), Label: x.label})
	}
	got, err := in.WritePages()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(pages) {
		t.Fatalf("%d pages, want %d", len(got), len(pages))
	}
	for i := range got {
		if !bytes.Equal(got[i], pages[i]) {
			t.Errorf("page %d\n% x\nwant\n% x", i+1, got[i], pages[i])
		}
	}
	if !bytes.Equal(commit, []byte{4, 1}) {
		t.Errorf("commit % x", commit)
	}
}
