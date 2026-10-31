package capture

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/brunoga/sofabaton/wire"
)

// The Android app updating our X2's firmware through the proxy: one
// FirmwareStart acknowledged as number 0, chunks numbered from 1 each
// acknowledged with its number, except the last, which FirmwareDone
// answers (docs/protocol/firmware-update.md).
func TestX2FirmwareUpdate(t *testing.T) {
	f, err := os.Open("../testdata/x2/app-firmware-update.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	selector := []byte{0, 0, 1, 0, 0, 0}
	var starts, chunks, acks, done int
	for _, e := range entries {
		if e.Hex == "" {
			continue
		}
		fr, err := e.Frame()
		if err != nil {
			t.Fatalf("%s: %v", e.Hex, err)
		}
		if done > 0 && (fr.Family == wire.FamFirmwareChunk || fr.Family == wire.FamFirmwareAck) {
			t.Fatal("update traffic after FirmwareDone")
		}
		p := fr.Payload
		switch {
		case e.Dir == wire.ToHub && fr.Family == wire.FamFirmwareStart:
			starts++
			if len(p) != 26 || string(p[:6]) != string(selector) {
				t.Errorf("start % x", p)
			}
		case e.Dir == wire.ToHub && fr.Family == wire.FamFirmwareChunk:
			chunks++
			if n := binary.BigEndian.Uint16(p[7:9]); int(n) != chunks || acks != chunks {
				t.Errorf("chunk %d after %d chunks and %d acks", n, chunks-1, acks)
			}
		case e.Dir == wire.FromHub && fr.Family == wire.FamFirmwareAck:
			if n := binary.BigEndian.Uint16(p[6:8]); int(n) != chunks || acks != chunks {
				t.Errorf("ack %d after chunk %d", n, chunks)
			}
			acks++
		case e.Dir == wire.FromHub && fr.Family == wire.FamFirmwareDone:
			done++
			if string(p) != string(selector) || acks != chunks {
				t.Errorf("done % x", p)
			}
		}
	}
	if starts != 1 || chunks != 217 || acks != 217 || done != 1 {
		t.Fatalf("%d starts, %d chunks, %d acks, %d done", starts, chunks, acks, done)
	}
}
