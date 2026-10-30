package record

import "fmt"

// Class is a device's transport class, the record's code-type byte. Command
// records carry the same byte per command, so a device of one class can
// hold commands of another (an RF device with learned IR commands).
type Class uint8

const (
	ClassBluetooth Class = 0x03
	ClassRoku      Class = 0x0A
	ClassIR        Class = 0x0D
	ClassHue       Class = 0x1A
	ClassSonos     Class = 0x1B
	ClassIP        Class = 0x1C // generic HTTP ("virtual IP")
	ClassRF433     Class = 0x1D // not yet seen in a capture
	ClassRF315     Class = 0x1E // not yet seen in a capture
	ClassMQTT      Class = 0x20 // X2 only
)

var classNames = map[Class]string{
	ClassBluetooth: "bluetooth",
	ClassRoku:      "wifi_roku",
	ClassIR:        "ir",
	ClassHue:       "wifi_hue",
	ClassSonos:     "wifi_sonos",
	ClassIP:        "wifi_ip",
	ClassRF433:     "rf_433mhz",
	ClassRF315:     "rf_315mhz",
	ClassMQTT:      "wifi_mqtt",
}

func (c Class) String() string {
	if n, ok := classNames[c]; ok {
		return n
	}
	return fmt.Sprintf("Class(0x%02X)", uint8(c))
}

// Known reports whether the class is one this package has a name for.
func (c Class) Known() bool {
	_, ok := classNames[c]
	return ok
}

// IsWiFi reports whether the class is one of the network classes.
func (c Class) IsWiFi() bool {
	switch c {
	case ClassRoku, ClassHue, ClassSonos, ClassIP, ClassMQTT:
		return true
	}
	return false
}

// Entity ids share one byte: devices below the activities.
const (
	MinDeviceID   = 0x01
	MaxDeviceID   = 0x63
	MinActivityID = 0x65
	MaxActivityID = 0xFF
	// NewID is the id a create write carries; the hub assigns the real one.
	NewID = 0xFF
)

// IsActivityID reports whether id names an activity rather than a device.
func IsActivityID(id byte) bool { return id >= MinActivityID }
