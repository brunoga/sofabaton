package profile

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Icon is a device's or activity's icon on the remote: the X2's icon id,
// written in profiles by name (or number). 0 leaves the hub's as it is.
// The names are upstream's, for the X2's 45 icons; X1 and X1S hubs number
// their icons differently.
type Icon uint8

// Color is a device's or activity's color on the X2 remote: a palette id
// (1-12), written in profiles by name (or number). 0 leaves the hub's as
// it is.
type Color uint8

var iconNames = []string{
	1: "Monitor", "Picture", "Music", "Film reel", "TV", "Gamepad",
	"Game controller", "Router", "Wifi router", "Projector", "Receiver",
	"Pendant lamp", "Apple TV", "Roku", "Fire TV", "Netflix",
	"Media player", "Curved TV", "Download", "Satellite dish", "Disc",
	"Set-top box", "Cassette deck", "Front projector", "Smart speaker",
	"Air conditioner", "Desk fan", "Ceiling fan", "Table lamp",
	"Power outlet", "Wall switch", "Presentation screen", "Party", "Sleep",
	"DAZN", "DIRECTV", "Disney", "Hulu", "NVIDIA", "Prime Video", "PS", "R",
	"SAM", "Xbox", "YouTube",
}

// colorNames approximate the X2 app's palette (upstream's hex values in
// the comments) by common color names.
var colorNames = []string{
	1:  "grey",       // #7F7F7F, also what the app draws for 0
	2:  "red",        // #F16161
	3:  "navy",       // #2E385E
	4:  "purple",     // #8171B4
	5:  "green",      // #64BB80
	6:  "pink",       // #E39E89
	7:  "blue",       // #00A8E0
	8:  "indigo",     // #717FB4
	9:  "cyan",       // #56CBCB
	10: "beige",      // #BCAD90
	11: "light blue", // #6EACEB
	12: "lavender",   // #97A5DB
}

func (i Icon) String() string             { return lookName(iconNames, uint8(i)) }
func (c Color) String() string            { return lookName(colorNames, uint8(c)) }
func (i Icon) IsZero() bool               { return i == 0 }
func (c Color) IsZero() bool              { return c == 0 }
func (i Icon) MarshalYAML() (any, error)  { return lookYAML(iconNames, uint8(i)), nil }
func (c Color) MarshalYAML() (any, error) { return lookYAML(colorNames, uint8(c)), nil }

func (i *Icon) UnmarshalYAML(n *yaml.Node) error {
	v, err := parseLook("icon", iconNames, n)
	*i = Icon(v)
	return err
}

func (c *Color) UnmarshalYAML(n *yaml.Node) error {
	v, err := parseLook("color", colorNames, n)
	*c = Color(v)
	return err
}

func lookName(names []string, v uint8) string {
	if int(v) < len(names) && names[v] != "" {
		return names[v]
	}
	return strconv.Itoa(int(v))
}

// lookYAML is a known id's name, else the number.
func lookYAML(names []string, v uint8) any {
	if int(v) < len(names) && names[v] != "" {
		return names[v]
	}
	return int(v)
}

// parseLook reads a name (regardless of case, spaces and hyphens) or a
// number of the list.
func parseLook(what string, names []string, n *yaml.Node) (uint8, error) {
	if n.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("profile: line %d: %s: a name or a number", n.Line, what)
	}
	if v, err := strconv.ParseUint(n.Value, 0, 8); err == nil {
		return uint8(v), nil // checked by Validate
	}
	key := lookKey(n.Value)
	for i, name := range names {
		if name != "" && lookKey(name) == key {
			return uint8(i), nil
		}
	}
	return 0, fmt.Errorf("profile: line %d: no %s %q", n.Line, what, n.Value)
}

func lookKey(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(s))
}

// validateLook checks a profile's color. Icons are not: an export
// writes the ids it has no name for as numbers, and applies them back.
func validateLook(color Color) error {
	if int(color) >= len(colorNames) {
		return fmt.Errorf("profile: color %d is not in the palette (1-%d)", color, len(colorNames)-1)
	}
	return nil
}

// lookChange describes a change of icon and color, for plans.
func lookChange(oldIcon, icon Icon, oldColor, color Color) string {
	var s string
	if oldIcon != icon {
		s += fmt.Sprintf(", icon %v -> %v", oldIcon, icon)
	}
	if oldColor != color {
		s += fmt.Sprintf(", color %v -> %v", oldColor, color)
	}
	return s
}
