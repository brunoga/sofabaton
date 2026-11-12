package hub_test

import (
	"bytes"
	"context"
	"net/netip"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
)

// The setup is laid out as the one our X2 took (2026-11-12), with another
// broker, MAC and credentials.
func TestMQTTPayload(t *testing.T) {
	b := hub.MQTTBroker{Addr: netip.MustParseAddrPort("10.0.0.2:1883"), User: "ab", Password: "xyz"}
	got, err := hub.MQTTPayload(b, [6]byte{1, 2, 3, 4, 5, 6})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0, 24, // what follows
		10, 0, 0, 2, 0x07, 0x5B, // the broker
		1, 2, 3, 4, 5, 6, // the hub's MAC
		2, 0, 'a', 0, 'b',
		3, 0, 'x', 0, 'y', 0, 'z',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("% x\nwant % x", got, want)
	}
	for _, bad := range []hub.MQTTBroker{
		{Addr: netip.MustParseAddrPort("[::1]:1883"), User: "a", Password: "b"},
		{Addr: netip.MustParseAddrPort("10.0.0.2:0"), User: "a", Password: "b"},
		{Addr: netip.MustParseAddrPort("10.0.0.2:1883"), Password: "b"},
	} {
		if _, err := hub.MQTTPayload(bad, [6]byte{}); err == nil {
			t.Errorf("%+v taken", bad)
		}
	}
}

func TestMQTT(t *testing.T) {
	ctx := context.Background()
	m := model(t)
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if ok, err := c.MQTTConfigured(ctx); ok || err != nil {
		t.Fatalf("before: %v %v", ok, err)
	}
	if err := c.SetMQTT(ctx, hub.MQTTBroker{Addr: netip.MustParseAddrPort("10.0.0.2:1883"), User: "a", Password: "b"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.MQTTConfigured(ctx); !ok || err != nil || m.MQTT == nil {
		t.Fatalf("set: %v %v", ok, err)
	}
	if err := c.ClearMQTT(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.MQTTConfigured(ctx); ok || err != nil {
		t.Fatalf("cleared: %v %v", ok, err)
	}
}
