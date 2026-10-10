package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func assistantBuildTestPath(name string) string {
	return "/nix/store/" + strings.Repeat("0", 32) + "-" + name + ".drv"
}

func assistantBuildTestWorkspace(t *testing.T) *AssistantWorkspace {
	t.Helper()
	w, _, _ := assistantWorkspaceFixture(t)
	digest, err := assistantOverlayDigest(w)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"candidate-drv-v25.json": assistantCandidateEvaluationCache{digest, "nixoriumComparisonConfigurations", map[string]string{"pc01": assistantBuildTestPath("root")}},
		"base-drv-v25.json":      assistantBaseEvaluationCache{"nixoriumComparisonConfigurations", map[string]string{"pc01": assistantBuildTestPath("base")}},
	} {
		data, _ := json.Marshal(value)
		if err := assistantWriteState(w.root, name, data); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

type assistantBuildTestRunner struct {
	t         *testing.T
	names     []string
	deps      map[string][]string
	fixed     map[string]string
	modern    bool
	dry       map[string]string
	fail      map[string]string
	dryFail   map[string]bool
	preflight string
	malformed bool
	built     []string
	dryCalls  int
	hook      func(stage string)
}

func (r *assistantBuildTestRunner) run(ctx context.Context, limit int, name string, args ...string) (string, string, error) {
	r.t.Helper()
	deadline, exists := ctx.Deadline()
	if !exists || time.Until(deadline) > 300*time.Second {
		r.t.Fatal("missing shared build deadline")
	}
	if limit != 1024*1024 {
		r.t.Fatal("unbounded command", limit)
	}
	stage := ""
	if name == "nix-store" {
		if !reflect.DeepEqual(args[:2], []string{"--query", "--requisites"}) {
			r.t.Fatal(args)
		}
		stage = "closure"
		if r.hook != nil {
			r.hook(stage)
		}
		if r.preflight == stage {
			return "", "", errors.New("ghp_synthetic " + strings.Repeat("è", 400) + "\nnot first")
		}
		if args[2] == assistantBuildTestPath("base") {
			return assistantBuildTestPath("base"), "", nil
		}
		paths := []string{assistantBuildTestPath("base"), assistantBuildTestPath("root"), "/nix/store/" + strings.Repeat("1", 32) + "-source"}
		for _, name := range r.names {
			paths = append(paths, assistantBuildTestPath(name))
		}
		return strings.Join(paths, "\n"), "", nil
	}
	if name != "nix" {
		r.t.Fatal(name, args)
	}
	if args[0] == "derivation" {
		stage = "show"
		if len(args)-2 > 200 {
			r.t.Fatal("show batch too large")
		}
		if r.hook != nil {
			r.hook(stage)
		}
		if r.preflight == stage {
			return "", "", errors.New("show unavailable")
		}
		if r.malformed {
			return "{}", "", nil
		}
		items := map[string]any{}
		for _, path := range args[2:] {
			name := assistantBuildName(path)
			deps := map[string]any{}
			for _, dependency := range r.deps[name] {
				key := assistantBuildTestPath(dependency)
				if r.modern {
					key = filepath.Base(key)
				}
				deps[key] = []string{"out"}
			}
			output := map[string]any{"path": "synthetic-output"}
			env := map[string]any{}
			if hash := r.fixed[name]; hash != "" {
				if hash == "env" {
					env["outputHash"] = "synthetic"
				} else {
					output[hash] = "synthetic"
				}
			}
			item := map[string]any{"outputs": map[string]any{"out": output}, "env": env}
			if r.modern {
				item["inputs"] = map[string]any{"drvs": deps}
				path = filepath.Base(path)
			} else {
				item["inputDrvs"] = deps
			}
			items[path] = item
		}
		var document any = items
		if r.modern {
			document = map[string]any{"version": 4, "derivations": items}
		}
		data, _ := json.Marshal(document)
		return string(data), "warning on stderr, not JSON", nil
	}
	if args[0] != "build" {
		r.t.Fatal(args)
	}
	if args[1] == "--dry-run" {
		r.dryCalls++
		stage = "selection"
		if r.dryCalls == 1 || len(args) > 4 {
			if len(args)-3 > 200 {
				r.t.Fatal("dry-run batch too large")
			}
			if r.hook != nil {
				r.hook(stage)
			}
			if r.preflight == stage {
				return "", "", errors.New("selection unavailable")
			}
			paths := []string{}
			for _, argument := range args[3:] {
				paths = append(paths, strings.TrimSuffix(argument, "^*"))
			}
			// A drv mentioned after fetches must not become a target.
			return "", strings.Join(paths, "\n") + "\n1 path will be fetched (1 MiB download, 2 MiB unpacked):\n" + assistantBuildTestPath("fetched"), nil
		}
		path := strings.TrimSuffix(args[3], "^*")
		short := assistantBuildName(path)
		if r.hook != nil {
			r.hook("dry:" + short)
		}
		if r.dryFail[short] {
			return "", "dry-run failed", errors.New("dry-run failed")
		}
		if text, exists := r.dry[short]; exists {
			return "", text, nil
		}
		return "", path, nil
	}
	if !reflect.DeepEqual(args[:7], []string{"build", "--no-link", "--max-jobs", "1", "--option", "max-silent-time", "60"}) {
		r.t.Fatal(args)
	}
	short := assistantBuildName(strings.TrimSuffix(args[7], "^*"))
	r.built = append(r.built, short)
	if r.hook != nil {
		r.hook("build:" + short)
	}
	if message, exists := r.fail[short]; exists {
		return "", message, errors.New(message)
	}
	return "", "successful diagnostic", nil
}

func assistantBuildTestResult(t *testing.T, w *AssistantWorkspace, ctx context.Context, r *assistantBuildTestRunner) assistantBuildResult {
	t.Helper()
	text := BuildAssistantCandidate(ctx, w, r.run)
	var result assistantBuildResult
	if err := json.Unmarshal([]byte(text), &result); err != nil || !strings.Contains(text, `"built"`) {
		t.Fatal(text, err)
	}
	return result
}

func TestAssistantBuildRequiresCurrentEvaluation(t *testing.T) {
	for _, stale := range []bool{false, true} {
		w, _, _ := assistantWorkspaceFixture(t)
		if stale {
			data, _ := json.Marshal(assistantCandidateEvaluationCache{"old-digest", "nixosConfigurations", map[string]string{"pc01": assistantBuildTestPath("root")}})
			if err := assistantWriteState(w.root, "candidate-drv-v25.json", data); err != nil {
				t.Fatal(err)
			}
		}
		result := BuildAssistantCandidate(t.Context(), w, func(context.Context, int, string, ...string) (string, string, error) {
			t.Fatal("must not run before evaluate")
			return "", "", nil
		})
		if result != `{"error":"evaluate_first","ok":false}` {
			t.Fatal(result)
		}
	}
}
func TestAssistantBuildFormatsExclusionsAndOrder(t *testing.T) {
	for _, modern := range []bool{false, true} {
		w := assistantBuildTestWorkspace(t)
		r := &assistantBuildTestRunner{t: t, modern: modern, names: []string{"z-leaf", "a-parent", "b-independent", "fixed-hash", "fixed-outputHash", "fixed-env"}, deps: map[string][]string{"a-parent": {"z-leaf"}, "root": {"a-parent"}}, fixed: map[string]string{"fixed-hash": "hash", "fixed-outputHash": "outputHash", "fixed-env": "env"}}
		result := assistantBuildTestResult(t, w, t.Context(), r)
		if !result.OK || result.Built != 3 || !reflect.DeepEqual(r.built, []string{"b-independent", "z-leaf", "a-parent"}) || result.Truncated {
			t.Fatal(result, r.built)
		}
	}
}
func TestAssistantBuildFailedAndBlockedLogs(t *testing.T) {
	w := assistantBuildTestWorkspace(t)
	r := &assistantBuildTestRunner{t: t, names: []string{"leaf", "parent", "grandparent"}, deps: map[string][]string{"parent": {"leaf"}, "grandparent": {"parent"}}, fail: map[string]string{"leaf": strings.Repeat("discarded early line\n", 50) + "ghp_synthetic\n" + strings.Repeat("à", 9000)}}
	result := assistantBuildTestResult(t, w, t.Context(), r)
	if result.OK || len(result.Failed) != 1 || result.Blocked != 2 || !reflect.DeepEqual(r.built, []string{"leaf"}) {
		t.Fatal(result, r.built)
	}
	log := result.Failed[0].Log
	if strings.Contains(log, "ghp_synthetic") || !strings.Contains(log, "[REDACTED]") || len(log) > 8192 || !utf8.ValidString(log) {
		t.Fatal("unsafe log", len(log))
	}
	if got := assistantBuildLog(strings.Repeat("early\n", 50) + strings.Repeat("late\n", 40)); strings.Contains(got, "early") || strings.Count(got, "late") != 40 {
		t.Fatal(got)
	}
}
func TestAssistantBuildSkipsUnsafeDryRunsAndBlocksDependents(t *testing.T) {
	w := assistantBuildTestWorkspace(t)
	r := &assistantBuildTestRunner{t: t, names: []string{"other", "large", "boundary", "dry-failed", "parent"}, deps: map[string][]string{"parent": {"dry-failed"}}, dryFail: map[string]bool{"dry-failed": true}, dry: map[string]string{
		"other":    assistantBuildTestPath("extra") + "\n" + assistantBuildTestPath("other"),
		"large":    "1 path will be fetched (100.01 MiB download, 300 MiB unpacked):",
		"boundary": "1 path will be fetched (100 MiB download, 300 MiB unpacked):",
	}}
	result := assistantBuildTestResult(t, w, t.Context(), r)
	want := []assistantBuildSkipped{{"dry-failed", "dry_run_failed"}, {"large", "download_too_large"}, {"other", "needs_other_builds"}}
	if !result.OK || result.Built != 1 || result.Blocked != 1 || !reflect.DeepEqual(result.Skipped, want) || !reflect.DeepEqual(r.built, []string{"boundary"}) {
		t.Fatal(result, r.built)
	}
}
func TestAssistantBuildTruncationAndBatching(t *testing.T) {
	w := assistantBuildTestWorkspace(t)
	r := &assistantBuildTestRunner{t: t}
	for i := 0; i < 205; i++ {
		r.names = append(r.names, fmt.Sprintf("item-%03d", i))
	}
	result := assistantBuildTestResult(t, w, t.Context(), r)
	if !result.OK || result.Built != 40 || !result.Truncated || r.built[39] != "item-039" {
		t.Fatal(result, r.built)
	}
	r.names = r.names[:40]
	r.dryCalls = 0
	r.built = nil
	result = assistantBuildTestResult(t, w, t.Context(), r)
	if result.Truncated || result.Built != 40 {
		t.Fatal(result)
	}
}
func TestAssistantBuildTooLarge(t *testing.T) {
	w := assistantBuildTestWorkspace(t)
	r := &assistantBuildTestRunner{t: t}
	for i := 0; i < 3000; i++ {
		r.names = append(r.names, fmt.Sprintf("item-%04d", i))
	}
	if got := BuildAssistantCandidate(t.Context(), w, r.run); got != `{"error":"too_large","ok":false}` {
		t.Fatal(got)
	}
}
func TestAssistantBuildPreliminaryErrors(t *testing.T) {
	for _, stage := range []string{"candidate-json", "base-json", "closure", "show", "selection", "missing-json"} {
		t.Run(stage, func(t *testing.T) {
			w := assistantBuildTestWorkspace(t)
			r := &assistantBuildTestRunner{t: t, names: []string{"leaf"}, preflight: stage, malformed: stage == "missing-json"}
			if stage == "candidate-json" || stage == "base-json" {
				file := "candidate-drv-v25.json"
				if stage == "base-json" {
					file = "base-drv-v25.json"
				}
				if err := assistantWriteState(w.root, file, []byte(`{"unknown":true}`)); err != nil {
					t.Fatal(err)
				}
			}
			text := BuildAssistantCandidate(t.Context(), w, r.run)
			var result struct {
				OK             bool
				Error, Message string
			}
			if err := json.Unmarshal([]byte(text), &result); err != nil || result.OK || result.Error != "build_unavailable" || result.Message == "" || len([]rune(result.Message)) > 300 || strings.Contains(result.Message, "\n") || strings.Contains(result.Message, "ghp_synthetic") {
				t.Fatal(text, err)
			}
		})
	}
}
func TestAssistantBuildDeadlineBeforeTargets(t *testing.T) {
	for _, stage := range []string{"closure", "show", "selection"} {
		w := assistantBuildTestWorkspace(t)
		ctx, cancel := context.WithCancel(t.Context())
		r := &assistantBuildTestRunner{t: t, names: []string{"leaf"}, hook: func(current string) {
			if current == stage {
				cancel()
			}
		}}
		got := BuildAssistantCandidate(ctx, w, r.run)
		cancel()
		if got != `{"error":"timeout","ok":false}` {
			t.Fatal(stage, got)
		}
	}
}
func TestAssistantBuildDeadlineSkipsRemainingTargets(t *testing.T) {
	w := assistantBuildTestWorkspace(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := &assistantBuildTestRunner{t: t, names: []string{"first", "second"}, hook: func(stage string) {
		if stage == "build:first" {
			cancel()
		}
	}}
	result := assistantBuildTestResult(t, w, ctx, r)
	if !result.OK || result.Built != 0 || !reflect.DeepEqual(result.Skipped, []assistantBuildSkipped{{"first", "timeout"}, {"second", "timeout"}}) {
		t.Fatal(result)
	}
}
func TestAssistantCommandRunnerKeepsBothOutputsAndLimits(t *testing.T) {
	stdout, stderr, err := runBoundedCommand(t.Context(), 1024, "sh", "-c", "printf stdout; printf stderr >&2")
	if stdout != "stdout" || stderr != "stderr" || err != nil {
		t.Fatal(stdout, stderr, err)
	}
	stdout, stderr, err = runBoundedCommand(t.Context(), 3, "sh", "-c", "printf stdout; printf stderr >&2")
	if stdout != "std" || stderr != "std" || err == nil {
		t.Fatal(stdout, stderr, err)
	}
	stdout, stderr, err = runBoundedCommand(t.Context(), 1024, "sh", "-c", "printf stdout; printf stderr >&2; exit 1")
	if stdout != "stdout" || stderr != "stderr" || err == nil {
		t.Fatal(stdout, stderr, err)
	}
}
