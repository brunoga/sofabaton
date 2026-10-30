package transport

import (
	"net"
	"sync"

	"github.com/brunoga/sofabaton/wire"
)

// Conn carries frames over a stream connection. Reads and writes may run
// concurrently; writes are serialized, one Write per frame.
type Conn struct {
	c   net.Conn
	dec *wire.Decoder

	wmu sync.Mutex
	enc *wire.Encoder
}

// NewConn wraps c.
func NewConn(c net.Conn) *Conn {
	return &Conn{c: c, dec: wire.NewDecoder(c), enc: wire.NewEncoder(c)}
}

// ReadFrame returns the next valid frame. Only one goroutine may read.
func (c *Conn) ReadFrame() (wire.Frame, error) { return c.dec.Decode() }

// WriteFrame writes f.
func (c *Conn) WriteFrame(f wire.Frame) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.enc.Encode(f)
}

// Skipped is the number of stream bytes discarded because they did not
// form valid frames.
func (c *Conn) Skipped() int64 { return c.dec.Skipped() }

// Close closes the connection.
func (c *Conn) Close() error { return c.c.Close() }

// RemoteAddr is the peer's address.
func (c *Conn) RemoteAddr() net.Addr { return c.c.RemoteAddr() }

// LocalAddr is the local address.
func (c *Conn) LocalAddr() net.Addr { return c.c.LocalAddr() }
