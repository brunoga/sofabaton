package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
)

// findDevice resolves a device given by id or by name (regardless of
// case).
func findDevice(ctx context.Context, c *hub.Client, which string) (*record.Entity, error) {
	devs, err := c.Devices(ctx)
	if err != nil {
		return nil, err
	}
	return pick("device", which, devs)
}

// findEntity resolves a device or an activity, by id or name.
func findEntity(ctx context.Context, c *hub.Client, which string) (*record.Entity, error) {
	devs, err := c.Devices(ctx)
	if err != nil {
		return nil, err
	}
	acts, err := c.Activities(ctx)
	if err != nil {
		return nil, err
	}
	return pick("device or activity", which, append(devs, acts...))
}

func pick(what, which string, entities []*record.Entity) (*record.Entity, error) {
	if n, err := strconv.ParseUint(which, 0, 8); err == nil {
		for _, e := range entities {
			if e.ID == byte(n) {
				return e, nil
			}
		}
		return nil, fmt.Errorf("no %s with id %d", what, n)
	}
	var found []*record.Entity
	for _, e := range entities {
		if strings.EqualFold(e.Name, which) {
			found = append(found, e)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		var names []string
		for _, e := range entities {
			names = append(names, fmt.Sprintf("%q", e.Name))
		}
		return nil, fmt.Errorf("no %s named %q (there are %s)", what, which, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("%d entities are named %q; use the id", len(found), which)
}

// parseInterspersed parses flags wherever they are among the arguments
// ("list commands TV -hub IP" as well as "list -hub IP commands TV") and
// returns the others in order.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// list shows what the hub holds.
func list(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: sbx list [-hub IP] [what]

  devices              the devices (the default, with activities)
  activities           the activities and their devices
  commands DEVICE      a device's commands
  buttons ENTITY       what the keys of an activity or device run
  sequences [ENTITY]   the sequences (macros) of an activity or device, or of all

DEVICE and ENTITY are names or ids.
`)
		fs.PrintDefaults()
	}
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	defer func() { _ = tw.Flush() }()

	what, rest := "", positional
	if len(rest) > 0 {
		what, rest = rest[0], rest[1:]
	}
	switch what {
	case "":
		if err := listDevices(ctx, c, tw); err != nil {
			return err
		}
		fmt.Fprintln(tw)
		return listActivities(ctx, c, tw)
	case "devices":
		return listDevices(ctx, c, tw)
	case "activities":
		return listActivities(ctx, c, tw)
	case "commands":
		if len(rest) != 1 {
			return errors.New("list commands takes one device")
		}
		d, err := findDevice(ctx, c, rest[0])
		if err != nil {
			return err
		}
		cmds, err := c.Commands(ctx, d.ID)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "ID\tCOMMAND\tTYPE\n")
		for _, x := range cmds {
			fmt.Fprintf(tw, "%d\t%s\t%v\n", x.ID, x.Label, x.Type)
		}
		return nil
	case "buttons":
		if len(rest) != 1 {
			return errors.New("list buttons takes one device or activity")
		}
		e, err := findEntity(ctx, c, rest[0])
		if err != nil {
			return err
		}
		return listButtons(ctx, c, tw, e)
	case "sequences":
		var entities []*record.Entity
		switch len(rest) {
		case 0:
			devs, err := c.Devices(ctx)
			if err != nil {
				return err
			}
			acts, err := c.Activities(ctx)
			if err != nil {
				return err
			}
			entities = append(acts, devs...)
		case 1:
			e, err := findEntity(ctx, c, rest[0])
			if err != nil {
				return err
			}
			entities = []*record.Entity{e}
		default:
			return errors.New("list sequences takes at most one device or activity")
		}
		return listSequences(ctx, c, tw, entities)
	}
	return fmt.Errorf("list: unknown %q (devices, activities, commands, buttons, sequences)", what)
}

func listDevices(ctx context.Context, c *hub.Client, w io.Writer) error {
	devs, err := c.Devices(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "ID\tDEVICE\tBRAND\tTYPE\n")
	for _, d := range devs {
		fmt.Fprintf(w, "%d\t%s\t%s\t%v\n", d.ID, d.Name, d.Brand, d.Class)
	}
	return nil
}

func listActivities(ctx context.Context, c *hub.Client, w io.Writer) error {
	acts, err := c.Activities(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "ID\tACTIVITY\tDEVICES\n")
	for _, a := range acts {
		members, err := c.Members(ctx, a.ID)
		if err != nil && !errors.Is(err, hub.ErrNotFound) {
			return err
		}
		var names []string
		for _, m := range members {
			names = append(names, m.Name)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\n", a.ID, a.Name, strings.Join(names, ", "))
	}
	return nil
}

// macroInput is the key activity sequences use after powering a device on;
// presumably the input the activity selects for it (not yet confirmed).
const macroInput = 0xC5

// commandNamer names devices' commands as "device: label", reading each
// device's commands once.
func commandNamer(ctx context.Context, c *hub.Client) (name func(device, id byte) string, devices map[byte]string, err error) {
	labels := map[byte]map[byte]string{} // device -> command id -> label
	devices = map[byte]string{}
	devs, err := c.Devices(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, d := range devs {
		devices[d.ID] = d.Name
	}
	return func(device, id byte) string {
		if labels[device] == nil {
			labels[device] = map[byte]string{}
			if cmds, err := c.Commands(ctx, device); err == nil {
				for _, x := range cmds {
					labels[device][x.ID] = x.Label
				}
			}
		}
		label, ok := labels[device][id]
		if !ok {
			label = fmt.Sprintf("command %d", id)
		}
		return devices[device] + ": " + label
	}, devices, nil
}

func listButtons(ctx context.Context, c *hub.Client, w io.Writer, e *record.Entity) error {
	rows, err := c.Keymap(ctx, e.ID)
	if errors.Is(err, hub.ErrNotFound) {
		fmt.Fprintf(w, "%s has no buttons\n", e.Name)
		return nil
	}
	if err != nil {
		return err
	}
	name, _, err := commandNamer(ctx, c)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "KEY\tPRESS\tLONG PRESS\n")
	for _, b := range rows {
		long := ""
		if !b.Long.IsZero() {
			long = name(b.Long.Device, b.Long.Command)
		}
		fmt.Fprintf(w, "%v\t%s\t%s\n", b.Button, name(b.Short.Device, b.Short.Command), long)
	}
	return nil
}

func listSequences(ctx context.Context, c *hub.Client, w io.Writer, entities []*record.Entity) error {
	command, names, err := commandNamer(ctx, c)
	if err != nil {
		return err
	}
	step := func(s record.Step) string {
		if s.IsDelay() {
			return fmt.Sprintf("wait %d", s.Delay)
		}
		// Activity steps call the device's own sequences by their keys.
		switch s.Command {
		case record.MacroPowerOn:
			return names[s.Device] + ": its POWER_ON sequence"
		case record.MacroPowerOff:
			return names[s.Device] + ": its POWER_OFF sequence"
		case macroInput:
			return names[s.Device] + ": its input for the activity"
		}
		return command(s.Device, s.Command)
	}
	fmt.Fprintf(w, "OF\tKEY\tSEQUENCE\tSTEPS\n")
	for _, e := range entities {
		macros, err := c.Macros(ctx, e.ID)
		if errors.Is(err, hub.ErrNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name, err)
		}
		for _, m := range macros {
			var steps []string
			for _, s := range m.Steps {
				steps = append(steps, step(s))
			}
			fmt.Fprintf(w, "%s\t%#02x\t%s\t%s\n", e.Name, m.ID, m.Label, strings.Join(steps, ", "))
		}
	}
	return nil
}
