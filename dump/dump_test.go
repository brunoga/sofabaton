package dump_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

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
