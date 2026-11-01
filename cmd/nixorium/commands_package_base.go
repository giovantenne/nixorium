package main

import (
	"errors"
)

// parsePackageBaseFlags reads the package-base-only flags.
func parsePackageBaseFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--allow-unverified":
		result.allowUnverified = true
	default:
		return index, false, nil
	}
	return index, true, nil
}

// parsePackageBaseWord reads the words that only mean something after package-base.
func parsePackageBaseWord(result *options, word string) (bool, error) {
	if word == "status" && result.command == "package-base" && result.subcommand == "" {
		result.subcommand = "status"
		return true, nil
	}
	return false, nil
}

// validatePackageBase rejects option combinations that do not belong to the command.
func validatePackageBase(result *options) error {
	if result.command == "package-base" && result.subcommand != "status" && result.subcommand != "plan" && result.subcommand != "apply" {
		return errors.New("package-base requires status, plan or apply")
	}
	if result.allowUnverified && (result.command != "package-base" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return errors.New("--allow-unverified is only valid with package-base plan/apply")
	}
	if result.command == "package-base" && result.subcommand == "apply" && result.expect == "" {
		return errors.New("package-base apply requires --expect from a reviewed plan")
	}
	return nil
}
