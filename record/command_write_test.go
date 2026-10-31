package record_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/brunoga/sofabaton/capture"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// Every command write the Android app made in our captures, rebuilt from
// its decoded fields, is byte for byte what the app sent.
func TestCommandWriteMatchesTheApp(t *testing.T) {
	n := 0
	for _, path := range []string{"../testdata/x2/app-add-ir-device.jsonl", "../testdata/x2/app-add-aetherion.jsonl"} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := capture.Read(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsNote() || e.Dir != wire.ToHub {
				continue
			}
			fr, err := e.Frame()
			if err != nil || fr.Family != wire.FamWriteCommand {
				continue
			}
			hdr, body, err := wire.SplitPage(fr.Payload)
			if err != nil || hdr.Page != 1 {
				t.Fatalf("page %+v %v", hdr, err)
			}
			count, cmd, data, err := record.ParseCommandWrite(wire.X2, body)
			if err != nil {
				t.Fatal(err)
			}
			text, err := record.ParseDescriptor(data)
			if err != nil {
				t.Fatal(err)
			}
			// Rebuild from the fields alone.
			fresh, err := record.NewCommand(wire.X2)
			if err != nil {
				t.Fatal(err)
			}
			fresh.Device, fresh.ID, fresh.Type, fresh.Code, fresh.Label = cmd.Device, cmd.ID, cmd.Type, cmd.Code, cmd.Label
			desc, err := record.Descriptor(text)
			if err != nil {
				t.Fatal(err)
			}
			pages, err := fresh.WritePages(int(hdr.Marker), count, desc)
			if err != nil {
				t.Fatal(err)
			}
			if len(pages) != 1 || !bytes.Equal(pages[0], fr.Payload) {
				t.Fatalf("%s %q: rebuilt\n %x\nsent\n %x", path, cmd.Label, pages, fr.Payload)
			}
			n++
		}
	}
	if n != 87 {
		t.Fatalf("checked %d writes, want 87", n)
	}
}
