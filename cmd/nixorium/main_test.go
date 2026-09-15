package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

type setupSecretReader struct {
	values [][]byte
}

func (r *setupSecretReader) ReadSecret(string) ([]byte, error) {
	if len(r.values) == 0 {
		return nil, errors.New("terminal unavailable")
	}
	value := r.values[0]
	r.values = r.values[1:]
	return value, nil
}

type setupPasswordHasher struct {
	calls int
}

func (h *setupPasswordHasher) HashPassword(context.Context, []byte) (string, error) {
	h.calls++
	return "$6$salt$hash", nil
}

func TestCollectSetupCredentialsRetriesOnlyCurrentAccount(t *testing.T) {
	inputs := [][]byte{
		[]byte("short"),
		[]byte("admin-password"), []byte("admin-password"),
		[]byte("teacher-password"), []byte("different-password"),
		[]byte("teacher-password"), []byte("teacher-password"),
		[]byte("nixos"),
		[]byte("student-password"), []byte("student-password"),
	}
	reader := &setupSecretReader{values: append([][]byte(nil), inputs...)}
	hasher := &setupPasswordHasher{}
	candidate := domain.LabSettingsFile{Lab: domain.LabSettings{
		PCCount:         42,
		AdminPassword:   domain.DefaultPasswordHash,
		TeacherPassword: domain.DefaultPasswordHash,
		StudentPassword: domain.DefaultPasswordHash,
	}}
	var output bytes.Buffer
	if err := collectSetupCredentials(context.Background(), reader, hasher, &output, &candidate); err != nil {
		t.Fatal(err)
	}
	if hasher.calls != 3 || candidate.Lab.AdminPassword != "$6$salt$hash" || candidate.Lab.TeacherPassword != "$6$salt$hash" || candidate.Lab.StudentPassword != "$6$salt$hash" {
		t.Fatalf("hasher calls = %d, candidate = %+v", hasher.calls, candidate.Lab)
	}
	if candidate.Lab.PCCount != 42 {
		t.Fatalf("previous wizard value was lost: %+v", candidate.Lab)
	}
	for _, expected := range []string{
		"retried without restarting configuration",
		"password must contain at least 8 bytes",
		"password confirmation does not match",
		"password must not use the public default",
		"Student password accepted (3/3)",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output omits %q: %s", expected, output.String())
		}
	}
	for _, input := range inputs {
		for _, value := range input {
			if value != 0 {
				t.Fatal("plaintext input was not wiped")
			}
		}
	}
}

func TestCollectSetupCredentialsStopsOnTerminalFailure(t *testing.T) {
	candidate := domain.LabSettingsFile{Lab: domain.LabSettings{AdminPassword: domain.DefaultPasswordHash}}
	reader := &setupSecretReader{values: [][]byte{[]byte("admin-password")}}
	err := collectSetupCredentials(context.Background(), reader, &setupPasswordHasher{}, &bytes.Buffer{}, &candidate)
	if err == nil || app.IsPasswordInputError(err) || !strings.Contains(err.Error(), "Administrator password: read password confirmation") {
		t.Fatalf("error = %v", err)
	}
}

func TestCollectSettingsPasswordRetriesOnlySelectedAccount(t *testing.T) {
	inputs := [][]byte{
		[]byte("short"),
		[]byte("new-teacher-password"), []byte("different-password"),
		[]byte("new-teacher-password"), []byte("new-teacher-password"),
	}
	reader := &setupSecretReader{values: append([][]byte(nil), inputs...)}
	hasher := &setupPasswordHasher{}
	settings := domain.LabSettingsFile{Lab: domain.LabSettings{
		AdminPassword:   "$6$old$admin",
		TeacherPassword: "$6$old$teacher",
		StudentPassword: "$6$old$student",
	}}
	var output bytes.Buffer
	candidate, err := collectSettingsPasswordWithReader(context.Background(), reader, hasher, &output, "teacher", settings)
	if err != nil {
		t.Fatal(err)
	}
	if hasher.calls != 1 || candidate.Lab.TeacherPassword != "$6$salt$hash" || candidate.Lab.AdminPassword != settings.Lab.AdminPassword || candidate.Lab.StudentPassword != settings.Lab.StudentPassword {
		t.Fatalf("hasher calls = %d, candidate = %+v", hasher.calls, candidate.Lab)
	}
	for _, expected := range []string{"without leaving this password step", "password must contain at least 8 bytes", "password confirmation does not match", "Teacher password accepted"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output omits %q: %s", expected, output.String())
		}
	}
	for _, input := range inputs {
		for _, value := range input {
			if value != 0 {
				t.Fatal("plaintext input was not wiped")
			}
		}
	}
}

func TestPXEPreparationActivityUsesStderrSafeGuidance(t *testing.T) {
	var output bytes.Buffer
	writePXEPreparationActivity(&output)
	for _, expected := range []string{"Preparing PXE artifacts and client closures", "journalctl -fu nixorium-prepare-pxe.service"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("PXE preparation activity omits %q: %s", expected, output.String())
		}
	}
}

func TestManagedProgressRendersOnlyCurrentTypedActivity(t *testing.T) {
	var output bytes.Buffer
	now := time.Now().UTC().Add(time.Second)
	result := runWithManagedProgress(
		func() int { return 7 },
		func() (domain.OperationProgress, error) {
			return domain.OperationProgress{
				Operation: "controller-apply", State: "completed", Phase: "complete",
				StartedAt: now, UpdatedAt: now.Add(time.Second), Current: 4, Total: 4,
				Recent: []string{"Controller revision activated and verified"},
			}, nil
		},
		&output,
	)
	if result != 7 || !strings.Contains(output.String(), "Progress [complete] (4/4): Controller revision activated and verified") {
		t.Fatalf("result = %d, output = %q", result, output.String())
	}

	output.Reset()
	old := time.Now().UTC().Add(-time.Hour)
	runWithManagedProgress(
		func() bool { return true },
		func() (domain.OperationProgress, error) {
			return domain.OperationProgress{StartedAt: old, UpdatedAt: old}, nil
		},
		&output,
	)
	if output.Len() != 0 {
		t.Fatalf("stale progress was rendered: %q", output.String())
	}
}

func TestOperationRecordMessagePersistsTypedOutcomeAndSurfacesFailure(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	report := domain.ActionReport{Operation: "pxe-prepare", State: "completed", Unit: "nixorium-prepare-pxe.service"}
	if message := operationRecordMessage("prepared", report); message != "prepared" {
		t.Fatalf("message = %q", message)
	}
	records, err := (adapters.Local{}).OperationRecords(10)
	if err != nil || len(records) != 1 || records[0].Operation != "pxe-prepare" {
		t.Fatalf("records = %+v, error = %v", records, err)
	}
	if message := operationRecordMessage("failed", domain.StatusReport{}); !strings.Contains(message, "was not recorded") {
		t.Fatalf("unsupported outcome failure was hidden: %q", message)
	}
}

func TestParseArgumentsAllowsFlagsBeforeOrAfterCommand(t *testing.T) {
	options, err := parseArguments([]string{"status", "--json", "--repo", "/tmp/site"})
	if err != nil {
		t.Fatal(err)
	}
	if options.command != "status" || !options.json || options.repository != "/tmp/site" {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestParseArgumentsAcceptsHosts(t *testing.T) {
	options, err := parseArguments([]string{"hosts", "--json"})
	if err != nil || options.command != "hosts" || !options.json {
		t.Fatalf("hosts options = %+v, error = %v", options, err)
	}
}

func TestParseArgumentsAcceptsDeploymentPlan(t *testing.T) {
	options, err := parseArguments([]string{"deploy", "plan", "--on", "pc01,pc02", "--json"})
	if err != nil || options.command != "deploy" || options.subcommand != "plan" || options.on != "pc01,pc02" || !options.json {
		t.Fatalf("deploy plan options = %+v, error = %v", options, err)
	}
	if _, err := parseArguments([]string{"deploy", "plan"}); err == nil {
		t.Fatal("deploy plan without targets was accepted")
	}
	if _, err := parseArguments([]string{"status", "--on", "pc01"}); err == nil {
		t.Fatal("status accepted deployment targets")
	}
}

func TestParseArgumentsAcceptsReviewedDeploymentApply(t *testing.T) {
	options, err := parseArguments([]string{"deploy", "apply", "--on", "@lab", "--expect", "0123456789abcdef", "--yes", "--json"})
	if err != nil || options.command != "deploy" || options.subcommand != "apply" || options.on != "@lab" || options.expect != "0123456789abcdef" || !options.yes || !options.json {
		t.Fatalf("deploy apply options = %+v, error = %v", options, err)
	}
	if _, err := parseArguments([]string{"deploy", "apply", "--on", "pc01"}); err == nil {
		t.Fatal("deploy apply without reviewed revision was accepted")
	}
	if _, err := parseArguments([]string{"deploy", "apply", "--expect", "revision"}); err == nil {
		t.Fatal("deploy apply without targets was accepted")
	}
}

func TestParseArgumentsAcceptsControllerPlanAndApply(t *testing.T) {
	plan, err := parseArguments([]string{"controller", "plan", "--json"})
	if err != nil || plan.command != "controller" || plan.subcommand != "plan" || !plan.json {
		t.Fatalf("controller plan = %+v, error = %v", plan, err)
	}
	apply, err := parseArguments([]string{"controller", "apply", "--expect", "0123456789abcdef0123456789abcdef01234567", "--yes", "--json"})
	if err != nil || apply.subcommand != "apply" || !apply.yes || apply.expect == "" {
		t.Fatalf("controller apply = %+v, error = %v", apply, err)
	}
	if _, err := parseArguments([]string{"controller", "apply"}); err == nil {
		t.Fatal("controller apply without reviewed revision was accepted")
	}
}

func TestParseArgumentsAcceptsServicesAndCacheRestart(t *testing.T) {
	list, err := parseArguments([]string{"services", "--json"})
	if err != nil || list.command != "services" || list.subcommand != "" || !list.json {
		t.Fatalf("services = %+v, error = %v", list, err)
	}
	restart, err := parseArguments([]string{"services", "restart", "cache", "--yes", "--json"})
	if err != nil || restart.command != "services" || restart.subcommand != "restart" || restart.service != "cache" || !restart.yes || !restart.json {
		t.Fatalf("services restart = %+v, error = %v", restart, err)
	}
	for _, arguments := range [][]string{
		{"services", "restart"},
		{"services", "restart", "pxe"},
		{"services", "--yes"},
		{"restart", "services", "cache"},
	} {
		if _, err := parseArguments(arguments); err == nil {
			t.Fatalf("unsafe/incomplete arguments were accepted: %v", arguments)
		}
	}
}

func TestParseArgumentsAcceptsOperationLogListAndShow(t *testing.T) {
	list, err := parseArguments([]string{"logs", "--json"})
	if err != nil || list.command != "logs" || list.subcommand != "" || !list.json {
		t.Fatalf("logs = %+v, error = %v", list, err)
	}
	id := "deploy-20260914T113000.000000000Z-11.log"
	show, err := parseArguments([]string{"logs", "show", id, "--json"})
	if err != nil || show.command != "logs" || show.subcommand != "show" || show.logID != id || !show.json {
		t.Fatalf("logs show = %+v, error = %v", show, err)
	}
	for _, arguments := range [][]string{
		{"logs", "show"},
		{"show", "logs", id},
		{"logs", "show", id, "another"},
	} {
		if _, err := parseArguments(arguments); err == nil {
			t.Fatalf("invalid log arguments were accepted: %v", arguments)
		}
	}
}

func TestParseArgumentsAcceptsOnlyGitReview(t *testing.T) {
	options, err := parseArguments([]string{"git", "review", "--json"})
	if err != nil || options.command != "git" || options.subcommand != "review" || !options.json {
		t.Fatalf("git review = %+v, error = %v", options, err)
	}
	for _, arguments := range [][]string{{"git"}, {"review", "git"}, {"git", "review", "review"}} {
		if _, err := parseArguments(arguments); err == nil {
			t.Fatalf("invalid Git arguments were accepted: %v", arguments)
		}
	}
}

func TestParseArgumentsAcceptsGitCommitPlanAndApply(t *testing.T) {
	plan, err := parseArguments([]string{"git", "commit", "plan", "--paths", "lab-settings.json,keys/admin-ssh.pub", "--json"})
	if err != nil || plan.command != "git" || plan.subcommand != "commit-plan" || plan.paths == "" || !plan.json {
		t.Fatalf("Git commit plan = %+v, error = %v", plan, err)
	}
	apply, err := parseArguments([]string{"git", "commit", "apply", "--paths", "lab-settings.json", "--expect", "sha256:review", "--yes", "--json"})
	if err != nil || apply.subcommand != "commit-apply" || apply.paths != "lab-settings.json" || apply.expect != "sha256:review" || !apply.yes {
		t.Fatalf("Git commit apply = %+v, error = %v", apply, err)
	}
	for _, arguments := range [][]string{
		{"git", "commit"},
		{"git", "commit", "plan"},
		{"git", "commit", "apply", "--paths", "lab-settings.json"},
		{"git", "review", "--paths", "lab-settings.json"},
		{"status", "--paths", "flake.nix"},
	} {
		if _, err := parseArguments(arguments); err == nil {
			t.Fatalf("incomplete/unsafe Git commit arguments were accepted: %v", arguments)
		}
	}
}

func TestParseArgumentsAcceptsUpdatePlanAndApplyPolicy(t *testing.T) {
	check, err := parseArguments([]string{"update", "check", "--json"})
	if err != nil || check.command != "update" || check.subcommand != "check" || !check.json {
		t.Fatalf("update check = %+v, error = %v", check, err)
	}
	plan, err := parseArguments([]string{"update", "plan", "--target", "v2.1.0-beta.1", "--allow-prerelease", "--json"})
	if err != nil || plan.command != "update" || plan.subcommand != "plan" || plan.target != "v2.1.0-beta.1" || !plan.allowPrerelease {
		t.Fatalf("update plan = %+v, error = %v", plan, err)
	}
	apply, err := parseArguments([]string{"update", "apply", "--target", "v1.9.0", "--allow-downgrade", "--expect", "sha256:review", "--yes", "--json"})
	if err != nil || apply.subcommand != "apply" || !apply.allowDowngrade || apply.expect != "sha256:review" || !apply.yes {
		t.Fatalf("update apply = %+v, error = %v", apply, err)
	}
	for _, arguments := range [][]string{
		{"update", "plan"},
		{"update", "apply", "--target", "v2.1.0"},
		{"status", "--target", "v2.1.0"},
		{"status", "--allow-prerelease"},
		{"update", "check", "--target", "v2.1.0"},
		{"update", "check", "--allow-prerelease"},
	} {
		if _, err := parseArguments(arguments); err == nil {
			t.Fatalf("invalid update arguments accepted: %v", arguments)
		}
	}
}

func TestParseArgumentsRejectsUnknownInput(t *testing.T) {
	if _, err := parseArguments([]string{"deploy"}); err == nil {
		t.Fatal("unknown command was accepted")
	}
}

func TestParseArgumentsRestrictsFullToDoctor(t *testing.T) {
	options, err := parseArguments([]string{"doctor", "--full"})
	if err != nil || !options.full {
		t.Fatalf("doctor --full = %+v, %v", options, err)
	}
	if _, err := parseArguments([]string{"status", "--full"}); err == nil {
		t.Fatal("status --full was accepted")
	}
}

func TestParseArgumentsAcceptsConfigValidate(t *testing.T) {
	options, err := parseArguments([]string{"config", "validate", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if options.command != "config" || options.subcommand != "validate" || !options.json {
		t.Fatalf("unexpected options: %+v", options)
	}
	if _, err := parseArguments([]string{"config"}); err == nil {
		t.Fatal("config without validate was accepted")
	}
	if _, err := parseArguments([]string{"validate", "config"}); err == nil {
		t.Fatal("validate before config was accepted")
	}
}

func TestParseArgumentsAcceptsReviewedConfigApply(t *testing.T) {
	options, err := parseArguments([]string{"config", "apply", "--file", "/tmp/candidate.json", "--expect", "sha256:value", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if options.subcommand != "apply" || options.file != "/tmp/candidate.json" || options.expect != "sha256:value" || !options.json {
		t.Fatalf("unexpected options: %+v", options)
	}
	if _, err := parseArguments([]string{"config", "apply", "--file", "/tmp/candidate.json"}); err == nil {
		t.Fatal("apply without reviewed fingerprint was accepted")
	}
	if _, err := parseArguments([]string{"config", "plan"}); err == nil {
		t.Fatal("plan without candidate file was accepted")
	}
}

func TestParseArgumentsDistinguishesSetupApply(t *testing.T) {
	options, err := parseArguments([]string{"setup", "apply", "--yes", "--json"})
	if err != nil || options.subcommand != "apply" || !options.yes {
		t.Fatalf("options = %+v, error = %v", options, err)
	}
	if _, err := parseArguments([]string{"setup", "status", "--yes"}); err == nil {
		t.Fatal("setup status accepted --yes")
	}
}

func TestParseArgumentsAcceptsSetupStatus(t *testing.T) {
	options, err := parseArguments([]string{"setup", "--json", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if options.command != "setup" || options.subcommand != "status" || !options.json {
		t.Fatalf("unexpected options: %+v", options)
	}
	bare, err := parseArguments([]string{"setup"})
	if err != nil || bare.subcommand != "configure" || !bare.guided {
		t.Fatalf("bare setup = %+v, %v", bare, err)
	}
}

func TestParseArgumentsRejectsJSONInteractiveSetup(t *testing.T) {
	if _, err := parseArguments([]string{"setup", "configure", "--json"}); err == nil {
		t.Fatal("interactive setup accepted --json")
	}
}

func TestParseArgumentsAcceptsInstallSecrets(t *testing.T) {
	options, err := parseArguments([]string{"setup", "install-secrets", "--json"})
	if err != nil || options.subcommand != "install-secrets" || !options.json {
		t.Fatalf("options = %+v, error = %v", options, err)
	}
}

func TestParseArgumentsAcceptsPXEPrepare(t *testing.T) {
	options, err := parseArguments([]string{"pxe", "prepare", "--json"})
	if err != nil || options.command != "pxe" || options.subcommand != "prepare" || !options.json {
		t.Fatalf("options = %+v, error = %v", options, err)
	}
	if _, err := parseArguments([]string{"pxe"}); err == nil {
		t.Fatal("bare pxe command was accepted")
	}
	if _, err := parseArguments([]string{"prepare", "pxe"}); err == nil {
		t.Fatal("prepare before pxe was accepted")
	}
}

func TestParseArgumentsAcceptsPXELifecycle(t *testing.T) {
	for _, subcommand := range []string{"start", "stop", "recover"} {
		arguments := []string{"pxe", subcommand, "--json"}
		if subcommand == "start" {
			arguments = append(arguments, "--yes")
		}
		options, err := parseArguments(arguments)
		if err != nil || options.command != "pxe" || options.subcommand != subcommand || !options.json {
			t.Fatalf("%s options = %+v, error = %v", subcommand, options, err)
		}
	}
	if _, err := parseArguments([]string{"pxe", "stop", "--yes"}); err == nil {
		t.Fatal("pxe stop accepted --yes")
	}
	if _, err := parseArguments([]string{"start", "pxe"}); err == nil {
		t.Fatal("start before pxe was accepted")
	}
}

func TestOnlyPXECleanupCanRunWithoutRepository(t *testing.T) {
	tests := []struct {
		options  options
		required bool
	}{
		{options: options{command: "status"}, required: true},
		{options: options{command: "logs"}, required: false},
		{options: options{command: "git", subcommand: "review"}, required: true},
		{options: options{command: "pxe", subcommand: "prepare"}, required: true},
		{options: options{command: "pxe", subcommand: "start"}, required: true},
		{options: options{command: "pxe", subcommand: "stop"}, required: false},
		{options: options{command: "pxe", subcommand: "recover"}, required: false},
	}
	for _, test := range tests {
		if actual := commandRequiresRepository(test.options); actual != test.required {
			t.Errorf("commandRequiresRepository(%+v) = %v, want %v", test.options, actual, test.required)
		}
	}
}

func TestParseArgumentsAcceptsSetupKeys(t *testing.T) {
	options, err := parseArguments([]string{"setup", "keys", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if options.command != "setup" || options.subcommand != "keys" || !options.json {
		t.Fatalf("unexpected options: %+v", options)
	}
	if _, err := parseArguments([]string{"keys", "setup"}); err == nil {
		t.Fatal("keys before setup was accepted")
	}
}

func TestParseArgumentsAcceptsReadOnlyKeyVerification(t *testing.T) {
	options, err := parseArguments([]string{"setup", "keys", "--verify-only", "--json"})
	if err != nil || !options.verifyOnly {
		t.Fatalf("options = %+v, error = %v", options, err)
	}
	if _, err := parseArguments([]string{"status", "--verify-only"}); err == nil {
		t.Fatal("status accepted --verify-only")
	}
}

func TestResolveRepositoryUsesConfiguredRoot(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "flake.nix"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NIXORIUM_REPO", directory)

	resolved, err := resolveRepository("")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != directory {
		t.Fatalf("resolved = %q, want %q", resolved, directory)
	}
}

func TestResolveRepositoryRejectsMissingFlake(t *testing.T) {
	if _, err := resolveRepository(t.TempDir()); err == nil {
		t.Fatal("repository without flake.nix was accepted")
	}
}

func TestReadCandidateSettingsRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.json")
	if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "candidate.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readCandidateSettings(link); err == nil {
		t.Fatal("candidate symlink was accepted")
	}
}
