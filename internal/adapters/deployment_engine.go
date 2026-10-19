package adapters

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The client deployment engine (ADR 0031). The build evaluates and builds
// every selected computer's system once, from the reviewed source, and
// contacts nobody. The apply reuses exactly those store paths: it copies each
// closure to its computer, which substitutes from the controller's cache, and
// activates it in a transient systemd unit on the computer. The activation
// therefore survives a dropped connection and leaves a result the controller
// can read later. Each computer's progress is written into the pending
// record before anything changes on it.

const (
	clientActivationUnit      = "nixorium-activate.service"
	clientActivationDirectory = "/var/lib/nixorium/activation"
	// clientBuildBatch bounds how many computers one evaluation process
	// holds in memory (about 750 MB each).
	clientBuildBatch            = 4
	clientCopyParallelism       = 4
	clientActivationParallelism = 10
	clientCopyTimeout           = 45 * time.Minute
	clientCommandTimeout        = 30 * time.Second
	// Exit status of the start command when another activation runs.
	clientActivationBusyStatus = 75
)

// Polling bounds, variables so that tests can shorten them.
var (
	clientActivationTimeout    = 20 * time.Minute
	clientActivationPoll       = 5 * time.Second
	clientActivationStartGrace = 30 * time.Second
)

var clientHostName = regexp.MustCompile(`^pc[0-9]{1,4}$`)

// clientBuildSource prepares the reviewed source of a build; tests replace it.
var clientBuildSource = func(ctx context.Context, repository, revision string) (string, error) {
	return (Local{}).EvaluationSource(ctx, repository, revision)
}

// builtClientSystems keeps, for the running process, the systems built for
// a reviewed revision so that the apply never evaluates again.
var builtClientSystems = struct {
	sync.Mutex
	systems map[string]map[string]string
}{systems: map[string]map[string]string{}}

func builtSystemsKey(repository, revision string) string {
	return repository + "\x00" + revision
}

// buildClientSystems builds the system of every selected computer and keeps
// the resulting store paths for the apply of this revision.
func buildClientSystems(ctx context.Context, plan domain.DeploymentPlanReport, output io.Writer) error {
	names := make([]string, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		if !clientHostName.MatchString(target.Name) {
			return fmt.Errorf("invalid computer name %q", target.Name)
		}
		names = append(names, target.Name)
	}
	if len(names) == 0 {
		return errors.New("no computer was selected")
	}
	source, err := clientBuildSource(ctx, plan.Repository, plan.Revision)
	if err != nil {
		return err
	}
	systems := map[string]string{}
	for start := 0; start < len(names); start += clientBuildBatch {
		batch := names[start:min(len(names), start+clientBuildBatch)]
		fmt.Fprintf(output, "Building %s\n", strings.Join(batch, ", "))
		built, err := buildClientBatch(ctx, source, batch, output)
		if err != nil {
			return err
		}
		for name, path := range built {
			systems[name] = path
		}
	}
	builtClientSystems.Lock()
	builtClientSystems.systems[builtSystemsKey(plan.Repository, plan.Revision)] = systems
	builtClientSystems.Unlock()
	return nil
}

func buildClientBatch(ctx context.Context, source string, names []string, output io.Writer) (map[string]string, error) {
	arguments := []string{"--extra-experimental-features", "nix-command flakes", "build", "--no-link", "--json", "--no-write-lock-file"}
	for _, name := range names {
		arguments = append(arguments, source+"#nixosConfigurations."+name+".config.system.build.toplevel")
	}
	command := exec.CommandContext(ctx, "nix", arguments...)
	configureProcessGroup(command)
	stdout := &boundedCommandBuffer{limit: 1024 * 1024}
	command.Stdout, command.Stderr = stdout, output
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("build %s: %w", strings.Join(names, ", "), err)
	}
	if stdout.truncated {
		return nil, errors.New("the build report is too large")
	}
	var results []struct {
		Outputs map[string]string `json:"outputs"`
	}
	if err := json.Unmarshal(stdout.buffer.Bytes(), &results); err != nil || len(results) != len(names) {
		return nil, errors.New("the build did not report one system per computer")
	}
	systems := map[string]string{}
	for index, name := range names {
		path := results[index].Outputs["out"]
		if !validSystemPath(path) || !strings.Contains(path, "-nixos-system-"+name+"-") {
			return nil, fmt.Errorf("the build returned an unexpected system for %s", name)
		}
		systems[name] = path
	}
	return systems, nil
}

// configureProcessGroup bounds a child and everything it starts: cancelling
// kills the whole group, so no ssh child keeps an output pipe open.
func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 2 * time.Second
}

// clientActivation is the outcome of one computer's apply.
type clientActivation struct {
	name     string
	started  bool
	result   string
	exitCode *int
	detail   string
}

// applyClientSystems copies and activates the built systems. It returns an
// error unless every computer activated; a DeploymentUnconfirmedError when an
// activation that may have changed a computer has no known result.
func applyClientSystems(ctx context.Context, plan domain.DeploymentPlanReport, output io.Writer, directory *os.File, managed bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	builtClientSystems.Lock()
	systems := builtClientSystems.systems[builtSystemsKey(plan.Repository, plan.Revision)]
	builtClientSystems.Unlock()
	for _, target := range plan.Targets {
		if systems[target.Name] == "" {
			return fmt.Errorf("the system of %s was not built in this update; nothing was started", target.Name)
		}
	}

	activations := make([]domain.PendingActivation, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		id, err := newActivationID()
		if err != nil {
			return fmt.Errorf("prepare the client update; nothing was started: %w", err)
		}
		activations = append(activations, domain.PendingActivation{Name: target.Name, State: domain.ActivationPlanned, ActivationID: id})
	}
	unlockClassroom, err := lockClassroomForPending(directory, managed, deploymentPendingLockWait)
	if err != nil {
		return fmt.Errorf("prepare the client update; nothing was started: %w", err)
	}
	record, err := createDeploymentPending(directory, plan, activations)
	unlockClassroom()
	if err != nil {
		return &domain.DeploymentUnconfirmedError{Err: fmt.Errorf("persist deployment intent; no new apply started: %w", err)}
	}

	log := &lockedWriter{writer: output}
	copySlots := make(chan struct{}, clientCopyParallelism)
	activationSlots := make(chan struct{}, clientActivationParallelism)
	outcomes := make([]clientActivation, len(plan.Targets))
	var group sync.WaitGroup
	for index, target := range plan.Targets {
		group.Add(1)
		go func() {
			defer group.Done()
			activation := record.activation(target.Name)
			outcomes[index] = applyClientSystem(ctx, target, deploymentAddress(plan, target), systems[target.Name], activation.ActivationID, log, record, copySlots, activationSlots)
		}()
	}
	group.Wait()

	unknown, failed := []string{}, []string{}
	for _, outcome := range outcomes {
		switch {
		case outcome.started && outcome.result == domain.ActivationUnknown:
			unknown = append(unknown, outcome.name)
		case !domain.ActivationSwitched(outcome.result):
			failed = append(failed, fmt.Sprintf("%s (%s)", outcome.name, outcome.detail))
		}
	}
	if len(unknown) > 0 {
		record.close()
		return &domain.DeploymentUnconfirmedError{Err: fmt.Errorf("the activation result of %s is not known yet", strings.Join(unknown, ", "))}
	}
	if err := record.clear(); err != nil {
		return &domain.DeploymentUnconfirmedError{Err: fmt.Errorf("every activation finished, but clearing the pending record failed: %w", err)}
	}
	if len(failed) > 0 {
		return fmt.Errorf("not updated: %s", strings.Join(failed, "; "))
	}
	return nil
}

func newActivationID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// deploymentAddress is the address that answered the last check: during a
// change of laboratory network a computer not yet moved is reached on its
// previous address.
func deploymentAddress(plan domain.DeploymentPlanReport, target domain.DeploymentTarget) string {
	for _, observed := range plan.Availability {
		if observed.Name == target.Name && observed.IP == target.IP && observed.Network == domain.HostNetworkPrevious && target.PreviousIP != "" {
			return target.PreviousIP
		}
	}
	return target.IP
}

func applyClientSystem(ctx context.Context, target domain.DeploymentTarget, address, system, activationID string, log io.Writer, record *pendingRecord, copySlots, activationSlots chan struct{}) clientActivation {
	outcome := clientActivation{name: target.Name}
	say := func(format string, arguments ...any) {
		fmt.Fprintf(log, "[%s] "+format+"\n", append([]any{target.Name}, arguments...)...)
	}
	if canonicalRemoteIPv4(address) == "" {
		outcome.detail = "invalid address"
		return outcome
	}

	copySlots <- struct{}{}
	say("copying the system to %s", address)
	err := copyClientClosure(ctx, address, system, &prefixedWriter{prefix: "[" + target.Name + "] ", writer: log})
	<-copySlots
	if err != nil {
		say("copy failed: %v", err)
		outcome.detail = "copy failed"
		return outcome
	}

	activationSlots <- struct{}{}
	defer func() { <-activationSlots }()
	// From here the computer's system profile may change: say so durably
	// before asking it to activate.
	if err := record.update(target.Name, func(activation *domain.PendingActivation) {
		activation.State = domain.ActivationStarted
	}); err != nil {
		say("the pending record could not be updated, activation not started: %v", err)
		outcome.detail = "pending record not updated"
		return outcome
	}
	outcome.started = true
	say("activating")
	status, startErr := runClientCommand(ctx, address, clientActivationStartCommand(system, activationID))
	switch {
	case startErr == nil:
	case status == clientActivationBusyStatus:
		say("another activation is running on this computer; nothing was started")
		outcome.started, outcome.detail = false, "another activation is running"
		_ = record.update(target.Name, func(activation *domain.PendingActivation) { activation.State = domain.ActivationPlanned })
		return outcome
	case status > 0 && status != 255:
		// The remote shell answered: systemd-run refused, so no unit exists.
		say("the activation could not start: %v", startErr)
		outcome.started, outcome.detail = false, "activation could not start"
		_ = record.update(target.Name, func(activation *domain.PendingActivation) { activation.State = domain.ActivationPlanned })
		return outcome
	default:
		say("the start request was not confirmed (%v); watching the computer", startErr)
	}

	result := waitForClientActivation(ctx, address, activationID)
	outcome.result, outcome.exitCode = result.result, result.exitCode
	if result.result == domain.ActivationUnknown {
		say("activation result unknown: %s", result.detail)
		outcome.detail = result.detail
		_ = record.update(target.Name, func(activation *domain.PendingActivation) {
			activation.Result = domain.ActivationUnknown
		})
		return outcome
	}
	say("activation finished: %s (exit status %d)", result.result, *result.exitCode)
	if journal, _ := runClientOutput(ctx, address, clientActivationJournalCommand); journal != "" {
		for _, line := range strings.Split(strings.TrimRight(journal, "\n"), "\n") {
			say("  %s", line)
		}
	}
	outcome.detail = "exit status " + fmt.Sprint(*result.exitCode)
	_ = record.update(target.Name, func(activation *domain.PendingActivation) {
		activation.State, activation.Result, activation.ExitCode = domain.ActivationFinished, result.result, result.exitCode
	})
	return outcome
}

// clientSSHOptions are those of the existing management access: keys only,
// the managed known hosts, bounded connection attempts.
func clientSSHOptions() []string {
	return []string{
		"-T",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ConnectionAttempts=1",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ServerAliveInterval=10",
		"-o", "ServerAliveCountMax=3",
		"-o", "LogLevel=ERROR",
	}
}

// copyClientClosure copies a system closure like Colmena did: the computer
// takes what it can from its substituters (the controller's cache) and
// receives the rest over SSH.
func copyClientClosure(ctx context.Context, address, system string, log io.Writer) error {
	copyContext, cancel := context.WithTimeout(ctx, clientCopyTimeout)
	defer cancel()
	command := exec.CommandContext(copyContext, "nix-copy-closure", "--to", "root@"+address, "--use-substitutes", system)
	configureProcessGroup(command)
	command.Env = environmentWithValue(os.Environ(), "NIX_SSHOPTS", strings.Join(clientSSHOptions()[1:], " "))
	command.Stdout, command.Stderr = log, log
	if err := command.Run(); err != nil {
		if copyContext.Err() != nil {
			return copyContext.Err()
		}
		return err
	}
	return nil
}

// runClientCommand runs one fixed command on the computer and returns the
// remote exit status (255 when SSH itself failed, -1 when nothing ran).
func runClientCommand(ctx context.Context, address, remote string) (int, error) {
	_, status, err := runClientSSH(ctx, address, remote)
	return status, err
}

func runClientOutput(ctx context.Context, address, remote string) (string, error) {
	output, _, err := runClientSSH(ctx, address, remote)
	return output, err
}

func runClientSSH(ctx context.Context, address, remote string) (string, int, error) {
	commandContext, cancel := context.WithTimeout(ctx, clientCommandTimeout)
	defer cancel()
	arguments := append(clientSSHOptions(), "root@"+address, remote)
	command := exec.CommandContext(commandContext, "ssh", arguments...)
	configureProcessGroup(command)
	stdout := &boundedCommandBuffer{limit: 256 * 1024}
	stderr := &boundedCommandBuffer{limit: 16 * 1024}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			message := strings.TrimSpace(stderr.buffer.String())
			if message == "" {
				message = err.Error()
			}
			return stdout.buffer.String(), exit.ExitCode(), errors.New(message)
		}
		return stdout.buffer.String(), -1, err
	}
	return stdout.buffer.String(), 0, nil
}

// clientActivationScript runs on the computer inside the transient unit. It
// sets the system profile, switches, and writes its result atomically. It
// uses no single quote: it is passed single-quoted to the remote shell.
const clientActivationScript = `dir=` + clientActivationDirectory + `; mkdir -p "$dir"; ` +
	`started=$(date -u +%Y-%m-%dT%H:%M:%SZ); ` +
	`if nix-env -p /nix/var/nix/profiles/system --set "$NIXORIUM_SYSTEM"; then "$NIXORIUM_SYSTEM/bin/switch-to-configuration" switch; status=$?; else status=200; fi; ` +
	`finished=$(date -u +%Y-%m-%dT%H:%M:%SZ); ` +
	`printf "{\"activationId\":\"%s\",\"systemPath\":\"%s\",\"exitCode\":%d,\"startedAt\":\"%s\",\"finishedAt\":\"%s\"}\n" "$NIXORIUM_ACTIVATION" "$NIXORIUM_SYSTEM" "$status" "$started" "$finished" > "$dir/last.json.tmp" && ` +
	`sync "$dir/last.json.tmp" && mv -f "$dir/last.json.tmp" "$dir/last.json" && sync "$dir"; ` +
	`exit "$status"`

// clientActivationStartCommand refuses while another activation runs, clears
// a finished one, and starts this one without waiting for it.
func clientActivationStartCommand(system, activationID string) string {
	return `state=$(systemctl show -p SubState --value ` + clientActivationUnit + ` 2>/dev/null); ` +
		`case "$state" in running|start|start-pre|start-post) exit ` + fmt.Sprint(clientActivationBusyStatus) + `;; esac; ` +
		`systemctl stop ` + clientActivationUnit + ` >/dev/null 2>&1; systemctl reset-failed ` + clientActivationUnit + ` >/dev/null 2>&1; ` +
		`systemd-run --unit=` + strings.TrimSuffix(clientActivationUnit, ".service") + ` --description=Nixorium-activation --service-type=exec ` +
		`--property=RemainAfterExit=yes --expand-environment=no --no-block --quiet ` +
		`--setenv=PATH=/run/current-system/sw/bin:/run/wrappers/bin ` +
		`--setenv=NIXORIUM_ACTIVATION=` + activationID + ` --setenv=NIXORIUM_SYSTEM=` + system + ` ` +
		`/bin/sh -c '` + clientActivationScript + `'`
}

// clientActivationObserveCommand prints the last activation result, a
// separator, and the state of the activation unit.
const clientActivationObserveCommand = `cat ` + clientActivationDirectory + `/last.json 2>/dev/null; echo; echo '--- unit'; ` +
	`systemctl show -p SubState --value ` + clientActivationUnit + ` 2>/dev/null; true`

// clientActivationJournalCommand prints this activation's log and then
// unloads the finished unit so that the next activation can start.
const clientActivationJournalCommand = `invocation=$(systemctl show -p InvocationID --value ` + clientActivationUnit + ` 2>/dev/null); ` +
	`if [ -n "$invocation" ]; then journalctl _SYSTEMD_INVOCATION_ID="$invocation" -o cat --no-pager -n 300 2>/dev/null; fi; ` +
	`systemctl stop ` + clientActivationUnit + ` >/dev/null 2>&1; systemctl reset-failed ` + clientActivationUnit + ` >/dev/null 2>&1; true`

type clientActivationObservation struct {
	result   string
	exitCode *int
	running  bool
	detail   string
}

// parseClientActivation reads the observe command's output for one
// activation identifier.
func parseClientActivation(output, activationID string) clientActivationObservation {
	recordText, unitText, found := strings.Cut(output, "--- unit\n")
	if !found {
		return clientActivationObservation{detail: "invalid observation"}
	}
	var record struct {
		ActivationID string `json:"activationId"`
		ExitCode     *int   `json:"exitCode"`
	}
	if text := strings.TrimSpace(recordText); text != "" && json.Unmarshal([]byte(text), &record) == nil &&
		record.ActivationID == activationID && record.ExitCode != nil {
		return clientActivationObservation{result: domain.ActivationResult(*record.ExitCode), exitCode: record.ExitCode}
	}
	switch strings.TrimSpace(unitText) {
	case "running", "start", "start-pre", "start-post":
		return clientActivationObservation{running: true}
	}
	return clientActivationObservation{}
}

// waitForClientActivation polls until this activation's result is written,
// tolerating the network restarts an activation may cause. Without a result
// the outcome is unknown, never failed.
func waitForClientActivation(ctx context.Context, address, activationID string) clientActivationObservation {
	started := time.Now()
	deadline := started.Add(clientActivationTimeout)
	lastDetail := "no answer from the computer"
	for {
		output, err := runClientOutput(ctx, address, clientActivationObserveCommand)
		if err == nil {
			observation := parseClientActivation(output, activationID)
			if observation.result != "" {
				return observation
			}
			if observation.running {
				lastDetail = "still activating"
			} else if time.Since(started) > clientActivationStartGrace {
				return clientActivationObservation{result: domain.ActivationUnknown, detail: "the activation stopped without a result"}
			}
		} else {
			lastDetail = "no answer from the computer: " + err.Error()
		}
		if !time.Now().Before(deadline) || ctx.Err() != nil {
			return clientActivationObservation{result: domain.ActivationUnknown, detail: lastDetail}
		}
		select {
		case <-ctx.Done():
		case <-time.After(clientActivationPoll):
		}
	}
}

// lockedWriter serializes the log lines of computers updated in parallel.
type lockedWriter struct {
	mutex  sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.writer.Write(data)
}

// prefixedWriter prefixes every complete line with the computer's name.
type prefixedWriter struct {
	prefix  string
	writer  io.Writer
	pending bytes.Buffer
}

func (w *prefixedWriter) Write(data []byte) (int, error) {
	w.pending.Write(data)
	for {
		line, err := w.pending.ReadBytes('\n')
		if err != nil {
			w.pending.Reset()
			w.pending.Write(line)
			return len(data), nil
		}
		if _, err := io.WriteString(w.writer, w.prefix+string(line)); err != nil {
			return len(data), err
		}
	}
}
