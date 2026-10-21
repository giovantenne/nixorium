package main

import (
	"errors"
	"fmt"
)

func parseArguments(arguments []string) (options, error) {
	if len(arguments) > 0 && arguments[0] == "home-files" {
		return parseHomeFilesArguments(arguments[1:])
	}
	if len(arguments) >= 2 && arguments[0] == "config" && arguments[1] == "source" {
		return parseEvaluationSourceArguments(arguments[2:])
	}
	if len(arguments) >= 2 && arguments[0] == "backup" && (arguments[1] == "plan" || arguments[1] == "publish" || arguments[1] == "clone") {
		return parseGitBackupArguments(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "telemetry" {
		return parseTelemetryArguments(arguments[1:])
	}
	if len(arguments) >= 2 && arguments[0] == "git" && arguments[1] == "undo" {
		return parseGitUndoArguments(arguments[2:])
	}
	if len(arguments) > 0 && arguments[0] == "host-key" {
		return parseHostTrustArguments(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "collect" {
		return parseCollectArguments(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "network" {
		return parseNetworkArguments(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "schedule" {
		return parseScheduleArguments(arguments[1:])
	}
	result := options{}
	for index := 0; index < len(arguments); index++ {
		next, handled, err := parseFlag(&result, arguments, index)
		if err != nil {
			return options{}, err
		}
		if handled {
			index = next
			continue
		}
		handled, err = parseWord(&result, arguments[index])
		if err != nil {
			return options{}, err
		}
		if handled {
			continue
		}
		if parseLogsPositional(&result, arguments[index]) || parseBackupPositional(&result, arguments[index]) {
			continue
		}
		return options{}, fmt.Errorf("unknown argument %q", arguments[index])
	}
	for _, validate := range optionValidators {
		if err := validate(&result); err != nil {
			return options{}, err
		}
	}
	return result, nil
}

// flagParsers and wordParsers are tried in order; every flag or word belongs
// to exactly one family (the shared words status, plan, apply, recover, cancel,
// start and prepare are claimed by the family whose context matches first and
// otherwise fall through to the generic reader or the pxe reader).
var flagParsers = []func(*options, []string, int) (int, bool, error){
	parseGlobalFlags, parseBackupFlags, parseInternetLockFlags, parseInstallFlags,
	parseGitFlags, parseUpdateFlags, parsePackageBaseFlags, parseSoftwareFlags,
	parseWorkspaceFlags, parseOperationsFlags, parseDeployFlags, parseSetupFlags,
}

var wordParsers = []func(*options, string) (bool, error){
	parseDeployWord, parseGitWord, parseSoftwareWord, parseInstallWord,
	parseSetupWord, parsePackageBaseWord, parseRecoveryWord, parseBackupWord,
	parsePXEWord, parseUpdateWord, parseConfigWord, parseWorkspaceWord,
	parseSupportWord, parseLogsWord, parseServicesWord, parseGenericWord,
}

// optionValidators run in this order after every argument was read; the first
// error wins, so the order is part of the CLI contract.
var optionValidators = []func(*options) error{
	validateFullFlag,
	validateSupport,
	validateVerifyOnly,
	validateYesFlag,
	validateConfigSubcommand,
	validateFileFlag,
	validateConfigFile,
	validateExpectFlag,
	validateWorkspaceOptions,
	validateWake,
	validateOnFlag,
	validateCleanup,
	validateDeployRecovery,
	validateDeployQueue,
	validateDeployCommand,
	validateControllerCommand,
	validateUpdateCommand,
	validatePackageBase,
	validateUpdateOptions,
	validateSoftware,
	validateShutdownRestart,
	validateLock,
	validateSend,
	validateInternet,
	validateAcknowledgeUnknown,
	validateServices,
	validateBackup,
	validateRecoveryCommand,
	validateLogs,
	validateGitPaths,
	validateDeployTarget,
	validateConfigApplyExpect,
	validateDeployApplyExpect,
	validateControllerApplyExpect,
	validateGitApplyExpect,
	validateUpdateApplyExpect,
	validateSetupCommand,
	validatePXECommand,
	validateSetupInteractive,
	validateInstall,
}

func parseFlag(result *options, arguments []string, index int) (int, bool, error) {
	for _, parse := range flagParsers {
		next, handled, err := parse(result, arguments, index)
		if err != nil || handled {
			return next, true, err
		}
	}
	return index, false, nil
}

func parseWord(result *options, word string) (bool, error) {
	for _, parse := range wordParsers {
		handled, err := parse(result, word)
		if err != nil || handled {
			return true, err
		}
	}
	return false, nil
}

// parseGlobalFlags reads the flags every command shares.
func parseGlobalFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--repo":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--repo requires a path")
		}
		result.repository = arguments[index]
	case "--file":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--file requires a path")
		}
		result.file = arguments[index]
	case "--expect":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--expect requires a review token")
		}
		result.expect = arguments[index]
	case "--on":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--on requires a client or @lab")
		}
		result.on = arguments[index]
	case "--json":
		result.json = true
	case "--full":
		result.full = true
	case "--yes":
		result.yes = true
	case "-h", "--help", "help":
		result.help = true
	default:
		return index, false, nil
	}
	return index, true, nil
}

// validateFullFlag rejects option combinations that do not belong to the command.
func validateFullFlag(result *options) error {
	if result.full && result.command != "doctor" {
		return errors.New("--full is only valid with doctor")
	}
	return nil
}

// validateYesFlag rejects option combinations that do not belong to the command.
func validateYesFlag(result *options) error {
	if result.yes && !((result.command == "setup" && result.subcommand == "apply") || (result.command == "pxe" && result.subcommand == "start") || ((result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "apply") || (result.command == "software" && result.subcommand == "preset-apply") || (result.command == "services" && result.subcommand == "restart") || (result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply")) || result.subcommand == "recover-apply") {
		return errors.New("--yes is only valid with setup apply, pxe start, deploy apply, controller apply, update apply, software apply, workspace apply, shutdown/restart apply, cleanup apply, services restart, or git commit apply")
	}
	return nil
}

// validateFileFlag rejects option combinations that do not belong to the command.
func validateFileFlag(result *options) error {
	if result.file != "" && ((result.command != "config" && result.command != "workspace" && result.command != "send") || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return errors.New("--file is only valid with config, workspace or send plan/apply")
	}
	return nil
}

// validateExpectFlag rejects option combinations that do not belong to the command.
func validateExpectFlag(result *options) error {
	if result.expect != "" && !(((result.command == "config" || result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "apply") || (result.command == "software" && result.subcommand == "preset-apply") || (result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply")) || result.subcommand == "recover-apply") {
		return errors.New("--expect is only valid with config apply, deploy apply, controller apply, update apply, software apply, workspace apply, shutdown/restart apply, cleanup apply, or git commit apply")
	}
	return nil
}

// validateOnFlag rejects option combinations that do not belong to the command.
func validateOnFlag(result *options) error {
	if result.on != "" && result.command != "wake" && ((result.command != "deploy" && result.command != "shutdown" && result.command != "restart" && result.command != "internet" && result.command != "lock" && result.command != "send" && result.command != "cleanup") || (result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "queue-cancel")) {
		return errors.New("--on is only valid with deploy, shutdown, restart, Internet or cleanup plan/apply, wake, and deploy queue cancel")
	}
	return nil
}

// parseGenericWord reads the command names and the words shared by several commands.
func parseGenericWord(result *options, word string) (bool, error) {
	switch word {
	case "status":
		if result.command != "" {
			return true, errors.New("only one command may be selected")
		}
		result.command = word
	case "doctor", "hosts", "deploy", "controller", "services", "logs", "git", "config", "setup", "pxe", "update", "package-base", "software", "workspace", "shutdown", "internet", "lock", "send", "classroom-view", "install", "support", "cleanup", "recovery", "template-reset", "backup", "wake":
		if result.command != "" {
			return true, errors.New("only one command may be selected")
		}
		result.command = word
	case "plan":
		if (result.command != "config" && result.command != "deploy" && result.command != "controller" && result.command != "update" && result.command != "package-base" && result.command != "software" && result.command != "workspace" && result.command != "shutdown" && result.command != "restart" && result.command != "internet" && result.command != "lock" && result.command != "send" && result.command != "cleanup") || result.subcommand != "" {
			return true, errors.New("plan must follow config, deploy, controller, update, software, workspace, shutdown, restart, Internet, or git commit")
		}
		result.subcommand = "plan"
	case "apply":
		if (result.command == "config" || result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "" {
			result.subcommand = "apply"
			return true, nil
		}
		if result.command != "setup" || result.subcommand != "" {
			return true, errors.New("apply must follow config, deploy, controller, update, software, workspace, shutdown, restart, Internet, or setup")
		}
		result.subcommand = "apply"
	case "restart":
		if result.command == "" {
			result.command = "restart"
			return true, nil
		}
		if result.command != "services" || result.subcommand != "" {
			return true, errors.New("restart must follow services")
		}
		result.subcommand = "restart"
	default:
		return false, nil
	}
	return true, nil
}
