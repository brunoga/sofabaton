package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"rsc.io/qr"

	"github.com/brunoga/sofabaton/hub"
)

// matterCmd shows the X2's Matter setup code, as a QR code for a Home
// app's camera and as the code to type, and the switches the hub offers
// once a Home app has added it.
func matterCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("matter", flag.ContinueOnError)
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
	dev, err := c.MatterDevice(ctx)
	switch {
	case errors.Is(err, hub.ErrNotFound):
		fmt.Fprintln(stdout, "no Home app has added the hub yet")
	case err != nil:
		return err
	default:
		fmt.Fprintf(stdout, "Matter switches: device %d %q\n", dev.ID, dev.Name)
		if cmds, err := c.Commands(ctx, dev.ID); err == nil {
			for _, cmd := range cmds {
				fmt.Fprintf(stdout, "  %3d %s\n", cmd.ID, cmd.Label)
			}
		}
	}
	code, err := c.MatterCode(ctx)
	if err != nil {
		return err
	}
	q, err := qr.Encode(code.QR, qr.M)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "\nto add the hub, scan this with your Home app, or type the code under it:")
	fmt.Fprint(stdout, halfBlocks(q))
	fmt.Fprintf(stdout, "%s\n%s\n", code.Manual, code.QR)
	return nil
}

// halfBlocks draws q in a terminal, two rows of the code to a line, dark
// on a light quiet zone (so it scans on dark terminals too).
func halfBlocks(q *qr.Code) string {
	const quiet = 2
	dark := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		return x >= 0 && y >= 0 && x < q.Size && y < q.Size && q.Black(x, y)
	}
	n := q.Size + 2*quiet
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		for x := range n {
			top, bottom := dark(x, y), dark(x, y+1)
			switch {
			case top && bottom:
				b.WriteRune(' ')
			case top:
				b.WriteRune('▄')
			case bottom:
				b.WriteRune('▀')
			default:
				b.WriteRune('█')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
