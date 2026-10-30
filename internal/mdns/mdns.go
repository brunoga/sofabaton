// Package mdns is a minimal multicast DNS (RFC 6762) browser and
// responder for one IPv4 service, enough to find a hub's advertisement and
// to advertise a proxy in its place.
package mdns

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
)

// Group is the IPv4 mDNS multicast group and port.
var Group = netip.MustParseAddrPort("224.0.0.251:5353")

// Service is one advertised service instance.
type Service struct {
	// Instance is the instance name ("X1-HUB-962c08"), Type the service
	// type ("_sofabaton_hub._udp"); the domain is always local.
	Instance string
	Type     string
	// Host is the target host name ("X1-HUB-962c08.local"), Addr its
	// address and Port the service port.
	Host string
	Addr netip.Addr
	Port uint16
	// TXT holds key=value strings.
	TXT []string
}

// TXTValue returns the value of key in the TXT record.
func (s Service) TXTValue(key string) (string, bool) {
	for _, kv := range s.TXT {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

func fqdn(name string) string {
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

func (s Service) typeName() string     { return fqdn(s.Type + ".local") }
func (s Service) instanceName() string { return fqdn(s.Instance + "." + s.Type + ".local") }
func (s Service) hostName() string {
	if s.Host == "" {
		return fqdn(s.Instance + ".local")
	}
	return fqdn(s.Host)
}

const (
	ttlShared  = 4500 // PTR
	ttlUnique  = 120  // SRV, TXT, A
	cacheFlush = 1 << 15
)

func name(s string) (dnsmessage.Name, error) { return dnsmessage.NewName(s) }

// answer builds a response carrying the service's PTR, SRV, TXT and A
// records; ttl 0 withdraws them.
func (s Service) answer(goodbye bool) ([]byte, error) {
	tn, err := name(s.typeName())
	if err != nil {
		return nil, err
	}
	in, err := name(s.instanceName())
	if err != nil {
		return nil, err
	}
	hn, err := name(s.hostName())
	if err != nil {
		return nil, err
	}
	if !s.Addr.Is4() {
		return nil, fmt.Errorf("mdns: %v is not IPv4", s.Addr)
	}
	shared, unique := uint32(ttlShared), uint32(ttlUnique)
	if goodbye {
		shared, unique = 0, 0
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	b.EnableCompression()
	if err := b.StartAnswers(); err != nil {
		return nil, err
	}
	in4 := dnsmessage.ClassINET
	flush := in4 | cacheFlush
	if err := b.PTRResource(dnsmessage.ResourceHeader{Name: tn, Class: in4, TTL: shared}, dnsmessage.PTRResource{PTR: in}); err != nil {
		return nil, err
	}
	if err := b.SRVResource(dnsmessage.ResourceHeader{Name: in, Class: flush, TTL: unique}, dnsmessage.SRVResource{Target: hn, Port: s.Port}); err != nil {
		return nil, err
	}
	txt := s.TXT
	if len(txt) == 0 {
		txt = []string{""}
	}
	if err := b.TXTResource(dnsmessage.ResourceHeader{Name: in, Class: flush, TTL: unique}, dnsmessage.TXTResource{TXT: txt}); err != nil {
		return nil, err
	}
	if err := b.AResource(dnsmessage.ResourceHeader{Name: hn, Class: flush, TTL: unique}, dnsmessage.AResource{A: s.Addr.As4()}); err != nil {
		return nil, err
	}
	return b.Finish()
}

// query builds a PTR question for a service type.
func query(serviceType string) ([]byte, error) {
	n, err := name(fqdn(serviceType + ".local"))
	if err != nil {
		return nil, err
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(dnsmessage.Question{Name: n, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}); err != nil {
		return nil, err
	}
	return b.Finish()
}

// parseServices extracts the instances of serviceType a response
// describes, joining PTR, SRV, TXT and A records from any section.
func parseServices(msg []byte, serviceType string) ([]Service, error) {
	var p dnsmessage.Parser
	h, err := p.Start(msg)
	if err != nil {
		return nil, err
	}
	if !h.Response {
		return nil, nil
	}
	if err := p.SkipAllQuestions(); err != nil {
		return nil, err
	}
	want := strings.ToLower(fqdn(serviceType + ".local"))
	var instances []string
	srv := map[string]dnsmessage.SRVResource{}
	txt := map[string][]string{}
	addr := map[string]netip.Addr{}
	read := func(next func() (dnsmessage.ResourceHeader, error)) error {
		for {
			rh, err := next()
			if err == dnsmessage.ErrSectionDone {
				return nil
			}
			if err != nil {
				return err
			}
			key := strings.ToLower(rh.Name.String())
			switch rh.Type {
			case dnsmessage.TypePTR:
				r, err := p.PTRResource()
				if err != nil {
					return err
				}
				if key == want {
					instances = append(instances, r.PTR.String())
				}
			case dnsmessage.TypeSRV:
				r, err := p.SRVResource()
				if err != nil {
					return err
				}
				srv[key] = r
			case dnsmessage.TypeTXT:
				r, err := p.TXTResource()
				if err != nil {
					return err
				}
				txt[key] = r.TXT
			case dnsmessage.TypeA:
				r, err := p.AResource()
				if err != nil {
					return err
				}
				addr[key] = netip.AddrFrom4(r.A)
			default:
				if _, err := p.UnknownResource(); err != nil {
					return err
				}
			}
		}
	}
	for _, next := range []func() (dnsmessage.ResourceHeader, error){p.AnswerHeader, p.AuthorityHeader, p.AdditionalHeader} {
		if err := read(next); err != nil {
			return nil, err
		}
	}
	var out []Service
	for _, inst := range instances {
		k := strings.ToLower(inst)
		if !strings.HasSuffix(k, "."+want) {
			continue
		}
		s := Service{Instance: inst[:len(inst)-len(want)-1], Type: serviceType, TXT: txt[k]}
		if r, ok := srv[k]; ok {
			s.Host = strings.TrimSuffix(r.Target.String(), ".")
			s.Port = r.Port
			s.Addr = addr[strings.ToLower(r.Target.String())]
		}
		out = append(out, s)
	}
	return out, nil
}

// asksFor reports whether a query asks for anything of svc.
func asksFor(msg []byte, svc Service) bool {
	var p dnsmessage.Parser
	h, err := p.Start(msg)
	if err != nil || h.Response {
		return false
	}
	names := map[string]bool{
		strings.ToLower(svc.typeName()):     true,
		strings.ToLower(svc.instanceName()): true,
		strings.ToLower(svc.hostName()):     true,
		"_services._dns-sd._udp.local.":     true,
	}
	for {
		q, err := p.Question()
		if err != nil {
			return false
		}
		if names[strings.ToLower(q.Name.String())] {
			return true
		}
	}
}

// listen joins the mDNS group on ifi (nil for the system default).
// ListenMulticastUDP turns multicast loopback off; it is turned back on so
// that browsers on the same host (avahi, the tests) see the answers.
func listen(ifi *net.Interface) (*net.UDPConn, error) {
	c, err := net.ListenMulticastUDP("udp4", ifi, net.UDPAddrFromAddrPort(Group))
	if err != nil {
		return nil, fmt.Errorf("mdns: joining %v: %w", Group, err)
	}
	if err := ipv4.NewPacketConn(c).SetMulticastLoopback(true); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("mdns: enabling multicast loopback: %w", err)
	}
	return c, nil
}
