package record

import (
	"errors"
	"fmt"
	"strings"
)

// RokuRequest is the signal of a Roku command (ClassRoku): a path of the
// Roku's External Control Protocol, which the hub POSTs to port 8060 of
// the address the device's record holds, as our X2 did (2026-11-06):
//
//	0d               path length
//	keypress/Home
//
// became POST /keypress/Home with a Host header and nothing else.
type RokuRequest struct {
	// Path is the ECP path, without a leading slash: keypress/Home,
	// launch/12.
	Path string
}

// Data is the request as the hub stores it.
func (r RokuRequest) Data() ([]byte, error) {
	if r.Path == "" || strings.HasPrefix(r.Path, "/") || strings.ContainsAny(r.Path, " \r\n") || len(r.Path) > 0xFF {
		return nil, fmt.Errorf("record: Roku path %q: keypress/KEY or launch/ID, without spaces, %d bytes at most", r.Path, 0xFF)
	}
	return append([]byte{byte(len(r.Path))}, r.Path...), nil
}

var errNotRoku = errors.New("record: not a Roku request as the hub stores it")

// ParseRokuRequest decodes a Roku command's signal. It fails unless Data
// gives the same bytes back.
func ParseRokuRequest(data []byte) (RokuRequest, error) {
	if len(data) < 2 || int(data[0]) != len(data)-1 {
		return RokuRequest{}, errNotRoku
	}
	r := RokuRequest{Path: string(data[1:])}
	if _, err := r.Data(); err != nil {
		return RokuRequest{}, errNotRoku
	}
	return r, nil
}
