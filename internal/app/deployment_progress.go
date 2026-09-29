package app

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Capture never calls the UI from an output-copy goroutine. Polling coalesces
// chatty output without blocking the subprocess on a terminal redraw.
type deploymentOutput struct {
	mu     sync.Mutex
	writer io.Writer
	tail   []byte
	last   time.Time
}

func (w *deploymentOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.writer.Write(data)
	if n > 0 {
		w.last = time.Now()
		const limit = 4096
		if n >= limit {
			w.tail = append(w.tail[:0], data[n-limit:n]...)
		} else {
			w.tail = append(w.tail, data[:n]...)
			if len(w.tail) > limit {
				w.tail = append([]byte(nil), w.tail[len(w.tail)-limit:]...)
			}
		}
	}
	return n, err
}

func (w *deploymentOutput) snapshot(progress domain.DeploymentProgress) domain.DeploymentProgress {
	w.mu.Lock()
	defer w.mu.Unlock()
	progress.LastOutputAt = w.last
	text := strings.ToValidUTF8(string(w.tail), "�")
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return '\n'
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return '�'
		}
		return r
	}, text)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 180 {
			line = string(runes[:180]) + "…"
		}
		progress.Output = append(progress.Output, line)
		if len(progress.Output) > 5 {
			progress.Output = progress.Output[1:]
		}
	}
	return progress
}

func (m *DeploymentManager) runPhase(ctx context.Context, plan domain.DeploymentPlanReport, progress domain.DeploymentProgress, output io.Writer, observe func(domain.DeploymentProgress)) error {
	if observe == nil {
		return m.source.RunDeploymentPhase(ctx, plan, progress.Phase, output)
	}
	capture := &deploymentOutput{writer: output}
	finished := make(chan error, 1)
	go func() { finished <- m.source.RunDeploymentPhase(ctx, plan, progress.Phase, capture) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-finished:
			emitDeploymentProgress(observe, capture.snapshot(progress))
			return err
		case <-ticker.C:
			emitDeploymentProgress(observe, capture.snapshot(progress))
		}
	}
}
