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

	"github.com/brunoga/sofabaton/hue"
)

// Hue's link button is good for 30 seconds once pressed; linkWait is how
// long to wait for it, linkPoll how often to ask.
var (
	linkWait   = 60 * time.Second
	linkPoll   = 2 * time.Second
	bridgeWait = 3 * time.Second
	hueHTTP    *http.Client // the tests' bridge
)

// hueCmd writes the profile of a Philips Hue bridge's commands, as the
// app makes them: it finds the bridge, gets a user once its link button
// is pressed (or takes -user), and lists its rooms and lights.
func hueCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("hue", flag.ContinueOnError)
	bridge := fs.String("bridge", "", "the bridge's IP address (default: the one found on the network)")
	user := fs.String("user", "", "a user the bridge gave before (default: get one, pressing the bridge's link button)")
	only := fs.String("only", "", "the rooms and lights to make commands for, comma-separated names (default: all)")
	noLights := fs.Bool("no-lights", false, "only the rooms and zones, not each light")
	noOnOff := fs.Bool("no-onoff", false, "no on and off commands")
	levels := fs.String("brightness", "10,50,80", "brightness levels to make commands for, in percent, comma-separated (empty for none)")
	out := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o := hue.Options{OnOff: !*noOnOff}
	for _, l := range strings.Split(*levels, ",") {
		if l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l), "%")); l == "" {
			continue
		}
		pct, err := strconv.Atoi(l)
		if err != nil || pct < 1 || pct > 100 {
			return fmt.Errorf("-brightness: %q is not a percentage from 1 to 100", l)
		}
		o.Brightness = append(o.Brightness, pct)
	}

	var addr netip.Addr
	if *bridge != "" {
		var err error
		if addr, err = netip.ParseAddr(*bridge); err != nil || !addr.Is4() {
			return fmt.Errorf("-bridge %q is not an IPv4 address", *bridge)
		}
	} else {
		dctx, cancel := context.WithTimeout(ctx, bridgeWait)
		found, err := hue.Discover(dctx)
		cancel()
		switch {
		case len(found) == 0 && err != nil:
			return fmt.Errorf("finding Hue bridges: %w", err)
		case len(found) == 0:
			return errors.New("no Hue bridge found on the network: give its address with -bridge")
		case len(found) > 1:
			var list []string
			for _, b := range found {
				list = append(list, b.Addr.String())
			}
			return fmt.Errorf("found Hue bridges at %s: pick one with -bridge", strings.Join(list, ", "))
		}
		addr = found[0].Addr
	}

	c := &hue.Client{Addr: addr, User: *user, HTTP: hueHTTP}
	if c.User == "" {
		host, _ := os.Hostname()
		app := "sbx#" + host
		fmt.Fprintf(stderr, "press the link button on the Hue bridge at %v (waiting %v)\n", addr, linkWait)
		lctx, cancel := context.WithTimeout(ctx, linkWait)
		defer cancel()
		for {
			l, err := hue.Link(lctx, addr, app, hueHTTP)
			if err == nil {
				c = l
				break
			}
			if !errors.Is(err, hue.ErrLinkButton) {
				return err
			}
			select {
			case <-lctx.Done():
				return errors.New("the bridge's link button was not pressed in time")
			case <-time.After(linkPoll):
			}
		}
		fmt.Fprintln(stderr, "linked: the profile holds the bridge's user, a key to its lights: keep it private")
	}

	targets, err := c.Rooms(ctx)
	if err != nil {
		return err
	}
	if !*noLights {
		lights, err := c.Lights(ctx)
		if err != nil {
			return err
		}
		targets = append(targets, lights...)
	}
	if *only != "" {
		want := map[string]bool{}
		for _, n := range strings.Split(*only, ",") {
			want[strings.ToLower(strings.TrimSpace(n))] = true
		}
		var picked []hue.Target
		for _, t := range targets {
			if want[strings.ToLower(t.Name)] {
				picked = append(picked, t)
				delete(want, strings.ToLower(t.Name))
			}
		}
		for n := range want {
			return fmt.Errorf("-only: the bridge has no room or light named %q", n)
		}
		targets = picked
	}
	cmds := hue.Commands(targets, o)
	if len(cmds) == 0 {
		return errors.New("no commands: pick on and off or a brightness level")
	}
	if len(cmds) > hue.MaxCommands {
		return fmt.Errorf("%d commands, more than the %d a Hue bridge's device takes: narrow them with -only, -no-lights or -brightness", len(cmds), hue.MaxCommands)
	}
	d := hue.Device(c, cmds)
	w := stdout
	if *out != "" {
		of, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = of.Close() }()
		w = of
	}
	return d.Encode(w)
}
