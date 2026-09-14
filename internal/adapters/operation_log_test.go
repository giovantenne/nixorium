package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeploymentOperationCreatesPrivateDurableLogAndSerializesRuns(t *testing.T) {
	stateRoot := t.TempDir()
	now := time.Date(2026, 9, 13, 12, 30, 0, 123, time.UTC)
	operation, err := openDeploymentOperation(stateRoot, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := operation.Writer().Write([]byte("deployment output\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeploymentOperation(stateRoot, now.Add(time.Second)); err == nil {
		t.Fatal("concurrent deployment operation was accepted")
	}
	path := operation.Path
	if err := operation.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("log mode = %o, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "deployment output\n" {
		t.Fatalf("log = %q, error = %v", data, err)
	}
	second, err := openDeploymentOperation(stateRoot, now.Add(time.Second))
	if err != nil {
		t.Fatalf("retry after close: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentOperationRejectsSymlinkDirectory(t *testing.T) {
	stateRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(stateRoot, "nixorium"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(stateRoot, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(stateRoot, "nixorium", "operations")); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeploymentOperation(stateRoot, time.Now()); err == nil {
		t.Fatal("symlink operation directory was accepted")
	}
}

func TestDeploymentOperationRejectsSymlinkLock(t *testing.T) {
	stateRoot := t.TempDir()
	directory := filepath.Join(stateRoot, "nixorium", "operations")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(stateRoot, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, "deploy.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeploymentOperation(stateRoot, time.Now()); err == nil {
		t.Fatal("symlink deployment lock was accepted")
	}
}

func TestOperationLogsListsNewestRecognizedPrivateFiles(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	directory := filepath.Join(stateRoot, "nixorium", "operations")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"deploy-20260914T103000.000000000Z-10.log": "old\nResult: failed\n",
		"deploy-20260914T113000.000000000Z-11.log": "new\nResult: completed\n",
		"notes.log": "not a Nixorium operation",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := (Local{}).OperationLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].ID != "deploy-20260914T113000.000000000Z-11.log" || logs[0].State != "completed" || !logs[0].Available || logs[0].Kind != "deployment" {
		t.Fatalf("logs = %+v", logs)
	}
}

func TestOperationLogsExposeUnsafeRecognizedFileWithoutReadingIt(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	directory := filepath.Join(stateRoot, "nixorium", "operations")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	name := "deploy-20260914T113000.000000000Z-11.log"
	if err := os.WriteFile(filepath.Join(directory, name), []byte("secret-looking data"), 0644); err != nil {
		t.Fatal(err)
	}
	logs, err := (Local{}).OperationLogs(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Available || logs[0].State != "unavailable" || !strings.Contains(logs[0].Detail, "0600") {
		t.Fatalf("logs = %+v", logs)
	}
}

func TestOperationLogShowRejectsArbitraryPathsAndSanitizesTail(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	directory := filepath.Join(stateRoot, "nixorium", "operations")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	name := "deploy-20260914T113000.000000000Z-11.log"
	content := strings.Repeat("x", 64) + "\nunsafe:\x1b[31m\roverwrite\nResult: partial\n"
	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := (Local{}).OperationLog(name, 32)
	if err != nil {
		t.Fatal(err)
	}
	if report.Log == nil || report.Log.State != "partial" || !report.Truncated || strings.ContainsAny(report.Content, "\x1b\r") || !strings.Contains(report.Content, "Result: partial") {
		t.Fatalf("report = %+v", report)
	}
	for _, id := range []string{"../../etc/passwd", "deploy.lock", "/absolute.log"} {
		if _, err := (Local{}).OperationLog(id, 32); err == nil {
			t.Fatalf("unsafe ID %q was accepted", id)
		}
	}
}

func TestOperationLogsRequirePrivateOwnedDirectories(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	directory := filepath.Join(stateRoot, "nixorium", "operations")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(stateRoot, "nixorium"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).OperationLogs(10); err == nil || !strings.Contains(err.Error(), "0700") {
		t.Fatalf("unsafe directory error = %v", err)
	}
}
