package capture

import (
	"bytes"
	"os"
	"testing"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// The app creating an activity, adding and editing a sequence, rebinding
// a key, removing a member, renaming and deleting the activity rebuilds
// byte for byte from records made from scratch, or from the hub's records
// with the app's change.
func TestX2Activity(t *testing.T) {
	f, err := os.Open("../testdata/x2/app-activity.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := Read(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	const act = 0x69
	wait := func(s byte) record.Step {
		return record.Step{Device: 0xFF, Command: 0xFF, Code: [6]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, Duration: 0xFF, Delay: s}
	}
	call := func(dev, key, duration byte) record.Step {
		return record.Step{Device: dev, Command: key, Duration: duration, Delay: 0xFF}
	}
	command := func(dev, cmd byte, code uint16) record.Step {
		s := record.Step{Device: dev, Command: cmd, Delay: 0xFF}
		s.Code[4], s.Code[5] = byte(code>>8), byte(code)
		return s
	}
	macro := func(id byte, label string, steps ...record.Step) []byte {
		m, _ := record.NewMacro(wire.X2, act, id, label)
		m.Steps = steps
		pages, err := m.WritePages()
		if err != nil || len(pages) != 1 {
			t.Fatalf("%s: %d pages, %v", label, len(pages), err)
		}
		return pages[0]
	}
	// The receiver (2) to its 4th input, the Aetherion (4) to its 2nd,
	// the Ugoos (5) to none; a second apart.
	powerOn := macro(record.MacroPowerOn, "POWER_ON",
		call(2, 0xC6, 0), wait(1), call(4, 0xC6, 0), wait(1), call(5, 0xC6, 0), wait(1),
		call(2, 0xC5, 4), wait(1), call(4, 0xC5, 2), wait(1), call(5, 0xC5, 0), wait(1))
	powerOff := macro(record.MacroPowerOff, "POWER_OFF",
		call(2, 0xC7, 0), wait(1), call(4, 0xC7, 0), wait(1), call(5, 0xC7, 0), wait(1))
	seq := []record.Step{command(4, 0x12, 0x2D), wait(4), command(5, 0x08, 0)}
	created, _ := record.NewActivity(wire.X2, "ZZ Actuvity", 4)
	createBody, _ := created.WritePayload()

	var row *record.Entity // the hub's record of the new activity
	var macros []*record.Macro
	seen := map[string]int{}
	for _, e := range entries {
		if e.IsNote() {
			continue
		}
		fr, err := e.Frame()
		if err != nil {
			t.Fatal(err)
		}
		p := fr.Payload
		if e.Dir == wire.FromHub {
			switch fr.Family {
			case wire.FamActivityRow:
				if r, err := record.ParseEntity(wire.X2, p[3:]); err == nil && r.ID == act {
					row = r
				}
			case wire.FamMacroPage:
				if m, err := record.ParseMacro(wire.X2, p[3:]); err == nil && m.Entity == act {
					macros = append(macros, m)
				}
			}
			continue
		}
		check := func(what string, want []byte) {
			t.Helper()
			seen[what]++
			if !bytes.Equal(p, want) {
				t.Errorf("%s\n% x\nwant\n% x", what, p, want)
			}
		}
		switch fr.Family {
		case wire.FamConfirmMember:
			if p[1] != 0 || (p[0] != 2 && p[0] != 4 && p[0] != 5) {
				t.Errorf("confirm member % x", p)
			}
			seen["confirm"]++
		case wire.FamCreateActivity:
			check("create", createBody)
		case wire.FamWriteMacro:
			switch {
			case p[6] == act && p[7] == record.MacroPowerOn && len(macros) == 0:
				check("power on", powerOn)
			case p[6] == act && p[7] == record.MacroPowerOff && len(macros) == 0:
				check("power off", powerOff)
			case p[6] == act && p[7] == 0:
				check("new sequence", macro(0, "ZZ Seq", seq...))
			case p[6] == act && p[7] == 1:
				check("edited sequence", macro(1, "ZZ Seq", append(seq, command(4, 0x17, 0x4FAF))...))
			case p[6] == act && p[7] == record.MacroPowerOn:
				// Removing the Ugoos: the hub's POWER_ON without its steps.
				m := latest(macros, record.MacroPowerOn)
				m.Steps = []record.Step{call(2, 0xC6, 0), call(4, 0xC6, 0), call(2, 0xC5, 4), call(4, 0xC5, 2)}
				pages, _ := m.WritePages()
				check("member removed, power on", pages[0])
			case p[6] == act && p[7] == record.MacroPowerOff:
				m := latest(macros, record.MacroPowerOff)
				m.Steps = []record.Step{call(2, 0xC7, 0), call(4, 0xC7, 0)}
				pages, _ := m.WritePages()
				check("member removed, power off", pages[0])
			}
		case wire.FamKeySort:
			if p[6] == act {
				check("sequence order", record.OrderPages(act, []record.Place{{Command: 1, Position: 1}})[0])
			}
		case wire.FamWriteButton:
			if p[6] == act && p[7] == 0xBE {
				check("key", record.Binding{Entity: act, Button: 0xBE, Short: record.Press{Device: 5, Command: 7}}.WritePayload())
			}
		case wire.FamWriteActivity:
			if row == nil {
				t.Fatal("update before the hub listed the activity")
			}
			r := *row
			r.Tail = append([]byte(nil), row.Tail...)
			r.PowerState = 1 // the activity was on by then; the row is from before
			if seen["update"] > 0 {
				r.Name = "ZZ Actuvity 2"
			}
			b, _ := r.WritePayload()
			check("update", b)
		case wire.FamDeleteActivity:
			check("delete", []byte{act})
		}
	}
	for what, n := range map[string]int{
		"confirm": 3, "create": 1, "power on": 1, "power off": 1, "new sequence": 1, "sequence order": 1,
		"key": 1, "edited sequence": 1, "member removed, power on": 1, "member removed, power off": 1,
		"update": 2, "delete": 1,
	} {
		if seen[what] != n {
			t.Errorf("%s: %d frames, want %d", what, seen[what], n)
		}
	}
}

func latest(macros []*record.Macro, id byte) *record.Macro {
	for i := len(macros) - 1; i >= 0; i-- {
		if macros[i].ID == id {
			return macros[i]
		}
	}
	return nil
}
