package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"io"
)

func parseTelemetryArguments(args []string) (options, error) {
	o := options{command: "telemetry"}
	for _, arg := range args {
		switch arg {
		case "--json":
			o.json = true
		case "--help", "-h":
			o.help = true
		case "status", "preview", "enable", "disable", "send":
			if o.subcommand != "" {
				return o, errors.New("select one telemetry action")
			}
			o.subcommand = arg
		default:
			return o, errors.New("telemetry accepts status, preview, enable or disable")
		}
	}
	if o.subcommand == "" && !o.help {
		return o, errors.New("telemetry requires status, preview, enable or disable")
	}
	if o.json && o.subcommand != "status" && o.subcommand != "preview" {
		return o, errors.New("--json is only available for telemetry status or preview")
	}
	return o, nil
}
func runTelemetryCommand(ctx context.Context, o options, stdout, stderr io.Writer) int {
	m := app.NewTelemetryManager(adapters.DefaultTelemetry())
	if o.subcommand == "send" {
		if err := m.Send(ctx); err != nil {
			return commandOutputError(err, stderr)
		}
		return 0
	}
	r, err := m.Run(ctx, o.subcommand)
	if err != nil {
		return commandOutputError(err, stderr)
	}
	var output any = r
	if o.subcommand == "preview" {
		output = r.Payload
	}
	if !o.json {
		fmt.Fprintln(stdout, domain.TelemetryNotice)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(output); err != nil {
		return commandOutputError(err, stderr)
	}
	return 0
}
