package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeWorkspaceSource struct {
	inspection domain.WorkspaceInspection
	inspectErr error
	writeErr   error
	inspects   int
	writes     int
	written    domain.WorkspaceProfile
	expected   domain.WorkspaceSnapshot
	afterRead  func()
}

func (f *fakeWorkspaceSource) InspectWorkspace(_ context.Context, _ string, candidate domain.WorkspaceProfile) (domain.WorkspaceInspection, error) {
	f.inspects++
	if f.afterRead != nil {
		defer f.afterRead()
	}
	f.inspection.Resolution.Declared = candidate
	data, _ := json.Marshal(f.inspection)
	var result domain.WorkspaceInspection
	_ = json.Unmarshal(data, &result)
	return result, f.inspectErr
}

func (f *fakeWorkspaceSource) WriteWorkspaceIfUnchanged(_ context.Context, _ string, expected domain.WorkspaceSnapshot, candidate domain.WorkspaceProfile) error {
	f.writes++
	f.written, f.expected = candidate, expected
	return f.writeErr
}

func workspaceManagerFixture() (*fakeWorkspaceSource, WorkspaceManager) {
	profile := domain.WorkspaceProfile{SchemaVersion: 1}
	f := &fakeWorkspaceSource{inspection: domain.WorkspaceInspection{
		Snapshot: domain.WorkspaceSnapshot{
			Revision: strings.Repeat("a", 40), SourceFingerprint: "sha256:" + strings.Repeat("b", 64),
			PinFingerprint: "sha256:" + strings.Repeat("c", 64), BaseFingerprint: "sha256:" + strings.Repeat("d", 64),
		},
		Resolution: domain.WorkspaceResolution{
			SchemaVersion: 1, State: "prepared", ManagedFile: domain.WorkspaceFileName, StudentUser: "learner",
			Declared: profile, Effective: profile,
			Catalog: domain.WorkspaceCatalog{
				SchemaVersion: 1, Baseline: profile,
				Applications: []domain.WorkspaceApplication{{ID: "code.desktop", Package: "vscode"}},
				Extensions:   []domain.WorkspaceExtension{{ID: "example.plugin", Package: "vscode-extensions.example.plugin", RequiredPackages: []string{"nodejs"}, RequiredExtensions: []string{}}},
			},
			RequiredPackages: []string{"vscode"}, Packages: []domain.WorkspacePackage{{Package: "vscode", Version: "1.0"}},
			Extensions: []domain.WorkspaceExtension{{ID: "example.plugin", Package: "vscode-extensions.example.plugin", Version: "1.0", RequiredPackages: []string{"nodejs"}, RequiredExtensions: []string{}}},
			Targets:    []domain.WorkspaceTarget{{Name: "controller", Role: "controller"}, {Name: "pc01", Role: "client"}},
		},
	}}
	return f, NewWorkspaceManager(f)
}

var minimalWorkspace = []byte(`{"schemaVersion":1}`)

func TestWorkspaceFirstProfileIsNotAnUnchangedLegacyHome(t *testing.T) {
	source, manager := workspaceManagerFixture()
	plan := manager.Plan(t.Context(), "/deployment", minimalWorkspace)
	if plan.HasErrors() || plan.State != "ready" || plan.Confirmation != "SAVE" || plan.ReviewToken == "" || source.writes != 0 || plan.Inspection.Base != nil {
		t.Fatalf("first profile plan: %+v", plan)
	}
	result := manager.Apply(t.Context(), "/deployment", minimalWorkspace, plan.ReviewToken)
	if result.HasErrors() || result.State != "saved" || result.StudentUser != "learner" || len(result.Targets) != 2 || source.inspects != 2 || source.writes != 1 {
		t.Fatalf("save = %+v, source = %+v", result, source)
	}
	if source.expected != plan.Inspection.Snapshot || !reflect.DeepEqual(source.written, *plan.Candidate) ||
		!strings.Contains(result.Message, "No commit, build, deployment, runtime enablement or home reset") {
		t.Fatalf("unexpected save boundary: %+v", result)
	}
}

func TestWorkspaceReviewBindsCompleteProposal(t *testing.T) {
	changes := map[string]func(*fakeWorkspaceSource){
		"revision": func(f *fakeWorkspaceSource) { f.inspection.Snapshot.Revision = strings.Repeat("e", 40) },
		"source": func(f *fakeWorkspaceSource) {
			f.inspection.Snapshot.SourceFingerprint = "sha256:" + strings.Repeat("e", 64)
		},
		"pin": func(f *fakeWorkspaceSource) {
			f.inspection.Snapshot.PinFingerprint = "sha256:" + strings.Repeat("e", 64)
		},
		"base bytes or mode": func(f *fakeWorkspaceSource) {
			f.inspection.Snapshot.BaseFingerprint = "sha256:" + strings.Repeat("e", 64)
		},
		"base created": func(f *fakeWorkspaceSource) {
			f.inspection.Snapshot.BaseExists = true
			f.inspection.Base = &domain.WorkspaceProfile{SchemaVersion: 1}
		},
		"student account": func(f *fakeWorkspaceSource) { f.inspection.Resolution.StudentUser = "student2" },
		"controller":      func(f *fakeWorkspaceSource) { f.inspection.Resolution.Targets[0].Name = "other" },
		"client":          func(f *fakeWorkspaceSource) { f.inspection.Resolution.Targets[1].Name = "pc02" },
		"inventory":       func(f *fakeWorkspaceSource) { f.inspection.Resolution.Targets = f.inspection.Resolution.Targets[:1] },
		"runtime opt in": func(f *fakeWorkspaceSource) {
			seed := "/nix/store/00000000000000000000000000000000-home"
			f.inspection.Resolution.RuntimeEnabled, f.inspection.Resolution.Seed = true, &seed
		},
		"baseline": func(f *fakeWorkspaceSource) {
			f.inspection.Resolution.Catalog.Baseline.Desktop = &domain.WorkspaceDesktop{}
		},
		"effective preferences": func(f *fakeWorkspaceSource) { f.inspection.Resolution.Effective.Desktop = &domain.WorkspaceDesktop{} },
		"catalog application":   func(f *fakeWorkspaceSource) { f.inspection.Resolution.Catalog.Applications[0].Package = "vscodium" },
		"catalog dependency": func(f *fakeWorkspaceSource) {
			f.inspection.Resolution.Catalog.Extensions[0].RequiredPackages = []string{"python3"}
		},
		"required packages": func(f *fakeWorkspaceSource) { f.inspection.Resolution.RequiredPackages = []string{"vscode", "nodejs"} },
		"package version":   func(f *fakeWorkspaceSource) { f.inspection.Resolution.Packages[0].Version = "2.0" },
		"extension version": func(f *fakeWorkspaceSource) { f.inspection.Resolution.Extensions[0].Version = "2.0" },
		"extension dependency": func(f *fakeWorkspaceSource) {
			f.inspection.Resolution.Extensions[0].RequiredExtensions = []string{"example.other"}
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			source, manager := workspaceManagerFixture()
			plan := manager.Plan(t.Context(), "/deployment", minimalWorkspace)
			change(source)
			result := manager.Apply(t.Context(), "/deployment", minimalWorkspace, plan.ReviewToken)
			if result.State != "conflict" || !result.HasErrors() || source.writes != 0 {
				t.Fatalf("stale review accepted: %+v", result)
			}
		})
	}
}

func TestWorkspaceTokensBindRepositoryAndNormalizedCandidate(t *testing.T) {
	source, manager := workspaceManagerFixture()
	a := manager.Plan(t.Context(), "/deployment", []byte(`{"schemaVersion":1,"vscode":{"extensions":["b.plugin","a.plugin"]}}`))
	b := manager.Plan(t.Context(), "/deployment", []byte("{\n\"vscode\": {\"extensions\":[\"a.plugin\",\"b.plugin\"]},\"schemaVersion\":1}"))
	if a.HasErrors() || b.HasErrors() || a.ReviewToken != b.ReviewToken {
		t.Fatalf("normalization changed review: %+v %+v", a, b)
	}
	for _, item := range []struct{ repository, data string }{
		{"/other", string(minimalWorkspace)}, {"/deployment", string(minimalWorkspace)},
	} {
		result := manager.Apply(t.Context(), item.repository, []byte(item.data), a.ReviewToken)
		if result.State != "conflict" || source.writes != 0 {
			t.Fatalf("different candidate/repository accepted: %+v", result)
		}
	}
	first := manager.Plan(t.Context(), "/deployment", []byte(`{"schemaVersion":1,"desktop":{"favorites":["a.desktop","b.desktop"]}}`))
	second := manager.Plan(t.Context(), "/deployment", []byte(`{"schemaVersion":1,"desktop":{"favorites":["b.desktop","a.desktop"]}}`))
	if first.ReviewToken == second.ReviewToken {
		t.Fatal("ordered favorites must affect review")
	}
}

func TestWorkspaceUnchangedStillRequiresFreshReview(t *testing.T) {
	source, manager := workspaceManagerFixture()
	source.inspection.Snapshot.BaseExists = true
	source.inspection.Base = &domain.WorkspaceProfile{SchemaVersion: 1}
	source.inspection.Resolution.Targets = source.inspection.Resolution.Targets[:1]
	plan := manager.Plan(t.Context(), "/deployment", minimalWorkspace)
	if plan.HasErrors() || plan.State != "unchanged" || plan.ReviewToken == "" || plan.Confirmation != "" {
		t.Fatalf("unchanged plan: %+v", plan)
	}
	for _, token := range []string{"", "stale", plan.ReviewToken} {
		result := manager.Apply(t.Context(), "/deployment", minimalWorkspace, token)
		want := "conflict"
		if token == plan.ReviewToken {
			want = "unchanged"
		}
		if result.State != want || source.writes != 0 {
			t.Fatalf("unchanged apply: %+v", result)
		}
	}
}

func TestWorkspaceInvalidInputDoesNotReachAdapter(t *testing.T) {
	for _, data := range []string{`{}`, `null`, `{"schemaVersion":1,"schemaVersion":1}`, `{"schemaVersion":1,"secret":"hidden"}`} {
		source, manager := workspaceManagerFixture()
		plan := manager.Plan(t.Context(), "/deployment", []byte(data))
		result := manager.Apply(t.Context(), "/deployment", []byte(data), "old-token")
		encoded, _ := json.Marshal(plan)
		if !plan.HasErrors() || !result.HasErrors() || source.inspects != 0 || source.writes != 0 || strings.Contains(string(encoded), "hidden") {
			t.Fatalf("invalid profile reached adapter or leaked: %+v %+v", plan, result)
		}
	}
}

func TestWorkspaceInspectionAndWriteFailures(t *testing.T) {
	for _, item := range []struct {
		name           string
		inspect, write error
		state          string
		recovery       bool
	}{
		{"validation", errors.New("private deployment policy rejected the candidate"), nil, "invalid", false},
		{"inspection drift", domain.ErrWorkspaceConflict, nil, "conflict", false},
		{"write drift", nil, domain.ErrWorkspaceConflict, "conflict", false},
		{"write failure", nil, errors.New("cannot create temporary file"), "failed", false},
		{"durability", nil, domain.ErrWorkspaceDurability, "partial", true},
	} {
		t.Run(item.name, func(t *testing.T) {
			source, manager := workspaceManagerFixture()
			plan := manager.Plan(t.Context(), "/deployment", minimalWorkspace)
			source.inspectErr, source.writeErr = item.inspect, item.write
			result := manager.Apply(t.Context(), "/deployment", minimalWorkspace, plan.ReviewToken)
			if !result.HasErrors() || result.State != item.state || result.RecoveryRequired != item.recovery {
				t.Fatalf("failure misreported: %+v", result)
			}
			if item.inspect != nil && source.writes != 0 {
				t.Fatal("write after rejected inspection")
			}
		})
	}
}

func TestWorkspaceCancellationBeforeWrite(t *testing.T) {
	source, manager := workspaceManagerFixture()
	plan := manager.Plan(t.Context(), "/deployment", minimalWorkspace)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source.afterRead = cancel
	result := manager.Apply(ctx, "/deployment", minimalWorkspace, plan.ReviewToken)
	if !result.HasErrors() || source.writes != 0 {
		t.Fatalf("write after cancellation: %+v", result)
	}
}

func TestWorkspaceInvalidInspectionNeverAuthorizesWrite(t *testing.T) {
	for name, change := range map[string]func(*domain.WorkspaceInspection){
		"missing identity":       func(i *domain.WorkspaceInspection) { i.Snapshot.SourceFingerprint = "" },
		"base presence":          func(i *domain.WorkspaceInspection) { i.Snapshot.BaseExists = true },
		"version":                func(i *domain.WorkspaceInspection) { i.Resolution.SchemaVersion = 2 },
		"wrong destination file": func(i *domain.WorkspaceInspection) { i.Resolution.ManagedFile = "flake.nix" },
		"active claim":           func(i *domain.WorkspaceInspection) { i.Resolution.State = "active" },
		"bad account":            func(i *domain.WorkspaceInspection) { i.Resolution.StudentUser = "../student" },
		"missing controller":     func(i *domain.WorkspaceInspection) { i.Resolution.Targets = i.Resolution.Targets[1:] },
		"duplicate host":         func(i *domain.WorkspaceInspection) { i.Resolution.Targets[1].Name = i.Resolution.Targets[0].Name },
		"two controllers":        func(i *domain.WorkspaceInspection) { i.Resolution.Targets[1].Role = "controller" },
		"no seed":                func(i *domain.WorkspaceInspection) { i.Resolution.RuntimeEnabled = true },
		"invalid baseline":       func(i *domain.WorkspaceInspection) { i.Resolution.Catalog.Baseline.SchemaVersion = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			source, manager := workspaceManagerFixture()
			change(&source.inspection)
			plan := manager.Plan(t.Context(), "/deployment", minimalWorkspace)
			if !plan.HasErrors() || plan.ReviewToken != "" || source.writes != 0 {
				t.Fatalf("invalid inspection accepted: %+v", plan)
			}
		})
	}
}
