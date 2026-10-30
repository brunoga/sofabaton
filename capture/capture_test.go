package capture

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/wire"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	clock := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	w.now = func() time.Time { clock = clock.Add(time.Millisecond); return clock }

	req := wire.Frame{Family: wire.FamReqDevices}
	ack := wire.Frame{Family: wire.FamStatusAck, Payload: []byte{0}}
	if err := w.Note("open the device list"); err != nil {
		t.Fatal(err)
	}
	if err := w.Frame("app", wire.ToHub, req); err != nil {
		t.Fatal(err)
	}
	if err := w.Frame("hub", wire.FromHub, ack); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"dir":"to-hub"`) || !strings.Contains(buf.String(), `"name":"StatusAck"`) {
		t.Fatalf("capture:\n%s", buf.String())
	}

	entries, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || !entries[0].IsNote() || entries[0].Note != "open the device list" {
		t.Fatalf("entries %+v", entries)
	}
	f, err := entries[2].Frame()
	if err != nil || f.Family != wire.FamStatusAck || entries[2].Dir != wire.FromHub || entries[2].Link != "hub" {
		t.Fatalf("frame %v %v %+v", f, err, entries[2])
	}
	if !entries[1].Time.After(entries[0].Time) {
		t.Fatal("times not recorded")
	}
}

func TestReadErrors(t *testing.T) {
	if _, err := Read(strings.NewReader(`{"t":"2026-10-30T12:00:00Z","dir":"sideways"}`)); err == nil {
		t.Fatal("unknown direction accepted")
	}
	if _, err := (Entry{Hex: "a55a"}).Frame(); err == nil {
		t.Fatal("cut frame accepted")
	}
}
