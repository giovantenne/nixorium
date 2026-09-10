package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/presentation"
)

type options struct {
	command    string
	subcommand string
	repository string
	file       string
	expect     string
	json       bool
	full       bool
	help       bool
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	options, err := parseArguments(arguments)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		usage(stderr)
		return 2
	}
	if options.help {
		usage(stdout)
		return 0
	}
	repository, err := resolveRepository(options.repository)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	inspector := app.NewInspector(adapters.Local{})
	switch options.command {
	case "":
		report, inspectErr := inspector.Status(ctx, repository)
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			if jsonErr := presentation.JSON(stdout, report); jsonErr != nil {
				fmt.Fprintln(stderr, "Error:", jsonErr)
				return 1
			}
			return 0
		}
		if tuiErr := presentation.RunDashboard(report); tuiErr != nil {
			fmt.Fprintln(stderr, "Error:", tuiErr)
			return 1
		}
		return 0
	case "status":
		report, inspectErr := inspector.Status(ctx, repository)
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.StatusText(stdout, report)
		}
	case "doctor":
		report, inspectErr := inspector.Doctor(ctx, repository, app.DoctorOptions{Full: options.full})
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.DoctorText(stdout, report)
		}
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		if report.HasErrors() {
			return 1
		}
	case "config":
		manager := app.NewSettingsManager(adapters.Local{})
		switch options.subcommand {
		case "validate":
			report := manager.Validate(ctx, repository)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.ConfigValidationText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		case "plan", "apply":
			candidate, readErr := readCandidateSettings(options.file)
			if readErr != nil {
				fmt.Fprintln(stderr, "Error: read candidate settings:", readErr)
				return 1
			}
			if options.subcommand == "plan" {
				report := manager.Plan(ctx, repository, candidate)
				if options.json {
					err = presentation.JSON(stdout, report)
				} else {
					presentation.ConfigPlanText(stdout, report)
				}
				if report.HasErrors() {
					return 1
				}
			} else {
				report := manager.Apply(ctx, repository, candidate, options.expect)
				if options.json {
					err = presentation.JSON(stdout, report)
				} else {
					presentation.ConfigApplyText(stdout, report)
				}
				if report.HasErrors() {
					return 1
				}
			}
		}
	case "setup":
		manager := app.NewSetupManager(adapters.Local{})
		if options.subcommand == "keys" {
			report, reconcileErr := manager.ReconcileKeys(ctx, repository)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.KeyReconcileText(stdout, report)
			}
			if err == nil && reconcileErr != nil {
				err = reconcileErr
			}
		} else {
			report := manager.Status(ctx, repository)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.SetupText(stdout, report)
			}
		}
	default:
		fmt.Fprintf(stderr, "Error: unknown command %q\n", options.command)
		usage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

func parseArguments(arguments []string) (options, error) {
	result := options{}
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--repo":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--repo requires a path")
			}
			result.repository = arguments[index]
		case "--file":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--file requires a path")
			}
			result.file = arguments[index]
		case "--expect":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--expect requires a fingerprint")
			}
			result.expect = arguments[index]
		case "--json":
			result.json = true
		case "--full":
			result.full = true
		case "-h", "--help", "help":
			result.help = true
		case "status":
			if result.command == "setup" && result.subcommand == "" {
				result.subcommand = "status"
				continue
			}
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
		case "doctor", "config", "setup":
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
		case "validate":
			if result.command != "config" || result.subcommand != "" {
				return options{}, errors.New("validate must follow config")
			}
			result.subcommand = "validate"
		case "plan", "apply":
			if result.command != "config" || result.subcommand != "" {
				return options{}, fmt.Errorf("%s must follow config", arguments[index])
			}
			result.subcommand = arguments[index]
		case "keys":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("keys must follow setup")
			}
			result.subcommand = "keys"
		default:
			return options{}, fmt.Errorf("unknown argument %q", arguments[index])
		}
	}
	if result.full && result.command != "doctor" {
		return options{}, errors.New("--full is only valid with doctor")
	}
	if result.command == "config" && result.subcommand != "validate" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("config requires the validate, plan, or apply subcommand")
	}
	if result.file != "" && (result.command != "config" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--file is only valid with config plan or config apply")
	}
	if result.command == "config" && (result.subcommand == "plan" || result.subcommand == "apply") && result.file == "" {
		return options{}, fmt.Errorf("config %s requires --file", result.subcommand)
	}
	if result.expect != "" && (result.command != "config" || result.subcommand != "apply") {
		return options{}, errors.New("--expect is only valid with config apply")
	}
	if result.command == "config" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("config apply requires --expect from config plan")
	}
	if result.command == "setup" && result.subcommand != "status" && result.subcommand != "keys" {
		return options{}, errors.New("setup requires the status or keys subcommand")
	}
	return result, nil
}

func resolveRepository(explicit string) (string, error) {
	if explicit != "" {
		return requireDeploymentRoot(explicit)
	}
	if configured := os.Getenv("NIXORIUM_REPO"); configured != "" {
		return requireDeploymentRoot(configured)
	}
	if current, err := os.Getwd(); err == nil && isDeploymentRoot(current) {
		return filepath.Abs(current)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, "nixorium-deployment")
		if isDeploymentRoot(candidate) {
			return filepath.Abs(candidate)
		}
	}
	return "", errors.New("deployment repository not found; run from its root or pass --repo <path>")
}

func requireDeploymentRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve deployment repository: %w", err)
	}
	if !isDeploymentRoot(absolute) {
		return "", fmt.Errorf("%s is not a deployment repository (missing flake.nix)", absolute)
	}
	return absolute, nil
}

func isDeploymentRoot(path string) bool {
	info, err := os.Stat(filepath.Join(path, "flake.nix"))
	return err == nil && info.Mode().IsRegular()
}

func readCandidateSettings(path string) ([]byte, error) {
	const maximumBytes = int64(1024 * 1024)
	fileDescriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fileDescriptor), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("candidate must be a regular file and not a symlink")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximumBytes {
		return nil, errors.New("candidate settings file is unexpectedly large")
	}
	return data, nil
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage: nixorium [status|doctor|config validate|config plan|config apply|setup status|setup keys] [options]")
	fmt.Fprintln(writer, "       config plan --file <candidate.json>")
	fmt.Fprintln(writer, "       config apply --file <candidate.json> --expect <sha256:fingerprint>")
	fmt.Fprintln(writer, "       nixorium opens the read-only management dashboard")
	fmt.Fprintln(writer, "       doctor --full also builds the controller configuration")
}
