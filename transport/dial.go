package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"
)

// Default ports.
const (
	// HubPort is the hub's UDP port for CALL_ME and discovery probes.
	HubPort = 8102
	// ReplyPort is where hubs broadcast discovery replies.
	ReplyPort = 8100
	// ListenPort is the first TCP port tried for the hub's connect-back;
	// ListenPorts ports from it are tried.
	ListenPort  = 8200
	ListenPorts = 64
)

// KeepAlive is the TCP keep-alive used on hub connections. A hub that
// reboots (after a firmware update, say) drops its connections without a
// word; short probes notice in seconds instead of minutes.
var KeepAlive = net.KeepAliveConfig{Enable: true, Idle: 5 * time.Second, Interval: 5 * time.Second, Count: 3}

// ErrRefused reports an address whose UDP port 8102 is closed: a hub
// closes it while it serves a client, and anything that is not a hub has
// it closed too.
var ErrRefused = errors.New("CALL_ME refused")

// Dialer opens sessions with hubs. The zero value uses the defaults.
type Dialer struct {
	// HubPort is the hub's UDP port (default 8102).
	HubPort int
	// LocalAddr is the address the hub connects back to; by default the
	// local address on the route to the hub.
	LocalAddr netip.Addr
	// ListenPort and ListenPorts pick the TCP ports tried for the
	// connect-back (default 8200 and 64). ListenPort -1 lets the system
	// choose one.
	ListenPort  int
	ListenPorts int
	// Retry is how often CALL_ME is resent while waiting (default 1s).
	Retry time.Duration
}

// Dial asks the hub at hub to connect back and returns the connection. The
// context bounds the whole exchange.
func (d *Dialer) Dial(ctx context.Context, hub netip.Addr) (*Conn, error) {
	hubPort := d.HubPort
	if hubPort == 0 {
		hubPort = HubPort
	}
	udp, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(hub, uint16(hubPort))))
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}
	defer func() { _ = udp.Close() }()

	local := d.LocalAddr
	if !local.IsValid() {
		local = udp.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Unmap()
	}
	ln, err := d.listen(ctx, local)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ln.Close() }()

	frame, err := CallMe(ln.Addr().(*net.TCPAddr).AddrPort())
	if err != nil {
		return nil, err
	}
	call, err := frame.MarshalBinary()
	if err != nil {
		return nil, err
	}

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				acceptErr <- err
				return
			}
			if addr, ok := c.RemoteAddr().(*net.TCPAddr); ok && addr.AddrPort().Addr().Unmap() == hub {
				accepted <- c
				return
			}
			_ = c.Close() // someone else; keep waiting for the hub
		}
	}()

	retry := d.Retry
	if retry <= 0 {
		retry = time.Second
	}
	tick := time.NewTicker(retry)
	defer tick.Stop()
	for {
		if _, err := udp.Write(call); err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) {
				return nil, fmt.Errorf("transport: %v: %w: a hub serving another client (it takes one at a time; close the Sofabaton app), or not a hub", hub, ErrRefused)
			}
			return nil, fmt.Errorf("transport: sending CALL_ME: %w", err)
		}
		select {
		case c := <-accepted:
			return NewConn(c), nil
		case err := <-acceptErr:
			return nil, fmt.Errorf("transport: waiting for the hub: %w", err)
		case <-ctx.Done():
			return nil, fmt.Errorf("transport: hub %v did not connect back: %w", hub, ctx.Err())
		case <-tick.C:
		}
	}
}

func (d *Dialer) listen(ctx context.Context, local netip.Addr) (net.Listener, error) {
	lc := net.ListenConfig{KeepAliveConfig: KeepAlive}
	first, n := d.ListenPort, d.ListenPorts
	if first == 0 {
		first = ListenPort
	}
	if n <= 0 {
		n = ListenPorts
	}
	if first < 0 {
		first, n = 0, 1
	}
	var errs []error
	for port := first; port < first+n; port++ {
		ln, err := lc.Listen(ctx, "tcp4", netip.AddrPortFrom(local, uint16(port)).String())
		if err == nil {
			return ln, nil
		}
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("transport: no free listen port from %d: %w", first, errors.Join(errs...))
}
