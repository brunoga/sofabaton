package main

import (
	"bytes"
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/irsource"
	"github.com/brunoga/sofabaton/record"
)

// sbx firmware checks the server for the hub, and installs what it
// offers; the hub comes back with it.
func TestFirmware(t *testing.T) {
	m := &hubsim.Model{Info: &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 8, Name: "X2 HUB"}}
	prev := dialHub
	dialHub = func(ctx context.Context, _ netip.Addr, opts hub.Options) (*hub.Client, error) {
		if m.Firmware != nil {
			m.Info.Firmware = 9 // restarted with it
		}
		return hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), opts)
	}
	image := bytes.Repeat([]byte("firmware"), 1000)
	offer := &irsource.Firmware{Product: "x2", Version: 9, Notes: "Fixes", URLs: []string{"https://x/hub.bin"}}
	prevFW, prevImage, prevPoll := hubFirmware, hubImage, rebootPoll
	hubFirmware = func(_ context.Context, key string, info *record.HubInfo) (*irsource.Firmware, error) {
		if key != "K" {
			t.Errorf("key %q", key)
		}
		if info.Firmware >= 9 {
			return nil, nil
		}
		return offer, nil
	}
	hubImage = func(context.Context, string, *irsource.Firmware) ([]byte, error) { return image, nil }
	rebootPoll = time.Millisecond
	t.Cleanup(func() { dialHub, hubFirmware, hubImage, rebootPoll = prev, prevFW, prevImage, prevPoll })

	var out, errs bytes.Buffer
	sbx := func(stdin string, args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(stdin), append([]string{"firmware", "-hub", "192.0.2.1", "-key", "K"}, args...), &out, &errs)
	}
	if err := sbx(""); err != nil || !strings.Contains(out.String(), "firmware 9 is available: Fixes") || m.Firmware != nil {
		t.Fatalf("check: %v\n%s", err, out.String())
	}
	if err := sbx("n\n", "-update"); err == nil || m.Firmware != nil {
		t.Fatalf("installed without a yes: %v\n%s", err, out.String())
	}
	if err := sbx("y\n", "-update"); err != nil || !bytes.Equal(m.Firmware, image) || !strings.Contains(out.String(), "sent 100%") || !strings.Contains(out.String(), "back with firmware 9") {
		t.Fatalf("update: %v\n%s", err, out.String())
	}
	if err := sbx(""); err != nil || !strings.Contains(out.String(), "the newest") {
		t.Fatalf("after: %v\n%s", err, out.String())
	}
}
