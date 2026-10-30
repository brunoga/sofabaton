package mdns

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// Responder advertises one service: it announces it, answers queries for
// it and withdraws it when closed.
type Responder struct {
	svc  Service
	conn *net.UDPConn
	ans  []byte
	wg   sync.WaitGroup
}

// Advertise starts advertising svc on ifi (nil for the system default).
func Advertise(svc Service, ifi *net.Interface) (*Responder, error) {
	ans, err := svc.answer(false)
	if err != nil {
		return nil, err
	}
	c, err := listen(ifi)
	if err != nil {
		return nil, err
	}
	r := &Responder{svc: svc, conn: c, ans: ans}
	r.wg.Go(r.serve)
	r.wg.Go(r.announce)
	return r, nil
}

// announce sends the unsolicited announcements RFC 6762 asks for.
func (r *Responder) announce() {
	group := net.UDPAddrFromAddrPort(Group)
	for i := range 3 {
		if _, err := r.conn.WriteToUDP(r.ans, group); err != nil {
			return
		}
		time.Sleep(time.Duration(1<<i) * time.Second)
	}
}

func (r *Responder) serve() {
	group := net.UDPAddrFromAddrPort(Group)
	buf := make([]byte, 9000)
	for {
		n, from, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if !asksFor(buf[:n], r.svc) {
			continue
		}
		_, _ = r.conn.WriteToUDP(r.ans, group)
		if from.Port != int(Group.Port()) {
			// A one-shot (legacy) querier wants a unicast answer.
			_, _ = r.conn.WriteToUDP(r.ans, from)
		}
	}
}

// Close withdraws the service and stops answering.
func (r *Responder) Close() error {
	if bye, err := r.svc.answer(true); err == nil {
		_, _ = r.conn.WriteToUDP(bye, net.UDPAddrFromAddrPort(Group))
	}
	err := r.conn.Close()
	r.wg.Wait()
	if err != nil {
		return fmt.Errorf("mdns: %w", err)
	}
	return nil
}
