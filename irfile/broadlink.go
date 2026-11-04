package irfile

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/brunoga/sofabaton/record"
)

// broadlinkTick is a Broadlink timing unit, in ns: 269/8192 ms.
const broadlinkTick = 269e6 / 8192

// Broadlink is the raw IR of a Broadlink code (as Broadlink's app and Home
// Assistant keep them, in base64 or hex): 0x26 for IR, a repeat count,
// the length of the timings, then each timing in ticks of about 32.8 µs,
// one byte, or a zero and two bytes, big-endian. The signal is sent as
// often as the code says, at 38 kHz (Broadlink does not keep the carrier).
func Broadlink(b []byte) (record.RawIR, error) {
	if len(b) < 4 {
		return record.RawIR{}, fmt.Errorf("Broadlink: a code of %d bytes", len(b))
	}
	if b[0] != 0x26 {
		return record.RawIR{}, fmt.Errorf("Broadlink: a code of type 0x%02X, not IR (0x26)", b[0])
	}
	repeat := int(b[1])
	n := int(b[2]) | int(b[3])<<8
	body := b[4:]
	if n > len(body) {
		return record.RawIR{}, fmt.Errorf("Broadlink: %d bytes of timings in %d", n, len(body))
	}
	body = body[:n]
	var once []uint32
	for i := 0; i < len(body); i++ {
		v := int(body[i])
		if v == 0 {
			if i+2 >= len(body) {
				break
			}
			v = int(body[i+1])<<8 | int(body[i+2])
			i += 2
		}
		if v == 0 {
			continue
		}
		once = append(once, uint32(float64(v)*broadlinkTick/1000+0.5))
	}
	if len(once) == 0 {
		return record.RawIR{}, fmt.Errorf("Broadlink: a code of no timings")
	}
	if len(once)%2 == 1 {
		once = append(once, 100000)
	}
	r := record.RawIR{Carrier: 38000}
	for i := 0; i <= min(repeat, 4); i++ {
		r.Timings = append(r.Timings, once...)
	}
	return r, nil
}

// broadlinkText is a Broadlink code written as base64 or hex.
func broadlinkText(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := hex.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// smartIR is a SmartIR device file (Home Assistant's SmartIR integration):
// codes for one controller, in one encoding, by command; a command may
// hold more, by name (a media player's sources), or more levels of them
// (an air conditioner's modes, fan speeds and temperatures).
type smartIR struct {
	Controller string          `json:"supportedController"`
	Encoding   string          `json:"commandsEncoding"`
	Commands   json.RawMessage `json:"commands"`
}

// ReadSmartIR reads a SmartIR device file (.json): its commands, those in
// a nested group named by their path ("cool auto 24"; a media player's
// sources by their own, "HDMI 1"). Codes in
// Broadlink's base64 or hex, or in Pronto, become raw IR; other encodings
// are left out, saying why.
func ReadSmartIR(r io.Reader) ([]Signal, error) {
	var f smartIR
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("irfile: not a SmartIR file: %w", err)
	}
	if len(f.Commands) == 0 {
		return nil, fmt.Errorf("irfile: not a SmartIR file (no commands)")
	}
	var cmds any
	if err := json.Unmarshal(f.Commands, &cmds); err != nil {
		return nil, fmt.Errorf("irfile: SmartIR commands: %w", err)
	}
	enc := strings.ToLower(f.Encoding)
	var out []Signal
	var walk func(path []string, v any)
	walk = func(path []string, v any) {
		switch v := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(append(path[:len(path):len(path)], k), v[k])
			}
		case string:
			// A media player's sources are named by themselves.
			if len(path) == 2 && strings.EqualFold(path[0], "sources") {
				path = path[1:]
			}
			name := strings.Join(path, " ")
			s := Signal{Name: commandName(name), Source: name}
			switch enc {
			case "base64", "hex":
				b, err := broadlinkText(v)
				if err != nil {
					s.Skip = "not a Broadlink code in base64 or hex"
					break
				}
				r, err := Broadlink(b)
				if err != nil {
					s.Skip = err.Error()
					break
				}
				s = s.raw(r)
			case "pronto":
				r, err := Pronto(v)
				if err != nil {
					s.Skip = err.Error()
					break
				}
				s = s.raw(r)
			default:
				s.Skip = fmt.Sprintf("codes in %s's %q encoding: not supported yet", f.Controller, f.Encoding)
			}
			out = append(out, s)
		}
	}
	walk(nil, cmds)
	return out, nil
}
