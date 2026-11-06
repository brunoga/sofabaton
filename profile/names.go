package profile

import (
	"strings"
	"unicode/utf16"
)

// MaxCommands is the most commands the app gives a device made from a
// network service (a Hue bridge's, on the X2: "the limit for the
// Sofabaton remote"); the integrations here keep to it.
const MaxCommands = 169

// CommandNameLen is the most UTF-16 units a command's name holds on the
// hub; the app cuts longer names there.
const CommandNameLen = 30

// FitName is the command name of what was named and the rest of the
// name ("Cinema", " 50% Brightness"), what's name cut so that the whole
// fits in what the hub holds: cutting the rest instead could leave two
// commands alike.
func FitName(what, rest string) string {
	u := utf16.Encode([]rune(what))
	room := CommandNameLen - len(utf16.Encode([]rune(rest)))
	if len(u) > room {
		what = strings.TrimSpace(string(utf16.Decode(u[:max(room, 0)])))
	}
	return what + rest
}
