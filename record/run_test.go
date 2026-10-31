package record_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/brunoga/sofabaton/capture"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// The app's two "test this command" frames (family 0x5F) are a macro body
// of the device with key 0, no label and one step.
func TestRunMacroMatchesTheApp(t *testing.T) {
	f, err := os.Open("../testdata/x2/app-add-aetherion.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := capture.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.IsNote() {
			continue
		}
		fr, err := e.Frame()
		if err != nil || fr.Family != wire.FamRunMacro {
			continue
		}
		_, body, _ := wire.SplitPage(fr.Payload)
		got, err := record.ParseMacro(wire.X2, body)
		if err != nil {
			t.Fatal(err)
		}
		m, _ := record.NewMacro(wire.X2, got.Entity, 0, "")
		m.Steps = []record.Step{{Device: got.Steps[0].Device, Command: got.Steps[0].Command, Code: got.Steps[0].Code, Delay: 0xFF}}
		pages, err := m.WritePages()
		if err != nil {
			t.Fatal(err)
		}
		if len(pages) != 1 || !bytes.Equal(pages[0], fr.Payload) {
			t.Fatalf("rebuilt\n %x\nsent\n %x", pages, fr.Payload)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("%d run frames, want 2", n)
	}
}
