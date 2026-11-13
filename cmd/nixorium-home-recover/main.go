// nixorium-home-recover runs on a student computer as root, started by the
// controller over SSH: it reads one recovery request on standard input and
// writes one JSON answer on standard output. It starts itself again as the
// student to write recovered files into the student's home.
package main

import (
	"fmt"
	"os"

	"github.com/giovantenne/nixorium/internal/homereset"
)

func main() {
	var err error
	switch {
	case len(os.Args) == 1:
		err = homereset.RecoverConfigured(os.Stdin, os.Stdout)
	case len(os.Args) == 2 && os.Args[1] == homereset.RecoverWriteArgument:
		err = homereset.WriteRecovered(os.Stdin, os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, "This internal helper accepts no arguments; the controller sends its request on standard input.")
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Student file recovery failed:", err)
		os.Exit(1)
	}
}
