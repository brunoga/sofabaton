package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brunoga/sofabaton/govee"
	"github.com/brunoga/sofabaton/lifx"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/roku"
	"github.com/brunoga/sofabaton/sonos"
)

var (
	// wifiHTTP makes the requests to the devices and clouds; the tests'.
	wifiHTTP *http.Client
	// findWait is how long the network is searched for devices.
	findWait = 3 * time.Second
	// findRokus and findSonos search it; tests replace them.
	findRokus = func(ctx context.Context) ([]roku.Player, error) { return roku.Discover(ctx, wifiHTTP) }
	findSonos = func(ctx context.Context) ([]sonos.Speaker, error) { return sonos.Discover(ctx, wifiHTTP) }
)

// writeProfile writes d to the file named, or to stdout.
func writeProfile(d *profile.Device, file string, stdout io.Writer) error {
	w := stdout
	if file != "" {
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: the file -o names
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		w = f
	}
	return d.Encode(w)
}

// levels parses -brightness: percentages, comma-separated.
func levels(s string) ([]int, error) {
	var out []int
	for _, l := range strings.Split(s, ",") {
		if l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l), "%")); l == "" {
			continue
		}
		pct, err := strconv.Atoi(l)
		if err != nil || pct < 1 || pct > 100 {
			return nil, fmt.Errorf("-brightness: %q is not a percentage from 1 to 100", l)
		}
		out = append(out, pct)
	}
	return out, nil
}

// pickDevice is the device at -address, or the only one found; what names the
// kind, for errors.
func pickDevice[T any](ctx context.Context, flagAddr, what string, find func(context.Context) ([]T, error), addr func(T) netip.Addr) (netip.Addr, *T, error) {
	if flagAddr != "" {
		a, err := netip.ParseAddr(flagAddr)
		if err != nil || !a.Is4() {
			return netip.Addr{}, nil, fmt.Errorf("-address %q is not an IPv4 address", flagAddr)
		}
		return a, nil, nil
	}
	fctx, cancel := context.WithTimeout(ctx, findWait)
	found, err := find(fctx)
	cancel()
	switch {
	case len(found) == 0 && err != nil:
		return netip.Addr{}, nil, fmt.Errorf("finding a %s: %w", what, err)
	case len(found) == 0:
		return netip.Addr{}, nil, fmt.Errorf("no %s found on the network: give its address with -address", what)
	case len(found) > 1:
		var list []string
		for _, f := range found {
			list = append(list, addr(f).String())
		}
		return netip.Addr{}, nil, fmt.Errorf("found a %s at each of %s: pick one with -address", what, strings.Join(list, ", "))
	}
	return addr(found[0]), &found[0], nil
}

// rokuCmd writes a Roku's profile, as the app makes it: its keys, and a
// command for each of its channels.
func rokuCmd(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("roku", flag.ContinueOnError)
	address := fs.String("address", "", "the Roku's IP address (default: the one found on the network)")
	name := fs.String("name", "", "the device's name (default: the Roku's)")
	noApps := fs.Bool("no-apps", false, "no commands launching its channels")
	out := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	addr, found, err := pickDevice(ctx, *address, "Roku", findRokus, func(p roku.Player) netip.Addr { return p.Addr })
	if err != nil {
		return err
	}
	n := *name
	if n == "" && found != nil {
		n = found.Name
	}
	if n == "" {
		if info, err := roku.Info(ctx, addr, wifiHTTP); err == nil {
			n = info.Name()
		}
	}
	if n == "" {
		n = "Roku " + addr.String()
	}
	var apps []roku.App
	if !*noApps {
		if apps, err = roku.Apps(ctx, addr, wifiHTTP); err != nil {
			return err
		}
	}
	return writeProfile(roku.Device(n, addr, apps), *out, stdout)
}

// sonosCmd writes a Sonos speaker's profile, as the app makes it.
func sonosCmd(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sonos", flag.ContinueOnError)
	address := fs.String("address", "", "the speaker's IP address (default: the one found on the network)")
	name := fs.String("name", "", "the device's name (default: the speaker's room)")
	out := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	addr, found, err := pickDevice(ctx, *address, "Sonos speaker", findSonos, func(s sonos.Speaker) netip.Addr { return s.Addr })
	if err != nil {
		return err
	}
	sp := sonos.Speaker{Addr: addr}
	if found != nil {
		sp = *found
	} else if d, err := sonos.Describe(ctx, addr, wifiHTTP); err == nil {
		sp.Room, sp.Model = d.Room, d.Model
	}
	n := *name
	if n == "" {
		n = sp.Name()
	}
	volume, err := sonos.Volume(ctx, addr, wifiHTTP)
	if err != nil {
		volume = 30 // the app's when it cannot ask
	}
	return writeProfile(sonos.Device(n, addr, volume), *out, stdout)
}

// goveeCmd writes the profile of a device of commands for a Govee
// account's lights.
func goveeCmd(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("govee", flag.ContinueOnError)
	key := fs.String("key", os.Getenv("SBX_GOVEE_KEY"), "the account's API key (default $SBX_GOVEE_KEY; Govee Home: Profile, Settings, Apply for API Key)")
	name := fs.String("name", "Govee", "the device's name")
	only := fs.String("only", "", "the lights to make commands for, comma-separated names (default: all)")
	noOnOff := fs.Bool("no-onoff", false, "no on and off commands")
	bright := fs.String("brightness", "10,50,80,100", "brightness levels to make commands for, in percent, comma-separated (empty for none)")
	out := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *key == "" {
		return errors.New("-key (or $SBX_GOVEE_KEY) is required")
	}
	o := govee.Options{OnOff: !*noOnOff}
	var err error
	if o.Brightness, err = levels(*bright); err != nil {
		return err
	}
	lights, err := govee.Lights(ctx, *key, wifiHTTP)
	if err != nil {
		return err
	}
	lights, err = pickNamed(lights, *only, func(l govee.Light) string { return l.Name })
	if err != nil {
		return err
	}
	d := govee.Device(*name, *key, lights, o)
	if len(d.Commands) == 0 {
		return errors.New("no commands: the account has no lights to control, or none were picked")
	}
	if len(d.Commands) > profile.MaxCommands {
		return fmt.Errorf("%d commands, more than the %d a device takes: narrow them with -only or -brightness", len(d.Commands), profile.MaxCommands)
	}
	return writeProfile(d, *out, stdout)
}

// lifxCmd writes the profile of a device of commands for a LIFX
// account's lights: all of them, as the app makes it, and each group
// and light.
func lifxCmd(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("lifx", flag.ContinueOnError)
	token := fs.String("token", os.Getenv("SBX_LIFX_TOKEN"), "the account's personal access token (default $SBX_LIFX_TOKEN; cloud.lifx.com: Settings)")
	name := fs.String("name", "LIFX", "the device's name")
	only := fs.String("only", "", "the groups and lights to make commands for, comma-separated names (default: all)")
	allOnly := fs.Bool("all-only", false, "only commands for all the lights at once, as the app makes them")
	noOnOff := fs.Bool("no-onoff", false, "no on and off commands")
	bright := fs.String("brightness", "10,50,80,100", "brightness levels to make commands for, in percent, comma-separated (empty for none)")
	out := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token == "" {
		return errors.New("-token (or $SBX_LIFX_TOKEN) is required")
	}
	o := lifx.Options{OnOff: !*noOnOff}
	var err error
	if o.Brightness, err = levels(*bright); err != nil {
		return err
	}
	targets := []lifx.Target{lifx.All}
	groups, lights, err := lifx.Targets(ctx, *token, wifiHTTP)
	if err != nil {
		return err
	}
	if !*allOnly {
		picked, err := pickNamed(append(groups, lights...), *only, func(t lifx.Target) string { return t.Name })
		if err != nil {
			return err
		}
		if *only != "" {
			targets = nil
		}
		targets = append(targets, picked...)
	}
	d := lifx.Device(*name, *token, targets, o)
	if len(d.Commands) == 0 {
		return errors.New("no commands: pick on and off or a brightness level")
	}
	if len(d.Commands) > profile.MaxCommands {
		return fmt.Errorf("%d commands, more than the %d a device takes: narrow them with -only, -all-only or -brightness", len(d.Commands), profile.MaxCommands)
	}
	return writeProfile(d, *out, stdout)
}

// pickNamed is the items of names, a comma-separated list (all of them if it
// is empty), each named once.
func pickNamed[T any](items []T, names string, name func(T) string) ([]T, error) {
	if names == "" {
		return items, nil
	}
	want := map[string]bool{}
	for _, n := range strings.Split(names, ",") {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var out []T
	for _, it := range items {
		if k := strings.ToLower(name(it)); want[k] {
			out = append(out, it)
			delete(want, k)
		}
	}
	for n := range want {
		return nil, fmt.Errorf("-only: none is named %q", n)
	}
	return out, nil
}
