package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/brunoga/sofabaton/irsource"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/sifli"
)

// The server's and Bluetooth's parts of a remote update, which the tests
// replace.
var (
	remoteFirmwares = func(ctx context.Context, key string) ([]*irsource.Firmware, error) {
		return irsource.SofabatonDB{Key: key}.RemoteFirmwares(ctx)
	}
	remotePackage = func(ctx context.Context, key string, f *irsource.Firmware) (sifli.Package, error) {
		return irsource.SofabatonDB{Key: key}.RemotePackage(ctx, f)
	}
	remoteDial = bleDial
)

// remoteUpdateTimeout bounds a remote's update: our remote's took a little
// over three minutes.
const remoteUpdateTimeout = 15 * time.Minute

// remoteFirmware is sbx firmware -remote: the hub's remotes and their
// firmware against the server's, and with -update (or -reinstall, for
// the version the remote has) an update over this computer's Bluetooth,
// as the app does it from the phone.
func remoteFirmware(ctx context.Context, h *hubFlags, key, which string, update, reinstall, yes bool, stdin io.Reader, stdout, stderr io.Writer) error {
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	remotes, err := c.Remotes(ctx)
	_ = c.Close() // the update goes to the remote, not through the hub
	if err != nil {
		return err
	}
	var r *record.RemoteInfo
	for _, x := range remotes {
		fmt.Fprintf(stdout, "remote %s (%v): firmware %d\n", x.Name, x.Addr, x.Firmware)
		if which == "" || strings.EqualFold(which, x.Name) || strings.EqualFold(which, x.Addr.String()) {
			if r != nil && which == "" {
				return errors.New("the hub has several remotes: pick one with -name")
			}
			r = x
		}
	}
	if r == nil {
		return fmt.Errorf("the hub has no remote %q", which)
	}
	all, err := remoteFirmwares(ctx, key)
	if err != nil {
		return err
	}
	var f *irsource.Firmware
	for _, x := range all {
		switch {
		case reinstall && x.Version == int(r.Firmware):
			f = x
		case !reinstall && x.Version > int(r.Firmware) && (f == nil || x.Version > f.Version):
			f = x
		}
	}
	switch {
	case f == nil && reinstall:
		return fmt.Errorf("the server has no firmware %d for the remote to reinstall", r.Firmware)
	case f == nil:
		fmt.Fprintln(stdout, "it is the newest firmware Sofabaton's server has")
		return nil
	case !reinstall:
		fmt.Fprintf(stdout, "firmware %d is available: %s\n", f.Version, f.Notes)
	}
	if !update && !reinstall {
		fmt.Fprintln(stdout, "install it with -update")
		return nil
	}
	p, err := remotePackage(ctx, key, f)
	if err != nil {
		return err
	}
	size := len(p.Ctrl)
	for _, img := range p.Images {
		size += len(img)
	}
	fmt.Fprintf(stdout, "downloaded firmware %d: %d files, %d bytes\n", f.Version, len(p.Images)+1, size)
	if !yes && !confirm(stdin, stdout, fmt.Sprintf("Send firmware %d to %s over this computer's Bluetooth? Keep the remote near the computer and awake (press a key) for the next few minutes.", f.Version, r.Name)) {
		return errors.New("not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, remoteUpdateTimeout)
	defer cancel()
	step, last := "", -1
	err = sifli.Update(ctx, remoteDial(net.HardwareAddr(r.Addr)), p, func(pr sifli.Progress) {
		if pr.Step != step {
			step = pr.Step
			fmt.Fprintln(stdout, step)
		}
		if pr.Total > 0 {
			if pct := pr.Done * 100 / pr.Total; pct/5 != last/5 {
				last = pct
				fmt.Fprintf(stdout, "sent %d%%\n", pct)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("the update stopped: %w (the remote keeps its firmware, or waits in its updater for another try: run this again)", err)
	}
	fmt.Fprintf(stdout, "%s took firmware %d and is installing it: it restarts in about a minute\n", r.Name, f.Version)
	return nil
}
