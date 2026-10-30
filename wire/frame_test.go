package wire

import (
	"bytes"
	"errors"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
)

func TestFrameMarshal(t *testing.T) {
	tests := []struct {
		name  string
		frame Frame
		want  []byte
	}{
		{
			name:  "request activities, empty payload",
			frame: Frame{Family: FamReqActivities},
			want:  []byte{0xA5, 0x5A, 0x00, 0x3A, 0x39},
		},
		{
			name:  "request buttons of activity 0x65",
			frame: Frame{Family: FamReqButtons, Payload: []byte{0x65, 0xFF}},
			want:  []byte{0xA5, 0x5A, 0x02, 0x3C, 0x65, 0xFF, 0xA1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.frame.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary = % x, want % x", got, tt.want)
			}
			var back Frame
			if err := back.UnmarshalBinary(got); err != nil {
				t.Fatal(err)
			}
			if back.Family != tt.frame.Family || !bytes.Equal(back.Payload, tt.frame.Payload) {
				t.Fatalf("round trip = %v, want %v", back, tt.frame)
			}
		})
	}
}

func TestFrameOpcode(t *testing.T) {
	f := Frame{Family: FamCommandPage, Payload: make([]byte, 0x4D)}
	if got := f.Opcode(); got != 0x4D5D {
		t.Fatalf("Opcode = %v, want 0x4D5D", got)
	}
	if got := f.Opcode().PayloadLen(); got != 0x4D {
		t.Fatalf("PayloadLen = %d", got)
	}
	if got := f.Opcode().Family(); got != FamCommandPage {
		t.Fatalf("Family = %v", got)
	}
	if f.Len() != 0x4D+5 {
		t.Fatalf("Len = %d", f.Len())
	}
}

func TestFramePayloadTooLong(t *testing.T) {
	f := Frame{Family: FamWriteCommand, Payload: make([]byte, MaxPayload+1)}
	if _, err := f.MarshalBinary(); !errors.Is(err, ErrPayloadTooLong) {
		t.Fatalf("MarshalBinary error = %v, want ErrPayloadTooLong", err)
	}
	if _, err := NewOpcode(FamWriteCommand, MaxPayload+1); !errors.Is(err, ErrPayloadTooLong) {
		t.Fatalf("NewOpcode error = %v, want ErrPayloadTooLong", err)
	}
	full := Frame{Family: FamWriteCommand, Payload: make([]byte, MaxPayload)}
	b, err := full.MarshalBinary()
	if err != nil || len(b) != MaxFrameLen {
		t.Fatalf("max payload: %d bytes, %v", len(b), err)
	}
}

func TestParseErrors(t *testing.T) {
	good := []byte{0xA5, 0x5A, 0x02, 0x3C, 0x65, 0xFF, 0xA1}
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", nil, ErrShort},
		{"one byte", good[:1], ErrShort},
		{"header cut", good[:3], ErrShort},
		{"payload cut", good[:6], ErrShort},
		{"no sync", []byte{0x5A, 0xA5, 0x00, 0x3A, 0x79}, ErrSync},
		{"bad checksum", append(append([]byte(nil), good[:6]...), 0x00), ErrChecksum},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := Parse(tt.data); !errors.Is(err, tt.want) {
				t.Fatalf("Parse error = %v, want %v", err, tt.want)
			}
		})
	}
	var f Frame
	if err := f.UnmarshalBinary(append(append([]byte(nil), good...), 0)); err == nil {
		t.Fatal("UnmarshalBinary accepted trailing bytes")
	}
}

func TestParseCopiesPayload(t *testing.T) {
	data := []byte{0xA5, 0x5A, 0x02, 0x3C, 0x65, 0xFF, 0xA1}
	f, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	data[4] = 0
	if f.Payload[0] != 0x65 {
		t.Fatal("Payload aliases the input")
	}
}

// Every complete frame captured upstream must parse and marshal back to the
// same bytes.
func TestUpstreamCaptures(t *testing.T) {
	caps := testcorpus.Complete(t)
	if len(caps) < 100 {
		t.Fatalf("only %d complete captures; corpus missing or truncated", len(caps))
	}
	for _, c := range caps {
		raw := c.Bytes()
		f, n, err := Parse(raw)
		if err != nil || n != len(raw) {
			t.Errorf("%s: Parse = %d bytes, %v", c.Name(), n, err)
			continue
		}
		back, err := f.MarshalBinary()
		if err != nil || !bytes.Equal(back, raw) {
			t.Errorf("%s: round trip differs", c.Name())
		}
	}
}

func TestChecksum(t *testing.T) {
	if got := Checksum([]byte{0xA5, 0x5A, 0x00, 0x3A}); got != 0x39 {
		t.Fatalf("Checksum = %#x, want 0x39", got)
	}
	if got := Checksum(nil); got != 0 {
		t.Fatalf("Checksum(nil) = %#x", got)
	}
}

func FuzzParse(f *testing.F) {
	for _, c := range testcorpus.Complete(f) {
		f.Add(c.Bytes())
	}
	f.Add([]byte{0xA5, 0x5A, 0xFF})
	f.Fuzz(func(t *testing.T, data []byte) {
		fr, n, err := Parse(data)
		if err != nil {
			return
		}
		if n > len(data) || n != fr.Len() {
			t.Fatalf("consumed %d of %d, frame length %d", n, len(data), fr.Len())
		}
		back, err := fr.MarshalBinary()
		if err != nil || !bytes.Equal(back, data[:n]) {
			t.Fatalf("re-marshal = % x, want % x (%v)", back, data[:n], err)
		}
	})
}
