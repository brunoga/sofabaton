package mdns

import (
	"context"
	"errors"
	"net"
	"time"
)

// Browse asks for instances of serviceType ("_sofabaton_hub._udp") until
// ctx is done and returns them, one per instance name, with their
// records.
//
// It sends one-shot queries from an ephemeral port, which responders
// answer by unicast (RFC 6762 section 5.1), instead of binding the mDNS
// port: two sockets of one process on port 5353 share its traffic rather
// than each seeing all of it, which would starve a Responder.
func Browse(ctx context.Context, serviceType string) ([]Service, error) {
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return nil, err
	}
	c := pc.(*net.UDPConn)
	defer func() { _ = c.Close() }()
	q, err := query(serviceType)
	if err != nil {
		return nil, err
	}
	group := net.UDPAddrFromAddrPort(Group)
	stop := context.AfterFunc(ctx, func() { _ = c.SetReadDeadline(time.Now()) })
	defer stop()

	found := map[string]Service{}
	var order []string
	buf := make([]byte, 9000)
	next := time.Now()
	for {
		if time.Now().After(next) {
			if _, err := c.WriteToUDP(q, group); err != nil {
				return collect(found, order), err
			}
			next = time.Now().Add(time.Second)
		}
		if ctx.Err() == nil {
			_ = c.SetReadDeadline(next)
		}
		n, _, err := c.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if ctx.Err() != nil {
				return collect(found, order), nil
			}
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return collect(found, order), err
		}
		svcs, err := parseServices(buf[:n], serviceType)
		if err != nil {
			continue
		}
		for _, s := range svcs {
			if _, ok := found[s.Instance]; !ok {
				order = append(order, s.Instance)
			}
			if prev, ok := found[s.Instance]; ok && !s.Addr.IsValid() {
				s.Addr, s.Host, s.Port = prev.Addr, prev.Host, prev.Port
			}
			found[s.Instance] = s
		}
	}
}

func collect(found map[string]Service, order []string) []Service {
	out := make([]Service, 0, len(order))
	for _, k := range order {
		out = append(out, found[k])
	}
	return out
}
