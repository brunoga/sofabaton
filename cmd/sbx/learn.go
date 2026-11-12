package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// learnCmd has the hub learn an IR code from another remote and adds it
// to a device as a command, bound to a key if asked.
func learnCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("learn", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	which := fs.String("device", "", "device name or id (required; see sbx list)")
	name := fs.String("name", "", "the new command's name (required)")
	key := fs.String("key", "", "a key of the remote to bind it to, e.g. Up")
	wait := fs.Duration("wait", time.Minute, "how long to wait for the code")
	rf := fs.Int("rf", 0, "learn an RF code at 433 or 315 MHz instead of IR")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *which == "" || *name == "" {
		return errors.New("-device and -name are required")
	}
	if *rf != 0 {
		if _, err := hub.RFClass(*rf); err != nil {
			return fmt.Errorf("-rf: %w", err)
		}
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Writable(); err != nil {
		return err
	}
	dev, err := findDevice(ctx, c, *which)
	if err != nil {
		return err
	}
	if dev.Class.IsWiFi() {
		return fmt.Errorf("%q is a network device: its commands are not IR or RF", dev.Name)
	}
	st, err := profile.Read(ctx, c, dev.ID)
	if err != nil {
		return err
	}
	p, err := profile.FromHub(dev, st)
	if err != nil {
		return err
	}
	for _, x := range p.Commands {
		if strings.EqualFold(x.Name, *name) {
			return fmt.Errorf("%q already has a command %q", dev.Name, x.Name)
		}
	}
	p.Commands = append(p.Commands, profile.Command{Name: *name})
	if *key != "" {
		if p.Buttons == nil {
			p.Buttons = profile.Buttons{}
		}
		p.Buttons[*key] = profile.Key{Short: *name}
	}

	lctx, cancel := context.WithTimeout(ctx, *wait)
	defer cancel()
	failed := func() {
		fmt.Fprintln(stdout, "the hub could not read that: press the key once again, briefly")
	}
	var data []byte
	if *rf != 0 {
		fmt.Fprintf(stdout, "hold the other remote near the hub and press the key once (RF at %d MHz; waiting %v)\n", *rf, *wait)
		data, err = c.LearnRF(lctx, *rf, failed)
	} else {
		fmt.Fprintf(stdout, "point the other remote at the hub and press the key once; don't hold it (waiting %v)\n", *wait)
		data, err = c.Learn(lctx, failed)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("the hub learned no code: %w", err)
		}
		return err
	}
	if raw, err := record.ParseRawIR(data); err == nil && *rf == 0 {
		fmt.Fprintf(stdout, "learned: %d timings at %d Hz\n", len(raw.Timings), raw.Carrier)
	} else {
		fmt.Fprintf(stdout, "learned: %d bytes\n", len(data))
	}
	p.Commands[len(p.Commands)-1].Data = hex.EncodeToString(data)
	p.Commands[len(p.Commands)-1].RF = *rf

	changes, _, err := profile.Plan(p, st, false)
	if err != nil {
		return err
	}
	return profile.Apply(ctx, c, changes, func(ch profile.Change) {
		fmt.Fprintln(stdout, "done:", ch)
	})
}
