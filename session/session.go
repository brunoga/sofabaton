// Package session runs request/reply exchanges with a hub over a frame
// connection.
//
// The hub handles one request at a time, so a Session serializes
// exchanges: each sends a request and feeds incoming frames to a Collector
// until the collector reports the reply complete. Frames no exchange
// claims (activity changes, OTA notices) go to the unsolicited handler.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/brunoga/sofabaton/wire"
)

// FrameConn is a connection carrying frames, such as a transport.Conn.
type FrameConn interface {
	ReadFrame() (wire.Frame, error)
	WriteFrame(wire.Frame) error
	Close() error
}

// ErrClosed reports an exchange on a session that has ended.
var ErrClosed = errors.New("session: closed")

// Option configures a Session.
type Option func(*Session)

// WithUnsolicited sets the handler for frames no exchange claims. It runs
// on the session's reader goroutine and must not block.
func WithUnsolicited(h func(wire.Frame)) Option {
	return func(s *Session) { s.unsolicited = h }
}

// WithTap sets a function that sees every frame sent and received, for
// logging and captures. It runs on the sending or reading goroutine and
// must not block.
func WithTap(h func(wire.Direction, wire.Frame)) Option {
	return func(s *Session) { s.tap = h }
}

// Session is an open conversation with a hub.
type Session struct {
	conn        FrameConn
	unsolicited func(wire.Frame)
	tap         func(wire.Direction, wire.Frame)

	watchMu  sync.Mutex
	watchers map[int]func(wire.Direction, wire.Frame)
	watchID  int

	slot chan struct{} // held by the exchange in progress

	mu     sync.Mutex
	active *exchange

	done chan struct{}
	err  error // set before done is closed
}

type exchange struct {
	c      Collector
	result chan error
}

// New starts a session on conn. It owns conn from now on.
func New(conn FrameConn, opts ...Option) *Session {
	s := &Session{
		conn: conn,
		slot: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
	for _, o := range opts {
		o(s)
	}
	go s.read()
	return s
}

func (s *Session) read() {
	for {
		f, err := s.conn.ReadFrame()
		if err != nil {
			s.err = fmt.Errorf("%w: %w", ErrClosed, err)
			close(s.done)
			return
		}
		s.see(wire.FromHub, f)
		s.mu.Lock()
		x := s.active
		s.mu.Unlock()
		if x != nil {
			handled, done, err := x.c.Collect(f)
			if handled {
				if done || err != nil {
					s.finish(x, err)
				}
				continue
			}
		}
		if s.unsolicited != nil {
			s.unsolicited(f)
		}
	}
}

func (s *Session) finish(x *exchange, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == x {
		s.active = nil
		x.result <- err
	}
}

func (s *Session) acquire(ctx context.Context) error {
	select {
	case s.slot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return s.err
	}
}

func (s *Session) release() { <-s.slot }

// Exchange sends req and feeds incoming frames to c until c reports the
// reply complete, c fails, ctx is done or the session ends. Exchanges run
// one at a time; Exchange waits for the one in progress.
func (s *Session) Exchange(ctx context.Context, req wire.Frame, c Collector) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()

	x := &exchange{c: c, result: make(chan error, 1)}
	s.mu.Lock()
	s.active = x
	s.mu.Unlock()
	if err := s.write(req); err != nil {
		s.finish(x, nil)
		return err
	}
	select {
	case err := <-x.result:
		return err
	case <-ctx.Done():
		s.finish(x, nil)
		return ctx.Err()
	case <-s.done:
		return s.err
	}
}

// Send writes f without waiting for a reply, in turn with exchanges.
func (s *Session) Send(ctx context.Context, f wire.Frame) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	return s.write(f)
}

// Watch has h see every frame sent and received, as a tap does (see
// WithTap), until stop is called: to record what an operation exchanged
// on a session already open.
func (s *Session) Watch(h func(wire.Direction, wire.Frame)) (stop func()) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watchers == nil {
		s.watchers = map[int]func(wire.Direction, wire.Frame){}
	}
	s.watchID++
	id := s.watchID
	s.watchers[id] = h
	return func() {
		s.watchMu.Lock()
		defer s.watchMu.Unlock()
		delete(s.watchers, id)
	}
}

// see shows f to the tap and the watchers.
func (s *Session) see(d wire.Direction, f wire.Frame) {
	if s.tap != nil {
		s.tap(d, f)
	}
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	for _, h := range s.watchers {
		h(d, f)
	}
}

func (s *Session) write(f wire.Frame) error {
	select {
	case <-s.done:
		return s.err
	default:
	}
	s.see(wire.ToHub, f)
	if err := s.conn.WriteFrame(f); err != nil {
		return fmt.Errorf("session: sending %v: %w", f.Opcode(), err)
	}
	return nil
}

// Done is closed when the session ends.
func (s *Session) Done() <-chan struct{} { return s.done }

// Err is why the session ended, once Done is closed.
func (s *Session) Err() error {
	select {
	case <-s.done:
		return s.err
	default:
		return nil
	}
}

// Close ends the session and closes the connection.
func (s *Session) Close() error {
	err := s.conn.Close()
	<-s.done
	return err
}
