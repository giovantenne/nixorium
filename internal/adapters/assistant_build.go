package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/giovantenne/nixorium/internal/domain"
)

type assistantCommandRunner func(ctx context.Context, limit int, name string, args ...string) (stdout string, stderr string, err error)

// Build commands need successful stderr too: Nix writes dry-run plans there.
func runBoundedCommand(ctx context.Context, limit int, name string, args ...string) (string, string, error) {
	if name == "nix" {
		args = append([]string{"--extra-experimental-features", "nix-command flakes"}, args...)
	}
	command := exec.CommandContext(ctx, name, args...)
	configureCommandCancellation(command)
	stdout := &boundedCommandBuffer{limit: limit}
	stderr := &boundedCommandBuffer{limit: limit}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if err != nil {
		message := strings.TrimSpace(stderr.buffer.String())
		if stderr.truncated {
			message += "\n<output truncated>"
		}
		if message == "" {
			message = err.Error()
		}
		err = fmt.Errorf("%s: %s", name, sanitizeOperationLog([]byte(message)))
	} else if stdout.truncated || stderr.truncated {
		err = fmt.Errorf("%s output exceeds the safety limit", name)
	}
	return stdout.buffer.String(), stderr.buffer.String(), err
}

type assistantBuildFailure struct {
	Name string `json:"name"`
	Log  string `json:"log"`
}
type assistantBuildSkipped struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}
type assistantBuildResult struct {
	OK        bool                    `json:"ok"`
	Built     int                     `json:"built"`
	Failed    []assistantBuildFailure `json:"failed"`
	Blocked   int                     `json:"blocked"`
	Skipped   []assistantBuildSkipped `json:"skipped"`
	Truncated bool                    `json:"truncated"`
}
type assistantBuildDerivation struct {
	dependencies map[string]bool
	fixed        bool
}

var assistantBuildDrvPath = regexp.MustCompile(`/nix/store/[0-9a-z]{32}-[^\s'"<>]+\.drv`)
var assistantBuildDownload = regexp.MustCompile(`will be fetched \(([0-9]+(?:\.[0-9]+)?) MiB download`)

func assistantBuildName(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".drv")
	if len(name) > 33 && name[32] == '-' {
		name = name[33:]
	}
	return name
}
func assistantBuildPath(path string) (string, error) {
	if !strings.HasPrefix(path, "/nix/store/") {
		path = "/nix/store/" + path
	}
	if assistantBuildDrvPath.FindString(path) != path {
		return "", errors.New("Invalid derivation path.")
	}
	return path, nil
}
func assistantBuildPaths(paths map[string]string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("Missing representative derivations.")
	}
	set := map[string]bool{}
	for _, path := range paths {
		path, err := assistantBuildPath(path)
		if err != nil {
			return nil, err
		}
		set[path] = true
	}
	return assistantBuildSorted(set), nil
}
func assistantBuildSorted(set map[string]bool) []string {
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		a, b := assistantBuildName(paths[i]), assistantBuildName(paths[j])
		if a == b {
			return paths[i] < paths[j]
		}
		return a < b
	})
	return paths
}

// Support both full-path inputDrvs and versioned, basename-keyed inputs.drvs.
func assistantBuildDecode(data string) (map[string]assistantBuildDerivation, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &document); err != nil {
		return nil, err
	}
	if nested, exists := document["derivations"]; exists {
		document = nil
		if err := json.Unmarshal(nested, &document); err != nil {
			return nil, err
		}
	}
	if len(document) == 0 {
		return nil, errors.New("Missing derivation JSON.")
	}
	result := map[string]assistantBuildDerivation{}
	for key, raw := range document {
		path, err := assistantBuildPath(key)
		if err != nil {
			return nil, err
		}
		var item struct {
			InputDrvs map[string]json.RawMessage `json:"inputDrvs"`
			Inputs    *struct {
				Drvs map[string]json.RawMessage `json:"drvs"`
			} `json:"inputs"`
			Outputs map[string]map[string]json.RawMessage `json:"outputs"`
			Env     map[string]json.RawMessage            `json:"env"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		deps := item.InputDrvs
		if deps == nil && item.Inputs != nil {
			deps = item.Inputs.Drvs
		}
		if deps == nil || len(item.Outputs) == 0 || item.Env == nil {
			return nil, errors.New("Incomplete derivation JSON.")
		}
		drv := assistantBuildDerivation{dependencies: map[string]bool{}}
		for key := range deps {
			dependency, err := assistantBuildPath(key)
			if err != nil {
				return nil, err
			}
			drv.dependencies[dependency] = true
		}
		_, drv.fixed = item.Env["outputHash"]
		for _, output := range item.Outputs {
			_, hash := output["hash"]
			_, outputHash := output["outputHash"]
			drv.fixed = drv.fixed || hash || outputHash
		}
		result[path] = drv
	}
	return result, nil
}
func assistantBuildWanted(stderr string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "will be fetched") {
			break
		}
		for _, path := range assistantBuildDrvPath.FindAllString(line, -1) {
			set[path] = true
		}
	}
	return set
}
func assistantBuildOrder(targets map[string]bool, drvs map[string]assistantBuildDerivation) ([]string, error) {
	remaining := map[string]bool{}
	for path := range targets {
		remaining[path] = true
	}
	order := make([]string, 0, len(targets))
	for len(remaining) > 0 {
		ready := map[string]bool{}
		for path := range remaining {
			blocked := false
			for dependency := range drvs[path].dependencies {
				blocked = blocked || remaining[dependency]
			}
			if !blocked {
				ready[path] = true
			}
		}
		if len(ready) == 0 {
			return nil, errors.New("Cyclic derivation dependencies.")
		}
		path := assistantBuildSorted(ready)[0]
		order = append(order, path)
		delete(remaining, path)
	}
	return order, nil
}
func assistantBuildLog(message string) string {
	lines := strings.Split(strings.TrimSpace(message), "\n")
	lines = lines[max(0, len(lines)-40):]
	message = domain.RedactForModel(strings.Join(lines, "\n"))
	if len(message) > 8192 {
		message = message[:8192]
	}
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message
}

func BuildAssistantCandidate(ctx context.Context, w *AssistantWorkspace, run assistantCommandRunner) string {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	failure := func(err error) string {
		if ctx.Err() != nil {
			return assistantToolResult(map[string]any{"ok": false, "error": "timeout"})
		}
		message := strings.SplitN(err.Error(), "\n", 2)[0]
		message = domain.RedactForModel(message)
		runes := []rune(message)
		message = string(runes[:min(300, len(runes))])
		return assistantToolResult(map[string]any{"ok": false, "error": "build_unavailable", "message": message})
	}
	if ctx.Err() != nil {
		return failure(ctx.Err())
	}
	data, err := assistantReadStateLimit(w.root, "candidate-drv-v25.json", 1024*1024)
	if errors.Is(err, fs.ErrNotExist) {
		return assistantToolResult(map[string]any{"ok": false, "error": "evaluate_first"})
	}
	if err != nil {
		return failure(err)
	}
	var candidate assistantCandidateEvaluationCache
	if err = assistantDecodeStrict(data, &candidate); err != nil {
		return failure(err)
	}
	if candidate.Digest == "" || (candidate.Output != "nixoriumComparisonConfigurations" && candidate.Output != "nixosConfigurations") {
		return failure(errors.New("Invalid candidate evaluation cache."))
	}
	digest, err := assistantOverlayDigest(w)
	if err != nil {
		return failure(err)
	}
	if candidate.Digest != digest {
		return assistantToolResult(map[string]any{"ok": false, "error": "evaluate_first"})
	}
	candidatePaths, err := assistantBuildPaths(candidate.Paths)
	if err != nil {
		return failure(err)
	}
	data, err = assistantReadStateLimit(w.root, "base-drv-v25.json", 1024*1024)
	if err != nil {
		return failure(err)
	}
	var base assistantBaseEvaluationCache
	if err = assistantDecodeStrict(data, &base); err != nil {
		return failure(err)
	}
	if base.Output != "nixoriumComparisonConfigurations" && base.Output != "nixosConfigurations" {
		return failure(errors.New("Invalid base evaluation cache."))
	}
	basePaths, err := assistantBuildPaths(base.Paths)
	if err != nil {
		return failure(err)
	}
	closure := func(paths []string) (map[string]bool, error) {
		stdout, _, err := run(ctx, 1024*1024, "nix-store", append([]string{"--query", "--requisites"}, paths...)...)
		if err != nil {
			return nil, err
		}
		set := map[string]bool{}
		for _, line := range strings.Fields(stdout) {
			if !strings.HasSuffix(line, ".drv") {
				continue
			}
			path, err := assistantBuildPath(line)
			if err != nil {
				return nil, err
			}
			set[path] = true
		}
		if len(set) == 0 {
			return nil, errors.New("Missing derivation closure.")
		}
		return set, nil
	}
	c, err := closure(candidatePaths)
	if err != nil {
		return failure(err)
	}
	b, err := closure(basePaths)
	if err != nil {
		return failure(err)
	}
	if ctx.Err() != nil {
		return failure(ctx.Err())
	}
	difference := map[string]bool{}
	for path := range c {
		if !b[path] {
			difference[path] = true
		}
	}
	if len(difference) > 3000 {
		return assistantToolResult(map[string]any{"ok": false, "error": "too_large"})
	}
	paths := assistantBuildSorted(difference)
	drvs := map[string]assistantBuildDerivation{}
	for start := 0; start < len(paths); start += 200 {
		if ctx.Err() != nil {
			return failure(ctx.Err())
		}
		batch := paths[start:min(start+200, len(paths))]
		stdout, _, err := run(ctx, 1024*1024, "nix", append([]string{"derivation", "show"}, batch...)...)
		if err != nil {
			return failure(err)
		}
		decoded, err := assistantBuildDecode(stdout)
		if err != nil {
			return failure(err)
		}
		for _, path := range batch {
			drv, exists := decoded[path]
			if !exists {
				return failure(errors.New("Missing derivation JSON."))
			}
			drvs[path] = drv
			if drv.fixed {
				delete(difference, path)
			}
		}
	}
	paths = assistantBuildSorted(difference)
	wanted := map[string]bool{}
	for start := 0; start < len(paths); start += 200 {
		if ctx.Err() != nil {
			return failure(ctx.Err())
		}
		args := []string{"build", "--dry-run", "--no-link"}
		for _, path := range paths[start:min(start+200, len(paths))] {
			args = append(args, path+"^*")
		}
		_, stderr, err := run(ctx, 1024*1024, "nix", args...)
		if err != nil {
			return failure(err)
		}
		for path := range assistantBuildWanted(stderr) {
			if difference[path] {
				wanted[path] = true
			}
		}
	}
	for _, path := range candidatePaths {
		delete(wanted, path)
	}
	if ctx.Err() != nil {
		return failure(ctx.Err())
	}
	order, err := assistantBuildOrder(wanted, drvs)
	if err != nil {
		return failure(err)
	}
	result := assistantBuildResult{OK: true, Failed: []assistantBuildFailure{}, Skipped: []assistantBuildSkipped{}, Truncated: len(order) > 40}
	order = order[:min(40, len(order))]
	status := map[string]string{}
	skip := func(path, reason string) {
		status[path] = "skipped"
		result.Skipped = append(result.Skipped, assistantBuildSkipped{assistantBuildName(path), reason})
	}
	for _, path := range order {
		if ctx.Err() != nil {
			skip(path, "timeout")
			continue
		}
		blocked := false
		for dependency := range drvs[path].dependencies {
			blocked = blocked || status[dependency] == "failed" || status[dependency] == "skipped" || status[dependency] == "blocked"
		}
		if blocked {
			status[path] = "blocked"
			result.Blocked++
			continue
		}
		_, stderr, err := run(ctx, 1024*1024, "nix", "build", "--dry-run", "--no-link", path+"^*")
		if ctx.Err() != nil {
			skip(path, "timeout")
			continue
		}
		if err != nil {
			skip(path, "dry_run_failed")
			continue
		}
		other := false
		for drv := range assistantBuildWanted(stderr) {
			other = other || drv != path
		}
		if other {
			skip(path, "needs_other_builds")
			continue
		}
		large := false
		for _, match := range assistantBuildDownload.FindAllStringSubmatch(stderr, -1) {
			mib, _ := strconv.ParseFloat(match[1], 64)
			large = large || mib > 100
		}
		if large {
			skip(path, "download_too_large")
			continue
		}
		_, _, err = run(ctx, 1024*1024, "nix", "build", "--no-link", "--max-jobs", "1", "--option", "max-silent-time", "60", path+"^*")
		if ctx.Err() != nil {
			skip(path, "timeout")
			continue
		}
		if err != nil {
			status[path] = "failed"
			result.OK = false
			result.Failed = append(result.Failed, assistantBuildFailure{assistantBuildName(path), assistantBuildLog(err.Error())})
		} else {
			status[path] = "ok"
			result.Built++
		}
	}
	return assistantToolResult(result)
}
