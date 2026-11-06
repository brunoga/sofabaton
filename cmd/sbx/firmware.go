package main

import (
	"context"
	"crypto/md5" //nolint:gosec // G501: the hub checks firmware images by MD5
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/brunoga/sofabaton/irsource"
	"github.com/brunoga/sofabaton/record"
)

// keyEnv names the environment variable with the key of Sofabaton's
// server, which the app carries.
const keyEnv = "SBX_SOFABATON_KEY"

// The server's answers, which the tests replace.
var (
	hubFirmware = func(ctx context.Context, key string, info *record.HubInfo) (*irsource.Firmware, error) {
		return irsource.SofabatonDB{Key: key}.HubFirmware(ctx, info)
	}
	hubImage = func(ctx context.Context, key string, f *irsource.Firmware) ([]byte, error) {
		return irsource.SofabatonDB{Key: key}.HubImage(ctx, f)
	}
	// rebootWait is how long to wait for the hub to come back after an
	// update; rebootPoll how often to look for it.
	rebootWait = 3 * time.Minute
	rebootPoll = 5 * time.Second
)

// firmwareCmd says whether the vendor's server has a newer firmware for
// the hub, and installs it with -update, as the app does.
func firmwareCmd(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("firmware", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	key := fs.String("key", os.Getenv(keyEnv), "the key of Sofabaton's server (default $"+keyEnv+")")
	update := fs.Bool("update", false, "install the newer firmware, if there is one")
	yes := fs.Bool("y", false, "install without asking")
	remote := fs.Bool("remote", false, "the remote's firmware, not the hub's: updated over this computer's Bluetooth (Linux)")
	name := fs.String("name", "", "with -remote, the remote's name or Bluetooth address (default: the only one)")
	reinstall := fs.Bool("reinstall", false, "with -remote, send the remote the firmware it has again")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *key == "" {
		return errors.New("-key (or $" + keyEnv + ") is required: the key of Sofabaton's server")
	}
	if *remote {
		return remoteFirmware(ctx, &h, *key, *name, *update, *reinstall, *yes, stdin, stdout, stderr)
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	info := c.Info()
	fmt.Fprintf(stdout, "hub %s (%v): firmware %d\n", info.Name, info.Variant, info.Firmware)
	f, err := hubFirmware(ctx, *key, info)
	if err != nil {
		_ = c.Close()
		return err
	}
	if f == nil {
		_ = c.Close()
		fmt.Fprintln(stdout, "it is the newest firmware Sofabaton's server has")
		return nil
	}
	fmt.Fprintf(stdout, "firmware %d is available: %s\n", f.Version, f.Notes)
	if !*update {
		_ = c.Close()
		fmt.Fprintln(stdout, "install it with -update")
		return nil
	}
	image, err := hubImage(ctx, *key, f)
	if err != nil {
		_ = c.Close()
		return err
	}
	fmt.Fprintf(stdout, "downloaded %d bytes, MD5 %x\n", len(image), md5.Sum(image)) //nolint:gosec // G401: the hub's check, not a security one
	if !*yes && !confirm(stdin, stdout, fmt.Sprintf("Install firmware %d on %s? Keep it powered and on the network until it is back (a few minutes).", f.Version, info.Name)) {
		_ = c.Close()
		return errors.New("not installed")
	}
	last := -1
	err = c.UpdateFirmware(ctx, image, func(done, total int) {
		if pct := done * 100 / total; pct/10 != last/10 {
			last = pct
			fmt.Fprintf(stdout, "sent %d%%\n", pct)
		}
	})
	_ = c.Close() // the hub reboots without closing it
	if err != nil {
		return fmt.Errorf("the update stopped: %w (the hub keeps its firmware until an image is in whole; try again)", err)
	}
	fmt.Fprintln(stdout, "the hub took the image and is restarting")
	back, err := waitForHub(ctx, &h, info, f.Version, stderr)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "the hub is back with firmware %d\n", back.Firmware)
	return nil
}

// waitForHub waits for the hub to answer again after an update, and
// says what firmware it has; it is an error if it is not version.
func waitForHub(ctx context.Context, h *hubFlags, was *record.HubInfo, version int, stderr io.Writer) (*record.HubInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, rebootWait)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("the hub did not come back in time: check it in a few minutes with sbx firmware")
		case <-time.After(rebootPoll):
		}
		c, err := h.connect(ctx, io.Discard)
		if err != nil {
			continue
		}
		info := *c.Info()
		_ = c.Close()
		if info.ID != was.ID {
			return nil, fmt.Errorf("another hub answered (%s)", info.Name)
		}
		if int(info.Firmware) != version {
			return &info, fmt.Errorf("the hub is back with firmware %d, not %d", info.Firmware, version)
		}
		return &info, nil
	}
}
