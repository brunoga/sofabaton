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

// girrOut is the Girr file writeGirr writes: one remote of one command
// set.
type girrOut struct {
	XMLName xml.Name `xml:"remotes"`
	NS      string   `xml:"xmlns,attr"`
	Version string   `xml:"girrVersion,attr"`
	Remote  struct {
		Name string `xml:"name,attr"`
		Set  struct {
			Name     string           `xml:"name,attr"`
			Commands []girrOutCommand `xml:"command"`
		} `xml:"commandSet"`
	} `xml:"remote"`
}

type girrOutCommand struct {
	Name   string `xml:"name,attr"`
	Params *struct {
		Protocol string             `xml:"protocol,attr"`
		Params   []girrOutParameter `xml:"parameter"`
	} `xml:"parameters,omitempty"`
	Raw struct {
		Frequency int    `xml:"frequency,attr"`
		Intro     string `xml:"intro"`
	} `xml:"raw"`
}

type girrOutParameter struct {
	Name  string `xml:"name,attr"`
	Value int    `xml:"value,attr"`
}

// writeGirr writes a Girr file: each signal's timings, and its protocol
// and parameters when it has them.
func writeGirr(w io.Writer, name string, signals []Signal) ([]Skipped, error) {
	var g girrOut
	g.NS, g.Version = "http://www.harctoolbox.org/Girr", "1.0"
	g.Remote.Name, g.Remote.Set.Name = name, name
	var skipped []Skipped
	for _, s := range signals {
		r, err := s.Timings()
		if err != nil {
			skipped = append(skipped, Skipped{Name: s.Source, Why: err.Error()})
			continue
		}
		c := girrOutCommand{Name: s.Name}
		if proto, d, sub, f, ok := irdbParams(s); ok {
			c.Params = &struct {
				Protocol string             `xml:"protocol,attr"`
				Params   []girrOutParameter `xml:"parameter"`
			}{Protocol: proto, Params: []girrOutParameter{{"D", d}}}
			if sub >= 0 {
				c.Params.Params = append(c.Params.Params, girrOutParameter{"S", sub})
			}
			c.Params.Params = append(c.Params.Params, girrOutParameter{"F", f})
		}
		t := pairs(r)
		words := make([]string, len(t))
		for i, v := range t {
			sign := "+"
			if i%2 == 1 {
				sign = "-"
			}
			words[i] = fmt.Sprintf("%s%d", sign, v)
		}
		c.Raw.Frequency, c.Raw.Intro = r.Carrier, strings.Join(words, " ")
		g.Remote.Set.Commands = append(g.Remote.Set.Commands, c)
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return nil, err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(g); err != nil {
		return nil, err
	}
	_, err := io.WriteString(w, "\n")
	return skipped, err
}
