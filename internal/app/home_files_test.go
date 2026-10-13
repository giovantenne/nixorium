package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeHomeFilesSource struct {
	base        domain.HomeFilesProfile
	exists      bool
	fingerprint string
	writeErr    error
	writes      int
	invalid     bool
}

func (f *fakeHomeFilesSource) ReadHomeFiles(string) ([]byte, error) {
	if !f.exists {
		return nil, fs.ErrNotExist
	}
	return domain.MarshalHomeFilesProfile(f.base)
}
func (f *fakeHomeFilesSource) CaptureHomeFiles(context.Context, string, string, string) (domain.HomeFilesCaptureReport, error) {
	return domain.HomeFilesCaptureReport{}, errors.New("unused")
}
func fakeHomeEntries(p domain.HomeFilesProfile) []domain.HomeFilesEntry {
	entries := []domain.HomeFilesEntry{}
	for _, d := range p.Directories {
		entries = append(entries, domain.HomeFilesEntry{Destination: d, Directory: true})
	}
	for _, e := range p.Files {
		text := "same"
		if e.Text != nil {
			text = *e.Text
		}
		entries = append(entries, domain.HomeFilesEntry{Destination: e.Destination, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(text))), Size: int64(len(text)), Executable: e.Executable})
	}
	return entries
}
func (f *fakeHomeFilesSource) InspectHomeFiles(_ context.Context, _ string, d domain.HomeFilesDraft) (domain.HomeFilesInspection, error) {
	i := domain.HomeFilesInspection{Base: f.base, BaseEntries: fakeHomeEntries(f.base), Entries: fakeHomeEntries(d.Profile), StudentUser: "learner", Targets: []domain.WorkspaceTarget{{Name: "pc99", Role: "controller"}, {Name: "pc01", Role: "client"}}, Snapshot: domain.HomeFilesSnapshot{Revision: strings.Repeat("a", 40), SourceFingerprint: "sha256:" + strings.Repeat("a", 64), PinFingerprint: "sha256:" + strings.Repeat("b", 64), BaseFingerprint: "sha256:" + strings.Repeat("c", 64), PayloadFingerprint: f.fingerprint, BaseExists: f.exists}}
	if f.invalid {
		i.Targets[0].Role = "client"
	}
	return i, nil
}
func (f *fakeHomeFilesSource) WriteHomeFilesIfUnchanged(context.Context, string, domain.HomeFilesSnapshot, domain.HomeFilesDraft) error {
	f.writes++
	return f.writeErr
}
func homeFilesManagerFixture() (*fakeHomeFilesSource, HomeFilesManager, domain.HomeFilesDraft) {
	text := "same"
	source := &fakeHomeFilesSource{base: domain.HomeFilesProfile{SchemaVersion: 1}, fingerprint: "sha256:" + strings.Repeat("d", 64)}
	return source, NewHomeFilesManager(source), domain.HomeFilesDraft{Profile: domain.HomeFilesProfile{SchemaVersion: 1, Files: []domain.HomeFile{{Destination: "Documents/lesson", Text: &text}}}}
}

func TestHomeFilesReviewBindsMetadataAndDraft(t *testing.T) {
	source, manager, draft := homeFilesManagerFixture()
	draft.CaptureIDs = []string{strings.Repeat("b", 32), strings.Repeat("a", 32)}
	plan := manager.Plan(t.Context(), "/deployment", draft)
	if plan.HasErrors() || len(plan.Changes) != 1 || plan.Changes[0].Kind != "added" || draft.CaptureIDs[0] != strings.Repeat("b", 32) {
		t.Fatal(plan)
	}
	source.fingerprint = "sha256:" + strings.Repeat("e", 64)
	if r := manager.Apply(t.Context(), "/deployment", draft, plan.ReviewToken); r.State != "conflict" || source.writes != 0 {
		t.Fatal(r)
	}
	source.invalid = true
	if r := manager.Plan(t.Context(), "/deployment", draft); !r.HasErrors() || r.ReviewToken != "" {
		t.Fatal("invalid controller metadata accepted", r)
	}
}
func TestHomeFilesReviewProvenanceAndPartial(t *testing.T) {
	source, manager, draft := homeFilesManagerFixture()
	source.base = draft.Profile
	source.exists = true
	hash := "imports/" + fmt.Sprintf("%x", sha256.Sum256([]byte("same")))
	draft.Profile.Files = []domain.HomeFile{{Destination: "Documents/lesson", Source: &hash}}
	plan := manager.Plan(t.Context(), "/deployment", draft)
	if plan.HasErrors() || plan.State != "ready" || len(plan.Changes) != 1 || plan.Changes[0].Kind != "declaration" || plan.ReviewToken != homeFilesToken(plan) {
		t.Fatal(plan)
	}
	source.writeErr = domain.ErrHomeFilesDurability
	r := manager.Apply(t.Context(), "/deployment", draft, plan.ReviewToken)
	if r.State != "partial" || !r.RecoveryRequired || !strings.Contains(r.Message, "Git review") {
		t.Fatal(r)
	}
}
func TestHomeFilesCancelledSaveDoesNotWrite(t *testing.T) {
	source, manager, draft := homeFilesManagerFixture()
	plan := manager.Plan(t.Context(), "/deployment", draft)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if r := manager.Apply(ctx, "/deployment", draft, plan.ReviewToken); r.State != "failed" || source.writes != 0 {
		t.Fatal(r)
	}
	draft.BaseFingerprint = "sha256:" + strings.Repeat("f", 64)
	if r := manager.Plan(t.Context(), "/deployment", draft); r.State != "conflict" {
		t.Fatal(r)
	}
}
