package main

import (
	"errors"
	"fmt"
	"strings"
)

// parseDeployFlags reads the deploy flags.
func parseDeployFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--acknowledge-unreachable":
		result.acknowledgeUnreachable = true
	case "--queue-unreachable":
		result.queueUnreachable = true
	default:
		return index, false, nil
	}
	return index, true, nil
}

// parseDeployWord reads the words that only mean something after deploy or template-reset.
func parseDeployWord(result *options, word string) (bool, error) {
	switch word {
	case "queue", "run":
		// deploy queue status|cancel|run: client updates that wait for
		// their computers.
		if result.command != "deploy" {
			return true, fmt.Errorf("%s must follow deploy", word)
		}
		if word == "queue" && result.subcommand == "" {
			result.subcommand = "queue"
			return true, nil
		}
		if word != "queue" && result.subcommand == "queue" {
			result.subcommand = "queue-" + word
			return true, nil
		}
		return true, errors.New("use deploy queue status, deploy queue cancel or deploy queue run")
	case "status":
		if result.command == "deploy" && result.subcommand == "queue" {
			result.subcommand = "queue-status"
			return true, nil
		}
	case "plan":
		if (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "recover" {
			result.subcommand = "recover-plan"
			return true, nil
		}
	case "apply":
		if (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "recover" {
			result.subcommand = "recover-apply"
			return true, nil
		}
	case "recover":
		if (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "" {
			result.subcommand = "recover"
			return true, nil
		}
	case "cancel":
		if result.command == "deploy" && result.subcommand == "queue" {
			result.subcommand = "queue-cancel"
			return true, nil
		}
	}
	return false, nil
}

// validateDeployRecovery rejects option combinations that do not belong to the command.
func validateDeployRecovery(result *options) error {
	if (result.command == "deploy" || result.command == "template-reset") && strings.HasPrefix(result.subcommand, "recover") {
		if result.subcommand != "recover-plan" && result.subcommand != "recover-apply" {
			return fmt.Errorf("%s recover requires plan or apply", result.command)
		}
		if result.on != "" {
			return errors.New("--on is not used by recovery; the interrupted record names its computers")
		}
		if result.subcommand == "recover-apply" && result.expect == "" {
			return fmt.Errorf("%s recover apply requires --expect from %s recover plan", result.command, result.command)
		}
		if result.acknowledgeUnreachable && result.command != "deploy" {
			return errors.New("--acknowledge-unreachable is only valid with deploy recover")
		}
	} else if result.command == "template-reset" {
		return errors.New("template-reset accepts only recover plan or recover apply")
	} else if result.acknowledgeUnreachable {
		return errors.New("--acknowledge-unreachable is only valid with deploy recover")
	}
	return nil
}

// validateDeployQueue rejects option combinations that do not belong to the command.
func validateDeployQueue(result *options) error {
	queueing := strings.HasPrefix(result.subcommand, "queue")
	if result.command == "deploy" && queueing {
		switch result.subcommand {
		case "queue-status", "queue-run":
			if result.on != "" {
				return fmt.Errorf("deploy %s does not take --on", strings.ReplaceAll(result.subcommand, "-", " "))
			}
		case "queue-cancel":
			if result.on == "" {
				return errors.New("deploy queue cancel requires --on <pcNN,...|@all>")
			}
		default:
			return errors.New("use deploy queue status, deploy queue cancel or deploy queue run")
		}
	}
	if result.queueUnreachable && (result.command != "deploy" || result.subcommand != "apply") {
		return errors.New("--queue-unreachable is only valid with deploy apply")
	}
	return nil
}

// validateDeployCommand rejects option combinations that do not belong to the command.
func validateDeployCommand(result *options) error {
	recovering := strings.HasPrefix(result.subcommand, "recover-")
	queueing := strings.HasPrefix(result.subcommand, "queue")
	if result.command == "deploy" && !recovering && !queueing && result.subcommand != "plan" && result.subcommand != "apply" {
		return errors.New("deploy requires the plan or apply subcommand")
	}
	return nil
}

// validateDeployTarget rejects option combinations that do not belong to the command.
func validateDeployTarget(result *options) error {
	recovering := strings.HasPrefix(result.subcommand, "recover-")
	queueing := strings.HasPrefix(result.subcommand, "queue")
	if result.command == "deploy" && !recovering && !queueing && result.on == "" {
		return fmt.Errorf("deploy %s requires --on", result.subcommand)
	}
	return nil
}

// validateDeployApplyExpect rejects option combinations that do not belong to the command.
func validateDeployApplyExpect(result *options) error {
	if result.command == "deploy" && result.subcommand == "apply" && result.expect == "" {
		return errors.New("deploy apply requires --expect from deploy plan")
	}
	return nil
}
