package hub

import (
	"context"
	"fmt"
	"sync"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// Learn has the hub learn an IR code from a remote pointed at it, and
// returns the code as a command's data (a record.RawIR's encoding), to
// write to a device with WriteCommand. It arms learning (family 0x04,
// empty), as the app does, then waits until ctx is done for the hub's
// code: pages of family 0x06 laid out as a command write, of device 0
// and an empty label. Each attempt the hub could not read (family 0x68:
// a key held rather than pressed once, on our X2) calls failed, if not
// nil, and the hub listens again (2026-11-07, the app's learning
// through the proxy). When the hub closes its window (family 0x05) Learn
// arms it again, as the app does; when ctx is done first it leaves
// learning (family 0x05, empty), as the app does when its dialog closes.
func (c *Client) Learn(ctx context.Context, failed func()) ([]byte, error) {
	return c.learn(ctx, wire.Frame{Family: wire.FamLearnEnter}, wire.Frame{Family: wire.FamLearnExit}, failed)
}

// RF433 and RF315 are the frequencies, in MHz, the X2 learns RF codes at.
const (
	RF433 = 433
	RF315 = 315
)

// LearnRF has the hub learn an RF code at mhz (RF433 or RF315) from a
// remote near it, as Learn does an IR code, and returns it as a command's
// data, to write with the class of its frequency (record.ClassRF433,
// record.ClassRF315). The app arms it with family 0x70 [01, f] and leaves
// it with 0x70 [00, f], f 0 for 433 MHz and 1 for 315, and takes the code
// as it takes an IR code (the app's code, 2026-11-12; not yet tried with
// an RF remote). The X2, and the X1S from firmware 4, learn RF.
func (c *Client) LearnRF(ctx context.Context, mhz int, failed func()) ([]byte, error) {
	var f byte
	switch mhz {
	case RF433:
	case RF315:
		f = 1
	default:
		return nil, fmt.Errorf("hub: RF at %d MHz: the hub learns at 433 and 315", mhz)
	}
	return c.learn(ctx, wire.Frame{Family: wire.FamRFLearn, Payload: []byte{1, f}}, wire.Frame{Family: wire.FamRFLearn, Payload: []byte{0, f}}, failed)
}

// RFClass is the command class of RF codes at mhz.
func RFClass(mhz int) (record.Class, error) {
	switch mhz {
	case RF433:
		return record.ClassRF433, nil
	case RF315:
		return record.ClassRF315, nil
	}
	return 0, fmt.Errorf("hub: no RF at %d MHz: 433 or 315", mhz)
}

func (c *Client) learn(ctx context.Context, enter, exit wire.Frame, failed func()) ([]byte, error) {
	if err := c.Writable(); err != nil {
		return nil, err
	}
	var (
		mu      sync.Mutex
		pages   wire.Burst
		pageErr error
	)
	learned := make(chan struct{})
	expired := make(chan struct{}, 1)
	var learnedOnce sync.Once
	stop := c.s.Watch(func(d wire.Direction, f wire.Frame) {
		if d != wire.FromHub {
			return
		}
		switch f.Family {
		case wire.FamLearnData:
			mu.Lock()
			done, err := pages.Add(f.Payload)
			if err != nil && pageErr == nil {
				pageErr = err
			}
			mu.Unlock()
			if done || err != nil {
				learnedOnce.Do(func() { close(learned) })
			}
		case wire.FamLearnFailed:
			if failed != nil {
				failed()
			}
		case wire.FamLearnExpired:
			select {
			case expired <- struct{}{}:
			default:
			}
		}
	})
	defer stop()
	// The hub leaves learning once it sends a code (the app sends no
	// 0x05 then); on any other way out, Learn leaves it.
	learnt := false
	defer func() {
		if !learnt {
			_ = c.s.Send(context.WithoutCancel(ctx), exit)
		}
	}()
	arm := func() error {
		sctx, cancel := c.bound(ctx)
		defer cancel()
		if err := c.s.Exchange(sctx, enter, session.Status()); err != nil {
			return fmt.Errorf("hub: arming learning: %w", err)
		}
		return nil
	}
	if err := arm(); err != nil {
		return nil, err
	}
wait:
	for {
		select {
		case <-learned:
			learnt = true
			break wait
		case <-expired:
			if err := arm(); err != nil {
				return nil, err
			}
		case <-c.s.Done():
			return nil, fmt.Errorf("hub: learning: %w", c.s.Err())
		case <-ctx.Done():
			return nil, fmt.Errorf("hub: learning: %w", ctx.Err())
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if pageErr != nil {
		return nil, fmt.Errorf("hub: the learned code: %w", pageErr)
	}
	bodies, err := pages.Records()
	if err != nil {
		return nil, fmt.Errorf("hub: the learned code: %w", err)
	}
	if len(bodies) != 1 {
		return nil, fmt.Errorf("hub: the learned code came as %d records, want 1", len(bodies))
	}
	_, _, data, err := record.ParseCommandWrite(c.variant, bodies[0])
	if err != nil {
		return nil, fmt.Errorf("hub: the learned code: %w", err)
	}
	return data, nil
}
