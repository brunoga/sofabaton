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
	// Tap sees every frame, after it is relayed. It must not block.
	Tap func(link string, dir wire.Direction, f wire.Frame)
	// Log receives progress messages; nil discards them.
	Log *slog.Logger
}

// Proxy is a running proxy.
type Proxy struct {
	cfg  Config
	log  *slog.Logger
	info *record.HubInfo
	hub  *transport.Conn
	udp  *net.UDPConn
	mdns *mdns.Responder

	mu  sync.Mutex
	app *transport.Conn
}

// Start connects to the hub and starts serving the app. It returns once
// the proxy is ready; Run then serves until ctx is done or the hub goes
// away.
func Start(ctx context.Context, cfg Config) (*Proxy, error) {
	p := &Proxy{cfg: cfg, log: cfg.Log}
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

	// The hub stops advertising once it has a client: read its
	// advertisement first.
	var svc *mdns.Service
	if !cfg.NoMDNS {
		svc = p.browse(ctx)
	}

	conn, err := cfg.Dialer.Dial(ctx, cfg.Hub)
	if err != nil {
		return nil, err
	}
	p.hub = conn
	if p.info, err = banner(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	p.log.Info("connected to the hub", "name", p.info.Name, "model", p.info.Variant, "firmware", p.info.Firmware)

	local := cfg.LocalAddr
	if !local.IsValid() {
		if addr, ok := conn.LocalAddr().(*net.TCPAddr); ok {
			local = addr.AddrPort().Addr().Unmap()
		}
	}
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp4", fmt.Sprintf(":%d", p.cfg.AppPort))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy: listening for the app: %w", err)
	}
	p.udp = pc.(*net.UDPConn)

	if !cfg.NoMDNS {
		s := mirror(svc, p.info)
		s.Addr = local
		if p.mdns, err = mdns.Advertise(s, nil); err != nil {
			p.close()
			return nil, err
		}
		p.log.Info("advertising", "instance", s.Instance, "addr", local)
	}
	return p, nil
}

// Info is the hub's identity.
func (p *Proxy) Info() *record.HubInfo { return p.info }

// AppAttached reports whether the app is connected.
func (p *Proxy) AppAttached() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.app != nil
}

// Run serves until ctx is done or the hub disconnects, then shuts down.
func (p *Proxy) Run(ctx context.Context) error {
	defer p.close()
	stop := context.AfterFunc(ctx, p.close)
	defer stop()

	hubErr := make(chan error, 1)
	go func() { hubErr <- p.fromHub() }()
	go p.serveUDP(ctx)

	select {
	case <-ctx.Done():
		return nil
	case err := <-hubErr:
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("proxy: hub connection: %w", err)
	}
}

func (p *Proxy) close() {
	if p.mdns != nil {
		_ = p.mdns.Close()
	}
	if p.udp != nil {
		_ = p.udp.Close()
	}
	p.mu.Lock()
	if p.app != nil {
		_ = p.app.Close()
	}
	p.mu.Unlock()
	_ = p.hub.Close()
}

func (p *Proxy) tap(link string, dir wire.Direction, f wire.Frame) {
	if p.cfg.Tap != nil {
		p.cfg.Tap(link, dir, f)
	}
}

// fromHub relays the hub's frames to the app, dropping them while no app
// is attached.
func (p *Proxy) fromHub() error {
	for {
		f, err := p.hub.ReadFrame()
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

// fromApp relays the app's frames to the hub until the app disconnects.
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
		if err := p.hub.WriteFrame(f); err != nil {
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
	info := *p.info
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
	d := net.Dialer{Timeout: 5 * time.Second}
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
