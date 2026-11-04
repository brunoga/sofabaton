package irfile

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/record"
)

// codeForms are the forms of code ReadCodes finds at a line's end, each
// with how it becomes a signal.
var codeForms = []struct {
	re   *regexp.Regexp
	make func(s Signal, code string) Signal
}{
	{regexp.MustCompile(`(?i)\bsendir,\S+$`), func(s Signal, code string) Signal {
		r, err := GlobalCache(code)
		return s.rawOr(r, err)
	}},
	{regexp.MustCompile(`(?i)\b[a-z_0-9]+\s*;\s*(0x)?[0-9a-f]+\s*;\s*\d+\s*;\s*\d+$`), ucHexSignal},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{4}(\s+[0-9a-f]{4}){3,}$`), func(s Signal, code string) Signal {
		r, err := Pronto(code)
		return s.rawOr(r, err)
	}},
	{regexp.MustCompile(`(?:\bJg[A-Za-z0-9+/]{6,}={0,2}|(?i)\b26[0-9a-f]{10,})$`), func(s Signal, code string) Signal {
		b, err := broadlinkText(code)
		if err != nil {
			s.Skip = "not a Broadlink code in base64 or hex"
			return s
		}
		r, err := Broadlink(b)
		return s.rawOr(r, err)
	}},
}

// rawOr is s carrying r, or left out for err.
func (s Signal) rawOr(r record.RawIR, err error) Signal {
	if err != nil {
		s.Skip = err.Error()
		return s
	}
	return s.raw(r)
}

// ReadCodes reads codes one a line, each after its name ("Power: 0000
// 006D …", or the name on the line before), as remote control sites,
// forums and other apps publish them: Pronto codes of learned form
// (0000), Global Caché's sendir commands, Broadlink's codes in base64 or
// hex, and Unfolded Circle's hex codes (protocol;value;bits;repeat).
// Those that cannot be sent are left out, saying why.
func ReadCodes(r io.Reader) ([]Signal, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var out []Signal
	last := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var loc []int
		var form func(Signal, string) Signal
		for _, f := range codeForms {
			if loc = f.re.FindStringIndex(line); loc != nil {
				form = f.make
				break
			}
		}
		if loc == nil {
			last = line
			continue
		}
		name := strings.TrimSpace(strings.Trim(strings.TrimSpace(line[:loc[0]]), ":=,;\t\""))
		if name == "" {
			name = last
		}
		if name == "" {
			name = fmt.Sprintf("Code %d", len(out)+1)
		}
		last = ""
		out = append(out, form(Signal{Name: commandName(name), Source: name}, line[loc[0]:]))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("irfile: no codes (Pronto, sendir, Broadlink or Unfolded Circle) found")
	}
	return out, nil
}

// GlobalCache is the raw IR of a Global Caché sendir command
// (sendir,<module:port>,<id>,<frequency>,<repeat>,<offset>,<on>,<off>,…):
// its timings, in carrier periods, sent repeat times, the repeats from
// the offset (counted from 1) of the timings on.
func GlobalCache(cmd string) (record.RawIR, error) {
	f := strings.Split(strings.TrimSpace(cmd), ",")
	if len(f) < 8 || !strings.EqualFold(f[0], "sendir") {
		return record.RawIR{}, fmt.Errorf("sendir: %q is not a sendir command", cmd)
	}
	var n []int
	for _, w := range f[3:] {
		v, err := strconv.Atoi(strings.TrimSpace(w))
		if err != nil || v < 0 {
			return record.RawIR{}, fmt.Errorf("sendir: %q is not a number", w)
		}
		n = append(n, v)
	}
	freq, repeat, offset, t := n[0], n[1], n[2], n[3:]
	if freq < 15000 || freq > 0xFFFF {
		return record.RawIR{}, fmt.Errorf("sendir: a frequency of %d Hz", freq)
	}
	if len(t)%2 == 1 || offset < 1 || offset > len(t) {
		return record.RawIR{}, fmt.Errorf("sendir: %d timings from offset %d", len(t), offset)
	}
	us := func(v int) uint32 { return uint32((int64(v)*1000000 + int64(freq)/2) / int64(freq)) }
	r := record.RawIR{Carrier: freq}
	for _, v := range t {
		r.Timings = append(r.Timings, us(v))
	}
	for i := 1; i < min(max(repeat, 1), 5); i++ {
		for _, v := range t[offset-1:] {
			r.Timings = append(r.Timings, us(v))
		}
	}
	return r, nil
}

// prontoClock is the Pronto's reference: a carrier word n is 4,145,146 /
// n Hz, and a timing of n is n carrier periods.
const prontoClock = 4145146

// Pronto is the raw IR of a Pronto code of learned form: its once
// sequence, then its repeat sequence once (a remote's short press).
func Pronto(code string) (record.RawIR, error) {
	var w []int
	for _, f := range strings.Fields(code) {
		v, err := strconv.ParseUint(f, 16, 16)
		if err != nil {
			return record.RawIR{}, fmt.Errorf("Pronto: %q is not a word in hex", f)
		}
		w = append(w, int(v))
	}
	if len(w) < 4 {
		return record.RawIR{}, fmt.Errorf("Pronto: a code of %d words", len(w))
	}
	if w[0] != 0 {
		return record.RawIR{}, fmt.Errorf("Pronto: a code of form %04X, not of learned form (0000)", w[0])
	}
	if w[1] == 0 {
		return record.RawIR{}, fmt.Errorf("Pronto: a carrier word of zero")
	}
	once, repeat := w[2], w[3]
	if len(w) != 4+2*(once+repeat) {
		return record.RawIR{}, fmt.Errorf("Pronto: %d words for %d and %d pairs", len(w), once, repeat)
	}
	r := record.RawIR{Carrier: (prontoClock + w[1]/2) / w[1]}
	period := 1e6 / (float64(prontoClock) / float64(w[1]))
	for _, v := range w[4:] {
		r.Timings = append(r.Timings, uint32(float64(v)*period+0.5))
	}
	return r, nil
}
