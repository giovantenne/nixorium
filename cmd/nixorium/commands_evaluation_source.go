package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/giovantenne/nixorium/internal/adapters"
)

func parseEvaluationSourceArguments(arguments []string) (options, error) {
	result := options{command: "config", subcommand: "source"}
	flags := flag.NewFlagSet("config source", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&result.repository, "repo", "", "deployment directory")
	flags.StringVar(&result.expect, "revision", "", "committed revision")
	if err := flags.Parse(arguments); err != nil {
		return result, err
	}
	if flags.NArg() != 0 {
		return result, errors.New("config source accepts only --repo and --revision")
	}
	return result, nil
}

func runEvaluationSource(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	source, err := (adapters.Local{}).EvaluationSource(ctx, repository, options.expect)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	fmt.Fprintln(stdout, source)
	return 0
}
