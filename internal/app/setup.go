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
	GitState(ctx context.Context, repository string) (domain.GitState, error)
	ControllerApplied(ctx context.Context, repository string) (bool, string)
	ArtifactState(repository, name, relativePath string) domain.ArtifactState
	PXEPreparation(ctx context.Context, repository string, meta domain.LabMeta) domain.PXEPreparationState
	CommandAvailable(name string) bool
	KeyMaterial(ctx context.Context, repository string) []domain.KeyMaterialState
	ReconcileKeyMaterial(ctx context.Context, repository string) error
	ImportKeyMaterial(ctx context.Context, repository, name, sourcePath string) (domain.KeyImportEvidence, error)
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

func (m SetupManager) ImportKey(ctx context.Context, repository, name, sourcePath string) (domain.KeyImportReport, error) {
	report := domain.KeyImportReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "setup-key-import",
		State:         "invalid",
		Repository:    repository,
		Key:           name,
		Source:        sourcePath,
		Issues:        []domain.ValidationIssue{},
	}
	evidence, err := m.source.ImportKeyMaterial(ctx, repository, name, sourcePath)
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "source", Message: err.Error()})
		report.Message = "The key was not imported; no existing key was replaced."
		return report, err
	}
	report.State = "imported"
	report.Source = evidence.Source
	report.Fingerprint = evidence.Fingerprint
	report.Message = "Existing key imported and verified. The source file was left unchanged."
	return report, nil
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
	facts := domain.SetupFacts{}
	if gitState, err := m.source.GitState(ctx, repository); err != nil {
		facts.Review.Detail = fmt.Sprintf("local configuration history is unavailable: %v", err)
	} else if pending := pendingManagedSetupPaths(gitState.Paths); len(pending) > 0 {
		facts.Review.Detail = fmt.Sprintf("save %d pending managed configuration file(s)", len(pending))
	} else {
		facts.Review.Complete = true
		facts.Review.Detail = "managed configuration is saved locally"
	}

	missingCommands := []string{}
	for _, command := range []string{"nix", "git"} {
		if !m.source.CommandAvailable(command) {
			missingCommands = append(missingCommands, command)
		}
	}
	facts.Environment.Complete = len(missingCommands) == 0
	if facts.Environment.Complete {
		facts.Environment.Detail = "deployment repository and required local tools are available"
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
	controllerOnly := settings.Lab.DeploymentMode == "controller"
	facts.Network.Complete = !invalidJSON && !hasIssuePrefix(issues, "lab.masterDhcpIp", "lab.network", "lab.pcCount", "lab.masterHostNumber", "lab.ifaceName", "lab.controllerIfaceName", "lab.clientIfaceName", "lab.hostIfaceNames") &&
		(controllerOnly || settings.Lab.MasterDHCPIP != domain.MasterDHCPPlaceholder)
	if facts.Network.Complete {
		if controllerOnly && settings.Lab.MasterDHCPIP == domain.MasterDHCPPlaceholder {
			facts.Network.Detail = "client installation network is deferred until Install computers is opened"
		} else {
			facts.Network.Detail = fmt.Sprintf("%s/%d on %s; controller DHCP %s", settings.Lab.NetworkBase, settings.Lab.NetworkPrefix, settings.Lab.ControllerInterface(), settings.Lab.MasterDHCPIP)
		}
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

	settingsReady := facts.Network.Complete && facts.Identity.Complete && facts.Credentials.Complete
	if settingsReady {
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
	} else {
		facts.Keys.Detail = "complete network, identity, and credentials before checking controller keys"
	}

	evaluatedMeta := domain.LabMeta{}
	localPrerequisitesReady := len(issues) == 0 && settingsReady && facts.Keys.Complete
	facts.Validation.Complete = localPrerequisitesReady
	if localPrerequisitesReady {
		if meta, err := m.source.LabMeta(ctx, repository); err != nil {
			facts.Validation.Complete = false
			facts.Validation.Detail = fmt.Sprintf("Nix evaluation failed: %v", err)
		} else {
			evaluatedMeta = meta
			facts.Validation.Detail = "management schema and Nix evaluation pass"
		}
	} else if len(issues) > 0 {
		facts.Validation.Detail = firstIssue(issues, "settings validation failed")
	} else {
		facts.Validation.Detail = "complete settings, credentials, and keys before Nix validation"
	}
	if facts.Validation.Complete {
		facts.Apply.Complete, facts.Apply.Detail = m.source.ControllerApplied(ctx, repository)
	} else {
		facts.Apply.Detail = "controller apply requires a valid candidate configuration"
	}

	preparation := domain.PXEPreparationState{}
	if facts.Validation.Complete {
		preparation = m.source.PXEPreparation(ctx, repository, evaluatedMeta)
	}
	if facts.Validation.Complete {
		artifacts := []domain.ArtifactState{
			m.source.ArtifactState(repository, "kernel", "result-kernel/bzImage"),
			m.source.ArtifactState(repository, "initrd", "result-initrd/initrd"),
			m.source.ArtifactState(repository, "iPXE script", "result-ipxe/netboot.ipxe"),
			m.source.ArtifactState(repository, "iPXE firmware", "assets/ipxe/snponly.efi"),
		}
		if preparation.Ready {
			artifacts = preparation.Artifacts
		}
		missingArtifacts := []string{}
		for _, artifact := range artifacts {
			if !artifact.Present {
				missingArtifacts = append(missingArtifacts, artifact.Name)
			}
		}
		facts.Artifacts.Complete = len(missingArtifacts) == 0
		if facts.Artifacts.Complete {
			if preparation.Ready {
				facts.Artifacts.Detail = preparation.Detail
			} else {
				facts.Artifacts.Detail = "all compatibility installation artifacts are present"
			}
		} else {
			facts.Artifacts.Detail = "missing: " + strings.Join(missingArtifacts, ", ")
			if preparation.Present && preparation.Detail != "" {
				facts.Artifacts.Detail = preparation.Detail
			}
		}
	} else {
		facts.Artifacts.Detail = "prepare installation artifacts after configuration validation"
	}

	return domain.ReconcileSetup(repository, facts)
}

func pendingManagedSetupPaths(paths []string) []string {
	managed := map[string]bool{
		"lab-settings.json":         true,
		"keys/cache-public-key":     true,
		"keys/admin-ssh.pub":        true,
		"keys/veyon-public-key.pem": true,
	}
	pending := []string{}
	for _, path := range paths {
		path = strings.TrimSpace(strings.Trim(path, `"`))
		if managed[path] {
			pending = append(pending, path)
		}
	}
	return pending
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
