package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

var nixoriumVersion = "development"

type options struct {
	internetAction       domain.InternetAction
	lockAction           domain.LockAction
	command              string
	subcommand           string
	repository           string
	file                 string
	homeFilesSource      string
	homeFilesDestination string
	homeFilesReplace     bool
	expect               string
	on                   string
	service              string
	logID                string
	host                 string
	operationID          string
	paths                string
	target               string
	softwarePackage      string
	softwarePreset       string
	softwareExclude      string
	softwareQuery        string
	softwareScope        string
	json                 bool
	full                 bool
	help                 bool
	guided               bool
	verifyOnly           bool
	yes                  bool
	allowPrerelease      bool
	allowDowngrade       bool
	allowUnverified      bool
	acknowledgeUnknown   bool
	// acknowledgeUnreachable confirms that computers that could not be
	// checked during a recovery review were inspected at their console.
	acknowledgeUnreachable bool
	queueUnreachable       bool
	// network is the new laboratory network of network migrate plan/start.
	network        string
	backupTarget   string
	backupRemote   string
	backupBranch   string
	backupFile     string
	passphraseFile string
	remove         bool
	// collectInto is the folder collect plan/apply saves desktops into
	// instead of ~/Nixorium/Collected.
	collectInto string
	// schedule holds the times and weekdays of the schedule command.
	schedule scheduleArguments
}

func main() {
	sanitizeRuntimeDir(os.Getenv, os.Setenv, os.Unsetenv, pathOwner, os.Getuid())
	if len(os.Args) == 3 && os.Args[1] == classroomBrowserCommand {
		os.Exit(runClassroomBrowser(os.Args[2]))
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run adds the next step after a failed command whose output named a known
// blocker. JSON output carries the same steps in its issues instead.
func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	recorder := &nextStepRecorder{}
	commandStdout := recordWriter(stdout, recorder)
	for _, argument := range arguments {
		if argument == "--json" {
			commandStdout = stdout
		}
	}
	code := runCommand(ctx, arguments, commandStdout, recordWriter(stderr, recorder))
	if code != 0 {
		recorder.report(stderr)
	}
	return code
}

func runCommand(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	// Works on a directory, never on the configured deployment (ADR 0034).
	if len(arguments) >= 2 && arguments[0] == "deployment" && arguments[1] == "migrate" {
		return runDeploymentMigrate(arguments[2:], stdout, stderr)
	}
	options, err := parseArguments(arguments)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		usage(stderr)
		return 2
	}
	if options.help {
		usage(stdout)
		return 0
	}
	if options.command == "telemetry" {
		return runTelemetryCommand(ctx, options, stdout, stderr)
	}
	// Switching on goes through the classroom service unless the
	// deployment is readable (the administrator).
	if options.command == "wake" {
		return runWakeCommand(ctx, options, stdout, stderr)
	}
	// The schedule is read and set through the classroom service; the
	// controller's timer runs it as the service user.
	if options.command == "schedule" {
		return runScheduleCommand(ctx, options, stdout, stderr)
	}
	// The classroom view needs only the classroom service: the teacher
	// cannot read the deployment repository.
	if options.command == "classroom-view" {
		message, err := openClassroomView(ctx)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		fmt.Fprintln(stdout, message)
		return 0
	}
	// Sending and collecting files need only the classroom service, like
	// the view: the teacher cannot read the deployment repository.
	if options.command == "send" {
		return runSendCommand(ctx, options, stdout, stderr)
	}
	if options.command == "collect" {
		return runCollectCommand(ctx, options, stdout, stderr)
	}
	repository := options.repository
	resolvedRepository, resolveErr := resolveRepository(options.repository)
	if resolveErr == nil {
		repository = resolvedRepository
	} else {
		if options.command == "" && options.repository == "" {
			if handled, code := tryRunClassroomDashboard(ctx, stderr); handled {
				return code
			}
			if message, teacher := classroomUnavailable(); teacher {
				fmt.Fprintln(stderr, message)
				return 1
			}
		}
		if commandRequiresRepository(options) {
			fmt.Fprintln(stderr, "Error:", resolveErr)
			return 1
		}
	}

	local := adapters.Local{}
	inspector := app.NewInspector(local)
	switch options.command {
	case "":
		if options.json {
			report, inspectErr := inspector.Status(ctx, repository)
			if inspectErr != nil {
				fmt.Fprintln(stderr, "Error:", inspectErr)
				return 1
			}
			if jsonErr := presentation.JSON(stdout, report); jsonErr != nil {
				fmt.Fprintln(stderr, "Error:", jsonErr)
				return 1
			}
			return 0
		}
		return runDashboardProgram(ctx, repository, false, stderr)
	case "recovery":
		report := app.NewRecoveryInspector(local).Observe(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.RecoveryText(stdout, report)
		}
	case "status":
		report, inspectErr := inspector.Status(ctx, repository)
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.StatusText(stdout, report)
		}
	case "hosts":
		report, inspectErr := inspector.Hosts(ctx, repository)
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.HostsText(stdout, report)
		}
	case "host-key":
		return runHostTrustCommand(ctx, repository, options, stdout, stderr)
	case "network":
		return runNetworkCommand(ctx, repository, options, stdout, stderr)
	case "deploy":
		if strings.HasPrefix(options.subcommand, "recover") {
			return runDeploymentRecoveryCommand(ctx, repository, options, stdout, stderr)
		}
		return runDeploymentCommand(ctx, repository, options, stdout, stderr)
	case "template-reset":
		return runTemplateResetRecoveryCommand(ctx, repository, options, stdout, stderr)
	case "backup":
		if options.subcommand == "plan" || options.subcommand == "publish" || options.subcommand == "clone" {
			return runGitBackupCommand(ctx, repository, options, stdout, stderr)
		}
		return runBackupCommand(ctx, repository, options, stdout, stderr)
	case "controller":
		return runControllerCommand(ctx, repository, options, stdout, stderr)
	case "services":
		return runServicesCommand(ctx, repository, options, stdout, stderr)
	case "logs":
		return runLogsCommand(options, stdout, stderr)
	case "support":
		return runSupportCommand(ctx, repository, options, stdout, stderr)
	case "git":
		return runGitCommand(ctx, repository, options, stdout, stderr)
	case "package-base":
		return runPackageBaseCommand(ctx, repository, options, stdout, stderr)
	case "update":
		return runUpdateCommand(ctx, repository, options, stdout, stderr)
	case "software":
		return runSoftwareCommand(ctx, repository, options, stdout, stderr)
	case "workspace":
		return runWorkspaceCommand(ctx, repository, options, stdout, stderr)
	case "home-files":
		return runHomeFilesCommand(ctx, repository, options, stdout, stderr)
	case "internet":
		return runInternetCommand(ctx, repository, options, stdout, stderr)
	case "lock":
		return runLockCommand(ctx, repository, options, stdout, stderr)
	case "cleanup":
		return runCleanupCommand(ctx, repository, options, stdout, stderr)
	case "shutdown", "restart":
		return runShutdownCommand(ctx, repository, options, stdout, stderr)
	case "install":
		return runInstallCommand(ctx, repository, options, stdout, stderr)
	case "doctor":
		report, inspectErr := inspector.Doctor(ctx, repository, app.DoctorOptions{Full: options.full})
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.DoctorText(stdout, report)
		}
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		if report.HasErrors() {
			return 1
		}
	case "config":
		if options.subcommand == "source" {
			return runEvaluationSource(ctx, repository, options, stdout, stderr)
		}
		return runConfigCommand(ctx, repository, options, stdout, stderr)
	case "setup":
		return runSetupCommand(ctx, repository, options, stdout, stderr)
	case "pxe":
		return runPXECommand(ctx, repository, options, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Error: unknown command %q\n", options.command)
		usage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

func runDashboardProgram(ctx context.Context, repository string, setupMode bool, stderr io.Writer) int {
	deps := newDashboardDeps(ctx, repository, setupMode)
	local := deps.local
	var actions presentation.DashboardActions
	wireAssistantActions(&actions, deps)
	wireOverviewActions(&actions, deps)
	wireSettingsActions(&actions, deps)
	wireSoftwareActions(&actions, deps)
	wireWorkspaceActions(&actions, deps)
	wireClassroomActions(&actions, deps)
	wireDeploymentActions(&actions, deps)
	wireInstallationActions(&actions, deps)
	wireUpdatesActions(&actions, deps)
	wireBackupActions(&actions, deps)
	// Locking screens needs the classroom agents on the clients.
	if !classroomViewSetting(local, repository) {
		actions.PlanLock, actions.ApplyLock = nil, nil
		actions.PlanShare, actions.ApplyShare = nil, nil
	}
	actions.InstalledInterface = func() (presentation.InterfaceReload, bool) {
		return detectInterfaceReload(os.Executable, installedInterfacePath)
	}
	notice := os.Getenv(presentation.ReopenNoticeEnvironment)
	_ = os.Unsetenv(presentation.ReopenNoticeEnvironment)
	reload, outcome, tuiErr := presentation.RunLoadingDashboardReload(actions, setupMode, notice)
	if tuiErr != nil {
		fmt.Fprintln(stderr, "Error:", tuiErr)
		return 1
	}
	if reload {
		// The terminal is restored: bubbletea released the alternate screen
		// and raw mode when Run returned.
		environment := append(os.Environ(), presentation.ReopenNoticeEnvironment+"="+outcome)
		return reopenInterface(installedInterfacePath, os.Args[1:], environment, stderr)
	}
	return 0
}

func writePXEPreparationActivity(writer io.Writer) {
	fmt.Fprintln(writer, "Preparing PXE artifacts and client closures; verbose output: journalctl -fu nixorium-prepare-pxe.service")
}

func writeControllerApplyActivity(writer io.Writer, unit string) {
	fmt.Fprintf(writer, "Building and activating the reviewed controller; verbose output: journalctl -fu %s\n", unit)
}

func runWithManagedProgress[T any](action func() T, load func() (domain.OperationProgress, error), writer io.Writer) T {
	started := time.Now().UTC()
	results := make(chan T, 1)
	go func() {
		results <- action()
	}()
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	var lastUpdate time.Time
	render := func() {
		progress, err := load()
		if err != nil || progress.StartedAt.Before(started) || !progress.UpdatedAt.After(lastUpdate) {
			return
		}
		lastUpdate = progress.UpdatedAt
		activity := "Waiting for the next managed activity"
		if len(progress.Recent) > 0 {
			activity = progress.Recent[len(progress.Recent)-1]
		}
		counter := ""
		if progress.Total > 0 {
			counter = fmt.Sprintf(" (%d/%d)", progress.Current, progress.Total)
		}
		fmt.Fprintf(writer, "Progress [%s]%s: %s\n", progress.Phase, counter, activity)
	}
	for {
		select {
		case result := <-results:
			render()
			return result
		case <-ticker.C:
			render()
		}
	}
}

func commandRequiresRepository(options options) bool {
	return options.command != "logs" && (options.command != "backup" || (options.subcommand == "create" || options.subcommand == "plan" || options.subcommand == "publish")) && (options.command != "pxe" || (options.subcommand != "stop" && options.subcommand != "recover"))
}

func resolveRepository(explicit string) (string, error) {
	if explicit != "" {
		return requireDeploymentRoot(explicit)
	}
	if configured := os.Getenv("NIXORIUM_REPO"); configured != "" {
		return requireDeploymentRoot(configured)
	}
	if current, err := os.Getwd(); err == nil && isDeploymentRoot(current) {
		return filepath.Abs(current)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, "nixorium-deployment")
		if isDeploymentRoot(candidate) {
			return filepath.Abs(candidate)
		}
	}
	return "", errors.New("deployment repository not found; run from its root or pass --repo <path>")
}

func requireDeploymentRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve deployment repository: %w", err)
	}
	if !isDeploymentRoot(absolute) {
		return "", fmt.Errorf("%s is not a deployment repository (missing flake.nix)", absolute)
	}
	return absolute, nil
}

func isDeploymentRoot(path string) bool {
	info, err := os.Stat(filepath.Join(path, "flake.nix"))
	return err == nil && info.Mode().IsRegular()
}

func readCandidateSettings(path string) ([]byte, error) {
	const maximumBytes = int64(1024 * 1024)
	fileDescriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fileDescriptor), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("candidate must be a regular file and not a symlink")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximumBytes {
		return nil, errors.New("candidate settings file is unexpectedly large")
	}
	return data, nil
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage: nixorium [status|hosts|doctor|install usb prepare|install usb start|install usb status|install usb reconcile|install usb reboot|install usb verify|install usb cancel|install usb close|software catalog|software search|software presets|software plan|software apply|software preset plan|software preset apply|shutdown plan|shutdown apply|restart plan|restart apply|internet plan|internet apply|lock plan|lock apply|send plan|send apply|wake|schedule show|schedule set|schedule clear|schedule run|classroom-view|cleanup plan|cleanup apply|recovery status|backup create|backup verify|backup restore|deploy recover plan|deploy recover apply|template-reset recover plan|template-reset recover apply|deployment migrate|deploy plan|deploy apply|controller plan|controller apply|services|services restart cache|logs|logs show|git review|git commit plan|git commit apply|git discard plan|git discard apply|git undo plan|git undo apply|update check|update plan|update apply|package-base status|package-base plan|package-base apply|workspace plan|workspace apply|home-files import plan|home-files plan|home-files apply|host-key plan|host-key apply|network migrate plan|network migrate start|network migrate status|network migrate finish|network migrate undo|support preview|support export|config validate|config plan|config apply|setup|setup configure|setup status|setup keys|setup install-secrets|setup apply|pxe prepare|pxe start|pxe stop|pxe recover] [options]")
	fmt.Fprintln(writer, "       workspace plan --file <candidate.json> previews student preferences without saving")
	fmt.Fprintln(writer, "       home-files import plan --source <local-file-or-folder> --destination <relative-home-path> [--replace] [--json] captures materials for review")
	fmt.Fprintln(writer, "       home-files plan --file <draft-or-profile.json> [--json] reviews shared student initial files")
	fmt.Fprintln(writer, "       home-files apply --file <draft-or-profile.json> --expect <review-token> [--yes] saves only the manifest and captured copies")
	fmt.Fprintln(writer, "       host-key plan --host <pcNN> [--json] reviews changed SSH trust after reinstall")
	fmt.Fprintln(writer, "       host-key apply --host <pcNN> --expect <review-token> [--yes] rotates only the reviewed key")
	fmt.Fprintln(writer, "       network migrate plan --to <network/prefix> [--acknowledge-unreachable] [--json] reviews a change of laboratory network")
	fmt.Fprintln(writer, "       network migrate start --to <network/prefix> --expect <review-token> [--acknowledge-unreachable] [--yes] saves it; then apply to the controller and update computers")
	fmt.Fprintln(writer, "       network migrate status [--json] shows which computers moved and the next step")
	fmt.Fprintln(writer, "       network migrate finish|undo [--expect <review-token> [--yes]] [--json] reviews, then saves the end or the undo of the change")
	fmt.Fprintln(writer, "       telemetry status|preview [--json], telemetry enable|disable: optional controller adoption statistics")
	fmt.Fprintln(writer, "       support preview [--json] shows the filtered diagnostic payload without saving")
	fmt.Fprintln(writer, "       support export reviews and confirms a private local file; no upload or build")
	fmt.Fprintln(writer, "       workspace apply --file <candidate.json> --expect <review-token> [--yes] saves only the profile JSON")
	fmt.Fprintln(writer, "       install usb prepare --host <pcNN> builds only pinned target-independent artifacts")
	fmt.Fprintln(writer, "       install usb start --host <pcNN> interactively verifies the live ISO, disk, and destructive review")
	fmt.Fprintln(writer, "       install usb status [--id <operation-id>] shows the given or the unfinished operation")
	fmt.Fprintln(writer, "       install usb {reconcile|reboot|verify|cancel|close} --id <operation-id>")
	fmt.Fprintln(writer, "       software catalog")
	fmt.Fprintln(writer, "       software search --query <package-name>")
	fmt.Fprintln(writer, "       software presets")
	fmt.Fprintln(writer, "       software preset plan --preset <id> --scope <scope> [--exclude <id[,id...]>]")
	fmt.Fprintln(writer, "       software preset apply --preset <id> --scope <scope> [--exclude <ids>] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       software plan --package <id> --scope <shared|controller|all-clients|group:NAME|clients:pcNN,...> [--remove]")
	fmt.Fprintln(writer, "       software apply --package <id> --scope <scope> [--remove] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       internet plan --on <clients|@lab> --action <block|unblock>")
	fmt.Fprintln(writer, "       internet apply --on <clients|@lab> --action <block|unblock> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       lock plan --on <clients|@lab> --action <lock|unlock> reviews locking students' screens (classroom view)")
	fmt.Fprintln(writer, "       lock apply --on <clients|@lab> --action <lock|unlock> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       deploy apply --on <clients|@lab> --expect <revision> --queue-unreachable  also queues computers that are off")
	fmt.Fprintln(writer, "       deploy queue status|run  shows or applies queued client updates; deploy queue cancel --on <clients|@all>")
	fmt.Fprintln(writer, "       send plan --file <file-or-folder> --on <clients|@lab> reviews copying it to students' desktops (classroom view)")
	fmt.Fprintln(writer, "       send apply --file <file-or-folder> --on <clients|@lab> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       collect plan --on <clients|@lab> [--into <folder>] reviews copying students' desktops into ~/Nixorium/Collected or the folder (classroom view)")
	fmt.Fprintln(writer, "       collect apply --on <clients|@lab> [--into <folder>] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       classroom-view opens the classroom view page (teacher and administrator)")
	fmt.Fprintln(writer, "       wake --on <clients|@lab> [--json] sends Wake-on-LAN packets to the selected computers that are off (teacher and administrator)")
	fmt.Fprintln(writer, "       schedule show|clear [--json] shows or clears the daily switch-on and shut-down of the computers (teacher and administrator)")
	fmt.Fprintln(writer, "       schedule set [--on <HH:MM>] [--off <HH:MM>] [--days <mon-fri|all|mon,wed,fri>] [--json] sets it, replacing the old one; --days is mon-fri when left out")
	fmt.Fprintln(writer, "       schedule run [--repo <deployment>] [--json] does what is due this minute (started by nixorium-classroom-schedule.timer)")
	fmt.Fprintln(writer, "       recovery status lists what blocks operations and the next step for each")
	fmt.Fprintln(writer, "       backup plan --remote <ssh-url> [--branch <branch>] [--repo <deployment>]")
	fmt.Fprintln(writer, "       backup publish --remote <ssh-url> [--branch <branch>] --expect <token> --yes [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup clone [--remote <ssh-url> [--branch <branch>] --to <new-directory> --yes] [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup create --to <directory> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup verify <file> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup restore <file> --to <empty-directory> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       deploy recover plan [--acknowledge-unreachable]  reviews an interrupted client update; the flag continues without the computers that do not answer")
	fmt.Fprintln(writer, "       deploy recover apply --expect <review-token> [--acknowledge-unreachable] [--yes]  marks it as reviewed after a yes/no question")
	fmt.Fprintln(writer, "       deployment migrate --root <deployment> [--contract N] [--json]  applies this release's changes to deployment files in that directory; review them with git diff")
	fmt.Fprintln(writer, "       template-reset recover plan")
	fmt.Fprintln(writer, "       template-reset recover apply --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       cleanup plan --on <controller,pcNN,...|@lab>")
	fmt.Fprintln(writer, "       cleanup apply --on <targets> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       shutdown plan --on <pcNN[,pcNN...]|@lab> [--acknowledge-unknown-sessions]")
	fmt.Fprintln(writer, "       shutdown apply --on <targets> [--acknowledge-unknown-sessions] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       restart plan --on <pcNN[,pcNN...]|@lab> [--acknowledge-unknown-sessions]")
	fmt.Fprintln(writer, "       restart apply --on <targets> [--acknowledge-unknown-sessions] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       deploy plan --on <pcNN[,pcNN...]|@lab>")
	fmt.Fprintln(writer, "       deploy apply --on <targets> --expect <git-revision> [--yes]")
	fmt.Fprintln(writer, "       controller plan")
	fmt.Fprintln(writer, "       controller apply --expect <git-revision> [--yes]")
	fmt.Fprintln(writer, "       services restart cache [--yes]")
	fmt.Fprintln(writer, "       logs show <operation-log-id>")
	fmt.Fprintln(writer, "       git review shows bounded staged and unstaged changes without mutating Git")
	fmt.Fprintln(writer, "       git commit plan --paths <path[,path...]> creates an isolated proposal")
	fmt.Fprintln(writer, "       git commit apply --paths <paths> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       git discard plan --paths <paths> restores files to the last commit after review")
	fmt.Fprintln(writer, "       git discard apply --paths <paths> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       git undo plan [--json] reviews undoing the last saved change")
	fmt.Fprintln(writer, "       git undo apply --expect <review-token> [--yes] [--json] saves the undo; then apply to the controller and update computers")
	fmt.Fprintln(writer, "       update check explicitly queries the configured public upstream")
	fmt.Fprintln(writer, "       update plan --target <vMAJOR.MINOR.PATCH[-PRERELEASE]>")
	fmt.Fprintln(writer, "       update apply --target <release> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       package-base status shows the effective deployment-owned system/package pin")
	fmt.Fprintln(writer, "       package-base plan [--target <nixos-YY.MM>] [--allow-unverified]")
	fmt.Fprintln(writer, "       package-base apply [--target <nixos-YY.MM>] [--allow-unverified] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       channel changes require --allow-unverified; apply saves files only, never activates machines")
	fmt.Fprintln(writer, "       config source [--revision <git-revision>] prepares local Nix input with account hashes")
	fmt.Fprintln(writer, "       config plan --file <candidate.json>")
	fmt.Fprintln(writer, "       config apply --file <candidate.json> --expect <sha256:fingerprint>")
	fmt.Fprintln(writer, "       setup keys --verify-only performs read-only correspondence checks")
	fmt.Fprintln(writer, "       nixorium opens the read-only management dashboard")
	fmt.Fprintln(writer, "       doctor --full also builds the controller configuration")
}

func runSetupConfigure(ctx context.Context, repository string, stdout, stderr io.Writer, reconcileKeys bool) int {
	local := adapters.Local{}
	data, err := local.ReadSettings(repository)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	settings, issues := domain.DecodeLabSettings(data)
	if len(issues) > 0 {
		fmt.Fprintf(stderr, "Error: current settings are invalid: %s: %s\n", issues[0].Field, issues[0].Message)
		return 1
	}
	if settings.Lab.MasterDHCPIP == domain.MasterDHCPPlaceholder {
		staticAddress, _ := domain.ControllerStaticAddress(settings.Lab)
		settings = applyDetectedNetworkDefaults(settings, local.DetectNetworkDefaults(staticAddress))
	}

	candidate, accepted, err := presentation.RunSettingsWizard(settings)
	if err != nil {
		fmt.Fprintln(stderr, "Error: configuration wizard:", err)
		return 1
	}
	if !accepted {
		fmt.Fprintln(stdout, "Configuration cancelled; no files changed.")
		return 0
	}

	secretReader := presentation.TerminalSecretReader{Input: os.Stdin, Output: stdout}
	if err := collectSetupCredentials(ctx, secretReader, local, stdout, &candidate); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	candidateData, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		fmt.Fprintln(stderr, "Error: prepare candidate:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Validating the complete candidate through Nix...")
	settingsManager := app.NewSettingsManager(local)
	plan := settingsManager.Plan(ctx, repository, candidateData)
	if plan.HasErrors() {
		presentation.ConfigPlanText(stderr, plan)
		return 1
	}
	gitState, err := local.GitState(ctx, repository)
	if err != nil {
		fmt.Fprintln(stderr, "Error: inspect Git worktree:", err)
		return 1
	}
	approved, err := presentation.RunConfigReview(plan, gitState)
	if err != nil {
		fmt.Fprintln(stderr, "Error: configuration review:", err)
		return 1
	}
	if !approved {
		fmt.Fprintln(stdout, "Configuration cancelled; no files changed.")
		return 0
	}
	report := settingsManager.Apply(ctx, repository, candidateData, plan.BaseFingerprint)
	recordOperationWarning(stderr, report)
	presentation.ConfigApplyText(stdout, report)
	if report.HasErrors() {
		return 1
	}
	if reconcileKeys {
		keyReport, keyErr := app.NewSetupManager(local).ReconcileKeys(ctx, repository)
		recordOperationWarning(stderr, keyReport)
		presentation.KeyReconcileText(stdout, keyReport)
		if keyErr != nil {
			fmt.Fprintln(stderr, "Error:", keyErr)
			return 1
		}
		actionReport := app.NewSystemActions(local).InstallSecrets(ctx)
		actionReport.Message = operationRecordMessage(actionReport.Message, actionReport)
		presentation.ActionText(stdout, actionReport)
		if actionReport.HasErrors() {
			fmt.Fprintln(stderr, "The settings and repository keys are intact; retry with `nixorium setup install-secrets` on the controller.")
			return 1
		}
		fmt.Fprintln(stdout, "Configuration and keys are ready. Opening the resumable first-run guide…")
	}
	return 0
}

func applyDetectedNetworkDefaults(settings domain.LabSettingsFile, detected domain.NetworkDefaults) domain.LabSettingsFile {
	if settings.Lab.MasterDHCPIP == domain.MasterDHCPPlaceholder && detected.DHCPAddress != "" {
		settings.Lab.MasterDHCPIP = detected.DHCPAddress
	}
	if detected.InterfaceName != "" {
		settings.Lab.ControllerInterfaceName = detected.InterfaceName
	}
	return settings
}

var remoteInstallOperationIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func setupStartsBeforeDashboardInspection(report domain.SetupReport) bool {
	switch report.CurrentStage {
	case domain.SetupStageInspectEnvironment, domain.SetupStageNetwork, domain.SetupStageIdentity, domain.SetupStageCredentials:
		return true
	default:
		return false
	}
}

func collectSetupCredentials(ctx context.Context, reader app.SecretReader, hasher app.PasswordHasher, output io.Writer, candidate *domain.LabSettingsFile) error {
	credentials := []struct {
		label string
		value *string
	}{
		{label: "Administrator password", value: &candidate.Lab.AdminPassword},
		{label: "Teacher password", value: &candidate.Lab.TeacherPassword},
		{label: "Student password", value: &candidate.Lab.StudentPassword},
	}
	pending := 0
	for _, credential := range credentials {
		if *credential.value == "" || *credential.value == domain.DefaultPasswordHash {
			pending++
		}
	}
	if pending == 0 {
		return nil
	}
	fmt.Fprintln(output, "Set account passwords. Each password must contain at least 8 bytes and must not use the public default.")
	fmt.Fprintln(output, "A short or mismatched password can be retried without restarting configuration.")
	candidate.Lab.CredentialsVersion++
	completed := 0
	for _, credential := range credentials {
		if *credential.value != "" && *credential.value != domain.DefaultPasswordHash {
			continue
		}
		for {
			hash, err := app.CollectNamedPasswordHash(ctx, reader, hasher, credential.label)
			if err == nil {
				*credential.value = hash
				break
			}
			if !app.IsPasswordInputError(err) {
				return fmt.Errorf("%s: %w", credential.label, err)
			}
			fmt.Fprintf(output, "Invalid %s: %v. Try again.\n", strings.ToLower(credential.label), err)
		}
		completed++
		fmt.Fprintf(output, "%s accepted (%d/%d).\n", credential.label, completed, pending)
	}
	return nil
}

func collectSettingsPassword(ctx context.Context, hasher app.PasswordHasher, input *os.File, output io.Writer, account string, settings domain.LabSettingsFile) (domain.LabSettingsFile, error) {
	reader := presentation.TerminalSecretReader{Input: input, Output: output}
	return collectSettingsPasswordWithReader(ctx, reader, hasher, output, account, settings)
}

func collectSettingsPasswordWithReader(ctx context.Context, reader app.SecretReader, hasher app.PasswordHasher, output io.Writer, account string, settings domain.LabSettingsFile) (domain.LabSettingsFile, error) {
	label := ""
	target := (*string)(nil)
	switch account {
	case "admin":
		label = "Administrator password"
		target = &settings.Lab.AdminPassword
	case "teacher":
		label = "Teacher password"
		target = &settings.Lab.TeacherPassword
	case "student":
		label = "Student password"
		target = &settings.Lab.StudentPassword
	default:
		return settings, fmt.Errorf("unsupported password account %q", account)
	}

	fmt.Fprintln(output, "Set one account password. It must contain at least 8 bytes and must not use the public default.")
	fmt.Fprintln(output, "A short or mismatched password can be retried without leaving this password step.")
	for {
		hash, err := app.CollectNamedPasswordHash(ctx, reader, hasher, label)
		if err == nil {
			*target = hash
			settings.Lab.CredentialsVersion++
			fmt.Fprintln(output, label+" accepted. Returning to settings review…")
			return settings, nil
		}
		if !app.IsPasswordInputError(err) {
			return settings, fmt.Errorf("%s: %w", label, err)
		}
		fmt.Fprintf(output, "Invalid %s: %v. Try again.\n", strings.ToLower(label), err)
	}
}

func operationRecordMessage(message string, outcome any) string {
	if err := app.RecordOperationOutcome(adapters.Local{}, outcome); err != nil {
		warning := "operation outcome was not recorded: " + err.Error()
		if message == "" {
			return warning
		}
		return message + "; " + warning
	}
	return message
}

func recordOperationWarning(writer io.Writer, outcome any) {
	if err := app.RecordOperationOutcome(adapters.Local{}, outcome); err != nil {
		fmt.Fprintln(writer, "Warning: operation outcome was not recorded:", err)
	}
}
