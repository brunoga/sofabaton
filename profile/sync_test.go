package profile_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/profile"
)

func TestSnapshot(t *testing.T) {
	c := populated(t)
	var steps []profile.Progress
	cfg, err := profile.Snapshot(context.Background(), c, func(p profile.Progress) { steps = append(steps, p) })
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Devices) != 2 || len(cfg.Activities) != 1 {
		t.Fatalf("got %d devices and %d activities, want 2 and 1", len(cfg.Devices), len(cfg.Activities))
	}
	var got []string
	for _, p := range steps {
		got = append(got, strings.Join([]string{p.Step, string(rune('0' + p.Done)), string(rune('0' + p.Total))}, " "))
	}
	want := "Reading device TV 0 3|Reading device AVR 1 3|Reading activity Watch TV 2 3|Done 3 3"
	if strings.Join(got, "|") != want {
		t.Errorf("progress:\n%s\nwant:\n%s", strings.Join(got, "|"), want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := profile.Snapshot(ctx, c, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled snapshot: %v", err)
	}
}

// An edited snapshot applied by id renames, deletes and creates; applying
// it again changes nothing.
func TestApplyConfig(t *testing.T) {
	ctx := context.Background()
	c := populated(t)
	cfg, err := profile.Snapshot(ctx, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Rename the TV and the activity, drop the AVR, and add a projector.
	if err := cfg.RenameDevice("TV", "Television"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.RemoveDevice("AVR"); err != nil {
		t.Fatal(err)
	}
	cfg.Activities[0].Name = "Movie Night"
	projector, err := profile.Load(strings.NewReader(`device: Projector
ir: {protocol: NEC, system: 0x00CE}
commands:
  - {name: Power On, code: 0x40}
  - {name: Power Off, code: 0x41}
power: {on: Power On, off: Power Off}
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Devices = append(cfg.Devices, projector)

	var log []string
	opts := profile.ApplyOptions{ByID: true, Prune: true, Delete: true, Report: func(s string) { log = append(log, s) }}
	if err := profile.ApplyConfig(ctx, c, cfg, opts); err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(log, "\n"))
	}
	got := export(t, c)
	if _, ok := got["AVR"]; ok {
		t.Error("the AVR is still on the hub")
	}
	if _, ok := got["Projector"]; !ok {
		t.Errorf("no projector:\n%s", strings.Join(log, "\n"))
	}
	if y := got["Movie Night"]; !strings.Contains(y, "Television: Menu Back") || !strings.Contains(y, "Television: Back") {
		t.Errorf("Movie Night:\n%s\nlog:\n%s", y, strings.Join(log, "\n"))
	}
	if y := got["Television"]; !strings.Contains(y, "Menu Back") {
		t.Errorf("Television:\n%s", y)
	}

	again, err := profile.Snapshot(ctx, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	log = nil
	opts.DryRun = true
	if err := profile.ApplyConfig(ctx, c, again, opts); err != nil {
		t.Fatal(err)
	}
	for _, l := range log {
		if !strings.HasSuffix(l, ": matches") {
			t.Errorf("again: %s", l)
		}
	}
}

func TestApplyConfigCancelled(t *testing.T) {
	c := populated(t)
	before := export(t, c)
	cfg, _ := profile.Snapshot(context.Background(), c, nil)
	cfg.Devices[0].Name = "Television"
	cfg.Activities = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := profile.ApplyConfig(ctx, c, cfg, profile.ApplyOptions{ByID: true, Delete: true}); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want cancelled", err)
	}
	after := export(t, c)
	for k := range before {
		if after[k] != before[k] {
			t.Errorf("%s changed", k)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	for _, tc := range []struct {
		cfg  profile.Config
		want string
	}{
		{profile.Config{Devices: []*profile.Device{{Name: "TV"}, {Name: "tv"}}}, `two devices are named "tv"`},
		{profile.Config{Activities: []*profile.Activity{{Name: "A"}, {Name: "A"}}}, `two activities are named "A"`},
		{profile.Config{Activities: []*profile.Activity{{Name: "A", Devices: []profile.Member{{Device: "TV"}}}}}, `uses device "TV", which is not in the configuration`},
	} {
		if err := tc.cfg.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("got %v, want %q", err, tc.want)
		}
	}
}

// With Base, what the config left as read is not read again: a change
// made on the hub behind its back stays, until the device is edited.
func TestApplyConfigBase(t *testing.T) {
	ctx := context.Background()
	c := populated(t)
	snap := func() *profile.Config {
		cfg, err := profile.Snapshot(ctx, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	cfg, base := snap(), snap()
	behind := snap()
	if err := behind.RenameCommand("TV", "Back", "Return"); err != nil {
		t.Fatal(err)
	}
	behind.Activities[0].Macros[0].Name = "Menus"
	opts := profile.ApplyOptions{ByID: true, Prune: true, Delete: true}
	if err := profile.ApplyConfig(ctx, c, behind, opts); err != nil {
		t.Fatal(err)
	}

	projector, err := profile.Load(strings.NewReader(`device: Projector
ir: {protocol: NEC, system: 0x00CE}
commands:
  - {name: Power On, code: 0x40}
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Devices = append(cfg.Devices, projector)
	var log []string
	opts.Base = base
	opts.Report = func(s string) { log = append(log, s) }
	if err := profile.ApplyConfig(ctx, c, cfg, opts); err != nil {
		t.Fatal(err)
	}
	got := export(t, c)
	if _, ok := got["Projector"]; !ok || !strings.Contains(got["TV"], "Return") || !strings.Contains(got["Watch TV"], "Menus") {
		t.Errorf("TV:\n%s\nWatch TV:\n%s\nlog:\n%s", got["TV"], got["Watch TV"], strings.Join(log, "\n"))
	}

	// Editing the TV reads it, and its activity, again.
	if err := cfg.RenameCommand("TV", "Menu", "Home"); err != nil {
		t.Fatal(err)
	}
	log = nil
	if err := profile.ApplyConfig(ctx, c, cfg, opts); err != nil {
		t.Fatal(err)
	}
	got = export(t, c)
	if tv := got["TV"]; strings.Contains(tv, "Return") || !strings.Contains(tv, "Home") {
		t.Errorf("TV:\n%s\nlog:\n%s", tv, strings.Join(log, "\n"))
	}
	if !strings.Contains(got["Watch TV"], "Both Menus") {
		t.Errorf("Watch TV:\n%s\nlog:\n%s", got["Watch TV"], strings.Join(log, "\n"))
	}
}
