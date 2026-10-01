package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

type supportCommandManager interface {
	Preview(context.Context, string, string) (domain.SupportSnapshot, error)
	Export(context.Context, domain.SupportSnapshot) domain.SupportExportResult
}

func runSupportCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	if options.subcommand == "export" && (!presentation.IsInteractive(os.Stdin) || !interactiveWriter(stdout)) {
		fmt.Fprintln(stderr, "Error: support export requires interactive input and output for preview; use support preview --json for inspection.")
		return 1
	}
	confirm := func() (bool, error) {
		if _, err := fmt.Fprint(stdout, "Save this exact report locally? [y/N] "); err != nil {
			return false, err
		}
		line, err := bufio.NewReader(io.LimitReader(os.Stdin, 32)).ReadString('\n')
		return strings.EqualFold(strings.TrimSpace(line), "y") && err == nil, err
	}
	return runSupportWithManager(ctx, app.NewSupportManager(adapters.Local{}), repository, options, confirm, stdout, stderr)
}

func interactiveWriter(writer io.Writer) bool {
	if recorded, ok := writer.(recordedWriter); ok {
		writer = recorded.file
	}
	file, ok := writer.(*os.File)
	return ok && presentation.IsInteractive(file)
}

func runSupportWithManager(ctx context.Context, manager supportCommandManager, repository string, options options, confirm func() (bool, error), stdout, stderr io.Writer) int {
	snapshot, err := manager.Preview(ctx, repository, nixoriumVersion)
	if err != nil || !snapshot.Valid() {
		// Do not print arbitrary collection errors beside a shareable report.
		return commandOutputError(errors.New("support collection did not produce a report"), stderr)
	}
	if !options.json {
		if _, err := fmt.Fprintln(stdout, "Local support preview — minimized, not anonymous. Review version, revision, time and counts before sharing.\nNo upload, build or remediation. Unavailable sections are not healthy results."); err != nil {
			return 1
		}
	}
	if _, err := io.WriteString(stdout, snapshot.JSON()); err != nil {
		return commandOutputError(err, stderr)
	}
	if options.subcommand == "preview" {
		return 0
	}
	approved, err := confirm()
	if err != nil {
		return commandOutputError(errors.New("support export confirmation could not be read; nothing was saved"), stderr)
	}
	if !approved {
		fmt.Fprintln(stdout, "Export cancelled; nothing was saved.")
		return 0
	}
	result := manager.Export(ctx, snapshot)
	fmt.Fprintln(stdout, result.Message)
	if result.Path != "" {
		fmt.Fprintf(stdout, "Local file: %q\nSHA-256: %s\n", result.Path, result.SHA256)
	}
	if result.State != "saved" {
		return 1
	}
	return 0
}
