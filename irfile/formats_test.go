package irfile_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
			continue // its timings are IRremoteESP8266's own: checked below
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
	// Sharp's address 1, function 1, as timings; with Denon's extension
	// bits, a Denon code.
	sigs, _ := irfile.Read("codes.txt", strings.NewReader("Sharp: 14;0x4202;15;0\nDenon: 14;0x4200;15;0\n"))
	want := irgen.Sharp(1, 1)
	if r := sigs[0].Raw; r == nil || !slices.Equal(r.Timings, want.Timings) {
		t.Errorf("Sharp: %+v", sigs[0])
	}
	if s := sigs[1]; s.IR.Protocol != "Denon" || *s.IR.Device != 1 || s.Code != 1 {
		t.Errorf("Denon: %+v", s)
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

# RC6 mode 0 as LIRC's files have it: the start bit, mode and toggle in
# pre_data, all inverted, the toggle twice as long.
begin remote
  name  rc6
  bits 16
  flags RC6|CONST_LENGTH
  header 2667 889
  one 444 444
  zero 444 444
  pre_data_bits 5
  pre_data 0xF
  gap 107000
  rc6_mask 0x10000
  begin codes
    KEY_MENU 0xFBF3
  end codes
end remote

begin remote
  name  rcmm
  bits 24
  flags RCMM
  begin codes
    KEY_INFO 0x1
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
	// Device 4, function 12.
	if _, menu := sig(t, sigs, "Menu"); menu == nil || menu.Carrier != 36000 || !near(menu.Timings, irgen.RC6(4, 12, false).Timings) {
		t.Errorf("Menu: %v, want %v", menu, irgen.RC6(4, 12, false))
	}
	for _, s := range sigs {
		if s.Source == "KEY_INFO" && !strings.Contains(s.Skip, "RCMM") {
			t.Errorf("KEY_INFO: %+v", s)
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

// A Sofabaton code set's descriptors become codes where profiles render
// their protocol, and stay descriptors where not.
func TestReadSofabaton(t *testing.T) {
	hexOf := func(s string) string { return fmt.Sprintf("%X", s) }
	set := fmt.Sprintf(`{"keys": [
		{"keyname": "Power on", "format": "1", "irCode": %q},
		{"keyname": "Ok/select", "format": "1", "irCode": %q},
		{"keyname": "\nSimp link", "format": "2", "irCode": ""}]}`,
		hexOf("P:NEC R:38400 D:4 S:251 F:196"), hexOf("P:RC5 R:36000 D:0 F:12"))
	sigs, err := irfile.Read("c1.sofabaton", strings.NewReader(set))
	if err != nil || len(sigs) != 3 {
		t.Fatalf("%+v, %v", sigs, err)
	}
	if s := sigs[0]; s.Name != "Power On" || s.IR.Protocol != "NEC" || s.IR.System != 0x04FB || s.Code != 196 {
		t.Errorf("Power on: %+v", s)
	}
	if s := sigs[1]; s.Name != "OK" || s.Descriptor != "P:RC5 R:36000 D:0 F:12" {
		t.Errorf("OK: %+v", s)
	}
	if s := sigs[2]; s.Source != "Simp link" || !strings.Contains(s.Skip, "format 2") {
		t.Errorf("Simp link: %+v", s)
	}
	d, skipped := irfile.Device("LG", sigs)
	if len(d.Commands) != 2 || d.Commands[1].Descriptor != "P:RC5 R:36000 D:0 F:12" || len(skipped) != 1 {
		t.Errorf("device %+v, skipped %+v", d.Commands, skipped)
	}
}

// A Sofabaton code set's timings are raw IR, and its power keys are the
// commands that switch it: discrete keys, a toggle for both, or a toggle
// for the side without a key.
func TestSofabatonPower(t *testing.T) {
	hexOf := func(s string) string { return fmt.Sprintf("%X", s) }
	keys := fmt.Sprintf(`"keys": [
		{"fid": 1, "keyname": "Power", "format": "1", "irCode": %q},
		{"fid": 5907, "keyname": "Power on", "format": "1", "fre": "37825", "irCode": "", "pulse": "2249,612,550,75871"},
		{"fid": 5912, "keyname": "Power off", "format": "1", "irCode": %q}]`,
		hexOf("P:NEC R:38400 D:4 S:251 F:8"), hexOf("P:NEC R:38400 D:4 S:251 F:197"))
	sigs, err := irfile.ReadSofabaton(strings.NewReader("{" + keys + "}"))
	if err != nil {
		t.Fatal(err)
	}
	if r := sigs[1].Raw; r == nil || r.Carrier != 37825 || !slices.Equal(r.Timings, []uint32{2249, 612, 550, 75871}) {
		t.Errorf("timings: %+v", sigs[1])
	}
	for _, c := range []struct{ power, on, off string }{
		{`{"type": 2, "on": [{"fid": 5907}], "off": [{"fid": 5912}]}`, "Power On", "Power Off"},
		{`{"type": 1, "on": [{"fid": 1}], "off": [{"fid": 1}]}`, "Power", "Power"},
		{`{"type": 2, "on": [], "off": [{"fid": 5912}]}`, "Power", "Power Off"},
		{`{"type": 2, "on": [{"fid": 777}], "off": [{"fid": 5912}]}`, "Power", "Power Off"},
		{`{"type": 0, "on": [], "off": []}`, "", ""},
	} {
		on, off, err := irfile.SofabatonPower(strings.NewReader(`{` + keys + `, "power": ` + c.power + `}`))
		if err != nil || on != c.on || off != c.off {
			t.Errorf("%s: %q, %q, %v; want %q, %q", c.power, on, off, err, c.on, c.off)
		}
	}
}

// A Bluetooth profile's keys are HID usages, which become the data our
// X2 holds for the device made of the AWOL profile
// (testdata/x2/bluetooth-projector.yaml).
func TestReadSofabatonBluetooth(t *testing.T) {
	set := `{"code_type": "3", "keys": [
		{"fid": 111, "keyname": "Input", "format": "3", "pulse": "0c01bb", "irCode": ""},
		{"fid": 106, "keyname": "Mute", "format": "3", "pulse": "0C00E2", "irCode": ""},
		{"fid": 1, "keyname": "Power", "format": "3", "pulse": "0C0030", "irCode": ""},
		{"fid": 2, "keyname": "Odd", "format": "3", "pulse": "0c", "irCode": ""}],
		"power": {"type": 1, "on": [{"fid": 1}], "off": [{"fid": 1}]}}`
	sigs, err := irfile.ReadSofabaton(strings.NewReader(set))
	if err != nil {
		t.Fatal(err)
	}
	d, skipped := irfile.Device("Projector", sigs)
	if len(skipped) != 1 || skipped[0].Name != "Odd" {
		t.Errorf("skipped %+v", skipped)
	}
	var got []string
	for _, c := range d.Commands {
		got = append(got, c.Name+" "+c.Data)
	}
	if want := []string{"Input 0c01bb", "Mute 0c00e2", "Power 0c0030"}; !slices.Equal(got, want) {
		t.Errorf("commands %q, want %q", got, want)
	}
	if on, off, err := irfile.SofabatonPower(strings.NewReader(set)); on != "Power" || off != "Power" || err != nil {
		t.Errorf("power %q, %q, %v", on, off, err)
	}
}
