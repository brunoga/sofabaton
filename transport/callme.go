package transport

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/brunoga/sofabaton/wire"
)

const callMeLen = 12

// CallMe returns the CALL_ME frame asking a hub to connect to addr.
func CallMe(addr netip.AddrPort) (wire.Frame, error) {
	if !addr.Addr().Is4() {
		return wire.Frame{}, fmt.Errorf("transport: CALL_ME needs an IPv4 address, not %v", addr.Addr())
	}
	p := make([]byte, 6, callMeLen)
	ip := addr.Addr().As4()
	p = append(p, ip[:]...)
	p = binary.BigEndian.AppendUint16(p, addr.Port())
	return wire.Frame{Family: wire.FamCallMe, Payload: p}, nil
}

// ParseCallMe returns the address in a CALL_ME payload. The hub ignores
// the IP field in favor of the packet's source address (and upstream saw
// the app write it little-endian, though our Android app writes it
// big-endian), so callers that have the source address should use it and
// only take the port from here. The first six bytes are not checked: our
// Android app puts the hub's id there, we send zeros, and the hub takes
// either.
func ParseCallMe(payload []byte) (netip.AddrPort, error) {
	if len(payload) != callMeLen {
		return netip.AddrPort{}, fmt.Errorf("transport: CALL_ME payload of %d bytes", len(payload))
	}
	ip := netip.AddrFrom4([4]byte(payload[6:10]))
	return netip.AddrPortFrom(ip, binary.BigEndian.Uint16(payload[10:12])), nil
}
