package profile

import (
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/record"
)

// Buttons binds keys of the remote, by name (record.Button's names, such
// as OK, Vol+ or Power, regardless of case), to commands of the profile
// for when the device's page is on the remote's screen.
//
//	buttons:
//	  OK: OK
//	  Back: {short: Back, long: Home}
type Buttons map[string]Key

// Key is what a key runs: a command on a short press and, optionally,
// another on a long one.
type Key struct {
	Short string `yaml:"short"`
	Long  string `yaml:"long,omitempty"`
}

// MarshalYAML writes the keys in the order of their button ids, the way
// the hub lists them.
func (bs Buttons) MarshalYAML() (any, error) {
	names := make([]string, 0, len(bs))
	for n := range bs {
		names = append(names, n)
	}
	id := func(n string) int {
		if b, err := record.ParseButton(n); err == nil {
			return int(b)
		}
		return 0x100
	}
	slices.SortFunc(names, func(a, b string) int {
		if d := id(a) - id(b); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, n := range names {
		k := bs[n]
		var v yaml.Node
		if err := v.Encode(k); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: n}, &v)
	}
	return node, nil
}

// MarshalYAML writes a key with no long press as its command's name.
func (k Key) MarshalYAML() (any, error) {
	if k.Long == "" {
		return k.Short, nil
	}
	return &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "short"}, {Kind: yaml.ScalarNode, Value: k.Short},
		{Kind: yaml.ScalarNode, Value: "long"}, {Kind: yaml.ScalarNode, Value: k.Long},
	}}, nil
}

// UnmarshalYAML reads a command's name or a {short, long} mapping.
func (k *Key) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*k = Key{Short: n.Value}
		return nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, v := n.Content[i], n.Content[i+1]
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: %s is not a command name", v.Line, key.Value)
			}
			switch key.Value {
			case "short":
				k.Short = v.Value
			case "long":
				k.Long = v.Value
			default:
				return fmt.Errorf("line %d: field %s not found in a key (short, long)", key.Line, key.Value)
			}
		}
		return nil
	}
	return fmt.Errorf("line %d: a key is a command name or {short, long}", n.Line)
}

// find returns the index of the profile's command named name: by exact
// name, else regardless of case; -1 if none.
func (d *Device) find(name string) int {
	for i, c := range d.Commands {
		if c.Name == name {
			return i
		}
	}
	for i, c := range d.Commands {
		if strings.EqualFold(c.Name, name) {
			return i
		}
	}
	return -1
}

// validateButtons checks that every key names a button once and commands
// of the profile.
func (d *Device) validateButtons() []error {
	var errs []error
	seen := map[record.Button]string{}
	for name, k := range d.Buttons {
		b, err := record.ParseButton(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("profile: buttons: %w", err))
			continue
		}
		if other, ok := seen[b]; ok {
			errs = append(errs, fmt.Errorf("profile: buttons: %q and %q are the same key", other, name))
		}
		seen[b] = name
		if k.Short == "" {
			errs = append(errs, fmt.Errorf("profile: buttons: %s has no command", name))
		}
		for _, c := range []string{k.Short, k.Long} {
			if c != "" && d.find(c) < 0 {
				errs = append(errs, fmt.Errorf("profile: buttons: %s runs %q, which is not a command of the profile", name, c))
			}
		}
	}
	return errs
}
