package irfile_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/irfile"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// descriptors are the descriptors d's commands render, by name.
func descriptors(t *testing.T, d *profile.Device) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, c := range d.Commands {
		sig, err := c.Signal(d)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if r, err := record.ParseRawIR(sig); err == nil {
			// Raw IR, as its carrier and how many timings.
			out[c.Name] = fmt.Sprintf("raw %d Hz, %d timings", r.Carrier, len(r.Timings))
			continue
		}
		desc, err := record.ParseDescriptor(sig)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		out[c.Name] = desc
	}
	return out
}

// skipped is why each signal was left out, by its name in the file.
func skipped(s []irfile.Skipped) map[string]string {
	out := map[string]string{}
	for _, k := range s {
		out[k.Name] = k.Why
	}
	return out
}

// A made-up Flipper file of a projector (NEC, a 16-bit address), with a
// key of another address, a Sony key, a raw signal, a protocol the X2 is
// not known to send, and a name twice.
const flipperFile = `Filetype: IR signals file
Version: 1
# A projector.
#
name: Power
type: parsed
protocol: NECext
address: 00 CE 00 00
command: 12 ED 00 00
#
name: Vol_up
type: parsed
protocol: NECext
address: 00 CE 00 00
command: 0A F5 00 00
#
name: Input
type: parsed
protocol: NEC
address: 04 00 00 00
command: 0B 00 00 00
#
name: Sleep
type: parsed
protocol: SIRC20
address: 3A 07 00 00
command: 36 00 00 00
#
name: Ok
type: raw
frequency: 38000
duty_cycle: 0.330000
data: 9000 4500 560 560 560 1690
#
name: Mute
type: parsed
protocol: RC5
address: 00 00 00 00
command: 0D 00 00 00
#
name: Vol_up
type: parsed
protocol: NECext
address: 00 CE 00 00
command: 0B F4 00 00
`

// A made-up Flipper file of a Denon receiver's keys (Kaseikyo, Denon's
// vendor: Denon-K, as Denon's charts number them), with a Panasonic key
// and one of another vendor.
const kaseikyoFile = `Filetype: IR signals file
Version: 1
#
name: Vol_up
type: parsed
protocol: Kaseikyo
address: 41 54 32 00
command: 70 01 00 00
#
name: Mute
type: parsed
protocol: Kaseikyo
address: 41 54 32 00
command: 72 01 00 00
#
name: Netflix
type: parsed
protocol: Kaseikyo
address: 80 02 20 01
command: A0 00 00 00
#
name: Odd
type: parsed
protocol: Kaseikyo
address: 00 03 01 00
command: 01 00 00 00
`

func TestFlipperKaseikyo(t *testing.T) {
	sigs, err := irfile.ReadFlipper(strings.NewReader(kaseikyoFile))
	if err != nil {
		t.Fatal(err)
	}
	d, skip := irfile.Device("Receiver", sigs)
	want := map[string]string{
		// Our X2's receiver has these two, byte for byte.
		"Volume Up": "P:DenonK R:37000 C0:84 C1:50 C2:0 D:4 S:1 F:368 CHECKSUM:86",
		"Mute":      "P:DenonK R:37000 C0:84 C1:50 C2:0 D:4 S:1 F:370 CHECKSUM:118",
		// Panasonic {128, 0, 74}, as Flipper keeps it: upstream's sample,
		// byte for byte (less the MUL:1 the hub does not send).
		"Netflix": "P:Panasonic R:37000 C0:2 C1:32 D:128 S:0 F:74 CHECKSUM:202",
		// Another vendor's: raw timings, a header, 48 bits, a stop.
		"Odd": "raw 37000 Hz, 100 timings",
	}
	got := descriptors(t, d)
	for n, w := range want {
		if got[n] != w {
			t.Errorf("%s: %q, want %q", n, got[n], w)
		}
	}
	if len(skip) != 0 {
		t.Errorf("skipped %v", skip)
	}
}

func TestFlipper(t *testing.T) {
	sigs, err := irfile.ReadFlipper(strings.NewReader(flipperFile))
	if err != nil {
		t.Fatal(err)
	}
	d, skip := irfile.Device("Projector", sigs)
	if d.IR == nil || d.IR.Protocol != "NEC" || d.IR.System != 0x00CE {
		t.Fatalf("the device's ir: %+v", d.IR)
	}
	want := map[string]string{
		"Power":     "P:NEC R:38400 D:0 S:206 F:18",
		"Volume Up": "P:NEC R:38400 D:0 S:206 F:10",
		"Input":     "P:NEC R:38400 D:4 S:251 F:11",
		// 0x073A: device 26, subdevice 57.
		"Sleep": "P:Sony20 R:40000 D:26 S:57 F:54 MUL:2",
		// Raw timings as they are; RC5 rendered as timings.
		"OK":   "raw 38000 Hz, 6 timings",
		"Mute": "raw 36000 Hz, 48 timings",
	}
	got := descriptors(t, d)
	if len(got) != len(want) {
		t.Errorf("commands %v", got)
	}
	for n, w := range want {
		if got[n] != w {
			t.Errorf("%s: %q, want %q", n, got[n], w)
		}
	}
	why := skipped(skip)
	for n, w := range map[string]string{"Vol_up": "same name"} {
		if !strings.Contains(why[n], w) {
			t.Errorf("%s skipped for %q, want %q", n, why[n], w)
		}
	}
	if err := d.Validate(); err != nil {
		t.Error(err)
	}
	if _, err := irfile.ReadFlipper(strings.NewReader("Filetype: Flipper SubGhz Key File\n")); err == nil {
		t.Error("a Sub-GHz file reads")
	}
}

// A made-up irdb file: NEC, a Sony key, RC5 and RCA (as timings), and
// a function beyond NEC's.
const irdbFile = `functionname,protocol,device,subdevice,function
POWER,NEC1,4,-1,8
VOLUME +,NEC1,4,-1,2
KEY_VOLUMEDOWN,NEC1,4,-1,3
HDMI_1,NEC1,4,-1,90
SLEEP,Sony12,1,-1,54
TEXT,RC5,0,-1,60
GUIDE,RCA,15,-1,229
BIG,NEC1,4,-1,300
SUB,RC5,0,3,1
`

func TestIRDB(t *testing.T) {
	sigs, err := irfile.ReadIRDB(strings.NewReader(irdbFile))
	if err != nil {
		t.Fatal(err)
	}
	d, skip := irfile.Device("TV", sigs)
	want := map[string]string{
		"Power":       "P:NEC R:38400 D:4 S:251 F:8",
		"Volume Up":   "P:NEC R:38400 D:4 S:251 F:2",
		"Volume Down": "P:NEC R:38400 D:4 S:251 F:3",
		"HDMI 1":      "P:NEC R:38400 D:4 S:251 F:90",
		"Sleep":       "P:Sony12 R:40000 D:1 F:54 MUL:2",
		"Text":        "raw 36000 Hz, 48 timings",
		"Guide":       "raw 58000 Hz, 52 timings",
	}
	got := descriptors(t, d)
	if len(got) != len(want) {
		t.Errorf("commands %v", got)
	}
	for n, w := range want {
		if got[n] != w {
			t.Errorf("%s: %q, want %q", n, got[n], w)
		}
	}
	why := skipped(skip)
	if !strings.Contains(why["SUB"], "RC5") || !strings.Contains(why["BIG"], "300") {
		t.Errorf("skipped %v", why)
	}
	if _, err := irfile.ReadIRDB(strings.NewReader("a,b\n1,2\n")); err == nil {
		t.Error("a CSV that is not irdb's reads")
	}
}

func TestPronto(t *testing.T) {
	file := `# A receiver
Power: 0000 006D 0002 0000 0157 00AC 0015 0689
Volume Up
0000 006D 0000 0002 0157 00AC 0015 0689
Odd = 0100 006D 0000 0001 0157 00AC
`
	sigs, err := irfile.Read("receiver.txt", strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	d, skip := irfile.Device("Receiver", sigs)
	got := descriptors(t, d)
	if len(got) != 2 || got["Power"] != "raw 38029 Hz, 4 timings" || got["Volume Up"] != "raw 38029 Hz, 4 timings" {
		t.Errorf("commands %v", got)
	}
	if why := skipped(skip)["Odd"]; !strings.Contains(why, "not of learned form") {
		t.Errorf("Odd skipped for %q", why)
	}
	r, err := irfile.Pronto("0000 006D 0002 0000 0157 00AC 0015 0689")
	if err != nil || r.Timings[0] != 9019 || r.Timings[1] != 4523 || r.Timings[2] != 552 || r.Timings[3] != 43993 {
		t.Errorf("timings %v (%v)", r.Timings, err)
	}
}
