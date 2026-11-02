package dump_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/dump"
	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

func simulated(t *testing.T) *hub.Client {
	t.Helper()
	tv, _ := record.NewEntity(wire.X2)
	tv.ID, tv.Name, tv.Class = 1, "TV", record.ClassIR
	act, _ := record.NewEntity(wire.X2)
	act.ID, act.Name = 0x65, "Watch TV"
	m := &hubsim.Model{
		Info:       &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 9, Name: "X2 HUB"},
		Devices:    []*record.Entity{tv},
		Activities: []*record.Entity{act},
		Members:    map[byte][]byte{0x65: {1}},
	}
	c, err := hub.Open(context.Background(), hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestReadProgress(t *testing.T) {
	c := simulated(t)
	var steps []string
	d, err := dump.ReadProgress(context.Background(), c, "test", nil, func(done, total int, name string) {
		steps = append(steps, fmt.Sprintf("%d/%d %s", done, total, name))
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(steps, "|"), "0/2 TV|1/2 Watch TV|2/2 "; got != want {
		t.Errorf("progress %q, want %q", got, want)
	}
	if len(d.Entities) != 2 {
		t.Errorf("%d entities, want 2", len(d.Entities))
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, err = dump.ReadProgress(ctx, c, "test", nil, func(done, _ int, _ string) {
		if done == 0 { // cancelled while reading the first
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want cancelled", err)
	}
}

// A request the hub refuses is kept in the dump; a hub that stops
// answering ends the read with an error, rather than with a dump missing
// what was not read.
func TestReadFailures(t *testing.T) {
	ctx := context.Background()
	m := func() *hubsim.Hub {
		tv, _ := record.NewEntity(wire.X2)
		tv.ID, tv.Name, tv.Class = 1, "TV", record.ClassIR
		return hubsim.FromModel(&hubsim.Model{
			Info:    &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 9, Name: "X2 HUB"},
			Devices: []*record.Entity{tv},
		})
	}

	h := m()
	h.Handle(wire.FamReqButtons, func(wire.Frame) []wire.Frame { return []wire.Frame{hubsim.Status(hubsim.NotFound)} })
	c, err := hub.Open(ctx, hubsim.Pipe(h), hub.Options{Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	d, err := dump.ReadProgress(ctx, c, "test", nil, nil)
	c.Close()
	if err != nil || d.Entities[0].Keymap.Error == "" {
		t.Errorf("refused keymap: %v, %+v", err, d.Entities[0].Keymap)
	}

	h = m()
	h.Handle(wire.FamReqButtons, func(wire.Frame) []wire.Frame { return nil })
	c, err = hub.Open(ctx, hubsim.Pipe(h), hub.Options{Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = dump.ReadProgress(ctx, c, "test", nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "keymap") {
		t.Errorf("unanswered keymap: got %v", err)
	}
	// The read stopped there: nothing after the keymap was asked for.
	for _, r := range h.Requests() {
		if r.Family == wire.FamReqMacros {
			t.Error("macros asked for after the hub stopped answering")
		}
	}
}

// Refresh reads again only the signals of commands whose record changed,
// and makes the same dump as a full read.
func TestRefresh(t *testing.T) {
	ctx := context.Background()
	tv, _ := record.NewEntity(wire.X2)
	tv.ID, tv.Name, tv.Class = 1, "TV", record.ClassIR
	h := hubsim.FromModel(&hubsim.Model{
		Info:    &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 9, Name: "X2 HUB"},
		Devices: []*record.Entity{tv},
	})
	c, err := hub.Open(ctx, hubsim.Pipe(h), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	write := func(id byte, label string, f int) {
		t.Helper()
		cmd, _ := record.NewCommand(wire.X2)
		cmd.Device, cmd.ID, cmd.Type, cmd.Label = 1, id, record.ClassIR, label
		data, _ := record.Descriptor(fmt.Sprintf("P:NEC R:38400 D:0 S:206 F:%d", f))
		if err := c.WriteCommand(ctx, cmd, data); err != nil {
			t.Fatal(err)
		}
	}
	write(1, "Power", 1)
	write(2, "Menu", 2)
	write(3, "Back", 3)
	old, err := dump.ReadProgress(ctx, c, "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	write(2, "Home", 9) // renamed and recoded

	blobs := func() int {
		n := 0
		for _, r := range h.Requests() {
			if r.Family == wire.FamReqBlob {
				n++
			}
		}
		return n
	}
	before := blobs()
	got, err := dump.Refresh(ctx, c, old, "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := blobs() - before; n != 1 {
		t.Errorf("read %d signals, want 1", n)
	}
	full, err := dump.ReadProgress(ctx, c, "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var a, b strings.Builder
	got.Write(&a)
	full.Write(&b)
	if a.String() != b.String() {
		t.Errorf("refresh:\n%s\nfull read:\n%s", a.String(), b.String())
	}

	other := *old
	other.Info.ID = "aabbccddeeff"
	if _, err := dump.Refresh(ctx, c, &other, "test", nil, nil); err == nil {
		t.Error("refreshed from another hub's dump")
	}
}
