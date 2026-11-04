package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/brunoga/sofabaton/irfile"
)

// importIR writes the profile of a device made of a file of IR codes
// (see irfile.Read for the formats), saying which codes it left out and
// why.
func importIR(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("import-ir", flag.ContinueOnError)
	file := fs.String("f", "", "a file of IR codes: Flipper .ir, irdb .csv, SmartIR .json, LIRC .conf, Girr, or codes one a line (required)")
	name := fs.String("name", "", "the device's name (default: the file's name)")
	out := fs.String("o", "", "write to this file instead of stdout")
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
	defer func() { _ = f.Close() }()
	sigs, err := irfile.Read(*file, f)
	if err != nil {
		return err
	}
	if *name == "" {
		*name = strings.ReplaceAll(strings.TrimSuffix(filepath.Base(*file), filepath.Ext(*file)), "_", " ")
	}
	d, skipped := irfile.Device(*name, sigs)
	for _, s := range skipped {
		fmt.Fprintf(stderr, "left out %s: %s\n", s.Name, s.Why)
	}
	if len(d.Commands) == 0 {
		return errors.New("none of the file's codes can be sent by the X2 yet")
	}
	w := stdout
	if *out != "" {
		of, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			return err
		}
		defer func() { _ = of.Close() }()
		w = of
	}
	return d.Encode(w)
}
