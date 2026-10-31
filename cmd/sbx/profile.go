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
	dev, err := findDevice(ctx, c, *device)
	if err != nil {
		return err
	}
	{
		st, err := profile.Read(ctx, c, dev.ID)
		if err != nil {
			return err
		}
		p, err := profile.FromHub(dev, st.Commands, st.Data)
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
	if !*yes {
		fmt.Fprintf(stdout, "apply %d changes to device %d? [y/N] ", len(changes), id)
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			return errors.New("not applied")
		}
	}
	return profile.Apply(ctx, c, changes, func(ch profile.Change) {
		fmt.Fprintln(stdout, "done:", ch)
	})
}
