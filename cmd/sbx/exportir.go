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
	"github.com/brunoga/sofabaton/profile"
)

// exportIR writes a device profile's IR codes as a file of another tool
// (see irfile.Formats), saying which it left out and why.
func exportIR(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export-ir", flag.ContinueOnError)
	file := fs.String("f", "", "a device's profile, as export writes it (required)")
	format := fs.String("format", "", "flipper, irdb, smartir, lirc, girr, pronto, sendir, broadlink or uc (default: told by -o's extension)")
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
	d, err := profile.Load(f)
	if err != nil {
		return err
	}
	fm, err := exportFormat(*format, *out)
	if err != nil {
		return err
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
	skipped, err := fm.Write(w, d.Name, irfile.Signals(d))
	for _, s := range skipped {
		fmt.Fprintf(stderr, "left out %s: %s\n", s.Name, s.Why)
	}
	return err
}

// exportFormat is the format named, or else the one whose files have
// out's extension, if only one does.
func exportFormat(name, out string) (irfile.Format, error) {
	if name != "" {
		return irfile.FormatNamed(name)
	}
	ext := strings.ToLower(filepath.Ext(out))
	var found []irfile.Format
	for _, f := range irfile.Formats {
		if f.Ext == ext {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		return irfile.Format{}, errors.New("-format is required: -o's extension does not tell the format")
	}
	return found[0], nil
}
