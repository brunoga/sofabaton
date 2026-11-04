package profile

import "go.yaml.in/yaml/v3"

// Category is a device's kind in the app (a TV, a receiver…): the type
// byte of its record, written in profiles by name (or number). 0 leaves
// the hub's as it is (a TV, for a new device). What the type changes is
// not known yet (PLAN R5).
//
// IR devices take the device types of the IR database the app uses,
// Kookong's (com.hzy.tvmao.ir.Device in the app: STB 1, TV 2, BOX 3, DVD
// 4, AC 5, PRO 6, PA 7…), as the X2 app wrote 2, 4, 6 and 7 for our TV,
// Blu-ray player, projector and receiver; the app writes 15 for Bluetooth
// devices (its SelectDeviceTypeActivity) and 16 for Wi-Fi ones, Kookong's
// 15 (a computer desk) aside.
type Category uint8

var categoryNames = []string{
	1: "Set-top box", 2: "TV", 3: "Media player", 4: "Blu-ray", 5: "Air conditioner",
	6: "Projector", 7: "Receiver", 8: "Fan", 9: "Camera", 10: "Light",
	11: "Air purifier", 12: "Water heater", 13: "Car multimedia", 14: "Foot bath",
	15: "Bluetooth", 16: "Wi-Fi",
}

func (c Category) String() string            { return lookName(categoryNames, uint8(c)) }
func (c Category) IsZero() bool              { return c == 0 }
func (c Category) MarshalYAML() (any, error) { return lookYAML(categoryNames, uint8(c)), nil }

func (c *Category) UnmarshalYAML(n *yaml.Node) error {
	v, err := parseLook("category", categoryNames, n)
	*c = Category(v)
	return err
}
