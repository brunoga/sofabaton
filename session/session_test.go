package session_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func start(t *testing.T, h *hubsim.Hub, opts ...session.Option) *session.Session {
	t.Helper()
	s := session.New(hubsim.Pipe(h), opts...)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBurstAcrossPages(t *testing.T) {
	h := hubsim.New()
	long := bytes.Repeat([]byte{0x42}, 600) // three pages
	h.Handle(wire.FamReqMacros, func(req wire.Frame) []wire.Frame {
		return hubsim.Reply(wire.FamMacroPage, []byte("first"), long, []byte("third"))
	})
	s := start(t, h)

	b := &session.Burst{Family: wire.FamMacroPage}
	if err := s.Exchange(ctx(t), wire.Frame{Family: wire.FamReqMacros, Payload: []byte{0x65, 0xFF}}, b); err != nil {
		t.Fatal(err)
	}
	recs, err := b.Records()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || !bytes.Equal(recs[1][3:], long) || !bytes.Equal(recs[2][3:], []byte("third")) {
		t.Fatalf("%d records", len(recs))
	}
}

func TestRejected(t *testing.T) {
	h := hubsim.New()
	h.Handle(wire.FamReqActivityInputs, func(wire.Frame) []wire.Frame { return []wire.Frame{hubsim.Status(0x07)} })
	h.Handle(wire.FamRemoteSync, func(wire.Frame) []wire.Frame { return []wire.Frame{hubsim.Status(0x00)} })
	s := start(t, h)

	err := s.Exchange(ctx(t), wire.Frame{Family: wire.FamReqActivityInputs, Payload: []byte{1}}, &session.Burst{Family: wire.FamActivityInputs})
	var rej *session.RejectedError
	if !errors.Is(err, session.ErrRejected) || !errors.As(err, &rej) || rej.Status != 0x07 {
		t.Fatalf("error = %v", err)
	}
	if err := s.Exchange(ctx(t), wire.Frame{Family: wire.FamRemoteSync}, session.Status()); err != nil {
		t.Fatalf("accepted status ack: %v", err)
	}
}

func TestAckEcho(t *testing.T) {
	h := hubsim.New()
	h.Handle(wire.FamWriteButton, func(req wire.Frame) []wire.Frame {
		return []wire.Frame{
			{Family: wire.FamButtonAck, Payload: []byte{0x01}}, // another button's ack
			{Family: wire.FamButtonAck, Payload: []byte{req.Payload[7]}},
		}
	})
	var stray atomic.Int32
	s := start(t, h, session.WithUnsolicited(func(wire.Frame) { stray.Add(1) }))
	button := byte(0xB0)
	a := &session.Ack{Family: wire.FamButtonAck, Echo: &button}
	req := wire.Frame{Family: wire.FamWriteButton, Payload: []byte{1, 0, 1, 1, 0, 1, 0x65, button}}
	if err := s.Exchange(ctx(t), req, a); err != nil {
		t.Fatal(err)
	}
	if a.Frame.Payload[0] != button {
		t.Fatalf("acked by %v", a.Frame)
	}
	if stray.Load() != 1 {
		t.Fatalf("%d unclaimed frames, want the other button's ack", stray.Load())
	}
}

func TestTimeoutThenNextExchange(t *testing.T) {
	h := hubsim.New()
	h.Handle(wire.FamRemoteSync, func(wire.Frame) []wire.Frame { return []wire.Frame{hubsim.Status(0)} })
	s := start(t, h)

	c, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.Exchange(c, wire.Frame{Family: wire.FamReqDevices}, session.Status()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unanswered request: %v", err)
	}
	if err := s.Exchange(ctx(t), wire.Frame{Family: wire.FamRemoteSync}, session.Status()); err != nil {
		t.Fatalf("exchange after a timeout: %v", err)
	}
}

func TestUnsolicitedAndTap(t *testing.T) {
	h := hubsim.New()
	ready := wire.Frame{Family: wire.FamReady, Payload: []byte{0x65}}
	h.Handle(wire.FamRemoteSync, func(wire.Frame) []wire.Frame {
		return []wire.Frame{ready, hubsim.Status(0)} // a push in the middle of a reply
	})
	var mu sync.Mutex
	var got []wire.Frame
	var sent, recv atomic.Int32
	s := start(t, h,
		session.WithUnsolicited(func(f wire.Frame) { mu.Lock(); got = append(got, f); mu.Unlock() }),
		session.WithTap(func(d wire.Direction, _ wire.Frame) {
			if d == wire.ToHub {
				sent.Add(1)
			} else {
				recv.Add(1)
			}
		}))
	if err := s.Exchange(ctx(t), wire.Frame{Family: wire.FamRemoteSync}, session.Status()); err != nil {
		t.Fatal(err)
	}
	if err := h.Push(ready); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(got) != 2 || got[0].Family != wire.FamReady {
		t.Fatalf("unsolicited %v", got)
	}
	if sent.Load() != 1 || recv.Load() != 3 {
		t.Fatalf("tap saw %d sent, %d received", sent.Load(), recv.Load())
	}
}

// Concurrent exchanges run one at a time, each getting its own reply.
func TestSerialized(t *testing.T) {
	h := hubsim.New()
	var inFlight, maxInFlight atomic.Int32
	h.Handle(wire.FamReqHubName, func(req wire.Frame) []wire.Frame {
		n := inFlight.Add(1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		defer inFlight.Add(-1)
		return []wire.Frame{{Family: wire.FamHubNameReply, Payload: req.Payload}}
	})
	s := start(t, h)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			a := &session.Ack{Family: wire.FamHubNameReply}
			if err := s.Exchange(ctx(t), wire.Frame{Family: wire.FamReqHubName, Payload: []byte{byte(i)}}, a); err != nil {
				t.Error(err)
				return
			}
			if a.Frame.Payload[0] != byte(i) {
				t.Errorf("exchange %d got the reply to %d", i, a.Frame.Payload[0])
			}
		})
	}
	wg.Wait()
	if maxInFlight.Load() != 1 {
		t.Fatalf("%d requests in flight at once", maxInFlight.Load())
	}
}

func TestSessionEnds(t *testing.T) {
	h := hubsim.New()
	s := session.New(hubsim.Pipe(h))
	errc := make(chan error, 1)
	go func() { errc <- s.Exchange(ctx(t), wire.Frame{Family: wire.FamReqDevices}, session.Status()) }()
	time.Sleep(20 * time.Millisecond)
	_ = s.Close()
	if err := <-errc; !errors.Is(err, session.ErrClosed) {
		t.Fatalf("pending exchange: %v", err)
	}
	if err := s.Send(ctx(t), wire.Frame{Family: wire.FamReqDevices}); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("send after close: %v", err)
	}
	if !errors.Is(s.Err(), session.ErrClosed) {
		t.Fatalf("Err = %v", s.Err())
	}
}
