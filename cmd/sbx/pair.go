package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
)

// pairTimeout is how long pairing waits for the device.
var pairTimeout = 2 * time.Minute

// pairCmd has the hub pair with a Bluetooth device again.
func pairCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("pair", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	which := fs.String("device", "", "Bluetooth device name or id (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *which == "" {
		return errors.New("-device is required")
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Writable(); err != nil {
		return err
	}
	e, err := findDevice(ctx, c, *which)
	if err != nil {
		return err
	}
	if e.Class != record.ClassBluetooth {
		return fmt.Errorf("%q is not a Bluetooth device", e.Name)
	}
	return pair(ctx, c, e.ID, e.Name, stdout)
}

// pair has the hub pair with the Bluetooth device, saying what to do.
func pair(ctx context.Context, c *hub.Client, id byte, name string, stdout io.Writer) error {
	fmt.Fprintf(stdout, "pairing %q: pair the device with the hub now, from the device's Bluetooth settings or its remote pairing (waiting %v)\n", name, pairTimeout)
	ctx, cancel := context.WithTimeout(ctx, pairTimeout)
	defer cancel()
	if err := c.Pair(ctx, id); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%q did not pair; try again with sbx pair -device %d", name, id)
		}
		return err
	}
	fmt.Fprintf(stdout, "paired %q\n", name)
	return nil
}
