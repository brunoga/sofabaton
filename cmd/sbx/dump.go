package main

import (
	"context"
	"flag"
	"io"
	"os"

	"github.com/brunoga/sofabaton/dump"
)

func dumpHub(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dump", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	out := fs.String("o", "", "write the dump to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	d := dump.Read(ctx, c, "sbx "+version, stderr)
	w := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		w = f
	}
	return d.Write(w)
}
