// Command sbx is a command-line client for Sofabaton hubs.
//
//	sbx discover            find hubs on the local network
//	sbx info -hub IP        show a hub's identity
//	sbx dump -hub IP        dump a hub's configuration as JSON
//	sbx proxy -hub IP       stand between the app and the hub, recording
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
  dump       dump a hub's configuration as JSON (a lossless backup: raw
             record bytes are kept next to the decoded view)
  proxy      stand between the Sofabaton app and the hub, relaying and
             recording every frame (-capture file.jsonl)
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
	case "dump":
		return dump(ctx, args, stdout, stderr)
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
