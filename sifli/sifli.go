// Package sifli updates the firmware of a device built on a SiFli
// Semiconductor chip over Bluetooth Low Energy, as the Sofabaton app
// updates the X2 remote with SiFli's library ("OTA V3"): seen in the
// phone's Bluetooth log of the app updating our remote from firmware 8
// to 9 (2026-11-06; docs/protocol/remote-firmware-update.md).
//
// The package speaks the protocol over a Conn, a GATT link to the
// device's data characteristic that the caller provides (cmd/sbx has one
// for Linux); it opens no Bluetooth connection itself.
package sifli

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// The device's serial-transport service and its data characteristic,
// which carries the update both ways: writes without response from the
// phone, notifications from the device.
const (
	ServiceUUID = "00000000-0000-0000-6473-5f696c666973"
	DataUUID    = "00000000-0000-0200-6473-5f696c666973"
)

// MaxMTU is the most ATT MTU the app uses, whatever the link allows.
const MaxMTU = 247

// Conn is a link to the device's data characteristic.
type Conn interface {
	// Write writes value to the characteristic without response.
	Write(value []byte) error
	// Notification is the characteristic's next notification.
	Notification(ctx context.Context) ([]byte, error)
	// MTU is the link's ATT MTU.
	MTU() int
	Close() error
}

// Dial connects to the device and enables the characteristic's
// notifications. Update dials twice: the device restarts into its updater
// between.
type Dial func(ctx context.Context) (Conn, error)

// Package is an update: the control packet the vendor signed, then the
// images, by image id from 0, each already cut into slices by SiFli's
// packager (a SHA-256, an offset and up to 512 bytes of data).
type Package struct {
	Ctrl   []byte
	Images [][]byte
}

// Progress is where an update is.
type Progress struct {
	// Step says what is going on: "restarting the device into its
	// updater", "sending image 2 of 5"…
	Step string
	// Done and Total are the image bytes sent and to send.
	Done, Total int
}

// Message ids (the app's names).
const (
	msgVersionReq       = 40
	msgVersionRsp       = 41
	msgReboot           = 42
	msgFileInitReq      = 43
	msgFileInitRsp      = 44
	msgInitCompletedReq = 45
	msgInitCompletedRsp = 46
	msgFileStart        = 47
	msgFileStartRsp     = 48
	msgFilePacket       = 49
	msgFilePacketRsp    = 50
	msgFileEnd          = 51
	msgFileEndRsp       = 52
	msgTotalEnd         = 53
	msgTotalEndRsp      = 54
	msgAbort            = 55
)

// File types.
const (
	fileCtrl  = 2
	fileImage = 3
)

// otaType is the app's "OTA V3 type" for a control packet of dfu_ID 1, as
// the X2 remote's are; 11 is for dfu_ID 2.
const otaType = 10

// Timeouts: most answers come within a second; an image's start waits for
// the device to erase (3.6 s seen), the restart about 7 s.
var (
	answerTimeout  = 20 * time.Second
	startTimeout   = 60 * time.Second
	restartTimeout = 60 * time.Second
)

// RestartPause is how long Update waits between tries to connect again
// while the device restarts into its updater (about 7 s on our remote).
var RestartPause = 5 * time.Second

// Error is the device's refusal: a non-zero result, or an abort.
type Error struct {
	Msg    int
	Result int
}

func (e *Error) Error() string {
	if e.Msg == msgAbort {
		return fmt.Sprintf("sifli: the device aborted the update (reason %d)", e.Result)
	}
	return fmt.Sprintf("sifli: the device answered message %d with error %d", e.Msg, e.Result)
}

// Update sends the device the package, as the app does: it asks the
// device to restart into its updater, connects again, sends the control
// packet, then each image in windows of slices the device acknowledges,
// and ends the update; the device then installs it and restarts. The
// device checks each slice and installs code only once all of it is in.
func Update(ctx context.Context, dial Dial, p Package, progress func(Progress)) error {
	if len(p.Ctrl) == 0 || len(p.Images) == 0 {
		return errors.New("sifli: a package needs a control packet and images")
	}
	say := func(pr Progress) {
		if progress != nil {
			progress(pr)
		}
	}
	total := 0
	for _, img := range p.Images {
		total += len(img)
	}

	say(Progress{Step: "asking the device to restart into its updater", Total: total})
	c, err := dial(ctx)
	if err != nil {
		return err
	}
	s := &link{c: c}
	rsp, err := s.ask(ctx, msgVersionReq, le16(4, otaType), msgVersionRsp, answerTimeout)
	if err != nil {
		_ = c.Close()
		return err
	}
	if len(rsp) >= 6 && binary.LittleEndian.Uint16(rsp[4:]) == 1 {
		// The device runs its app: it restarts into the updater.
		err := s.send(msgReboot, []byte{1})
		_ = c.Close()
		if err != nil {
			return err
		}
		say(Progress{Step: "waiting for the device to restart", Total: total})
		if c, err = redial(ctx, dial); err != nil {
			return err
		}
		s = &link{c: c}
	}
	defer func() { _ = s.c.Close() }()

	init := le16(4, otaType)
	init = binary.LittleEndian.AppendUint32(init, uint32(len(p.Images)+1))
	init = binary.LittleEndian.AppendUint32(init, uint32(total))
	init = append(init, make([]byte, 16)...) // an MD5 the app leaves zero
	init = append(init, 2, 1)
	rsp, err = s.ask(ctx, msgFileInitReq, init, msgFileInitRsp, answerTimeout)
	if err != nil {
		return err
	}
	if len(rsp) < 18 {
		return fmt.Errorf("sifli: a file init answer of %d bytes", len(rsp))
	}
	window, sliceLen := int(rsp[5]), int(binary.LittleEndian.Uint16(rsp[16:]))
	if window == 0 || sliceLen == 0 {
		return fmt.Errorf("sifli: the device asks for windows of %d slices of %d bytes", window, sliceLen)
	}
	if _, err := s.ask(ctx, msgInitCompletedReq, make([]byte, 8), msgInitCompletedRsp, answerTimeout); err != nil {
		return err
	}

	say(Progress{Step: "sending the control packet", Total: total})
	if _, err := s.ask(ctx, msgFileStart, append(le16(fileCtrl), p.Ctrl...), msgFileStartRsp, answerTimeout); err != nil {
		return err
	}
	if _, err := s.ask(ctx, msgFileEnd, []byte{1, fileCtrl, 0, 0}, msgFileEndRsp, answerTimeout); err != nil {
		return err
	}

	done := 0
	for id, img := range p.Images {
		step := fmt.Sprintf("sending image %d of %d", id+1, len(p.Images))
		say(Progress{Step: step, Done: done, Total: total})
		slices := (len(img) + sliceLen - 1) / sliceLen
		start := le16(fileImage)
		start = binary.LittleEndian.AppendUint32(start, uint32(len(img)))
		start = binary.LittleEndian.AppendUint32(start, uint32(slices))
		start = binary.LittleEndian.AppendUint16(start, uint16(id))
		start = append(start, 0, 0, 0, 0, 0) // offset, resume
		if _, err := s.ask(ctx, msgFileStart, start, msgFileStartRsp, startTimeout); err != nil {
			return err
		}
		for i := range slices {
			slice := img[i*sliceLen : min(len(img), (i+1)*sliceLen)]
			pkt := binary.LittleEndian.AppendUint32(nil, uint32(i+1))
			pkt = binary.LittleEndian.AppendUint16(pkt, uint16(id))
			pkt = binary.LittleEndian.AppendUint16(pkt, uint16(len(slice)))
			pkt = append(pkt, fileImage)
			if err := s.send(msgFilePacket, append(pkt, slice...)); err != nil {
				return err
			}
			done += len(slice)
			// The device acknowledges each window, and the last slice.
			if (i+1)%window == 0 || i+1 == slices {
				if _, err := s.await(ctx, msgFilePacketRsp, answerTimeout); err != nil {
					return fmt.Errorf("%w (image %d, slice %d)", err, id, i+1)
				}
				say(Progress{Step: step, Done: done, Total: total})
			}
		}
		more := byte(1)
		if id == len(p.Images)-1 {
			more = 0
		}
		if _, err := s.ask(ctx, msgFileEnd, []byte{more, fileImage, byte(id), byte(id >> 8)}, msgFileEndRsp, answerTimeout); err != nil {
			return err
		}
	}
	if _, err := s.ask(ctx, msgTotalEnd, nil, msgTotalEndRsp, answerTimeout); err != nil {
		return err
	}
	say(Progress{Step: "the device is installing the update", Done: total, Total: total})
	return nil
}

// redial connects to the device again once it has restarted.
func redial(ctx context.Context, dial Dial) (Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, restartTimeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("sifli: the device did not come back after restarting into its updater")
		case <-time.After(RestartPause):
		}
		if c, err := dial(ctx); err == nil {
			return c, nil
		}
	}
}

func le16(vs ...uint16) []byte {
	var b []byte
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint16(b, v)
	}
	return b
}
