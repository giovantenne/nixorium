package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func assistantReceiptFixture(t *testing.T) (*AssistantWorkspace, AssistantWorkspaceStore) {
	t.Helper()
	w, store, _ := assistantEvaluationFixture(t, false)
	if err := w.Write("course.nix", "{ value = 42; }", false); err != nil {
		t.Fatal(err)
	}
	return w, store
}
func assistantReadReceipt(t *testing.T, w *AssistantWorkspace) assistantEvaluationReceipt {
	t.Helper()
	data, err := assistantReadStateLimit(w.root, assistantEvaluationReceiptName, assistantEvaluationReceiptLimit)
	if err != nil {
		t.Fatal(err)
	}
	var receipt assistantEvaluationReceipt
	if err := assistantDecodeStrict(data, &receipt); err != nil {
		t.Fatal(err, string(data))
	}
	return receipt
}
func assistantWriteReceipt(t *testing.T, w *AssistantWorkspace, receipt assistantEvaluationReceipt) {
	t.Helper()
	data, _ := json.Marshal(receipt)
	if err := assistantWriteState(w.root, assistantEvaluationReceiptName, data); err != nil {
		t.Fatal(err)
	}
}
func assistantReceiptExists(t *testing.T, w *AssistantWorkspace) bool {
	t.Helper()
	var stat unix.Stat_t
	err := unix.Fstatat(int(w.root.Fd()), assistantEvaluationReceiptName, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil && !errors.Is(err, unix.ENOENT) {
		t.Fatal(err)
	}
	return err == nil
}

// A fake evaluation that fails fast proves only whether Nix was invoked.
func assistantCountingFailure(calls *atomic.Int32) assistantNixRunner {
	return func(context.Context, int, ...string) (string, error) {
		calls.Add(1)
		return "", errors.New("synthetic evaluation failure")
	}
}
func assistantNoNix(t *testing.T) assistantNixRunner {
	return func(_ context.Context, _ int, args ...string) (string, error) {
		t.Errorf("unexpected Nix evaluation: %v", args)
		return "", errors.New("unexpected Nix evaluation")
	}
}

func TestAssistantSaveReusesEvaluationReceipt(t *testing.T) {
	w, store := assistantReceiptFixture(t)
	evaluation, err := EvaluateAssistantCandidate(t.Context(), w)
	if err != nil || !evaluation.OK {
		t.Fatalf("evaluation=%+v error=%v", evaluation, err)
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(int(w.root.Fd()), assistantEvaluationReceiptName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil || stat.Mode&0777 != 0600 {
		t.Fatalf("receipt mode %o: %v", stat.Mode&0777, err)
	}
	receipt := assistantReadReceipt(t, w)
	key, err := assistantEvaluationInputs(t.Context(), w)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 1 || receipt.Key != key || receipt.VerifiedAt.IsZero() || !reflect.DeepEqual(receipt.Evaluation, evaluation) {
		t.Fatalf("receipt=%+v key=%+v", receipt, key)
	}
	if key.NixoriumVersion != "test" || key.Base != w.Conversation.Base || !strings.HasPrefix(key.FlakeLockDigest, "100644:") || len(key.OverlayDigest) != 64 {
		t.Fatalf("key: %+v", key)
	}
	// The save review and the apply step's revalidation both reuse it.
	plan := planAssistantSave(t.Context(), store, w.Conversation.ID, assistantNoNix(t))
	if plan.State != "ready" || !plan.EvaluationReused || !plan.EvaluationVerifiedAt.Equal(receipt.VerifiedAt) || !reflect.DeepEqual(plan.Evaluation, evaluation) {
		t.Fatalf("plan: %+v", plan)
	}
	again := planAssistantSave(t.Context(), store, w.Conversation.ID, assistantNoNix(t))
	if again.ReviewToken != plan.ReviewToken || !again.EvaluationReused {
		t.Fatalf("again: %+v", again)
	}
	encoded, _ := json.Marshal(plan)
	if !strings.Contains(string(encoded), `"evaluationReused":true`) || !strings.Contains(string(encoded), `"evaluationVerifiedAt":`) {
		t.Fatal(string(encoded))
	}
}

func TestAssistantSaveEvaluatesWhenReceiptMisses(t *testing.T) {
	w, store := assistantReceiptFixture(t)
	key, err := assistantEvaluationInputs(t.Context(), w)
	if err != nil {
		t.Fatal(err)
	}
	valid := assistantEvaluationReceipt{SchemaVersion: 1, Key: key, VerifiedAt: time.Now().Add(-time.Minute).UTC(), Evaluation: domain.AssistantEvaluation{OK: true, ChangedHosts: []string{"pc01", "pc02"}}}
	assistantWriteReceipt(t, w, valid)
	if plan := planAssistantSave(t.Context(), store, w.Conversation.ID, assistantNoNix(t)); !plan.EvaluationReused || strings.Join(plan.ChangedHosts, ",") != "pc01,pc02" {
		t.Fatalf("synthetic receipt not reused: %+v", plan)
	}
	for name, change := range map[string]func(*assistantEvaluationReceipt){
		"overlay":   func(r *assistantEvaluationReceipt) { r.Key.OverlayDigest = strings.Repeat("0", 64) },
		"base":      func(r *assistantEvaluationReceipt) { r.Key.Base = strings.Repeat("1", 40) },
		"flakeLock": func(r *assistantEvaluationReceipt) { r.Key.FlakeLockDigest = "100644:" + strings.Repeat("2", 64) },
		"version":   func(r *assistantEvaluationReceipt) { r.Key.NixoriumVersion = "3.7.0" },
		"failure": func(r *assistantEvaluationReceipt) {
			r.Evaluation = domain.AssistantEvaluation{OK: false, ChangedHosts: []string{}, Error: "recorded failure"}
		},
		"unchecked": func(r *assistantEvaluationReceipt) { r.Evaluation = domain.AssistantEvaluation{OK: true} },
		"undated":   func(r *assistantEvaluationReceipt) { r.VerifiedAt = time.Time{} },
		"schema":    func(r *assistantEvaluationReceipt) { r.SchemaVersion = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := valid
			change(&receipt)
			assistantWriteReceipt(t, w, receipt)
			var calls atomic.Int32
			plan := planAssistantSave(t.Context(), store, w.Conversation.ID, assistantCountingFailure(&calls))
			if calls.Load() == 0 || plan.EvaluationReused || !plan.EvaluationVerifiedAt.IsZero() || plan.Evaluation.OK {
				t.Fatalf("calls=%d plan=%+v", calls.Load(), plan)
			}
			if assistantReceiptExists(t, w) {
				t.Fatal("a failed evaluation left a receipt")
			}
		})
	}
	t.Run("running version", func(t *testing.T) {
		assistantWriteReceipt(t, w, valid)
		other := store
		other.Version = "3.7.1"
		var calls atomic.Int32
		if plan := planAssistantSave(t.Context(), other, w.Conversation.ID, assistantCountingFailure(&calls)); calls.Load() == 0 || plan.EvaluationReused {
			t.Fatalf("calls=%d plan=%+v", calls.Load(), plan)
		}
	})
	t.Run("no version", func(t *testing.T) {
		assistantWriteReceipt(t, w, valid)
		unversioned := store
		unversioned.Version = ""
		var calls atomic.Int32
		if plan := planAssistantSave(t.Context(), unversioned, w.Conversation.ID, assistantCountingFailure(&calls)); calls.Load() == 0 || plan.EvaluationReused {
			t.Fatalf("calls=%d plan=%+v", calls.Load(), plan)
		}
	})
	t.Run("oversized", func(t *testing.T) {
		data, _ := json.Marshal(valid)
		data = append(data, []byte(strings.Repeat(" ", assistantEvaluationReceiptLimit))...)
		if err := assistantWriteState(w.root, assistantEvaluationReceiptName, data); err != nil {
			t.Fatal(err)
		}
		if _, ok := assistantReusableEvaluation(w, key); ok {
			t.Fatal("oversized receipt trusted")
		}
		var calls atomic.Int32
		if plan := planAssistantSave(t.Context(), store, w.Conversation.ID, assistantCountingFailure(&calls)); calls.Load() == 0 || plan.EvaluationReused {
			t.Fatalf("calls=%d plan=%+v", calls.Load(), plan)
		}
	})
	for _, mode := range []uint32{0, 0644} {
		t.Run("mode "+map[uint32]string{0: "0000", 0644: "0644"}[mode], func(t *testing.T) {
			if mode == 0 && os.Geteuid() == 0 {
				t.Skip("root can read an unreadable file")
			}
			assistantWriteReceipt(t, w, valid)
			if err := unix.Fchmodat(int(w.root.Fd()), assistantEvaluationReceiptName, mode, 0); err != nil {
				t.Fatal(err)
			}
			if _, ok := assistantReusableEvaluation(w, key); ok {
				t.Fatal("unsafe receipt trusted")
			}
			var calls atomic.Int32
			if plan := planAssistantSave(t.Context(), store, w.Conversation.ID, assistantCountingFailure(&calls)); calls.Load() == 0 || plan.EvaluationReused {
				t.Fatalf("calls=%d plan=%+v", calls.Load(), plan)
			}
		})
	}
	t.Run("malformed", func(t *testing.T) {
		if err := assistantWriteState(w.root, assistantEvaluationReceiptName, []byte(`{"schemaVersion":1,"extra":true}`)); err != nil {
			t.Fatal(err)
		}
		if _, ok := assistantReusableEvaluation(w, key); ok {
			t.Fatal("malformed receipt trusted")
		}
	})
}

func TestAssistantWorkspaceWritesRemoveEvaluationReceipt(t *testing.T) {
	w, _ := assistantReceiptFixture(t)
	key, err := assistantEvaluationInputs(t.Context(), w)
	if err != nil {
		t.Fatal(err)
	}
	receipt := assistantEvaluationReceipt{SchemaVersion: 1, Key: key, VerifiedAt: time.Now().UTC(), Evaluation: domain.AssistantEvaluation{OK: true, ChangedHosts: []string{}}}
	for _, call := range []struct {
		name string
		args any
	}{
		{"write_file", map[string]any{"path": "special.nix", "content": "{ value = 5; }"}},
		{"delete_file", map[string]any{"path": "unused.nix"}},
	} {
		t.Run(call.name, func(t *testing.T) {
			assistantWriteReceipt(t, w, receipt)
			if result := assistantFileCall(t, w, call.name, call.args); result["ok"] != true {
				t.Fatal(result)
			}
			if assistantReceiptExists(t, w) {
				t.Fatal("receipt survived a workspace write")
			}
		})
	}
	// A refused write leaves the overlay and therefore the receipt unchanged.
	assistantWriteReceipt(t, w, receipt)
	if result := assistantFileCall(t, w, "write_file", map[string]any{"path": "flake.lock", "content": "{}"}); result["ok"] == true {
		t.Fatal(result)
	}
	if !assistantReceiptExists(t, w) {
		t.Fatal("refused write removed the receipt")
	}
}

func TestAssistantEvaluationReceiptNeedsUnchangedInputs(t *testing.T) {
	w, _ := assistantReceiptFixture(t)
	var calls atomic.Int32
	if result, err := evaluateAssistantCandidate(t.Context(), w, assistantCountingFailure(&calls)); err == nil || result.OK {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if assistantReceiptExists(t, w) {
		t.Fatal("failed evaluation recorded a receipt")
	}
	unversioned := *w
	unversioned.version = ""
	if result, err := EvaluateAssistantCandidate(t.Context(), &unversioned); err != nil || !result.OK {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if assistantReceiptExists(t, w) {
		t.Fatal("receipt recorded without a Nixorium version")
	}
	// The overlay changes while Nix runs: the result is not recorded.
	run := func(ctx context.Context, limit int, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "nixoriumComparisonConfigurations") && !strings.Contains(strings.Join(args, " "), "rev=") {
			if err := w.Write("special.nix", "{ value = 9; }", false); err != nil {
				t.Error(err)
			}
		}
		return runBoundedNix(ctx, limit, args...)
	}
	if result, err := evaluateAssistantCandidate(t.Context(), w, run); err != nil || !result.OK {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if assistantReceiptExists(t, w) {
		t.Fatal("receipt recorded for an overlay that changed during evaluation")
	}
}
