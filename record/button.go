package record

import (
	"fmt"
	"strconv"
	"strings"
)

// Button is a key of the remote, or a favorite slot, as keymap rows and
// binding writes name it. Favorite slots use low ids; the hard keys start
// at 0x97.
type Button uint8

// Hard keys. The X2 remote has every one; the X1 and X1S remotes lack the
// ones from ButtonC through ButtonNum1 (see Button.X2Only).
const (
	ButtonC        Button = 0x97
	ButtonB        Button = 0x98
	ButtonA        Button = 0x99
	ButtonExit     Button = 0x9A
	ButtonDVR      Button = 0x9B
	ButtonPlay     Button = 0x9C
	ButtonGuide    Button = 0x9D
	ButtonNumEnter Button = 0x9E
	ButtonNum0     Button = 0x9F
	ButtonNumDash  Button = 0xA0
	ButtonNum9     Button = 0xA1
	ButtonNum8     Button = 0xA2
	ButtonNum7     Button = 0xA3
	ButtonNum6     Button = 0xA4
	ButtonNum5     Button = 0xA5
	ButtonNum4     Button = 0xA6
	ButtonNum3     Button = 0xA7
	ButtonNum2     Button = 0xA8
	ButtonNum1     Button = 0xA9
	// ButtonPower is the power key: on a device page, what the app calls
	// the device's power toggle.
	ButtonPower   Button = 0xAB
	ButtonUp      Button = 0xAE
	ButtonLeft    Button = 0xAF
	ButtonOK      Button = 0xB0
	ButtonRight   Button = 0xB1
	ButtonDown    Button = 0xB2
	ButtonBack    Button = 0xB3
	ButtonHome    Button = 0xB4
	ButtonMenu    Button = 0xB5
	ButtonVolUp   Button = 0xB6
	ButtonChUp    Button = 0xB7
	ButtonMute    Button = 0xB8
	ButtonVolDown Button = 0xB9
	ButtonChDown  Button = 0xBA
	ButtonRewind  Button = 0xBB
	ButtonPause   Button = 0xBC
	ButtonForward Button = 0xBD
	ButtonRed     Button = 0xBE
	ButtonGreen   Button = 0xBF
	ButtonYellow  Button = 0xC0
	ButtonBlue    Button = 0xC1
	// ButtonPowerOn and ButtonPowerOff are not keys of the remote but
	// rows of a device's keymap that the app binds to the device's power
	// on and off commands; what reads them is not known.
	ButtonPowerOn  Button = 0xC2
	ButtonPowerOff Button = 0xC3
)

var buttonNames = map[Button]string{
	ButtonC: "C", ButtonB: "B", ButtonA: "A", ButtonExit: "Exit", ButtonDVR: "DVR",
	ButtonPlay: "Play", ButtonGuide: "Guide", ButtonNumEnter: "Enter", ButtonNum0: "0",
	ButtonNumDash: "-", ButtonNum9: "9", ButtonNum8: "8", ButtonNum7: "7", ButtonNum6: "6",
	ButtonNum5: "5", ButtonNum4: "4", ButtonNum3: "3", ButtonNum2: "2", ButtonNum1: "1",
	ButtonUp: "Up", ButtonLeft: "Left", ButtonOK: "OK", ButtonRight: "Right", ButtonDown: "Down",
	ButtonBack: "Back", ButtonHome: "Home", ButtonMenu: "Menu", ButtonVolUp: "Vol+",
	ButtonChUp: "Ch+", ButtonMute: "Mute", ButtonVolDown: "Vol-", ButtonChDown: "Ch-",
	ButtonRewind: "Rewind", ButtonPause: "Pause", ButtonForward: "Forward", ButtonRed: "Red",
	ButtonGreen: "Green", ButtonYellow: "Yellow", ButtonBlue: "Blue",
	ButtonPower: "Power", ButtonPowerOn: "Power On", ButtonPowerOff: "Power Off",
}

// ParseButton returns the button a name (as String gives it, regardless of
// case) or a number (0x05) stands for.
func ParseButton(s string) (Button, error) {
	for b, n := range buttonNames {
		if strings.EqualFold(n, s) {
			return b, nil
		}
	}
	if v, err := strconv.ParseUint(s, 0, 8); err == nil && strings.HasPrefix(s, "0x") {
		return Button(v), nil
	}
	return 0, fmt.Errorf("record: no button %q", s)
}

// IsHardKey reports whether b is a key of the remote rather than a
// favorite slot.
func (b Button) IsHardKey() bool {
	_, ok := buttonNames[b]
	return ok
}

// X2Only reports whether the key exists on the X2 remote only.
func (b Button) X2Only() bool { return b >= ButtonC && b <= ButtonNum1 }

func (b Button) String() string {
	if n, ok := buttonNames[b]; ok {
		return n
	}
	return fmt.Sprintf("Button(0x%02X)", uint8(b))
}
