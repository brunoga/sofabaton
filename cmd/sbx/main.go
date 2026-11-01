// Command sbx is a command-line client for Sofabaton hubs.
//
//	sbx discover          find hubs on the local network
//	sbx info              show a hub's identity
//	sbx list              list devices, activities, commands, sequences
//	sbx dump              dump a hub's configuration as JSON
//	sbx proxy             stand between the app and the hub, recording
//	sbx add-ir ...        add an IR command to a device
//	sbx remove-ir ...     delete a command of a device
//	sbx send ...          send a device's command now
//	sbx export ...        write a device's profile (YAML)
//	sbx apply ...         make a device match a profile, or create it
//	sbx remove-device ... delete a device
//	sbx remove-activity . delete an activity
//	sbx sync ...          update the remotes' copy of a device
//
// Commands that talk to a hub take -hub IP, defaulting to $SBX_HUB, else
// to the only hub discovery finds.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/transport"
	"github.com/brunoga/sofabaton/wire"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// dialHub opens a client; tests replace it with a simulated hub.
var dialHub = hub.Dial

// localAddrs returns this machine's addresses; tests replace it.
var localAddrs = func() map[netip.Addr]bool {
	m := map[netip.Addr]bool{}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			m[p.Addr().Unmap()] = true
		}
	}
	return m
}

// discoverHubs finds the hubs on the network; tests replace it.
var discoverHubs = func(ctx context.Context) ([]transport.Announcement, error) {
	var d transport.Discoverer
	return d.Discover(ctx)
}

// banner heads the help, with the version.
const banner = `sbx %s: local control of Sofabaton X1, X1S and X2 hubs, without the
app or the cloud. https://github.com/brunoga/sofabaton

`

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
             keys and power sequences (YAML: -device ID [-o file]); or
             an activity's: devices and inputs, keys, sequences (-activity)
  apply      make a device or activity match a profile, changing only
             what differs (-f file; -n shows the plan, -prune removes
             commands or sequences the profile lacks; -create makes a new
             IR device or activity)
  remove-device    delete a device (-device ID)
  remove-activity  delete an activity (-activity ID)
  sync       update the remotes' copy of a device or activity (-device ID);
             apply does it after changing anything
  version    print the version

Commands that talk to a hub take -hub IP. Without it they use $SBX_HUB
or, if that is unset, the hub discovery finds when only one answers.

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
	case "remove-activity":
		return removeActivity(ctx, args, stdin, stdout, stderr)
	case "remove-device":
		return removeDevice(ctx, args, stdin, stdout, stderr)
	case "proxy":
		return proxyCmd(ctx, args, stdin, stdout, stderr)
	case "version", "-version", "--version":
		fmt.Fprintln(stdout, version)
		return nil
	case "help", "-h", "-help", "--help":
		fmt.Fprintf(stdout, banner, version)
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
	fs.StringVar(&h.addr, "hub", os.Getenv(hubEnv), "hub IP address (default $"+hubEnv+", else the one hub discovery finds)")
	fs.BoolVar(&h.verbose, "v", false, "log every frame to stderr")
	fs.DurationVar(&h.timeout, "timeout", 20*time.Second, "time to wait for the hub to connect back")
}

func (h *hubFlags) connect(ctx context.Context, stderr io.Writer) (*hub.Client, error) {
	addr, err := hubAddr(ctx, h.addr, stderr)
	if err != nil {
		return nil, err
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

// hubEnv names the environment variable with the default hub address.
const hubEnv = "SBX_HUB"

// discoverWait is how long hubAddr listens for hubs.
const discoverWait = 3 * time.Second

// hubAddr parses a -hub flag or, when it is empty, discovers the hub: the
// only one that answers, or an error naming them all.
func hubAddr(ctx context.Context, flag string, stderr io.Writer) (netip.Addr, error) {
	if flag != "" {
		addr, err := netip.ParseAddr(flag)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("-hub: %w", err)
		}
		return addr, nil
	}
	ctx, cancel := context.WithTimeout(ctx, discoverWait)
	defer cancel()
	all, err := discoverHubs(ctx)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("finding a hub (or give -hub): %w", err)
	}
	// An sbx proxy on this machine answers discovery as the hub it holds;
	// connecting to it would get in the way of the app it serves.
	local := localAddrs()
	var found []transport.Announcement
	for _, a := range all {
		if !local[a.Addr] {
			found = append(found, a)
		}
	}
	switch len(found) {
	case 0:
		if len(all) > 0 {
			return netip.Addr{}, errors.New("only an sbx proxy on this machine answered discovery; stop it, or give -hub")
		}
		return netip.Addr{}, errors.New("no hub answered discovery; give -hub (a hub connected to the app does not answer)")
	case 1:
		a := found[0]
		fmt.Fprintf(stderr, "using hub %v (%s, %v)\n", a.Addr, a.Info.Name, a.Info.Variant)
		return a.Addr, nil
	}
	names := make([]string, len(found))
	for i, a := range found {
		names[i] = fmt.Sprintf("%v (%s)", a.Addr, a.Info.Name)
	}
	return netip.Addr{}, fmt.Errorf("%d hubs answered, give -hub: %s", len(found), strings.Join(names, ", "))
}
