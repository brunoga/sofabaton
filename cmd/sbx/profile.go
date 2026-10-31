package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/profile"
)

// export writes a device's profile from the hub.
func export(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	device := fs.String("device", "", "device name or id (required; see sbx list)")
	out := fs.String("o", "", "write to this file instead of stdout")
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
	{
		st, err := profile.Read(ctx, c, dev.ID)
		if err != nil {
			return err
		}
		p, err := profile.FromHub(dev, st)
		if err != nil {
			return err
		}
		w := stdout
		if *out != "" {
			f, err := os.Create(*out)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			w = f
		}
		fmt.Fprintf(w, "# %s (%s), exported from %s\n", p.Name, p.Brand, c.Info().Name)
		return p.Encode(w)
	}
}

// apply makes a device on the hub match a profile.
func apply(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	file := fs.String("f", "", "the profile (required)")
	device := fs.String("device", "", "device name or id (default: the profile's id, else its device name)")
	prune := fs.Bool("prune", false, "also remove the hub's commands the profile lacks")
	dry := fs.Bool("n", false, "show the plan only")
	yes := fs.Bool("y", false, "apply without asking")
	create := fs.Bool("create", false, "create the profile's device as a new IR device")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("-f is required")
	}
	f, err := os.Open(*file)
	if err != nil {
		return err
	}
	p, err := profile.Load(f)
	_ = f.Close()
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
