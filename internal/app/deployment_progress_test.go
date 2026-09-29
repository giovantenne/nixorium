package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type waitingDeploymentSource struct {
	*fakeDeploymentSource
	verificationContextError error
}

func (s *waitingDeploymentSource) RunDeploymentPhase(ctx context.Context, plan domain.DeploymentPlanReport, phase domain.DeploymentPhase, output io.Writer) error {
	if phase == domain.DeploymentPhaseBuild {
		return s.fakeDeploymentSource.RunDeploymentPhase(ctx, plan, phase, output)
	}
	io.WriteString(output, "client exposes the reviewed revision\nwaiting for remote activation\n")
	<-ctx.Done()
	return &domain.DeploymentUnconfirmedError{Err: ctx.Err()}
}

func (s *waitingDeploymentSource) CurrentSystems(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.HostSystemProbe {
	s.verificationContextError = ctx.Err()
	return s.fakeDeploymentSource.CurrentSystems(ctx, hosts, timeout)
}

func TestDeploymentCancelledAfterClientUpdateNeverClaimsCompletionOrSafeRetry(t *testing.T) {
	source := &waitingDeploymentSource{fakeDeploymentSource: readyDeploymentSource()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observedOutput := false
	watchdog := time.AfterFunc(5*time.Second, cancel)
	defer watchdog.Stop()
	report := NewDeploymentManager(source).ExecuteWithProgress(ctx, "/deployment", "pc01", source.revision, "", io.Discard, func(progress domain.DeploymentProgress) {
		if progress.Phase == domain.DeploymentPhaseApply && len(progress.Output) > 0 {
			observedOutput = true
			cancel()
		}
	})
	if !observedOutput {
		t.Fatal("no live command output before cancellation")
	}
	if !report.HasErrors() || !report.RecoveryRequired || report.RetrySafe || report.ApplyCompleted {
		t.Fatalf("unsafe interrupted result: %+v", report)
	}
	if report.Verification.Verified != 1 || report.Verification.Recorded != 0 || len(source.recorded) != 0 {
		t.Fatalf("revision match was misreported as completed activation: %+v", report)
	}
	if source.verificationContextError != nil {
		t.Fatalf("cancelled context prevented observations: %v", source.verificationContextError)
	}
}

func TestDeploymentOutputIsBoundedTerminalSafeAndIndependent(t *testing.T) {
	w := &deploymentOutput{writer: io.Discard}
	w.Write([]byte(strings.Repeat("old\n", 5000) + "final\x1b]52;clipboard\a\u202e\r\n" + strings.Repeat("x", 300)))
	first := w.snapshot(domain.DeploymentProgress{})
	if first.LastOutputAt.IsZero() || len(first.Output) > 5 || len(w.tail) > 4096 {
		t.Fatalf("unbounded capture: %+v", first)
	}
	for _, line := range first.Output {
		if strings.ContainsAny(line, "\x1b\a\r\n\u202e") || len([]rune(line)) > 181 {
			t.Fatalf("unsafe line: %q", line)
		}
	}
	first.Output[0] = "changed"
	if strings.Contains(strings.Join(w.snapshot(domain.DeploymentProgress{}).Output, "\n"), "changed") {
		t.Fatal("snapshot aliases internal state")
	}
}

func TestDeploymentOutputPreservesWriteErrors(t *testing.T) {
	errExpected := errors.New("log write failed")
	w := &deploymentOutput{writer: deploymentErrorWriter{errExpected}}
	if _, err := w.Write([]byte("output")); !errors.Is(err, errExpected) {
		t.Fatalf("lost output failure: %v", err)
	}
}

type deploymentErrorWriter struct{ err error }

func (w deploymentErrorWriter) Write([]byte) (int, error) { return 0, w.err }
