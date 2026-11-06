package profile

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// Hue is what makes a device a Philips Hue bridge (record.ClassHue): its
// address on the network and the user (the key) its API gave, for which
// the hub sends the commands' requests (HueCommand).
//
//	device: Living room lights
//	hue: {bridge: 192.168.1.183, user: KEY}
//	commands:
//	  - name: Cinema on
//	    hue: {path: groups/81/action, body: '{"on": true}'}
type Hue struct {
	Bridge string `yaml:"bridge"`
	User   string `yaml:"user"`
}

// HueCommand is a command of a Hue bridge: a PUT of body to the bridge's
// api/USER/path, such as lights/27/state or groups/3/action.
type HueCommand struct {
	Path string `yaml:"path"`
	Body string `yaml:"body"`
	// Length is the Content-Length the hub sends, when not the body's (as
	// the app writes it, see record.HueRequest).
	Length int `yaml:"length,omitempty"`
}

func (h *Hue) check() error {
	if ip, err := netip.ParseAddr(h.Bridge); err != nil || !ip.Is4() {
		return fmt.Errorf("profile: hue: bridge %q is not an IPv4 address", h.Bridge)
	}
	if h.User == "" || strings.ContainsAny(h.User, "/ \r\n") {
		return fmt.Errorf("profile: hue: user %q is not a key the bridge gave", h.User)
	}
	return nil
}

// request is the command's request for the device's bridge.
func (c HueCommand) request(h *Hue) (record.HueRequest, error) {
	if h == nil {
		return record.HueRequest{}, errors.New("profile: a hue command on a device with no hue bridge")
	}
	p := strings.TrimPrefix(c.Path, "/")
	if p == "" {
		return record.HueRequest{}, errors.New("profile: hue command without a path")
	}
	return record.HueRequest{Path: "api/" + h.User + "/" + p, Body: c.Body, Length: c.Length}, nil
}

// hueCommandOf is the command of r, for the user's bridge; false if r is
// for another user.
func hueCommandOf(r record.HueRequest, user string) (*HueCommand, bool) {
	p, ok := strings.CutPrefix(r.Path, "api/"+user+"/")
	if !ok || p == "" {
		return nil, false
	}
	return &HueCommand{Path: p, Body: r.Body, Length: r.Length}, true
}

// hueFromHub is the Hue block of a bridge device as the hub holds it:
// its address from the record, its user from the commands' paths.
func hueFromHub(dev *record.Entity, st *State) *Hue {
	h := &Hue{}
	if t, err := dev.DeviceTail(); err == nil && t.IP.IsValid() {
		h.Bridge = t.IP.String()
	}
	for _, c := range st.Commands {
		if r, err := record.ParseHueRequest(st.Data[c.ID]); err == nil {
			if rest, ok := strings.CutPrefix(r.Path, "api/"); ok {
				if user, _, ok := strings.Cut(rest, "/"); ok {
					h.User = user
					break
				}
			}
		}
	}
	return h
}

// hueType is the app's category for Wi-Fi devices, a Hue bridge's.
const hueType = 16

// createHue makes a Hue bridge device as the app did on our X2
// (2026-11-06): the record, with the bridge's address and, as the app
// writes it, the address's last byte as its channel; the commands; the
// keys; idle behavior 4; an empty inputs page; and the update that
// commits it. The app sends no sync.
func createHue(ctx context.Context, c *hub.Client, d *Device, done func(string)) (byte, error) {
	if c.Variant() != wire.X2 {
		return 0, errors.New("profile: Hue bridges are made on the X2 only")
	}
	report := func(s string) {
		if done != nil {
			done(s)
		}
	}
	bridge, _ := netip.ParseAddr(d.Hue.Bridge) // validated
	e, err := record.NewEntity(c.Variant())
	if err != nil {
		return 0, err
	}
	e.Name, e.Brand, e.Class, e.Type, e.Icon = d.Name, d.Brand, record.ClassHue, hueType, defaultIcon
	e.Channel = bridge.As4()[3]
	if d.Icon != 0 {
		e.Icon = byte(d.Icon)
	}
	if d.Category != 0 {
		e.Type = byte(d.Category)
	}
	tail := record.NewHueDeviceTail(bridge)
	if d.Color != 0 {
		tail.Color = byte(d.Color)
	}
	if err := e.SetDeviceTail(tail); err != nil {
		return 0, err
	}
	id, err := c.CreateDevice(ctx, e)
	if err != nil {
		return 0, err
	}
	report(fmt.Sprintf("created device %d %q", id, d.Name))

	committed := *e
	committed.ID = id
	committed.Tail = append([]byte(nil), e.Tail...)
	tail.Marker = 1
	if err := committed.SetDeviceTail(tail); err != nil {
		return id, err
	}
	changes, _, err := Plan(d, &State{Variant: c.Variant(), Device: id, Data: map[byte][]byte{}, Entity: &committed}, false)
	if err != nil {
		return id, err
	}
	var commands, keys []Change
	for _, ch := range changes {
		switch ch.Kind {
		case Bind:
			keys = append(keys, ch)
		case Inputs, Record, Sync:
			// Written below, as the app does.
		default:
			commands = append(commands, ch)
		}
	}
	apply := func(chs []Change) error {
		return Apply(ctx, c, chs, func(ch Change) { report(ch.String()) })
	}
	if err := apply(commands); err != nil {
		return id, err
	}
	if err := apply(keys); err != nil {
		return id, err
	}
	if err := c.SetIdleBehavior(ctx, id, 4); err != nil {
		return id, err
	}
	in, err := record.NewInputs(c.Variant(), id, 0)
	if err != nil {
		return id, err
	}
	if err := c.WriteInputs(ctx, in); err != nil {
		return id, err
	}
	if err := c.UpdateDevice(ctx, &committed); err != nil {
		return id, err
	}
	report(fmt.Sprintf("committed device %d", id))
	return id, nil
}
