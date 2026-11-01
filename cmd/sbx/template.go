package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/templates"
)

// listTemplates prints the templates sbx carries.
func listTemplates(args []string, stdout io.Writer) error {
	if len(args) > 0 {
		return errors.New("templates takes no arguments")
	}
	for _, name := range templates.Names() {
		data, err := templates.Read(name)
		if err != nil {
			return err
		}
		d, err := profile.LoadTemplate(data)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		fmt.Fprintf(stdout, "%-28s %s, %d commands\n", name, templateModel(data), len(d.Commands))
	}
	return nil
}

// templateModel is a template's template key.
func templateModel(data []byte) string {
	for line := range strings.Lines(string(data)) {
		if v, ok := strings.CutPrefix(line, "template:"); ok {
			return strings.Trim(strings.TrimSpace(v), `'"`)
		}
	}
	return ""
}

// newProfile writes a device's profile made from a template, to edit and
// apply.
func newProfile(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	tmpl := fs.String("template", "", "a template of sbx templates, or a template file (required)")
	name := fs.String("name", "", "the device's name on the hub (default: the template's model)")
	out := fs.String("o", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tmpl == "" {
		return errors.New("-template is required (see sbx templates)")
	}
	data, err := templates.Read(*tmpl)
	if err != nil {
		if data, err = os.ReadFile(*tmpl); err != nil {
			return fmt.Errorf("no template %q (see sbx templates)", *tmpl)
		}
	}
	p, err := profile.FromTemplate(data, strings.TrimSuffix(*tmpl, ".yaml"), *name)
	if err != nil {
		return err
	}
	if *out == "" {
		_, err = stdout.Write(p)
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(p); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
