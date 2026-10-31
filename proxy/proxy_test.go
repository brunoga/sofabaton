package proxy

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/internal/mdns"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/transport"
	"github.com/brunoga/sofabaton/wire"
)

var loopback = netip.MustParseAddr("127.0.0.1")

func udpPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

// fakeHub answers CALL_ME on a UDP port by connecting back and serving
// the connection with a simulated hub, one connection at a time, like a
// hub. Each connection is sent on conns, if not nil.
func fakeHub(t *testing.T, h *hubsim.Hub, conns chan<- net.Conn) int {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	var busy atomic.Bool
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			f, _, err := wire.Parse(buf[:n])
			if err != nil || f.Family != wire.FamCallMe || busy.Load() {
				continue
			}
			addr, _ := transport.ParseCallMe(f.Payload)
			c, err := net.Dial("tcp4", netip.AddrPortFrom(from.(*net.UDPAddr).AddrPort().Addr(), addr.Port()).String())
			if err != nil {
				continue
			}
			busy.Store(true)
			if conns != nil {
				conns <- c
			}
			go func() {
				_ = h.Serve(transport.NewConn(c))
				busy.Store(false)
			}()
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

type tapLog struct {
	mu     sync.Mutex
	frames []string
}

func (l *tapLog) tap(link string, dir wire.Direction, f wire.Frame) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.frames = append(l.frames, link+" "+dir.String()+" "+f.Family.Name(dir))
}

func (l *tapLog) has(s string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, f := range l.frames {
		if f == s {
			return true
		}
	}
	return false
}

func TestProxyRelaysTheApp(t *testing.T) {
	tv, _ := record.NewEntity(wire.X2)
	tv.ID, tv.Name = 1, "TV"
	h := hubsim.FromModel(&hubsim.Model{
		Info:    &record.HubInfo{ID: [6]byte{0x80, 0xf1, 0xb2, 0x96, 0x2c, 0x08}, Model: 3, Firmware: 8, Name: "X2 HUB"},
		Devices: []*record.Entity{tv},
	})
	appPort, replyPort := udpPort(t), udpPort(t)
	var log tapLog
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := Start(ctx, Config{
		Hub:       loopback,
		Dialer:    transport.Dialer{HubPort: fakeHub(t, h, nil), ListenPort: -1, Retry: 50 * time.Millisecond},
		AppPort:   appPort,
		ReplyPort: replyPort,
		Broadcast: loopback,
		NoMDNS:    true,
		Tap:       log.tap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Info().Name != "X2 HUB" {
		t.Fatalf("info %+v", p.Info())
	}
	runErr := make(chan error, 1)
	go func() { runErr <- p.Run(ctx) }()

	// The app: discovery first, then CALL_ME, then a request through the
	// proxy.
	replies, err := net.ListenPacket("udp4", netip.AddrPortFrom(loopback, uint16(replyPort)).String())
	if err != nil {
		t.Fatal(err)
	}
	defer replies.Close()
	app, err := net.Dial("udp4", netip.AddrPortFrom(loopback, uint16(appPort)).String())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	probe, _ := (wire.Frame{Family: wire.FamNotifyMe}).MarshalBinary()
	if _, err := app.Write(probe); err != nil {
		t.Fatal(err)
	}
	_ = replies.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 300)
	n, _, err := replies.ReadFrom(buf)
	if err != nil {
		t.Fatalf("no discovery reply: %v", err)
	}
	f, _, err := wire.Parse(buf[:n])
	if err != nil || f.Family != wire.FamNotifyReply {
		t.Fatalf("reply %v %v", f, err)
	}
	if info, _ := record.ParseHubInfo(f.Payload); info.Name != "X2 HUB" || info.Marker != 0x64 {
		t.Fatalf("reply info %+v", info)
	}

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	call, _ := transport.CallMe(ln.Addr().(*net.TCPAddr).AddrPort())
	raw, _ := call.MarshalBinary()
	if _, err := app.Write(raw); err != nil {
		t.Fatal(err)
	}
	_ = ln.(*net.TCPListener).SetDeadline(time.Now().Add(3 * time.Second))
	c, err := ln.Accept()
	if err != nil {
		t.Fatalf("the proxy did not call back: %v", err)
	}
	conn := transport.NewConn(c)
	defer conn.Close()
	if err := conn.WriteFrame(wire.Frame{Family: wire.FamReqDevices}); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	row, err := conn.ReadFrame()
	if err != nil || row.Family != wire.FamDeviceRow {
		t.Fatalf("relayed %v %v", row, err)
	}
	if !p.AppAttached() {
		t.Fatal("app not attached")
	}
	for _, want := range []string{"udp A→H NotifyMe", "udp H→A NotifyReply", "udp A→H CallMe", "app A→H ReqDevices", "hub H→A DeviceRow"} {
		if !log.has(want) {
			t.Errorf("tap did not see %q (saw %v)", want, log.frames)
		}
	}

	conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for p.AppAttached() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.AppAttached() {
		t.Fatal("app still attached after it closed")
	}
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// callApp plays the app's CALL_ME and returns the proxy's connection.
func callApp(t *testing.T, appPort int) *transport.Conn {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	app, err := net.Dial("udp4", netip.AddrPortFrom(loopback, uint16(appPort)).String())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	call, _ := transport.CallMe(ln.Addr().(*net.TCPAddr).AddrPort())
	raw, _ := call.MarshalBinary()
	accepted := make(chan net.Conn, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			accepted <- c
		}
	}()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(5 * time.Second)
	for {
		_, _ = app.Write(raw) // the app repeats CALL_ME until called
		select {
		case c := <-accepted:
			t.Cleanup(func() { c.Close() })
			return transport.NewConn(c)
		case <-deadline:
			t.Fatal("the proxy did not call back")
		case <-tick.C:
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A hub that reboots (after a firmware update) drops the proxy's
// connection: the proxy drops the app, reconnects and serves it again.
func TestProxyReconnects(t *testing.T) {
	info := &record.HubInfo{ID: [6]byte{0x80, 0xf1, 0xb2, 0x96, 0x2c, 0x08}, Model: 3, Firmware: 8, Name: "X2 HUB"}
	h := hubsim.FromModel(&hubsim.Model{Info: info})
	conns := make(chan net.Conn, 2)
	appPort := udpPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	p, err := Start(ctx, Config{
		Hub:       loopback,
		Dialer:    transport.Dialer{HubPort: fakeHub(t, h, conns), ListenPort: -1, Retry: 50 * time.Millisecond},
		AppPort:   appPort,
		ReplyPort: udpPort(t),
		Broadcast: loopback,
		NoMDNS:    true,
		Backoff:   20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- p.Run(ctx) }()
	first := <-conns

	app := callApp(t, appPort)
	waitFor(t, "the app", p.AppAttached)

	info.Firmware = 9 // the update
	first.Close()     // the reboot
	if _, err := app.ReadFrame(); err == nil {
		t.Fatal("the app was not dropped with the hub")
	}
	<-conns
	waitFor(t, "the hub to come back", p.HubAttached)
	if fw := p.Info().Firmware; fw != 9 {
		t.Fatalf("firmware %d after reconnecting, want the new banner's 9", fw)
	}

	app = callApp(t, appPort)
	if err := app.WriteFrame(wire.Frame{Family: wire.FamReqBanner}); err != nil {
		t.Fatal(err)
	}
	if f, err := app.ReadFrame(); err != nil || f.Family != wire.FamBanner {
		t.Fatalf("relayed %v %v after reconnecting", f, err)
	}
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMirror(t *testing.T) {
	info := &record.HubInfo{ID: [6]byte{0x80, 0xf1, 0xb2, 0x96, 0x2c, 0x08}, Variant: wire.X2, Batch: [4]byte{0x20, 0x22, 0x11, 0x20}, Firmware: 8, Name: "X2 HUB"}
	built := mirror(nil, info)
	want := []string{"HVER=3", "NO=20221120", "AVER=8", "NAME=X2 HUB", "MAC=80f1b2962c08"}
	if built.Instance != "X1-HUB-962c08" || built.Type != serviceX2 || built.Port != 8102 || len(built.TXT) != len(want) {
		t.Fatalf("built %+v", built)
	}
	for i := range want {
		if built.TXT[i] != want[i] {
			t.Fatalf("TXT %v, want %v (what our X2 advertises)", built.TXT, want)
		}
	}
	found := mdns.Service{Instance: "X1-HUB-962c08", Type: serviceX2, Host: "X1-HUB-962c08.local", Port: 8102, TXT: want}
	if m := mirror(&found, info); m.Host != "X1-HUB-962c08-proxy.local" || m.Instance != found.Instance {
		t.Fatalf("mirrored %+v", m)
	}
}
