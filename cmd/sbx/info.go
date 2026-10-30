package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

func info(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	i := c.Info()
	fmt.Fprintf(stdout, "name      %s\nmodel     %v\nfirmware  %d\nbatch     %s\nid        %x\nflags     % x\n",
		i.Name, i.Variant, i.Firmware, i.BatchDate(), i.ID, i.Flags)
	return nil
}
