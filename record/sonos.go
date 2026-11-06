package record

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// SonosRequest is the signal of a Sonos command (ClassSonos): a UPnP
// control request the hub POSTs to port 1400 of the address the device's
// record holds. It is laid out as a Hue request is, a path and the rest
// of the request after the hub's first two lines:
//
//	22               path length
//	01 4b            length of the rest
//	MediaRenderer/RenderingControl/Control
//	SOAPACTION: "urn:...:RenderingControl:1#SetVolume"\nContent-Length:323\n\n<s:Envelope ...
//
// Our X2 sent that as POST /MediaRenderer/RenderingControl/Control, Host,
// then the rest as it is (2026-11-06). The app's Volume up and down are
// the paths MediaRenderer/VolumeUp/Control and VolumeDown/Control with a
// volume as the rest ("30"): for those the hub sent the last SetVolume
// it had sent, one up or down.
type SonosRequest struct {
	// Path is the control path, without a leading slash.
	Path string
	// Action is the SOAPACTION, "urn:schemas-upnp-org:service:SVC:1#ACT",
	// or empty for a rest that is Body alone (the volume steps).
	Action string
	// Body is the SOAP envelope, or what a step's rest is.
	Body string
	// Length is the Content-Length written, 0 for the body's length (the
	// app writes some that are not).
	Length int
	// Space is the space the app writes after the action's closing quote
	// in most of its commands, kept so they read back as written.
	Space bool
}

// Data is the request as the hub stores it.
func (r SonosRequest) Data() ([]byte, error) {
	if r.Path == "" || strings.HasPrefix(r.Path, "/") || strings.ContainsAny(r.Path, " \r\n") || len(r.Path) > 0xFF {
		return nil, fmt.Errorf("record: Sonos path %q: SERVICE/Control without spaces, %d bytes at most", r.Path, 0xFF)
	}
	if strings.ContainsAny(r.Action, "\"\r\n") {
		return nil, fmt.Errorf("record: Sonos action %q", r.Action)
	}
	rest := r.Body
	if r.Action != "" {
		n := r.Length
		if n == 0 {
			n = len(r.Body)
		}
		space := ""
		if r.Space {
			space = " "
		}
		rest = "SOAPACTION: \"" + r.Action + "\"" + space + "\nContent-Length:" + strconv.Itoa(n) + "\n\n" + r.Body
	}
	if len(rest) > 0xFFFF {
		return nil, fmt.Errorf("record: Sonos request of %d bytes, the most is %d", len(rest), 0xFFFF)
	}
	out := []byte{byte(len(r.Path))}
	out = binary.BigEndian.AppendUint16(out, uint16(len(rest)))
	out = append(out, r.Path...)
	return append(out, rest...), nil
}

var errNotSonos = errors.New("record: not a Sonos request as the hub stores it")

// ParseSonosRequest decodes a Sonos command's signal. It fails unless
// Data gives the same bytes back.
func ParseSonosRequest(data []byte) (SonosRequest, error) {
	if len(data) < 3 {
		return SonosRequest{}, errNotSonos
	}
	n, m := int(data[0]), int(binary.BigEndian.Uint16(data[1:]))
	if len(data) != 3+n+m {
		return SonosRequest{}, errNotSonos
	}
	r := SonosRequest{Path: string(data[3 : 3+n])}
	rest := string(data[3+n:])
	if action, ok := strings.CutPrefix(rest, "SOAPACTION: \""); ok {
		head, body, ok := strings.Cut(action, "\n\n")
		if !ok {
			return SonosRequest{}, errNotSonos
		}
		quoted, length, ok := strings.Cut(head, "\nContent-Length:")
		if !ok {
			return SonosRequest{}, errNotSonos
		}
		if q, ok := strings.CutSuffix(quoted, "\" "); ok {
			r.Action, r.Space = q, true
		} else if r.Action, ok = strings.CutSuffix(quoted, "\""); !ok {
			return SonosRequest{}, errNotSonos
		}
		l, err := strconv.Atoi(length)
		if err != nil {
			return SonosRequest{}, errNotSonos
		}
		r.Body = body
		if l != len(body) {
			r.Length = l
		}
	} else {
		r.Body = rest
	}
	if again, err := r.Data(); err != nil || string(again) != string(data) {
		return SonosRequest{}, errNotSonos
	}
	return r, nil
}
