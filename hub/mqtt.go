package hub

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"unicode/utf16"

	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// The X2 can publish to an MQTT broker: a device of MQTT commands
// (record.ClassMQTT, the app's "HA remote") publishes one message for each
// command run, to the broker the hub is set up with. The app sets one
// broker for the hub (family 0x73), asks whether it has one (0x74) and
// unbinds it (0x75), from the app's code (2026-11-12).

// MQTTBroker is the broker an X2 publishes to.
type MQTTBroker struct {
	Addr     netip.AddrPort // an IPv4 address; the hub takes no names
	User     string
	Password string
}

// mqttField is the most characters the app's user and password take.
const mqttField = 255

// MQTTPayload is the body of the X2's broker setup (family 0x73), all
// big-endian: the length of what follows, the broker's IPv4 address and
// port, the hub's MAC, then the user and the password, each as its length
// in UTF-16 units and the units (the app's code).
func MQTTPayload(b MQTTBroker, mac [6]byte) ([]byte, error) {
	ip := b.Addr.Addr()
	if !ip.Is4() {
		return nil, fmt.Errorf("hub: MQTT broker %v is not an IPv4 address", b.Addr)
	}
	if b.Addr.Port() == 0 {
		return nil, errors.New("hub: MQTT broker without a port")
	}
	var rest []byte
	a := ip.As4()
	rest = append(rest, a[:]...)
	rest = binary.BigEndian.AppendUint16(rest, b.Addr.Port())
	rest = append(rest, mac[:]...)
	for _, s := range []string{b.User, b.Password} {
		u := utf16.Encode([]rune(s))
		if len(u) == 0 || len(u) > mqttField {
			return nil, fmt.Errorf("hub: MQTT user and password are 1 to %d characters", mqttField)
		}
		rest = append(rest, byte(len(u)))
		for _, c := range u {
			rest = binary.BigEndian.AppendUint16(rest, c)
		}
	}
	out := binary.BigEndian.AppendUint16(nil, uint16(len(rest)))
	return append(out, rest...), nil
}

// SetMQTT sets the broker the hub publishes its MQTT commands to.
func (c *Client) SetMQTT(ctx context.Context, b MQTTBroker) error {
	if err := c.mqttHub(); err != nil {
		return err
	}
	var mac [6]byte
	copy(mac[:], c.info.ID[:])
	p, err := MQTTPayload(b, mac)
	if err != nil {
		return err
	}
	// Our X2 answers in the status's family (2026-11-12).
	ok, err := c.mqttExchange(ctx, wire.Frame{Family: wire.FamHubMQTTSetup, Payload: p}, wire.FamHubMQTTStatusAck)
	if err != nil {
		return fmt.Errorf("hub: setting the MQTT broker: %w", err)
	}
	if !ok {
		return errors.New("hub: the hub did not take the MQTT broker")
	}
	return nil
}

// MQTTConfigured reports whether the hub has a broker.
func (c *Client) MQTTConfigured(ctx context.Context) (bool, error) {
	if err := c.mqttHub(); err != nil {
		return false, err
	}
	ok, err := c.mqttExchange(ctx, wire.Frame{Family: wire.FamHubMQTTStatus}, wire.FamHubMQTTStatusAck)
	if err != nil {
		return false, fmt.Errorf("hub: MQTT status: %w", err)
	}
	return ok, nil
}

// ClearMQTT unbinds the hub's broker.
func (c *Client) ClearMQTT(ctx context.Context) error {
	if err := c.mqttHub(); err != nil {
		return err
	}
	ok, err := c.mqttExchange(ctx, wire.Frame{Family: wire.FamHubMQTTClear}, wire.FamHubMQTTClearAck)
	if err != nil {
		return fmt.Errorf("hub: unbinding the MQTT broker: %w", err)
	}
	if !ok {
		return errors.New("hub: the hub kept its MQTT broker")
	}
	return nil
}

func (c *Client) mqttHub() error {
	if c.variant != wire.X2 {
		return fmt.Errorf("%w: MQTT is the X2's", ErrUnsupported)
	}
	return c.Writable()
}

// mqttExchange sends req and reads the hub's one-byte answer in fam: true
// when not zero.
func (c *Client) mqttExchange(ctx context.Context, req wire.Frame, fam wire.Family) (bool, error) {
	ctx, cancel := c.bound(ctx)
	defer cancel()
	ack := &session.Ack{Family: fam}
	if err := c.s.Exchange(ctx, req, ack); err != nil {
		return false, err
	}
	return len(ack.Frame.Payload) > 0 && ack.Frame.Payload[0] != 0, nil
}
