// Command scramblekey prints the key of Sofabaton's catalog, from
// $SOFABATON_KEY or the file named, scrambled for building into sbx:
//
//	go build -ldflags "-X main.builtinKey=$(go run ./tools/scramblekey)"
//
// It prints nothing when there is no key, which builds sbx without one.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/brunoga/sofabaton/irsource"
)

func main() {
	key := os.Getenv("SOFABATON_KEY")
	if key == "" && len(os.Args) > 1 {
		b, err := os.ReadFile(os.Args[1]) //nolint:gosec // G703: a build tool reading the file it is told to
		if err == nil {
			key = string(b)
		}
	}
	if key = strings.TrimSpace(key); key != "" {
		fmt.Print(irsource.ScrambleKey(key))
	}
}
