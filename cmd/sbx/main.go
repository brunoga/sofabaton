// Command sbx is a command-line client for Sofabaton hubs.
//
//	sbx discover            find hubs on the local network
//	sbx info -hub IP        show a hub's identity
//	sbx list -hub IP        list devices, activities, commands, sequences
//	sbx dump -hub IP        dump a hub's configuration as JSON
//	sbx proxy -hub IP       stand between the app and the hub, recording
//	sbx add-ir -hub IP ...  add an IR command to a device
//	sbx remove-ir -hub IP . delete a command of a device
//	sbx send -hub IP ...    send a device's command now
//	sbx export -hub IP ...  write a device's profile (YAML)
//	sbx apply -hub IP ...   make a device match a profile, or create it
//	sbx remove-device ...   delete a device
//	sbx sync -hub IP ...    update the remotes' copy of a device
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/signal"
	"time"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// dialHub opens a client; tests replace it with a simulated hub.
var dialHub = hub.Dial

const usage = `usage: sbx <command> [flags]

commands:
  discover   find hubs on the local network
  info       show a hub's identity
  list       list devices and activities, a device's commands
             (list commands NAME), what keys run (list buttons NAME) or
             sequences (list sequences [NAME])
  dump       dump a hub's configuration as JSON (a lossless backup: raw
             record bytes are kept next to the decoded view)
  proxy      stand between the Sofabaton app and the hub, relaying and
             recording every frame (-capture file.jsonl)
  add-ir     add an IR command to a device, from a descriptive code
             (-device ID -label NAME -code "P:NEC ..."; -n shows the write)
  remove-ir  delete a command of a device (-device ID, -label NAME or -id N)
  send       send a device's command now (-device ID, -command NAME or -id N)
  export     write a device's profile: commands in the remote's order,
             keys and power sequences (YAML: -device ID [-o file])
  apply      make a device match a profile, changing only what differs
             (-f file; -n shows the plan, -prune removes commands the
             profile lacks; -create makes it a new IR device)
  remove-device  delete a device (-device ID)
  sync       update the remotes' copy of a device or activity (-device ID);
             apply does it after changing anything
  version    print the version

Close the Sofabaton app first: a hub serves one client at a time and does
not answer discovery while the app is connected.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Stdin, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "sbx:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, stdin io.Reader, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return errors.New("no command")
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case "discover":
		return discover(ctx, args, stdout)
	case "info":
		return info(ctx, args, stdout, stderr)
	case "list":
		return list(ctx, args, stdout, stderr)
	case "dump":
		return dump(ctx, args, stdout, stderr)
	case "add-ir":
		return addIR(ctx, args, stdout, stderr)
	case "remove-ir":
		return removeIR(ctx, args, stdout, stderr)
	case "send":
		return send(ctx, args, stdout, stderr)
	case "export":
		return export(ctx, args, stdout, stderr)
	case "apply":
		return apply(ctx, args, stdin, stdout, stderr)
	case "sync":
		return syncCmd(ctx, args, stdout, stderr)
	case "remove-device":
		return removeDevice(ctx, args, stdin, stdout, stderr)
	case "proxy":
		return proxyCmd(ctx, args, stdin, stdout, stderr)
	case "version", "-version", "--version":
		fmt.Fprintln(stdout, version)
		return nil
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	}
	fmt.Fprint(stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

// hubFlags are the flags of the commands that connect to a hub.
type hubFlags struct {
	addr    string
	verbose bool
	timeout time.Duration
}

func (h *hubFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&h.addr, "hub", "", "hub IP address (required)")
	fs.BoolVar(&h.verbose, "v", false, "log every frame to stderr")
	fs.DurationVar(&h.timeout, "timeout", 20*time.Second, "time to wait for the hub to connect back")
}

func (h *hubFlags) connect(ctx context.Context, stderr io.Writer) (*hub.Client, error) {
	if h.addr == "" {
		return nil, errors.New("-hub is required")
	}
	addr, err := netip.ParseAddr(h.addr)
	if err != nil {
		return nil, fmt.Errorf("-hub: %w", err)
	}
	var opts hub.Options
	if h.verbose {
		opts.Session = append(opts.Session, session.WithTap(func(d wire.Direction, f wire.Frame) {
			fmt.Fprintf(stderr, "%s %v %-18s % x\n", d, f.Opcode(), f.Family.Name(d), f.Payload)
		}))
	}
	dctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	return dialHub(dctx, addr, opts)
}
