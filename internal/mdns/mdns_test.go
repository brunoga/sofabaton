package mdns

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"
)

// The X2's own advertisement, as avahi-browse reported it.
var x2 = Service{
	Instance: "X1-HUB-962c08",
	Type:     "_sofabaton_hub._udp",
	Host:     "X1-HUB-962c08.local",
	Addr:     netip.MustParseAddr("192.168.1.244"),
	Port:     8102,
	TXT:      []string{"HVER=3", "NO=20221120", "AVER=8", "NAME=X2 HUB", "MAC=80f1b2962c08"},
}

func TestAnswerRoundTrip(t *testing.T) {
	msg, err := x2.answer(false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseServices(msg, x2.Type)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%d services", len(got))
	}
	s := got[0]
	if s.Instance != x2.Instance || s.Host != x2.Host || s.Addr != x2.Addr || s.Port != x2.Port || len(s.TXT) != 5 {
		t.Fatalf("parsed %+v", s)
	}
	if v, ok := s.TXTValue("hver"); !ok || v != "3" {
		t.Fatalf("HVER = %q %v", v, ok)
	}
	if _, ok := s.TXTValue("missing"); ok {
		t.Fatal("missing key found")
	}
	if other, _ := parseServices(msg, "_x1hub._udp"); len(other) != 0 {
		t.Fatalf("another type matched: %+v", other)
	}
}

func TestAsksFor(t *testing.T) {
	q, err := query(x2.Type)
	if err != nil {
		t.Fatal(err)
	}
	if !asksFor(q, x2) {
		t.Fatal("PTR query for the type not recognized")
	}
	other, _ := query("_x1hub._udp")
	if asksFor(other, x2) {
		t.Fatal("query for another type answered")
	}
	ans, _ := x2.answer(false)
	if asksFor(ans, x2) {
		t.Fatal("a response treated as a query")
	}
	if _, err := (Service{Instance: "a", Type: "_b._udp", Addr: netip.MustParseAddr("::1")}).answer(false); err == nil {
		t.Fatal("IPv6 address accepted")
	}
}

func TestGoodbye(t *testing.T) {
	bye, err := x2.answer(true)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := parseServices(bye, x2.Type); len(got) != 1 {
		t.Fatalf("goodbye parses to %d services", len(got))
	}
}

func FuzzParseServices(f *testing.F) {
	msg, _ := x2.answer(false)
	f.Add(msg)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = parseServices(b, x2.Type)
		_ = asksFor(b, x2)
	})
}

// TestLive advertises a service and browses for it on the real network.
// It needs multicast, so it runs only with SOFABATON_MDNS_TEST=1.
func TestLive(t *testing.T) {
	if os.Getenv("SOFABATON_MDNS_TEST") == "" {
		t.Skip("SOFABATON_MDNS_TEST not set")
	}
	svc := x2
	svc.Instance, svc.Host = "sbx-test-proxy", "sbx-test-proxy.local"
	svc.Type = "_sbx_test._udp"
	r, err := Advertise(svc, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	found, err := Browse(ctx, svc.Type)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Instance != svc.Instance || found[0].Port != 8102 {
		t.Fatalf("found %+v", found)
	}
}
