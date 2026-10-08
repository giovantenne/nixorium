package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantIntegrationFlake = `{
 outputs = { self }: let
  classes = import ./classes.nix;
  host = name: file: { config.system.build.toplevel.drvPath = "/nix/store/" + builtins.hashString "sha256" (name + builtins.toJSON (import file)) + ".drv"; };
 in {
  nixoriumHostClasses = classes;
  nixosConfigurations = {
   pc99 = host "pc99" ./controller.nix;
   pc01 = host "pc01" ./course.nix;
   pc02 = host "pc02" ./course.nix;
   pc03 = host "pc03" ./special.nix;
  };
  nixoriumSearchSoftwarePackages = request: assert request.limit == 50; [ { id = "python3"; version = "3.fixture"; summary = "Synthetic ghp_secret description"; label = "Python"; availability = "available"; } ];
 };
}`
const assistantIntegrationClasses = `[ { representative = "pc99"; members = [ "pc99" ]; } { representative = "pc01"; members = [ "pc01" "pc02" ]; } { representative = "pc03"; members = [ "pc03" ]; } ]`

func assistantEvaluationFixture(t *testing.T, legacy bool) (*AssistantWorkspace, AssistantWorkspaceStore, string) {
	t.Helper()
	if _, err := exec.LookPath("nix"); err != nil {
		t.Skip("real Nix integration requires nix in PATH")
	}
	repo := workspaceRepository(t)
	flake := assistantIntegrationFlake
	if legacy {
		flake = strings.Replace(flake, "nixoriumHostClasses = classes;", "", 1)
	}
	for path, data := range map[string]string{"flake.nix": flake, "classes.nix": assistantIntegrationClasses, "course.nix": "{ value = 1; }", "special.nix": "{ value = 2; }", "controller.nix": "{ value = 3; }", "unused.nix": "{}"} {
		writeGitReviewFile(t, repo, path, data)
	}
	workspaceTestGit(t, repo, "add", "-A")
	workspaceTestGit(t, repo, "commit", "-qm", "Assistant evaluation fixture")
	store := AssistantWorkspaceStore{Directory: t.TempDir()}
	os.Chmod(store.Directory, 0700)
	conversation, err := store.CreateConversation(repo)
	if err != nil {
		t.Fatal(err)
	}
	w, err := store.Open(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w, store, repo
}
func assistantGitSnapshot(t *testing.T, repo string) map[string]string {
	t.Helper()
	result := assistantRepositorySnapshot(t, repo)
	for _, args := range [][]string{{"status", "--porcelain"}, {"worktree", "list"}, {"for-each-ref"}} {
		result[strings.Join(args, " ")] = workspaceTestGit(t, repo, args...)
	}
	var names []string
	err := filepath.WalkDir(filepath.Join(repo, ".git"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		names = append(names, strings.TrimPrefix(path, repo))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result["git-files"] = strings.Join(names, "\n")
	return result
}
func TestAssistantEvaluationUsesClassesAndCachesBase(t *testing.T) {
	w, store, repo := assistantEvaluationFixture(t, false)
	before := assistantGitSnapshot(t, repo)
	calls := []string{}
	run := func(ctx context.Context, limit int, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "#nixosConfigurations") {
			if count := strings.Count(args[len(args)-1], ".config.system.build.toplevel.drvPath"); count < 1 || count > 2 {
				t.Fatalf("expected one or two representatives, got %d: %v", count, args)
			}
		}
		calls = append(calls, strings.Join(args, " "))
		return runBoundedNix(ctx, limit, args...)
	}
	evaluate := func(expected []string) {
		t.Helper()
		got, err := evaluateAssistantCandidate(t.Context(), w, run)
		if err != nil || !got.OK || !reflect.DeepEqual(got.ChangedHosts, expected) {
			t.Fatalf("evaluation=%+v error=%v want=%v", got, err, expected)
		}
	}
	evaluate([]string{})
	baseCalls := func() int {
		n := 0
		for _, call := range calls {
			if strings.Contains(call, "?rev=") {
				n++
			}
		}
		return n
	}
	if baseCalls() != 3 {
		t.Fatal(calls)
	}
	evaluate([]string{})
	if baseCalls() != 3 {
		t.Fatal("base cache was not reused", calls)
	}
	if err := w.Write("course.nix", "{ value = 4; }", false); err != nil {
		t.Fatal(err)
	}
	evaluate([]string{"pc01", "pc02"})
	w.Write("course.nix", "{ value = 1; }", false)
	w.Write("special.nix", "{ value = 8; }", false)
	evaluate([]string{"pc03"})
	w.Write("special.nix", "{ value = 2; }", false)
	w.Write("classes.nix", `[ { representative = "pc99"; members = [ "pc99" ]; } { representative = "pc01"; members = [ "pc01" ]; } { representative = "pc03"; members = [ "pc02" "pc03" ]; } ]`, false)
	evaluate([]string{"pc02"})
	w.Write("classes.nix", `[ { representative = "pc99"; members = [ "pc99" ]; } { representative = "pc02"; members = [ "pc02" ]; } { representative = "pc03"; members = [ "pc01" "pc03" ]; } ]`, false)
	evaluate([]string{"pc01", "pc02"})
	if baseCalls() != 4 {
		t.Fatal("new base representative was not evaluated once", calls)
	}
	w.Write("classes.nix", assistantIntegrationClasses, false)
	if err := w.Delete("unused.nix"); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("script.sh", "#!/bin/sh\necho synthetic\n", true); err != nil {
		t.Fatal(err)
	}
	inspected := false
	inspect := func(ctx context.Context, limit int, args ...string) (string, error) {
		for _, arg := range args {
			if strings.HasPrefix(arg, "git+file://") && !strings.Contains(arg, "?rev=") {
				path := strings.Split(strings.TrimPrefix(arg, "git+file://"), "#")[0]
				if info, err := os.Stat(filepath.Join(path, "script.sh")); err == nil {
					inspected = true
					if info.Mode().Perm() != 0755 {
						t.Fatal(info.Mode())
					}
					if _, err = os.Stat(filepath.Join(path, "unused.nix")); !os.IsNotExist(err) {
						t.Fatal("deleted file still exists")
					}
				}
			}
		}
		return run(ctx, limit, args...)
	}
	got, err := evaluateAssistantCandidate(t.Context(), w, inspect)
	if err != nil || !got.OK || !inspected {
		t.Fatal(got, err, inspected)
	}
	if !reflect.DeepEqual(before, assistantGitSnapshot(t, repo)) {
		t.Fatal("evaluation changed the original repository")
	}
	matches, _ := filepath.Glob(filepath.Join(store.Directory, "conversations", w.Conversation.ID, "eval-*"))
	if len(matches) != 0 {
		t.Fatal(matches)
	}
}
func TestAssistantEvaluationFailuresAndProtocol(t *testing.T) {
	w, store, _ := assistantEvaluationFixture(t, false)
	w.Write("course.nix", `throw "ghp_private Bearer private-token $6$salt$hash"`, false)
	result, err := EvaluateAssistantCandidate(t.Context(), w)
	if err == nil || result.OK || !strings.Contains(result.Error, "[REDACTED]") || strings.Contains(result.Error, "ghp_private") {
		t.Fatal(result, err)
	}
	w.Write("course.nix", "{ invalid = ; }", false)
	result, err = EvaluateAssistantCandidate(t.Context(), w)
	if err == nil || result.OK || result.Error == "" {
		t.Fatal(result, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = EvaluateAssistantCandidate(ctx, w)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, call := range []domain.AssistantToolCall{{Name: "evaluate", Arguments: json.RawMessage(`{}`)}, {Name: "search_packages", Arguments: json.RawMessage(`{"query":"python"}`)}} {
		response := AssistantWorkspaceTools{Store: store}.Execute(t.Context(), w.Conversation, call)
		if call.Name == "evaluate" {
			if !strings.Contains(response, "evaluation_failed") {
				t.Fatal(response)
			}
		} else {
			if !strings.Contains(response, `"attr":"python3"`) || strings.Contains(response, "ghp_secret") {
				t.Fatal(response)
			}
		}
	}
	legacy, _, _ := assistantEvaluationFixture(t, true)
	result, err = EvaluateAssistantCandidate(t.Context(), legacy)
	if err == nil || result.Error != assistantNeedsUpdate {
		t.Fatal(result, err)
	}
}
func TestAssistantHostClassesRejectUnsafeOrDuplicateMembers(t *testing.T) {
	for _, classes := range [][]assistantHostClass{nil, {{Representative: "pc01", Members: []string{"pc02"}}}, {{Representative: "x\";abort", Members: []string{"pc01"}}}, {{Representative: "pc01", Members: []string{"pc01", "pc01"}}}} {
		if _, err := assistantClassMembership(classes); err == nil {
			t.Fatal(classes)
		}
	}
	if message := assistantEvaluationError(strings.Repeat("ghp_secret\n", 10000)); len(message) > 8192 || strings.Contains(message, "ghp_secret") {
		t.Fatal("unbounded or unredacted diagnostics")
	}
}

func TestAssistantEvaluationStopsAfterFailedBatch(t *testing.T) {
	for _, mode := range []string{"error", "incomplete", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			w, store, repo := assistantEvaluationFixture(t, false)
			before := assistantGitSnapshot(t, repo)
			w.Write("flake.nix", strings.Replace(assistantIntegrationFlake, `pc03 = host "pc03" ./special.nix;`, `pc03 = host "pc03" ./special.nix; pc04 = host "pc04" ./special.nix;`, 1), false)
			w.Write("classes.nix", `[ { representative = "pc99"; members = [ "pc99" ]; } { representative = "pc01"; members = [ "pc01" ]; } { representative = "pc02"; members = [ "pc02" ]; } { representative = "pc03"; members = [ "pc03" ]; } { representative = "pc04"; members = [ "pc04" ]; } ]`, false)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			candidateCalls := 0
			var deadline time.Time
			run := func(ctx context.Context, limit int, args ...string) (string, error) {
				current, ok := ctx.Deadline()
				if !ok || (!deadline.IsZero() && current != deadline) {
					t.Fatal("batches must share one deadline")
				}
				deadline = current
				call := strings.Join(args, " ")
				if strings.Contains(call, "#nixosConfigurations") && !strings.Contains(call, "?rev=") {
					candidateCalls++
					if candidateCalls == 2 {
						switch mode {
						case "error":
							return "", errors.New("synthetic batch failure")
						case "incomplete":
							return `{}`, nil
						case "cancel":
							data, err := runBoundedNix(ctx, limit, args...)
							cancel()
							return data, err
						}
					}
				}
				return runBoundedNix(ctx, limit, args...)
			}
			result, err := evaluateAssistantCandidate(ctx, w, run)
			if err == nil || result.OK || len(result.ChangedHosts) != 0 || candidateCalls != 2 {
				t.Fatalf("partial evaluation must fail closed: %+v, %v, calls=%d", result, err, candidateCalls)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, assistantGitSnapshot(t, repo)) {
				t.Fatal("failed evaluation changed the deployment")
			}
			if matches, _ := filepath.Glob(filepath.Join(store.Directory, "conversations", w.Conversation.ID, "eval-*")); len(matches) != 0 {
				t.Fatal("temporary evaluation remains", matches)
			}
		})
	}
}
