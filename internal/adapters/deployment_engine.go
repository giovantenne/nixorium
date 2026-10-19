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
	// Exit status of the identity command when the record names another
	// computer.
	clientIdentityConflictStatus = 3
	// Exit status of the start command when systemd-run itself failed; the
	// unit may still exist (a reply lost while the manager was busy).
	clientActivationStartFailedStatus = 70
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
// the resulting store paths for the apply of this revision. Computers of a
// class share one system (ADR 0033): it is built once, through its first
// selected member.
func buildClientSystems(ctx context.Context, plan domain.DeploymentPlanReport, output io.Writer) error {
	type build struct {
		name     string
		fragment string
		members  []string
	}
	builds := []*build{}
	byKey := map[string]*build{}
	for _, target := range plan.Targets {
		if !clientHostName.MatchString(target.Name) {
			return fmt.Errorf("invalid computer name %q", target.Name)
		}
		if !domain.ValidClientClass(target.Name, target.Class) {
			return fmt.Errorf("invalid class %q of %s", target.Class, target.Name)
		}
		key, fragment := target.Name, "-nixos-system-"+domain.ClientSystemName(target.Name, target.Class)+"-"
		if target.SharedSystem() {
			key = "class:" + target.Class
		}
		if existing := byKey[key]; existing != nil {
			existing.members = append(existing.members, target.Name)
			continue
		}
		byKey[key] = &build{name: target.Name, fragment: fragment, members: []string{target.Name}}
		builds = append(builds, byKey[key])
	}
	if len(builds) == 0 {
		return errors.New("no computer was selected")
	}
	source, err := clientBuildSource(ctx, plan.Repository, plan.Revision)
	if err != nil {
		return err
	}
	if len(builds) < len(plan.Targets) {
		fmt.Fprintf(output, "%d configuration(s) for %d computer(s)\n", len(builds), len(plan.Targets))
	}
	systems := map[string]string{}
	for start := 0; start < len(builds); start += clientBuildBatch {
		batch := builds[start:min(len(builds), start+clientBuildBatch)]
		names, fragments, labels := []string{}, map[string]string{}, []string{}
		for _, entry := range batch {
			names = append(names, entry.name)
			fragments[entry.name] = entry.fragment
			labels = append(labels, strings.Join(entry.members, ", "))
		}
		fmt.Fprintf(output, "Building %s\n", strings.Join(labels, "; "))
		built, err := buildClientBatch(ctx, source, names, fragments, output)
		if err != nil {
			return err
		}
		for _, entry := range batch {
			for _, member := range entry.members {
				systems[member] = built[entry.name]
			}
		}
	}
	builtClientSystems.Lock()
	builtClientSystems.systems[builtSystemsKey(plan.Repository, plan.Revision)] = systems
	builtClientSystems.Unlock()
	return nil
}

func buildClientBatch(ctx context.Context, source string, names []string, fragments map[string]string, output io.Writer) (map[string]string, error) {
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
		if !validSystemPath(path) || !strings.Contains(path, fragments[name]) {
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
		message := fmt.Sprintf("the activation result of %s is not known yet", strings.Join(unknown, ", "))
		if err := record.close(); err != nil {
			// Without the mark, recovery waits for this process: say so.
			message += fmt.Sprintf("; the record could not be marked as returned (%v), so close Nixorium before reviewing it", err)
		}
		return &domain.DeploymentUnconfirmedError{Err: errors.New(message)}
	}
	if err := record.clear(); err != nil {
		return &domain.DeploymentUnconfirmedError{Err: fmt.Errorf("every activation finished, but clearing the pending record failed: %w", err)}
	}
	if len(failed) > 0 {
		return fmt.Errorf("not updated: %s", strings.Join(failed, "; "))
	}
	return nil
}

// clientIdentityCommand writes the identity record of a computer that is
// about to run a shared system, durably. A valid record naming another
// computer is kept (exit clientIdentityConflictStatus): two computers with
// the same number need a person to decide which one is which. A damaged
// record (empty after a power loss, edited) is written again: the computer
// answered with its pinned host key at this computer's address.
func clientIdentityCommand(number int) string {
	expected := fmt.Sprintf(`{"schemaVersion":1,"number":%d}`, number)
	return fmt.Sprintf(`set -eu
record=/var/lib/nixorium/identity.json
expected='%s'
if [ -e "$record" ]; then
  [ "$(cat "$record")" = "$expected" ] && exit 0
  if grep -Eqx '\{"schemaVersion":1,"number":[1-9][0-9]{0,3}\}' "$record"; then
    exit %d
  fi
fi
mkdir -p /var/lib/nixorium
printf '%%s\n' "$expected" > "$record.tmp"
chmod 0644 "$record.tmp"
sync "$record.tmp"
mv -f "$record.tmp" "$record"
sync /var/lib/nixorium
[ "$(cat "$record")" = "$expected" ]`, expected, clientIdentityConflictStatus)
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

	// A shared system takes the computer's name and address from its
	// identity record: write it first, or leave the computer unchanged.
	if target.SharedSystem() {
		number, _ := domain.ClientNumber(target.Name)
		status, err := runClientCommand(ctx, address, clientIdentityCommand(number))
		switch {
		case err == nil:
		case status == clientIdentityConflictStatus:
			say("its identity record names another computer; it keeps its current system")
			outcome.detail = "identity record names another computer"
			return outcome
		default:
			say("its identity record could not be written (%v); it keeps its current system", err)
			outcome.detail = "identity record not written"
			return outcome
		}
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
	startOutput, status, startErr := runClientSSH(ctx, address, clientActivationStartCommand(system, activationID))
	notStarted := func(message, detail string) clientActivation {
		say("%s", message)
		outcome.started, outcome.detail = false, detail
		_ = record.update(target.Name, func(activation *domain.PendingActivation) { activation.State = domain.ActivationPlanned })
		return outcome
	}
	switch {
	case startErr == nil && strings.Contains(startOutput, clientActivationStarted):
	case status == clientActivationBusyStatus:
		return notStarted("another activation is running on this computer; nothing was started", "another activation is running")
	case status == clientActivationStartFailedStatus && clientActivationAbsent(ctx, address, activationID):
		// systemd-run failed and the computer shows neither a unit nor a
		// result for this activation: nothing was started.
		return notStarted(fmt.Sprintf("the activation could not start: %v", startErr), "activation could not start")
	case status == 255 && sshNeverConnected(startErr):
		// The connection failed before authentication: nothing ran.
		return notStarted(fmt.Sprintf("the computer did not accept the connection (%v); nothing was started", startErr), "computer not reachable")
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
		`/bin/sh -c '` + clientActivationScript + `' && echo ` + clientActivationStarted + ` || exit ` + fmt.Sprint(clientActivationStartFailedStatus)
}

// clientActivationStarted is printed once systemd-run accepted the unit.
const clientActivationStarted = "nixorium-activation-started"

// clientActivationAbsent reports that the computer has no activation unit
// (or only a stopped one) and no result for this activation.
func clientActivationAbsent(ctx context.Context, address, activationID string) bool {
	output, err := runClientOutput(ctx, address, clientActivationObserveCommand)
	if err != nil {
		return false
	}
	_, unitText, found := strings.Cut(output, "--- unit\n")
	observation := parseClientActivation(output, activationID)
	state := strings.TrimSpace(unitText)
	return found && observation.result == "" && !observation.running && (state == "" || state == "dead")
}

// sshNeverConnected recognises ssh errors raised before authentication,
// when no remote command can have run.
func sshNeverConnected(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, reason := range []string{"Connection refused", "Connection timed out", "No route to host", "Network is unreachable", "Could not resolve hostname"} {
		if strings.Contains(message, reason) {
			return true
		}
	}
	return false
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
