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

// prontoCode is a Pronto code of learned (raw) form: 0000, the carrier's
// word, and the two sequences' lengths, then the pairs.
var prontoCode = regexp.MustCompile(`(?i)\b0000(\s+[0-9a-f]{4}){3,}`)

// ReadPronto reads Pronto codes, one a line, each after its name ("Power:
// 0000 006D 0022 0002 …", or the name on the line before), as published
// by remote control sites and Global Caché's database. Codes of learned
// form (0000) become raw IR; the others are left out, saying why.
func ReadPronto(r io.Reader) ([]Signal, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var out []Signal
	last := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		loc := regexp.MustCompile(`(?i)\b[0-9a-f]{4}(\s+[0-9a-f]{4}){3,}\s*$`).FindStringIndex(line)
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
		s := Signal{Name: commandName(name), Source: name}
		r, err := Pronto(line[loc[0]:])
		if err != nil {
			s.Skip = err.Error()
			out = append(out, s)
			continue
		}
		out = append(out, s.raw(r))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("irfile: no Pronto codes")
	}
	return out, nil
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
