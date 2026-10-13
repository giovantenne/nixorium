package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (m HomeFilesManager) DiscardCapture(ctx context.Context, repository, id string) error {
	if source, ok := m.source.(interface {
		DiscardHomeFilesCapture(context.Context, string, string) error
	}); ok {
		return source.DiscardHomeFilesCapture(ctx, repository, id)
	}
	return errors.New("capture cleanup unavailable; retained private captures expire after 24 hours")
}

type HomeFilesSource interface {
	ReadHomeFiles(string) ([]byte, error)
	CaptureHomeFiles(context.Context, string, string, string) (domain.HomeFilesCaptureReport, error)
	InspectHomeFiles(context.Context, string, domain.HomeFilesDraft) (domain.HomeFilesInspection, error)
	WriteHomeFilesIfUnchanged(context.Context, string, domain.HomeFilesSnapshot, domain.HomeFilesDraft) error
}
type HomeFilesManager struct{ source HomeFilesSource }

func NewHomeFilesManager(source HomeFilesSource) HomeFilesManager { return HomeFilesManager{source} }

func (m HomeFilesManager) Load(ctx context.Context, repository string) domain.HomeFilesPlanReport {
	data, err := m.source.ReadHomeFiles(repository)
	existed := !errors.Is(err, fs.ErrNotExist)
	if errors.Is(err, fs.ErrNotExist) {
		data = []byte(`{"schemaVersion":1}`)
	} else if err != nil {
		return homeFilesPlanIssue(repository, "file", err)
	}
	p, issues := domain.DecodeHomeFilesProfile(data)
	if len(issues) != 0 {
		r := homeFilesPlanIssue(repository, "file", errors.New("initial home declaration is invalid; inspect Git review"))
		r.Issues = issues
		return r
	}
	r := m.Plan(ctx, repository, domain.HomeFilesDraft{Profile: p})
	if r.Snapshot != nil && (r.Snapshot.BaseExists != existed || (existed && r.State != "unchanged")) {
		return homeFilesPlanIssue(repository, "source", domain.ErrHomeFilesConflict)
	}
	if r.Candidate == nil {
		r.Candidate = &domain.HomeFilesDraft{Profile: p}
	}
	if r.Snapshot != nil {
		r.Candidate.BaseFingerprint = r.Snapshot.BaseFingerprint
	}
	return r
}

func (m HomeFilesManager) Capture(ctx context.Context, repository, sourcePath, destination string) domain.HomeFilesCaptureReport {
	r, err := m.source.CaptureHomeFiles(ctx, repository, sourcePath, destination)
	if err != nil {
		return domain.HomeFilesCaptureReport{SchemaVersion: 1, State: "invalid", Destination: destination, Issues: []domain.ValidationIssue{{Field: "source", Message: err.Error()}}, Message: "No declaration was changed. Choose a regular non-secret file or folder and retry in Initial files; CLI: home-files import plan."}
	}
	return r
}

func (m HomeFilesManager) Plan(ctx context.Context, repository string, draft domain.HomeFilesDraft) domain.HomeFilesPlanReport {
	data, err := domain.MarshalHomeFilesProfile(draft.Profile)
	if err != nil {
		return homeFilesPlanIssue(repository, "candidate", err)
	}
	p, _ := domain.DecodeHomeFilesProfile(data)
	draft.Profile = p
	if len(draft.CaptureIDs) > 16 {
		return homeFilesPlanIssue(repository, "captures", errors.New("at most 16 captures may be reviewed together"))
	}
	draft.CaptureIDs = slices.Clone(draft.CaptureIDs)
	slices.Sort(draft.CaptureIDs)
	draft.CaptureIDs = slices.Compact(draft.CaptureIDs)
	i, err := m.source.InspectHomeFiles(ctx, repository, draft)
	if err != nil {
		return homeFilesPlanIssue(repository, "inspection", err)
	}
	if err := domain.ValidateHomeFilesInspection(i, draft.Profile); err != nil {
		return homeFilesPlanIssue(repository, "inspection", err)
	}
	if draft.BaseFingerprint != "" && draft.BaseFingerprint != i.Snapshot.BaseFingerprint {
		return homeFilesPlanIssue(repository, "source", domain.ErrHomeFilesConflict)
	}
	r := domain.HomeFilesPlanReport{SchemaVersion: 1, Operation: "home-files-plan", State: "ready", Repository: repository, Candidate: &draft, Snapshot: &i.Snapshot, Entries: i.Entries, Changes: homeFilesChanges(i.BaseEntries, i.Entries), StudentUser: i.StudentUser, Targets: i.Targets, Confirmation: "SAVE", Issues: []domain.ValidationIssue{}, Message: "Save initial files for the student account on the controller and all clients. Files are private; explicit executables are 0700. Materials enter Git, the Nix store and cache. Systems and homes change only after separate application and normal boot."}
	baseJSON, _ := domain.MarshalHomeFilesProfile(i.Base)
	if len(r.Changes) == 0 && !bytes.Equal(baseJSON, data) {
		r.Changes = append(r.Changes, domain.HomeFilesChange{Kind: "declaration", Destination: domain.HomeFilesFileName})
	}
	if len(r.Changes) == 0 && bytes.Equal(baseJSON, data) && i.Snapshot.BaseExists {
		r.State = "unchanged"
		r.Confirmation = ""
		r.Message = "The initial-file declaration is unchanged; deployed and running homes were not observed."
	}
	r.ReviewToken = homeFilesToken(r)
	return r
}

func homeFilesToken(r domain.HomeFilesPlanReport) string {
	r.ReviewToken = ""
	data, _ := json.Marshal(r)
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}
func homeFilesChanges(before, after []domain.HomeFilesEntry) []domain.HomeFilesChange {
	old := map[string]domain.HomeFilesEntry{}
	next := map[string]domain.HomeFilesEntry{}
	for _, e := range before {
		old[e.Destination] = e
	}
	for _, e := range after {
		next[e.Destination] = e
	}
	paths := []string{}
	for p := range old {
		paths = append(paths, p)
	}
	for p := range next {
		if _, ok := old[p]; !ok {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	changes := []domain.HomeFilesChange{}
	for _, p := range paths {
		a, had := old[p]
		b, has := next[p]
		kind := ""
		if !had {
			kind = "added"
		} else if !has {
			kind = "removed"
		} else if a.Directory != b.Directory || a.SHA256 != b.SHA256 || a.Size != b.Size {
			kind = "replaced"
		} else if a.Executable != b.Executable {
			kind = "permissions"
		}
		if kind != "" {
			changes = append(changes, domain.HomeFilesChange{Destination: p, Kind: kind, Directory: b.Directory, BeforeDirectory: a.Directory, AfterDirectory: b.Directory, BeforeSize: a.Size, AfterSize: b.Size, BeforeExecutable: a.Executable, AfterExecutable: b.Executable})
		}
	}
	return changes
}

func (m HomeFilesManager) Apply(ctx context.Context, repository string, draft domain.HomeFilesDraft, expected string) domain.HomeFilesApplyReport {
	r := domain.HomeFilesApplyReport{SchemaVersion: 1, Operation: "home-files-apply", State: "invalid", Repository: repository, Targets: []domain.WorkspaceTarget{}, Issues: []domain.ValidationIssue{}}
	fresh := m.Plan(ctx, repository, draft)
	if fresh.HasErrors() {
		r.State = fresh.State
		r.Issues = fresh.Issues
		r.Message = fresh.Message
		return r
	}
	if expected == "" || expected != fresh.ReviewToken {
		r.State = "conflict"
		r.Issues = append(r.Issues, domain.ValidationIssue{Field: "review", Message: "The declaration or captured content changed; review again in Initial files or home-files plan."})
		return r
	}
	r.StudentUser = fresh.StudentUser
	r.Targets = fresh.Targets
	if fresh.State == "unchanged" {
		r.State = "unchanged"
		r.Message = fresh.Message
		return r
	}
	if err := ctx.Err(); err != nil {
		r.State = "failed"
		r.Issues = append(r.Issues, domain.ValidationIssue{Field: "save", Message: err.Error()})
		return r
	}
	if err := m.source.WriteHomeFilesIfUnchanged(ctx, repository, *fresh.Snapshot, *fresh.Candidate); err != nil {
		r.State = "failed"
		if errors.Is(err, domain.ErrHomeFilesConflict) {
			r.State = "conflict"
		}
		if errors.Is(err, domain.ErrHomeFilesDurability) {
			r.State = "partial"
			r.RecoveryRequired = true
		}
		r.Issues = append(r.Issues, domain.ValidationIssue{Field: "save", Message: err.Error()})
		r.Message = "Inspect Initial files and Maintenance → Git review before creating a fresh review; CLI: nixorium git review. No system application or home reset was requested."
		return r
	}
	r.State = "saved"
	r.Message = "Saved home-files.json and captured payloads locally. Apply systems separately; the student baseline changes at the next normal boot."
	return r
}

func homeFilesPlanIssue(repository, field string, err error) domain.HomeFilesPlanReport {
	state := "invalid"
	if errors.Is(err, domain.ErrHomeFilesConflict) {
		state = "conflict"
	}
	return domain.HomeFilesPlanReport{SchemaVersion: 1, Operation: "home-files-plan", State: state, Repository: repository, Issues: []domain.ValidationIssue{{Field: field, Message: err.Error()}}, Message: "No declaration was changed. Inspect Initial files, Student workspace preferences and Git review, then create a fresh plan; CLI: nixorium home-files plan."}
}

func MergeHomeFilesCapture(base domain.HomeFilesProfile, capture domain.HomeFilesCaptureReport, replace bool) (domain.HomeFilesProfile, error) {
	if capture.HasErrors() {
		return base, errors.New("capture did not complete")
	}
	under := func(path string) bool {
		return path == capture.Destination || strings.HasPrefix(path, capture.Destination+"/")
	}
	if replace {
		base.Files = slices.DeleteFunc(slices.Clone(base.Files), func(f domain.HomeFile) bool { return under(f.Destination) })
		base.Directories = slices.DeleteFunc(slices.Clone(base.Directories), under)
	}
	base.Files = append(slices.Clone(base.Files), capture.Files...)
	base.Directories = append(slices.Clone(base.Directories), capture.Directories...)
	data, err := domain.MarshalHomeFilesProfile(base)
	if err != nil {
		return domain.HomeFilesProfile{}, err
	}
	result, _ := domain.DecodeHomeFilesProfile(data)
	return result, nil
}
