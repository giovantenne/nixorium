package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseArgumentsAllowsFlagsBeforeOrAfterCommand(t *testing.T) {
	options, err := parseArguments([]string{"status", "--json", "--repo", "/tmp/site"})
	if err != nil {
		t.Fatal(err)
	}
	if options.command != "status" || !options.json || options.repository != "/tmp/site" {
		t.Fatalf("unexpected options: %+v", options)
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
