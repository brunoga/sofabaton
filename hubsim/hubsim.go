// Package hubsim is a simulated hub for tests: it answers requests with
// frames that handlers build, and can push unsolicited frames.
package hubsim

import (
	"errors"
	"io"
	"net"
	"sync"

	"github.com/brunoga/sofabaton/transport"
	"github.com/brunoga/sofabaton/wire"
)

// Handler answers one request with the frames to send back, in order.
type Handler func(req wire.Frame) []wire.Frame

// Hub is a simulated hub.
type Hub struct {
	mu       sync.Mutex
	handlers map[wire.Family]Handler
	requests []wire.Frame
	conn     *transport.Conn
}

// New returns a hub that answers nothing.
func New() *Hub { return &Hub{handlers: map[wire.Family]Handler{}} }

// Handle sets the handler for requests of family fam.
func (h *Hub) Handle(fam wire.Family, handler Handler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[fam] = handler
}

// Requests returns the requests received so far.
func (h *Hub) Requests() []wire.Frame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]wire.Frame(nil), h.requests...)
}

// Push sends frames to the client outside any exchange.
func (h *Hub) Push(frames ...wire.Frame) error {
	h.mu.Lock()
	c := h.conn
	h.mu.Unlock()
	if c == nil {
		return errors.New("hubsim: no client")
	}
	for _, f := range frames {
		if err := c.WriteFrame(f); err != nil {
			return err
		}
	}
	return nil
}

// Serve answers requests on c until the client goes away.
func (h *Hub) Serve(c *transport.Conn) error {
	h.mu.Lock()
	h.conn = c
	h.mu.Unlock()
	for {
		req, err := c.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		h.mu.Lock()
		h.requests = append(h.requests, req)
		handler := h.handlers[req.Family]
		h.mu.Unlock()
		if handler == nil {
			continue
		}
		for _, f := range handler(req) {
			if err := c.WriteFrame(f); err != nil {
				return err
			}
		}
	}
}

// Pipe connects a client to h in memory and serves it. Closing the client
// stops the hub.
func Pipe(h *Hub) *transport.Conn {
	client, hub := net.Pipe()
	go func() { _ = h.Serve(transport.NewConn(hub)) }()
	return transport.NewConn(client)
}

// Reply returns the frames of a reply of family fam carrying records:
// each record's body gets the record count in its first byte and its page
// count after it, and goes out in pages behind the record's index.
func Reply(fam wire.Family, records ...[]byte) []wire.Frame {
	var out []wire.Frame
	for i, rec := range records {
		body := append([]byte{byte(len(records)), 0, 0}, rec...)
		n := wire.PageCount(len(body))
		body[1], body[2] = byte(n>>8), byte(n)
		for _, p := range wire.Pages(byte(i+1), body) {
			out = append(out, wire.Frame{Family: fam, Payload: p})
		}
	}
	return out
}

// Status returns a status ack with status s (0 accepts).
func Status(s byte) wire.Frame {
	return wire.Frame{Family: wire.FamStatusAck, Payload: []byte{s}}
}
