package hub_test

import (
	"context"
	"errors"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// matterReply is laid out as our X2's (2026-11-12), with the Matter
// specification's example code in place of its own.
var matterReply = append(append([]byte{22}, "MT:Y.K9042C00KA0648G00"...), "\r3497-011-2332"...)

func TestParseMatterCode(t *testing.T) {
	got, err := hub.ParseMatterCode(matterReply)
	if err != nil || got != (hub.MatterCode{QR: "MT:Y.K9042C00KA0648G00", Manual: "3497-011-2332"}) {
		t.Fatalf("%+v, %v", got, err)
	}
	for _, bad := range [][]byte{nil, []byte("MT:ABC"), []byte("XX\r123"), []byte("MT:ABC\r")} {
		if _, err := hub.ParseMatterCode(bad); err == nil {
			t.Errorf("%q taken", bad)
		}
	}
}

// The setup code is read; the switches' device is not found until a Home
// app has added the hub, then read.
func TestMatter(t *testing.T) {
	ctx := context.Background()
	m := model(t)
	m.MatterCode = matterReply
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	code, err := c.MatterCode(ctx)
	if err != nil || code.QR != "MT:Y.K9042C00KA0648G00" {
		t.Fatalf("%+v, %v", code, err)
	}
	if _, err := c.MatterDevice(ctx); !errors.Is(err, hub.ErrNotFound) {
		t.Fatalf("before: %v, want ErrNotFound", err)
	}
	e, err := record.NewEntity(wire.X2)
	if err != nil {
		t.Fatal(err)
	}
	e.ID, e.Name = 51, "Matter"
	m.MatterDevice = e
	got, err := c.MatterDevice(ctx)
	if err != nil || got.ID != 51 || got.Name != "Matter" {
		t.Fatalf("after: %+v, %v", got, err)
	}
}
