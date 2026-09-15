package app

import (
	"errors"
	"testing"
)

type fakeOperationProgressSource struct {
	operation string
	data      []byte
	err       error
}

func (f *fakeOperationProgressSource) ReadOperationProgress(operation string) ([]byte, error) {
	f.operation = operation
	return f.data, f.err
}

func TestOperationProgressManagerLoadsAndValidatesTypedState(t *testing.T) {
	source := &fakeOperationProgressSource{data: []byte(`{
      "schemaVersion": 1,
      "operation": "pxe-prepare",
      "state": "running",
      "phase": "artifacts",
      "startedAt": "2026-09-15T10:00:00Z",
      "updatedAt": "2026-09-15T10:00:02Z",
      "recent": ["Building shared netboot artifacts"]
    }`)}
	progress, err := NewOperationProgressManager(source).Current("pxe-prepare")
	if err != nil || source.operation != "pxe-prepare" || progress.Phase != "artifacts" {
		t.Fatalf("progress = %+v, operation = %q, error = %v", progress, source.operation, err)
	}

	source.err = errors.New("missing")
	if _, err := NewOperationProgressManager(source).Current("pxe-prepare"); err == nil {
		t.Fatal("source error was not returned")
	}
	source.err = nil
	source.data = []byte(`{"schemaVersion":1}`)
	if _, err := NewOperationProgressManager(source).Current("pxe-prepare"); err == nil {
		t.Fatal("invalid progress was accepted")
	}
}
