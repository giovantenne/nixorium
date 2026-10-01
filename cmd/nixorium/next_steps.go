package main

import (
	"bytes"
	"io"
	"sync"

	"github.com/giovantenne/nixorium/internal/domain"
)

// nextStepRecorder watches command output for recognized blockers so a failed
// command ends with the next step, whichever renderer printed the problem.
type nextStepRecorder struct {
	mutex   sync.Mutex
	pending []byte
	codes   []string
	steps   map[string]domain.NextStep
}

func (recorder *nextStepRecorder) Write(data []byte) (int, error) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	recorder.pending = append(recorder.pending, data...)
	for {
		index := bytes.IndexByte(recorder.pending, '\n')
		if index < 0 {
			break
		}
		recorder.observe(string(recorder.pending[:index]))
		recorder.pending = recorder.pending[index+1:]
	}
	if len(recorder.pending) > 64*1024 {
		recorder.pending = recorder.pending[:0]
	}
	return len(data), nil
}

func (recorder *nextStepRecorder) observe(line string) {
	step, found := domain.NextStepFor(line)
	if !found {
		return
	}
	if recorder.steps == nil {
		recorder.steps = map[string]domain.NextStep{}
	}
	if _, seen := recorder.steps[step.Code]; !seen {
		recorder.steps[step.Code] = step
		recorder.codes = append(recorder.codes, step.Code)
	}
}

func (recorder *nextStepRecorder) report(writer io.Writer) {
	recorder.mutex.Lock()
	if len(recorder.pending) > 0 {
		recorder.observe(string(recorder.pending))
		recorder.pending = nil
	}
	codes := append([]string(nil), recorder.codes...)
	steps := recorder.steps
	recorder.mutex.Unlock()
	for _, code := range codes {
		_, _ = io.WriteString(writer, steps[code].Line()+"\n")
	}
}
