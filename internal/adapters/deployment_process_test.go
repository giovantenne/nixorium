package adapters

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Fake commands of the deployment engine. The fake ssh plays the computer:
// NIXORIUM_TEST_ACTIVATION sets how the activation ends (an exit status,
// "vanish" for a unit that stops without a result, "busy" for another
// activation running, "hang" for one that never ends).
const fakeEngineNix = `#!/bin/sh
printf '['
first=1
for argument in "$@"; do
  case "$argument" in
    *'#nixosConfigurations.'*)
      name=${argument#*#nixosConfigurations.}; name=${name%%.*}
      [ "$first" = 1 ] || printf ','
      first=0
      printf '%s\n' "$name" >> "$NIXORIUM_TEST_STATE/builds"
      system=${NIXORIUM_TEST_SYSTEM_NAME:-$name}
      printf '{"drvPath":"/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-x.drv","outputs":{"out":"/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-nixos-system-%s-test"}}' "$system"
      ;;
  esac
done
printf ']\n'
echo "build log" >&2
`

const fakeEngineCopy = `#!/bin/sh
printf '%s\n' "$*" >> "$NIXORIUM_TEST_STATE/copies"
[ -z "$NIXORIUM_TEST_COPY_FAIL" ] || exit 1
echo "copying path"
`

const fakeEngineSSH = `#!/bin/sh
host=
for remote; do case "$remote" in root@*) host=${remote#root@} ;; esac; done
state=$NIXORIUM_TEST_STATE
case "$remote" in
  *identity.json*)
    printf '%s %s\n' "$host" "$(printf '%s' "$remote" | sed -n "s/^expected='\(.*\)'$/\1/p")" >> "$state/identities"
    [ "$NIXORIUM_TEST_IDENTITY" = conflict ] && exit 3
    exit 0
    ;;
  *systemd-run*)
    [ "$NIXORIUM_TEST_ACTIVATION" = refused ] && { echo "ssh: connect to host $host port 22: Connection refused" >&2; exit 255; }
    test -s "$NIXORIUM_TEST_PENDING" || exit 99
    grep -q '"state":"started"' "$NIXORIUM_TEST_PENDING" || exit 98
    [ "$NIXORIUM_TEST_ACTIVATION" = busy ] && exit 75
    [ "$NIXORIUM_TEST_ACTIVATION" = startfail ] && exit 70
    printf '%s' "$remote" | sed -n 's/.*NIXORIUM_ACTIVATION=\([0-9a-f]*\).*/\1/p' > "$state/id-$host"
    touch "$state/started"
    # The unit started although systemd-run reported a failure.
    [ "$NIXORIUM_TEST_ACTIVATION" = startfail-ran ] && exit 70
    echo nixorium-activation-started
    exit 0
    ;;
  *'--- unit'*)
    id=$(cat "$state/id-$host" 2>/dev/null)
    case "$NIXORIUM_TEST_ACTIVATION" in
      hang) printf '\n--- unit\nrunning\n' ;;
      vanish|startfail) printf '\n--- unit\ndead\n' ;;
      startfail-ran) printf '{"activationId":"%s","systemPath":"/nix/store/x","exitCode":0}\n\n--- unit\nexited\n' "$id" ;;
      *) printf '{"activationId":"%s","systemPath":"/nix/store/x","exitCode":%s}\n\n--- unit\nexited\n' "$id" "$NIXORIUM_TEST_ACTIVATION" ;;
    esac
    ;;
  *journalctl*) echo "activation output" ;;
esac
`

func deploymentProcessFixture(t *testing.T, activation string) (domain.DeploymentPlanReport, string, string) {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "nix"), fakeEngineNix)
	writeExecutable(t, filepath.Join(bin, "nix-copy-closure"), fakeEngineCopy)
	writeExecutable(t, filepath.Join(bin, "ssh"), fakeEngineSSH)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	state := t.TempDir()
	t.Setenv("NIXORIUM_TEST_STATE", state)
	t.Setenv("NIXORIUM_TEST_ACTIVATION", activation)
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NIXORIUM_TEST_PENDING", filepath.Join(directory, deploymentPendingName))
	previous := clientBuildSource
	clientBuildSource = func(context.Context, string, string) (string, error) { return "path:/nix/store/source", nil }
	pollInterval, grace, timeout := clientActivationPoll, clientActivationStartGrace, clientActivationTimeout
	clientActivationPoll, clientActivationStartGrace, clientActivationTimeout = 10*time.Millisecond, 100*time.Millisecond, 2*time.Second
	t.Cleanup(func() {
		clientBuildSource = previous
		clientActivationPoll, clientActivationStartGrace, clientActivationTimeout = pollInterval, grace, timeout
	})
	plan := domain.DeploymentPlanReport{Repository: t.TempDir(), Revision: strings.Repeat("a", 40), ColmenaSelector: "pc01", Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "192.0.2.1"}}}
	return plan, directory, state
}

func buildAndApply(t *testing.T, plan domain.DeploymentPlanReport, directory string, output io.Writer) error {
	t.Helper()
	if err := runDeploymentPhase(context.Background(), plan, domain.DeploymentPhaseBuild, output, directory, false, time.Minute); err != nil {
		t.Fatalf("build: %v", err)
	}
	return runDeploymentPhase(context.Background(), plan, domain.DeploymentPhaseApply, output, directory, false, time.Minute)
}

func readTestPending(t *testing.T, directory string) domain.PendingDeployment {
	t.Helper()
	record, present, err := pendingDeploymentAt(directory)
	if !present || err != nil {
		t.Fatalf("pending record: present %v, %v", present, err)
	}
	return record
}

func TestDeploymentEngineActivatesOnceAndClearsTheRecord(t *testing.T) {
	for _, exit := range []string{"0", "4", "100"} {
		t.Run(exit, func(t *testing.T) {
			plan, directory, state := deploymentProcessFixture(t, exit)
			var output bytes.Buffer
			if err := buildAndApply(t, plan, directory, &output); err != nil {
				t.Fatalf("apply: %v\n%s", err, output.String())
			}
			if err := checkDeploymentPendingPath(directory); err != nil {
				t.Fatalf("record kept after a known result: %v", err)
			}
			copies, _ := os.ReadFile(filepath.Join(state, "copies"))
			if !strings.Contains(string(copies), "--to root@192.0.2.1 --use-substitutes /nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-nixos-system-pc01-test") {
				t.Fatalf("copy = %q", copies)
			}
			for _, want := range []string{"build log", "[pc01] copying path", "[pc01]   activation output", "activation finished"} {
				if !strings.Contains(output.String(), want) {
					t.Fatalf("log lacks %q:\n%s", want, output.String())
				}
			}
		})
	}
}

func TestDeploymentEngineBuildsAClassOnceAndWritesIdentities(t *testing.T) {
	plan, directory, state := deploymentProcessFixture(t, "0")
	t.Setenv("NIXORIUM_TEST_SYSTEM_NAME", "nixorium-client-shared-1")
	plan.Targets = []domain.DeploymentTarget{
		{Name: "pc01", IP: "192.0.2.1", Class: "shared-1"},
		{Name: "pc02", IP: "192.0.2.2", Class: "shared-1"},
	}
	var output bytes.Buffer
	if err := buildAndApply(t, plan, directory, &output); err != nil {
		t.Fatalf("apply: %v\n%s", err, output.String())
	}
	if builds, _ := os.ReadFile(filepath.Join(state, "builds")); string(builds) != "pc01\n" {
		t.Fatalf("builds = %q, want the class once", builds)
	}
	if !strings.Contains(output.String(), "1 configuration(s) for 2 computer(s)") {
		t.Fatalf("log:\n%s", output.String())
	}
	identities, _ := os.ReadFile(filepath.Join(state, "identities"))
	for _, want := range []string{`192.0.2.1 {"schemaVersion":1,"number":1}`, `192.0.2.2 {"schemaVersion":1,"number":2}`} {
		if !strings.Contains(string(identities), want) {
			t.Fatalf("identities = %q", identities)
		}
	}
	copies, _ := os.ReadFile(filepath.Join(state, "copies"))
	if strings.Count(string(copies), "nixos-system-nixorium-client-shared-1-test") != 2 {
		t.Fatalf("copies = %q", copies)
	}
}

func TestDeploymentEngineLeavesAComputerWithAnotherIdentityUnchanged(t *testing.T) {
	plan, directory, state := deploymentProcessFixture(t, "0")
	t.Setenv("NIXORIUM_TEST_SYSTEM_NAME", "nixorium-client-shared-1")
	t.Setenv("NIXORIUM_TEST_IDENTITY", "conflict")
	plan.Targets = []domain.DeploymentTarget{{Name: "pc01", IP: "192.0.2.1", Class: "shared-1"}}
	var output bytes.Buffer
	err := buildAndApply(t, plan, directory, &output)
	if err == nil || !strings.Contains(err.Error(), "identity record names another computer") {
		t.Fatalf("apply = %v\n%s", err, output.String())
	}
	if _, statErr := os.Lstat(filepath.Join(state, "started")); statErr == nil {
		t.Fatal("an activation started on a computer with another identity")
	}
	if checkDeploymentPendingPath(directory) != nil {
		t.Fatal("a computer left unchanged must not stay pending")
	}
}

func TestDeploymentEngineRefusesASharedSystemUnderAnotherName(t *testing.T) {
	plan, directory, _ := deploymentProcessFixture(t, "0")
	plan.Targets = []domain.DeploymentTarget{{Name: "pc01", IP: "192.0.2.1", Class: "shared-1"}}
	err := runDeploymentPhase(context.Background(), plan, domain.DeploymentPhaseBuild, io.Discard, directory, false, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "unexpected system for pc01") {
		t.Fatalf("build = %v", err)
	}
}

func TestClientIdentityCommandWritesOnceAndKeepsAnotherComputer(t *testing.T) {
	root := t.TempDir()
	run := func(number int) int {
		script := strings.ReplaceAll(clientIdentityCommand(number), "/var/lib/nixorium", root)
		command := exec.Command("sh", "-c", script)
		if err := command.Run(); err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return exit.ExitCode()
			}
			t.Fatal(err)
		}
		return 0
	}
	if status := run(7); status != 0 {
		t.Fatalf("first write = %d", status)
	}
	content, _ := os.ReadFile(filepath.Join(root, "identity.json"))
	if string(content) != "{\"schemaVersion\":1,\"number\":7}\n" {
		t.Fatalf("record = %q", content)
	}
	if status := run(7); status != 0 {
		t.Fatalf("same number again = %d", status)
	}
	if status := run(8); status != clientIdentityConflictStatus {
		t.Fatalf("another number = %d", status)
	}
	// A damaged record is written again.
	if err := os.WriteFile(filepath.Join(root, "identity.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if status := run(8); status != 0 {
		t.Fatalf("damaged record = %d", status)
	}
	if content, _ := os.ReadFile(filepath.Join(root, "identity.json")); string(content) != "{\"schemaVersion\":1,\"number\":8}\n" {
		t.Fatalf("rewritten record = %q", content)
	}
	if err := os.WriteFile(filepath.Join(root, "identity.json"), []byte("{\"schemaVersion\":1,\"number\":7}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(filepath.Join(root, "identity.json")); string(content) != "{\"schemaVersion\":1,\"number\":7}\n" {
		t.Fatalf("record changed to %q", content)
	}
}

// A start command whose systemd-run reported a failure is "not started"
// only when the computer shows no unit and no result; a refused connection
// started nothing either.
func TestDeploymentEngineTellsAFailedStartFromAnUnconfirmedOne(t *testing.T) {
	for _, test := range []struct {
		activation string
		failed     string
	}{
		{"startfail", "activation could not start"},
		{"refused", "computer not reachable"},
	} {
		t.Run(test.activation, func(t *testing.T) {
			plan, directory, state := deploymentProcessFixture(t, test.activation)
			var output bytes.Buffer
			err := buildAndApply(t, plan, directory, &output)
			if err == nil || !strings.Contains(err.Error(), test.failed) {
				t.Fatalf("apply = %v\n%s", err, output.String())
			}
			if checkDeploymentPendingPath(directory) != nil {
				t.Fatal("a computer where nothing started stays pending")
			}
			if _, statErr := os.Lstat(filepath.Join(state, "started")); statErr == nil {
				t.Fatal("the fake unit started")
			}
		})
	}
	plan, directory, _ := deploymentProcessFixture(t, "startfail-ran")
	var output bytes.Buffer
	if err := buildAndApply(t, plan, directory, &output); err != nil || !strings.Contains(output.String(), "watching the computer") || !strings.Contains(output.String(), "activation finished") {
		t.Fatalf("an activation that ran after a reported failure was not followed: %v\n%s", err, output.String())
	}
}

func TestDeploymentEngineReportsAFinishedFailureWithoutRecovery(t *testing.T) {
	plan, directory, _ := deploymentProcessFixture(t, "2")
	err := buildAndApply(t, plan, directory, io.Discard)
	var uncertain *domain.DeploymentUnconfirmedError
	if err == nil || errors.As(err, &uncertain) || !strings.Contains(err.Error(), "pc01") {
		t.Fatalf("finished failure: %v", err)
	}
	if err := checkDeploymentPendingPath(directory); err != nil {
		t.Fatalf("a known failure kept the laboratory blocked: %v", err)
	}
}

func TestDeploymentEngineKeepsTheRecordWhenTheOutcomeIsUnknown(t *testing.T) {
	for _, activation := range []string{"vanish", "hang"} {
		t.Run(activation, func(t *testing.T) {
			plan, directory, _ := deploymentProcessFixture(t, activation)
			err := buildAndApply(t, plan, directory, io.Discard)
			var uncertain *domain.DeploymentUnconfirmedError
			if !errors.As(err, &uncertain) {
				t.Fatalf("unknown outcome not reported as unconfirmed: %v", err)
			}
			record := readTestPending(t, directory)
			activationState := record.ActivationOf("pc01")
			if activationState.State != domain.ActivationStarted || activationState.Result != domain.ActivationUnknown || len(activationState.ActivationID) != 32 {
				t.Fatalf("activation = %+v", activationState)
			}
			if gate, err := acquireOperationGateAt(directory, false); err == nil {
				gate.Close()
				t.Fatal("new operation bypassed the pending record")
			} else if code, _ := domain.BlockedCodeOf(err); code != "DEPLOY-PENDING" {
				t.Fatalf("refusal = %v", err)
			}
			if (Local{}).OwnDeploymentApplyRunning() {
				t.Fatal("returned apply still counted as running")
			}
		})
	}
}

func TestDeploymentEngineStartsNothingOnAComputerItCouldNotPrepare(t *testing.T) {
	for _, test := range []struct{ name, activation, copyFail string }{
		{"copy failed", "0", "1"},
		{"another activation", "busy", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, directory, state := deploymentProcessFixture(t, test.activation)
			t.Setenv("NIXORIUM_TEST_COPY_FAIL", test.copyFail)
			err := buildAndApply(t, plan, directory, io.Discard)
			var uncertain *domain.DeploymentUnconfirmedError
			if err == nil || errors.As(err, &uncertain) {
				t.Fatalf("unstarted computer: %v", err)
			}
			if err := checkDeploymentPendingPath(directory); err != nil {
				t.Fatalf("record kept for a computer that never started: %v", err)
			}
			if _, statErr := os.Lstat(filepath.Join(state, "started")); statErr == nil {
				t.Fatal("activation started after a failed preparation")
			}
		})
	}
}

func TestDeploymentEngineAppliesOnlyWhatItBuilt(t *testing.T) {
	plan, directory, _ := deploymentProcessFixture(t, "0")
	plan.Revision = strings.Repeat("c", 40)
	err := runDeploymentPhase(context.Background(), plan, domain.DeploymentPhaseApply, io.Discard, directory, false, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "not built") {
		t.Fatalf("apply without build: %v", err)
	}
	if err := checkDeploymentPendingPath(directory); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentEngineReachesComputersOnThePreviousNetwork(t *testing.T) {
	plan, directory, state := deploymentProcessFixture(t, "0")
	plan.Targets[0].PreviousIP = "192.0.2.201"
	plan.Availability = []domain.DeploymentTargetAvailability{{Name: "pc01", IP: "192.0.2.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Network: domain.HostNetworkPrevious}}
	if err := buildAndApply(t, plan, directory, io.Discard); err != nil {
		t.Fatal(err)
	}
	copies, _ := os.ReadFile(filepath.Join(state, "copies"))
	if !strings.Contains(string(copies), "root@192.0.2.201") {
		t.Fatalf("copy did not use the previous address: %q", copies)
	}
}

func TestDeploymentProcessCancelledBeforeStartDoesNotReserveRemoteWork(t *testing.T) {
	plan, directory, _ := deploymentProcessFixture(t, "0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runDeploymentPhase(ctx, plan, domain.DeploymentPhaseApply, io.Discard, directory, false, time.Second)
	var uncertain *domain.DeploymentUnconfirmedError
	if !errors.Is(err, context.Canceled) || errors.As(err, &uncertain) {
		t.Fatalf("unstarted operation was not safely cancelled: %v", err)
	}
	if err := checkDeploymentPendingPath(directory); err != nil {
		t.Fatal(err)
	}
}

func TestClientActivationCommandsAreFixed(t *testing.T) {
	command := clientActivationStartCommand("/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-nixos-system-pc01-test", strings.Repeat("f", 32))
	for _, want := range []string{"--expand-environment=no", "--property=RemainAfterExit=yes", "--no-block", "--unit=nixorium-activate ", "exit 75"} {
		if !strings.Contains(command, want) {
			t.Fatalf("start command lacks %q: %s", want, command)
		}
	}
	if strings.Count(clientActivationScript, "'") != 0 {
		t.Fatal("the activation script must not contain a single quote")
	}
	observation := parseClientActivation(`{"activationId":"`+strings.Repeat("f", 32)+`","exitCode":4}`+"\n\n--- unit\nexited\n", strings.Repeat("f", 32))
	if observation.result != domain.ActivationFailedUnits || *observation.exitCode != 4 {
		t.Fatalf("observation = %+v", observation)
	}
	if other := parseClientActivation(`{"activationId":"other","exitCode":0}`+"\n\n--- unit\nrunning\n", strings.Repeat("f", 32)); other.result != "" || !other.running {
		t.Fatalf("another activation's result was taken: %+v", other)
	}
}

func TestDeploymentPendingGateRefusesMalformedSpecialAndSymlinkEvidence(t *testing.T) {
	for _, kind := range []string{"malformed", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			_, directory, _ := deploymentProcessFixture(t, "0")
			path := filepath.Join(directory, deploymentPendingName)
			var err error
			switch kind {
			case "malformed":
				err = os.WriteFile(path, []byte("{"), 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "symlink":
				err = os.Symlink(filepath.Join(directory, "missing"), path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if gate, err := acquireOperationGateAt(directory, false); err == nil {
				gate.Close()
				t.Fatal("unsafe evidence bypassed gate")
			}
		})
	}
}
