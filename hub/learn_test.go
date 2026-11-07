package hub_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/capture"
	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// learnedFrames is what our X2 sent the app for the code it learned, the
// Receiver's Up (testdata/x2/app-learn-ir.jsonl), and what the app wrote.
func learnedFrames(t *testing.T) (learned, written []wire.Frame) {
	t.Helper()
	f, err := os.Open("../testdata/x2/app-learn-ir.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := capture.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsNote() {
			continue
		}
		fr, err := e.Frame()
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case e.Dir == wire.FromHub && fr.Family == wire.FamLearnData:
			learned = append(learned, fr)
		case e.Dir == wire.ToHub && fr.Family == wire.FamWriteCommand:
			written = append(written, fr)
		}
	}
	return learned, written
}

// TestLearnedCode checks the code our X2 learned: its pages are a command
// write, as the app's write of it, and its data the Receiver's Up as
// timings, the Denon code F:27 twice.
func TestLearnedCode(t *testing.T) {
	learned, written := learnedFrames(t)
	if len(learned) != 4 || len(written) != 4 {
		t.Fatalf("%d learned pages and %d written, want 4 and 4", len(learned), len(written))
	}
	body := func(frames []wire.Frame) (*record.Command, []byte) {
		var b wire.Burst
		for _, f := range frames {
			if _, err := b.Add(f.Payload); err != nil {
				t.Fatal(err)
			}
		}
		recs, err := b.Records()
		if err != nil || len(recs) != 1 {
			t.Fatalf("%d records, %v", len(recs), err)
		}
		_, c, data, err := record.ParseCommandWrite(wire.X2, recs[0])
		if err != nil {
			t.Fatal(err)
		}
		return c, data
	}
	lc, ld := body(learned)
	wc, wd := body(written)
	if lc.Device != 0 || lc.Type != 0 || lc.Label != "" {
		t.Errorf("learned command: device %d, type %v, label %q", lc.Device, lc.Type, lc.Label)
	}
	if wc.Device != 7 || wc.Type != record.ClassIR || wc.Label != "receiver up" {
		t.Errorf("written command: device %d, type %v, label %q", wc.Device, wc.Type, wc.Label)
	}
	if !bytes.Equal(ld, wd) {
		t.Error("the app wrote other data than the hub learned")
	}
	raw, err := record.ParseRawIR(ld)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Carrier != 38659 || len(raw.Timings) != 200 || raw.Timings[0] != 3311 || raw.Timings[1] != 1680 {
		t.Errorf("carrier %d, %d timings, starting %v", raw.Carrier, len(raw.Timings), raw.Timings[:2])
	}
}

func TestLearn(t *testing.T) {
	learned, _ := learnedFrames(t)
	var b wire.Burst
	for _, f := range learned {
		if _, err := b.Add(f.Payload); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := b.Records()
	if err != nil {
		t.Fatal(err)
	}
	_, _, code, err := record.ParseCommandWrite(wire.X2, recs[0])
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := model(t)
	m.Learned, m.LearnFailures = code, 2
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	failed := 0
	data, err := c.Learn(ctx, func() { failed++ })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, code) || failed != 2 {
		t.Errorf("learned %d bytes (want %d), %d failures (want 2)", len(data), len(code), failed)
	}

	// A window that closes is armed again, until ctx is done; then Learn
	// leaves learning.
	m.Learned, m.LearnFailures = nil, 0
	short, cancelShort := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancelShort()
	if _, err := c.Learn(short, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("no code: %v, want the deadline", err)
	}
	// A read after it, answered once the simulator has taken the 0x05.
	if _, err := c.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	if m.LearnArms < 2 || !m.LearnLeft {
		t.Errorf("armed %d times, left %v; want 2 or more, and left", m.LearnArms, m.LearnLeft)
	}
}
