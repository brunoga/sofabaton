package profile

import "go.yaml.in/yaml/v3"

// Category is a device's kind in the app (a TV, a receiver…): the type
// byte of its record, written in profiles by name (or number). 0 leaves
// the hub's as it is (a TV, for a new device). Only the kinds the X2 app
// was seen to write have names; what the type changes is not known yet
// (PLAN R5).
type Category uint8

var categoryNames = []string{2: "TV", 4: "Blu-ray", 6: "Projector", 7: "Receiver"}

func (c Category) String() string            { return lookName(categoryNames, uint8(c)) }
func (c Category) IsZero() bool              { return c == 0 }
func (c Category) MarshalYAML() (any, error) { return lookYAML(categoryNames, uint8(c)), nil }

func (c *Category) UnmarshalYAML(n *yaml.Node) error {
	v, err := parseLook("category", categoryNames, n)
	*c = Category(v)
	return err
}
