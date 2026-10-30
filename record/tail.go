package record

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/brunoga/sofabaton/wire"
)

const tailMarker = 0xFC

// DeviceTail is the decoded tail slot of a device record. The slot is a
// sequence of 0xFC-marked fields; activity records use the slot
// differently and are not decoded with it.
type DeviceTail struct {
	// IP is the device's address (network classes), invalid when unset.
	IP netip.Addr
	// PollTime is the hub's poll interval; HasPollTime tells whether the
	// record carries the field at all.
	HasPollTime bool
	PollTime    uint16
	// InputMode is how the device switches inputs: 0 not configured, 1
	// discrete inputs, 2 a source list, 3 not yet observed.
	InputMode byte
	// PowerMode is 1 on almost every device; it does not mean power is
	// configured (the idle-behavior byte does) but the hub relies on it.
	PowerMode  byte
	PowerStyle byte
	ShareMode  byte
	// Flag is hub-maintained (the hub may change it after a write); keep it.
	Flag byte
	// Marker is 1 once a create has been committed by its update write,
	// and on generic HTTP devices.
	Marker byte
	// HasExtra tells whether the vendor block is present; on the X2 its
	// three bytes are the IR emitter routing (see Emitters).
	HasExtra bool
	Extra    [3]byte
	// Color (X2 palette 1-12, 0 none) and BTMode (Bluetooth link: 0
	// classic, 2 BLE) exist on the 60-byte X1S/X2 tail only.
	Color  byte
	BTMode byte
}

// Tail offsets.
const (
	tIP       = 0 // FC 55 a b c d
	tPoll     = 6 // FC hi lo
	tModes    = 9 // FC input power style share
	tFlag     = 14
	tMarker   = 16
	tExtra    = 18 // FC a b c
	tColor    = 22
	tBTMode   = 23
	tKnownLen = 24
	ipTag     = 0x55
)

// wideTail is the X1S/X2 tail width; only it carries color and BT mode.
const wideTail = 60

// ParseDeviceTail decodes a device record's tail slot.
func ParseDeviceTail(tail []byte) (DeviceTail, error) {
	if len(tail) < tExtra {
		return DeviceTail{}, fmt.Errorf("record: device tail is %d bytes", len(tail))
	}
	var t DeviceTail
	if tail[tIP] == tailMarker && tail[tIP+1] == ipTag {
		if ip := netip.AddrFrom4([4]byte(tail[tIP+2 : tIP+6])); !ip.IsUnspecified() {
			t.IP = ip
		}
	}
	if tail[tPoll] == tailMarker {
		t.HasPollTime = true
		t.PollTime = binary.BigEndian.Uint16(tail[tPoll+1:])
	}
	t.InputMode = tail[tModes+1]
	t.PowerMode = tail[tModes+2]
	t.PowerStyle = tail[tModes+3]
	t.ShareMode = tail[tModes+4]
	t.Flag = tail[tFlag+1]
	t.Marker = tail[tMarker+1]
	if len(tail) >= tColor && tail[tExtra] == tailMarker {
		t.HasExtra = true
		copy(t.Extra[:], tail[tExtra+1:tExtra+4])
	}
	if len(tail) >= wideTail {
		t.Color = tail[tColor]
		t.BTMode = tail[tBTMode]
	}
	return t, nil
}

// Put writes t into a device record's tail slot. It rewrites the first 24
// bytes (18 on a narrow tail without room for them) and keeps the rest.
func (t DeviceTail) Put(tail []byte) error {
	if len(tail) < tExtra {
		return fmt.Errorf("record: device tail is %d bytes", len(tail))
	}
	clear(tail[:min(len(tail), tKnownLen)])
	if t.IP.IsValid() {
		if !t.IP.Is4() {
			return fmt.Errorf("record: device IP %v is not IPv4", t.IP)
		}
		ip := t.IP.As4()
		tail[tIP], tail[tIP+1] = tailMarker, ipTag
		copy(tail[tIP+2:], ip[:])
	}
	if t.HasPollTime {
		tail[tPoll] = tailMarker
		binary.BigEndian.PutUint16(tail[tPoll+1:], t.PollTime)
	}
	tail[tModes] = tailMarker
	tail[tModes+1] = t.InputMode
	tail[tModes+2] = t.PowerMode
	tail[tModes+3] = t.PowerStyle
	tail[tModes+4] = t.ShareMode
	tail[tFlag] = tailMarker
	tail[tFlag+1] = t.Flag
	tail[tMarker] = tailMarker
	tail[tMarker+1] = t.Marker
	if t.HasExtra && len(tail) >= tColor {
		tail[tExtra] = tailMarker
		copy(tail[tExtra+1:], t.Extra[:])
	}
	if len(tail) >= wideTail {
		tail[tColor] = t.Color
		tail[tBTMode] = t.BTMode
	}
	return nil
}

// DeviceTail decodes the entity's tail as a device tail.
func (e *Entity) DeviceTail() (DeviceTail, error) { return ParseDeviceTail(e.Tail) }

// SetDeviceTail writes t into the entity's tail, keeping the bytes beyond
// the decoded fields.
func (e *Entity) SetDeviceTail(t DeviceTail) error {
	if e.Tail == nil {
		s, err := e.variant.Schema()
		if err != nil {
			return err
		}
		e.Tail = make([]byte, s.DeviceSlotLen)
	}
	return t.Put(e.Tail)
}

// Emitters is the X2 IR emitter routing of a device: which outputs its IR
// commands are sent from.
type Emitters uint32

// Emitter bits, from the low byte of the routing (tail byte 21). Bits 3-7
// are remotes 1-5.
const (
	EmitHub     Emitters = 1 << 0 // the hub's built-in emitter
	EmitPort1   Emitters = 1 << 1 // the cable in IR port 1
	EmitPort2   Emitters = 1 << 2
	EmitRemote1 Emitters = 1 << 3
)

// Emitters returns the X2 IR routing. ok is false when the record has no
// routing block or is not an X2 record.
func (e *Entity) Emitters() (r Emitters, ok bool) {
	if e.variant != wire.X2 {
		return 0, false
	}
	t, err := e.DeviceTail()
	if err != nil || !t.HasExtra {
		return 0, false
	}
	return Emitters(t.Extra[0])<<16 | Emitters(t.Extra[1])<<8 | Emitters(t.Extra[2]), true
}
