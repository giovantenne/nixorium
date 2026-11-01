package main

import (
	"errors"
)

// parseLogsWord reads the word that only means something after logs.
func parseLogsWord(result *options, word string) (bool, error) {
	switch word {
	case "show":
		if result.command != "logs" || result.subcommand != "" {
			return true, errors.New("show must follow logs")
		}
		result.subcommand = "show"
		return true, nil
	}
	return false, nil
}

// parseLogsPositional takes the operation ID after logs show.
func parseLogsPositional(result *options, word string) bool {
	if result.command == "logs" && result.subcommand == "show" && result.logID == "" {
		result.logID = word
		return true
	}
	return false
}

// validateLogs rejects option combinations that do not belong to the command.
func validateLogs(result *options) error {
	if result.command == "logs" && result.subcommand != "" && result.subcommand != "show" {
		return errors.New("logs accepts only the show subcommand")
	}
	if result.command == "logs" && result.subcommand == "show" && result.logID == "" {
		return errors.New("logs show requires an operation log ID")
	}
	return nil
}
