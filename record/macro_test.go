package record

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
	"github.com/brunoga/sofabaton/wire"
)

type macroBurstCase struct {
	Source  string   `json:"source"`
	Variant string   `json:"variant"`
	Frames  []string `json:"frames"`
	Records []struct {
		Activity byte       `json:"activity"`
		Key      byte       `json:"key"`
		Label    string     `json:"label"`
		Slot     string     `json:"slot"`
		Steps    [][]uint64 `json:"steps"`
	} `json:"records"`
}

func burstOf(t testing.TB, frames []string) [][]byte {
	t.Helper()
	var b wire.Burst
	for _, h := range frames {
		raw, _ := hex.DecodeString(h)
		f, _, err := wire.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.Add(f.Payload); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := b.Records()
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func TestOracleMacroBursts(t *testing.T) {
	var cases []macroBurstCase
	testcorpus.Oracle(t, "macros", &cases)
	if len(cases) < 4 {
		t.Fatalf("%d bursts", len(cases))
	}
	for _, c := range cases {
		v := variantOf(t, c.Variant)
		bodies := burstOf(t, c.Frames)
		if len(bodies) != len(c.Records) {
			t.Fatalf("%s: %d records, upstream %d", c.Source, len(bodies), len(c.Records))
		}
		for i, body := range bodies {
			m, err := ParseMacro(v, body)
			if err != nil {
				t.Fatalf("%s #%d: %v", c.Source, i, err)
			}
			w := c.Records[i]
			if m.Entity != w.Activity || m.ID != w.Key || m.Label != w.Label || hex.EncodeToString(m.slot) != w.Slot {
				t.Errorf("%s #%d: %#x/%#x %q, upstream %#x/%#x %q", c.Source, i, m.Entity, m.ID, m.Label, w.Activity, w.Key, w.Label)
			}
			if len(m.Steps) != len(w.Steps) {
				t.Fatalf("%s #%d: %d steps, upstream %d", c.Source, i, len(m.Steps), len(w.Steps))
			}
			for j, st := range m.Steps {
				var code [8]byte
				copy(code[2:], st.Code[:])
				got := []uint64{uint64(st.Device), uint64(st.Command), binary.BigEndian.Uint64(code[:]), uint64(st.Duration), uint64(st.Delay)}
				for k := range got {
					if got[k] != w.Steps[j][k] {
						t.Errorf("%s #%d step %d: %v, upstream %v", c.Source, i, j, got, w.Steps[j])
						break
					}
				}
			}
			// Re-encoded, the record is its own bytes but for the first,
			// where a read carries the record count and a write the marker.
			out, err := m.Body()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out[1:len(out)-1], body[1:len(body)-1]) {
				t.Errorf("%s #%d: re-encoded body differs", c.Source, i)
			}
		}
	}
}

// The X1S capture in which the POWER_ON record spans two pages.
func TestMacroBurstMultiPageRecord(t *testing.T) {
	var frames []string
	for _, line := range []string{"25", "27", "29", "31", "33"} {
		frames = append(frames, testcorpus.BySource(t, "tests/test_phase3_real_capture.py:"+line).Hex)
	}
	bodies := burstOf(t, frames)
	if len(bodies) != 4 {
		t.Fatalf("%d records", len(bodies))
	}
	var labels []string
	for _, body := range bodies {
		m, err := ParseMacro(wire.X1S, body)
		if err != nil {
			t.Fatal(err)
		}
		labels = append(labels, m.Label)
		if m.ID == MacroPowerOn && len(m.Steps) != 20 {
			t.Fatalf("POWER_ON has %d steps", len(m.Steps))
		}
	}
	if labels[2] != "POWER_ON" || labels[3] != "POWER_OFF" {
		t.Fatalf("labels %q", labels)
	}
}

func TestOracleMacroWrites(t *testing.T) {
	var cases []struct {
		Variant string   `json:"variant"`
		Entity  byte     `json:"entity"`
		Key     byte     `json:"key"`
		Label   string   `json:"label"`
		Steps   string   `json:"steps"`
		Pages   []string `json:"pages"`
		Acks    []uint16 `json:"acks"`
	}
	testcorpus.Oracle(t, "macro_writes", &cases)
	for i, c := range cases {
		v := variantOf(t, c.Variant)
		m, err := NewMacro(v, c.Entity, c.Key, c.Label)
		if err != nil {
			t.Fatal(err)
		}
		rows, _ := hex.DecodeString(c.Steps)
		for len(rows) >= StepLen {
			r := rows[:StepLen]
			st := Step{Device: r[0], Command: r[1], Duration: r[8], Delay: r[9]}
			copy(st.Code[:], r[2:8])
			m.Steps = append(m.Steps, st)
			rows = rows[StepLen:]
		}
		pages, err := m.WritePages()
		if err != nil {
			t.Fatal(err)
		}
		// Upstream's macro writer cuts labels at whole UTF-16 pairs, as
		// ours does, so its pages compare unpatched.
		want := c.Pages
		if len(pages) != len(want) {
			t.Fatalf("case %d: %d pages, upstream %d", i, len(pages), len(want))
		}
		for j := range pages {
			if hex.EncodeToString(pages[j]) != want[j] {
				t.Errorf("case %d (%v, %d steps, %q) page %d:\n got %x\nwant %s", i, v, len(m.Steps), c.Label, j+1, pages[j], want[j])
			}
		}
		for j, ack := range c.Acks {
			wantFam := wire.FamStatusAck
			if j == len(c.Acks)-1 {
				wantFam = wire.FamMacroAck
			}
			if wire.Opcode(ack).Family() != wantFam {
				t.Errorf("case %d page %d: upstream acks with %#x", i, j+1, ack)
			}
		}
	}
}

func TestMacroLabelEdits(t *testing.T) {
	s, _ := wire.X2.Schema()
	slot := wire.UTF16BE.Slot("Movie", s.MacroLabelLen)
	slot[54], slot[55] = 0xD4, 0xD4 // hub metadata after the label
	body := append([]byte{1, 0, 1, 0x65, 0x01, 0}, slot...)
	body = append(body, wire.Checksum(body))

	m, err := ParseMacro(wire.X2, body)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := m.Body()
	if !bytes.Equal(out, body) {
		t.Fatal("unchanged macro does not re-encode to its bytes")
	}
	m.Steps = append(m.Steps, Step{Device: 3, Command: 7, Duration: 1})
	out, _ = m.Body()
	if out[len(out)-1-60+54] != 0xD4 {
		t.Fatal("slot metadata lost when only the steps changed")
	}
	m.Label = "Film"
	out, _ = m.Body()
	back, err := ParseMacro(wire.X2, out)
	if err != nil || back.Label != "Film" || len(back.Steps) != 1 {
		t.Fatalf("relabelled: %+v %v", back, err)
	}
}

func TestMacroPowerLabelWithLeadingBytes(t *testing.T) {
	s, _ := wire.X1S.Schema()
	slot := make([]byte, s.MacroLabelLen)
	copy(slot[2:], wire.UTF16BE.Slot("POWER_ON", 16)) // a unit before the text
	slot[1] = 0x05
	if got := decodeMacroLabel(wire.UTF16BE, slot); got != "POWER_ON" {
		t.Fatalf("label %q", got)
	}
}

func TestMacroErrors(t *testing.T) {
	if _, err := ParseMacro(wire.X2, []byte{1, 0, 1, 0x65, 1}); err == nil {
		t.Fatal("5-byte macro accepted")
	}
	if _, err := ParseMacro(wire.X2, append([]byte{1, 0, 1, 0x65, 1, 2}, make([]byte, 61)...)); err == nil {
		t.Fatal("macro short of its steps accepted")
	}
	m, _ := NewMacro(wire.X2, 0x65, 1, "x")
	m.Steps = make([]Step, 256)
	if _, err := m.Body(); err == nil {
		t.Fatal("256 steps accepted")
	}
	if (Step{Command: 0xFF}).IsDelay() != true {
		t.Fatal("IsDelay")
	}
}

func FuzzParseMacro(f *testing.F) {
	var cases []macroBurstCase
	testcorpus.Oracle(f, "macros", &cases)
	for _, c := range cases {
		for _, body := range burstOf(f, c.Frames) {
			f.Add(body)
		}
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		for _, v := range []wire.Variant{wire.X1, wire.X2} {
			m, err := ParseMacro(v, body)
			if err != nil {
				continue
			}
			out, err := m.Body()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out[3:len(out)-1], body[3:len(body)-1]) {
				t.Fatalf("%v: round trip differs", v)
			}
		}
	})
}
