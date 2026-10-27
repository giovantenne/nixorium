package adapters

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestAssistantOverlayDigestBindsContentModeAndDeletion(t *testing.T) {
	w, _, _ := assistantEvaluationFixture(t, false)
	if err := w.Write("z.sh", "echo test\n", true); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("a.txt", "hello", false); err != nil {
		t.Fatal(err)
	}
	if err := w.Delete("unused.nix"); err != nil {
		t.Fatal(err)
	}
	lines := fmt.Sprintf("a.txt\x000644\x00%x\nunused.nix\x00deleted\nz.sh\x000755\x00%x\n", sha256.Sum256([]byte("hello")), sha256.Sum256([]byte("echo test\n")))
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(lines)))
	got, err := assistantOverlayDigest(w)
	if err != nil || got != want {
		t.Fatal(got, err, want)
	}
	if err := w.Write("z.sh", "echo test\n", false); err != nil {
		t.Fatal(err)
	}
	modeDigest, err := assistantOverlayDigest(w)
	if err != nil || modeDigest == got {
		t.Fatal("mode must affect digest", modeDigest, err)
	}
	if err := w.Write("a.txt", "changed", false); err != nil {
		t.Fatal(err)
	}
	contentDigest, err := assistantOverlayDigest(w)
	if err != nil || contentDigest == modeDigest {
		t.Fatal("content must affect digest", contentDigest, err)
	}
	w.modes["missing/overlay.txt"] = false
	if _, err := assistantOverlayDigest(w); err == nil {
		t.Fatal("missing overlay must fail")
	}
}

func TestAssistantCandidateEvaluationStateAndFailedEvaluation(t *testing.T) {
	w, _, _ := assistantEvaluationFixture(t, false)
	result, err := evaluateAssistantCandidate(t.Context(), w, runBoundedNix, runBoundedNix)
	if err != nil || !result.OK {
		t.Fatal(result, err)
	}
	data, err := assistantReadStateLimit(w.root, "candidate-drv-v25.json", 1024*1024)
	var cache assistantCandidateEvaluationCache
	if err != nil || assistantDecodeStrict(data, &cache) != nil {
		t.Fatal(string(data), err)
	}
	digest, err := assistantOverlayDigest(w)
	if err != nil || cache.Digest != digest || cache.Output != "nixoriumComparisonConfigurations" || len(cache.Paths) != 3 {
		t.Fatal(cache, digest, err)
	}
	if err := w.Write("course.nix", "{ value = 8; }", false); err != nil {
		t.Fatal(err)
	}
	changedDigest, err := assistantOverlayDigest(w)
	if err != nil || changedDigest == cache.Digest {
		t.Fatal("old candidate cache must be identifiable as stale", changedDigest, err)
	}
	run := func(ctx context.Context, limit int, args ...string) (string, error) {
		call := strings.Join(args, " ")
		if strings.Contains(call, "#nixoriumComparisonConfigurations") && !strings.Contains(call, "?rev=") {
			return "", errors.New("synthetic candidate failure")
		}
		return runBoundedNix(ctx, limit, args...)
	}
	result, err = evaluateAssistantCandidate(t.Context(), w, run, run)
	if err == nil || result.OK {
		t.Fatal(result, err)
	}
	if _, err := assistantReadStateLimit(w.root, "candidate-drv-v25.json", 1024*1024); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("failed evaluation must remove candidate state", err)
	}
}
