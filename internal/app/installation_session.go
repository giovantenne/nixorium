package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type InstallationSessionSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	GitRevision(context.Context, string) (string, error)
	InstallationSession(string) (domain.InstallationSessionRecord, bool, error)
	WriteInstallationSession(string, domain.InstallationSessionRecord) error
}

type InstallationHostObserver interface {
	Host(context.Context, string, string) (domain.HostsReport, error)
}

type InstallationSessionManager struct {
	source   InstallationSessionSource
	observer InstallationHostObserver
	now      func() time.Time
}

func NewInstallationSessionManager(source InstallationSessionSource, observer InstallationHostObserver) InstallationSessionManager {
	return InstallationSessionManager{source: source, observer: observer, now: time.Now}
}

func (m InstallationSessionManager) Status(ctx context.Context, repository string) domain.InstallationSessionReport {
	root, err := filepath.Abs(repository)
	if err != nil {
		return installationSessionFailure("installation-session-status", repository, "repository", err.Error())
	}
	record, present, err := m.source.InstallationSession(root)
	if err != nil {
		return installationSessionFailure("installation-session-status", root, "session", err.Error())
	}
	if !present {
		return domain.InstallationSessionReport{
			SchemaVersion: domain.InstallationSessionSchemaVersion,
			Operation:     "installation-session-status",
			State:         "none",
			Repository:    root,
			Evidence:      []domain.InstallationEvidence{},
			Issues:        []domain.ValidationIssue{},
			Message:       "No resumable installation session is recorded.",
		}
	}
	report := installationSessionReport("installation-session-status", record)
	revision, err := m.source.GitRevision(ctx, root)
	if err != nil {
		return installationSessionIssue(report, "revision", "inspect current Git revision: "+err.Error())
	}
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		return installationSessionIssue(report, "configuration", "evaluate client inventory: "+err.Error())
	}
	if record.Revision != revision {
		report.State = "stale"
		report.Message = "The saved installation session belongs to an older laboratory revision. Selecting a computer starts a new session."
		return report
	}
	configured := make(map[string]bool, len(meta.Clients.Hosts))
	for _, host := range meta.Clients.Hosts {
		configured[host.Name] = true
	}
	if !configured[record.Selected] {
		report.State = "stale"
		report.Message = "The selected computer is no longer in the evaluated inventory. Selecting a computer starts a new session."
		return report
	}
	for _, evidence := range record.Evidence {
		if !configured[evidence.Name] {
			report.State = "stale"
			report.Message = "The saved session contains an identity that is no longer configured. Selecting a computer starts a new session."
			return report
		}
	}
	report.Message = "Installation session restored from private operator state."
	return report
}

func (m InstallationSessionManager) Select(ctx context.Context, repository, name string) domain.InstallationSessionReport {
	root, err := filepath.Abs(repository)
	if err != nil {
		return installationSessionFailure("installation-session-select", repository, "repository", err.Error())
	}
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		return installationSessionFailure("installation-session-select", root, "configuration", err.Error())
	}
	if !installationInventoryContains(meta.Clients.Hosts, name) {
		return installationSessionFailure("installation-session-select", root, "target", fmt.Sprintf("client identity %q is not in the evaluated inventory", name))
	}
	revision, err := m.source.GitRevision(ctx, root)
	if err != nil {
		return installationSessionFailure("installation-session-select", root, "revision", err.Error())
	}
	record, present, err := m.source.InstallationSession(root)
	if err != nil {
		return installationSessionFailure("installation-session-select", root, "session", err.Error())
	}
	now := m.now().UTC()
	startedNew := !present || record.Revision != revision || !installationSessionMatchesInventory(record, meta.Clients.Hosts)
	if startedNew {
		record = domain.InstallationSessionRecord{
			SchemaVersion: domain.InstallationSessionSchemaVersion,
			Repository:    root,
			State:         domain.InstallationSessionActive,
			Revision:      revision,
			StartedAt:     now,
			Evidence:      []domain.InstallationEvidence{},
		}
	} else {
		now = installationSessionTimestamp(record, now)
	}
	record.Selected = name
	record.UpdatedAt = now
	if err := m.source.WriteInstallationSession(root, record); err != nil {
		return installationSessionFailure("installation-session-select", root, "session", err.Error())
	}
	report := installationSessionReport("installation-session-select", record)
	report.Message = name + " selected for this installation session."
	if startedNew && present {
		report.Message = name + " selected in a new session for the current laboratory configuration; older session evidence was not reused."
	}
	return report
}

func (m InstallationSessionManager) Verify(ctx context.Context, repository, name string) domain.InstallationSessionReport {
	status := m.Status(ctx, repository)
	status.Operation = "installation-session-verify"
	if status.HasErrors() || status.State == "none" || status.State == "stale" {
		if !status.HasErrors() {
			status.State = "blocked"
			status.Issues = append(status.Issues, domain.ValidationIssue{Field: "session", Message: "select the computer again before verification"})
		}
		return status
	}
	if status.Selected != name {
		return installationSessionIssue(status, "target", "the requested computer does not match the selected installation target")
	}
	hosts, err := m.observer.Host(ctx, status.Repository, name)
	if err != nil {
		return installationSessionIssue(status, "observation", err.Error())
	}
	if len(hosts.Hosts) != 1 || hosts.Hosts[0].Name != name {
		return installationSessionIssue(status, "observation", "focused host observation returned an unexpected identity")
	}
	host := hosts.Hosts[0]
	status.Observation = &host
	level, label, guidance := domain.ComputerCondition(host)
	if level != domain.LevelOK || host.Deployment != domain.DeploymentCurrent || host.CurrentRevision != status.Revision || host.CurrentSystem == "" {
		status.State = "attention"
		status.Message = label + ". " + guidance
		return status
	}
	record, _, err := m.source.InstallationSession(status.Repository)
	if err != nil {
		return installationSessionIssue(status, "session", err.Error())
	}
	now := installationSessionTimestamp(record, m.now().UTC())
	evidence := domain.InstallationEvidence{
		Name:                name,
		Revision:            status.Revision,
		SystemPath:          host.CurrentSystem,
		TechnicalVerifiedAt: now,
	}
	for _, previous := range record.Evidence {
		if previous.Name == name && previous.Revision == evidence.Revision && previous.SystemPath == evidence.SystemPath {
			evidence.PracticalConfirmedAt = previous.PracticalConfirmedAt
		}
	}
	record.Evidence = replaceInstallationEvidence(record.Evidence, evidence)
	record.UpdatedAt = now
	if err := m.source.WriteInstallationSession(status.Repository, record); err != nil {
		return installationSessionIssue(status, "session", err.Error())
	}
	status = installationSessionReport("installation-session-verify", record)
	status.Observation = &host
	status.Message = name + " authenticated at the saved revision; practical checks remain separate."
	return status
}

func (m InstallationSessionManager) ConfirmPractical(ctx context.Context, repository, name string) domain.InstallationSessionReport {
	status := m.Status(ctx, repository)
	status.Operation = "installation-session-confirm-practical"
	if status.HasErrors() || status.State == "none" || status.State == "stale" {
		if !status.HasErrors() {
			status.State = "blocked"
			status.Issues = append(status.Issues, domain.ValidationIssue{Field: "session", Message: "repeat technical verification before the practical check"})
		}
		return status
	}
	if status.Selected != name {
		return installationSessionIssue(status, "target", "the practical check does not match the selected installation target")
	}
	record, _, err := m.source.InstallationSession(status.Repository)
	if err != nil {
		return installationSessionIssue(status, "session", err.Error())
	}
	now := installationSessionTimestamp(record, m.now().UTC())
	found := false
	for index := range record.Evidence {
		if record.Evidence[index].Name == name && record.Evidence[index].Revision == record.Revision {
			record.Evidence[index].PracticalConfirmedAt = &now
			found = true
			break
		}
	}
	if !found {
		return installationSessionIssue(status, "evidence", "authenticated technical verification is required before recording the practical check")
	}
	record.UpdatedAt = now
	if err := m.source.WriteInstallationSession(status.Repository, record); err != nil {
		return installationSessionIssue(status, "session", err.Error())
	}
	status = installationSessionReport("installation-session-confirm-practical", record)
	status.Message = name + " practical check recorded for this revision."
	return status
}

func installationSessionReport(operation string, record domain.InstallationSessionRecord) domain.InstallationSessionReport {
	return domain.InstallationSessionReport{
		SchemaVersion: record.SchemaVersion,
		Operation:     operation,
		State:         record.State,
		Repository:    record.Repository,
		Revision:      record.Revision,
		Selected:      record.Selected,
		StartedAt:     record.StartedAt,
		UpdatedAt:     record.UpdatedAt,
		Evidence:      append([]domain.InstallationEvidence(nil), record.Evidence...),
		Issues:        []domain.ValidationIssue{},
	}
}

func installationSessionFailure(operation, repository, field, message string) domain.InstallationSessionReport {
	return domain.InstallationSessionReport{
		SchemaVersion: domain.InstallationSessionSchemaVersion,
		Operation:     operation,
		State:         "failed",
		Repository:    repository,
		Evidence:      []domain.InstallationEvidence{},
		Issues:        []domain.ValidationIssue{{Field: field, Message: message}},
		Message:       message,
	}
}

func installationSessionIssue(report domain.InstallationSessionReport, field, message string) domain.InstallationSessionReport {
	report.State = "blocked"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = message
	return report
}

func installationInventoryContains(hosts []domain.HostMeta, name string) bool {
	for _, host := range hosts {
		if host.Name == name {
			return true
		}
	}
	return false
}

func installationSessionMatchesInventory(record domain.InstallationSessionRecord, hosts []domain.HostMeta) bool {
	if !installationInventoryContains(hosts, record.Selected) {
		return false
	}
	for _, evidence := range record.Evidence {
		if !installationInventoryContains(hosts, evidence.Name) {
			return false
		}
	}
	return true
}

func replaceInstallationEvidence(existing []domain.InstallationEvidence, evidence domain.InstallationEvidence) []domain.InstallationEvidence {
	result := append([]domain.InstallationEvidence(nil), existing...)
	for index := range result {
		if result[index].Name == evidence.Name {
			result[index] = evidence
			return result
		}
	}
	return append(result, evidence)
}

func installationSessionTimestamp(record domain.InstallationSessionRecord, candidate time.Time) time.Time {
	if candidate.Before(record.UpdatedAt) {
		return record.UpdatedAt
	}
	return candidate
}
