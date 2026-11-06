package sifli

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// step is one message of the app's update of our X2 remote, from the
// phone's Bluetooth log (testdata/sifli): a run of slices of an image
// stands for the slices and their acknowledgements.
type step struct {
	Dir     string `json:"dir"`
	ID      int    `json:"id"`
	Frags   int    `json:"frags"`
	Payload string `json:"payload"`
	Ctrl    int    `json:"ctrl"`
	Image   int    `json:"image"`
	Slices  int    `json:"slices"`
	Acks    int    `json:"acks"`
	Last    int    `json:"last"`
}

// device plays the remote of the log: it checks each message the update
// sends against the app's, and answers as the remote did.
type device struct {
	t       *testing.T
	steps   []step
	next    int
	slice   int // slices of the current run received
	acks    int
	pending [][]byte
	buf     []byte // a message being reassembled
	frags   int
	closed  bool
	dials   int
}

func (d *device) expect() *step {
	for d.next < len(d.steps) && d.steps[d.next].Dir == "R" {
		d.next++ // answers are queued as their request arrives
	}
	if d.next == len(d.steps) {
		d.t.Fatalf("message after the end")
	}
	return &d.steps[d.next]
}

// answer queues the remote's answers after the request at next.
func (d *device) answer() {
	d.next++
	for d.next < len(d.steps) && d.steps[d.next].Dir == "R" {
		p, _ := hex.DecodeString(d.steps[d.next].Payload)
		d.queue(d.steps[d.next].ID, p)
		d.next++
	}
}

func (d *device) queue(id int, p []byte) {
	msg := binary.LittleEndian.AppendUint16(nil, uint16(id))
	msg = binary.LittleEndian.AppendUint16(msg, uint16(len(p)))
	d.pending = append(d.pending, frames(append(msg, p...), MaxMTU)...)
}

type conn struct{ d *device }

func (c conn) MTU() int { return 517 } // as our remote's, which the app caps
func (c conn) Close() error {
	c.d.closed = true
	return nil
}

func (c conn) Notification(ctx context.Context) ([]byte, error) {
	if len(c.d.pending) == 0 {
		return nil, errors.New("no answer to wait for")
	}
	n := c.d.pending[0]
	c.d.pending = c.d.pending[1:]
	return n, nil
}

func (c conn) Write(f []byte) error {
	d := c.d
	if len(f) > MaxMTU-3 {
		d.t.Fatalf("a write of %d bytes", len(f))
	}
	d.frags++
	switch f[1] {
	case flagWhole, flagFirst:
		d.buf = append([]byte(nil), f[4:]...)
	default:
		d.buf = append(d.buf, f[2:]...)
	}
	if f[1] == flagFirst || f[1] == flagMiddle {
		return nil
	}
	msg, frags := d.buf, d.frags
	d.frags = 0
	id, p := int(binary.LittleEndian.Uint16(msg)), msg[4:]
	want := d.expect()
	// The log keeps a run's first slice's writes: the last is shorter.
	if id != want.ID || (frags != want.Frags && (id != msgFilePacket || binary.LittleEndian.Uint16(p[6:]) == 548)) {
		return fmt.Errorf("sent message %d in %d writes, the app %d in %d (step %d)", id, frags, want.ID, want.Frags, d.next)
	}
	switch {
	case id == msgFilePacket:
		d.slice++
		idx, img, n := binary.LittleEndian.Uint32(p), binary.LittleEndian.Uint16(p[4:]), binary.LittleEndian.Uint16(p[6:])
		if int(idx) != d.slice || int(img) != want.Image || p[8] != fileImage || int(n) != len(p)-9 {
			return fmt.Errorf("slice % x after %d of image %d", p[:9], d.slice-1, want.Image)
		}
		if d.slice%20 == 0 || d.slice == want.Slices {
			d.acks++
			d.queue(msgFilePacketRsp, make([]byte, 8))
		}
		if d.slice == want.Slices {
			if d.acks != want.Acks || int(n) != want.Last {
				return fmt.Errorf("image %d: %d acks, last slice %d; the app's %d and %d", want.Image, d.acks, n, want.Acks, want.Last)
			}
			d.slice, d.acks = 0, 0
			d.answer()
		}
		return nil
	case want.Ctrl > 0:
		if hex.EncodeToString(p[:2]) != want.Payload || len(p)-2 != want.Ctrl {
			return fmt.Errorf("ctrl start % x… of %d", p[:2], len(p)-2)
		}
	case hex.EncodeToString(p) != want.Payload:
		return fmt.Errorf("message %d: % x, the app's %s", id, p, want.Payload)
	}
	d.answer()
	return nil
}

// An update sends what the app sent our remote, in the same writes, and
// the remote's answers lead it through: the restart into the updater,
// the control packet, the five images in windows of 20 slices, the end.
func TestUpdate(t *testing.T) {
	b, err := os.ReadFile("../testdata/sifli/remote-8-to-9.json")
	if err != nil {
		t.Fatal(err)
	}
	d := &device{t: t}
	if err := json.Unmarshal(b, &d.steps); err != nil {
		t.Fatal(err)
	}
	// The package of the app's: files of the sizes it sent.
	p := Package{Ctrl: make([]byte, 1653)}
	for _, n := range []int{1323340, 44196, 12236, 791748, 1164164} {
		p.Images = append(p.Images, make([]byte, n))
	}
	RestartPause = time.Millisecond
	t.Cleanup(func() { RestartPause = 5 * time.Second })
	dial := func(context.Context) (Conn, error) {
		d.dials++
		d.closed = false
		return conn{d}, nil
	}
	var last Progress
	if err := Update(context.Background(), dial, p, func(pr Progress) { last = pr }); err != nil {
		t.Fatal(err)
	}
	if d.next != len(d.steps) || d.dials != 2 || !d.closed || last.Done != last.Total || last.Total != 3335684 {
		t.Fatalf("at step %d of %d, %d dials, closed %v, progress %+v", d.next, len(d.steps), d.dials, d.closed, last)
	}
}

// A refusal stops the update with the device's error.
func TestUpdateRefused(t *testing.T) {
	d := &device{t: t, steps: []step{{Dir: "S", ID: msgVersionReq, Frags: 1, Payload: "04000a00"}, {Dir: "R", ID: msgVersionRsp, Payload: "0700"}}}
	err := Update(context.Background(), func(context.Context) (Conn, error) { return conn{d}, nil }, Package{Ctrl: []byte{1}, Images: [][]byte{{1}}}, nil)
	var e *Error
	if !errors.As(err, &e) || e.Msg != msgVersionRsp || e.Result != 7 {
		t.Fatalf("%v", err)
	}
}
