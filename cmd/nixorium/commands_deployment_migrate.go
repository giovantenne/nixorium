package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/deploymigrate"
)

// runDeploymentMigrate applies this release's deployment migrations to the
// deployment copy at --root (ADR 0034). The release that runs Update
// Nixorium calls it in a sandbox and reviews the file difference; an
// administrator may also run it on a deployment checkout and review it with
// git diff. Exit status: 0 ran (manual steps allowed), 2 unsupported
// contract or usage, 3 the deployment cannot be migrated, 1 other errors.
func runDeploymentMigrate(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("deployment migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "deployment directory to migrate")
	contract := flags.Int("contract", deploymigrate.ContractVersion, "contract version of the caller")
	asJSON := flags.Bool("json", false, "print the report as JSON")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *root == "" {
		fmt.Fprintln(stderr, "Usage: nixorium deployment migrate --root <deployment> [--contract N] [--json]")
		return 2
	}
	if !deploymigrate.SupportsContract(*contract) {
		fmt.Fprintf(stderr, "Error: this release supports migration contract %v, not %d; update to an intermediate release first\n", deploymigrate.SupportedContracts, *contract)
		return 2
	}
	directory, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 2
	}
	report, err := deploymigrate.Run(directory, nixoriumVersion, deploymigrate.Registry)
	if errors.Is(err, deploymigrate.ErrRefused) {
		fmt.Fprintln(stderr, "Error:", err)
		return 3
	}
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if *asJSON {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			return 1
		}
		return 0
	}
	if len(report.Migrations) == 0 {
		fmt.Fprintln(stdout, "No deployment migration to apply.")
	}
	for _, entry := range report.Migrations {
		fmt.Fprintf(stdout, "%-8s %s — %s\n", entry.State, entry.ID, entry.Description)
		if entry.Detail != "" {
			fmt.Fprintf(stdout, "         %s\n", entry.Detail)
		}
	}
	return 0
}
