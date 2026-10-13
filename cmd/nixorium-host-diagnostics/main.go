// nixorium-host-diagnostics is an internal, fixed read-only observation helper.
package main

import (
	"context"
	"fmt"
	"github.com/giovantenne/nixorium/internal/hostdiagnostics"
	"os"
)

func main() {
	if len(os.Args) != 2 || !hostdiagnostics.ValidArea(os.Args[1]) {
		fmt.Fprintln(os.Stderr, "Choose one supported diagnostics area.")
		os.Exit(1)
	}
	config, err := hostdiagnostics.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Native diagnostics are unavailable; apply the current configuration through its ordinary review.")
		os.Exit(1)
	}
	data, err := hostdiagnostics.Encode(hostdiagnostics.Collect(context.Background(), config, os.Args[1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Native diagnostics could not be collected.")
		os.Exit(1)
	}
	os.Stdout.Write(append(data, '\n'))
}
