package main

import (
	"errors"
	"fmt"
)

// parsePXEWord reads the pxe subcommands; install usb and deploy recover claim their own uses of these words first.
func parsePXEWord(result *options, word string) (bool, error) {
	switch word {
	case "prepare":
		if result.command != "pxe" || result.subcommand != "" {
			return true, errors.New("prepare must follow pxe")
		}
		result.subcommand = "prepare"
	case "start":
		if result.command != "pxe" || result.subcommand != "" {
			return true, errors.New("start must follow pxe or install usb")
		}
		result.subcommand = "start"
	case "stop", "recover":
		if result.command != "pxe" || result.subcommand != "" {
			return true, fmt.Errorf("%s must follow pxe", word)
		}
		result.subcommand = word
	default:
		return false, nil
	}
	return true, nil
}

// validatePXECommand rejects option combinations that do not belong to the command.
func validatePXECommand(result *options) error {
	if result.command == "pxe" && result.subcommand != "prepare" && result.subcommand != "start" && result.subcommand != "stop" && result.subcommand != "recover" {
		return errors.New("pxe requires prepare, start, stop, or recover")
	}
	return nil
}
