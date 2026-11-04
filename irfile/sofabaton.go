package irfile

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// ReadSofabaton reads a code set of Sofabaton's catalog
// (irsource.SofabatonDB.Fetch): its keys, each an X2 descriptor in hex.
// Those in a protocol profiles render become its codes, others stay
// descriptors, as the hub takes them; keys of no descriptor (learned
// pulses) are left out, saying why.
func ReadSofabaton(r io.Reader) ([]Signal, error) {
	var set struct {
		Keys []struct {
			Name   string `json:"keyname"`
			Format string `json:"format"`
			IRCode string `json:"irCode"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r).Decode(&set); err != nil {
		return nil, fmt.Errorf("irfile: not a Sofabaton code set: %w", err)
	}
	if set.Keys == nil {
		return nil, fmt.Errorf("irfile: not a Sofabaton code set (no keys)")
	}
	var out []Signal
	for _, k := range set.Keys {
		name := strings.TrimSpace(k.Name)
		s := Signal{Name: commandName(name), Source: name}
		b, err := hex.DecodeString(k.IRCode)
		desc := string(b)
		switch {
		case err != nil || !strings.HasPrefix(desc, "P:"):
			s.Skip = fmt.Sprintf("a code of format %s, not a descriptor: not supported yet", k.Format)
		default:
			if _, err := record.Descriptor(desc); err != nil {
				s.Skip = err.Error()
			} else if ir, code, ok := profile.ParseCode(desc); ok {
				s.IR, s.Code = ir, code
			} else {
				s.Descriptor = desc
			}
		}
		out = append(out, s)
	}
	return out, nil
}
