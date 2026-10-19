// nixorium-host-diagnostics is an internal, fixed read-only observation helper.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/giovantenne/nixorium/internal/hostdiagnostics"
	"github.com/giovantenne/nixorium/internal/systemfiles"
)

func main() {
	// "file PATH" reads one allowlisted system file or directory (ADR 0032).
	if len(os.Args) == 3 && os.Args[1] == "file" {
		os.Exit(readSystemFile(os.Args[2]))
	}
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
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		os.Exit(1)
	}
}

// readSystemFile prints the answer for one path. Exit status 3 means the path
// is refused, 4 that it does not exist, 5 that this account cannot read it.
func readSystemFile(path string) int {
	result, err := systemfiles.Read(path)
	switch {
	case errors.Is(err, systemfiles.ErrRefused):
		fmt.Fprintln(os.Stderr, "This path is not readable by the Assistant.")
		return 3
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintln(os.Stderr, "No such file or directory.")
		return 4
	case errors.Is(err, os.ErrPermission):
		fmt.Fprintln(os.Stderr, "This account cannot read it.")
		return 5
	case err != nil:
		fmt.Fprintln(os.Stderr, "The file could not be read.")
		return 1
	}
	data, err := systemfiles.Encode(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "The answer is too large.")
		return 1
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		return 1
	}
	return 0
}
