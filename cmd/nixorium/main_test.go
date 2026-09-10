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

func TestParseArgumentsAcceptsSetupStatus(t *testing.T) {
	options, err := parseArguments([]string{"setup", "--json", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if options.command != "setup" || options.subcommand != "status" || !options.json {
		t.Fatalf("unexpected options: %+v", options)
	}
	if _, err := parseArguments([]string{"setup"}); err == nil {
		t.Fatal("setup without status was accepted")
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
