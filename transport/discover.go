package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// Announcement is a hub's answer to a discovery probe.
type Announcement struct {
	Addr netip.Addr
	Info *record.HubInfo
}

// Discoverer finds hubs with the NOTIFY_ME broadcast. The zero value uses
// the defaults. Hubs connected to a client (the app, or a session of ours)
// do not answer.
type Discoverer struct {
	// Targets are the addresses probed (default the limited broadcast
	// address, 255.255.255.255).
	Targets []netip.Addr
	// HubPort is the hubs' UDP port (default 8102), ReplyPort the port
	// they broadcast replies to (default 8100).
	HubPort, ReplyPort int
	// Retry is how often the probe is resent (default 1s).
	Retry time.Duration
}

// Discover probes until ctx is done and returns the hubs that answered,
// one per hub id.
func (d *Discoverer) Discover(ctx context.Context) ([]Announcement, error) {
	hubPort, replyPort := d.HubPort, d.ReplyPort
	if hubPort == 0 {
		hubPort = HubPort
	}
	if replyPort == 0 {
		replyPort = ReplyPort
	}
	targets := d.Targets
	if len(targets) == 0 {
		targets = []netip.Addr{netip.AddrFrom4([4]byte{255, 255, 255, 255})}
	}

	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp4", fmt.Sprintf(":%d", replyPort))
	if err != nil {
		return nil, fmt.Errorf("transport: listening for discovery replies: %w", err)
	}
	defer func() { _ = pc.Close() }()

	probe, _ := (wire.Frame{Family: wire.FamNotifyMe}).MarshalBinary()
	send := func() error {
		for _, t := range targets {
			if _, err := pc.WriteTo(probe, net.UDPAddrFromAddrPort(netip.AddrPortFrom(t, uint16(hubPort)))); err != nil {
				return fmt.Errorf("transport: sending discovery probe to %v: %w", t, err)
			}
		}
		return nil
	}
	if err := send(); err != nil {
		return nil, err
	}

	retry := d.Retry
	if retry <= 0 {
		retry = time.Second
	}
	stop := context.AfterFunc(ctx, func() { _ = pc.SetReadDeadline(time.Now()) })
	defer stop()

	seen := map[[record.HubIDLen]byte]bool{}
	var found []Announcement
	buf := make([]byte, wire.MaxFrameLen)
	next := time.Now().Add(retry)
	for {
		if ctx.Err() == nil {
			_ = pc.SetReadDeadline(next)
		}
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if ctx.Err() != nil {
				return found, nil
			}
			if errors.As(err, &ne) && ne.Timeout() {
				if err := send(); err != nil {
					return found, err
				}
				next = time.Now().Add(retry)
				continue
			}
			return found, fmt.Errorf("transport: reading discovery replies: %w", err)
		}
		a, ok := parseAnnouncement(buf[:n], from)
		if !ok || seen[a.Info.ID] {
			continue
		}
		seen[a.Info.ID] = true
		found = append(found, a)
	}
}

func parseAnnouncement(pkt []byte, from net.Addr) (Announcement, bool) {
	f, n, err := wire.Parse(pkt)
	if err != nil || n != len(pkt) || f.Family != wire.FamNotifyReply {
		return Announcement{}, false
	}
	info, err := record.ParseHubInfo(f.Payload)
	if err != nil {
		return Announcement{}, false
	}
	udp, ok := from.(*net.UDPAddr)
	if !ok {
		return Announcement{}, false
	}
	return Announcement{Addr: udp.AddrPort().Addr().Unmap(), Info: info}, true
}
