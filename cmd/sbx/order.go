package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"

	"github.com/brunoga/sofabaton/record"
)

// order shows or sets the display order of the hub's devices or
// activities: the ones named go first, in the order given, and the
// others follow in theirs.
func order(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("order", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	acts := fs.Bool("activities", false, "order the activities instead of the devices")
	dry := fs.Bool("n", false, "show the new order without writing it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	what, list, sortEnts := "device", c.Devices, c.SortDevices
	if *acts {
		what, list, sortEnts = "activity", c.Activities, c.SortActivities
	}
	ents, err := list(ctx)
	if err != nil {
		return err
	}
	slices.SortStableFunc(ents, func(a, b *record.Entity) int { return int(a.Sort) - int(b.Sort) })
	var want []*record.Entity
	for _, which := range fs.Args() {
		e, err := pick(what, which, ents)
		if err != nil {
			return err
		}
		if slices.Contains(want, e) {
			return fmt.Errorf("%q is named twice", e.Name)
		}
		want = append(want, e)
	}
	for _, e := range ents {
		if !slices.Contains(want, e) {
			want = append(want, e)
		}
	}
	show := func() {
		for i, e := range want {
			fmt.Fprintf(stdout, "%2d. %s (%d)\n", i+1, e.Name, e.ID)
		}
	}
	if fs.NArg() == 0 || *dry || slices.Equal(want, ents) {
		show()
		return nil
	}
	if err := c.Writable(); err != nil {
		return err
	}
	if err := sortEnts(ctx, want); err != nil {
		return err
	}
	if err := c.SyncOrder(ctx); err != nil {
		return err
	}
	show()
	return nil
}
