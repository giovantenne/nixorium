package main

import (
	"errors"
	"fmt"
)

// parseConfigWord reads the word that only means something after config.
func parseConfigWord(result *options, word string) (bool, error) {
	switch word {
	case "validate":
		if result.command != "config" || result.subcommand != "" {
			return true, errors.New("validate must follow config")
		}
		result.subcommand = "validate"
		return true, nil
	}
	return false, nil
}

// validateConfigSubcommand rejects option combinations that do not belong to the command.
func validateConfigSubcommand(result *options) error {
	if result.command == "config" && result.subcommand != "validate" && result.subcommand != "plan" && result.subcommand != "apply" {
		return errors.New("config requires the validate, plan, or apply subcommand")
	}
	return nil
}

// validateConfigFile rejects option combinations that do not belong to the command.
func validateConfigFile(result *options) error {
	if result.command == "config" && (result.subcommand == "plan" || result.subcommand == "apply") && result.file == "" {
		return fmt.Errorf("config %s requires --file", result.subcommand)
	}
	return nil
}

// validateConfigApplyExpect rejects option combinations that do not belong to the command.
func validateConfigApplyExpect(result *options) error {
	if result.command == "config" && result.subcommand == "apply" && result.expect == "" {
		return errors.New("config apply requires --expect from config plan")
	}
	return nil
}
