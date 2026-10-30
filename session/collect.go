package session

import (
	"errors"
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

// Collector receives the frames that arrive during an exchange. Collect
// reports whether the frame belongs to the reply (handled) and whether the
// reply is complete (done); an error ends the exchange with it. Collectors
// run on the session's reader goroutine; their results are safe to read
// once Exchange returns.
type Collector interface {
	Collect(f wire.Frame) (handled, done bool, err error)
}

// CollectorFunc adapts a function to a Collector.
type CollectorFunc func(f wire.Frame) (handled, done bool, err error)

func (c CollectorFunc) Collect(f wire.Frame) (bool, bool, error) { return c(f) }

// ErrRejected reports a request the hub refused with a non-zero status.
var ErrRejected = errors.New("session: rejected by the hub")

// RejectedError is a refusal with the hub's status byte.
type RejectedError struct{ Status byte }

func (e *RejectedError) Error() string {
	return fmt.Sprintf("session: rejected by the hub (status 0x%02X)", e.Status)
}

func (e *RejectedError) Is(target error) bool { return target == ErrRejected }

// statusVerdict reads a status ack: nil when accepted.
func statusVerdict(f wire.Frame) error {
	if len(f.Payload) == 0 {
		return errors.New("session: status ack without a status")
	}
	if f.Payload[0] != 0 {
		return &RejectedError{Status: f.Payload[0]}
	}
	return nil
}

// Status waits for a status ack (family 0x03): accepted, or rejected with
// its status.
func Status() Collector {
	return CollectorFunc(func(f wire.Frame) (bool, bool, error) {
		if f.Family != wire.FamStatusAck {
			return false, false, nil
		}
		return true, true, statusVerdict(f)
	})
}

// Ack waits for a reply of family fam and keeps it in Frame. A rejecting
// status ack ends the wait with its error. When Echo is set, only a reply
// whose first payload byte is *Echo counts.
type Ack struct {
	Family wire.Family
	Echo   *byte
	Frame  wire.Frame
}

func (a *Ack) Collect(f wire.Frame) (bool, bool, error) {
	if f.Family == wire.FamStatusAck && len(f.Payload) > 0 && f.Payload[0] != 0 {
		return true, true, statusVerdict(f)
	}
	if f.Family != a.Family {
		return false, false, nil
	}
	if a.Echo != nil && (len(f.Payload) == 0 || f.Payload[0] != *a.Echo) {
		return false, false, nil
	}
	a.Frame = f
	return true, true, nil
}

// Burst collects a reply of records of one family (see wire.Burst). A
// rejecting status ack ends it with its error: hubs answer that way for
// data they do not hold.
type Burst struct {
	Family wire.Family
	burst  wire.Burst
}

func (b *Burst) Collect(f wire.Frame) (bool, bool, error) {
	if f.Family == wire.FamStatusAck && len(f.Payload) > 0 && f.Payload[0] != 0 {
		return true, true, statusVerdict(f)
	}
	if f.Family != b.Family {
		return false, false, nil
	}
	done, err := b.burst.Add(f.Payload)
	if err != nil {
		return true, true, fmt.Errorf("session: %v reply: %w", f.Family, err)
	}
	return true, done, nil
}

// Records returns the records' bodies once the burst is complete.
func (b *Burst) Records() ([][]byte, error) { return b.burst.Records() }
