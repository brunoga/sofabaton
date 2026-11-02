package capture

import (
	"bytes"
	"os"
	"slices"
	"testing"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// The app clears a device's key (the Receiver's Up) with the frame that
// clears an activity's, 0x10 [device, key], then syncs; the keymap reads
// back without the key and with the rest as they were, and the commands
// are untouched. Assigning it again is a binding write, as in a create.
func TestX2UnbindDeviceKey(t *testing.T) {
	f, err := os.Open("../testdata/x2/app-clear-device-key.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := Read(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	const receiver = 0x02
	var (
		keymaps  [][]record.Binding
		commands [][]byte // each read of the Receiver's commands, its pages joined
		pages    wire.Assembler
		sent     []string
		at       int // the commands read before the clear
	)
	for _, e := range entries {
		if e.IsNote() {
			continue
		}
		fr, err := e.Frame()
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case e.Dir == wire.ToHub && fr.Family == wire.FamDeleteKey:
			if !bytes.Equal(fr.Payload, []byte{receiver, byte(record.ButtonUp)}) {
				t.Errorf("delete % x", fr.Payload)
			}
			sent = append(sent, "unbind")
			at = len(commands)
		case e.Dir == wire.ToHub && fr.Family == wire.FamWriteButton:
			b, err := record.ParseBinding(fr.Payload[6 : 6+record.BindingLen])
			if err != nil || b.Entity != receiver || b.Button != record.ButtonUp {
				t.Errorf("write %+v %v", b, err)
			}
			sent = append(sent, "bind")
		case e.Dir == wire.ToHub && fr.Family == wire.FamAssignCommit:
			if !bytes.Equal(fr.Payload, []byte{receiver, 0x01}) {
				t.Errorf("sync % x", fr.Payload)
			}
			sent = append(sent, "sync")
		case e.Dir == wire.ToHub && fr.Family == wire.FamReqCommands:
			commands = append(commands, nil)
		case e.Dir == wire.FromHub && fr.Family == wire.FamCommandPage:
			commands[len(commands)-1] = append(commands[len(commands)-1], fr.Payload...)
		case e.Dir == wire.FromHub && fr.Family == wire.FamKeymapPage:
			done, err := pages.Add(fr.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if !done {
				continue
			}
			body, err := pages.Body()
			if err != nil {
				t.Fatal(err)
			}
			pages.Reset()
			rows, err := record.ParseKeymap(body)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) > 0 && rows[0].Entity == receiver {
				keymaps = append(keymaps, rows)
			}
		}
	}
	if want := []string{"unbind", "sync", "bind", "sync"}; !slices.Equal(sent, want) {
		t.Fatalf("sent %q, want %q", sent, want)
	}
	// Read before, after the clear, after the assignment, and on leaving.
	if len(keymaps) != 5 {
		t.Fatalf("%d keymap reads", len(keymaps))
	}
	before, cleared, after := keymaps[1], keymaps[2], keymaps[3]
	without := slices.DeleteFunc(slices.Clone(before), func(b record.Binding) bool { return b.Button == record.ButtonUp })
	if len(without) != len(before)-1 || !slices.Equal(cleared, without) {
		t.Fatalf("cleared %v, before %v", cleared, before)
	}
	if !slices.Equal(after, before) {
		t.Fatalf("assigned again %v, before %v", after, before)
	}
	if at == 0 || len(commands) <= at || len(commands[at]) == 0 || !bytes.Equal(commands[at-1], commands[at]) {
		t.Fatal("the commands changed")
	}
}
