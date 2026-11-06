package profile

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/brunoga/sofabaton/record"
)

// Roku makes a device a Roku player or TV (record.ClassRoku): its address,
// to whose port 8060 the hub POSTs the commands' paths of Roku's External
// Control Protocol (Command.Roku).
//
//	device: Living room Roku
//	roku: {address: 192.168.1.50}
//	commands:
//	  - {name: Home, roku: keypress/Home}
//	  - {name: Netflix, roku: launch/12}
type Roku struct {
	Address string `yaml:"address"`
}

// Sonos makes a device a Sonos speaker (record.ClassSonos): its address,
// to whose port 1400 the hub POSTs the commands' UPnP requests
// (SonosCommand).
//
//	device: Kitchen
//	sonos: {address: 192.168.1.60}
//	commands:
//	  - name: Play
//	    sonos:
//	      path: MediaRenderer/AVTransport/Control
//	      action: 'urn:schemas-upnp-org:service:AVTransport:1#Play'
//	      body: '<s:Envelope ...>'
type Sonos struct {
	Address string `yaml:"address"`
}

// SonosCommand is a command of a Sonos speaker: a SOAP request to the
// control path, or, without an action, one of the hub's volume steps
// (path MediaRenderer/VolumeUp/Control or VolumeDown/Control, the body a
// volume): see record.SonosRequest.
type SonosCommand struct {
	Path   string `yaml:"path"`
	Action string `yaml:"action,omitempty"`
	Body   string `yaml:"body"`
	// Length and Space keep the app's commands as it writes them (see
	// record.SonosRequest).
	Length int  `yaml:"length,omitempty"`
	Space  bool `yaml:"space,omitempty"`
}

func checkAddress(kind, addr string) error {
	if ip, err := netip.ParseAddr(addr); err != nil || !ip.Is4() {
		return fmt.Errorf("profile: %s: address %q is not an IPv4 address", kind, addr)
	}
	return nil
}

func (r *Roku) check() error  { return checkAddress("roku", r.Address) }
func (s *Sonos) check() error { return checkAddress("sonos", s.Address) }

func (c SonosCommand) request() record.SonosRequest {
	return record.SonosRequest{Path: strings.TrimPrefix(c.Path, "/"), Action: c.Action, Body: c.Body, Length: c.Length, Space: c.Space}
}

func sonosCommandOf(r record.SonosRequest) *SonosCommand {
	return &SonosCommand{Path: r.Path, Action: r.Action, Body: r.Body, Length: r.Length, Space: r.Space}
}

// address is the IPv4 address a device's record holds, or "".
func address(dev *record.Entity) string {
	if t, err := dev.DeviceTail(); err == nil && t.IP.IsValid() {
		return t.IP.String()
	}
	return ""
}

// addressed is the class and address of a device that is one of the
// hub's network integrations (Hue, Roku, Sonos), whose record holds the
// address its commands go to; false for other devices.
func (d *Device) addressed() (record.Class, string, bool) {
	switch {
	case d.Hue != nil:
		return record.ClassHue, d.Hue.Bridge, true
	case d.Roku != nil:
		return record.ClassRoku, d.Roku.Address, true
	case d.Sonos != nil:
		return record.ClassSonos, d.Sonos.Address, true
	}
	return 0, "", false
}

// kinds is how many of the kinds a device can be (IR aside) it is.
func (d *Device) kinds() int {
	n := 0
	for _, set := range []bool{d.Bluetooth != nil, d.Hue != nil, d.Roku != nil, d.Sonos != nil, d.network()} {
		if set {
			n++
		}
	}
	return n
}

// validateWiFi checks a Roku's or Sonos's block and commands: theirs
// only (or data kept as the hub holds it), and no inputs.
func (d *Device) validateWiFi() []error {
	var errs []error
	if d.kinds() > 1 {
		errs = append(errs, errors.New("profile: a device is one of Bluetooth, Hue, Roku, Sonos or HTTP"))
	}
	if d.Roku != nil {
		if err := d.Roku.check(); err != nil {
			errs = append(errs, err)
		}
	}
	if d.Sonos != nil {
		if err := d.Sonos.check(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, c := range d.Commands {
		if (d.Roku != nil) != (c.Roku != "") && (d.Roku == nil || c.Data == "") {
			errs = append(errs, fmt.Errorf("profile: %q: a Roku's commands are roku paths, and only its", c.Name))
		}
		if (d.Sonos != nil) != (c.Sonos != nil) && (d.Sonos == nil || c.Data == "") {
			errs = append(errs, fmt.Errorf("profile: %q: a Sonos's commands are sonos requests, and only its", c.Name))
		}
	}
	if (d.Roku != nil || d.Sonos != nil) && len(d.Inputs) > 0 {
		errs = append(errs, errors.New("profile: inputs: a Roku or Sonos cannot have inputs yet"))
	}
	return errs
}
