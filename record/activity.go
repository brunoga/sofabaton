package record

import "github.com/brunoga/sofabaton/wire"

// ActivityIcon is the icon the X2 app gave the activity it created.
const ActivityIcon = 10

// activityColorAt is the tail byte with an activity's color (palette
// 1-12, as for devices); the tail starts with the bytes the app writes.
const activityColorAt = 22

var activityTailStart = []byte{0xFC, 0x00, 0xFC, 0x00}

// NewActivity returns the record of a new activity (family 0x37), as the
// X2 app writes it: ActivityIcon, the given color and name.
func NewActivity(v wire.Variant, name string, color byte) (*Entity, error) {
	e, err := NewEntity(v)
	if err != nil {
		return nil, err
	}
	e.Icon, e.Name = ActivityIcon, name
	copy(e.Tail, activityTailStart)
	if len(e.Tail) > activityColorAt {
		e.Tail[activityColorAt] = color
	}
	return e, nil
}
