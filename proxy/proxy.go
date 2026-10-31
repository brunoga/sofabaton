// Package proxy stands between the official Sofabaton app and a hub: it
// holds the hub's one client connection, advertises itself to the app as
// that hub, and relays every frame between the two, so the traffic can be
// recorded and studied.
//
// The hub stops advertising while a client is connected, so the app finds
// only the proxy. The proxy mirrors the hub's mDNS advertisement (the
// Android app's discovery) and answers the NOTIFY_ME broadcast (the iOS
// app's); the app then sends CALL_ME and the proxy connects to it.
package proxy

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/brunoga/sofabaton/internal/mdns"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/transport"
	"github.com/brunoga/sofabaton/wire"
)

// Links a frame can cross, as a Tap sees them.
const (
	LinkApp = "app" // between the app and the proxy
	LinkHub = "hub" // between the proxy and the hub
	LinkUDP = "udp" // discovery and CALL_ME
)

// Config configures a proxy.
type Config struct {
	// Hub is the hub's address.
	Hub netip.Addr
	// Dialer connects to the hub; the zero value uses the defaults.
	Dialer transport.Dialer
	// LocalAddr is the address advertised to the app; by default the
	// local address on the route to the hub.
	LocalAddr netip.Addr
	// AppPort is the UDP port the app's probes and CALL_ME arrive on
	// (default 8102), ReplyPort the port discovery replies are broadcast
	// to (default 8100) and Broadcast the address (default
	// 255.255.255.255).
	AppPort, ReplyPort int
	Broadcast          netip.Addr
	// NoMDNS turns the mDNS advertisement off.
	NoMDNS bool
	// DialTimeout bounds each reconnection attempt (default 20s), and
	// Backoff is the first wait between attempts (default 1s, doubling up
	// to 30s).
	DialTimeout, Backoff time.Duration
	// Tap sees every frame, after it is relayed. It must not block.
	Tap func(link string, dir wire.Direction, f wire.Frame)
	// Log receives progress messages; nil discards them.
	Log *slog.Logger
}

// Proxy is a running proxy.
type Proxy struct {
	cfg   Config
	log   *slog.Logger
	udp   *net.UDPConn
	local netip.Addr

	mu     sync.Mutex
	closed bool
	hub    *transport.Conn // nil while the hub is away
	info   *record.HubInfo
	mdns   *mdns.Responder
	app    *transport.Conn
}

// Start connects to the hub and starts serving the app. It returns once
// the proxy is ready; Run then serves until ctx is done.
func Start(ctx context.Context, cfg Config) (*Proxy, error) {
	p := &Proxy{cfg: cfg, log: cfg.Log, local: cfg.LocalAddr}
	if p.log == nil {
		p.log = slog.New(slog.DiscardHandler)
	}
	if p.cfg.AppPort == 0 {
		p.cfg.AppPort = transport.HubPort
	}
	if p.cfg.ReplyPort == 0 {
		p.cfg.ReplyPort = transport.ReplyPort
	}
	if !p.cfg.Broadcast.IsValid() {
		p.cfg.Broadcast = netip.AddrFrom4([4]byte{255, 255, 255, 255})
	}
	if p.cfg.Backoff <= 0 {
		p.cfg.Backoff = time.Second
	}

	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp4", fmt.Sprintf(":%d", p.cfg.AppPort))
	if err != nil {
		return nil, fmt.Errorf("proxy: listening for the app: %w", err)
	}
	p.udp = pc.(*net.UDPConn)
	if err := p.connect(ctx); err != nil {
		p.close()
		return nil, err
	}
	return p, nil
}

// connect reads the hub's advertisement, connects to the hub, reads its
// identity and advertises the proxy in its place.
func (p *Proxy) connect(ctx context.Context) error {
	// The hub stops advertising once it has a client: read its
	// advertisement first.
	var svc *mdns.Service
	if !p.cfg.NoMDNS {
		svc = p.browse(ctx)
	}
	conn, err := p.cfg.Dialer.Dial(ctx, p.cfg.Hub)
	if err != nil {
		return err
	}
	info, err := banner(ctx, conn)
	if err != nil {
		_ = conn.Close()
		return err
	}
	p.log.Info("connected to the hub", "name", info.Name, "model", info.Variant, "firmware", info.Firmware)

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		_ = conn.Close()
		return net.ErrClosed
	}
	if !p.local.IsValid() {
		if addr, ok := conn.LocalAddr().(*net.TCPAddr); ok {
			p.local = addr.AddrPort().Addr().Unmap()
		}
	}
	if !p.cfg.NoMDNS {
		s := mirror(svc, info)
		s.Addr = p.local
		r, err := mdns.Advertise(s, nil)
		if err != nil {
			_ = conn.Close()
			return err
		}
		p.mdns = r
		p.log.Info("advertising", "instance", s.Instance, "addr", p.local)
	}
	p.hub, p.info = conn, info
	return nil
}

// Info is the hub's identity, as last read.
func (p *Proxy) Info() *record.HubInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info
}

// AppAttached reports whether the app is connected.
func (p *Proxy) AppAttached() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.app != nil
}

// HubAttached reports whether the proxy holds the hub's connection.
func (p *Proxy) HubAttached() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hub != nil
}

// Run serves until ctx is done, then shuts down. When the hub goes away
// (it reboots after a firmware update) the proxy withdraws its
// advertisement, drops the app and reconnects, retrying with backoff, so
// the app finds the hub again once it is back.
func (p *Proxy) Run(ctx context.Context) error {
	defer p.close()
	stop := context.AfterFunc(ctx, p.close)
	defer stop()

	go p.serveUDP(ctx)
	for {
		p.mu.Lock()
		hub := p.hub
		p.mu.Unlock()
		if hub == nil {
			return nil // closed
		}
		err := p.fromHub(hub)
		if ctx.Err() != nil {
			return nil
		}
		p.log.Warn("lost the hub; withdrawing and reconnecting", "err", err)
		p.drop()
		if err := p.reconnect(ctx); err != nil {
			return nil // ctx is done
		}
	}
}

// reconnect retries connect until it succeeds or ctx is done.
func (p *Proxy) reconnect(ctx context.Context) error {
	wait := p.cfg.Backoff
	for {
		dctx, cancel := context.WithTimeout(ctx, p.dialTimeout())
		err := p.connect(dctx)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.log.Info("the hub is not back yet", "err", err, "retry", wait)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		wait = min(2*wait, 30*time.Second)
	}
}

func (p *Proxy) dialTimeout() time.Duration {
	if p.cfg.DialTimeout > 0 {
		return p.cfg.DialTimeout
	}
	return 20 * time.Second
}

// drop releases the hub, the app and the advertisement, leaving the UDP
// listener up (and silent until the hub is back).
func (p *Proxy) drop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mdns != nil {
		_ = p.mdns.Close() // sends goodbye
		p.mdns = nil
	}
	if p.app != nil {
		_ = p.app.Close()
		p.app = nil
	}
	if p.hub != nil {
		_ = p.hub.Close()
		p.hub = nil
	}
}

func (p *Proxy) close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.drop()
	_ = p.udp.Close()
}

func (p *Proxy) tap(link string, dir wire.Direction, f wire.Frame) {
	if p.cfg.Tap != nil {
		p.cfg.Tap(link, dir, f)
	}
}

// fromHub relays the hub's frames to the app, dropping them while no app
// is attached, until the hub connection fails.
func (p *Proxy) fromHub(hub *transport.Conn) error {
	for {
		f, err := hub.ReadFrame()
		if err != nil {
			return err
		}
		p.tap(LinkHub, wire.FromHub, f)
		p.mu.Lock()
		app := p.app
		p.mu.Unlock()
		if app == nil {
			p.log.Debug("dropped a hub frame, no app attached", "family", f.Family.Name(wire.FromHub))
			continue
		}
		if err := app.WriteFrame(f); err != nil {
			p.log.Warn("writing to the app", "err", err)
		}
	}
}

// fromApp relays the app's frames to the hub until the app disconnects or
// the hub goes away.
func (p *Proxy) fromApp(app *transport.Conn) {
	defer func() {
		p.mu.Lock()
		if p.app == app {
			p.app = nil
		}
		p.mu.Unlock()
		_ = app.Close()
		p.log.Info("app detached")
	}()
	for {
		f, err := app.ReadFrame()
		if err != nil {
			return
		}
		p.tap(LinkApp, wire.ToHub, f)
		p.mu.Lock()
		hub := p.hub
		p.mu.Unlock()
		if hub == nil {
			return
		}
		if err := hub.WriteFrame(f); err != nil {
			p.log.Warn("writing to the hub", "err", err)
			return
		}
	}
}

func (p *Proxy) serveUDP(ctx context.Context) {
	buf := make([]byte, wire.MaxFrameLen)
	for {
		n, from, err := p.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		f, used, err := wire.Parse(buf[:n])
		if err != nil || used != n {
			continue
		}
		p.tap(LinkUDP, wire.ToHub, f)
		if !p.HubAttached() {
			continue // stay silent until the hub is back
		}
		switch f.Family {
		case wire.FamNotifyMe:
			p.notifyReply()
		case wire.FamCallMe:
			p.callMe(ctx, f, from)
		}
	}
}

// notifyReply answers a discovery probe the way the hub does: a broadcast
// of its identity.
func (p *Proxy) notifyReply() {
	info := *p.Info()
	info.Marker = 0x64 // what hubs send in discovery replies
	reply := wire.Frame{Family: wire.FamNotifyReply, Payload: info.AppendPayload(nil)}
	raw, err := reply.MarshalBinary()
	if err != nil {
		return
	}
	to := net.UDPAddrFromAddrPort(netip.AddrPortFrom(p.cfg.Broadcast, uint16(p.cfg.ReplyPort)))
	if _, err := p.udp.WriteToUDP(raw, to); err == nil {
		p.tap(LinkUDP, wire.FromHub, reply)
	}
}

// callMe connects to the app that sent CALL_ME, at its source address and
// the port in the frame (the app's own IP field is little-endian).
func (p *Proxy) callMe(ctx context.Context, f wire.Frame, from *net.UDPAddr) {
	addr, err := transport.ParseCallMe(f.Payload)
	if err != nil {
		return
	}
	if p.AppAttached() {
		return // the app repeats CALL_ME until it is called
	}
	target := netip.AddrPortFrom(from.AddrPort().Addr().Unmap(), addr.Port())
	d := net.Dialer{Timeout: 5 * time.Second, KeepAliveConfig: transport.KeepAlive}
	c, err := d.DialContext(ctx, "tcp4", target.String())
	if err != nil {
		p.log.Warn("calling the app back", "addr", target, "err", err)
		return
	}
	app := transport.NewConn(c)
	p.mu.Lock()
	if p.app != nil {
		p.mu.Unlock()
		_ = app.Close()
		return
	}
	p.app = app
	p.mu.Unlock()
	p.log.Info("app attached", "addr", target)
	go p.fromApp(app)
}

// banner reads the hub's identity before relaying starts.
func banner(ctx context.Context, c *transport.Conn) (*record.HubInfo, error) {
	if err := c.WriteFrame(wire.Frame{Family: wire.FamReqBanner}); err != nil {
		return nil, err
	}
	type result struct {
		f   wire.Frame
		err error
	}
	got := make(chan result, 1)
	go func() {
		for {
			f, err := c.ReadFrame()
			if err != nil || f.Family == wire.FamBanner {
				got <- result{f, err}
				return
			}
		}
	}()
	select {
	case r := <-got:
		if r.err != nil {
			return nil, fmt.Errorf("proxy: reading the banner: %w", r.err)
		}
		return record.ParseHubInfo(r.f.Payload)
	case <-ctx.Done():
		_ = c.Close()
		return nil, fmt.Errorf("proxy: reading the banner: %w", ctx.Err())
	}
}

// Service type of the hubs' mDNS advertisements.
const (
	serviceX2 = "_sofabaton_hub._udp"
	serviceX1 = "_x1hub._udp"
)

func (p *Proxy) browse(ctx context.Context) *mdns.Service {
	for _, t := range []string{serviceX2, serviceX1} {
		bctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		found, err := mdns.Browse(bctx, t)
		cancel()
		if err != nil {
			p.log.Warn("browsing for the hub", "err", err)
			continue
		}
		for _, s := range found {
			if s.Addr == p.cfg.Hub {
				p.log.Info("found the hub's advertisement", "instance", s.Instance)
				return &s
			}
		}
	}
	p.log.Info("the hub is not advertising (connected to another client?); building its advertisement from the banner")
	return nil
}

// mirror is the advertisement the proxy makes: the hub's instance name and
// TXT record (its own when it was found, else built from its banner the way
// hubs build theirs) under a host name of the proxy's, so that cached
// records of the hub's host do not compete with the proxy's address.
func mirror(svc *mdns.Service, info *record.HubInfo) mdns.Service {
	if svc != nil {
		s := *svc
		s.Host = s.Instance + "-proxy.local"
		s.Port = transport.HubPort
		return s
	}
	instance := "X1-HUB-" + hex.EncodeToString(info.ID[3:])
	t := serviceX2
	hver := "3"
	switch info.Variant {
	case wire.X1:
		t, hver = serviceX1, "1"
	case wire.X1S:
		t, hver = serviceX1, "2"
	}
	return mdns.Service{
		Instance: instance,
		Type:     t,
		Host:     instance + "-proxy.local",
		Port:     transport.HubPort,
		TXT: []string{
			"HVER=" + hver,
			"NO=" + hex.EncodeToString(info.Batch[:]),
			fmt.Sprintf("AVER=%d", info.Firmware),
			"NAME=" + info.Name,
			"MAC=" + hex.EncodeToString(info.ID[:]),
		},
	}
}
