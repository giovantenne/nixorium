package main

import (
	"errors"
)

// validateControllerCommand rejects option combinations that do not belong to the command.
func validateControllerCommand(result *options) error {
	if result.command == "controller" && result.subcommand != "plan" && result.subcommand != "apply" {
		return errors.New("controller requires the plan or apply subcommand")
	}
	return nil
}

// validateControllerApplyExpect rejects option combinations that do not belong to the command.
func validateControllerApplyExpect(result *options) error {
	if result.command == "controller" && result.subcommand == "apply" && result.expect == "" {
		return errors.New("controller apply requires --expect from controller plan")
	}
	return nil
}
