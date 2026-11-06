package record

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// HueRequest is the signal of a Philips Hue command (ClassHue): a request
// to the Hue bridge's API, whose address the device's record holds, as
// the app wrote 100 of them to our X2 (2026-11-06), and the hub sent them:
//
//	3c               path length
//	00 21            length of the rest
//	api/KEY/groups/3/action
//	Content-Length:16\n\n{\n"on": true\n}
//
// The method, PUT, and the rest of the request are the hub's. Whether the
// first three bytes are a byte and a big-endian 16-bit length, as the
// hub's lengths are elsewhere, or a little-endian 16-bit one and a byte,
// the app's paths and bodies are too short to tell.
type HueRequest struct {
	// Path is the API path, without a leading slash: api/KEY/lights/27/state.
	Path string
	// Body is the request's body, JSON: {"on": true}.
	Body string
	// Length is the Content-Length written, 0 for the body's length. The
	// app writes 2 or 3 more than that ({"on": true} on two lines is 14
	// bytes, its length 16), which the hub and the bridge take.
	Length int
}

// Data is the request as the hub stores it.
func (r HueRequest) Data() ([]byte, error) {
	if r.Path == "" || strings.HasPrefix(r.Path, "/") || strings.ContainsAny(r.Path, " \r\n") {
		return nil, fmt.Errorf("record: Hue path %q: api/KEY/... without spaces", r.Path)
	}
	n := r.Length
	if n == 0 {
		n = len(r.Body)
	}
	rest := "Content-Length:" + strconv.Itoa(n) + "\n\n" + r.Body
	if len(r.Path) > 0xFF || len(rest) > 0xFF {
		return nil, fmt.Errorf("record: Hue request of a %d-byte path and a %d-byte body: the most is %d and %d", len(r.Path), len(r.Body), 0xFF, 0xFF-len("Content-Length:000\n\n"))
	}
	out := []byte{byte(len(r.Path))}
	out = binary.BigEndian.AppendUint16(out, uint16(len(rest)))
	out = append(out, r.Path...)
	return append(out, rest...), nil
}

var errNotHue = errors.New("record: not a Hue request as the hub stores it")

// ParseHueRequest decodes a Hue command's signal. It fails unless Data
// gives the same bytes back.
func ParseHueRequest(data []byte) (HueRequest, error) {
	if len(data) < 3 {
		return HueRequest{}, errNotHue
	}
	n, m := int(data[0]), int(binary.BigEndian.Uint16(data[1:]))
	if len(data) != 3+n+m {
		return HueRequest{}, errNotHue
	}
	r := HueRequest{Path: string(data[3 : 3+n])}
	rest := data[3+n:]
	head, body, ok := strings.Cut(string(rest), "\n\n")
	length, found := strings.CutPrefix(head, "Content-Length:")
	if !ok || !found {
		return HueRequest{}, errNotHue
	}
	l, err := strconv.Atoi(length)
	if err != nil {
		return HueRequest{}, errNotHue
	}
	r.Body = body
	if l != len(body) {
		r.Length = l
	}
	if again, err := r.Data(); err != nil || string(again) != string(data) {
		return HueRequest{}, errNotHue
	}
	return r, nil
}
