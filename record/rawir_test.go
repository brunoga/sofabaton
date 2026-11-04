package record_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"regexp"
	"testing"

	"github.com/brunoga/sofabaton/record"
)

// The projector's learned power code, as the X2 held it, parses and
// encodes back to the same bytes.
func TestRawIRLearned(t *testing.T) {
	y, err := os.ReadFile("../testdata/x2/bluetooth-projector.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := hex.DecodeString(string(regexp.MustCompile(`Power \(IR\)\n    data: ([0-9a-f]+)`).FindSubmatch(y)[1]))
	r, err := record.ParseRawIR(data)
	if err != nil {
		t.Fatal(err)
	}
	if r.Carrier != 26445 || len(r.Timings) != 280 || r.Timings[0] != 7525 || r.Timings[279] != 200000 {
		t.Errorf("carrier %d, %d timings, first %d, last %d", r.Carrier, len(r.Timings), r.Timings[0], r.Timings[len(r.Timings)-1])
	}
	back, err := r.Encode()
	if err != nil || !bytes.Equal(back, data) {
		t.Errorf("encoded back differs (%v)", err)
	}
	if !record.IsIR(data) {
		t.Error("not IR")
	}
	desc, _ := record.Descriptor("P:NEC R:38400 D:0 S:206 F:1")
	if !record.IsIR(desc) || record.IsIR([]byte{0x0c, 0x01, 0x11}) {
		t.Error("IsIR: a descriptor is IR, a Bluetooth command's data is not")
	}
}

func TestRawIREncode(t *testing.T) {
	got, err := record.RawIR{Carrier: 38000, Timings: []uint32{9000, 4500, 560}}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := "0010" + "00000000" + "9470" + "00002328" + "00001194" + "00000230" + "00009c40" + "00000000"
	if hex.EncodeToString(got) != want {
		t.Errorf("got %x, want %s", got, want)
	}
	for _, bad := range []record.RawIR{{Carrier: 38000}, {Carrier: 0, Timings: []uint32{1, 2}}, {Carrier: 38000, Timings: []uint32{0, 1}}} {
		if _, err := bad.Encode(); err == nil {
			t.Errorf("%+v encoded", bad)
		}
	}
}
