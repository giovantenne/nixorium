package presentation

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func demoHomeFilesCapture() domain.HomeFilesCaptureReport {
	lesson, script := "imports/"+strings.Repeat("1", 64), "imports/"+strings.Repeat("2", 64)
	return domain.HomeFilesCaptureReport{SchemaVersion: 1, State: "captured", CaptureID: "synthetic-capture", Destination: "Documents/Lessons",
		Files:       []domain.HomeFile{{Destination: "Documents/Lessons/lesson.pdf", Source: &lesson}, {Destination: "Documents/Lessons/run.sh", Source: &script}},
		Directories: []string{"Documents/Lessons", "Documents/Lessons/Empty"},
		Entries:     []domain.HomeFilesEntry{{Destination: "Documents/Lessons/lesson.pdf", Size: 8192}, {Destination: "Documents/Lessons/run.sh", Size: 96}, {Destination: "Documents/Lessons", Directory: true}, {Destination: "Documents/Lessons/Empty", Directory: true}},
		Message:     "Copied materials captured; the local source is no longer read for this proposal."}
}

func demoHomeFilesPlan() domain.HomeFilesPlanReport {
	c := demoHomeFilesCapture()
	c.Files[1].Executable = true
	return domain.HomeFilesPlanReport{SchemaVersion: 1, Operation: "home-files-plan", State: "ready", Repository: "/demo/lab",
		Candidate: &domain.HomeFilesDraft{Profile: domain.HomeFilesProfile{SchemaVersion: 1, Files: c.Files, Directories: c.Directories}, CaptureIDs: []string{c.CaptureID}, BaseFingerprint: "base"},
		Snapshot:  &domain.HomeFilesSnapshot{BaseFingerprint: "base", Revision: strings.Repeat("a", 40)},
		Changes: []domain.HomeFilesChange{
			{Destination: "Documents/Lessons/lesson.pdf", Kind: "added", AfterSize: 8192},
			{Destination: "Documents/Lessons/run.sh", Kind: "added", AfterSize: 96, AfterExecutable: true},
			{Destination: "Documents/Lessons/Empty", Kind: "added", Directory: true, AfterDirectory: true},
		},
		Entries: c.Entries, StudentUser: "student", Targets: []domain.WorkspaceTarget{{Name: "pc99", Role: "controller"}, {Name: "pc01", Role: "client"}, {Name: "pc02", Role: "client"}}, ReviewToken: "synthetic-home-files-review",
		Message: "Only the manifest and captured copies will be saved locally; system application and home restoration are separate."}
}

// RenderHomeFilesDemo uses synthetic immutable captures with the ordinary
// feature model. No source file, deployment or student home is accessed.
func RenderHomeFilesDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	actions.LoadHomeFiles = func(context.Context) domain.HomeFilesPlanReport {
		p := demoHomeFilesPlan()
		p.Candidate = &domain.HomeFilesDraft{Profile: domain.HomeFilesProfile{SchemaVersion: 1}, BaseFingerprint: "base"}
		p.Entries = nil
		p.Changes = nil
		return p
	}
	actions.CaptureHomeFiles = func(context.Context, string, string) domain.HomeFilesCaptureReport { return demoHomeFilesCapture() }
	actions.CompleteHomeFilesPath = func(context.Context, string) ([]string, error) { return []string{"/demo/materials/Lessons/"}, nil }
	actions.PlanHomeFiles = func(_ context.Context, draft domain.HomeFilesDraft) domain.HomeFilesPlanReport {
		p := demoHomeFilesPlan()
		p.Candidate = &draft
		return p
	}
	actions.SaveHomeFiles = func(domain.HomeFilesPlanReport) domain.HomeFilesApplyReport {
		return domain.HomeFilesApplyReport{Operation: "home-files-save", State: "saved", Recorded: true, Revision: revision, Message: "Initial-file manifest and reviewed copies saved and recorded locally."}
	}
	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Overview", 1000)
	load := r.model.openHomeFiles
	next, cmd := load()
	r.model = next.(dashboardModel)
	r.capture("Cancellable initial-file load", 1000)
	r.command(cmd)
	r.capture("Empty initial-file declarations", 1600)
	r.key(demoText("a"))
	r.key(demoText("/demo/materials/Les"))
	r.command(r.key(demoCode(tea.KeyTab)))
	r.capture("Local source selection with completion", 2000)
	r.key(demoCode(tea.KeyEnter))
	r.key(demoText("Documents/Lessons"))
	r.capture("Relative student-home destination", 1800)
	capture := r.key(demoCode(tea.KeyEnter))
	r.capture("Cancellable material capture", 900)
	r.command(capture)
	r.capture("Captured files and empty directories", 2400)
	r.key(demoCode(tea.KeyDown))
	r.key(demoCode(tea.KeySpace))
	r.capture("Explicit executable choice for one file", 2200)
	r.key(demoCode(tea.KeyEnter))
	r.command(r.key(demoText("v")))
	r.capture("Initial-file save review", 2600)
	save := r.key(demoCode(tea.KeyEnter))
	r.capture("Protected initial-file save", 900)
	r.command(save)
	r.capture("Saved copies await separate system application", 2600)
	// A second synthetic review demonstrates edits to an existing declaration.
	// It is never applied; the native change rows remain visible before content.
	r.model.homeFiles.stage = "review"
	r.model.homeFiles.plan = demoHomeFilesPlan()
	r.model.homeFiles.plan.Changes = []domain.HomeFilesChange{
		{Destination: "Documents/Lessons/previous.pdf", Kind: "removed", BeforeSize: 4096},
		{Destination: "Documents/Lessons/lesson.pdf", Kind: "replaced", BeforeSize: 4096, AfterSize: 8192},
		{Destination: "Documents/Lessons/run.sh", Kind: "permissions", BeforeSize: 96, AfterSize: 96, AfterExecutable: true},
		{Destination: "Documents/Lessons/Empty", Kind: "added", Directory: true, AfterDirectory: true},
	}
	r.capture("Review removals, replacements and mode changes", 2600)
	r.model.homeFiles.stage = "result"
	r.model.homeFiles.result = domain.HomeFilesApplyReport{Operation: "home-files-save", State: "partial", RecoveryRequired: true, Message: "Captured copies saved; the local Git record could not be confirmed. Inspect Git before applying systems."}
	r.capture("Partial initial-file save requires inspection", 2400)
	r.model.homeFiles.stage = "source"
	r.model.homeFiles.source = "/demo/materials"
	r.model.message = "The folder contains a symbolic link. Choose a clean materials folder without links, keys or credentials; no copies were added."
	r.capture("Rejected source selection has an actionable next step", 2400)
	return DemoScenario{ID: "student-initial-files", Title: "Import shared initial student-home files", Description: "Capture educational materials, review per-file executability and save locally. System application and next-boot restoration remain separate.", Frames: r.frames}
}
