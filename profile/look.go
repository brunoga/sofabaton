package profile

import "fmt"

// maxColor is the last color of the X2's palette (1-12).
const maxColor = 12

// validateLook checks a profile's color: 0 (leave the hub's) or one of
// the palette's.
func validateLook(color uint8) error {
	if color > maxColor {
		return fmt.Errorf("profile: color %d is not in the palette (1-%d)", color, maxColor)
	}
	return nil
}

// lookChange describes a change of icon and color, for plans.
func lookChange(oldIcon, icon, oldColor, color byte) string {
	var s string
	if oldIcon != icon {
		s += fmt.Sprintf(", icon %d -> %d", oldIcon, icon)
	}
	if oldColor != color {
		s += fmt.Sprintf(", color %d -> %d", oldColor, color)
	}
	return s
}
