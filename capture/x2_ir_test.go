package capture

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// The Android app adding an IR device (a Hisense TV) from the cloud
// catalog through the proxy (docs/protocol/write-flows.md): every command
// it writes carries its signal as a descriptive string, and the create and
// the closing update differ only in the id and the commit marker.
func TestX2AddIRDevice(t *testing.T) {
	f, err := os.Open("../testdata/x2/app-add-ir-device.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	magic := []byte{0x00, 0x00, 0x11, 0x00, 0x94, 0x70}
	var created, updated *record.Entity
	var commands, buttons, macros int
	for _, e := range entries {
		if e.IsNote() || e.Dir != wire.ToHub {
			continue
		}
		fr, err := e.Frame()
		if err != nil {
			t.Fatal(err)
		}
		p := fr.Payload
		switch fr.Family {
		case wire.FamCreateDevice, wire.FamUpdateDevice:
			en, err := record.ParseEntity(wire.X2, p[3:])
			if err != nil {
				t.Fatal(err)
			}
			if fr.Family == wire.FamCreateDevice {
				created = en
			} else {
				updated = en
			}
		case wire.FamWriteCommand:
			commands++
			// [seq, page BE16] [count, pages BE16] device id type code(6)
			// label(60) | length BE16, magic(6), text, 00 00 00 00 |
			// sum8 of the body before it
			if int(p[0]) != commands || p[3] != 61 || p[6] != 4 || p[7] != 0 || p[8] != byte(record.ClassIR) {
				t.Errorf("command %d header % x", commands, p[:15])
			}
			blob := p[6+69:]
			n := int(binary.BigEndian.Uint16(blob))
			var sum byte
			for _, b := range p[3 : len(p)-1] {
				sum += b
			}
			if !bytes.Equal(blob[2:8], magic) || len(blob) != 8+n+5 || !bytes.Equal(blob[8+n:8+n+4], make([]byte, 4)) || p[len(p)-1] != sum {
				t.Errorf("command %d blob % x", commands, blob)
				continue
			}
			if text := string(blob[8 : 8+n]); !strings.HasPrefix(text, "P:NEC R:38400 D:") {
				t.Errorf("command %d descriptor %q", commands, text)
			}
		case wire.FamWriteButton:
			buttons++
		case wire.FamWriteMacro:
			macros++
		}
	}
	if created == nil || updated == nil {
		t.Fatal("no create or update")
	}
	if created.ID != record.NewID || updated.ID != 4 || created.Name != "Hisense" {
		t.Fatalf("create id %d %q, update id %d", created.ID, created.Name, updated.ID)
	}
	ct, _ := created.DeviceTail()
	ut, _ := updated.DeviceTail()
	if ct.Marker != 0 || ut.Marker != 1 {
		t.Errorf("markers %d then %d, want 0 then 1", ct.Marker, ut.Marker)
	}
	if e := ct.Extra; !ct.HasExtra || e != [3]byte{0, 0, 7} {
		t.Errorf("emitter routing % x", e)
	}
	ct.Marker = 1
	if ct != ut {
		t.Errorf("tails differ beyond the marker:\n%+v\n%+v", ct, ut)
	}
	if commands != 61 || buttons != 36 || macros != 2 {
		t.Fatalf("%d commands, %d buttons, %d macros", commands, buttons, macros)
	}
}
