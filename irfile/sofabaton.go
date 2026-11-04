package irfile

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// sofabatonSet is a code set of Sofabaton's catalog
// (irsource.SofabatonDB.Fetch).
type sofabatonSet struct {
	Keys []struct {
		FID     int    `json:"fid"`
		Name    string `json:"keyname"`
		Format  string `json:"format"`
		Carrier string `json:"fre"`
		IRCode  string `json:"irCode"`
		Pulse   string `json:"pulse"`
	} `json:"keys"`
	// Power is how the device switches on and off: type 0 not at all, 1
	// a toggle (on and off the same key), 2 discrete keys; each a list
	// of keys by function id.
	Power struct {
		Type    int `json:"type"`
		On, Off []struct {
			FID int `json:"fid"`
		}
	} `json:"power"`
}

func readSofabatonSet(r io.Reader) (*sofabatonSet, error) {
	var set sofabatonSet
	if err := json.NewDecoder(r).Decode(&set); err != nil {
		return nil, fmt.Errorf("irfile: not a Sofabaton code set: %w", err)
	}
	if set.Keys == nil {
		return nil, fmt.Errorf("irfile: not a Sofabaton code set (no keys)")
	}
	return &set, nil
}

// ReadSofabaton reads a code set of Sofabaton's catalog
// (irsource.SofabatonDB.Fetch): its keys, most an X2 descriptor in hex,
// some (learned codes) timings. Descriptors in a protocol profiles
// render become its codes, others stay descriptors, as the hub takes
// them; timings become raw IR.
func ReadSofabaton(r io.Reader) ([]Signal, error) {
	set, err := readSofabatonSet(r)
	if err != nil {
		return nil, err
	}
	var out []Signal
	for _, k := range set.Keys {
		name := strings.TrimSpace(k.Name)
		out = append(out, sofabatonSignal(Signal{Name: commandName(name), Source: name}, k.IRCode, k.Pulse, k.Carrier, k.Format))
	}
	return out, nil
}

func sofabatonSignal(s Signal, irCode, pulse, carrier, format string) Signal {
	b, err := hex.DecodeString(irCode)
	desc := string(b)
	switch {
	case err == nil && strings.HasPrefix(desc, "P:"):
		if _, err := record.Descriptor(desc); err != nil {
			s.Skip = err.Error()
		} else if ir, code, ok := profile.ParseCode(desc); ok {
			s.IR, s.Code = ir, code
		} else {
			s.Descriptor = desc
		}
		return s
	case strings.TrimSpace(pulse) != "":
		// Timings in µs, mark first, comma-separated, at the carrier.
		hz, err := strconv.Atoi(strings.TrimSpace(carrier))
		if err != nil {
			s.Skip = fmt.Sprintf("timings at a carrier of %q", carrier)
			return s
		}
		var t []uint32
		for _, w := range strings.Split(pulse, ",") {
			v, err := strconv.ParseUint(strings.TrimSpace(w), 10, 32)
			if err != nil {
				s.Skip = fmt.Sprintf("a timing %q is not a number", w)
				return s
			}
			t = append(t, uint32(v))
		}
		return s.raw(record.RawIR{Carrier: hz, Timings: t})
	}
	s.Skip = fmt.Sprintf("a code of format %s, neither a descriptor nor timings: not supported yet", format)
	return s
}

// SofabatonPower is the commands of a Sofabaton code set that switch the
// device on and off, by the names ReadSofabaton gives them: the set's
// own power keys, a toggle for both when it has one, or a toggle for the
// side it lacks a key for. Both are "" when the set says none.
func SofabatonPower(r io.Reader) (on, off string, err error) {
	set, err := readSofabatonSet(r)
	if err != nil {
		return "", "", err
	}
	name := map[int]string{}
	for _, k := range set.Keys {
		if _, ok := name[k.FID]; !ok && (k.IRCode != "" || k.Pulse != "") {
			name[k.FID] = commandName(strings.TrimSpace(k.Name))
		}
	}
	first := func(keys []struct {
		FID int `json:"fid"`
	}) string {
		for _, k := range keys {
			if n := name[k.FID]; n != "" {
				return n
			}
		}
		return ""
	}
	if set.Power.Type == 0 {
		return "", "", nil
	}
	on, off = first(set.Power.On), first(set.Power.Off)
	// The catalog's toggle is function 1.
	toggle := name[1]
	if on == "" {
		on = toggle
	}
	if off == "" {
		off = toggle
	}
	if on == "" || off == "" {
		return "", "", nil
	}
	return on, off, nil
}
