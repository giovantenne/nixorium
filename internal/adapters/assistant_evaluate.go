package adapters

import (
	"context"
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
)

type assistantHostClass struct {
	Representative string   `json:"representative"`
	Members        []string `json:"members"`
}
type assistantNixRunner func(context.Context, int, ...string) (string, error)

var assistantHostName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

const assistantNeedsUpdate = "Update Nixorium in this deployment before using Assistant."

func EvaluateAssistantCandidate(ctx context.Context, workspace *AssistantWorkspace) (domain.AssistantEvaluation, error) {
	return evaluateAssistantCandidate(ctx, workspace, runBoundedNix)
}
func evaluateAssistantCandidate(ctx context.Context, w *AssistantWorkspace, run assistantNixRunner) (result domain.AssistantEvaluation, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	result.ChangedHosts = []string{}
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil {
			result.OK = false
			result.Error = assistantEvaluationError(err.Error())
		}
	}()
	evaluate := func(ref string, apply string, target any) error {
		args := []string{"eval", "--json", "--no-write-lock-file", "--no-update-lock-file", "--option", "allow-import-from-derivation", "false", ref}
		if apply != "" {
			args = append(args, "--apply", apply)
		}
		data, runErr := run(ctx, 1024*1024, args...)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if runErr != nil {
			return runErr
		}
		return json.Unmarshal([]byte(data), target)
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
	candidateRef := (&url.URL{Scheme: "git+file", Path: candidate}).String()
	var candidateClasses []assistantHostClass
	if err = evaluate(candidateRef+"#nixoriumHostClasses", "", &candidateClasses); err != nil {
		return result, err
	}
	candidateMembership, err := assistantClassMembership(candidateClasses)
	if err != nil {
		return result, err
	}
	basePaths := map[string]string{}
	data, err = assistantReadStateLimit(w.root, "base-drv.json", 1024*1024)
	if err == nil {
		err = assistantDecodeStrict(data, &basePaths)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return result, err
	}
	if basePaths == nil {
		return result, errors.New("Invalid base evaluation cache.")
	}
	representatives, missing := []string{}, []string{}
	for _, class := range candidateClasses {
		representatives = append(representatives, class.Representative)
		if _, exists := baseMembership[class.Representative]; exists && basePaths[class.Representative] == "" {
			missing = append(missing, class.Representative)
		}
	}
	if len(missing) > 0 {
		paths := map[string]string{}
		if err = evaluate(baseRef+"#nixosConfigurations", assistantDrvExpression(missing), &paths); err != nil {
			return result, err
		}
		for _, name := range missing {
			if paths[name] == "" {
				return result, errors.New("Incomplete base evaluation.")
			}
			basePaths[name] = paths[name]
		}
		data, _ = json.Marshal(basePaths)
		if err = assistantWriteState(w.root, "base-drv.json", data); err != nil {
			return result, err
		}
	}
	candidatePaths := map[string]string{}
	if err = evaluate(candidateRef+"#nixosConfigurations", assistantDrvExpression(representatives), &candidatePaths); err != nil {
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
	result.OK = true
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
