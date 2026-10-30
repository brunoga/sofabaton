package record

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
	"github.com/brunoga/sofabaton/wire"
)

func TestHubInfoCorpus(t *testing.T) {
	for _, tt := range []struct {
		src      string
		fam      wire.Family
		id       string
		marker   byte
		variant  wire.Variant
		batch    string
		firmware byte
		name     string
	}{
		{"tests/lib/test_aio_real_engine.py:1642", wire.FamBanner, "e26a44861b45", 0x00, wire.X1S, "2022-11-20", 5, "X1 HUB bench230"},
		{"tests/test_notify_demuxer.py:106", wire.FamNotifyReply, "e26a44861b45", 0x64, wire.X1S, "2022-11-20", 5, "Souterrain hub"},
		{"tests/test_notify_demuxer.py:125", wire.FamNotifyReply, "fc012c39d390", 0x64, wire.X2, "2022-11-20", 8, "X2 HUB"},
		{"tests/test_notify_demuxer.py:87", wire.FamNotifyReply, "cb383539684b", 0x64, wire.X1, "2021-06-09", 17, "X1 HUB test"},
	} {
		c := testcorpus.BySource(t, tt.src)
		if wire.Family(c.Bytes()[3]) != tt.fam {
			t.Fatalf("%s: family %#x", tt.src, c.Bytes()[3])
		}
		h, err := ParseHubInfo(c.Payload())
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(h.ID[:]) != tt.id || h.Marker != tt.marker || h.Variant != tt.variant ||
			h.BatchDate() != tt.batch || h.Firmware != tt.firmware || h.Name != tt.name {
			t.Errorf("%s: %+v (batch %s)", tt.src, h, h.BatchDate())
		}
		if got := h.AppendPayload(nil); !bytes.Equal(got, c.Payload()) {
			t.Errorf("%s: re-encoded payload differs", tt.src)
		}
	}
}

func TestHubInfoOddInput(t *testing.T) {
	if _, err := ParseHubInfo(make([]byte, 14)); err == nil {
		t.Fatal("14-byte hub info accepted")
	}
	p := make([]byte, 15)
	p[7] = 0x09 // a model we do not know
	p[8] = 0xAB // not BCD
	h, err := ParseHubInfo(append(p, 0xC4, 0xE3))
	if err != nil {
		t.Fatal(err)
	}
	if h.Variant != 0 || h.Model != 0x09 || h.BatchDate() != "" || h.Name != "Äã" {
		t.Fatalf("%+v", h)
	}
}

func FuzzParseHubInfo(f *testing.F) {
	f.Add(testcorpus.BySource(f, "tests/test_notify_demuxer.py:125").Payload())
	f.Fuzz(func(t *testing.T, p []byte) {
		h, err := ParseHubInfo(p)
		if err != nil {
			return
		}
		_ = h.BatchDate()
		if got := h.AppendPayload(nil); !bytes.Equal(got, p) {
			t.Fatal("round trip differs")
		}
	})
}
