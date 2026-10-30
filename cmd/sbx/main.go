// Command sbx is a command-line client for Sofabaton hubs.
package main

import (
	"flag"
	"fmt"
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	fmt.Fprintln(os.Stderr, "sbx: no commands yet; see PLAN.md")
	os.Exit(2)
}
