package record

import (
	"bytes"
	"fmt"
	"net"
)

// RemoteInfo is a remote the X2 hub knows, as it answers ReqRemoteList
// (family 0x2F), one frame per remote. Our X2's one remote, "Remote 1"
// (2026-11-06):
//
//	01 01 | 00 00 08 00 | 04 | 20 25 06 10 | 09 | 00 01 00 | fa ba 25 00 0f 4e | "Remote 1"…
//	count, index [guess] | ? | hardware | batch (BCD) | firmware | ? | Bluetooth address | name
//
// The firmware byte went from 8 to 9 with the app's update of the remote,
// and the hardware byte is the HW_version of its update's control packet.
type RemoteInfo struct {
	// Count is how many remotes the hub has, Index this one's, from 1.
	Count, Index int
	Hardware     byte
	Batch        [4]byte
	Firmware     byte
	// Addr is the remote's Bluetooth address, which its firmware updates
	// connect to.
	Addr net.HardwareAddr
	Name string
}

// remoteInfoLen is the record's bytes before its name.
const remoteInfoLen = 21

// ParseRemoteInfo decodes a RemoteStatus payload.
func ParseRemoteInfo(p []byte) (*RemoteInfo, error) {
	if len(p) < remoteInfoLen || p[0] == 0 || p[1] == 0 || p[1] > p[0] {
		return nil, fmt.Errorf("record: remote status % x", p)
	}
	r := &RemoteInfo{Count: int(p[0]), Index: int(p[1]), Hardware: p[6], Firmware: p[11], Addr: net.HardwareAddr(append([]byte(nil), p[15:21]...))}
	copy(r.Batch[:], p[7:11])
	name := p[remoteInfoLen:]
	if i := bytes.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	r.Name = string(name)
	return r, nil
}

// BatchDate is the remote's production batch as a date, 2025-06-10.
func (r *RemoteInfo) BatchDate() string {
	return fmt.Sprintf("%02x%02x-%02x-%02x", r.Batch[0], r.Batch[1], r.Batch[2], r.Batch[3])
}
