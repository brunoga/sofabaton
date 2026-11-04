package irfile

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/record"
)

// lircRemote is a remote of a LIRC configuration (lircd.conf): how its
// signals are made, and its codes.
type lircRemote struct {
	name  string
	flags map[string]bool
	// num is the remote's numeric settings: bits, frequency, gap,
	// pre_data_bits, pre_data, post_data_bits, post_data, plead, ptrail,
	// min_repeat; pair its pairs: header, one, zero, pre, post, foot.
	num   map[string]uint64
	pair  map[string][2]uint32
	codes []lircCode
}

type lircCode struct {
	name string
	// code is a coded remote's; raw a raw one's timings.
	code uint64
	raw  []uint32
}

// ReadLIRC reads a LIRC configuration (lircd.conf, as in LIRC's remotes
// database): its remotes' codes, all of them, as raw IR. Remotes of
// pulse-distance (SPACE_ENC) and bi-phase (RC5, SHIFT_ENC) codes are
// rendered from their timings; raw ones are taken as they are; others
// (RC6, RCMM…) are left out, saying why.
func ReadLIRC(r io.Reader) ([]Signal, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var remotes []*lircRemote
	var cur *lircRemote
	section := ""
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		key := strings.ToLower(f[0])
		switch {
		case key == "begin" && len(f) > 1:
			switch strings.ToLower(f[1]) {
			case "remote":
				cur = &lircRemote{flags: map[string]bool{}, num: map[string]uint64{}, pair: map[string][2]uint32{}}
				remotes = append(remotes, cur)
			case "codes", "raw_codes":
				section = strings.ToLower(f[1])
			}
			continue
		case key == "end" && len(f) > 1:
			if strings.ToLower(f[1]) == "remote" {
				cur = nil
			}
			section = ""
			continue
		case cur == nil:
			continue
		}
		switch section {
		case "codes":
			if len(f) < 2 {
				return nil, fmt.Errorf("irfile: LIRC code %q", line)
			}
			v, err := strconv.ParseUint(f[1], 0, 64)
			if err != nil {
				return nil, fmt.Errorf("irfile: LIRC code %q", line)
			}
			cur.codes = append(cur.codes, lircCode{name: f[0], code: v})
		case "raw_codes":
			if key == "name" && len(f) > 1 {
				cur.codes = append(cur.codes, lircCode{name: f[1]})
				f = f[2:]
			}
			if len(cur.codes) == 0 {
				continue
			}
			c := &cur.codes[len(cur.codes)-1]
			for _, w := range f {
				v, err := strconv.ParseUint(w, 10, 32)
				if err != nil {
					return nil, fmt.Errorf("irfile: LIRC raw timing %q", w)
				}
				c.raw = append(c.raw, uint32(v))
			}
		default:
			switch {
			case key == "name" && len(f) > 1:
				cur.name = strings.Join(f[1:], " ")
			case key == "flags" && len(f) > 1:
				for _, fl := range strings.Split(strings.Join(f[1:], ""), "|") {
					cur.flags[strings.ToUpper(fl)] = true
				}
			case len(f) == 3:
				a, err1 := strconv.ParseUint(f[1], 0, 32)
				b, err2 := strconv.ParseUint(f[2], 0, 32)
				if err1 == nil && err2 == nil {
					cur.pair[key] = [2]uint32{uint32(a), uint32(b)}
				}
			case len(f) == 2:
				if v, err := strconv.ParseUint(f[1], 0, 64); err == nil {
					cur.num[key] = v
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(remotes) == 0 {
		return nil, fmt.Errorf("irfile: not a LIRC configuration (no remote)")
	}
	var out []Signal
	for _, rm := range remotes {
		for _, c := range rm.codes {
			// Remotes' codes are named as LIRC's keys (KEY_VOLUMEUP).
			name := strings.TrimPrefix(c.name, "KEY_")
			s := Signal{Name: commandName(name), Source: c.name}
			r, err := rm.render(c)
			if err != nil {
				s.Skip = err.Error()
			} else {
				s = s.raw(r)
			}
			out = append(out, s)
		}
	}
	return out, nil
}

// lircFrame builds a signal, merging neighboring marks or spaces.
type lircFrame struct {
	t    []uint32
	mark bool
}

func (f *lircFrame) add(mark bool, us uint32) {
	switch {
	case us == 0:
	case len(f.t) > 0 && f.mark == mark:
		f.t[len(f.t)-1] += us
	case len(f.t) == 0 && !mark:
		// A signal starts with a mark.
	default:
		f.t = append(f.t, us)
		f.mark = mark
	}
}

// render is a code of the remote as raw IR: once, then as many repeats as
// the remote's min_repeat asks.
func (rm *lircRemote) render(c lircCode) (record.RawIR, error) {
	freq := int(rm.num["frequency"])
	switch {
	case freq != 0:
	case rm.flags["RC5"] || rm.flags["SHIFT_ENC"]:
		freq = 36000 // RC5's, which its remotes' files leave out
	default:
		freq = 38000
	}
	gap := uint32(rm.num["gap"])
	var f lircFrame
	switch {
	case c.raw != nil || rm.flags["RAW_CODES"]:
		for i, v := range c.raw {
			f.add(i%2 == 0, v)
		}
		if gap == 0 {
			gap = 100000
		}
		f.add(false, gap)
	case rm.flags["SPACE_ENC"] || rm.flags["RC5"] || rm.flags["SHIFT_ENC"]:
		biphase := !rm.flags["SPACE_ENC"]
		one, zero := rm.pair["one"], rm.pair["zero"]
		bits := func(v uint64, n uint64) {
			for i := int(n) - 1; i >= 0; i-- {
				k := uint64(i)
				if rm.flags["REVERSE"] {
					k = n - 1 - uint64(i)
				}
				b := v>>k&1 == 1
				switch {
				case biphase && b:
					f.add(false, one[0])
					f.add(true, one[1])
				case biphase:
					f.add(true, zero[0])
					f.add(false, zero[1])
				case b:
					f.add(true, one[0])
					f.add(false, one[1])
				default:
					f.add(true, zero[0])
					f.add(false, zero[1])
				}
			}
		}
		if h, ok := rm.pair["header"]; ok {
			f.add(true, h[0])
			f.add(false, h[1])
		}
		f.add(true, uint32(rm.num["plead"]))
		bits(rm.num["pre_data"], rm.num["pre_data_bits"])
		if p, ok := rm.pair["pre"]; ok {
			f.add(true, p[0])
			f.add(false, p[1])
		}
		n := rm.num["bits"]
		if n == 0 || n > 64 {
			return record.RawIR{}, fmt.Errorf("LIRC remote %s: %d bits a code", rm.name, n)
		}
		bits(c.code, n)
		if p, ok := rm.pair["post"]; ok {
			f.add(true, p[0])
			f.add(false, p[1])
		}
		bits(rm.num["post_data"], rm.num["post_data_bits"])
		f.add(true, uint32(rm.num["ptrail"]))
		if ft, ok := rm.pair["foot"]; ok {
			f.add(false, ft[0])
			f.add(true, ft[1])
		}
		if gap == 0 {
			gap = 40000
		}
		var sum uint32
		for _, v := range f.t {
			sum += v
		}
		if rm.flags["CONST_LENGTH"] && gap > sum {
			gap -= sum
		}
		f.add(false, gap)
	default:
		flags := make([]string, 0, len(rm.flags))
		for fl := range rm.flags {
			flags = append(flags, fl)
		}
		return record.RawIR{}, fmt.Errorf("LIRC remote %s: codes of kind %s: not supported yet", rm.name, strings.Join(flags, "|"))
	}
	if len(f.t) == 0 {
		return record.RawIR{}, fmt.Errorf("LIRC remote %s: a code of no timings", rm.name)
	}
	r := record.RawIR{Carrier: freq}
	for i := uint64(0); i <= min(rm.num["min_repeat"], 4); i++ {
		r.Timings = append(r.Timings, f.t...)
	}
	return r, nil
}

// writeLIRC writes a LIRC configuration of raw codes: a remote for each
// carrier the signals use, its gap the longest of their last spaces.
func writeLIRC(w io.Writer, name string, signals []Signal) ([]Skipped, error) {
	type code struct {
		name string
		t    []uint32
	}
	var carriers []int
	byCarrier := map[int][]code{}
	gaps := map[int]uint32{}
	var skipped []Skipped
	for _, s := range signals {
		r, err := s.Timings()
		if err != nil {
			skipped = append(skipped, Skipped{Name: s.Source, Why: err.Error()})
			continue
		}
		t := pairs(r)
		if _, ok := byCarrier[r.Carrier]; !ok {
			carriers = append(carriers, r.Carrier)
		}
		gaps[r.Carrier] = max(gaps[r.Carrier], t[len(t)-1])
		// LIRC's raw codes end on a mark: the remote's gap follows.
		byCarrier[r.Carrier] = append(byCarrier[r.Carrier], code{lircName(s.Name), t[:len(t)-1]})
	}
	var b strings.Builder
	for _, c := range carriers {
		rn := lircName(name)
		if len(carriers) > 1 {
			rn = fmt.Sprintf("%s_%d", rn, c)
		}
		fmt.Fprintf(&b, "begin remote\n  name  %s\n  flags RAW_CODES\n  eps 30\n  aeps 100\n  frequency %d\n  gap %d\n\n  begin raw_codes\n", rn, c, gaps[c])
		for _, cd := range byCarrier[c] {
			fmt.Fprintf(&b, "    name %s\n", cd.name)
			for i := 0; i < len(cd.t); i += 6 {
				line := make([]string, 0, 6)
				for _, v := range cd.t[i:min(i+6, len(cd.t))] {
					line = append(line, fmt.Sprintf("%7d", v))
				}
				fmt.Fprintf(&b, "      %s\n", strings.Join(line, " "))
			}
		}
		b.WriteString("  end raw_codes\nend remote\n\n")
	}
	_, err := io.WriteString(w, b.String())
	return skipped, err
}

// lircName is a name as LIRC's: KEY_ and its words in capitals, joined by
// underscores (KEY_VOLUME_UP), which ReadLIRC reads back as the name.
func lircName(n string) string {
	return "KEY_" + strings.ToUpper(strings.Join(strings.Fields(n), "_"))
}
