package main

import (
	"errors"
	"fmt"
)

// parseServicesWord reads the word that only means something after services restart.
func parseServicesWord(result *options, word string) (bool, error) {
	switch word {
	case "cache":
		if result.command != "services" || result.subcommand != "restart" || result.service != "" {
			return true, fmt.Errorf("unexpected argument %q", word)
		}
		result.service = "cache"
		return true, nil
	}
	return false, nil
}

// validateServices rejects option combinations that do not belong to the command.
func validateServices(result *options) error {
	if result.command == "services" && result.subcommand != "" && result.subcommand != "restart" {
		return errors.New("services accepts only the restart subcommand")
	}
	if result.command == "services" && result.subcommand == "restart" && result.service == "" {
		return errors.New("services restart requires cache")
	}
	return nil
}
