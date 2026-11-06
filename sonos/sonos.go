// Package sonos finds Sonos speakers on the network and makes their
// devices as the Sofabaton app does: the hub POSTs UPnP control requests
// to them (profile.Sonos). The requests are Sonos's own (its UPnP
// services, on port 1400), each with its own length: the app's are some
// bytes off, and its group volumes adjust the volume rather than set it.
package sonos

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/internal/mdns"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// Port is the speakers' UPnP port.
const Port = 1400

// Speaker is a Sonos speaker found on the network.
type Speaker struct {
	Addr netip.Addr
	// Room is the room it is in, Model its model; empty if it did not
	// say.
	Room, Model string
}

// Name is the speaker's room, else its address.
func (s Speaker) Name() string {
	if s.Room != "" {
		return s.Room
	}
	return "Sonos " + s.Addr.String()
}

// Discover finds the speakers on the local network until ctx is done, by
// their mDNS announcements (_sonos._tcp), and asks each its room.
func Discover(ctx context.Context, hc *http.Client) ([]Speaker, error) {
	svcs, err := mdns.Browse(ctx, "_sonos._tcp")
	var out []Speaker
	for _, s := range svcs {
		if !s.Addr.Is4() || slices.ContainsFunc(out, func(sp Speaker) bool { return sp.Addr == s.Addr }) {
			continue
		}
		sp := Speaker{Addr: s.Addr}
		if d, derr := Describe(context.WithoutCancel(ctx), s.Addr, hc); derr == nil {
			sp.Room, sp.Model = d.Room, d.Model
		}
		out = append(out, sp)
	}
	return out, err
}

// Description is the part of a speaker's device description Describe
// reads.
type Description struct {
	Room  string `xml:"device>roomName"`
	Model string `xml:"device>modelName"`
}

// Describe asks the speaker at addr its room and model; hc nil is
// http.DefaultClient.
func Describe(ctx context.Context, addr netip.Addr, hc *http.Client) (Description, error) {
	var d Description
	b, err := do(ctx, hc, http.MethodGet, addr, "xml/device_description.xml", "", "")
	if err != nil {
		return d, err
	}
	if err := xml.Unmarshal(b, &d); err != nil {
		return d, fmt.Errorf("sonos: description: %w", err)
	}
	d.Room, d.Model = strings.TrimSpace(d.Room), strings.TrimSpace(d.Model)
	return d, nil
}

// Volume asks the speaker its volume, as the app does to start the hub's
// volume steps from it.
func Volume(ctx context.Context, addr netip.Addr, hc *http.Client) (int, error) {
	c := call(renderingControl, "GetVolume", "<InstanceID>0</InstanceID><Channel>Master</Channel>")
	b, err := do(ctx, hc, http.MethodPost, addr, c.Path, c.Action, c.Body)
	if err != nil {
		return 0, err
	}
	var v struct {
		Volume string `xml:"Body>GetVolumeResponse>CurrentVolume"`
	}
	if err := xml.Unmarshal(b, &v); err != nil {
		return 0, fmt.Errorf("sonos: volume: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(v.Volume))
	if err != nil {
		return 0, fmt.Errorf("sonos: volume %q", v.Volume)
	}
	return n, nil
}

func do(ctx context.Context, hc *http.Client, method string, addr netip.Addr, path, action, body string) ([]byte, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://%s/%s", netip.AddrPortFrom(addr, Port), path), bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	if action != "" {
		req.Header.Set("SOAPACTION", `"`+action+`"`)
		req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sonos: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("sonos: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sonos: %s: %s", path, resp.Status)
	}
	return b, nil
}

// A UPnP service of the speakers: its control path and type.
type service struct{ path, urn string }

var (
	avTransport           = service{"MediaRenderer/AVTransport/Control", "urn:schemas-upnp-org:service:AVTransport:1"}
	renderingControl      = service{"MediaRenderer/RenderingControl/Control", "urn:schemas-upnp-org:service:RenderingControl:1"}
	groupRenderingControl = service{"MediaRenderer/GroupRenderingControl/Control", "urn:schemas-upnp-org:service:GroupRenderingControl:1"}
)

// call is the command of a SOAP action of s with args.
func call(s service, action, args string) *profile.SonosCommand {
	body := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		`<u:` + action + ` xmlns:u="` + s.urn + `">` + args + `</u:` + action + `></s:Body></s:Envelope>`
	return &profile.SonosCommand{Path: s.path, Action: s.urn + "#" + action, Body: body}
}

// Volumes are the volumes the app makes commands for.
var Volumes = []int{10, 30, 50, 80, 100}

// Device is the profile of the speaker at addr named name, with the app's
// commands: play, pause, next and previous; mute and unmute, the room's
// and its group's; set volumes, the room's and its group's; and the hub's
// volume steps, from volume (the speaker's now, see Volume; the app's
// default is 30). The remote's keys are bound as the app binds them.
func Device(name string, addr netip.Addr, volume int) *profile.Device {
	d := &profile.Device{Name: name, Brand: "Sonos", Sonos: &profile.Sonos{Address: addr.String()}, Buttons: profile.Buttons{}}
	add := func(n string, c *profile.SonosCommand, b record.Button) {
		d.Commands = append(d.Commands, profile.Command{Name: n, Sonos: c})
		if b != 0 {
			d.Buttons[b.String()] = profile.Key{Short: n}
		}
	}
	const instance = "<InstanceID>0</InstanceID>"
	add("Play", call(avTransport, "Play", instance+"<Speed>1</Speed>"), record.ButtonPause)
	add("Pause", call(avTransport, "Pause", instance), 0)
	add("Next", call(avTransport, "Next", instance), record.ButtonForward)
	add("Previous", call(avTransport, "Previous", instance), record.ButtonRewind)
	add("Mute", call(renderingControl, "SetMute", instance+"<Channel>Master</Channel><DesiredMute>1</DesiredMute>"), record.ButtonMute)
	add("Unmute", call(renderingControl, "SetMute", instance+"<Channel>Master</Channel><DesiredMute>0</DesiredMute>"), 0)
	add("Group Mute", call(groupRenderingControl, "SetGroupMute", instance+"<DesiredMute>1</DesiredMute>"), 0)
	add("Group Unmute", call(groupRenderingControl, "SetGroupMute", instance+"<DesiredMute>0</DesiredMute>"), 0)
	for _, v := range Volumes {
		add(fmt.Sprintf("Volume %d", v), call(renderingControl, "SetVolume", fmt.Sprintf(instance+"<Channel>Master</Channel><DesiredVolume>%d</DesiredVolume>", v)), 0)
	}
	for _, v := range Volumes {
		add(fmt.Sprintf("Group Volume %d", v), call(groupRenderingControl, "SetGroupVolume", fmt.Sprintf(instance+"<DesiredVolume>%d</DesiredVolume>", v)), 0)
	}
	volume = max(0, min(100, volume))
	add("Volume Up", &profile.SonosCommand{Path: "MediaRenderer/VolumeUp/Control", Body: strconv.Itoa(volume)}, record.ButtonVolUp)
	add("Volume Down", &profile.SonosCommand{Path: "MediaRenderer/VolumeDown/Control", Body: strconv.Itoa(volume)}, record.ButtonVolDown)
	return d
}
