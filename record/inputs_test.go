package record

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
	"github.com/brunoga/sofabaton/wire"
)

type inputsCase struct {
	Variant string   `json:"variant"`
	Pages   []string `json:"pages"`
	Parsed  struct {
		Device  byte `json:"device"`
		Mode    byte `json:"mode"`
		FlagA   byte `json:"flag_a"`
		FlagB   byte `json:"flag_b"`
		State   byte `json:"state"`
		Entries []struct {
			Command byte   `json:"command"`
			FID     uint64 `json:"fid"`
			Ordinal byte   `json:"ordinal"`
			Label   string `json:"label"`
		} `json:"entries"`
		ControlKeys []string `json:"control_keys"`
		Favorites   []string `json:"favorites"`
	} `json:"parsed"`
}

func assemble(t testing.TB, pages []string) []byte {
	t.Helper()
	var a wire.Assembler
	for _, h := range pages {
		p, _ := hex.DecodeString(h)
		if _, err := a.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	body, err := a.Body()
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// Decoding upstream's pages must give its fields, and rebuilding the page
// from them its pages.
func TestOracleInputs(t *testing.T) {
	var cases []inputsCase
	testcorpus.Oracle(t, "inputs", &cases)
	for i, c := range cases {
		v := variantOf(t, c.Variant)
		in, err := ParseInputs(v, assemble(t, c.Pages))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		w := c.Parsed
		if in.Device != w.Device || in.Mode != w.Mode || in.FlagA != w.FlagA || in.FlagB != w.FlagB || in.State != w.State {
			t.Errorf("case %d: header %+v, upstream %+v", i, in, w)
		}
		if len(in.Entries) != len(w.Entries) {
			t.Fatalf("case %d: %d entries, upstream %d", i, len(in.Entries), len(w.Entries))
		}
		for j, e := range in.Entries {
			var code [8]byte
			copy(code[2:], e.Code[:])
			we := w.Entries[j]
			if e.Command != we.Command || binary.BigEndian.Uint64(code[:]) != we.FID || e.Ordinal != we.Ordinal ||
				strings.TrimSpace(e.Label) != we.Label {
				t.Errorf("case %d entry %d: %+v, upstream %+v", i, j, e, we)
			}
		}
		for j, k := range in.ControlKeys {
			if hex.EncodeToString(k[:]) != w.ControlKeys[j] {
				t.Errorf("case %d control key %d differs", i, j)
			}
		}
		for j, f := range in.Favorites {
			if hex.EncodeToString(f[:]) != w.Favorites[j] {
				t.Errorf("case %d favorite %d differs", i, j)
			}
		}

		pages, err := in.WritePages()
		if err != nil {
			t.Fatal(err)
		}
		if len(pages) != len(c.Pages) {
			t.Fatalf("case %d: %d pages, upstream %d", i, len(pages), len(c.Pages))
		}
		for j := range pages {
			if hex.EncodeToString(pages[j]) != c.Pages[j] {
				t.Errorf("case %d (%v, %d entries) page %d differs", i, v, len(in.Entries), j+1)
			}
		}
	}
}

func TestInputsFromScratch(t *testing.T) {
	in, err := NewInputs(wire.X2, 0x05, InputsDirect)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range []string{"HDMI 1", "HDMI 2", "Optical"} {
		in.Entries = append(in.Entries, InputEntry{Command: byte(10 + i), Code: CodeFromUint(uint64(0x100 + i)), Ordinal: byte(i + 1), Label: l})
	}
	body, err := in.Body()
	if err != nil {
		t.Fatal(err)
	}
	if want := 8 + 3*48 + 107 + 1; len(body) != want {
		t.Fatalf("body %d bytes, want %d", len(body), want)
	}
	back, err := ParseInputs(wire.X2, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Entries) != 3 || back.Entries[2].Label != "Optical" || back.Entries[2].Ordinal != 3 {
		t.Fatalf("round trip %+v", back.Entries)
	}
	again, _ := back.Body()
	if !bytes.Equal(again, body) {
		t.Fatal("decoded page does not re-encode to its bytes")
	}
}

// The hub may leave bytes between the entries and the trailer; they are
// kept and the trailer is still found from the end.
func TestInputsSlack(t *testing.T) {
	in, _ := NewInputs(wire.X1, 1, InputsDirect)
	in.Entries = []InputEntry{{Command: 1, Label: "TV"}}
	body, _ := in.Body()
	trailer := len(body) - 1 - inputsTrailerLen
	slacked := append(append(append([]byte(nil), body[:trailer]...), 0xEE, 0xEE), body[trailer:]...)
	slacked[len(slacked)-1] = wire.Checksum(slacked[:len(slacked)-1])
	back, err := ParseInputs(wire.X1, slacked)
	if err != nil {
		t.Fatal(err)
	}
	if back.State != in.State || len(back.Entries) != 1 {
		t.Fatalf("decoded %+v", back)
	}
	out, _ := back.Body()
	if !bytes.Equal(out, slacked) {
		t.Fatal("slack lost")
	}
}

func TestInputsErrors(t *testing.T) {
	if _, err := ParseInputs(wire.X2, make([]byte, 20)); err == nil {
		t.Fatal("short page accepted")
	}
	body := make([]byte, 8+107+1)
	body[5] = 3 // three entries that are not there
	if _, err := ParseInputs(wire.X2, body); err == nil {
		t.Fatal("page short of its entries accepted")
	}
}

func FuzzParseInputs(f *testing.F) {
	var cases []inputsCase
	testcorpus.Oracle(f, "inputs", &cases)
	for _, c := range cases {
		f.Add(assemble(f, c.Pages))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		for _, v := range []wire.Variant{wire.X1, wire.X2} {
			in, err := ParseInputs(v, body)
			if err != nil {
				continue
			}
			out, err := in.Body()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out[3:len(out)-1], body[3:len(body)-1]) {
				t.Fatalf("%v: round trip differs", v)
			}
		}
	})
}
