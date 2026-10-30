package record

import (
	"encoding/hex"
	"net/netip"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
	"github.com/brunoga/sofabaton/wire"
)

// upstreamConfig is upstream's DeviceConfig as the oracle writes it.
type upstreamConfig struct {
	Name          string  `json:"name"`
	Brand         string  `json:"brand"`
	DeviceID      byte    `json:"device_id"`
	RecordKind    byte    `json:"record_kind"`
	Icon          byte    `json:"icon"`
	Sort          byte    `json:"sort"`
	CodeType      byte    `json:"code_type"`
	DeviceType    byte    `json:"device_type"`
	CodeID        string  `json:"code_id"`
	Hide          byte    `json:"hide"`
	InputFlag     byte    `json:"input_flag"`
	Channel       byte    `json:"channel"`
	PowerState    byte    `json:"power_state"`
	IPAddress     *string `json:"ip_address"`
	PollTime      int     `json:"poll_time"`
	InputMode     byte    `json:"input_mode"`
	PowerMode     byte    `json:"power_mode"`
	PowerStyle    byte    `json:"power_style"`
	ShareMode     byte    `json:"share_mode"`
	TailFlag      byte    `json:"tail_flag"`
	TailMarker    byte    `json:"tail_marker"`
	ExtraA        byte    `json:"extra_a"`
	ExtraB        byte    `json:"extra_b"`
	ExtraC        byte    `json:"extra_c"`
	ExtrasPresent bool    `json:"extras_present"`
	ColorID       byte    `json:"color_id"`
	BTMode        byte    `json:"bt_mode"`
}

func (c upstreamConfig) entity(t *testing.T, v wire.Variant) *Entity {
	t.Helper()
	e, err := NewEntity(v)
	if err != nil {
		t.Fatal(err)
	}
	e.Name, e.Brand = c.Name, c.Brand
	e.ID, e.Kind, e.Icon, e.Sort = c.DeviceID, c.RecordKind, c.Icon, c.Sort
	e.Class, e.Type = Class(c.CodeType), c.DeviceType
	code, _ := hex.DecodeString(c.CodeID)
	copy(e.CodeID[:], code)
	e.Hide, e.InputFlag, e.Channel, e.PowerState = c.Hide, c.InputFlag, c.Channel, c.PowerState
	if err := e.SetDeviceTail(c.tail()); err != nil {
		t.Fatal(err)
	}
	return e
}

func (c upstreamConfig) tail() DeviceTail {
	d := DeviceTail{
		HasPollTime: c.PollTime >= 0,
		InputMode:   c.InputMode, PowerMode: c.PowerMode, PowerStyle: c.PowerStyle, ShareMode: c.ShareMode,
		Flag: c.TailFlag, Marker: c.TailMarker,
		HasExtra: c.ExtrasPresent, Extra: [3]byte{c.ExtraA, c.ExtraB, c.ExtraC},
		Color: c.ColorID, BTMode: c.BTMode,
	}
	if c.PollTime >= 0 {
		d.PollTime = uint16(c.PollTime)
	}
	if c.IPAddress != nil {
		d.IP = netip.MustParseAddr(*c.IPAddress)
	}
	return d
}

func variantOf(t *testing.T, s string) wire.Variant {
	t.Helper()
	switch s {
	case "X1":
		return wire.X1
	case "X1S":
		return wire.X1S
	case "X2":
		return wire.X2
	}
	t.Fatalf("variant %q", s)
	return 0
}

// The same fields must give upstream's create payload.
func TestOracleEntityBuild(t *testing.T) {
	var cases []struct {
		Variant string         `json:"variant"`
		Config  upstreamConfig `json:"config"`
		Payload string         `json:"payload"`
	}
	testcorpus.Oracle(t, "entities", &cases)
	for i, c := range cases {
		v := variantOf(t, c.Variant)
		got, err := c.Config.entity(t, v).WritePayload()
		if err != nil {
			t.Fatal(err)
		}
		want := withoutHalfPairs(t, v, c.Payload)
		if hex.EncodeToString(got) != want {
			t.Errorf("case %d (%v, name %q, brand %q):\n got %x\nwant %s", i, v, c.Config.Name, c.Config.Brand, got, want)
		}
	}
}

// Decoding upstream's payloads must give the fields upstream decodes.
func TestOracleEntityParse(t *testing.T) {
	var cases []struct {
		Variant string         `json:"variant"`
		Payload string         `json:"payload"`
		Parsed  upstreamConfig `json:"parsed"`
	}
	testcorpus.Oracle(t, "entities", &cases)
	for i, c := range cases {
		checkParsed(t, i, variantOf(t, c.Variant), c.Payload, c.Parsed)
	}
}

// Upstream's decoding of the corpus's record frames must match ours.
func TestOracleCorpusRecords(t *testing.T) {
	var cases []struct {
		Source  string         `json:"source"`
		Variant string         `json:"variant"`
		Parsed  upstreamConfig `json:"parsed"`
	}
	testcorpus.Oracle(t, "corpus_records", &cases)
	for i, c := range cases {
		payload := hex.EncodeToString(testcorpus.BySource(t, c.Source).Payload())
		checkParsed(t, i, variantOf(t, c.Variant), payload, c.Parsed)
	}
}

func checkParsed(t *testing.T, i int, v wire.Variant, payloadHex string, want upstreamConfig) {
	t.Helper()
	p, _ := hex.DecodeString(payloadHex)
	e, err := ParseEntity(v, p[3:])
	if err != nil {
		t.Fatalf("case %d: %v", i, err)
	}
	tail, err := e.DeviceTail()
	if err != nil {
		t.Fatal(err)
	}
	wantTail := want.tail()
	for _, f := range []struct {
		name      string
		got, want any
	}{
		{"name", strings.TrimSpace(e.Name), want.Name}, {"brand", strings.TrimSpace(e.Brand), want.Brand},
		{"id", e.ID, want.DeviceID},
		{"kind", e.Kind, want.RecordKind}, {"icon", e.Icon, want.Icon}, {"sort", e.Sort, want.Sort},
		{"class", byte(e.Class), want.CodeType}, {"type", e.Type, want.DeviceType},
		{"code id", hex.EncodeToString(e.CodeID[:]), want.CodeID}, {"hide", e.Hide, want.Hide},
		{"input flag", e.InputFlag, want.InputFlag}, {"channel", e.Channel, want.Channel},
		{"power state", e.PowerState, want.PowerState}, {"tail", tail, wantTail},
	} {
		if f.got != f.want {
			t.Errorf("case %d (%v): %s = %+v, upstream %+v", i, v, f.name, f.got, f.want)
		}
	}
}

// withoutHalfPairs is upstream's payload with the lone high surrogate it
// writes when a UTF-16 label is cut mid-pair zeroed, and the checksum
// redone. Upstream's device-record writer cuts bytes; we never write half a
// pair (its own label writer for commands and macros does not either).
// Upstream also trims whitespace from decoded labels, which we keep.
func withoutHalfPairs(t *testing.T, v wire.Variant, payloadHex string) string {
	t.Helper()
	p, _ := hex.DecodeString(payloadHex)
	s, _ := v.Schema()
	if s.Labels != wire.UTF16BE {
		return payloadHex
	}
	body := p[3:]
	for _, start := range []int{offName, offName + s.DeviceSlotLen} {
		slot := body[start : start+s.DeviceSlotLen]
		for i := 0; i+1 < len(slot); i += 2 {
			u := uint16(slot[i])<<8 | uint16(slot[i+1])
			if u >= 0xD800 && u < 0xDC00 {
				if i+3 < len(slot) {
					i += 2 // a whole pair
					continue
				}
				slot[i], slot[i+1] = 0, 0
			}
		}
	}
	body[len(body)-1] = wire.Checksum(body[:len(body)-1])
	return hex.EncodeToString(p)
}
