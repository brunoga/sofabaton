package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// addIR adds an IR command, given as a descriptive code, to a device.
func addIR(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("add-ir", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	device := fs.String("device", "", "device name or id (required; see sbx list)")
	label := fs.String("label", "", "the command's label (required)")
	code := fs.String("code", "", `descriptive IR code (required), e.g. "P:NEC R:38400 D:0 S:206 F:217"`)
	dry := fs.Bool("n", false, "show the write without sending it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *device == "" || *label == "" || *code == "" {
		return errors.New("-device, -label and -code are required")
	}
	data, err := record.Descriptor(*code)
	if err != nil {
		return err
	}

	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Writable(); err != nil {
		return err
	}
	d, err := findDevice(ctx, c, *device)
	if err != nil {
		return err
	}
	dev := d.ID
	cmds, err := c.Commands(ctx, dev)
	if err != nil {
		return err
	}
	for _, x := range cmds {
		if strings.EqualFold(x.Label, *label) {
			return fmt.Errorf("device %d already has a command %q (id %d)", dev, x.Label, x.ID)
		}
	}
	id, err := hub.NextCommandID(cmds)
	if err != nil {
		return err
	}
	cmd, err := record.NewCommand(c.Variant())
	if err != nil {
		return err
	}
	cmd.Device, cmd.ID, cmd.Type, cmd.Label = dev, id, record.ClassIR, *label

	if *dry {
		pages, err := cmd.WritePages(1, 1, data)
		if err != nil {
			return err
		}
		for _, p := range pages {
			fmt.Fprintf(stdout, "%v % x\n", wire.FamWriteCommand.Name(wire.ToHub), p)
		}
		return nil
	}
	if err := c.WriteCommand(ctx, cmd, data); err != nil {
		return err
	}
	after, err := c.Commands(ctx, dev)
	if err != nil {
		return fmt.Errorf("written, but reading the commands back: %w", err)
	}
	for _, x := range after {
		if x.ID == id {
			fmt.Fprintf(stdout, "added %q to device %d: id %d, code %x, sort %d\n", x.Label, dev, x.ID, x.Code, x.Sort)
			return nil
		}
	}
	return fmt.Errorf("written and acknowledged, but the hub does not list command %d", id)
}

// removeIR deletes a command of a device, by label or id.
func removeIR(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("remove-ir", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	device := fs.String("device", "", "device name or id (required; see sbx list)")
	label := fs.String("label", "", "the command's label")
	id := fs.Uint("id", 0, "the command's id, instead of -label")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *device == "" || (*label == "") == (*id == 0) {
		return errors.New("-device and one of -label or -id are required")
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Writable(); err != nil {
		return err
	}
	d, err := findDevice(ctx, c, *device)
	if err != nil {
		return err
	}
	dev := d.ID
	cmds, err := c.Commands(ctx, dev)
	if err != nil {
		return err
	}
	var target *record.Command
	for _, x := range cmds {
		if (*id != 0 && uint(x.ID) == *id) || (*label != "" && strings.EqualFold(x.Label, *label)) {
			if target != nil {
				return fmt.Errorf("device %d has more than one command %q; use -id", dev, *label)
			}
			target = x
		}
	}
	if target == nil {
		return fmt.Errorf("device %d has no such command", dev)
	}
	if err := c.DeleteCommand(ctx, dev, target.ID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "removed %q (id %d) from device %d\n", target.Label, target.ID, dev)
	return nil
}

// send makes the hub send one of a device's commands now.
func send(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	device := fs.String("device", "", "device name or id (required; see sbx list)")
	label := fs.String("command", "", "the command's label")
	id := fs.Uint("id", 0, "the command's id, instead of -command")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *device == "" || (*label == "") == (*id == 0) {
		return errors.New("-device and one of -command or -id are required")
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Writable(); err != nil {
		return err
	}
	d, err := findDevice(ctx, c, *device)
	if err != nil {
		return err
	}
	dev := d.ID
	cmds, err := c.Commands(ctx, dev)
	if err != nil {
		return err
	}
	var target *record.Command
	for _, x := range cmds {
		if (*id != 0 && uint(x.ID) == *id) || (*label != "" && strings.EqualFold(x.Label, *label)) {
			target = x
			break
		}
	}
	if target == nil {
		return fmt.Errorf("device %d has no such command", dev)
	}
	if err := c.Run(ctx, dev, []record.Step{hub.StepFor(target)}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "sent %q (id %d) of device %d\n", target.Label, target.ID, dev)
	return nil
}
