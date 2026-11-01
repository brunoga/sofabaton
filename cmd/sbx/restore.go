package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/brunoga/sofabaton/dump"
	"github.com/brunoga/sofabaton/profile"
)

// restore writes a dump back to a hub: the plan first, then, once
// confirmed, the writes.
func restore(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	file := fs.String("f", "", "the dump (required)")
	prune := fs.Bool("prune", false, "also remove what the dump's devices and activities lack (commands, macros, keys, favorites)")
	dry := fs.Bool("n", false, "show the plan only")
	yes := fs.Bool("y", false, "restore without asking")
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
	d, err := dump.Load(f)
	_ = f.Close()
	if err != nil {
		return err
	}
	src, err := d.Open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if src.Variant() != c.Variant() {
		return fmt.Errorf("the dump is of an %v, the hub an %v", src.Variant(), c.Variant())
	}
	say := func(s string) { fmt.Fprintln(stdout, s) }
	fmt.Fprintf(stdout, "restoring %q (%s, dumped by %s)\n", d.Info.Name, d.Info.Model, d.Tool)
	if err := profile.Restore(ctx, src, c, *prune, true, say); err != nil {
		return err
	}
	if *dry {
		return nil
	}
	if !*yes && !confirm(stdin, stdout, "restore?") {
		return errors.New("not restored")
	}
	return profile.Restore(ctx, src, c, *prune, false, func(s string) { say("done: " + s) })
}
