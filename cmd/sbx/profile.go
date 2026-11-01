package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/profile"
)

// export writes a device's or an activity's profile from the hub.
func export(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	device := fs.String("device", "", "device name or id (see sbx list)")
	activity := fs.String("activity", "", "activity name or id, instead of -device")
	out := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*device == "") == (*activity == "") {
		return errors.New("one of -device and -activity is required")
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	output := func() (io.Writer, func(), error) {
		if *out == "" {
			return stdout, func() {}, nil
		}
		f, err := os.Create(*out)
		if err != nil {
			return nil, nil, err
		}
		return f, func() { _ = f.Close() }, nil
	}
	if *activity != "" {
		acts, err := c.Activities(ctx)
		if err != nil {
			return err
		}
		act, err := pick("activity", *activity, acts)
		if err != nil {
			return err
		}
		st, err := profile.ReadActivity(ctx, c, act.ID)
		if err != nil {
			return err
		}
		p, err := profile.ActivityFromHub(st)
		if err != nil {
			return err
		}
		w, done, err := output()
		if err != nil {
			return err
		}
		defer done()
		fmt.Fprintf(w, "# %s, exported from %s\n", p.Name, c.Info().Name)
		return p.Encode(w)
	}
	dev, err := findDevice(ctx, c, *device)
	if err != nil {
		return err
	}
	{
		st, err := profile.Read(ctx, c, dev.ID)
		if err != nil {
			return err
		}
		p, err := profile.FromHub(dev, st)
		if err != nil {
			return err
		}
		w, done, err := output()
		if err != nil {
			return err
		}
		defer done()
		fmt.Fprintf(w, "# %s (%s), exported from %s\n", p.Name, p.Brand, c.Info().Name)
		return p.Encode(w)
	}
}

// apply makes a device or an activity on the hub match a profile.
func apply(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	file := fs.String("f", "", "the profile (required)")
	device := fs.String("device", "", "device name or id (default: the profile's id, else its device name)")
	prune := fs.Bool("prune", false, "also remove the hub's commands the profile lacks")
	dry := fs.Bool("n", false, "show the plan only")
	yes := fs.Bool("y", false, "apply without asking")
	create := fs.Bool("create", false, "create the profile's device (a new IR device) or activity")
	activity := fs.String("activity", "", "activity name or id, for an activity's profile (default: the profile's id, else its name)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("-f is required")
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	if isActivityProfile(data) {
		if *device != "" {
			return errors.New("an activity's profile takes -activity, not -device")
		}
		return applyActivity(ctx, data, &h, *activity, *prune, *dry, *yes, *create, stdin, stdout, stderr)
	}
	if *activity != "" {
		return errors.New("a device's profile takes -device, not -activity")
	}
	p, err := profile.Load(bytes.NewReader(data))
	if err != nil {
		return err
	}
	which := *device
	switch {
	case which != "":
	case p.ID != 0:
		which = strconv.Itoa(int(p.ID))
	default:
		which = p.Name
	}

	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if *create {
		if *device != "" {
			return errors.New("-create makes a new device; it takes no -device")
		}
		if _, err := findDevice(ctx, c, p.Name); err == nil {
			return fmt.Errorf("a device named %q exists; rename the profile's device or apply to it", p.Name)
		}
		fmt.Fprintf(stdout, "create IR device %q with %d commands, %d buttons", p.Name, len(p.Commands), len(p.Buttons))
		if p.Power != nil {
			fmt.Fprintf(stdout, ", powered by %q and %q", p.Power.On, p.Power.Off)
		}
		fmt.Fprintln(stdout)
		if *dry {
			return nil
		}
		if err := c.Writable(); err != nil {
			return err
		}
		if !*yes && !confirm(stdin, stdout, "create it?") {
			return errors.New("not created")
		}
		id, err := profile.Create(ctx, c, p, func(s string) { fmt.Fprintln(stdout, "done:", s) })
		if err != nil && id != 0 {
			return fmt.Errorf("%w (device %d is on the hub, uncommitted: sbx remove-device -device %d)", err, id, id)
		}
		return err
	}
	dev, err := findDevice(ctx, c, which)
	if err != nil {
		return err
	}
	id := dev.ID
	st, err := profile.Read(ctx, c, id)
	if err != nil {
		return err
	}
	changes, kept, err := profile.Plan(p, st, *prune)
	if err != nil {
		return err
	}
	for _, ch := range changes {
		fmt.Fprintln(stdout, ch)
	}
	for _, k := range kept {
		fmt.Fprintf(stdout, "keep    %3d %q (not in the profile; -prune removes it)\n", k.ID, k.Label)
	}
	if len(changes) == 0 {
		fmt.Fprintln(stdout, "the hub already matches the profile")
		return nil
	}
	if *dry {
		return nil
	}
	if err := c.Writable(); err != nil {
		return err
	}
	if !*yes && !confirm(stdin, stdout, fmt.Sprintf("apply %d changes to device %d?", len(changes), id)) {
		return errors.New("not applied")
	}
	return profile.Apply(ctx, c, changes, func(ch profile.Change) {
		fmt.Fprintln(stdout, "done:", ch)
	})
}

// isActivityProfile reports whether a profile describes an activity (its
// first key is activity:) rather than a device.
func isActivityProfile(data []byte) bool {
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return false
	}
	_, ok := top["activity"]
	return ok
}

// applyActivity makes an activity on the hub match a profile, or creates
// it.
func applyActivity(ctx context.Context, data []byte, h *hubFlags, which string, prune, dry, yes, create bool, stdin io.Reader, stdout, stderr io.Writer) error {
	a, err := profile.LoadActivity(bytes.NewReader(data))
	if err != nil {
		return err
	}
	switch {
	case which != "":
	case a.ID != 0:
		which = strconv.Itoa(int(a.ID))
	default:
		which = a.Name
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	acts, err := c.Activities(ctx)
	if err != nil {
		return err
	}
	if create {
		if _, err := pick("activity", a.Name, acts); err == nil {
			return fmt.Errorf("an activity named %q exists; rename the profile's activity or apply to it", a.Name)
		}
		fmt.Fprintf(stdout, "create activity %q with %d devices, %d buttons, %d macros\n", a.Name, len(a.Devices), len(a.Buttons), len(a.Macros))
		if dry {
			return nil
		}
		if err := c.Writable(); err != nil {
			return err
		}
		if !yes && !confirm(stdin, stdout, "create it?") {
			return errors.New("not created")
		}
		id, err := profile.CreateActivity(ctx, c, a, func(s string) { fmt.Fprintln(stdout, "done:", s) })
		if err != nil && id != 0 {
			return fmt.Errorf("%w (activity %d is on the hub: sbx remove-activity -activity %d)", err, id, id)
		}
		return err
	}
	act, err := pick("activity", which, acts)
	if err != nil {
		return err
	}
	st, err := profile.ReadActivity(ctx, c, act.ID)
	if err != nil {
		return err
	}
	changes, err := profile.PlanActivity(a, st, prune)
	if err != nil {
		return err
	}
	for _, ch := range changes {
		fmt.Fprintln(stdout, ch)
	}
	if len(changes) == 0 {
		fmt.Fprintln(stdout, "the hub already matches the profile")
		return nil
	}
	if dry {
		return nil
	}
	if err := c.Writable(); err != nil {
		return err
	}
	if !yes && !confirm(stdin, stdout, fmt.Sprintf("apply %d changes to activity %d?", len(changes), act.ID)) {
		return errors.New("not applied")
	}
	return profile.ApplyActivity(ctx, c, changes, func(ch profile.ActivityChange) {
		fmt.Fprintln(stdout, "done:", ch)
	})
}

// removeActivity deletes an activity from the hub.
func removeActivity(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("remove-activity", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	which := fs.String("activity", "", "activity name or id (required)")
	yes := fs.Bool("y", false, "delete without asking")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *which == "" {
		return errors.New("-activity is required")
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Writable(); err != nil {
		return err
	}
	acts, err := c.Activities(ctx)
	if err != nil {
		return err
	}
	act, err := pick("activity", *which, acts)
	if err != nil {
		return err
	}
	if !*yes && !confirm(stdin, stdout, fmt.Sprintf("delete activity %d %q, its keys and macros?", act.ID, act.Name)) {
		return errors.New("not deleted")
	}
	if err := c.DeleteActivity(ctx, act.ID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "deleted activity %d %q\n", act.ID, act.Name)
	return nil
}

// confirm asks a yes or no question.
func confirm(stdin io.Reader, stdout io.Writer, question string) bool {
	fmt.Fprintf(stdout, "%s [y/N] ", question)
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes"
}

// removeDevice deletes a device from the hub.
func removeDevice(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("remove-device", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	device := fs.String("device", "", "device name or id (required)")
	yes := fs.Bool("y", false, "delete without asking")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *device == "" {
		return errors.New("-device is required")
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Writable(); err != nil {
		return err
	}
	dev, err := findDevice(ctx, c, *device)
	if err != nil {
		return err
	}
	if !*yes && !confirm(stdin, stdout, fmt.Sprintf("delete device %d %q, its commands, and its place in activities?", dev.ID, dev.Name)) {
		return errors.New("not deleted")
	}
	if err := c.DeleteDevice(ctx, dev.ID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "deleted device %d %q\n", dev.ID, dev.Name)
	return nil
}

// syncCmd has the hub update the remotes' copy of a device or activity.
func syncCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	which := fs.String("device", "", "device or activity name or id (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *which == "" {
		return errors.New("-device is required")
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Writable(); err != nil {
		return err
	}
	e, err := findEntity(ctx, c, *which)
	if err != nil {
		return err
	}
	if err := c.Sync(ctx, e.ID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "synced %d %q to the remotes\n", e.ID, e.Name)
	return nil
}
