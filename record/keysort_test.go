package record

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/brunoga/sofabaton/wire"
)

// The X2's order of its Bluetooth device (2026-10-31): 25 commands, none
// placed. Its IR devices have no order at all (status 0x07).
const x2Order = "0100010101ff02ff03ff04ff05ff06ff07ff08ff09ff0aff0bff0cff0dff0eff0fff10ff11ff12ff13ff14ff15ff16ff17ff18ff19ff"

func TestOrder(t *testing.T) {
	body, _ := hex.DecodeString(x2Order)
	dev, order, err := ParseOrder(body)
	if err != nil || dev != 1 || len(order) != 25 {
		t.Fatalf("device %d, %d places, %v", dev, len(order), err)
	}
	for i, p := range order {
		if p.Command != byte(i+1) || p.Placed() {
			t.Fatalf("place %d: %+v", i, p)
		}
	}
	pages := OrderPages(dev, order)
	if len(pages) != 1 {
		t.Fatalf("%d pages", len(pages))
	}
	want := append([]byte{0x01, 0x00, 0x01}, body...)
	if p := pages[0]; !bytes.Equal(p[:len(p)-1], want) || p[len(p)-1] != wire.Checksum(body) {
		t.Fatalf("write % x", p)
	}
	if _, _, err := ParseOrder(body[:5]); err == nil {
		t.Fatal("odd order accepted")
	}
	if !(Place{Command: 1, Position: 3}).Placed() || (Place{Command: 1}).Placed() {
		t.Fatal("Placed")
	}
}
