package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

type assistantHostClass struct {
	Representative string   `json:"representative"`
	Members        []string `json:"members"`
}
type assistantNixRunner func(context.Context, int, ...string) (string, error)

type assistantBaseEvaluationCache struct {
	Output string            `json:"output"`
	Paths  map[string]string `json:"paths"`
}
type assistantCandidateEvaluationCache struct {
	Digest string            `json:"digest"`
	Output string            `json:"output"`
	Paths  map[string]string `json:"paths"`
}

func assistantOverlayDigest(w *AssistantWorkspace) (string, error) {
	paths := make([]string, 0, len(w.modes)+len(w.deleted))
	for path := range w.modes {
		if !w.deleted[path] {
			paths = append(paths, path)
		}
	}
	for path := range w.deleted {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	digest := sha256.New()
	for _, path := range paths {
		if w.deleted[path] {
			fmt.Fprintf(digest, "%s\x00deleted\n", path)
			continue
		}
		data, err := w.raw(path)
		if err != nil {
			return "", err
		}
		mode := "0644"
		if w.modes[path] {
			mode = "0755"
		}
		fmt.Fprintf(digest, "%s\x00%s\x00%x\n", path, mode, sha256.Sum256(data))
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

const (
	assistantEvaluationReceiptName  = "evaluation-receipt-v25.json"
	assistantEvaluationReceiptLimit = 256 * 1024
)

// assistantEvaluationKey lists every input that determines an evaluation
// result. The evaluation reads the repository only through Git objects of the
// conversation base (the base flake ref and the candidate clone checked out at
// base), so all files outside the overlay are fixed by Base; flake.lock, which
// pins every input by hash and can never be in the overlay, is also recorded
// on its own.
type assistantEvaluationKey struct {
	OverlayDigest   string `json:"overlayDigest"`
	Base            string `json:"base"`
	FlakeLockDigest string `json:"flakeLockDigest"`
	NixoriumVersion string `json:"nixoriumVersion"`
}

// assistantEvaluationReceipt is the private record of a successful evaluation,
// kept in the conversation's workspace root next to candidate-drv-v25.json.
type assistantEvaluationReceipt struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Key           assistantEvaluationKey     `json:"key"`
	VerifiedAt    time.Time                  `json:"verifiedAt"`
	Evaluation    domain.AssistantEvaluation `json:"evaluation"`
}

// assistantEvaluationInputs is shared by the evaluation, which records the
// receipt, and by the save review, which binds to and looks up the same keys.
func assistantEvaluationInputs(ctx context.Context, w *AssistantWorkspace) (assistantEvaluationKey, error) {
	key := assistantEvaluationKey{Base: w.Conversation.Base, NixoriumVersion: w.version}
	if key.NixoriumVersion == "" || !assistantBaseCommit.MatchString(key.Base) {
		return key, errors.New("evaluation inputs unavailable")
	}
	var err error
	if key.OverlayDigest, err = assistantOverlayDigest(w); err != nil {
		return key, err
	}
	entry, err := resetGit(ctx, w.Conversation.Repository, nil, 4096, "ls-tree", "-z", key.Base, "--", "flake.lock")
	if err != nil {
		return key, err
	}
	key.FlakeLockDigest = "absent"
	if entry != "" {
		info, path, ok := strings.Cut(strings.TrimSuffix(entry, "\x00"), "\t")
		fields := strings.Fields(info)
		if !ok || path != "flake.lock" || len(fields) != 3 || fields[1] != "blob" {
			return key, errors.New("invalid flake.lock entry")
		}
		data, err := resetGit(ctx, w.Conversation.Repository, nil, 4*1024*1024, "cat-file", "blob", fields[2])
		if err != nil {
			return key, err
		}
		key.FlakeLockDigest = fmt.Sprintf("%s:%x", fields[0], sha256.Sum256([]byte(data)))
	}
	return key, nil
}

// invalidateEvaluationReceipt runs before any overlay change, so a receipt
// never outlives the files it describes.
func (w *AssistantWorkspace) invalidateEvaluationReceipt() error {
	if err := unix.Unlinkat(int(w.root.Fd()), assistantEvaluationReceiptName, 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

// assistantReusableEvaluation returns the recorded result only when the
// receipt is a safe, bounded, well-formed record of a successful evaluation of
// exactly these inputs. Any other case means a fresh evaluation.
func assistantReusableEvaluation(w *AssistantWorkspace, key assistantEvaluationKey) (assistantEvaluationReceipt, bool) {
	var receipt assistantEvaluationReceipt
	data, err := assistantReadStateLimit(w.root, assistantEvaluationReceiptName, assistantEvaluationReceiptLimit)
	if err != nil || assistantDecodeStrict(data, &receipt) != nil {
		return receipt, false
	}
	if receipt.SchemaVersion != 1 || receipt.Key != key || receipt.VerifiedAt.IsZero() ||
		!receipt.Evaluation.OK || receipt.Evaluation.Error != "" || receipt.Evaluation.ChangedHosts == nil ||
		!sort.StringsAreSorted(receipt.Evaluation.ChangedHosts) {
		return receipt, false
	}
	for _, host := range receipt.Evaluation.ChangedHosts {
		if !assistantHostName.MatchString(host) {
			return receipt, false
		}
	}
	return receipt, true
}

var assistantHostName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var assistantBaseCommit = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

const assistantNeedsUpdate = "Update Nixorium in this deployment before using Assistant."

func EvaluateAssistantCandidate(ctx context.Context, workspace *AssistantWorkspace) (domain.AssistantEvaluation, error) {
	return evaluateAssistantCandidate(ctx, workspace, runBoundedNix, runIsolatedNix)
}

// evaluateAssistantCandidate evaluates the committed base with run and the
// candidate, which holds the model's changes, with isolated (no network).
func evaluateAssistantCandidate(ctx context.Context, w *AssistantWorkspace, run, isolated assistantNixRunner) (result domain.AssistantEvaluation, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	result.ChangedHosts = []string{}
	// A receipt describes only a completed evaluation of unchanged inputs.
	if err = w.invalidateEvaluationReceipt(); err != nil {
		result.Error = assistantEvaluationError(err.Error())
		return result, err
	}
	key, keyErr := assistantEvaluationInputs(ctx, w)
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil {
			if removeErr := unix.Unlinkat(int(w.root.Fd()), "candidate-drv-v25.json", 0); removeErr != nil && !errors.Is(removeErr, unix.ENOENT) {
				err = errors.Join(err, removeErr)
			}
			result.OK = false
			result.Error = assistantEvaluationError(err.Error())
		}
	}()
	candidateRef := ""
	evaluate := func(ref string, apply string, target any) error {
		args := []string{"eval", "--json", "--no-write-lock-file", "--no-update-lock-file", "--option", "allow-import-from-derivation", "false", ref}
		if apply != "" {
			args = append(args, "--apply", apply)
		}
		runner := run
		if candidateRef != "" && strings.HasPrefix(ref, candidateRef+"#") {
			runner = isolated
		}
		data, runErr := runner(ctx, 1024*1024, args...)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if runErr != nil {
			return runErr
		}
		return json.Unmarshal([]byte(data), target)
	}
	var cache assistantBaseEvaluationCache
	cacheData, cacheErr := assistantReadStateLimit(w.root, "base-drv-v25.json", 1024*1024)
	confirmed := cacheErr == nil
	if cacheErr == nil {
		if err := assistantDecodeStrict(cacheData, &cache); err != nil {
			return result, err
		}
	} else if !errors.Is(cacheErr, fs.ErrNotExist) {
		return result, cacheErr
	} else {
		cache = assistantBaseEvaluationCache{Output: "nixoriumComparisonConfigurations", Paths: map[string]string{}}
	}
	if (cache.Output != "nixoriumComparisonConfigurations" && cache.Output != "nixosConfigurations") || cache.Paths == nil {
		return result, errors.New("Invalid base evaluation cache.")
	}
	// Keep each process small even when host-specific modules create many classes.
	// Every batch shares the evaluation's original deadline and runs sequentially.
	evaluatePaths := func(ref string, names []string, paths map[string]string) error {
		for start := 0; start < len(names); start += 2 {
			if err := ctx.Err(); err != nil {
				return err
			}
			batch := names[start:min(start+2, len(names))]
			var values map[string]string
			err := evaluate(ref+"#"+cache.Output, assistantDrvExpression(batch), &values)
			if err != nil && !confirmed && strings.Contains(err.Error(), "nixoriumComparisonConfigurations") && strings.Contains(err.Error(), "does not provide attribute") {
				cache.Output = "nixosConfigurations"
				err = evaluate(ref+"#"+cache.Output, assistantDrvExpression(batch), &values)
			}
			if err != nil {
				return err
			}
			confirmed = true
			for _, name := range batch {
				if values[name] == "" {
					return errors.New("Incomplete host evaluation.")
				}
				paths[name] = values[name]
			}
		}
		return nil
	}
	baseRef := (&url.URL{Scheme: "git+file", Path: w.Conversation.Repository, RawQuery: "rev=" + w.Conversation.Base}).String()
	var baseClasses []assistantHostClass
	data, err := assistantReadStateLimit(w.root, "base-classes.json", 1024*1024)
	if errors.Is(err, fs.ErrNotExist) {
		err = evaluate(baseRef+"#nixoriumHostClasses", "", &baseClasses)
		if err != nil {
			if strings.Contains(err.Error(), "nixoriumHostClasses") && (strings.Contains(err.Error(), "does not provide attribute") || strings.Contains(err.Error(), "missing")) {
				return result, errors.New(assistantNeedsUpdate)
			}
			return result, err
		}
		data, _ = json.Marshal(baseClasses)
		if err = assistantWriteState(w.root, "base-classes.json", data); err != nil {
			return result, err
		}
	} else if err != nil {
		return result, err
	} else if err = assistantDecodeStrict(data, &baseClasses); err != nil {
		return result, err
	}
	baseMembership, err := assistantClassMembership(baseClasses)
	if err != nil {
		return result, err
	}
	directory, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", w.root.Fd()))
	if err != nil {
		return result, err
	}
	temporary, err := os.MkdirTemp(directory, "eval-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(temporary)
	candidate := filepath.Join(temporary, "src")
	if _, err = resetGit(ctx, temporary, nil, 1024*1024, "clone", "--quiet", "--shared", "--no-checkout", "--template=", "--", w.Conversation.Repository, candidate); err != nil {
		return result, err
	}
	if _, err = resetGit(ctx, candidate, nil, 1024*1024, "checkout", "--quiet", "--detach", w.Conversation.Base); err != nil {
		return result, err
	}
	if err = copyAssistantOverlay(w, candidate); err != nil {
		return result, err
	}
	if _, err = resetGit(ctx, candidate, nil, 1024*1024, "add", "-A"); err != nil {
		return result, err
	}
	candidateRef = (&url.URL{Scheme: "git+file", Path: candidate}).String()
	var candidateClasses []assistantHostClass
	if err = evaluate(candidateRef+"#nixoriumHostClasses", "", &candidateClasses); err != nil {
		return result, err
	}
	candidateMembership, err := assistantClassMembership(candidateClasses)
	if err != nil {
		return result, err
	}
	basePaths := cache.Paths
	representatives, missing := []string{}, []string{}
	for _, class := range candidateClasses {
		representatives = append(representatives, class.Representative)
		if _, exists := baseMembership[class.Representative]; exists && basePaths[class.Representative] == "" {
			missing = append(missing, class.Representative)
		}
	}
	if len(missing) > 0 {
		paths := map[string]string{}
		if err = evaluatePaths(baseRef, missing, paths); err != nil {
			return result, err
		}
		for _, name := range missing {
			if paths[name] == "" {
				return result, errors.New("Incomplete base evaluation.")
			}
			basePaths[name] = paths[name]
		}
		data, _ = json.Marshal(cache)
		if err = assistantWriteState(w.root, "base-drv-v25.json", data); err != nil {
			return result, err
		}
	}
	candidatePaths := map[string]string{}
	if err = evaluatePaths(candidateRef, representatives, candidatePaths); err != nil {
		return result, err
	}
	data, _ = json.Marshal(cache)
	if err = assistantWriteState(w.root, "base-drv-v25.json", data); err != nil {
		return result, err
	}
	changed := map[string]bool{}
	for _, class := range candidateClasses {
		if candidatePaths[class.Representative] == "" {
			return result, errors.New("Incomplete candidate evaluation.")
		}
		if candidatePaths[class.Representative] != basePaths[class.Representative] {
			for _, host := range class.Members {
				changed[host] = true
			}
		}
	}
	// When a representative leaves, remaining members get a new representative.
	// Counting them as changed is an intentional conservative overestimate.
	for host, representative := range candidateMembership {
		if baseMembership[host] != representative {
			changed[host] = true
		}
	}
	for host := range baseMembership {
		if _, exists := candidateMembership[host]; !exists {
			changed[host] = true
		}
	}
	for host := range changed {
		result.ChangedHosts = append(result.ChangedHosts, host)
	}
	sort.Strings(result.ChangedHosts)
	digest, err := assistantOverlayDigest(w)
	if err != nil {
		return result, err
	}
	data, err = json.Marshal(assistantCandidateEvaluationCache{Digest: digest, Output: cache.Output, Paths: candidatePaths})
	if err != nil {
		return result, err
	}
	if err = assistantWriteState(w.root, "candidate-drv-v25.json", data); err != nil {
		return result, err
	}
	result.OK = true
	// Record the receipt only when the inputs did not change while evaluating;
	// failing to record it never fails the evaluation itself.
	if after, afterErr := assistantEvaluationInputs(ctx, w); keyErr == nil && afterErr == nil && after == key && after.OverlayDigest == digest {
		receipt, _ := json.Marshal(assistantEvaluationReceipt{SchemaVersion: 1, Key: key, VerifiedAt: time.Now().UTC(), Evaluation: result})
		if assistantWriteState(w.root, assistantEvaluationReceiptName, receipt) != nil {
			_ = w.invalidateEvaluationReceipt()
		}
	}
	return result, nil
}
func assistantClassMembership(classes []assistantHostClass) (map[string]string, error) {
	members := map[string]string{}
	if len(classes) == 0 {
		return nil, errors.New("Invalid host classes.")
	}
	for _, class := range classes {
		if !assistantHostName.MatchString(class.Representative) || len(class.Members) == 0 {
			return nil, errors.New("Invalid host classes.")
		}
		found := false
		for _, host := range class.Members {
			if !assistantHostName.MatchString(host) || members[host] != "" {
				return nil, errors.New("Invalid host classes.")
			}
			members[host] = class.Representative
			found = found || host == class.Representative
		}
		if !found {
			return nil, errors.New("Invalid host classes.")
		}
	}
	return members, nil
}
func assistantDrvExpression(names []string) string {
	var expression strings.Builder
	expression.WriteString("cs: { ")
	for _, name := range names {
		quoted := strconv.Quote(name)
		expression.WriteString(quoted + " = cs." + quoted + ".config.system.build.toplevel.drvPath; ")
	}
	expression.WriteString("}")
	return expression.String()
}
func assistantEvaluationError(message string) string {
	message = domain.RedactForModel(message)
	lines := strings.Split(message, "\n")
	if len(lines) > 60 {
		message = strings.Join(lines[:40], "\n") + "\n…\n" + strings.Join(lines[len(lines)-20:], "\n")
	}
	if len(message) > 8192 {
		message = message[:8192]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
	}
	return message
}
func copyAssistantOverlay(w *AssistantWorkspace, candidate string) error {
	root, err := os.Open(candidate)
	if err != nil {
		return err
	}
	defer root.Close()
	files := map[string]domain.TemplateFile{}
	for path := range w.deleted {
		files[path] = domain.TemplateFile{}
	}

	if err = resetCheckCandidateDestinations(root, files); err != nil {
		return err
	}
	for path := range w.deleted {
		if err = os.Remove(filepath.Join(candidate, path)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		for parent := filepath.Dir(path); parent != "."; parent = filepath.Dir(parent) {
			if os.Remove(filepath.Join(candidate, parent)) != nil {
				break
			}
		}
	}
	files = map[string]domain.TemplateFile{}
	for path := range w.modes {
		files[path] = domain.TemplateFile{}
	}
	if err = resetCheckCandidateDestinations(root, files); err != nil {
		return err
	}
	for path, executable := range w.modes {
		if w.deleted[path] {
			continue
		}
		data, err := w.raw(path)
		if err != nil {
			return err
		}
		full := filepath.Join(candidate, path)
		if err = os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			return err
		}
		// Replace rather than follow any tracked symlink at the destination.
		if err = os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		mode := os.FileMode(0644)
		if executable {
			mode = 0755
		}
		if err = os.WriteFile(full, data, mode); err != nil {
			return err
		}
	}
	return nil
}
