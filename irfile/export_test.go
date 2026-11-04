package irfile_test

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/irfile"
	"github.com/brunoga/sofabaton/irgen"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// learned is the projector's learned power code (testdata/x2).
const learned = "046000000000674d00001d6500001197000001c50000022d000001c50000022d000001ce0000022d000001b40000022d000001a30000022d000001d80000022d000001e10000022d000001eb0000022d000001ba0000022d000001be00000698000001bb00000698000001cb00000698000001d50000022d000001de0000022d000001e400000698000001c100000698000001e10000022d000001c700000698000001be0000022d000001ce00000698000001ab00000698000001ac0000022d000001ac0000022d000001ad0000022d000001ae00000698000001b10000022d000001b100000698000001b40000022c000001b40000022d000001b500000698000001b800000697000001ba00000697000001bd0000ab5100001df3000008bf000001c6000177b5000020d0000008c1000001ce000177b700002223000008c2000001fe000177b8000021df000008c1000001df000177b7000021aa000008c2000001fd000177b700002009000008c10000019f000177b700001da8000008bf000001c4000177b500001e84000008bf000001cb000177b500001d24000008c1000001ac000177b600002035000008c1000001ed000177b70000223e000008c2000001ef000177b80000224d000008c2000001fe000177b8000021e0000008c20000020c000177b8000022d9000008c2000001fe000177b8000022d9000008c20000020c000177b8000022cb000008c20000020b000177b8000022cb000008c20000020c000177b8000022cb000008c20000020c000177b8000022d9000008c20000020c000177b8000022d9000008c20000020c000177b80000226a000008c20000020c000177b80000226a000008c20000020b000177b80000218d000008c20000020b000177b8000022cb000008c20000020c000177b8000022bd000008c20000020b000177b8000022e7000008c2000001fe000177b8000022e7000008c20000020c000177b8000022e6000008c2000001fe000177b8000022e7000008c20000020b000177b8000022d9000008c20000020c000177b8000022af000008c20000020c000177b80000220d000008c20000020c000177b8000021e1000008c20000020c000177b8000022bd000008c2000001fe000177b8000021fc000008c2000001fe000177b8000021d1000008c20000020b000177b800002234000008c20000020c000177b8000021b5000008c20000020c000177b800002230000008c2000001f0000177b7000021af000008c1000001be000177b600001dd2000008bf000001c5000177b500001cf2000008c0000001b4000177b600002213000008c2000001fd000177b700001c81000008c0000001b5000177b600001fc1000008be000001e1000177b400001d53000008c0000001b9000177b600002203000008c2000001ef000177b7000021cf000008c20000020c000177b800002278000008c20000020c000177b8000021fc000008c20000020c000177b800002208000008c20000020c000177b80000225c000008c20000020c000177b80000217c000008c2000001fe00030d4000000000"

// exportDevice has a code in each protocol profiles render, a learned
// code, and a Bluetooth command.
func exportDevice() *profile.Device {
	u8 := func(v uint8) *uint8 { return &v }
	code := func(name string, ir profile.IR, v uint16) profile.Command {
		return profile.Command{Name: name, IR: &ir, Code: &profile.Code{Value: v}}
	}
	return &profile.Device{
		Name: "Test Device",
		IR:   &profile.IR{Protocol: "NEC", System: 0x04FB},
		Commands: []profile.Command{
			{Name: "Power", Code: &profile.Code{Value: 0x08}},
			code("Volume Up", profile.IR{Protocol: "NEC", System: 0x7A85}, 0x1A),
			code("Mute", profile.IR{Protocol: "NEC", System: 0x1234}, 0x09),
			code("Channel Up", profile.IR{Protocol: "NECx", Device: u8(7), Subdevice: u8(7)}, 18),
			code("Channel Down", profile.IR{Protocol: "NECx", Device: u8(7), Subdevice: u8(9)}, 16),
			code("Input HDMI 1", profile.IR{Protocol: "Sony12", Device: u8(1)}, 21),
			code("Input HDMI 2", profile.IR{Protocol: "Sony15", Device: u8(164)}, 26),
			code("Input HDMI 3", profile.IR{Protocol: "Sony20", Device: u8(26), Subdevice: u8(73)}, 97),
			code("Play", profile.IR{Protocol: "JVC", Device: u8(3)}, 23),
			code("Pause", profile.IR{Protocol: "RC6", Device: u8(4)}, 0x0C),
			code("Stop", profile.IR{Protocol: "Samsung20", Device: u8(8), Subdevice: u8(8)}, 5),
			code("Menu", profile.IR{Protocol: "Panasonic", Device: u8(0x80), Subdevice: u8(0x00)}, 0x3D),
			code("Info", profile.IR{Protocol: "Denon", Device: u8(2)}, 121),
			code("Volume Down", profile.IR{Protocol: "DenonK", Device: u8(4), Subdevice: u8(1)}, 369),
			{Name: "Learned", Data: learned},
			{Name: "Pair", Data: "0102030405"},
		},
	}
}

func TestExport(t *testing.T) {
	d := exportDevice()
	want := map[string]irfile.Signal{}
	for _, s := range irfile.Signals(d) {
		if s.Skip == "" {
			want[s.Name] = s
		} else if s.Name != "Pair" {
			t.Errorf("%s left out: %s", s.Name, s.Skip)
		}
	}
	// codes are the formats that keep codes as codes, and the commands
	// whose protocols they lack (kept as timings, or left out).
	codes := map[string][]string{
		"flipper": {"Channel Down", "Play", "Stop", "Info", "Learned"},
		"irdb":    {"Learned"},
		"girr":    {"Learned"},
		"uc":      {"Play", "Stop", "Learned"},
	}
	for _, f := range irfile.Formats {
		t.Run(f.ID, func(t *testing.T) {
			var b bytes.Buffer
			skipped, err := f.Write(&b, d.Name, irfile.Signals(d))
			if err != nil {
				t.Fatal(err)
			}
			got, err := irfile.Read("x"+f.Ext, &b)
			if err != nil {
				t.Fatalf("%v\n%s", err, b.String())
			}
			byName := map[string]irfile.Signal{}
			for _, s := range got {
				if s.Skip != "" {
					t.Errorf("%s read back left out: %s", s.Source, s.Skip)
				}
				byName[s.Name] = s
			}
			left := map[string]bool{}
			for _, s := range skipped {
				left[s.Name] = true
			}
			lacks := map[string]bool{}
			for _, n := range codes[f.ID] {
				lacks[n] = true
			}
			for name, w := range want {
				g, ok := byName[name]
				if left[name] {
					if ok || f.ID != "irdb" && f.ID != "uc" || !lacks[name] {
						t.Errorf("%s left out", name)
					}
					continue
				}
				if !ok {
					t.Errorf("%s not read back (%d read)", name, len(got))
					continue
				}
				if _, isCodes := codes[f.ID]; isCodes && w.Raw == nil && !lacks[name] && g.Raw != nil {
					t.Errorf("%s read back as timings, not a %s code", name, w.IR.Protocol)
				}
				wt, _ := w.Timings()
				gt, err := g.Timings()
				if err != nil {
					t.Errorf("%s: %v", name, err)
					continue
				}
				if f.ID != "broadlink" && f.ID != "smartir" && (gt.Carrier*100 < wt.Carrier*98 || gt.Carrier*100 > wt.Carrier*102) {
					t.Errorf("%s: carrier %d, want %d", name, gt.Carrier, wt.Carrier)
				}
				if len(gt.Timings) != len(wt.Timings)+len(wt.Timings)%2 || !within(gt.Timings, wt.Timings) {
					t.Errorf("%s: timings\n%v, want\n%v", name, gt.Timings, wt.Timings)
				}
			}
			if !left["Pair"] {
				t.Error("the Bluetooth command was not left out")
			}
			if t.Failed() {
				t.Log(b.String())
			}
		})
	}
}

func TestFormatNamed(t *testing.T) {
	for _, n := range []string{"flipper", "Flipper Zero", "LIRC", "uc"} {
		if _, err := irfile.FormatNamed(n); err != nil {
			t.Error(err)
		}
	}
	if _, err := irfile.FormatNamed("wav"); err == nil || !strings.Contains(err.Error(), "pronto") {
		t.Errorf("an unknown format: %v", err)
	}
}

// within reports whether got's timings are want's within 3% or 40 µs
// (a carrier period at 26 kHz, or a Broadlink tick, as files of periods
// and ticks round them), but for the last, a gap.
func within(got, want []uint32) bool {
	for i, w := range want[:len(want)-1] {
		d := int(got[i]) - int(w)
		if d < 0 {
			d = -d
		}
		if d > 40 && d*100 > int(w)*3 {
			return false
		}
	}
	return true
}

// TestTimingsVectors checks the timings codes render to against those of
// the Unfolded Circle vectors (another library's encoders).
func TestTimingsVectors(t *testing.T) {
	data, err := os.ReadFile("testdata/uc-hex-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Vectors []struct {
			Name, Text string
			Timings    []uint32 `json:"timings_us"`
		}
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, v := range f.Vectors {
		switch v.Name {
		case "nec-standard-repeat":
			continue // NEC's repeat code, which we leave out
		case "sharp":
			// IRremoteESP8266's Sharp has its own unit: the bits must
			// match (a 1 a space over twice its mark).
			bitsOf := func(t []uint32) (b []bool) {
				for i := 1; i < len(t); i += 2 {
					b = append(b, t[i] > 2*t[i-1])
				}
				return b
			}
			if got, want := bitsOf(irgen.Sharp(1, 1).Timings), bitsOf(v.Timings); !slices.Equal(got, want) {
				t.Errorf("sharp: bits\n%v, want\n%v", got, want)
			}
			checked++
			continue
		case "rc6-toggle":
			// Codes leave the toggle out: the encoder has it.
			if r := irgen.RC6(1, 12, true); !near(r.Timings, v.Timings[:len(v.Timings)-1]) {
				t.Errorf("%s: timings\n%v, want\n%v", v.Name, r.Timings, v.Timings)
			}
			checked++
			continue
		}
		sigs, err := irfile.ReadCodes(strings.NewReader("Key: " + v.Text + "\n"))
		if err != nil || sigs[0].Skip != "" || sigs[0].Raw != nil {
			continue // raw already: compared by the import tests
		}
		r, err := sigs[0].Timings()
		if err != nil {
			t.Errorf("%s: %v", v.Name, err)
			continue
		}
		if !near(r.Timings, v.Timings[:len(v.Timings)-1]) {
			t.Errorf("%s (%s): timings\n%v, want\n%v", v.Name, sigs[0].IR.Protocol, r.Timings, v.Timings)
		}
		checked++
	}
	if checked < 9 {
		t.Errorf("%d vectors checked", checked)
	}
}

// TestDenonChart checks irgen's Denon and Denon-K against Denon's own
// Pronto codes.
func TestDenonChart(t *testing.T) {
	f, err := os.Open("testdata/denon-chart.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sigs, err := irfile.ReadCodes(f)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]record.RawIR{
		"Zone 2 On":          irgen.DenonK(4, 3, 6),
		"Zone 2 volume up":   irgen.DenonK(4, 3, 368),
		"Zone 3 On":          irgen.Denon(2, 125),
		"Zone 3 volume up":   irgen.Denon(2, 102),
		"Zone 3 Volume Mute": irgen.DenonK(4, 5, 370),
	}
	for _, s := range sigs {
		w, ok := want[s.Source]
		if !ok || s.Raw == nil {
			t.Errorf("%s: %+v", s.Source, s)
			continue
		}
		delete(want, s.Source)
		if len(s.Raw.Timings) != len(w.Timings) || !within(s.Raw.Timings, w.Timings) || s.Raw.Carrier/1000 != w.Carrier/1000 {
			t.Errorf("%s: chart's\n%v, irgen's\n%v", s.Source, s.Raw.Timings, w.Timings)
		}
	}
	for n := range want {
		t.Errorf("%s: not read", n)
	}
}
