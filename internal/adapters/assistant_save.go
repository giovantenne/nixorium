package adapters

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

const assistantSaveChanged = "The proposal or the deployment changed. Open the review again."
const assistantSaveDirty = "Commit or discard the deployment changes, then start a new chat."
const assistantSaveInvalid = "Saving is not available: the proposal does not evaluate. Return to the chat and ask the assistant to fix it."
const assistantSaveTooLarge = "This proposal is too large to review here. Ask for a smaller change."

var assistantCommit = configurationCommit{
	Kind: "assistant-save", CommitMessage: "Save reviewed Assistant proposal", BackupPrefix: "refs/nixorium/assistant-backups/",
	SavedMessage:       "Saved in a local commit. Nothing was built or activated.",
	InterruptedMessage: "Saving was interrupted. The previous configuration is kept in a backup. Open Overview to recover before any other operation.",
	RefLogMessage:      "Nixorium Assistant save",
}

func PlanAssistantSave(ctx context.Context, store AssistantWorkspaceStore, conversationID string) domain.AssistantSavePlan {
	return planAssistantSave(ctx, store, conversationID, runBoundedNix)
}
func planAssistantSave(ctx context.Context, store AssistantWorkspaceStore, conversationID string, run assistantNixRunner) domain.AssistantSavePlan {
	plan := domain.AssistantSavePlan{State: "blocked", ConversationID: conversationID, Added: []string{}, Changed: []string{}, Removed: []string{}, ChangedHosts: []string{}, SettingsImpacts: []domain.SettingImpact{}}
	fail := func(message string) domain.AssistantSavePlan {
		plan.State = "blocked"
		plan.Message = message
		return plan
	}
	w, err := store.Open(conversationID)
	if err != nil {
		return fail(assistantSaveChanged)
	}
	defer w.Close()
	plan.Repository, plan.Base = w.Conversation.Repository, w.Conversation.Base
	status, err := resetGit(ctx, plan.Repository, nil, 1024*1024, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return fail(err.Error())
	}
	if status != "" {
		return fail(assistantSaveDirty)
	}
	root, err := openWorkspaceRoot(plan.Repository, unix.LOCK_SH)
	if err != nil {
		return fail(err.Error())
	}
	defer root.Close()
	proposal, err := inspectResetRepository(ctx, root)
	if err != nil {
		return fail(err.Error())
	}
	if proposal.Revision != plan.Base {
		return fail(assistantSaveChanged)
	}
	proposal.Candidate, err = assistantSaveCandidate(w, proposal.Original)
	if err != nil {
		return fail(err.Error())
	}
	changes := assistantSaveChanges(proposal.Original, proposal.Candidate)
	for _, change := range changes {
		if domain.AssistantSettingsPath(change.Path) && change.Action == "replace" && proposal.Original[change.Path].Mode == proposal.Candidate[change.Path].Mode {
			// The settings file is the one protected path a proposal may
			// change, and only the way a native settings save may: the same
			// transition rules, and the same impacts shown in the review.
			review, err := domain.ReviewAssistantSettings(proposal.Original[change.Path].Data, proposal.Candidate[change.Path].Data)
			if err != nil {
				var refusal *domain.AssistantSettingsError
				if errors.As(err, &refusal) {
					return fail(refusal.ReviewMessage())
				}
				return fail(err.Error())
			}
			plan.SettingsImpacts = review.Impacts
			if plan.SettingsImpacts == nil {
				plan.SettingsImpacts = []domain.SettingImpact{}
			}
			plan.Changed = append(plan.Changed, change.Path)
			continue
		}
		if !domain.ValidAssistantPath(change.Path) || domain.AssistantHiddenPath(change.Path) || domain.AssistantReadOnlyPath(change.Path) {
			return fail("The proposal changes a protected path. Return to the chat and discard it.")
		}
		switch change.Action {
		case "add":
			plan.Added = append(plan.Added, change.Path)
		case "remove":
			plan.Removed = append(plan.Removed, change.Path)
		default:
			plan.Changed = append(plan.Changed, change.Path)
		}
	}
	if len(changes) == 0 {
		return fail("The assistant has not changed any file.")
	}
	plan.CoreOverrideFiles = domain.AssistantCoreOverrideFiles(proposal.Candidate, append(append([]string{}, plan.Added...), plan.Changed...))
	// Reuse the evaluation Assistant already ran when every input still matches
	// its receipt; anything else evaluates again exactly as before.
	key, keyErr := assistantEvaluationInputs(ctx, w)
	if receipt, reusable := assistantReusableEvaluation(w, key); keyErr == nil && reusable {
		plan.Evaluation = receipt.Evaluation
		plan.EvaluationReused, plan.EvaluationVerifiedAt = true, receipt.VerifiedAt
	} else {
		plan.Evaluation, _ = evaluateAssistantCandidate(ctx, w, run)
	}
	plan.ChangedHosts = plan.Evaluation.ChangedHosts
	// Bind the evaluation and review to the same overlay bytes and modes.
	fresh, err := store.Open(conversationID)
	if err != nil {
		return fail(assistantSaveChanged)
	}
	after, err := assistantSaveCandidate(fresh, proposal.Original)
	var afterKey assistantEvaluationKey
	afterKeyErr := err
	if err == nil {
		afterKey, afterKeyErr = assistantEvaluationInputs(ctx, fresh)
	}
	fresh.Close()
	if err != nil || resetFileDigest(after) != resetFileDigest(proposal.Candidate) || (keyErr == nil) != (afterKeyErr == nil) || afterKey != key {
		return fail(assistantSaveChanged)
	}
	plan.Proposal = proposal
	plan.Diff, err = assistantSaveDiff(ctx, proposal, changes)
	if err != nil {
		return fail(err.Error())
	}
	changedFiles := map[string]domain.TemplateFile{}
	for _, change := range changes {
		if file, ok := proposal.Candidate[change.Path]; ok {
			changedFiles[change.Path] = file
		}
	}
	plan.ReviewToken = resetFileDigest(struct {
		Base                    string
		Added, Changed, Removed []string
		Candidate               map[string]domain.TemplateFile
		Evaluation              domain.AssistantEvaluation
		Diff                    string
	}{plan.Base, plan.Added, plan.Changed, plan.Removed, changedFiles, plan.Evaluation, plan.Diff})
	plan.State = "ready"
	plan.ExpiresAt = time.Now().Add(10 * time.Minute)
	return plan
}
func assistantSaveCandidate(w *AssistantWorkspace, original map[string]domain.TemplateFile) (map[string]domain.TemplateFile, error) {
	candidate := map[string]domain.TemplateFile{}
	for path, file := range original {
		candidate[path] = file
	}
	for path := range w.deleted {
		delete(candidate, path)
	}
	for path, executable := range w.modes {
		if w.deleted[path] {
			continue
		}
		data, err := w.raw(path)
		if err != nil {
			return nil, err
		}
		mode := "100644"
		if executable {
			mode = "100755"
		}
		candidate[path] = domain.TemplateFile{Mode: mode, Data: data}
	}
	return candidate, domain.ValidateTemplateFiles(candidate)
}
func SaveAssistant(ctx context.Context, store AssistantWorkspaceStore, plan domain.AssistantSavePlan) domain.TemplateResetResult {

	fail := func(message string) domain.TemplateResetResult {
		return domain.TemplateResetResult{State: "blocked", Message: message}
	}
	current := PlanAssistantSave(ctx, store, plan.ConversationID)
	if !plan.Evaluation.OK || (current.State == "ready" && !current.Evaluation.OK) {
		return fail(assistantSaveInvalid)
	}
	if plan.State != "ready" || current.State != "ready" || plan.ReviewToken == "" || current.ReviewToken != plan.ReviewToken || current.Repository != plan.Repository || !time.Now().Before(plan.ExpiresAt) {
		return fail(assistantSaveChanged)
	}
	gate, err := acquireManagedOperationGate()
	if err != nil {
		return fail(err.Error())
	}
	defer gate.Close()
	gate.describe("Save Assistant proposal")
	if err = configurationCheckoutIdle(ctx); err != nil {
		return fail(err.Error())
	}
	root, err := openWorkspaceRoot(current.Repository, unix.LOCK_EX)
	if err != nil {
		return fail(err.Error())
	}
	defer root.Close()
	result := commitConfigurationTree(ctx, root, current.Proposal, current.ReviewToken, assistantCommit, nil)
	if result.State == "saved" && !result.RecoveryRequired {
		if err := store.markConversationSaved(plan.ConversationID); err != nil {
			result.Message += " Could not record the saved conversation."
		}
	}
	return result
}
func assistantSaveDiff(ctx context.Context, proposal domain.TemplateResetProposal, changes []domain.TemplateResetChange) (string, error) {
	directory, err := os.MkdirTemp("", "nixorium-assistant-diff-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(directory)
	for _, side := range []string{"before", "after"} {
		if err = os.Mkdir(filepath.Join(directory, side), 0700); err != nil {
			return "", err
		}
		files := proposal.Original
		if side == "after" {
			files = proposal.Candidate
		}
		for _, change := range changes {
			file, exists := files[change.Path]
			if !exists {
				continue
			}
			path := filepath.Join(directory, side, change.Path)
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return "", err
			}
			if file.Mode == "120000" {
				err = os.Symlink(string(file.Data), path)
			} else {
				mode := os.FileMode(0644)
				if file.Mode == "100755" {
					mode = 0755
				}
				err = os.WriteFile(path, file.Data, mode)
			}
			if err != nil {
				return "", err
			}
		}
	}
	command := exec.CommandContext(ctx, "git", "-c", "core.attributesFile=/dev/null", "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--binary", "--", filepath.Join(directory, "before"), filepath.Join(directory, "after"))
	configureCommandCancellation(command)
	command.Env = append(workspaceEnvironment(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	output := &boundedCommandBuffer{limit: 2 * 1024 * 1024}
	command.Stdout = output
	command.Stderr = &boundedCommandBuffer{limit: 16 * 1024}
	err = command.Run()
	var exit *exec.ExitError
	if output.truncated {
		return "", errors.New(assistantSaveTooLarge)
	}
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return "", errors.New("Could not compare the proposal. Open the review again.")
	}
	diff := output.buffer.String()
	for _, side := range []string{"before", "after"} {
		diff = strings.ReplaceAll(diff, strings.TrimPrefix(filepath.Join(directory, side), "/")+"/", "")
	}
	return diff, nil
}

func assistantSaveChanges(original, candidate map[string]domain.TemplateFile) []domain.TemplateResetChange {
	changes := []domain.TemplateResetChange{}
	for path, old := range original {
		next, exists := candidate[path]
		if !exists {
			changes = append(changes, domain.TemplateResetChange{Path: path, Action: "remove"})
		} else if old.Mode != next.Mode || !bytes.Equal(old.Data, next.Data) {
			changes = append(changes, domain.TemplateResetChange{Path: path, Action: "replace"})
		}
	}
	for path := range candidate {
		if _, exists := original[path]; !exists {
			changes = append(changes, domain.TemplateResetChange{Path: path, Action: "add"})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

// ChangedFiles is a local comparison only; opening the chat never evaluates Nix.
func (store AssistantWorkspaceStore) ChangedFiles(id string) (int, error) {
	w, err := store.Open(id)
	if err != nil {
		return 0, err
	}
	defer w.Close()
	count := 0
	for path := range w.deleted {
		if _, exists := w.base[path]; exists {
			count++
		}
	}
	for path, executable := range w.modes {
		if w.deleted[path] {
			continue
		}
		base, exists := w.base[path]
		if !exists {
			count++
			continue
		}
		data, err := w.raw(path)
		if err != nil {
			return 0, err
		}
		original, err := resetGit(context.Background(), w.Conversation.Repository, nil, 1024*1024, "show", w.Conversation.Base+":"+path)
		if err != nil {
			return 0, err
		}
		mode := "100644"
		if executable {
			mode = "100755"
		}
		if base.mode != mode || string(data) != original {
			count++
		}
	}
	return count, nil
}
