package domain

import (
	"strings"
	"testing"
)

const validOperationProgress = `{
  "schemaVersion": 1,
  "operation": "pxe-prepare",
  "state": "running",
  "phase": "clients",
  "startedAt": "2026-09-15T10:00:00Z",
  "updatedAt": "2026-09-15T10:00:10Z",
  "current": 2,
  "total": 10,
  "recent": ["Validated deployment configuration", "Built client pc02 (2/10)"]
}`

func TestDecodeOperationProgressStrictly(t *testing.T) {
	progress, err := DecodeOperationProgress([]byte(validOperationProgress))
	if err != nil {
		t.Fatal(err)
	}
	if progress.Phase != "clients" || progress.Current != 2 || progress.Total != 10 || len(progress.Recent) != 2 {
		t.Fatalf("progress = %+v", progress)
	}

	for _, mutation := range []string{
		strings.Replace(validOperationProgress, `"schemaVersion": 1`, `"schemaVersion": 2`, 1),
		strings.Replace(validOperationProgress, `"operation": "pxe-prepare"`, `"operation": "arbitrary"`, 1),
		strings.Replace(validOperationProgress, `"state": "running"`, `"state": "unknown"`, 1),
		strings.Replace(validOperationProgress, `"phase": "clients"`, `"phase": "shell"`, 1),
		strings.Replace(validOperationProgress, `"state": "running"`, `"state": "completed"`, 1),
		strings.Replace(validOperationProgress, `"updatedAt": "2026-09-15T10:00:10Z"`, `"updatedAt": "2026-09-14T10:00:10Z"`, 1),
		strings.Replace(validOperationProgress, `"current": 2`, `"current": 11`, 1),
		strings.Replace(validOperationProgress, `"total": 10`, `"total": 0`, 1),
		strings.Replace(validOperationProgress, `"Built client pc02 (2/10)"`, `"\u001b[31munsafe"`, 1),
		strings.Replace(validOperationProgress, `"recent": [`, `"unknown": true, "recent": [`, 1),
		validOperationProgress + `{}`,
	} {
		if _, err := DecodeOperationProgress([]byte(mutation)); err == nil {
			t.Fatalf("invalid operation progress accepted: %s", mutation)
		}
	}
}

func TestDecodeOperationProgressBoundsRecentActivity(t *testing.T) {
	tooMany := strings.Replace(validOperationProgress,
		`["Validated deployment configuration", "Built client pc02 (2/10)"]`,
		`["one", "two", "three", "four", "five", "six"]`, 1)
	if _, err := DecodeOperationProgress([]byte(tooMany)); err == nil {
		t.Fatal("more than five progress activities were accepted")
	}
}
