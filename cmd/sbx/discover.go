package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/brunoga/sofabaton/transport"
)

func discover(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	wait := fs.Duration("wait", 3*time.Second, "how long to listen for replies")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, *wait)
	defer cancel()
	var d transport.Discoverer
	found, err := d.Discover(ctx)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		fmt.Fprintln(stdout, "no hubs answered (a hub connected to the app does not)")
		return nil
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ADDRESS\tMODEL\tFIRMWARE\tNAME\tID")
	for _, a := range found {
		fmt.Fprintf(tw, "%v\t%v\t%d\t%s\t%x\n", a.Addr, a.Info.Variant, a.Info.Firmware, a.Info.Name, a.Info.ID)
	}
	return tw.Flush()
}
