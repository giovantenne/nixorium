package main

import (
	"fmt"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/presentation"
)

// applyConfirmation describes the interactive confirmation of a reviewed
// apply: what to say without a terminal, how to report an unreadable answer
// and what to do when the answer is no.
type applyConfirmation struct {
	// noTerminal is printed to stderr (exit 2) when there is no terminal.
	noTerminal string
	// readFailure prefixes a read error on stderr; empty prints the bare error.
	readFailure string
	// cancelled is printed to the confirmation stream when the answer is no;
	// empty prints nothing.
	cancelled string
	// cancelCode is the exit code of a declined confirmation.
	cancelCode int
}

// confirmationStream is where a review is shown: stdout, or stderr when
// stdout carries JSON.
func confirmationStream(stdout, stderr io.Writer, jsonOutput bool) io.Writer {
	if jsonOutput {
		return stderr
	}
	return stdout
}

// confirmReviewedApply asks for the interactive confirmation of an apply whose
// review token has already been checked. It returns proceed=true when the
// apply may go on (assumeYes or an approved answer); otherwise code is the
// exit code of the command.
func confirmReviewedApply(assumeYes bool, stderr, out io.Writer, c applyConfirmation, confirm func(output io.Writer) (bool, error)) (code int, proceed bool) {
	if assumeYes {
		return 0, true
	}
	if !presentation.IsInteractive(os.Stdin) {
		fmt.Fprintln(stderr, c.noTerminal)
		return 2, false
	}
	approved, err := confirm(out)
	if err != nil {
		if c.readFailure != "" {
			fmt.Fprintln(stderr, c.readFailure, err)
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1, false
	}
	if !approved {
		if c.cancelled != "" {
			fmt.Fprintln(out, c.cancelled)
		}
		return c.cancelCode, false
	}
	return 0, true
}
