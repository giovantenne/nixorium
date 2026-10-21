package main

import (
	"errors"
	"fmt"
)

// parseUpdateFlags reads the update (and package-base) release flags.
func parseUpdateFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--target":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--target requires a release tag")
		}
		result.target = arguments[index]
	case "--allow-prerelease":
		result.allowPrerelease = true
	case "--allow-downgrade":
		result.allowDowngrade = true
	default:
		return index, false, nil
	}
	return index, true, nil
}

// parseUpdateWord reads the word that only means something after update.
func parseUpdateWord(result *options, word string) (bool, error) {
	switch word {
	case "check":
		if result.command != "update" || result.subcommand != "" {
			return true, errors.New("check must follow update")
		}
		result.subcommand = "check"
		return true, nil
	}
	return false, nil
}

// validateUpdateCommand rejects option combinations that do not belong to the command.
func validateUpdateCommand(result *options) error {
	if result.command == "update" && result.subcommand != "check" && result.subcommand != "plan" && result.subcommand != "apply" {
		return errors.New("update requires the check, plan, or apply subcommand")
	}
	return nil
}

// validateUpdateOptions rejects option combinations that do not belong to the command.
func validateUpdateOptions(result *options) error {
	if result.target != "" && ((result.command != "update" && result.command != "package-base") || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return errors.New("--target is only valid with update plan or update apply")
	}
	if (result.allowPrerelease || result.allowDowngrade) && (result.command != "update" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return errors.New("update policy flags are only valid with update plan or update apply")
	}
	if result.command == "update" && (result.subcommand == "plan" || result.subcommand == "apply") && result.target == "" {
		return fmt.Errorf("update %s requires --target", result.subcommand)
	}
	return nil
}

// validateUpdateApplyExpect rejects option combinations that do not belong to the command.
func validateUpdateApplyExpect(result *options) error {
	if result.command == "update" && result.subcommand == "apply" && result.expect == "" {
		return errors.New("update apply requires --expect from update plan")
	}
	return nil
}
