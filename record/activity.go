package record

import (
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

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

// ActivityColor is an activity's color (palette 1-12), 0 when its tail
// is too short to carry one (X1).
func (e *Entity) ActivityColor() byte {
	if len(e.Tail) > activityColorAt {
		return e.Tail[activityColorAt]
	}
	return 0
}

// SetActivityColor sets an activity's color, in a copy of its tail.
func (e *Entity) SetActivityColor(c byte) error {
	if len(e.Tail) <= activityColorAt {
		return fmt.Errorf("record: the %v's activity records have no color", e.variant)
	}
	e.Tail = append([]byte(nil), e.Tail...)
	e.Tail[activityColorAt] = c
	return nil
}
