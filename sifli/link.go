package sifli

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

// The serial transport SiFli's devices speak over the data
// characteristic: each write is a frame of a category (1, updates), a
// flag (0 a whole message, 1 its first fragment, 2 a middle one, 3 its
// last), the message's length on whole and first frames, and data. A
// message is an id, a length and a payload, little-endian.
const category = 1

const (
	flagWhole = iota
	flagFirst
	flagMiddle
	flagLast
)

// link exchanges messages over a Conn.
type link struct {
	c Conn
	// msg is a message being reassembled from fragments.
	msg []byte
}

// frames are the writes of a message for an MTU: whole if it fits in
// one, else a first fragment and the rest, as the app cuts them.
func frames(msg []byte, mtu int) [][]byte {
	room := min(mtu, MaxMTU) - 3 // an ATT write's header
	if len(msg) <= room-4 {
		return [][]byte{append([]byte{category, flagWhole, byte(len(msg)), byte(len(msg) >> 8)}, msg...)}
	}
	out := [][]byte{append([]byte{category, flagFirst, byte(len(msg)), byte(len(msg) >> 8)}, msg[:room-4]...)}
	for rest := msg[room-4:]; len(rest) > 0; {
		n := min(len(rest), room-2)
		flag := byte(flagMiddle)
		if n == len(rest) {
			flag = flagLast
		}
		out = append(out, append([]byte{category, flag}, rest[:n]...))
		rest = rest[n:]
	}
	return out
}

// send writes message id.
func (l *link) send(id int, payload []byte) error {
	msg := binary.LittleEndian.AppendUint16(nil, uint16(id))
	msg = binary.LittleEndian.AppendUint16(msg, uint16(len(payload)))
	for _, f := range frames(append(msg, payload...), l.c.MTU()) {
		if err := l.c.Write(f); err != nil {
			return fmt.Errorf("sifli: writing message %d: %w", id, err)
		}
	}
	return nil
}

// receive is the device's next message: its id and payload.
func (l *link) receive(ctx context.Context) (int, []byte, error) {
	for {
		n, err := l.c.Notification(ctx)
		if err != nil {
			return 0, nil, err
		}
		if len(n) < 2 || n[0] != category {
			continue // another category's
		}
		switch n[1] {
		case flagWhole, flagFirst:
			if len(n) < 4 {
				return 0, nil, fmt.Errorf("sifli: a frame % x", n)
			}
			l.msg = append([]byte(nil), n[4:]...)
		case flagMiddle, flagLast:
			l.msg = append(l.msg, n[2:]...)
		}
		if n[1] == flagFirst || n[1] == flagMiddle {
			continue
		}
		msg := l.msg
		l.msg = nil
		if len(msg) < 4 || int(binary.LittleEndian.Uint16(msg[2:])) != len(msg)-4 {
			return 0, nil, fmt.Errorf("sifli: a message % x", msg)
		}
		return int(binary.LittleEndian.Uint16(msg)), msg[4:], nil
	}
}

// await is the payload of the device's answer id, within timeout: an
// error if it answers with a non-zero result, or aborts.
func (l *link) await(ctx context.Context, id int, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		got, p, err := l.receive(ctx)
		if err != nil {
			return nil, fmt.Errorf("sifli: waiting for answer %d: %w", id, err)
		}
		if got == msgAbort {
			reason := 0
			if len(p) >= 4 {
				reason = int(binary.LittleEndian.Uint16(p[2:]))
			}
			return nil, &Error{Msg: msgAbort, Result: reason}
		}
		if got != id {
			continue // not what the update waits for
		}
		if len(p) < 2 {
			return nil, fmt.Errorf("sifli: answer %d of %d bytes", id, len(p))
		}
		if r := binary.LittleEndian.Uint16(p); r != 0 {
			return nil, &Error{Msg: id, Result: int(r)}
		}
		return p, nil
	}
}

// ask sends message id and awaits answer.
func (l *link) ask(ctx context.Context, id int, payload []byte, answer int, timeout time.Duration) ([]byte, error) {
	if err := l.send(id, payload); err != nil {
		return nil, err
	}
	return l.await(ctx, answer, timeout)
}
