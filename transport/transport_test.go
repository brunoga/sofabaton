package transport

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/internal/testcorpus"
	"github.com/brunoga/sofabaton/wire"
)

var loopback = netip.MustParseAddr("127.0.0.1")

// fakeHub answers CALL_ME on a UDP port by connecting back, dropping the
// first drop requests, and serves each connection with serve.
func fakeHub(t *testing.T, drop int32, serve func(*Conn)) (port int) {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	var seen atomic.Int32
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			f, _, err := wire.Parse(buf[:n])
			if err != nil || f.Family != wire.FamCallMe {
				continue
			}
			if seen.Add(1) <= drop {
				continue
			}
			addr, err := ParseCallMe(f.Payload)
			if err != nil {
				continue
			}
			back := netip.AddrPortFrom(from.(*net.UDPAddr).AddrPort().Addr(), addr.Port())
			c, err := net.Dial("tcp4", back.String())
			if err != nil {
				continue
			}
			go serve(NewConn(c))
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func TestDial(t *testing.T) {
	banner := testcorpus.BySource(t, "tests/lib/test_aio_real_engine.py:1642").Payload()
	port := fakeHub(t, 1, func(c *Conn) {
		defer c.Close()
		req, err := c.ReadFrame()
		if err != nil || req.Family != wire.FamReqBanner {
			return
		}
		_ = c.WriteFrame(wire.Frame{Family: wire.FamBanner, Payload: banner})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := Dialer{HubPort: port, ListenPort: -1, Retry: 50 * time.Millisecond}
	c, err := d.Dial(ctx, loopback)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.WriteFrame(wire.Frame{Family: wire.FamReqBanner}); err != nil {
		t.Fatal(err)
	}
	f, err := c.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if f.Family != wire.FamBanner || len(f.Payload) != len(banner) {
		t.Fatalf("got %v", f)
	}
}

func TestDialTimeout(t *testing.T) {
	port := fakeHub(t, 1<<30, nil) // never connects back
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	d := Dialer{HubPort: port, ListenPort: -1, Retry: 20 * time.Millisecond}
	if _, err := d.Dial(ctx, loopback); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Dial error = %v", err)
	}
}

func TestDialPortRange(t *testing.T) {
	// Occupy a port and ask for a range starting at it: the next is used.
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	first := busy.Addr().(*net.TCPAddr).Port
	d := Dialer{ListenPort: first, ListenPorts: 4}
	ln, err := d.listen(context.Background(), loopback)
	if err != nil {
		t.Skipf("no free port after %d: %v", first, err)
	}
	defer ln.Close()
	if got := ln.Addr().(*net.TCPAddr).Port; got == first || got > first+3 {
		t.Fatalf("listened on %d, range %d+4", got, first)
	}
	if _, err := (&Dialer{ListenPort: first, ListenPorts: 1}).listen(context.Background(), loopback); err == nil {
		t.Fatal("a range of one busy port succeeded")
	}
}

func TestCallMe(t *testing.T) {
	addr := netip.MustParseAddrPort("192.168.1.100:8200")
	f, err := CallMe(addr)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := f.MarshalBinary()
	// Layout from the protocol docs: 6 zero bytes, IP, port.
	want := []byte{0xA5, 0x5A, 0x0C, 0xC3, 0, 0, 0, 0, 0, 0, 0xC0, 0xA8, 0x01, 0x64, 0x20, 0x08}
	if string(raw[:len(want)]) != string(want) || raw[len(raw)-1] != wire.Checksum(raw[:len(raw)-1]) {
		t.Fatalf("CALL_ME = % x", raw)
	}
	back, err := ParseCallMe(f.Payload)
	if err != nil || back != addr {
		t.Fatalf("ParseCallMe = %v, %v", back, err)
	}
	if _, err := CallMe(netip.MustParseAddrPort("[::1]:8200")); err == nil {
		t.Fatal("IPv6 accepted")
	}
	if _, err := ParseCallMe(make([]byte, 11)); err == nil {
		t.Fatal("11-byte payload accepted")
	}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func TestDiscover(t *testing.T) {
	replyPort := freeUDPPort(t)
	hub, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	reply, _ := (wire.Frame{
		Family:  wire.FamNotifyReply,
		Payload: testcorpus.BySource(t, "tests/test_notify_demuxer.py:125").Payload(),
	}).MarshalBinary()
	go func() {
		buf := make([]byte, 64)
		for {
			n, _, err := hub.ReadFrom(buf)
			if err != nil {
				return
			}
			if f, _, err := wire.Parse(buf[:n]); err == nil && f.Family == wire.FamNotifyMe {
				// Hubs broadcast their reply to the reply port; answer twice
				// to check that a hub is reported once.
				to := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: replyPort}
				_, _ = hub.WriteTo(reply, to)
				_, _ = hub.WriteTo(reply, to)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	d := Discoverer{
		Targets:   []netip.Addr{loopback},
		HubPort:   hub.LocalAddr().(*net.UDPAddr).Port,
		ReplyPort: replyPort,
		Retry:     100 * time.Millisecond,
	}
	found, err := d.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Info.Name != "X2 HUB" || found[0].Addr != loopback {
		t.Fatalf("found %+v", found)
	}
}
