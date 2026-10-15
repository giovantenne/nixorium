package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// GitUndoSource gives the reviewed undo of the last saved change its Git
// observations and its one write. Undo shares the safety rules of the
// reviewed commit: a clean tree, fixed Git commands, no hook, signing helper,
// remote or push.
type GitUndoSource interface {
	GitReview(context.Context, string) (domain.GitReviewSnapshot, error)
	GitRevision(context.Context, string) (string, error)
	GitUndoFacts(context.Context, string) (domain.GitUndoFacts, error)
	GitFileAt(ctx context.Context, repository, revision, path string) ([]byte, bool, error)
	GitUndoProposal(context.Context, string, domain.GitUndoFacts) (domain.GitUndoProposal, error)
	UndoGitCommit(ctx context.Context, repository string, facts domain.GitUndoFacts, tree, message string) (string, error)
}

// GitUndoManager reverts the last saved change of the deployment repository.
// It saves the undo as a new change and never touches a computer: applying
// the result to the controller and updating the computers stay separate
// reviewed operations.
type GitUndoManager struct {
	source GitUndoSource
}

func NewGitUndoManager(source GitUndoSource) *GitUndoManager {
	return &GitUndoManager{source: source}
}

const (
	gitUndoSettingsFile = "lab-settings.json"
	gitUndoSubjectRunes = 120
)

// Files whose earlier version must be valid, and present, before it comes
// back. The same decoders guard a reviewed commit of these paths.
var gitUndoManagedFiles = []string{gitUndoSettingsFile, "lab-software.json", domain.WorkspaceFileName, domain.HomeFilesFileName}

// Updates of Nixorium and of the package base own these files and their
// recovery.
var gitUndoUpdateFiles = map[string]bool{"flake.nix": true, "flake.lock": true}

// Public keys and the encrypted recovery file belong with this controller's
// private keys; undoing them would separate the pair.
var gitUndoKeyFiles = map[string]bool{"keys/cache-public-key": true, "keys/admin-ssh.pub": true, "nixorium-recovery.age": true}

// Private files are never saved in Git by Nixorium.
var gitUndoPrivateFiles = map[string]bool{"secret-key": true, "admin-ssh": true, "lab-credentials.json": true}

// Plan reviews undoing the last saved change. It changes nothing.
func (m *GitUndoManager) Plan(ctx context.Context, repository string) domain.GitUndoPlanReport {
	report, _ := m.plan(ctx, repository)
	return report
}

func (m *GitUndoManager) plan(ctx context.Context, repository string) (domain.GitUndoPlanReport, domain.GitUndoFacts) {
	report := domain.GitUndoPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "git-undo-plan",
		GeneratedAt:   time.Now().UTC(),
		State:         "blocked",
		Undone:        domain.GitUndoCommit{Files: []domain.GitUndoFile{}},
		Diff:          domain.GitDiff{Scope: "undo-changes"},
		Impacts:       []domain.SettingImpact{},
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return gitUndoPlanIssue(report, "repository", fmt.Sprintf("resolve path: %v", err)), domain.GitUndoFacts{}
	}
	report.Repository = root
	review := NewGitReviewManager(m.source).Review(ctx, root)
	report.Revision = review.Revision
	if review.HasErrors() {
		report.Issues = append(report.Issues, review.Issues...)
		if len(report.Issues) == 0 {
			report = gitUndoPlanIssue(report, "review", "Git change review is unavailable")
		}
		return report, domain.GitUndoFacts{}
	}
	if len(review.Changes) > 0 {
		report.Issues = append(report.Issues, domain.BlockedIssue("git", "GIT-DIRTY", fmt.Sprintf("deployment worktree has %d changed path(s); save or inspect them in Maintenance → Review Git changes (`nixorium git review`)", len(review.Changes))))
		return report, domain.GitUndoFacts{}
	}
	facts, err := m.source.GitUndoFacts(ctx, root)
	if err != nil {
		if code, typed := domain.BlockedCodeOf(err); typed {
			report.Issues = append(report.Issues, domain.BlockedIssue("git", code, err.Error()))
			return report, facts
		}
		return gitUndoPlanIssue(report, "revision", fmt.Sprintf("cannot read the last saved change: %v", err)), facts
	}
	report.Revision = facts.Revision
	report.Undone = domain.GitUndoCommit{Revision: facts.Revision, Subject: facts.Subject, Date: facts.Date, Files: append([]domain.GitUndoFile{}, facts.Files...)}
	if len(facts.Parents) == 1 {
		report.Undone.Parent = facts.Parents[0]
	}
	if issues := gitUndoRefusals(facts); len(issues) > 0 {
		report.Issues = append(report.Issues, issues...)
		return report, facts
	}
	if issues := m.managedFileIssues(ctx, root, facts); len(issues) > 0 {
		report.Issues = append(report.Issues, issues...)
		return report, facts
	}
	proposal, err := m.source.GitUndoProposal(ctx, root, facts)
	if err != nil {
		return gitUndoPlanIssue(report, "undo", fmt.Sprintf("Nixorium cannot put the files back safely: %v. Use Git by hand to review the change.", err)), facts
	}
	report.TreeID = proposal.TreeID
	report.Diff = proposal.Diff
	report.CommitMessage = gitUndoMessage(facts)
	report.ReviewToken = gitUndoReviewToken(facts, proposal.TreeID, report.CommitMessage)
	report.Confirmation = domain.GitUndoConfirmation
	report.Impacts = domain.GitUndoImpacts(m.undoReachesClients(ctx, root, facts))
	report.State = "ready"
	return report, facts
}

// Apply repeats the whole review, requires its token and saves the undo.
func (m *GitUndoManager) Apply(ctx context.Context, repository, expectedToken string) domain.GitUndoReport {
	plan, facts := m.plan(ctx, repository)
	report := domain.GitUndoReport{
		SchemaVersion:    domain.SchemaVersion,
		Operation:        "git-undo",
		State:            "blocked",
		Repository:       plan.Repository,
		PreviousRevision: plan.Revision,
		CommitMessage:    plan.CommitMessage,
		RetrySafe:        true,
		Impacts:          []domain.SettingImpact{},
		Issues:           append([]domain.ValidationIssue{}, plan.Issues...),
	}
	if plan.HasErrors() {
		report.Message = "Nothing was undone: the review found a problem."
		return report
	}
	if expectedToken == "" || expectedToken != plan.ReviewToken {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "review", Message: "review token does not match the current last saved change"})
		report.Message = "Nothing was undone; create a fresh review."
		return report
	}
	revision, err := m.source.UndoGitCommit(ctx, plan.Repository, facts, plan.TreeID, plan.CommitMessage)
	if err != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("The last saved change was not undone: %v", err)
		if current, revisionErr := m.source.GitRevision(ctx, plan.Repository); revisionErr == nil && current != plan.Revision {
			report.State = "partial"
			report.Revision = current
			report.Committed = true
			report.RetrySafe = false
			report.Message += "; the undo was saved but the files may not match it, so run `nixorium git review` instead of retrying"
		}
		return report
	}
	report.State = "completed"
	report.Revision = revision
	report.Committed = true
	report.RetrySafe = false
	report.Impacts = plan.Impacts
	report.Message = "The last saved change was undone and the undo was saved locally. No computer was changed and nothing was pushed."
	return report
}

// gitUndoRefusals lists what makes the last saved change something other than
// an ordinary saved change that can go back. Each message names the next step.
func gitUndoRefusals(facts domain.GitUndoFacts) []domain.ValidationIssue {
	var issues []domain.ValidationIssue
	refuse := func(field, message string) {
		issues = append(issues, domain.ValidationIssue{Field: field, Message: message})
	}
	if facts.InProgress != "" {
		refuse("git", fmt.Sprintf("A Git %s is unfinished in this configuration. Finish it or cancel it with Git first; nothing is undone while it is open.", facts.InProgress))
	}
	if facts.Branch == "" {
		refuse("branch", "The configuration is not on a branch (Git is on a detached HEAD), so there is no last saved change to undo. Go back to your branch with Git first.")
	}
	switch {
	case len(facts.Parents) == 0:
		refuse("revision", "The last saved change is the first saved version of this configuration. There is nothing earlier to go back to.")
		return issues
	case len(facts.Parents) > 1:
		refuse("revision", "The last saved change is a merge of two histories. Only an ordinary saved change can be undone here; review the merge with Git by hand.")
		return issues
	}
	if facts.Reverts != "" {
		refuse("revision", "The last saved change already undoes an earlier one. Undoing it again would only bring that change back; make the change again from its usual screen (Settings, Software, ...) instead.")
	}
	if facts.Truncated {
		refuse("files", "The last saved change touches more than 1000 files. Review it with Git by hand.")
	}
	if len(facts.Files) == 0 && !facts.Truncated {
		refuse("files", "The last saved change contains no file changes, so there is nothing to undo.")
	}
	reported := map[string]bool{}
	once := func(kind, field, message string) {
		if !reported[kind] {
			reported[kind] = true
			refuse(field, message)
		}
	}
	for _, file := range facts.Files {
		switch {
		case gitUndoUpdateFiles[file.Path]:
			once("update", file.Path, "The last saved change updated Nixorium or the package base (flake.nix or flake.lock). Those updates have their own recovery: follow UPDATES.md, section \"Recovery and long-term autonomy\".")
		case gitUndoKeyFiles[file.Path]:
			once("keys", file.Path, fmt.Sprintf("The last saved change recorded public keys or the encrypted recovery file (%s). They belong with the private keys of this controller, so they are not undone here; see TROUBLESHOOTING.md, section \"Keys are missing or inconsistent\".", file.Path))
		case gitUndoPrivateFiles[file.Path]:
			once("private", file.Path, fmt.Sprintf("The last saved change contains a private key or credentials file (%s), which must never be saved in Git. Review it with Git by hand and keep the file out of Git.", file.Path))
		case path.Base(file.Path) == ".gitattributes":
			once("attributes", file.Path, "The last saved change changes Git attribute rules, which change how files are written. Review it with Git by hand.")
		case file.Special:
			once("special", "files", fmt.Sprintf("The last saved change contains a symbolic link, a submodule or a file name with unusual characters (%q). Review it with Git by hand.", file.Path))
		}
	}
	return issues
}

// managedFileIssues checks the earlier version of each managed file that the
// last saved change touched: it must exist and be valid, and it must not move
// the laboratory network, which has its own guided change.
func (m *GitUndoManager) managedFileIssues(ctx context.Context, root string, facts domain.GitUndoFacts) []domain.ValidationIssue {
	changed := map[string]bool{}
	for _, file := range facts.Files {
		changed[file.Path] = true
	}
	var issues []domain.ValidationIssue
	add := func(field, message string) {
		issues = append(issues, domain.ValidationIssue{Field: field, Message: message})
	}
	for _, name := range gitUndoManagedFiles {
		if !changed[name] {
			continue
		}
		content, exists, err := m.source.GitFileAt(ctx, root, facts.Parents[0], name)
		switch {
		case err != nil:
			add(name, err.Error())
			continue
		case !exists:
			add(name, fmt.Sprintf("Undoing would remove %s, which this configuration needs. Review the change with Git by hand.", name))
			continue
		}
		var problems []string
		switch name {
		case gitUndoSettingsFile:
			_, found := domain.DecodeLabSettings(content)
			for _, issue := range found {
				problems = append(problems, issue.Field+": "+issue.Message)
			}
		case "lab-software.json":
			if _, err := domain.DecodeLabSoftware(content); err != nil {
				problems = append(problems, err.Error())
			}
		case domain.WorkspaceFileName:
			_, found := domain.DecodeWorkspaceProfile(content)
			for _, issue := range found {
				problems = append(problems, issue.Field+": "+issue.Message)
			}
		case domain.HomeFilesFileName:
			_, found := domain.DecodeHomeFilesProfile(content)
			for _, issue := range found {
				problems = append(problems, issue.Field+": "+issue.Message)
			}
		}
		if len(problems) > 0 {
			add(name, fmt.Sprintf("%s as it was before that change is not valid (%s). Review the change with Git by hand.", name, strings.Join(problems, "; ")))
		}
	}
	if changed[gitUndoSettingsFile] {
		issues = append(issues, m.networkIssues(ctx, root, facts)...)
	}
	return issues
}

// networkIssues keeps an undo from moving the laboratory network behind the
// back of the guided change. When the current settings cannot be read the
// undo may repair them, so only a readable pair is compared.
func (m *GitUndoManager) networkIssues(ctx context.Context, root string, facts domain.GitUndoFacts) []domain.ValidationIssue {
	current, after := m.undoSettings(ctx, root, facts)
	if current == nil || after == nil {
		return nil
	}
	changes := domain.DiffLabSettings(*current, *after)
	for _, change := range changes {
		if change.Field == "lab.networkTransition" {
			return []domain.ValidationIssue{{Field: gitUndoSettingsFile, Message: "The last saved change belongs to a change of the laboratory network. Continue it or undo it from Maintenance → Change network addressing (`nixorium network migrate status`)."}}
		}
	}
	return domain.SettingsTransitionIssues(*current, *after)
}

// undoSettings reads the settings saved by the last change and before it. A
// missing or invalid file gives nil.
func (m *GitUndoManager) undoSettings(ctx context.Context, root string, facts domain.GitUndoFacts) (current, after *domain.LabSettingsFile) {
	read := func(revision string) *domain.LabSettingsFile {
		content, exists, err := m.source.GitFileAt(ctx, root, revision, gitUndoSettingsFile)
		if err != nil || !exists {
			return nil
		}
		file, issues := domain.DecodeLabSettings(content)
		if len(issues) > 0 {
			return nil
		}
		return &file
	}
	if len(facts.Parents) != 1 {
		return nil, nil
	}
	return read(facts.Revision), read(facts.Parents[0])
}

// undoReachesClients is false only for a controller-only deployment before
// and after the undo: there are no computers to update.
func (m *GitUndoManager) undoReachesClients(ctx context.Context, root string, facts domain.GitUndoFacts) bool {
	current, after := m.undoSettings(ctx, root, facts)
	return current == nil || after == nil || current.Lab.DeploymentMode != "controller" || after.Lab.DeploymentMode != "controller"
}

// gitUndoMessage is "Undo: <subject>" plus the line Git itself writes for a
// revert, which also lets a later review recognize the commit.
func gitUndoMessage(facts domain.GitUndoFacts) string {
	subject := facts.Subject
	if subject == "" {
		subject = "saved change " + facts.Revision[:7]
	}
	if runes := []rune(subject); len(runes) > gitUndoSubjectRunes {
		subject = string(runes[:gitUndoSubjectRunes]) + "…"
	}
	return fmt.Sprintf("Undo: %s\n\nThis reverts commit %s.\n", subject, facts.Revision)
}

// gitUndoReviewToken binds the review to the reverted change, its files and
// the saved version it returns to.
func gitUndoReviewToken(facts domain.GitUndoFacts, tree, message string) string {
	digest := sha256.New()
	values := []string{"git-undo", facts.Revision, facts.Parents[0], facts.Tree, tree, message}
	for _, file := range facts.Files {
		values = append(values, file.Path, file.Change)
	}
	for _, value := range values {
		_, _ = digest.Write([]byte(value))
		_, _ = digest.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", digest.Sum(nil))
}

func gitUndoPlanIssue(report domain.GitUndoPlanReport, field, message string) domain.GitUndoPlanReport {
	report.State = "blocked"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}
