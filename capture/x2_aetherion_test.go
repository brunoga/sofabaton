package capture

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/brunoga/sofabaton/wire"
)

// descriptor returns the descriptive IR string at the end of a command
// write or blob page payload (docs/protocol/write-flows.md).
func descriptor(p []byte) (string, error) {
	const head = 6 + 69 // [seq, page] [count, pages] then the command record
	if len(p) < head+8 {
		return "", fmt.Errorf("payload of %d bytes", len(p))
	}
	blob := p[head:]
	n := int(binary.BigEndian.Uint16(blob))
	if len(blob) < 8+n {
		return "", fmt.Errorf("descriptor of %d bytes in a %d-byte blob", n, len(blob))
	}
	return string(blob[8 : 8+n]), nil
}

// The Android app adding "Aetherion Max", a profile Sofabaton built from
// AWOL Vision's published IR list: every command it writes is that list's
// NEC code, system code 0x00CE as D:0 S:206 and the key value as F, and
// the hub reads them back unchanged.
func TestX2Aetherion(t *testing.T) {
	var sheet struct {
		SystemCode string `json:"system_code"`
		Keys       []struct {
			Name, Value string
		}
	}
	data, err := os.ReadFile("../testdata/x2/aetherion-ir-list.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &sheet); err != nil {
		t.Fatal(err)
	}
	sys, err := strconv.ParseUint(sheet.SystemCode, 0, 16)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{} // descriptor → sheet key
	for _, k := range sheet.Keys {
		v, err := strconv.ParseUint(k.Value, 0, 8)
		if err != nil {
			t.Fatal(err)
		}
		want[fmt.Sprintf("P:NEC R:38400 D:%d S:%d F:%d", sys>>8, sys&0xff, v)] = k.Name
	}

	f, err := os.Open("../testdata/x2/app-add-aetherion.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	written := map[string]string{} // command code → descriptor
	var reads int
	for _, e := range entries {
		if e.IsNote() {
			continue
		}
		fr, err := e.Frame()
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case e.Dir == wire.ToHub && fr.Family == wire.FamWriteCommand:
			d, err := descriptor(fr.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := want[d]; !ok {
				t.Errorf("wrote %q, which is no key of the sheet", d)
			}
			written[string(fr.Payload[9:15])] = d
		case e.Dir == wire.FromHub && fr.Family == wire.FamBlobPage:
			reads++
			d, err := descriptor(fr.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if w := written[string(fr.Payload[9:15])]; d != w {
				t.Errorf("read back %q, wrote %q", d, w)
			}
		}
	}
	if len(written) != 26 || reads != 4 {
		t.Fatalf("%d commands written, %d read back", len(written), reads)
	}
}
