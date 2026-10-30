package wire

import (
	"errors"
	"testing"
)

func TestVariants(t *testing.T) {
	for _, tt := range []struct {
		hver  string
		model byte
		want  Variant
		body  int
		enc   TextEncoding
	}{
		{"1", 0x01, X1, 120, ASCII},
		{"2", 0x02, X1S, 210, UTF16BE},
		{"3", 0x03, X2, 210, UTF16BE},
	} {
		v, err := ParseHVER(tt.hver)
		if err != nil || v != tt.want {
			t.Fatalf("ParseHVER(%q) = %v, %v", tt.hver, v, err)
		}
		if m, err := VariantFromModel(tt.model); err != nil || m != tt.want {
			t.Fatalf("VariantFromModel(%#x) = %v, %v", tt.model, m, err)
		}
		s, err := v.Schema()
		if err != nil || s.DeviceBodyLen != tt.body || s.Labels != tt.enc {
			t.Fatalf("%v schema = %+v, %v", v, s, err)
		}
		// The name, brand and tail slots plus the 29 bytes before them and
		// the checksum make up the body.
		if 29+3*s.DeviceSlotLen+1 != s.DeviceBodyLen {
			t.Fatalf("%v: slots do not add up to the body", v)
		}
	}
	if _, err := ParseHVER("4"); !errors.Is(err, ErrUnknownVariant) {
		t.Fatalf("ParseHVER(4): %v", err)
	}
	if _, err := Variant(0).Schema(); !errors.Is(err, ErrUnknownVariant) {
		t.Fatalf("Variant(0).Schema: %v", err)
	}
}

func TestFamilyNames(t *testing.T) {
	if got := FamLearnExit.Name(ToHub); got != "LearnExit" {
		t.Fatalf("0x05 to hub = %s", got)
	}
	if got := FamLearnExpired.Name(FromHub); got != "LearnExpired" {
		t.Fatalf("0x05 from hub = %s", got)
	}
	if got := Family(0xEE).Name(FromHub); got != "0xEE" {
		t.Fatalf("unknown family = %s", got)
	}
	if FamReqDevices.Known(FromHub) {
		t.Fatal("0x0A is not a hub-sent family")
	}
}
