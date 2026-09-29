package app

import (
	"context"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type SupportSource interface {
	Source
	SupportCurrentSystems(context.Context, []domain.HostMeta, time.Duration) map[string]domain.HostSystemProbe
	OperationRecords(int) ([]domain.OperationRecord, error)
	WriteSupport(context.Context, domain.SupportSnapshot) domain.SupportExportResult
}

type supportInspectorSource struct{ SupportSource }

func (s supportInspectorSource) CurrentSystems(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.HostSystemProbe {
	return s.SupportCurrentSystems(ctx, hosts, timeout)
}

type SupportManager struct {
	source SupportSource
	now    func() time.Time
}

func NewSupportManager(source SupportSource) *SupportManager {
	return &SupportManager{source: source, now: time.Now}
}

// Preview reuses the inspector, but never DoctorOptions.Full, detailed log
// reads, or mutations. Collection failures become unavailable sections, not
// arbitrary error strings in the sharing payload.
func (m *SupportManager) Preview(ctx context.Context, repository, version string) (domain.SupportSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	i := NewInspector(supportInspectorSource{m.source})
	input := domain.SupportInput{Version: version, Collected: m.now()}
	if ctx.Err() != nil {
		return domain.SupportSnapshot{}, ctx.Err()
	}
	input.Revision, _ = m.source.GitRevision(ctx, repository)
	if status, err := i.Status(ctx, repository); err == nil {
		input.Status = &status
		if ctx.Err() == nil && len(status.Meta.Clients.Hosts) <= domain.SupportMaxHosts {
			doctor := i.doctorFromStatus(ctx, status, DoctorOptions{})
			input.Doctor = &doctor
		}
	}
	if ctx.Err() == nil {
		if hosts, err := i.Hosts(ctx, repository); err == nil {
			input.Hosts = &hosts
		}
	}
	if ctx.Err() == nil {
		if records, err := m.source.OperationRecords(domain.SupportMaxOperations); err == nil {
			input.Operations = &records
		}
	}
	// A moving checkout cannot label observations as belonging to one revision.
	if ctx.Err() == nil {
		after, err := m.source.GitRevision(ctx, repository)
		if err != nil || after != input.Revision {
			input.Revision = ""
			input.Hosts = nil
		}
	}
	return domain.NewSupportSnapshot(input)
}

func (m *SupportManager) Export(ctx context.Context, snapshot domain.SupportSnapshot) domain.SupportExportResult {
	if !snapshot.Valid() {
		return domain.SupportExportResult{State: "blocked", Message: "No reviewed support snapshot is available."}
	}
	if ctx.Err() != nil {
		return domain.SupportExportResult{State: "cancelled", Message: "Export cancelled before saving."}
	}
	return m.source.WriteSupport(ctx, snapshot)
}
