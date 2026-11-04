package irfile

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/record"
)

// girrCommand is a command of a Girr file: its protocol and parameters,
// its timings, or its Pronto code, any of them.
type girrCommand struct {
	Name   string `xml:"name,attr"`
	Params struct {
		Protocol string `xml:"protocol,attr"`
		Params   []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"parameter"`
	} `xml:"parameters"`
	Raw []struct {
		Frequency int    `xml:"frequency,attr"`
		Intro     string `xml:"intro"`
		Repeat    string `xml:"repeat"`
		Ending    string `xml:"ending"`
	} `xml:"raw"`
	CCF string `xml:"ccf"`
}

// ReadGirr reads a Girr file (.girr, the XML IrScrutinizer exports and
// IR databases publish): its commands, wherever they are. A command's
// protocol and parameters are taken as irdb's are; failing those, its
// timings (intro, repeat once, ending) or its Pronto code are raw IR.
func ReadGirr(r io.Reader) ([]Signal, error) {
	dec := xml.NewDecoder(r)
	var out []Signal
	root := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("irfile: not a Girr file: %w", err)
		}
		st, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch st.Name.Local {
		case "remotes", "remote", "commandSet":
			root = true
		case "command":
			var c girrCommand
			if err := dec.DecodeElement(&c, &st); err != nil {
				return nil, fmt.Errorf("irfile: Girr command: %w", err)
			}
			out = append(out, girrSignal(c))
		}
	}
	if !root && len(out) == 0 {
		return nil, fmt.Errorf("irfile: not a Girr file (no remotes or commands)")
	}
	return out, nil
}

func girrSignal(c girrCommand) Signal {
	s := Signal{Name: commandName(c.Name), Source: c.Name}
	var why []string
	if p := c.Params.Protocol; p != "" {
		v := map[string]string{"D": "0", "S": "-1", "F": "0"}
		for _, x := range c.Params.Params {
			v[strings.ToUpper(x.Name)] = x.Value
		}
		got := irdbSignal(c.Name, p, v["D"], v["S"], v["F"])
		if got.Skip == "" {
			got.Name, got.Source = s.Name, s.Source
			return got
		}
		why = append(why, got.Skip)
	}
	for _, raw := range c.Raw {
		r := record.RawIR{Carrier: raw.Frequency}
		if r.Carrier == 0 {
			r.Carrier = 38000
		}
		for _, part := range []string{raw.Intro, raw.Repeat, raw.Ending} {
			for _, w := range strings.Fields(part) {
				v, err := strconv.ParseInt(strings.TrimPrefix(w, "+"), 10, 64)
				if err != nil {
					return s.rawOr(record.RawIR{}, fmt.Errorf("irfile: a Girr timing %q", w))
				}
				if v < 0 {
					v = -v
				}
				r.Timings = append(r.Timings, uint32(v))
			}
		}
		if len(r.Timings) > 0 {
			return s.raw(r)
		}
	}
	if strings.TrimSpace(c.CCF) != "" {
		r, err := Pronto(c.CCF)
		return s.rawOr(r, err)
	}
	if len(why) == 0 {
		why = append(why, "a command of no code")
	}
	s.Skip = strings.Join(why, "; ")
	return s
}
