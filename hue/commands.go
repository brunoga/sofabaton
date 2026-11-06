package hue

import (
	"fmt"
	"strings"

	"github.com/brunoga/sofabaton/profile"
)

// MaxCommands is the most commands the app gives a Hue bridge on the X2
// ("the limit for the Sofabaton remote"); for more, it adds the bridge
// again.
const MaxCommands = profile.MaxCommands

// Brightness levels the app offers, in percent.
var Brightness = []int{10, 50, 80}

// Options say which commands Commands makes for each target.
type Options struct {
	// OnOff makes an on and an off command.
	OnOff bool
	// Brightness makes a command for each level, in percent (1-100).
	Brightness []int
}

// Commands are the commands of the targets, as the app makes them: on
// and off for each room, then each room's brightness levels, then the
// same for each light, named "Cinema on", "Cinema 50% Brightness". Where
// the hub cannot hold a name, the target's name is cut, not the rest:
// the app cuts the whole name ("Poster The Godfather 10% Brigh"), which
// can leave on and off alike. A name taken already gets a number.
func Commands(targets []Target, o Options) []profile.Command {
	var out []profile.Command
	seen := map[string]bool{}
	add := func(target, what string, t Target, body string) {
		name := named(target, what, "")
		for i := 2; seen[strings.ToLower(name)]; i++ {
			name = named(target, what, fmt.Sprint(" ", i))
		}
		seen[strings.ToLower(name)] = true
		out = append(out, profile.Command{Name: name, Hue: &profile.HueCommand{Path: t.path(), Body: body}})
	}
	for _, group := range []bool{true, false} {
		if o.OnOff {
			for _, t := range targets {
				if t.Group == group {
					add(t.Name, " on", t, "{\n\"on\": true\n}")
					add(t.Name, " off", t, "{\n\"on\": false\n}")
				}
			}
		}
		for _, t := range targets {
			if t.Group != group {
				continue
			}
			for _, pct := range o.Brightness {
				pct = max(1, min(100, pct))
				add(t.Name, fmt.Sprintf(" %d%% Brightness", pct), t, fmt.Sprintf("{\n\"bri\": %d\n}", pct*254/100))
			}
		}
	}
	return out
}

// named is the command name target+what+n, the target's name cut so that
// it fits in what the hub holds.
func named(target, what, n string) string { return profile.FitName(target, what+n) }

// Device is the profile of a bridge device of commands, named as the app
// names it: "Hue Bridge (192.168.1.183)".
func Device(c *Client, commands []profile.Command) *profile.Device {
	name := fmt.Sprintf("Hue Bridge (%s)", c.Addr)
	return &profile.Device{
		Name: name, Brand: name,
		Hue:      &profile.Hue{Bridge: c.Addr.String(), User: c.User},
		Commands: commands,
	}
}
