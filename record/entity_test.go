package record

import (
	"bytes"
	"encoding/hex"
	"net/netip"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
	"github.com/brunoga/sofabaton/wire"
)

// The X1 create and update frames of a Bose IR device, as the app sent them.
const (
	boseCreate = "tests/test_device_create.py:94"
	boseUpdate = "tests/test_device_create.py:104"
)

func TestEntityBoseCreate(t *testing.T) {
	payload := testcorpus.BySource(t, boseCreate).Payload()
	e, err := ParseEntity(wire.X1, payload[3:])
	if err != nil {
		t.Fatal(err)
	}
	wantCode, _ := hex.DecodeString("3fb31f0f15c24cccb3c3c4f7db3cb4d8")
	if e.ID != NewID || e.Name != "Bose" || e.Brand != "Bose" || e.Icon != 0x16 ||
		e.Class != ClassIR || e.Type != 0x03 || !bytes.Equal(e.CodeID[:], wantCode) {
		t.Fatalf("decoded %+v", e)
	}
	tail, err := e.DeviceTail()
	if err != nil {
		t.Fatal(err)
	}
	want := DeviceTail{HasPollTime: true, InputMode: 2, PowerStyle: 1}
	if tail != want {
		t.Fatalf("tail = %+v, want %+v", tail, want)
	}

	got, err := e.WritePayload()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("WritePayload differs from the app's create:\n got % x\nwant % x", got, payload)
	}
}

// The app's update write is the create with the assigned id and the tail
// marker set; building it from the create must give the app's bytes.
func TestEntityBoseUpdateFromCreate(t *testing.T) {
	create := testcorpus.BySource(t, boseCreate).Payload()
	update := testcorpus.BySource(t, boseUpdate).Payload()

	e, err := ParseEntity(wire.X1, create[3:])
	if err != nil {
		t.Fatal(err)
	}
	e.ID = 0x0D
	tail, _ := e.DeviceTail()
	tail.Marker = 1
	if err := e.SetDeviceTail(tail); err != nil {
		t.Fatal(err)
	}
	got, err := e.WritePayload()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, update) {
		t.Fatalf("update differs:\n got % x\nwant % x", got, update)
	}
}

// A record built from scratch with the same fields gives the same bytes as
// one decoded from the capture.
func TestEntityFromScratch(t *testing.T) {
	payload := testcorpus.BySource(t, boseCreate).Payload()
	e, err := NewEntity(wire.X1)
	if err != nil {
		t.Fatal(err)
	}
	e.Name, e.Brand, e.Icon, e.Class, e.Type = "Bose", "Bose", 0x16, ClassIR, 0x03
	code, _ := hex.DecodeString("3fb31f0f15c24cccb3c3c4f7db3cb4d8")
	copy(e.CodeID[:], code)
	if err := e.SetDeviceTail(DeviceTail{HasPollTime: true, InputMode: 2, PowerStyle: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := e.WritePayload()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("from scratch:\n got % x\nwant % x", got, payload)
	}
}

func TestCatalogRowX1Roku(t *testing.T) {
	c := testcorpus.BySource(t, "tests/test_opcode_handlers.py:1061")
	row, err := ParseCatalogRow(wire.X1, c.Payload())
	if err != nil {
		t.Fatal(err)
	}
	e := row.Entity
	if row.Index != 9 || row.Total != 9 || e.ID != 0x01 || e.Class != ClassRoku ||
		e.Name != "Roku" || e.Brand != "Streaming Stick 4K" {
		t.Fatalf("row %+v entity %+v", row, e)
	}
	tail, err := e.DeviceTail()
	if err != nil {
		t.Fatal(err)
	}
	want := DeviceTail{
		IP:          netip.MustParseAddr("192.168.1.128"),
		HasPollTime: true,
		InputMode:   1, PowerMode: 1, PowerStyle: 2,
		Flag: 1, Marker: 1,
	}
	if tail != want {
		t.Fatalf("tail = %+v, want %+v", tail, want)
	}
	if e.Channel != 0x80 {
		t.Fatalf("channel = %#x, want the IP's last octet", e.Channel)
	}
}

func TestCatalogRowsX1SActivities(t *testing.T) {
	for _, tt := range []struct {
		src  string
		id   byte
		name string
	}{
		{"tests/test_opcode_handlers.py:1362", 0x66, "Watch Apple TV"},
		{"tests/test_opcode_handlers.py:1367", 0x67, "PC"},
		{"tests/test_opcode_handlers.py:1382", 0x67, "Play Switch 2"},
		{"tests/test_opcode_handlers.py:1387", 0x68, "Play Xbox"},
		{"tests/test_opcode_handlers.py:1392", 0x69, "Play Playstation 5"},
	} {
		row, err := ParseCatalogRow(wire.X1S, testcorpus.BySource(t, tt.src).Payload())
		if err != nil {
			t.Fatalf("%s: %v", tt.src, err)
		}
		if row.Entity.ID != tt.id || row.Entity.Name != tt.name || !IsActivityID(row.Entity.ID) {
			t.Errorf("%s: id %#x name %q, want %#x %q", tt.src, row.Entity.ID, row.Entity.Name, tt.id, tt.name)
		}
	}
}

// Every complete catalog row and create/update write in the corpus must
// re-encode to its own bytes, apart from the header (a row carries its row
// count where a write has the page marker) and the checksum, which the hub
// does not keep current.
func TestEntityRoundTripCorpus(t *testing.T) {
	n := 0
	for _, c := range testcorpus.Complete(t) {
		raw := c.Bytes()
		fam := wire.Family(raw[3])
		if fam != wire.FamDeviceRow && fam != wire.FamActivityRow && fam != wire.FamCreateDevice && fam != wire.FamUpdateDevice {
			continue
		}
		p := c.Payload()
		v := wire.X1S
		if len(p) == 123 {
			v = wire.X1
		}
		body := p[3:]
		e, err := ParseEntity(v, body)
		if err != nil {
			t.Errorf("%s: %v", c.Name(), err)
			continue
		}
		got, err := e.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got[3:len(got)-1], body[3:len(body)-1]) {
			t.Errorf("%s: re-encoded body differs", c.Name())
		}
		if got[len(got)-1] != wire.Checksum(got[:len(got)-1]) {
			t.Errorf("%s: checksum not recomputed", c.Name())
		}
		n++
	}
	if n < 8 {
		t.Fatalf("only %d entity frames in the corpus", n)
	}
}

func TestEntityEditsKeepUnknownBytes(t *testing.T) {
	s, _ := wire.X2.Schema()
	body := make([]byte, s.DeviceBodyLen)
	tailStart := offName + 2*s.DeviceSlotLen
	body[tailStart+40] = 0xAB // beyond every decoded tail field
	copy(body[offName:], wire.UTF16BE.Slot("TV", s.DeviceSlotLen))
	body[offName+10] = 0xEE // junk after the name's terminator

	e, err := ParseEntity(wire.X2, body)
	if err != nil {
		t.Fatal(err)
	}
	e.Brand = "Sony"
	if err := e.SetDeviceTail(DeviceTail{InputMode: 1, Color: 5, HasExtra: true, Extra: [3]byte{0, 0, 0x07}}); err != nil {
		t.Fatal(err)
	}
	out, err := e.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if out[tailStart+40] != 0xAB {
		t.Fatal("byte beyond the tail fields lost")
	}
	if out[offName+10] != 0xEE {
		t.Fatal("unchanged name slot rewritten")
	}
	back, _ := ParseEntity(wire.X2, out)
	if back.Brand != "Sony" || back.Name != "TV" {
		t.Fatalf("labels %q %q", back.Name, back.Brand)
	}
	if r, ok := back.Emitters(); !ok || r != EmitHub|EmitPort1|EmitPort2 {
		t.Fatalf("emitters %v %v", r, ok)
	}
	tail, _ := back.DeviceTail()
	if tail.Color != 5 {
		t.Fatalf("color %d", tail.Color)
	}

	e.Name = "Living room"
	out, _ = e.MarshalBinary()
	back, _ = ParseEntity(wire.X2, out)
	if back.Name != "Living room" {
		t.Fatalf("renamed to %q", back.Name)
	}
}

func TestEntityErrors(t *testing.T) {
	if _, err := ParseEntity(wire.X2, make([]byte, 120)); err == nil {
		t.Fatal("X2 accepted a 120-byte body")
	}
	if _, err := ParseEntity(wire.Variant(9), make([]byte, 210)); err == nil {
		t.Fatal("unknown variant accepted")
	}
	e, _ := NewEntity(wire.X2)
	e.Tail = make([]byte, 3)
	if _, err := e.MarshalBinary(); err == nil {
		t.Fatal("short tail accepted")
	}
	if err := (DeviceTail{IP: netip.MustParseAddr("::1")}).Put(make([]byte, 60)); err == nil {
		t.Fatal("IPv6 accepted")
	}
	if _, err := ParseCatalogRow(wire.X2, []byte{1, 0}); err == nil {
		t.Fatal("2-byte row accepted")
	}
}

func FuzzParseEntity(f *testing.F) {
	for _, c := range testcorpus.Complete(f) {
		if p := c.Payload(); len(p) == 213 || len(p) == 123 {
			f.Add(p[3:])
		}
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		for _, v := range []wire.Variant{wire.X1, wire.X2} {
			e, err := ParseEntity(v, body)
			if err != nil {
				continue
			}
			_, _ = e.DeviceTail()
			out, err := e.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			// Unchanged, a record re-encodes to its own bytes apart from
			// the header and the checksum.
			if !bytes.Equal(out[3:len(out)-1], body[3:len(body)-1]) {
				t.Fatalf("%v: round trip differs", v)
			}
		}
	})
}

func FuzzDeviceTail(f *testing.F) {
	f.Add(make([]byte, 60))
	f.Add(make([]byte, 30))
	f.Fuzz(func(t *testing.T, tail []byte) {
		d, err := ParseDeviceTail(tail)
		if err != nil {
			return
		}
		buf := append([]byte(nil), tail...)
		if err := d.Put(buf); err != nil {
			t.Fatal(err)
		}
		again, err := ParseDeviceTail(buf)
		if err != nil || again != d {
			t.Fatalf("decode(put(%+v)) = %+v, %v", d, again, err)
		}
	})
}
