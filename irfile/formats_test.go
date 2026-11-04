package irfile_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/irfile"
	"github.com/brunoga/sofabaton/irgen"
	"github.com/brunoga/sofabaton/record"
)

// near reports whether got's timings are want's within 3%, but for
// gaps (over 10 ms), which encoders round to their frames differently.
func near(got, want []uint32) bool {
	if len(got) < len(want) {
		return false
	}
	for i, w := range want {
		if w > 10000 {
			continue
		}
		g := got[i]
		if g*100 < w*97 || g*100 > w*103 {
			return false
		}
	}
	return true
}

// sig is the signal of the command named name: its descriptor, or its raw
// timings.
func sig(t *testing.T, sigs []irfile.Signal, name string) (string, *record.RawIR) {
	t.Helper()
	d, _ := irfile.Device("X", sigs)
	for _, c := range d.Commands {
		if c.Name != name {
			continue
		}
		data, err := c.Signal(d)
		if err != nil {
			t.Fatal(err)
		}
		if r, err := record.ParseRawIR(data); err == nil {
			return "", &r
		}
		desc, err := record.ParseDescriptor(data)
		if err != nil {
			t.Fatal(err)
		}
		return desc, nil
	}
	t.Fatalf("no command %q in %+v", name, d.Commands)
	return "", nil
}

// Upstream's vectors: codes the X2 has descriptors for become them, the
// others the same timings as Home Assistant's encoders give.
func TestUCHex(t *testing.T) {
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
	desc := map[string]string{
		"onkyo-nec-extended":  "P:NEC R:38400 D:210 S:108 F:203",
		"nec-standard-repeat": "P:NEC R:38400 D:4 S:251 F:8",
		"sony12":              "P:Sony12 R:40000 D:1 F:21 MUL:2",
		"sony12-repeat-2":     "P:Sony12 R:40000 D:1 F:21 MUL:2",
		"sony15":              "P:Sony15 R:40000 D:77 F:29 MUL:2",
		"samsung":             "P:NECx R:38400 D:7 S:7 F:2",
		"rc6-toggle":          "P:RC6 R:36000 D:1 F:12 MUL:1",
		"sony20":              "P:Sony20 R:40000 D:1 S:29 F:52 MUL:2",
		// Panasonic's vendor: the descriptor (its checksum the
		// renderer's).
		"panasonic": "P:Panasonic R:37000 C0:2 C1:32 D:128 S:0 F:61 ",
	}
	for _, v := range f.Vectors {
		sigs, err := irfile.Read("codes.txt", strings.NewReader("Key: "+v.Text+"\n"))
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		if v.Name == "sharp" {
			continue // not supported: checked below
		}
		got, raw := sig(t, sigs, "Key")
		switch {
		case desc[v.Name] != "":
			// A want ending in a space is the descriptor's start.
			prefix := strings.HasSuffix(desc[v.Name], " ") && strings.HasPrefix(got, desc[v.Name])
			if got != desc[v.Name] && !prefix {
				t.Errorf("%s: %q, want %q", v.Name, got, desc[v.Name])
			}
		case raw == nil:
			t.Errorf("%s: %q, want timings", v.Name, got)
		case !near(raw.Timings, v.Timings):
			t.Errorf("%s: timings %v, want %v", v.Name, raw.Timings, v.Timings)
		}
	}
	sigs, _ := irfile.Read("codes.txt", strings.NewReader("Sharp: 14;0x4202;15;0\n"))
	if !strings.Contains(sigs[0].Skip, "not supported") {
		t.Errorf("Sharp: %+v", sigs[0])
	}
}

// broadlink is a Broadlink code: 0x26, a repeat count, the timings'
// length, and the timings in ticks (a zero and two bytes when long).
func broadlink(repeat byte, ticks ...int) []byte {
	var body []byte
	for _, v := range ticks {
		if v > 255 {
			body = append(body, 0, byte(v>>8), byte(v))
		} else {
			body = append(body, byte(v))
		}
	}
	return append([]byte{0x26, repeat, byte(len(body)), byte(len(body) >> 8)}, append(body, 0x0d, 0x05)...)
}

func TestBroadlink(t *testing.T) {
	r, err := irfile.Broadlink(broadlink(1, 274, 137, 17, 51))
	if err != nil {
		t.Fatal(err)
	}
	if r.Carrier != 38000 || !slices.Equal(r.Timings, []uint32{8997, 4499, 558, 1675, 8997, 4499, 558, 1675}) {
		t.Errorf("%+v", r)
	}
	if _, err := irfile.Broadlink([]byte{0xb2, 0, 1, 0, 9}); err == nil {
		t.Error("an RF code read")
	}

	code := base64.StdEncoding.EncodeToString(broadlink(0, 274, 137, 17, 51))
	file := `{"manufacturer": "LG", "supportedController": "Broadlink", "commandsEncoding": "Base64",
	"commands": {"off": "` + code + `", "volumeUp": "` + code + `", "sources": {"HDMI 1": "` + code + `"}}}`
	sigs, err := irfile.Read("1060.json", strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range sigs {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"Power Off", "HDMI 1", "Volume Up"}) {
		t.Errorf("names %q", names)
	}
	if _, raw := sig(t, sigs, "Volume Up"); raw == nil || raw.Timings[0] != 8997 {
		t.Errorf("Volume Up: %+v", raw)
	}
	other := `{"supportedController": "ESPHome", "commandsEncoding": "Raw", "commands": {"off": "[1, 2]"}}`
	if sigs, err := irfile.ReadSmartIR(strings.NewReader(other)); err != nil || !strings.Contains(sigs[0].Skip, "Raw") {
		t.Errorf("an ESPHome file: %+v %v", sigs, err)
	}
	if sigs, err := irfile.Read("tv.txt", strings.NewReader("Power: "+code+"\n")); err != nil || sigs[0].Raw == nil {
		t.Errorf("a line of base64: %+v %v", sigs, err)
	}
}

const lircConf = `# A Yamaha receiver, an RC5 amplifier and raw codes.
begin remote
  name  Yamaha_RAV
  bits           16
  flags SPACE_ENC|CONST_LENGTH
  header       9067  4393
  one           642  1572
  zero          642   472
  ptrail        643
  pre_data_bits   16
  pre_data       0x5EA1
  gap          108000
      begin codes
          KEY_POWER                0xF807
          KEY_VOLUMEUP             0x58A7   # comment
      end codes
end remote

begin remote
  name  Philips
  bits           13
  flags RC5|CONST_LENGTH
  one           889   889
  zero          889   889
  plead         889
  gap          113792
  min_repeat      1
  begin codes
    KEY_MUTE 0x100D
  end codes
end remote

begin remote
  name  learned
  flags RAW_CODES
  frequency 36000
  gap 50000
  begin raw_codes
    name KEY_HOME
      9000 4500 560
    name Eject
      100 200 300
      400 500
  end raw_codes
end remote

begin remote
  name  rc6
  bits 16
  flags RC6|CONST_LENGTH
  begin codes
    KEY_MENU 0x1
  end codes
end remote
`

func TestLIRC(t *testing.T) {
	sigs, err := irfile.Read("lircd.conf", strings.NewReader(lircConf))
	if err != nil {
		t.Fatal(err)
	}
	_, power := sig(t, sigs, "Power")
	if power == nil || power.Carrier != 38000 || power.Timings[0] != 9067 || power.Timings[1] != 4393 {
		t.Fatalf("Power: %+v", power)
	}
	var v uint64
	for i := 0; i < 32; i++ {
		if power.Timings[3+2*i] > 1000 {
			v |= 1 << (31 - i)
		}
	}
	if v != 0x5EA1F807 {
		t.Errorf("Power's bits %08X", v)
	}
	var sum uint32
	for _, x := range power.Timings {
		sum += x
	}
	if sum != 108000 {
		t.Errorf("Power's frame of %d µs, want 108000", sum)
	}
	// RC5 as LIRC has it is RC5 as irgen renders it: two frames.
	_, mute := sig(t, sigs, "Mute")
	want := irgen.RC5(0, 13, false).Timings
	if mute == nil || mute.Carrier != 36000 || !near(mute.Timings, want) || len(mute.Timings) != len(want) {
		t.Errorf("Mute: %v, want %v", mute, want)
	}
	if _, home := sig(t, sigs, "Home"); home == nil || !slices.Equal(home.Timings, []uint32{9000, 4500, 560, 50000}) || home.Carrier != 36000 {
		t.Errorf("Home: %+v", home)
	}
	if _, eject := sig(t, sigs, "Eject"); eject == nil || !slices.Equal(eject.Timings, []uint32{100, 200, 300, 400, 500, 50000}) {
		t.Errorf("Eject: %+v", eject)
	}
	for _, s := range sigs {
		if s.Source == "KEY_MENU" && !strings.Contains(s.Skip, "RC6") {
			t.Errorf("KEY_MENU: %+v", s)
		}
	}
}

func TestGlobalCache(t *testing.T) {
	r, err := irfile.GlobalCache("sendir,1:1,1,38000,2,3,342,171,21,21,21,1500")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Timings, []uint32{9000, 4500, 553, 553, 553, 39474, 553, 553, 553, 39474}) || r.Carrier != 38000 {
		t.Errorf("%+v", r)
	}
	sigs, err := irfile.Read("tv.txt", strings.NewReader("Power, sendir,1:1,1,38000,1,1,342,171,21,1500\n"))
	if err != nil || sigs[0].Name != "Power" || sigs[0].Raw == nil {
		t.Errorf("%+v %v", sigs, err)
	}
}

const girr = `<?xml version="1.0" encoding="UTF-8"?>
<remotes>
  <remote name="TV">
    <commandSet name="tv">
      <command name="power">
        <parameters protocol="nec1">
          <parameter name="D" value="4"/>
          <parameter name="F" value="8"/>
        </parameters>
      </command>
      <command name="mute">
        <parameters protocol="Weird"/>
        <raw frequency="36000"><intro>+889 -889 +1778 -889</intro><repeat/><ending/></raw>
      </command>
      <command name="menu"><ccf>0000 006D 0002 0000 0157 00AC 0015 0689</ccf></command>
      <command name="text"><parameters protocol="Weird"/></command>
    </commandSet>
  </remote>
</remotes>`

func TestGirr(t *testing.T) {
	sigs, err := irfile.Read("tv.girr", strings.NewReader(girr))
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := sig(t, sigs, "Power"); d != "P:NEC R:38400 D:4 S:251 F:8" {
		t.Errorf("Power: %q", d)
	}
	if _, m := sig(t, sigs, "Mute"); m == nil || m.Carrier != 36000 || !slices.Equal(m.Timings, []uint32{889, 889, 1778, 889}) {
		t.Errorf("Mute: %+v", m)
	}
	if _, m := sig(t, sigs, "Menu"); m == nil || m.Timings[0] != 9019 {
		t.Errorf("Menu: %+v", m)
	}
	if sigs[3].Raw != nil || !strings.Contains(sigs[3].Skip, "Weird") {
		t.Errorf("text: %+v", sigs[3])
	}
}
