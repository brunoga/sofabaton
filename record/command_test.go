package record

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
	"github.com/brunoga/sofabaton/wire"
)

type commandBurst struct {
	Source  string   `json:"source"`
	Variant string   `json:"variant"`
	Frames  []string `json:"frames"`
	Device  byte     `json:"device"`
	Records []struct {
		Device  byte   `json:"device"`
		ID      byte   `json:"id"`
		Control string `json:"control"`
		Label   string `json:"label"`
		Sort    byte   `json:"sort"`
	} `json:"records"`
}

func assembleFrames(t *testing.T, frames []string) []byte {
	t.Helper()
	var a wire.Assembler
	for _, h := range frames {
		raw, _ := hex.DecodeString(h)
		f, _, err := wire.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Add(f.Payload); err != nil {
			t.Fatal(err)
		}
	}
	body, err := a.Body()
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// Assembling and decoding the corpus's command bursts must give upstream's
// commands, and re-encoding each record its own bytes.
func TestOracleCommandBursts(t *testing.T) {
	var bursts []commandBurst
	testcorpus.Oracle(t, "commands", &bursts)
	if len(bursts) < 8 {
		t.Fatalf("%d bursts", len(bursts))
	}
	for _, b := range bursts {
		v := variantOf(t, b.Variant)
		body := assembleFrames(t, b.Frames)
		cmds, err := ParseCommandList(v, body)
		if err != nil {
			t.Fatalf("%s: %v", b.Source, err)
		}
		if len(cmds) != len(b.Records) {
			t.Fatalf("%s: %d commands, upstream %d", b.Source, len(cmds), len(b.Records))
		}
		s, _ := v.Schema()
		for i, c := range cmds {
			w := b.Records[i]
			control := append([]byte{byte(c.Type)}, c.Code[:]...)
			if c.Device != w.Device || c.ID != w.ID || hex.EncodeToString(control) != w.Control ||
				c.Label != w.Label || c.Sort != w.Sort {
				t.Errorf("%s #%d: got %d/%d %x %q sort %d, upstream %d/%d %s %q sort %d", b.Source, i,
					c.Device, c.ID, control, c.Label, c.Sort, w.Device, w.ID, w.Control, w.Label, w.Sort)
			}
			got, err := c.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if want := body[4+i*s.CommandStride : 4+(i+1)*s.CommandStride]; !bytes.Equal(got, want) {
				t.Errorf("%s #%d: re-encoded record differs", b.Source, i)
			}
		}
	}
}

func TestCommandFromScratch(t *testing.T) {
	c := &Command{Device: 2, ID: 1, Type: ClassMQTT, Label: "Lights Toggle", Sort: 0xFF, variant: wire.X2}
	rec, err := c.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseCommand(wire.X2, rec)
	if err != nil {
		t.Fatal(err)
	}
	if back.Label != c.Label || back.Type != ClassMQTT || back.Sort != 0xFF || len(rec) != 70 {
		t.Fatalf("round trip %+v", back)
	}
	back.Label = "Lights Off"
	rec2, _ := back.MarshalBinary()
	if again, _ := ParseCommand(wire.X2, rec2); again.Label != "Lights Off" {
		t.Fatalf("relabelled to %q", again.Label)
	}
}

func TestCommandListErrors(t *testing.T) {
	if _, err := ParseCommandList(wire.X2, []byte{1, 0, 1}); err == nil {
		t.Fatal("headerless list accepted")
	}
	if _, err := ParseCommandList(wire.X2, append([]byte{1, 0, 1, 2}, make([]byte, 70)...)); err == nil {
		t.Fatal("list short of its count accepted")
	}
	if _, err := ParseCommand(wire.X1, make([]byte, 70)); err == nil {
		t.Fatal("X1 accepted a 70-byte record")
	}
	if l, err := ParseCommandList(wire.X1, []byte{1, 0, 1, 0}); err != nil || len(l) != 0 {
		t.Fatalf("empty list: %v %v", l, err)
	}
}

func FuzzParseCommandList(f *testing.F) {
	var bursts []commandBurst
	testcorpus.Oracle(f, "commands", &bursts)
	for _, b := range bursts {
		var a wire.Assembler
		for _, h := range b.Frames {
			raw, _ := hex.DecodeString(h)
			if fr, _, err := wire.Parse(raw); err == nil {
				_, _ = a.Add(fr.Payload)
			}
		}
		if body, err := a.Body(); err == nil {
			f.Add(body)
		}
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		for _, v := range []wire.Variant{wire.X1, wire.X2} {
			cmds, err := ParseCommandList(v, body)
			if err != nil {
				continue
			}
			s, _ := v.Schema()
			for i, c := range cmds {
				rec, err := c.MarshalBinary()
				if err != nil || !bytes.Equal(rec, body[4+i*s.CommandStride:4+(i+1)*s.CommandStride]) {
					t.Fatalf("%v #%d: round trip differs", v, i)
				}
			}
		}
	})
}
