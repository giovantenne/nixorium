package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type SetupSource interface {
	ReadSettings(repository string) ([]byte, error)
	LabMeta(ctx context.Context, repository string) (domain.LabMeta, error)
	DeploymentStatus(ctx context.Context, repository string) (domain.DeploymentStatus, error)
	GitState(ctx context.Context, repository string) (domain.GitState, error)
	ControllerApplied(ctx context.Context, repository string) (bool, string)
	ArtifactState(repository, name, relativePath string) domain.ArtifactState
	CommandAvailable(name string) bool
	KeyMaterial(ctx context.Context, repository string) []domain.KeyMaterialState
	ReconcileKeyMaterial(ctx context.Context, repository string) error
}

type SetupManager struct {
	source SetupSource
}

func NewSetupManager(source SetupSource) SetupManager {
	return SetupManager{source: source}
}

func (m SetupManager) ReconcileKeys(ctx context.Context, repository string) (domain.KeyReconcileReport, error) {
	reconcileErr := m.source.ReconcileKeyMaterial(ctx, repository)
	return m.keyReport(ctx, repository, "setup-keys"), reconcileErr
}

func (m SetupManager) VerifyKeys(ctx context.Context, repository string) domain.KeyReconcileReport {
	return m.keyReport(ctx, repository, "setup-keys-verify")
}

func (m SetupManager) keyReport(ctx context.Context, repository, operation string) domain.KeyReconcileReport {
	states := m.source.KeyMaterial(ctx, repository)
	report := domain.KeyReconcileReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     operation,
		State:         "ready",
		Repository:    repository,
		Keys:          states,
	}
	for _, state := range states {
		if !state.Ready() {
			report.State = "action-required"
			break
		}
	}
	return report
}

func (m SetupManager) Status(ctx context.Context, repository string) domain.SetupReport {
	facts := domain.SetupFacts{
		Install: domain.SetupObservation{Detail: "guided client installation is not available yet"},
	}
	if gitState, err := m.source.GitState(ctx, repository); err != nil {
		facts.Review.Detail = fmt.Sprintf("Git review state is unavailable: %v", err)
	} else if gitState.Dirty {
		facts.Review.Detail = fmt.Sprintf("review and accept %d Git worktree change(s)", gitState.Changes)
	} else {
		facts.Review.Complete = true
		facts.Review.Detail = "deployment worktree is clean; reviewed changes are recorded"
	}

	missingCommands := []string{}
	for _, command := range []string{"nix", "git"} {
		if !m.source.CommandAvailable(command) {
			missingCommands = append(missingCommands, command)
		}
	}
	facts.Environment.Complete = len(missingCommands) == 0
	if facts.Environment.Complete {
		facts.Environment.Detail = "deployment repository, Nix, and Git are available"
	} else {
		facts.Environment.Detail = "missing commands: " + strings.Join(missingCommands, ", ")
	}

	data, readErr := m.source.ReadSettings(repository)
	settings := domain.LabSettingsFile{}
	issues := []domain.ValidationIssue{}
	if readErr != nil {
		issues = append(issues, domain.ValidationIssue{Field: "$", Message: readErr.Error()})
	} else {
		settings, issues = domain.DecodeLabSettings(data)
	}
	invalidJSON := hasIssueFor(issues, "$", "schemaVersion")
	facts.Network.Complete = !invalidJSON && !hasIssuePrefix(issues, "lab.masterDhcpIp", "lab.network", "lab.pcCount", "lab.masterHostNumber", "lab.ifaceName") && settings.Lab.MasterDHCPIP != domain.MasterDHCPPlaceholder
	if facts.Network.Complete {
		facts.Network.Detail = fmt.Sprintf("%s/%d on %s; controller DHCP %s", settings.Lab.NetworkBase, settings.Lab.NetworkPrefix, settings.Lab.InterfaceName, settings.Lab.MasterDHCPIP)
	} else if settings.Lab.MasterDHCPIP == domain.MasterDHCPPlaceholder {
		facts.Network.Detail = "controller DHCP address still uses MASTER_DHCP_IP"
	} else {
		facts.Network.Detail = firstIssue(issues, "network settings are incomplete or invalid")
	}

	facts.Identity.Complete = !invalidJSON && !hasIssuePrefix(issues,
		"lab.teacherUser", "lab.studentUser", "lab.homepageUrl", "lab.studentGit", "lab.adminGit",
		"lab.timeZone", "lab.defaultLocale", "lab.extraLocale", "lab.keyboardLayout", "lab.consoleKeyMap", "lab.veyonNativeHosts")
	if facts.Identity.Complete {
		facts.Identity.Detail = fmt.Sprintf("users %s/%s; timezone %s", settings.Lab.TeacherUser, settings.Lab.StudentUser, settings.Lab.TimeZone)
	} else {
		facts.Identity.Detail = firstIssue(issues, "lab identity or locale is incomplete")
	}

	facts.Credentials.Complete = !invalidJSON && !hasIssuePrefix(issues, "lab.teacherPassword", "lab.studentPassword", "lab.adminPassword") &&
		settings.Lab.TeacherPassword != domain.DefaultPasswordHash &&
		settings.Lab.StudentPassword != domain.DefaultPasswordHash &&
		settings.Lab.AdminPassword != domain.DefaultPasswordHash
	if facts.Credentials.Complete {
		facts.Credentials.Detail = "all account password hashes differ from the public default"
	} else {
		facts.Credentials.Detail = "one or more account hashes are missing, invalid, or still use the public default"
	}

	keyStates := m.source.KeyMaterial(ctx, repository)
	keyProblems := []string{}
	for _, state := range keyStates {
		if !state.Ready() {
			problem := state.Name
			if state.Problem != "" {
				problem += ": " + state.Problem
			}
			keyProblems = append(keyProblems, problem)
		}
	}
	facts.Keys.Complete = len(keyStates) == 3 && len(keyProblems) == 0
	if facts.Keys.Complete {
		facts.Keys.Detail = "cache, SSH, and Veyon pairs have safe private modes and verified correspondence"
	} else {
		facts.Keys.Detail = "incomplete key pairs: " + strings.Join(keyProblems, ", ")
	}

	facts.Validation.Complete = len(issues) == 0
	if facts.Validation.Complete {
		if _, err := m.source.LabMeta(ctx, repository); err != nil {
			facts.Validation.Complete = false
			facts.Validation.Detail = fmt.Sprintf("Nix evaluation failed: %v", err)
		} else {
			facts.Validation.Detail = "management schema and Nix evaluation pass"
		}
	} else {
		facts.Validation.Detail = firstIssue(issues, "settings validation failed")
	}
	if facts.Validation.Complete {
		facts.Apply.Complete, facts.Apply.Detail = m.source.ControllerApplied(ctx, repository)
	} else {
		facts.Apply.Detail = "controller apply requires a valid candidate configuration"
	}

	artifacts := []domain.ArtifactState{
		m.source.ArtifactState(repository, "kernel", "result-kernel/bzImage"),
		m.source.ArtifactState(repository, "initrd", "result-initrd/initrd"),
		m.source.ArtifactState(repository, "iPXE script", "result-ipxe/netboot.ipxe"),
		m.source.ArtifactState(repository, "iPXE firmware", "assets/ipxe/snponly.efi"),
	}
	missingArtifacts := []string{}
	for _, artifact := range artifacts {
		if !artifact.Present {
			missingArtifacts = append(missingArtifacts, artifact.Name)
		}
	}
	facts.Artifacts.Complete = len(missingArtifacts) == 0
	if facts.Artifacts.Complete {
		facts.Artifacts.Detail = "all installation artifacts are prepared"
	} else {
		facts.Artifacts.Detail = "missing: " + strings.Join(missingArtifacts, ", ")
	}

	if readiness, err := m.source.DeploymentStatus(ctx, repository); err == nil {
		facts.Readiness.Complete = readiness.Ready
		if readiness.Ready {
			facts.Readiness.Detail = "deploymentStatus.ready is true"
		} else {
			facts.Readiness.Detail = strings.Join(readiness.Issues, "; ")
		}
	} else {
		facts.Readiness.Detail = fmt.Sprintf("deployment readiness evaluation failed: %v", err)
	}
	return domain.ReconcileSetup(repository, facts)
}

func hasIssueFor(issues []domain.ValidationIssue, fields ...string) bool {
	for _, issue := range issues {
		for _, field := range fields {
			if issue.Field == field {
				return true
			}
		}
	}
	return false
}

func hasIssuePrefix(issues []domain.ValidationIssue, prefixes ...string) bool {
	for _, issue := range issues {
		for _, prefix := range prefixes {
			if strings.HasPrefix(issue.Field, prefix) {
				return true
			}
		}
	}
	return false
}

func firstIssue(issues []domain.ValidationIssue, fallback string) string {
	if len(issues) == 0 {
		return fallback
	}
	return issues[0].Field + ": " + issues[0].Message
}
