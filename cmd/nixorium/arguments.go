package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
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
		switch arguments[index] {
		case "--to", "--passphrase-file":
			flag := arguments[index]
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, fmt.Errorf("%s requires a path", flag)
			}
			if flag == "--to" {
				result.backupTarget = arguments[index]
			} else {
				result.passphraseFile = arguments[index]
			}
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
				return options{}, errors.New("--expect requires a review token")
			}
			result.expect = arguments[index]
		case "--action":
			index++
			if index >= len(arguments) {
				return options{}, errors.New("--action requires block/unblock or lock/unlock")
			}
			result.internetAction = domain.InternetAction(arguments[index])
		case "--on":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--on requires a client or @lab")
			}
			result.on = arguments[index]
		case "--host":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--host requires an inventory client name")
			}
			result.host = arguments[index]
		case "--id":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--id requires a remote installation operation ID")
			}
			result.operationID = arguments[index]
		case "--paths":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--paths requires comma-separated repository paths")
			}
			result.paths = arguments[index]
		case "--target":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--target requires a release tag")
			}
			result.target = arguments[index]
		case "--package":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--package requires a catalog identifier")
			}
			result.softwarePackage = arguments[index]
		case "--preset":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--preset requires a software profile id")
			}
			result.softwarePreset = arguments[index]
		case "--exclude":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--exclude requires comma-separated package ids")
			}
			result.softwareExclude = arguments[index]
		case "--query":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--query requires a package-name fragment")
			}
			result.softwareQuery = arguments[index]
		case "--extension":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--extension requires a Marketplace identifier such as publisher.name")
			}
			result.extension = arguments[index]
		case "--scope":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--scope requires shared, controller, all-clients, group:NAME, or clients:pcNN,...")
			}
			result.softwareScope = arguments[index]
		case "--remove":
			result.remove = true
		case "--allow-prerelease":
			result.allowPrerelease = true
		case "--allow-downgrade":
			result.allowDowngrade = true
		case "--allow-unverified":
			result.allowUnverified = true
		case "--acknowledge-unknown-sessions":
			result.acknowledgeUnknown = true
		case "--acknowledge-unreachable":
			result.acknowledgeUnreachable = true
		case "--queue-unreachable":
			result.queueUnreachable = true
		case "queue", "run":
			// deploy queue status|cancel|run: client updates that wait for
			// their computers.
			if result.command != "deploy" {
				return options{}, fmt.Errorf("%s must follow deploy", arguments[index])
			}
			if arguments[index] == "queue" && result.subcommand == "" {
				result.subcommand = "queue"
				continue
			}
			if arguments[index] != "queue" && result.subcommand == "queue" {
				result.subcommand = "queue-" + arguments[index]
				continue
			}
			return options{}, errors.New("use deploy queue status, deploy queue cancel or deploy queue run")
		case "--json":
			result.json = true
		case "--full":
			result.full = true
		case "--verify-only":
			result.verifyOnly = true
		case "--yes":
			result.yes = true
		case "-h", "--help", "help":
			result.help = true
		case "status":
			if result.command == "deploy" && result.subcommand == "queue" {
				result.subcommand = "queue-status"
				continue
			}
			if result.command == "install" && result.subcommand == "usb" {
				result.subcommand = "usb-status"
				continue
			}
			if (result.command == "setup" || result.command == "package-base" || result.command == "recovery") && result.subcommand == "" {
				result.subcommand = "status"
				continue
			}
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
		case "doctor", "hosts", "deploy", "controller", "services", "logs", "git", "config", "setup", "pxe", "update", "package-base", "software", "workspace", "shutdown", "internet", "lock", "send", "classroom-view", "install", "support", "cleanup", "recovery", "template-reset", "backup", "wake":
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
		case "preview", "export":
			if result.command != "support" || result.subcommand != "" {
				return options{}, errors.New("preview/export must follow support")
			}
			result.subcommand = arguments[index]
		case "usb":
			if result.command != "install" || result.subcommand != "" {
				return options{}, errors.New("usb must follow install")
			}
			result.subcommand = "usb"
		case "validate":
			if result.command != "config" || result.subcommand != "" {
				return options{}, errors.New("validate must follow config")
			}
			result.subcommand = "validate"
		case "check":
			if result.command != "update" || result.subcommand != "" {
				return options{}, errors.New("check must follow update")
			}
			result.subcommand = "check"
		case "catalog":
			if result.command != "software" || result.subcommand != "" {
				return options{}, errors.New("catalog must follow software")
			}
			result.subcommand = "catalog"
		case "presets":
			if result.command != "software" || result.subcommand != "" {
				return options{}, errors.New("presets must follow software")
			}
			result.subcommand = "presets"
		case "preset":
			if result.command != "software" || result.subcommand != "" {
				return options{}, errors.New("preset must follow software")
			}
			result.subcommand = "preset"
		case "marketplace":
			if result.command != "workspace" || result.subcommand != "" {
				return options{}, errors.New("marketplace must follow workspace")
			}
			result.subcommand = "marketplace"
		case "search":
			if result.command != "software" || result.subcommand != "" {
				return options{}, errors.New("search must follow software")
			}
			result.subcommand = "search"
		case "plan":
			if result.command == "git" && result.subcommand == "discard" {
				result.subcommand = "discard-plan"
				continue
			}
			if result.command == "git" && result.subcommand == "commit" {
				result.subcommand = "commit-plan"
				continue
			}
			if result.command == "software" && result.subcommand == "preset" {
				result.subcommand = "preset-plan"
				continue
			}
			if (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "recover" {
				result.subcommand = "recover-plan"
				continue
			}
			if (result.command != "config" && result.command != "deploy" && result.command != "controller" && result.command != "update" && result.command != "package-base" && result.command != "software" && result.command != "workspace" && result.command != "shutdown" && result.command != "restart" && result.command != "internet" && result.command != "lock" && result.command != "send" && result.command != "cleanup") || result.subcommand != "" {
				return options{}, errors.New("plan must follow config, deploy, controller, update, software, workspace, shutdown, restart, Internet, or git commit")
			}
			result.subcommand = "plan"
		case "keys":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("keys must follow setup")
			}
			result.subcommand = "keys"
		case "configure":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("configure must follow setup")
			}
			result.subcommand = "configure"
		case "install-secrets":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("install-secrets must follow setup")
			}
			result.subcommand = "install-secrets"
		case "apply":
			if result.command == "git" && result.subcommand == "discard" {
				result.subcommand = "discard-apply"
				continue
			}
			if result.command == "git" && result.subcommand == "commit" {
				result.subcommand = "commit-apply"
				continue
			}
			if result.command == "software" && result.subcommand == "preset" {
				result.subcommand = "preset-apply"
				continue
			}
			if (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "recover" {
				result.subcommand = "recover-apply"
				continue
			}
			if (result.command == "config" || result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "" {
				result.subcommand = "apply"
				continue
			}
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("apply must follow config, deploy, controller, update, software, workspace, shutdown, restart, Internet, or setup")
			}
			result.subcommand = "apply"
		case "restart":
			if result.command == "" {
				result.command = "restart"
				continue
			}
			if result.command != "services" || result.subcommand != "" {
				return options{}, errors.New("restart must follow services")
			}
			result.subcommand = "restart"
		case "show":
			if result.command != "logs" || result.subcommand != "" {
				return options{}, errors.New("show must follow logs")
			}
			result.subcommand = "show"
		case "review":
			if result.command != "git" || result.subcommand != "" {
				return options{}, errors.New("review must follow git")
			}
			result.subcommand = "review"
		case "commit", "discard":
			if result.command != "git" || result.subcommand != "" {
				return options{}, fmt.Errorf("%s must follow git", arguments[index])
			}
			result.subcommand = arguments[index]
		case "cache":
			if result.command != "services" || result.subcommand != "restart" || result.service != "" {
				return options{}, fmt.Errorf("unexpected argument %q", arguments[index])
			}
			result.service = "cache"
		case "prepare":
			if result.command == "install" && result.subcommand == "usb" {
				result.subcommand = "usb-prepare"
				continue
			}
			if result.command != "pxe" || result.subcommand != "" {
				return options{}, errors.New("prepare must follow pxe")
			}
			result.subcommand = "prepare"
		case "start":
			if result.command == "install" && result.subcommand == "usb" {
				result.subcommand = "usb-start"
				continue
			}
			if result.command != "pxe" || result.subcommand != "" {
				return options{}, errors.New("start must follow pxe or install usb")
			}
			result.subcommand = "start"
		case "stop", "recover":
			if arguments[index] == "recover" && (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "" {
				result.subcommand = "recover"
				continue
			}
			if result.command != "pxe" || result.subcommand != "" {
				return options{}, fmt.Errorf("%s must follow pxe", arguments[index])
			}
			result.subcommand = arguments[index]
		case "create", "verify", "restore":
			if result.command == "backup" && result.subcommand == "" {
				result.subcommand = arguments[index]
				continue
			}
			if arguments[index] != "verify" {
				return options{}, fmt.Errorf("%s must follow backup", arguments[index])
			}
			if result.command != "install" || result.subcommand != "usb" {
				return options{}, errors.New("verify must follow install usb or backup")
			}
			result.subcommand = "usb-verify"
		case "reconcile", "reboot", "cancel", "close":
			if arguments[index] == "cancel" && result.command == "deploy" && result.subcommand == "queue" {
				result.subcommand = "queue-cancel"
				continue
			}
			if result.command != "install" || result.subcommand != "usb" {
				return options{}, fmt.Errorf("%s must follow install usb", arguments[index])
			}
			result.subcommand = "usb-" + arguments[index]
		default:
			if result.command == "logs" && result.subcommand == "show" && result.logID == "" {
				result.logID = arguments[index]
				continue
			}
			if result.command == "backup" && (result.subcommand == "verify" || result.subcommand == "restore") && result.backupFile == "" {
				result.backupFile = arguments[index]
				continue
			}
			return options{}, fmt.Errorf("unknown argument %q", arguments[index])
		}
	}
	if result.full && result.command != "doctor" {
		return options{}, errors.New("--full is only valid with doctor")
	}
	if result.command == "support" {
		if result.subcommand != "preview" && result.subcommand != "export" {
			return options{}, errors.New("support requires preview or export")
		}
		if result.subcommand == "export" && result.json {
			return options{}, errors.New("support export requires interactive review; use support preview --json for machine-readable output")
		}
	}
	if result.verifyOnly && (result.command != "setup" || result.subcommand != "keys") {
		return options{}, errors.New("--verify-only is only valid with setup keys")
	}
	if result.yes && !((result.command == "setup" && result.subcommand == "apply") || (result.command == "pxe" && result.subcommand == "start") || ((result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "apply") || (result.command == "software" && result.subcommand == "preset-apply") || (result.command == "services" && result.subcommand == "restart") || (result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply")) || result.subcommand == "recover-apply") {
		return options{}, errors.New("--yes is only valid with setup apply, pxe start, deploy apply, controller apply, update apply, software apply, workspace apply, shutdown/restart apply, cleanup apply, services restart, or git commit apply")
	}
	if result.command == "config" && result.subcommand != "validate" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("config requires the validate, plan, or apply subcommand")
	}
	if result.file != "" && ((result.command != "config" && result.command != "workspace" && result.command != "send") || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--file is only valid with config, workspace or send plan/apply")
	}
	if result.command == "config" && (result.subcommand == "plan" || result.subcommand == "apply") && result.file == "" {
		return options{}, fmt.Errorf("config %s requires --file", result.subcommand)
	}
	if result.expect != "" && !(((result.command == "config" || result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "apply") || (result.command == "software" && result.subcommand == "preset-apply") || (result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply")) || result.subcommand == "recover-apply") {
		return options{}, errors.New("--expect is only valid with config apply, deploy apply, controller apply, update apply, software apply, workspace apply, shutdown/restart apply, cleanup apply, or git commit apply")
	}
	if result.extension != "" && (result.command != "workspace" || result.subcommand != "marketplace") {
		return options{}, errors.New("--extension is only valid with workspace marketplace")
	}
	if result.command == "workspace" && result.subcommand == "marketplace" && result.extension == "" {
		return options{}, errors.New("workspace marketplace requires --extension")
	}
	if result.command == "workspace" && result.subcommand != "marketplace" {
		if result.subcommand != "plan" && result.subcommand != "apply" {
			return options{}, errors.New("workspace requires plan, apply or marketplace")
		}
		if result.file == "" {
			return options{}, errors.New("workspace plan/apply requires --file")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("workspace apply requires --expect from workspace plan")
		}
	}
	if result.command == "wake" && (result.subcommand != "" || result.on == "") {
		return options{}, errors.New("wake requires --on <clients|@lab> and no subcommand")
	}
	if result.on != "" && result.command != "wake" && ((result.command != "deploy" && result.command != "shutdown" && result.command != "restart" && result.command != "internet" && result.command != "lock" && result.command != "send" && result.command != "cleanup") || (result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "queue-cancel")) {
		return options{}, errors.New("--on is only valid with deploy, shutdown, restart, Internet or cleanup plan/apply, wake, and deploy queue cancel")
	}
	if result.command == "cleanup" {
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" {
			return options{}, errors.New("cleanup requires plan or apply and --on <controller,pcNN,...|@lab>")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("cleanup apply requires --expect from cleanup plan")
		}
	}
	if (result.command == "deploy" || result.command == "template-reset") && strings.HasPrefix(result.subcommand, "recover") {
		if result.subcommand != "recover-plan" && result.subcommand != "recover-apply" {
			return options{}, fmt.Errorf("%s recover requires plan or apply", result.command)
		}
		if result.on != "" {
			return options{}, errors.New("--on is not used by recovery; the interrupted record names its computers")
		}
		if result.subcommand == "recover-apply" && result.expect == "" {
			return options{}, fmt.Errorf("%s recover apply requires --expect from %s recover plan", result.command, result.command)
		}
		if result.acknowledgeUnreachable && result.command != "deploy" {
			return options{}, errors.New("--acknowledge-unreachable is only valid with deploy recover")
		}
	} else if result.command == "template-reset" {
		return options{}, errors.New("template-reset accepts only recover plan or recover apply")
	} else if result.acknowledgeUnreachable {
		return options{}, errors.New("--acknowledge-unreachable is only valid with deploy recover")
	}
	recovering := strings.HasPrefix(result.subcommand, "recover-")
	queueing := strings.HasPrefix(result.subcommand, "queue")
	if result.command == "deploy" && queueing {
		switch result.subcommand {
		case "queue-status", "queue-run":
			if result.on != "" {
				return options{}, fmt.Errorf("deploy %s does not take --on", strings.ReplaceAll(result.subcommand, "-", " "))
			}
		case "queue-cancel":
			if result.on == "" {
				return options{}, errors.New("deploy queue cancel requires --on <pcNN,...|@all>")
			}
		default:
			return options{}, errors.New("use deploy queue status, deploy queue cancel or deploy queue run")
		}
	}
	if result.queueUnreachable && (result.command != "deploy" || result.subcommand != "apply") {
		return options{}, errors.New("--queue-unreachable is only valid with deploy apply")
	}
	if result.command == "deploy" && !recovering && !queueing && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("deploy requires the plan or apply subcommand")
	}
	if result.command == "controller" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("controller requires the plan or apply subcommand")
	}
	if result.command == "update" && result.subcommand != "check" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("update requires the check, plan, or apply subcommand")
	}
	if result.command == "package-base" && result.subcommand != "status" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("package-base requires status, plan or apply")
	}
	if result.allowUnverified && (result.command != "package-base" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--allow-unverified is only valid with package-base plan/apply")
	}
	if result.command == "package-base" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("package-base apply requires --expect from a reviewed plan")
	}
	if result.target != "" && ((result.command != "update" && result.command != "package-base") || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--target is only valid with update plan or update apply")
	}
	if (result.allowPrerelease || result.allowDowngrade) && (result.command != "update" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("update policy flags are only valid with update plan or update apply")
	}
	if result.command == "update" && (result.subcommand == "plan" || result.subcommand == "apply") && result.target == "" {
		return options{}, fmt.Errorf("update %s requires --target", result.subcommand)
	}
	if result.command == "software" && result.subcommand != "catalog" && result.subcommand != "search" && result.subcommand != "presets" && result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "preset-plan" && result.subcommand != "preset-apply" {
		return options{}, errors.New("software requires catalog, search, presets, plan, apply, preset plan, or preset apply")
	}
	if result.command == "software" && result.subcommand == "search" && result.softwareQuery == "" {
		return options{}, errors.New("software search requires --query")
	}
	if result.command == "software" && (result.subcommand == "plan" || result.subcommand == "apply") && (result.softwarePackage == "" || result.softwareScope == "") {
		return options{}, fmt.Errorf("software %s requires --package and --scope", result.subcommand)
	}
	if result.command == "software" && (result.subcommand == "plan" || result.subcommand == "apply") {
		if _, err := parseSoftwareScope(result.softwareScope); err != nil {
			return options{}, err
		}
	}
	if result.command == "software" && (result.subcommand == "preset-plan" || result.subcommand == "preset-apply") && (result.softwarePreset == "" || result.softwareScope == "") {
		return options{}, fmt.Errorf("software %s requires --preset and --scope", strings.ReplaceAll(result.subcommand, "-", " "))
	}
	if result.command == "software" && (result.subcommand == "preset-plan" || result.subcommand == "preset-apply") {
		if _, err := parseSoftwareScope(result.softwareScope); err != nil {
			return options{}, err
		}
	}
	if (result.softwarePackage != "" || result.remove) && (result.command != "software" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--package and --remove are only valid with software plan or apply")
	}
	if result.softwareScope != "" && (result.command != "software" || (result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "preset-plan" && result.subcommand != "preset-apply")) {
		return options{}, errors.New("--scope is only valid with software plan/apply or software preset plan/apply")
	}
	if (result.softwarePreset != "" || result.softwareExclude != "") && (result.command != "software" || (result.subcommand != "preset-plan" && result.subcommand != "preset-apply")) {
		return options{}, errors.New("--preset and --exclude are only valid with software preset plan or apply")
	}
	if result.softwareQuery != "" && (result.command != "software" || result.subcommand != "search") {
		return options{}, errors.New("--query is only valid with software search")
	}
	if result.command == "software" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("software apply requires --expect from software plan")
	}
	if result.command == "software" && result.subcommand == "preset-apply" && result.expect == "" {
		return options{}, errors.New("software preset apply requires --expect from software preset plan")
	}
	if (result.command == "shutdown" || result.command == "restart") && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, fmt.Errorf("%s requires the plan or apply subcommand", result.command)
	}
	if (result.command == "shutdown" || result.command == "restart") && result.on == "" {
		return options{}, fmt.Errorf("%s %s requires --on", result.command, result.subcommand)
	}
	if (result.command == "shutdown" || result.command == "restart") && result.subcommand == "apply" && result.expect == "" {
		return options{}, fmt.Errorf("%s apply requires --expect from %s plan", result.command, result.command)
	}
	if result.command == "lock" {
		// --action is parsed once; lock reads it as lock or unlock.
		result.lockAction, result.internetAction = domain.LockAction(result.internetAction), ""
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" || !result.lockAction.Valid() {
			return options{}, errors.New("lock requires plan/apply, --on and --action lock/unlock")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("lock apply requires --expect from lock plan")
		}
	}
	if result.command == "send" {
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" || result.file == "" {
			return options{}, errors.New("send requires plan/apply, --file and --on")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("send apply requires --expect from send plan")
		}
	}
	if result.internetAction != "" && result.command != "internet" {
		return options{}, errors.New("--action is only valid with internet or lock")
	}
	if result.command == "internet" {
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" || !result.internetAction.Valid() {
			return options{}, errors.New("internet requires plan/apply, --on and --action block/unblock")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("internet apply requires --expect from internet plan")
		}
	}
	if result.acknowledgeUnknown && result.command != "shutdown" && result.command != "restart" {
		return options{}, errors.New("--acknowledge-unknown-sessions is only valid with shutdown or restart")
	}
	if result.command == "services" && result.subcommand != "" && result.subcommand != "restart" {
		return options{}, errors.New("services accepts only the restart subcommand")
	}
	if result.command == "services" && result.subcommand == "restart" && result.service == "" {
		return options{}, errors.New("services restart requires cache")
	}
	if result.command == "backup" {
		switch {
		case result.subcommand != "create" && result.subcommand != "verify" && result.subcommand != "restore":
			return options{}, errors.New("backup requires create, verify or restore")
		case result.subcommand == "create" && result.backupTarget == "":
			return options{}, errors.New("backup create requires --to <directory>")
		case result.subcommand == "create" && result.backupFile != "":
			return options{}, errors.New("backup create does not take a file")
		case result.subcommand != "create" && result.backupFile == "":
			return options{}, fmt.Errorf("backup %s requires the backup file", result.subcommand)
		case result.subcommand == "verify" && result.backupTarget != "":
			return options{}, errors.New("--to is only valid with backup create or restore")
		case result.subcommand == "restore" && result.backupTarget == "":
			return options{}, errors.New("backup restore requires --to <empty directory>")
		}
	} else if result.backupTarget != "" || result.passphraseFile != "" {
		return options{}, errors.New("--to and --passphrase-file are only valid with backup")
	}
	if result.command == "recovery" && result.subcommand != "status" {
		return options{}, errors.New("recovery accepts only the status subcommand")
	}
	if result.command == "logs" && result.subcommand != "" && result.subcommand != "show" {
		return options{}, errors.New("logs accepts only the show subcommand")
	}
	if result.command == "logs" && result.subcommand == "show" && result.logID == "" {
		return options{}, errors.New("logs show requires an operation log ID")
	}
	gitPaths := result.subcommand == "commit-plan" || result.subcommand == "commit-apply" || result.subcommand == "discard-plan" || result.subcommand == "discard-apply"
	if result.paths != "" && (result.command != "git" || !gitPaths) {
		return options{}, errors.New("--paths is only valid with git commit or git discard plan/apply")
	}
	if result.command == "git" && result.subcommand != "review" && !gitPaths {
		return options{}, errors.New("git requires review, commit plan/apply, or discard plan/apply")
	}
	if result.command == "git" && gitPaths && result.paths == "" {
		return options{}, fmt.Errorf("git %s requires --paths", strings.ReplaceAll(result.subcommand, "-", " "))
	}
	if result.command == "deploy" && !recovering && !queueing && result.on == "" {
		return options{}, fmt.Errorf("deploy %s requires --on", result.subcommand)
	}
	if result.command == "config" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("config apply requires --expect from config plan")
	}
	if result.command == "deploy" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("deploy apply requires --expect from deploy plan")
	}
	if result.command == "controller" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("controller apply requires --expect from controller plan")
	}
	if result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply") && result.expect == "" {
		return options{}, errors.New("git commit apply requires --expect from git commit plan")
	}
	if result.command == "update" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("update apply requires --expect from update plan")
	}
	if result.command == "setup" && result.subcommand == "" {
		result.subcommand = "configure"
		result.guided = true
	}
	if result.command == "setup" && result.subcommand != "status" && result.subcommand != "keys" && result.subcommand != "configure" && result.subcommand != "install-secrets" && result.subcommand != "apply" {
		return options{}, errors.New("setup requires configure, status, keys, install-secrets, or apply")
	}
	if result.command == "pxe" && result.subcommand != "prepare" && result.subcommand != "start" && result.subcommand != "stop" && result.subcommand != "recover" {
		return options{}, errors.New("pxe requires prepare, start, stop, or recover")
	}
	if result.command == "setup" && result.subcommand == "configure" && result.json {
		return options{}, errors.New("--json is not valid with interactive setup configure")
	}
	if result.command == "install" {
		valid := map[string]bool{
			"usb-prepare": true, "usb-start": true, "usb-status": true, "usb-reconcile": true,
			"usb-reboot": true, "usb-verify": true, "usb-cancel": true, "usb-close": true,
		}
		if !valid[result.subcommand] {
			return options{}, errors.New("install requires usb prepare, start, status, reconcile, reboot, verify, cancel, or close")
		}
		if (result.subcommand == "usb-prepare" || result.subcommand == "usb-start") != (result.host != "") {
			return options{}, errors.New("install usb prepare/start require --host; other USB operations do not accept it")
		}
		if result.host != "" && !domain.ValidRemoteHostName(result.host) {
			return options{}, errors.New("--host must be a canonical pcNN inventory name")
		}
		needsID := result.subcommand != "usb-prepare" && result.subcommand != "usb-start"
		// status without --id shows the unfinished operation, if any.
		if needsID != (result.operationID != "") && !(result.subcommand == "usb-status" && result.operationID == "") {
			return options{}, errors.New("install usb reconcile/reboot/verify/cancel/close require --id; prepare/start do not accept it")
		}
		if result.operationID != "" && !remoteInstallOperationIDPattern.MatchString(result.operationID) {
			return options{}, errors.New("--id must contain exactly 32 lowercase hexadecimal characters")
		}
		if result.json && (result.subcommand == "usb-start" || result.subcommand == "usb-reboot" || result.subcommand == "usb-close") {
			return options{}, errors.New("--json is not valid with interactive install usb start, reboot, or close")
		}
	}
	if result.host != "" && result.command != "install" {
		return options{}, errors.New("--host is only valid with install usb prepare or start")
	}
	if result.operationID != "" && result.command != "install" {
		return options{}, errors.New("--id is only valid with install usb status, reconcile, reboot, verify, cancel, or close")
	}
	return result, nil
}
