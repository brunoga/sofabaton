package irfile

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/brunoga/sofabaton/irgen"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// Signals are a device's commands as signals, to write to a file: codes
// in a protocol profiles render keep it, learned (raw) codes their
// timings. Others (Bluetooth and network commands, descriptors in other
// protocols) are left out, saying why.
func Signals(d *profile.Device) []Signal {
	var out []Signal
	for _, c := range d.Commands {
		s := Signal{Name: c.Name, Source: c.Name}
		data, err := c.Signal(d)
		if err != nil {
			s.Skip = err.Error()
			out = append(out, s)
			continue
		}
		if r, err := record.ParseRawIR(data); err == nil {
			s.Raw = &r
		} else if desc, err := record.ParseDescriptor(data); err == nil {
			if ir, code, ok := profile.ParseCode(desc); ok {
				s.IR, s.Code = ir, code
			} else {
				s.Skip = fmt.Sprintf("IR code %q: a protocol files cannot hold yet", desc)
			}
		} else {
			s.Skip = "not an IR code"
		}
		out = append(out, s)
	}
	return out
}

// Timings is the signal as timings: a raw signal's own, a code's
// rendered from its protocol (package irgen).
func (s Signal) Timings() (record.RawIR, error) {
	if s.Raw != nil {
		return *s.Raw, nil
	}
	ir := s.IR
	val := func(p *uint8) uint8 {
		if p == nil {
			return 0
		}
		return *p
	}
	d, sub, f := val(ir.Device), val(ir.Subdevice), uint8(s.Code)
	switch strings.ToLower(ir.Protocol) {
	case "nec":
		return irgen.NEC(uint8(ir.System>>8), uint8(ir.System), f), nil
	case "necx":
		return irgen.NECx(d, sub, f), nil
	case "sony12":
		return irgen.Sony(12, d, 0, f), nil
	case "sony15":
		return irgen.Sony(15, d, 0, f), nil
	case "sony20":
		return irgen.Sony(20, d, sub, f), nil
	case "jvc":
		return irgen.JVC(d, f), nil
	case "rc6":
		return irgen.RC6(d, f, false), nil
	case "samsung20":
		return irgen.Samsung20(d, sub, f), nil
	case "panasonic":
		return irgen.Panasonic(d, sub, f), nil
	case "denon":
		return irgen.Denon(d, f), nil
	case "denonk":
		return irgen.DenonK(d, sub, s.Code), nil
	}
	return record.RawIR{}, fmt.Errorf("protocol %s: no timings for it yet", ir.Protocol)
}

// pairs are a signal's timings as mark and space pairs: one ending on a
// mark gets the hub's 40 ms space.
func pairs(r record.RawIR) []uint32 {
	t := r.Timings
	if len(t)%2 == 1 {
		t = append(t[:len(t):len(t)], 40000)
	}
	return t
}

// A Format is a kind of file of IR codes that Write writes, and Read
// reads back.
type Format struct {
	// ID is the format's short name (flipper), Name what people call it,
	// Ext its files' extension.
	ID, Name, Ext string
	write         func(w io.Writer, name string, signals []Signal) ([]Skipped, error)
}

// Formats are the formats signals can be written in: those that keep
// protocols' codes (Flipper's, irdb's, Girr's and Unfolded Circle's) and
// those of timings (the rest), which take any signal.
var Formats = []Format{
	{"flipper", "Flipper Zero", ".ir", writeFlipper},
	{"irdb", "irdb", ".csv", writeIRDB},
	{"smartir", "SmartIR (Broadlink)", ".json", writeSmartIR},
	{"lirc", "LIRC", ".conf", writeLIRC},
	{"girr", "Girr", ".girr", writeGirr},
	{"pronto", "Pronto", ".txt", codesWriter(prontoCode)},
	{"sendir", "Global Caché sendir", ".txt", codesWriter(sendirCode)},
	{"broadlink", "Broadlink", ".txt", codesWriter(broadlinkCode)},
	{"uc", "Unfolded Circle", ".txt", codesWriter(ucHexCode)},
}

// FormatNamed is the format of an ID or name, in any case.
func FormatNamed(name string) (Format, error) {
	var ids []string
	for _, f := range Formats {
		if strings.EqualFold(f.ID, name) || strings.EqualFold(f.Name, name) {
			return f, nil
		}
		ids = append(ids, f.ID)
	}
	return Format{}, fmt.Errorf("irfile: no format %q (%s)", name, strings.Join(ids, ", "))
}

// Write writes the signals of a device named name as a file of the
// format. Signals left out (by Signals, or as the format cannot hold
// them) are listed, each with why.
func (f Format) Write(w io.Writer, name string, signals []Signal) ([]Skipped, error) {
	var skipped []Skipped
	var kept []Signal
	for _, s := range signals {
		if s.Skip != "" {
			skipped = append(skipped, Skipped{Name: s.Source, Why: s.Skip})
			continue
		}
		kept = append(kept, s)
	}
	more, err := f.write(w, name, kept)
	if err != nil {
		return nil, err
	}
	skipped = append(skipped, more...)
	if len(skipped) == len(signals) {
		return skipped, errors.New("irfile: none of the signals can be written in this format")
	}
	return skipped, nil
}

// codesWriter writes codes one a line, each after its name, as ReadCodes
// reads them.
func codesWriter(code func(Signal) (string, error)) func(io.Writer, string, []Signal) ([]Skipped, error) {
	return func(w io.Writer, name string, signals []Signal) ([]Skipped, error) {
		var b strings.Builder
		fmt.Fprintf(&b, "# %s\n", name)
		var skipped []Skipped
		for _, s := range signals {
			c, err := code(s)
			if err != nil {
				skipped = append(skipped, Skipped{Name: s.Source, Why: err.Error()})
				continue
			}
			fmt.Fprintf(&b, "%s: %s\n", s.Name, c)
		}
		_, err := io.WriteString(w, b.String())
		return skipped, err
	}
}

// irdbParams are a code's protocol, device, subdevice (-1 for none) and
// function in irdb's terms, which Girr's are too.
func irdbParams(s Signal) (proto string, d, sub, f int, ok bool) {
	if s.Raw != nil {
		return "", 0, 0, 0, false
	}
	ir := s.IR
	v := func(p *uint8) int {
		if p == nil {
			return 0
		}
		return int(*p)
	}
	d, sub, f = v(ir.Device), -1, int(s.Code)
	switch strings.ToLower(ir.Protocol) {
	case "nec":
		return "NEC1", int(ir.System >> 8), int(ir.System & 0xFF), f, true
	case "necx":
		return "NECx2", d, v(ir.Subdevice), f, true
	case "sony12":
		return "Sony12", d, sub, f, true
	case "sony15":
		return "Sony15", d, sub, f, true
	case "sony20":
		return "Sony20", d, v(ir.Subdevice), f, true
	case "jvc":
		return "JVC", d, sub, f, true
	case "rc6":
		return "RC6", d, sub, f, true
	case "samsung20":
		return "Samsung20", d, v(ir.Subdevice), f, true
	case "panasonic":
		return "Panasonic", d, v(ir.Subdevice), f, true
	case "denon":
		return "Denon", d, sub, f, true
	case "denonk":
		return "Denon-K", d, v(ir.Subdevice), f, true
	}
	return "", 0, 0, 0, false
}
