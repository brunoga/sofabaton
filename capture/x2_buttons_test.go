package capture

import (
	"bytes"
	"os"
	"testing"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// The app's key bindings in both device creates rebuild byte for byte from
// a record.Binding that names the command by code only (command id 0, as
// the app writes them); the hub fills in the id (its keymap reads back with
// it), and acks with the button.
func TestX2ButtonWrites(t *testing.T) {
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
		n := 0
		var want byte
		for _, e := range entries {
			if e.IsNote() {
				continue
			}
			fr, err := e.Frame()
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case e.Dir == wire.ToHub && fr.Family == wire.FamWriteButton:
				n++
				p := fr.Payload
				b, err := record.ParseBinding(p[6 : 6+record.BindingLen])
				if err != nil {
					t.Fatal(err)
				}
				if b.Short.Command != 0 || !b.Long.IsZero() {
					t.Errorf("%s: binding %+v", path, b)
				}
				if got := b.WritePayload(); !bytes.Equal(got, p) {
					t.Errorf("%s: rebuilt\n% x\nwant\n% x", path, got, p)
				}
				want = byte(b.Button)
			case e.Dir == wire.FromHub && fr.Family == wire.FamButtonAck:
				if fr.Payload[0] != want {
					t.Errorf("%s: ack % x for button %#x", path, fr.Payload, want)
				}
			}
		}
		if n == 0 {
			t.Errorf("%s: no button writes", path)
		}
	}
}
