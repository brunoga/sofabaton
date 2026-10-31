package record

import (
	"bytes"
	"encoding/hex"
	"sort"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
	"github.com/brunoga/sofabaton/wire"
)

// keymapBursts groups the corpus's complete family-0x3D frames by the test
// they come from and assembles the bursts whose pages are all there.
func keymapBursts(t testing.TB) map[string][]byte {
	t.Helper()
	type frame struct {
		line    int
		payload []byte
	}
	groups := map[string][]frame{}
	for _, c := range testcorpus.Complete(t) {
		raw := c.Bytes()
		if wire.Family(raw[3]) != wire.FamKeymapPage || len(raw) < 8 {
			continue
		}
		path, line, _ := strings.Cut(c.Sources[0], ":")
		n := 0
		for _, d := range line {
			n = n*10 + int(d-'0')
		}
		key := path + "::" + strings.SplitN(c.Context, "/", 2)[0]
		groups[key] = append(groups[key], frame{n, c.Payload()})
	}
	out := map[string][]byte{}
	for key, frames := range groups {
		sort.Slice(frames, func(i, j int) bool { return frames[i].line < frames[j].line })
		var a wire.Assembler
		for i, f := range frames {
			if h, _, _ := wire.SplitPage(f.payload); h.Page == 1 && i > 0 {
				break // a second burst in the same test
			}
			if _, err := a.Add(f.payload); err != nil {
				break
			}
		}
		if body, err := a.Body(); err == nil {
			out[key] = body
		}
	}
	return out
}

func TestKeymapCorpus(t *testing.T) {
	bursts := keymapBursts(t)
	if len(bursts) < 8 {
		t.Fatalf("only %d complete keymap bursts", len(bursts))
	}
	for key, body := range bursts {
		rows, err := ParseKeymap(body)
		if err != nil {
			t.Errorf("%s: %v", key, err)
			continue
		}
		for i, r := range rows {
			if r.Entity != rows[0].Entity {
				t.Errorf("%s row %d: entity %#x, first row %#x", key, i, r.Entity, rows[0].Entity)
			}
			got, _ := r.MarshalBinary()
			if !bytes.Equal(got, body[4+i*BindingLen:4+(i+1)*BindingLen]) {
				t.Errorf("%s row %d: round trip differs", key, i)
			}
		}
	}
}

// Upstream's expectations for the X1S favorites response.
func TestKeymapFavorites(t *testing.T) {
	var body []byte
	for key, b := range keymapBursts(t) {
		if strings.HasSuffix(key, "test_keymap_handler_parses_favorites_from_complete_response") {
			body = b
		}
	}
	rows, err := ParseKeymap(body)
	if err != nil {
		t.Fatal(err)
	}
	fav := map[Button]Press{}
	keys := 0
	for _, r := range rows {
		if r.Button.IsHardKey() {
			keys++
		} else {
			fav[r.Button] = r.Short
		}
	}
	if fav[1].Device != 3 || fav[1].Command != 3 || fav[2].Device != 3 || fav[2].Command != 7 {
		t.Fatalf("favorites %+v", fav)
	}
	if keys != len(rows)-2 {
		t.Fatalf("%d hard keys in %d rows", keys, len(rows))
	}
}

func TestOracleBindings(t *testing.T) {
	var cases []struct {
		Args struct {
			DeviceID    byte   `json:"device_id"`
			ButtonID    byte   `json:"button_id"`
			ShortDevice byte   `json:"short_press_device_id"`
			ShortCode   uint64 `json:"short_press_button_code"`
			ShortID     byte   `json:"short_press_button_id"`
			LongDevice  byte   `json:"long_press_device_id"`
			LongCode    uint64 `json:"long_press_button_code"`
			LongID      byte   `json:"long_press_button_id"`
		} `json:"args"`
		Family    byte   `json:"family"`
		Payload   string `json:"payload"`
		AckOpcode uint16 `json:"ack_opcode"`
	}
	testcorpus.Oracle(t, "bindings", &cases)
	for i, c := range cases {
		a := c.Args
		b := Binding{
			Entity: a.DeviceID, Button: Button(a.ButtonID),
			Short: Press{Device: a.ShortDevice, Code: CodeFromUint(a.ShortCode), Command: a.ShortID},
			Long:  Press{Device: a.LongDevice, Code: CodeFromUint(a.LongCode), Command: a.LongID},
		}
		if got := hex.EncodeToString(b.WritePayload()); got != c.Payload {
			t.Errorf("case %d: got %s, upstream %s", i, got, c.Payload)
		}
		if wire.Family(c.Family) != wire.FamWriteButton || wire.Opcode(c.AckOpcode).Family() != wire.FamButtonAck {
			t.Errorf("case %d: family %#x ack %#x", i, c.Family, c.AckOpcode)
		}
	}
}

func TestButtons(t *testing.T) {
	if ButtonOK.String() != "OK" || !ButtonOK.IsHardKey() || ButtonOK.X2Only() {
		t.Fatal("OK")
	}
	if !ButtonNum5.X2Only() || Button(0x01).IsHardKey() || Button(0x01).String() != "Button(0x01)" {
		t.Fatal("numpad / favorite slot")
	}
	if len(buttonNames) != 42 {
		t.Fatalf("%d buttons", len(buttonNames))
	}
	for s, want := range map[string]Button{"ok": ButtonOK, "Vol+": ButtonVolUp, "power on": ButtonPowerOn, "0x05": 0x05, "5": ButtonNum5} {
		if b, err := ParseButton(s); err != nil || b != want {
			t.Errorf("ParseButton(%q) = %v, %v; want %v", s, b, err, want)
		}
	}
	for _, s := range []string{"", "Volume", "0x100", "07"} {
		if b, err := ParseButton(s); err == nil {
			t.Errorf("ParseButton(%q) = %v", s, b)
		}
	}
}

func TestKeymapErrors(t *testing.T) {
	if _, err := ParseKeymap([]byte{1, 0, 1}); err == nil {
		t.Fatal("headerless keymap accepted")
	}
	if _, err := ParseKeymap([]byte{1, 0, 1, 1, 0x65}); err == nil {
		t.Fatal("short keymap accepted")
	}
	if _, err := ParseBinding(make([]byte, 17)); err == nil {
		t.Fatal("17-byte row accepted")
	}
	if (Press{}).IsZero() != true || (Press{Device: 1}).IsZero() {
		t.Fatal("IsZero")
	}
}

func FuzzParseKeymap(f *testing.F) {
	for _, body := range keymapBursts(f) {
		f.Add(body)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		rows, err := ParseKeymap(body)
		if err != nil {
			return
		}
		for i, r := range rows {
			got, _ := r.MarshalBinary()
			if !bytes.Equal(got, body[4+i*BindingLen:4+(i+1)*BindingLen]) {
				t.Fatalf("row %d: round trip differs", i)
			}
		}
	})
}
