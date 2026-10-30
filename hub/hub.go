// Package hub is the client for a Sofabaton hub: it connects, identifies
// the hub, and reads its configuration as typed records.
package hub

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/transport"
	"github.com/brunoga/sofabaton/wire"
)

// DefaultTimeout bounds a request whose context has no deadline.
const DefaultTimeout = 15 * time.Second

// Client is an open connection to one hub.
type Client struct {
	s       *session.Session
	info    *record.HubInfo
	variant wire.Variant
	timeout time.Duration
}

// Options configures a Client.
type Options struct {
	// Dialer connects to the hub; the zero value uses the defaults.
	Dialer transport.Dialer
	// Timeout bounds requests whose context has no deadline (default
	// DefaultTimeout).
	Timeout time.Duration
	// Session options, such as a tap for logging.
	Session []session.Option
}

// Dial connects to the hub at addr and identifies it.
func Dial(ctx context.Context, addr netip.Addr, opts Options) (*Client, error) {
	conn, err := opts.Dialer.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}
	return Open(ctx, conn, opts)
}

// Open starts a client on an established connection (see Dial) and
// identifies the hub. The client owns conn.
func Open(ctx context.Context, conn session.FrameConn, opts Options) (*Client, error) {
	c := &Client{s: session.New(conn, opts.Session...), timeout: opts.Timeout}
	if c.timeout <= 0 {
		c.timeout = DefaultTimeout
	}
	info, err := c.readInfo(ctx)
	if err != nil {
		_ = c.s.Close()
		return nil, err
	}
	if info.Variant == 0 {
		_ = c.s.Close()
		return nil, fmt.Errorf("hub: %w: model code 0x%02X", wire.ErrUnknownVariant, info.Model)
	}
	c.info, c.variant = info, info.Variant
	return c, nil
}

// Close ends the session.
func (c *Client) Close() error { return c.s.Close() }

// Info is the hub's identity, read when the client opened.
func (c *Client) Info() *record.HubInfo { return c.info }

// Variant is the hub's model line.
func (c *Client) Variant() wire.Variant { return c.variant }

// Session is the underlying session, for requests this package does not
// cover yet.
func (c *Client) Session() *session.Session { return c.s }

func (c *Client) bound(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.timeout)
}

func (c *Client) readInfo(ctx context.Context) (*record.HubInfo, error) {
	if c.timeout <= 0 {
		c.timeout = DefaultTimeout
	}
	ctx, cancel := c.bound(ctx)
	defer cancel()
	a := &session.Ack{Family: wire.FamBanner}
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamReqBanner}, a); err != nil {
		return nil, fmt.Errorf("hub: reading the banner: %w", err)
	}
	return record.ParseHubInfo(a.Frame.Payload)
}

// Records sends req and returns the raw bodies of the records the hub
// answers with in family fam. The typed readers are built on it; it is
// exported for dumps that keep the hub's exact bytes.
func (c *Client) Records(ctx context.Context, req wire.Frame, fam wire.Family) ([][]byte, error) {
	ctx, cancel := c.bound(ctx)
	defer cancel()
	b := &session.Burst{Family: fam}
	if err := c.s.Exchange(ctx, req, b); err != nil {
		return nil, fmt.Errorf("hub: %v: %w", req.Family.Name(wire.ToHub), err)
	}
	return b.Records()
}

// ErrNotFound reports data the hub does not hold, such as the inputs of a
// device without an inputs page.
var ErrNotFound = errors.New("hub: not found")

func notFound(err error) error {
	if errors.Is(err, session.ErrRejected) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
