// nixorium-home-reset is a root-only systemd helper with no target arguments.
package main

import (
	"fmt"
	"os"

	"github.com/giovantenne/nixorium/internal/homereset"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "This internal helper accepts no arguments.")
		os.Exit(1)
	}
	if err := homereset.ResetConfigured(); err != nil {
		fmt.Fprintln(os.Stderr, "Student workspace reset failed:", err)
		os.Exit(1)
	}
}
