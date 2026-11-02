package profile_test

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/profile"
)

func editConfig(t *testing.T) *profile.Config {
	t.Helper()
	tv, err := profile.Load(strings.NewReader(`device: TV
ir: {protocol: NEC, system: 0x00CE}
commands:
  - {name: Power On, code: 0x01}
  - {name: Power Off, code: 0x02}
  - {name: Menu, code: 0x10}
power: {on: Power On, off: Power Off}
buttons:
  OK: Menu
macros:
  - name: Twice
    steps: [Menu, Menu]
`))
	if err != nil {
		t.Fatal(err)
	}
	avr, err := profile.Load(strings.NewReader(`device: AVR
ir: {protocol: NEC, system: 0x00CE}
commands:
  - {name: HDMI 1, code: 0x23}
  - {name: Volume Up, code: 0x24}
inputs: [HDMI 1]
`))
	if err != nil {
		t.Fatal(err)
	}
	a, err := profile.LoadActivity(strings.NewReader(`activity: Watch TV
devices:
  - {device: AVR, input: HDMI 1}
  - {device: TV}
buttons:
  Vol+: 'AVR: Volume Up'
  Menu: {short: 'TV: Menu', long: 'AVR: HDMI 1'}
  Red: Both
power_on: [{power on: TV}, {input: AVR}, TV: Menu]
macros:
  - name: Both
    steps: [TV: Menu, AVR: HDMI 1]
favorites: ['TV: Menu', 'AVR: HDMI 1']
`))
	if err != nil {
		t.Fatal(err)
	}
	return &profile.Config{Devices: []*profile.Device{tv, avr}, Activities: []*profile.Activity{a}}
}

func yamlOf(t *testing.T, v any) string {
	t.Helper()
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestConfigEdits(t *testing.T) {
	cfg := editConfig(t)
	if err := cfg.RenameDevice("tv", "Television"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.RenameCommand("Television", "menu", "Home"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := yamlOf(t, cfg.Activities[0]) + yamlOf(t, cfg.Devices[0])
	for _, want := range []string{"device: Television", "short: 'Television: Home'", "- Television: Home", "- power on: Television",
		"favorites:\n    - 'Television: Home'", "OK: Home", "steps:\n        - Home\n        - Home"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, " TV:") || strings.Contains(got, "'TV:") || strings.Contains(got, ": TV\n") || strings.Contains(got, ": Menu") || strings.Contains(got, "- Menu") {
		t.Errorf("old names left:\n%s", got)
	}

	if err := cfg.RemoveDevice("AVR"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got = yamlOf(t, cfg.Activities[0])
	if strings.Contains(got, "AVR") {
		t.Errorf("AVR left:\n%s", got)
	}
	// The Menu key keeps its short press; Vol+ goes.
	if !strings.Contains(got, "Menu: 'Television: Home'") || strings.Contains(got, "Vol+") {
		t.Errorf("keys:\n%s", got)
	}

	for _, err := range []error{
		cfg.RenameDevice("Nope", "X"),
		cfg.RenameCommand("Television", "Nope", "X"),
		cfg.RenameCommand("Television", "Home", "power on"),
		cfg.RemoveDevice("AVR"),
	} {
		if err == nil {
			t.Error("an edit that should fail did not")
		}
	}
	cfg.Devices = append(cfg.Devices, &profile.Device{Name: "AVR"})
	if err := cfg.RenameDevice("Television", "avr"); err == nil {
		t.Error("renamed onto another device's name")
	}
}

func TestRemoveCommand(t *testing.T) {
	cfg := editConfig(t)
	if err := cfg.RemoveCommand("tv", "menu"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.RemoveCommand("AVR", "HDMI 1"); err != nil {
		t.Fatal(err)
	}
	got := yamlOf(t, cfg.Devices[0]) + yamlOf(t, cfg.Devices[1]) + yamlOf(t, cfg.Activities[0])
	for _, bad := range []string{"Menu", "HDMI 1", "OK:"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q left in:\n%s", bad, got)
		}
	}
	// The TV's macro lost its steps; its power stays.
	if tv := cfg.Devices[0]; tv.Power == nil || len(tv.Macros[0].Steps) != 0 {
		t.Errorf("TV: %+v", tv)
	}
	if err := cfg.Validate(); err != nil {
		t.Error(err)
	}
	if cfg.RemoveCommand("TV", "Nope") == nil || cfg.RemoveCommand("Nope", "Menu") == nil {
		t.Error("removed what is not there")
	}
}

func TestMacroEdits(t *testing.T) {
	cfg := editConfig(t)
	a := cfg.Activities[0]
	a.Buttons["Blue"] = profile.Key{Short: "TV: Twice"}
	a.Favorites = append(a.Favorites, "TV: Twice")
	if err := cfg.RenameMacro("tv", "twice", "Double Menu"); err != nil {
		t.Fatal(err)
	}
	got := yamlOf(t, a)
	if !strings.Contains(got, "Blue: 'TV: Double Menu'") || !strings.Contains(got, "- 'TV: Double Menu'") || strings.Contains(got, "Twice") {
		t.Errorf("after the rename:\n%s", got)
	}
	if cfg.RenameMacro("TV", "Double Menu", "Menu") == nil {
		t.Error("a macro took a command's name")
	}
	if err := cfg.RemoveMacro("TV", "double menu"); err != nil {
		t.Fatal(err)
	}
	got = yamlOf(t, a)
	if strings.Contains(got, "Double Menu") || len(cfg.Devices[0].Macros) != 0 {
		t.Errorf("after the removal:\n%s", got)
	}
	if err := cfg.Validate(); err != nil {
		t.Error(err)
	}
}
