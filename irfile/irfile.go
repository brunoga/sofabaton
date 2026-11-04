// Package irfile reads files of IR codes that other tools and databases
// publish, Flipper Zero's .ir files and irdb's .csv files, into device
// profiles.
//
// A file's signals become commands in the protocols profiles render for
// the X2 (profile.IR), or, for raw timings and protocols the X2 has no
// descriptor for, raw IR the hub sends as it is (package irgen); signals
// in protocols neither covers are left out and listed, each with why. Command names are made the usual
// ones ("Volume Up", "Channel Down", "OK"), so that apps can put them on
// the remote's keys.
package irfile

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// Signal is one signal of a file: a command in a protocol profiles can
// render, raw IR, or one left out (Skip says why).
type Signal struct {
	// Name is the command's name, made the usual one where it is a known
	// key ("Volume Up" for Vol_up); Source is its name in the file.
	Name, Source string
	IR           profile.IR
	Code         uint16
	// Raw, when set, is the signal as timings, instead of IR and Code.
	Raw  *record.RawIR
	Skip string
}

// raw is s carrying r as its signal, or left out if r cannot be sent.
func (s Signal) raw(r record.RawIR) Signal {
	if _, err := r.Encode(); err != nil {
		s.Skip = err.Error()
		return s
	}
	s.Raw = &r
	return s
}

// Skipped is a signal left out of a device, and why.
type Skipped struct {
	Name, Why string
}

// Device makes a device named name of the signals: its IR is the one most
// of them share, and the others carry their own. Signals left out, and
// those whose name repeats an earlier one's, are listed.
func Device(name string, signals []Signal) (*profile.Device, []Skipped) {
	d := &profile.Device{Name: name}
	var skipped []Skipped
	count := map[string]int{}
	var kept []Signal
	seen := map[string]bool{}
	for _, s := range signals {
		switch {
		case s.Skip != "":
			skipped = append(skipped, Skipped{Name: s.Source, Why: s.Skip})
			continue
		case seen[strings.ToLower(s.Name)]:
			// Files list some keys twice (a toggle's two codes, say): the
			// first is the one remotes send first.
			skipped = append(skipped, Skipped{Name: s.Source, Why: "another signal has the same name"})
			continue
		}
		if s.Raw != nil {
			seen[strings.ToLower(s.Name)] = true
			kept = append(kept, s)
			continue
		}
		cmd := profile.Command{Name: s.Name, Code: &profile.Code{Value: s.Code, Decimal: decimal(s.IR.Protocol)}}
		ir := s.IR
		if _, err := cmd.Signal(&profile.Device{IR: &ir}); err != nil {
			skipped = append(skipped, Skipped{Name: s.Source, Why: err.Error()})
			continue
		}
		seen[strings.ToLower(s.Name)] = true
		kept = append(kept, s)
		count[irKey(s.IR)]++
	}
	best := ""
	for k, n := range count {
		if n > count[best] || n == count[best] && k < best {
			best = k
		}
	}
	for _, s := range kept {
		if s.Raw != nil {
			data, _ := s.Raw.Encode() // checked by Signal.raw
			d.Commands = append(d.Commands, profile.Command{Name: s.Name, Data: hex.EncodeToString(data)})
			continue
		}
		cmd := profile.Command{Name: s.Name, Code: &profile.Code{Value: s.Code, Decimal: decimal(s.IR.Protocol)}}
		if irKey(s.IR) == best {
			if d.IR == nil {
				ir := s.IR
				d.IR = &ir
			}
		} else {
			ir := s.IR
			cmd.IR = &ir
		}
		d.Commands = append(d.Commands, cmd)
	}
	return d, skipped
}

// decimal reports whether codes of the protocol are written in decimal,
// as irdb does for all but NEC's.
func decimal(protocol string) bool { return !strings.EqualFold(protocol, "NEC") }

// irKey identifies IR parameters, to count the signals that share them.
func irKey(ir profile.IR) string {
	v := func(p *uint8) string {
		if p == nil {
			return "-"
		}
		return fmt.Sprint(*p)
	}
	return fmt.Sprintf("%s/%04X/%s/%s", strings.ToUpper(ir.Protocol), uint16(ir.System), v(ir.Device), v(ir.Subdevice))
}

// usual are the usual names of commands, by the names files give them,
// lowercased, with runs of spaces, underscores and dashes made one space.
var usual = map[string]string{}

func init() {
	for name, aliases := range map[string][]string{
		"Power":        {"power", "pwr", "power toggle", "on off", "on/off", "standby"},
		"Power On":     {"power on", "poweron", "on", "pwr on", "discrete on"},
		"Power Off":    {"power off", "poweroff", "off", "pwr off", "discrete off"},
		"Volume Up":    {"vol up", "vol+", "vol +", "volume up", "volume +", "volume+", "volumeup", "vol plus", "volume plus"},
		"Volume Down":  {"vol dn", "vol down", "vol-", "vol -", "volume down", "volume -", "volume-", "volumedown", "vol minus", "volume minus"},
		"Channel Up":   {"ch next", "ch up", "ch+", "ch +", "channel up", "channel +", "channel+", "channelup", "chan up", "program up", "prog+", "p+"},
		"Channel Down": {"ch prev", "ch dn", "ch down", "ch-", "ch -", "channel down", "channel -", "channel-", "channeldown", "chan down", "program down", "prog-", "p-"},
		"Mute":         {"mute", "muting"},
		"OK":           {"ok", "enter", "select", "confirm"},
		"Up":           {"up", "arrow up", "cursor up", "nav up", "navigate up"},
		"Down":         {"down", "arrow down", "cursor down", "nav down", "navigate down"},
		"Left":         {"left", "arrow left", "cursor left", "nav left", "navigate left"},
		"Right":        {"right", "arrow right", "cursor right", "nav right", "navigate right"},
		"Menu":         {"menu"},
		"Back":         {"back", "return"},
		"Exit":         {"exit"},
		"Home":         {"home"},
		"Input":        {"input", "source", "src"},
		"Play":         {"play"},
		"Pause":        {"pause"},
		"Play/Pause":   {"play pause", "play/pause", "playpause"},
		"Stop":         {"stop"},
		"Rewind":       {"rew", "rewind", "rwd"},
		"Fast Forward": {"ff", "fast fo", "fast forward", "fastforward", "fwd", "fast fwd"},
		"Next":         {"next", "skip next", "skip+", "next track"},
		"Previous":     {"prev", "previous", "skip prev", "skip-", "previous track"},
		"Info":         {"info", "display"},
		"Guide":        {"guide", "epg"},
		"Red":          {"red"},
		"Green":        {"green"},
		"Yellow":       {"yellow"},
		"Blue":         {"blue"},
	} {
		for _, a := range aliases {
			usual[a] = name
		}
	}
	for i := range 10 {
		n := fmt.Sprint(i)
		for _, a := range []string{n, "num " + n, "number " + n, "digit " + n, "key " + n} {
			usual[a] = n
		}
	}
}

// acronyms keep their capitals when names are tidied.
var acronyms = map[string]bool{
	"av": true, "aux": true, "bd": true, "cd": true, "dvd": true, "dvr": true, "epg": true,
	"hd": true, "hdmi": true, "hdr": true, "ir": true, "led": true, "lcd": true, "pc": true,
	"pip": true, "tv": true, "usb": true, "vcr": true, "vga": true, "3d": true, "4k": true,
}

// commandName is the usual name of a command named source in a file:
// "Volume Up" for Vol_up, "VOLUME +" or KEY_VOLUMEUP, and for names
// that are not a known key, the name tidied (underscores made spaces,
// irdb's capitals made words: "HDMI 1" for HDMI_1, "Sleep" for SLEEP).
func commandName(source string) string {
	words := strings.FieldsFunc(source, func(r rune) bool { return r == '_' || unicode.IsSpace(r) })
	key := strings.TrimPrefix(strings.ToLower(strings.Join(words, " ")), "key ")
	if n, ok := usual[key]; ok {
		return n
	}
	// Dashes as separators ("Vol-Up"), once "vol-" was looked up whole.
	if n, ok := usual[strings.Join(strings.Fields(strings.ReplaceAll(key, "-", " ")), " ")]; ok {
		return n
	}
	if len(words) == 0 {
		return source
	}
	if strings.ToUpper(source) == source {
		// irdb's names are capitals: words, but for acronyms.
		for i, w := range words {
			if !acronyms[strings.ToLower(w)] {
				words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
			}
		}
	}
	return strings.Join(words, " ")
}

// Read reads a file of IR codes in any of the formats, told by its
// name's extension (.ir is Flipper's, .csv irdb's) or else by its
// content: Flipper's header, Pronto codes, or irdb's.
func Read(name string, r io.Reader) ([]Signal, error) {
	br := bufio.NewReader(r)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ir":
		return ReadFlipper(br)
	case ".csv":
		return ReadIRDB(br)
	}
	head, _ := br.Peek(4096)
	switch {
	case strings.HasPrefix(strings.TrimSpace(string(head)), "Filetype:"):
		return ReadFlipper(br)
	case prontoCode.Match(head):
		return ReadPronto(br)
	}
	return ReadIRDB(br)
}
