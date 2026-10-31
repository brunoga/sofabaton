package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/brunoga/sofabaton/capture"
	"github.com/brunoga/sofabaton/proxy"
	"github.com/brunoga/sofabaton/wire"
)

func proxyCmd(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("proxy", flag.ContinueOnError)
	hubFlag := fs.String("hub", os.Getenv(hubEnv), "hub IP address (default $"+hubEnv+", else the one hub discovery finds)")
	out := fs.String("capture", "", "append every frame to this capture file (JSON lines)")
	verbose := fs.Bool("v", false, "log every frame to stderr")
	noMDNS := fs.Bool("no-mdns", false, "do not advertise over mDNS (the Android app needs it)")
	timeout := fs.Duration("timeout", 20*time.Second, "time to wait for the hub to connect back, on each (re)connection")
	if err := fs.Parse(args); err != nil {
		return err
	}
	addr, err := hubAddr(ctx, *hubFlag, stderr)
	if err != nil {
		return err
	}

	var rec *capture.Writer
	if *out != "" {
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // captures can hold Wi-Fi command URLs
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		rec = capture.NewWriter(f)
	}
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := proxy.Config{
		Hub:         addr,
		NoMDNS:      *noMDNS,
		DialTimeout: *timeout,
		Log:         log,
		Tap: func(link string, dir wire.Direction, f wire.Frame) {
			if rec != nil {
				_ = rec.Frame(link, dir, f)
			}
			if *verbose {
				fmt.Fprintf(stderr, "%-3s %s %v %-18s % x\n", link, dir, f.Opcode(), f.Family.Name(dir), f.Payload)
			}
		},
	}

	sctx, cancel := context.WithTimeout(ctx, *timeout)
	p, err := proxy.Start(sctx, cfg)
	cancel()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "proxying %s (%v). Open the Sofabaton app now.\n", p.Info().Name, p.Info().Variant)
	if rec != nil {
		fmt.Fprintln(stdout, "Type a line and Enter to add a note to the capture; Ctrl-C to stop.")
		go notes(stdin, rec, stdout)
	}
	return p.Run(ctx)
}

// notes turns each line of input into a note in the capture.
func notes(in io.Reader, rec *capture.Writer, stdout io.Writer) {
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			if err := rec.Note(line); err == nil {
				fmt.Fprintf(stdout, "noted: %s\n", line)
			}
		}
	}
}
