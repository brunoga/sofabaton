// Package roku finds Roku players and TVs on the network, lists their
// channels (apps), and makes their devices as the Sofabaton app does: the
// hub POSTs Roku's External Control Protocol (ECP) paths to them
// (profile.Roku).
package roku

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/brunoga/sofabaton/internal/ssdp"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// Port is ECP's.
const Port = 8060

// Player is a Roku found on the network.
type Player struct {
	Addr netip.Addr
	// Name is the one its owner gave it, else its model's; empty if it
	// did not say.
	Name string
	// Serial is its serial number, if it said.
	Serial string
}

// Discover finds the Rokus on the local network until ctx is done, by
// SSDP (roku:ecp), and asks each its name.
func Discover(ctx context.Context, hc *http.Client) ([]Player, error) {
	rs, err := ssdp.Search(ctx, "roku:ecp")
	var out []Player
	for _, r := range rs {
		if !r.Addr.Is4() || slices.ContainsFunc(out, func(p Player) bool { return p.Addr == r.Addr }) {
			continue
		}
		p := Player{Addr: r.Addr}
		if u, perr := url.Parse(r.Location); perr == nil && u.Hostname() != "" {
			if a, perr := netip.ParseAddr(u.Hostname()); perr == nil && a.Is4() {
				p.Addr = a
			}
		}
		// Discovery's ctx is done by now: the names get their own.
		if info, ierr := Info(context.WithoutCancel(ctx), p.Addr, hc); ierr == nil {
			p.Name, p.Serial = info.Name(), info.Serial
		}
		out = append(out, p)
	}
	return out, err
}

// DeviceInfo is the part of ECP's query/device-info Info reads.
type DeviceInfo struct {
	Serial       string `xml:"serial-number"`
	UserName     string `xml:"user-device-name"`
	FriendlyName string `xml:"friendly-device-name"`
	ModelName    string `xml:"model-name"`
}

// Name is the name its owner gave it, else its model's.
func (d DeviceInfo) Name() string {
	for _, n := range []string{d.UserName, d.FriendlyName, d.ModelName} {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
	}
	return ""
}

// Info asks the Roku at addr about itself; hc nil is
// http.DefaultClient.
func Info(ctx context.Context, addr netip.Addr, hc *http.Client) (DeviceInfo, error) {
	var d DeviceInfo
	err := get(ctx, hc, addr, "query/device-info", &d)
	return d, err
}

// App is a channel of a Roku.
type App struct {
	ID   string `xml:"id,attr"`
	Name string `xml:",chardata"`
}

// Apps lists the Roku's channels (query/apps), as the app does to make
// their launch commands.
func Apps(ctx context.Context, addr netip.Addr, hc *http.Client) ([]App, error) {
	var v struct {
		Apps []App `xml:"app"`
	}
	if err := get(ctx, hc, addr, "query/apps", &v); err != nil {
		return nil, err
	}
	var out []App
	for _, a := range v.Apps {
		if a.ID = strings.TrimSpace(a.ID); a.ID != "" && !strings.ContainsAny(a.ID, " /") {
			a.Name = strings.TrimSpace(a.Name)
			out = append(out, a)
		}
	}
	return out, nil
}

func get(ctx context.Context, hc *http.Client, addr netip.Addr, path string, v any) error {
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s/%s", netip.AddrPortFrom(addr, Port), path), nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("roku: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("roku: %s: %s", path, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("roku: %w", err)
	}
	if err := xml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("roku: %s: %w", path, err)
	}
	return nil
}

// key is a command the app makes for every Roku, and the remote's key it
// binds it to (0 for none).
type key struct {
	name, path string
	button     record.Button
}

// keys are the app's, in its order, with readable names.
var keys = []key{
	{"Power Off", "keypress/PowerOff", 0},
	{"Power On", "keypress/PowerOn", record.ButtonPower},
	{"Input Tuner", "keypress/InputTuner", 0},
	{"Input HDMI 1", "keypress/InputHDMI1", 0},
	{"Input HDMI 2", "keypress/InputHDMI2", 0},
	{"Input HDMI 3", "keypress/InputHDMI3", 0},
	{"Input HDMI 4", "keypress/InputHDMI4", 0},
	{"Input AV 1", "keypress/InputAV1", 0},
	{"Back", "keypress/Back", record.ButtonBack},
	{"Home", "keypress/Home", record.ButtonHome},
	{"Left", "keypress/Left", record.ButtonLeft},
	{"Up", "keypress/Up", record.ButtonUp},
	{"Right", "keypress/Right", record.ButtonRight},
	{"Down", "keypress/Down", record.ButtonDown},
	{"OK", "keypress/Select", record.ButtonOK},
	{"Instant Replay", "keypress/InstantReplay", 0},
	{"Info", "keypress/Info", record.ButtonMenu},
	{"Mute", "keypress/VolumeMute", record.ButtonMute},
	{"Volume Down", "keypress/VolumeDown", record.ButtonVolDown},
	{"Volume Up", "keypress/VolumeUp", record.ButtonVolUp},
	{"Rewind", "keypress/Rev", record.ButtonRewind},
	{"Play/Pause", "keypress/Play", record.ButtonPause},
	{"Forward", "keypress/Fwd", record.ButtonForward},
}

// Device is the profile of a Roku at addr named name: the app's keys, a
// launch command for each of apps (up to profile.MaxCommands in all), the
// remote's keys bound as the app binds them (and Info to Menu), and power
// on and off. Names the hub cannot hold are cut; one taken already gets a
// number.
func Device(name string, addr netip.Addr, apps []App) *profile.Device {
	d := &profile.Device{Name: name, Brand: "Roku", Roku: &profile.Roku{Address: addr.String()}, Buttons: profile.Buttons{}}
	seen := map[string]bool{}
	add := func(base, path string) string {
		n := profile.FitName(base, "")
		for i := 2; seen[strings.ToLower(n)]; i++ {
			n = profile.FitName(base, fmt.Sprint(" ", i))
		}
		seen[strings.ToLower(n)] = true
		d.Commands = append(d.Commands, profile.Command{Name: n, Roku: path})
		return n
	}
	for _, k := range keys {
		n := add(k.name, k.path)
		if k.button != 0 {
			d.Buttons[k.button.String()] = profile.Key{Short: n}
		}
	}
	for _, a := range apps {
		if len(d.Commands) == profile.MaxCommands {
			break
		}
		if a.Name == "" {
			a.Name = "App " + a.ID
		}
		add(a.Name, "launch/"+a.ID)
	}
	d.Power = &profile.Power{On: "Power On", Off: "Power Off"}
	return d
}
