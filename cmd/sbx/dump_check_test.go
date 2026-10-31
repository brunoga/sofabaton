package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// TestDumpRoundTrip checks dumps of real hubs, those under testdata/x2
// and the one named by SOFABATON_DUMP: every raw record must decode and
// re-encode to its own bytes (apart from the first, which a reply and a
// write set differently, and the record checksum, which the hub does not
// keep current).
func TestDumpRoundTrip(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/x2/dump-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if p := os.Getenv("SOFABATON_DUMP"); p != "" {
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		t.Fatal("no dumps")
	}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) { checkDump(t, p) })
	}
}

func checkDump(t *testing.T, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Info       struct{ Model string }
		Devices    struct{ Raw []string }
		Activities struct{ Raw []string }
		Entities   []struct {
			Name     string
			Commands *struct{ Raw []string }
			Keymap   *struct{ Raw []string }
			Macros   *struct{ Raw []string }
			Inputs   *struct{ Raw []string }
			Members  *struct{ Raw []string }
		}
	}
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	v := map[string]wire.Variant{"X1": wire.X1, "X1S": wire.X1S, "X2": wire.X2}[d.Info.Model]
	if v == 0 {
		t.Fatalf("model %q", d.Info.Model)
	}

	same := func(what string, got, raw []byte, checksummed bool) {
		t.Helper()
		end := len(raw)
		if checksummed {
			end--
		}
		if len(got) != len(raw) || !bytes.Equal(got[1:end], raw[1:end]) {
			t.Errorf("%s: re-encoded record differs\n got %x\nwant %x", what, got, raw)
		}
	}
	decode := func(h string) []byte {
		b, err := hex.DecodeString(h)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	n := 0
	entity := func(what string, raws []string) {
		for _, h := range raws {
			raw := decode(h)
			e, err := record.ParseEntity(v, raw)
			if err != nil {
				t.Errorf("%s: %v", what, err)
				continue
			}
			got, _ := e.MarshalBinary()
			same(what+" "+e.Name, got, raw, true)
			n++
		}
	}
	entity("device", d.Devices.Raw)
	entity("activity", d.Activities.Raw)
	for _, e := range d.Entities {
		if e.Members != nil {
			entity(e.Name+" member", e.Members.Raw)
		}
		if e.Commands != nil {
			for _, h := range e.Commands.Raw {
				raw := decode(h)
				cmds, err := record.ParseCommandList(v, raw)
				if err != nil {
					t.Errorf("%s commands: %v", e.Name, err)
					continue
				}
				got := append([]byte(nil), raw[:4]...)
				for _, c := range cmds {
					got, _ = c.AppendRecord(got)
				}
				same(e.Name+" commands", got, raw, false)
				n++
			}
		}
		if e.Keymap != nil {
			for _, h := range e.Keymap.Raw {
				raw := decode(h)
				rows, err := record.ParseKeymap(raw)
				if err != nil {
					t.Errorf("%s keymap: %v", e.Name, err)
					continue
				}
				got := append([]byte(nil), raw[:4]...)
				for _, r := range rows {
					got, _ = r.AppendBinary(got)
				}
				same(e.Name+" keymap", got, raw, false)
				n++
			}
		}
		if e.Macros != nil {
			for _, h := range e.Macros.Raw {
				raw := decode(h)
				m, err := record.ParseMacro(v, raw)
				if err != nil {
					t.Errorf("%s macro: %v", e.Name, err)
					continue
				}
				got, _ := m.Body()
				same(e.Name+" macro "+m.Label, got, raw, true)
				n++
			}
		}
		if e.Inputs != nil {
			for _, h := range e.Inputs.Raw {
				raw := decode(h)
				in, err := record.ParseInputs(v, raw)
				if err != nil {
					t.Errorf("%s inputs: %v", e.Name, err)
					continue
				}
				got, _ := in.Body()
				same(e.Name+" inputs", got, raw, false)
				n++
			}
		}
	}
	t.Logf("%d records round-trip", n)
}
