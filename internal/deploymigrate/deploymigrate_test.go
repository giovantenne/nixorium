package deploymigrate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const oldWorkstation = "args@{ ... }:\nlet\n  isController = args ? hostName && args.hostName == (labSettings.masterHostName or null);\nin { }\n"

func treeWith(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestTheWorkstationMigrationRunsOnceAndIsRecorded(t *testing.T) {
	root := treeWith(t, map[string]string{"modules/workstation.nix": oldWorkstation})
	report, err := Run(root, "3.10.0", Registry)
	if err != nil || len(report.Migrations) != 1 || report.Migrations[0].State != Applied || report.ContractVersion != 1 || report.TargetVersion != "3.10.0" {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if got := read(t, root, "modules/workstation.nix"); !strings.Contains(got, "isController = args.isController or false;") || strings.Contains(got, "args.hostName") {
		t.Fatalf("workstation.nix = %q", got)
	}
	if got := report.Migrations[0].Files; len(got) != 1 || got[0] != "modules/workstation.nix" {
		t.Fatalf("files = %v", got)
	}
	var record struct{ Applied []string }
	if json.Unmarshal([]byte(read(t, root, RecordFile)), &record) != nil || len(record.Applied) != 1 {
		t.Fatalf("record = %q", read(t, root, RecordFile))
	}
	// Recorded: it does not run again, even if the old text comes back.
	if err := os.WriteFile(filepath.Join(root, "modules/workstation.nix"), []byte(oldWorkstation), 0o644); err != nil {
		t.Fatal(err)
	}
	if report, err := Run(root, "3.10.0", Registry); err != nil || len(report.Migrations) != 0 || read(t, root, "modules/workstation.nix") != oldWorkstation {
		t.Fatalf("second run = %+v, %v", report, err)
	}
}

func TestMigrationsLeaveChangedFilesAndKeepUnknownRecords(t *testing.T) {
	root := treeWith(t, map[string]string{
		"modules/workstation.nix": "  isController = true;\n",
		RecordFile:                "{\"schemaVersion\":1,\"applied\":[\"2027-01-from-a-newer-release\"]}\n",
	})
	report, err := Run(root, "3.10.0", Registry)
	if err != nil || len(report.Migrations) != 1 || report.Migrations[0].State != Manual || !strings.Contains(report.Migrations[0].Detail, "Replace") {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if read(t, root, "modules/workstation.nix") != "  isController = true;\n" || !strings.Contains(read(t, root, RecordFile), "2027-01-from-a-newer-release") {
		t.Fatal("a manual migration changed files or the record")
	}
	done := treeWith(t, map[string]string{"modules/workstation.nix": "  isController = args.isController or false;\n"})
	if report, err := Run(done, "3.10.0", Registry); err != nil || report.Migrations[0].State != Done || !strings.Contains(read(t, done, RecordFile), "2026-10-workstation-is-controller") {
		t.Fatalf("done = %+v, %v", report, err)
	}
}

func TestAnInvalidRecordIsRefused(t *testing.T) {
	root := treeWith(t, map[string]string{RecordFile: "{\"applied\":[\"no\"]}\n"})
	if _, err := Run(root, "3.10.0", Registry); !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v", err)
	}
}

func TestTheTreeStaysInsideTheDeployment(t *testing.T) {
	root := treeWith(t, map[string]string{"a/file": "x"})
	if err := os.Symlink("/etc", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	tree := Tree{Root: root}
	for _, name := range []string{"../outside", "/etc/passwd", "a/../../x", "link/passwd", "link", "", "."} {
		if err := tree.Write(name, []byte("x"), 0o644); err == nil {
			t.Fatalf("write of %q accepted", name)
		}
	}
	if _, _, _, err := tree.Read("link"); err == nil {
		t.Fatal("a symbolic link was read")
	}
	if err := tree.Remove("a/file"); err != nil {
		t.Fatal(err)
	}
}

func TestEveryRegistryIDIsValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, migration := range Registry {
		if !idPattern.MatchString(migration.ID) || seen[migration.ID] || migration.Description == "" || migration.Apply == nil {
			t.Fatalf("invalid registry entry %+v", migration)
		}
		seen[migration.ID] = true
	}
	if !SupportsContract(ContractVersion) || SupportsContract(ContractVersion+1) {
		t.Fatal("contract versions")
	}
}
