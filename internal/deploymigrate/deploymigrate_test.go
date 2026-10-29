package deploymigrate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

const oldWorkstation = "args@{ ... }:\nlet\n  isController = args ? hostName && args.hostName == (labSettings.masterHostName or null);\nin { }\n"

// workstationOnly runs the first migration alone, so its tests do not
// depend on the migrations added after it.
var workstationOnly = Registry[:1]

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
	report, err := Run(root, "3.10.0", workstationOnly)
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
	if report, err := Run(root, "3.10.0", workstationOnly); err != nil || len(report.Migrations) != 0 || read(t, root, "modules/workstation.nix") != oldWorkstation {
		t.Fatalf("second run = %+v, %v", report, err)
	}
}

func TestMigrationsLeaveChangedFilesAndKeepUnknownRecords(t *testing.T) {
	root := treeWith(t, map[string]string{
		"modules/workstation.nix": "  isController = true;\n",
		RecordFile:                "{\"schemaVersion\":1,\"applied\":[\"2027-01-from-a-newer-release\"]}\n",
	})
	report, err := Run(root, "3.10.0", workstationOnly)
	if err != nil || len(report.Migrations) != 1 || report.Migrations[0].State != Manual || !strings.Contains(report.Migrations[0].Detail, "Replace") {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if read(t, root, "modules/workstation.nix") != "  isController = true;\n" || !strings.Contains(read(t, root, RecordFile), "2027-01-from-a-newer-release") {
		t.Fatal("a manual migration changed files or the record")
	}
	done := treeWith(t, map[string]string{"modules/workstation.nix": "  isController = args.isController or false;\n"})
	if report, err := Run(done, "3.10.0", workstationOnly); err != nil || report.Migrations[0].State != Done || !strings.Contains(read(t, done, RecordFile), "2026-10-workstation-is-controller") {
		t.Fatalf("done = %+v, %v", report, err)
	}
}

func TestAnInvalidRecordIsRefused(t *testing.T) {
	root := treeWith(t, map[string]string{RecordFile: "{\"applied\":[\"no\"]}\n"})
	if _, err := Run(root, "3.10.0", workstationOnly); !errors.Is(err, ErrRefused) {
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

func TestJqIsAddedToEveryLaboratoryOnce(t *testing.T) {
	jq := []Migration{Registry[1]}
	original := "{\n  \"schemaVersion\": 1,\n  \"packages\": [\n    { \"package\": \"git\", \"scope\": { \"kind\": \"shared\" } }\n  ]\n}\n"
	root := treeWith(t, map[string]string{SoftwareFile: original})
	report, err := Run(root, "3.10.0", jq)
	if err != nil || len(report.Migrations) != 1 || report.Migrations[0].State != Applied || len(report.Migrations[0].Files) != 1 || report.Migrations[0].Files[0] != SoftwareFile {
		t.Fatalf("report = %+v, %v", report, err)
	}
	software, err := domain.DecodeLabSoftware([]byte(read(t, root, SoftwareFile)))
	if err != nil || len(software.Packages) != 2 || software.Packages[1].Package != "jq" || software.Packages[1].Scope.Kind != domain.SoftwareScopeShared {
		t.Fatalf("software = %+v, %v", software, err)
	}

	// An administrator who already chose jq, for some clients only, keeps it.
	chosen := "{\"schemaVersion\":1,\"packages\":[{\"package\":\"jq\",\"scope\":{\"kind\":\"clients\",\"clients\":[\"pc01\"]}}]}\n"
	kept := treeWith(t, map[string]string{SoftwareFile: chosen})
	if report, err := Run(kept, "3.10.0", jq); err != nil || report.Migrations[0].State != Done || read(t, kept, SoftwareFile) != chosen {
		t.Fatalf("chosen jq = %+v, %v", report, err)
	}

	// An unreadable declaration is left alone and explained.
	broken := treeWith(t, map[string]string{SoftwareFile: "{"})
	if report, err := Run(broken, "3.10.0", jq); err != nil || report.Migrations[0].State != Manual || !strings.Contains(report.Migrations[0].Detail, "add jq in Software") || read(t, broken, SoftwareFile) != "{" {
		t.Fatalf("broken = %+v, %v", report, err)
	}
}

func TestTeacherDockMigrationAppliesOnceAndRespectsEdits(t *testing.T) {
	dock := registryWith("teacher-dock")
	old := "  teacherFavorites = lib.optionals isController\n    [ (if labSettings.classroomView or true then \"nixorium-classroom.desktop\" else \"nixorium.desktop\") ]\n    ++ studentFavorites;\n"
	root := treeWith(t, map[string]string{"modules/workstation.nix": old})
	report, err := Run(root, "3.10.0", dock)
	if err != nil || report.Migrations[0].State != Applied || !strings.Contains(read(t, root, "modules/workstation.nix"), "    [ \"nixorium-classroom.desktop\" ]\n") {
		t.Fatalf("report = %+v, %v", report, err)
	}
	edited := strings.Replace(old, "nixorium.desktop", "other.desktop", 1)
	report, err = Run(treeWith(t, map[string]string{"modules/workstation.nix": edited}), "3.10.0", dock)
	if err != nil || report.Migrations[0].State != Manual {
		t.Fatalf("an edited line was not left alone: %+v, %v", report, err)
	}
	// The template of this release has the migrated line.
	template, err := os.ReadFile(filepath.Join("..", "..", "templates", "site", "modules", "workstation.nix"))
	if err == nil && !strings.Contains(string(template), "    [ \"nixorium-classroom.desktop\" ]\n") {
		t.Fatal("the template lacks the migrated line")
	}
}

// registryWith selects the migrations whose id contains part, so new entries
// never shift a test onto another migration.
func registryWith(part string) []Migration {
	var found []Migration
	for _, migration := range Registry {
		if strings.Contains(migration.ID, part) {
			found = append(found, migration)
		}
	}
	return found
}

func TestADeletionIsDoneWhenTheTextIsGone(t *testing.T) {
	tree := Tree{Root: treeWith(t, map[string]string{"a.txt": "keep\nremove me\nkeep\n", "b.txt": "keep\nkeep\n", "c.txt": "x\nx\n"})}
	if outcome := ReplaceExact(tree, "a.txt", "remove me\n", ""); outcome.State != Applied || read(t, tree.Root, "a.txt") != "keep\nkeep\n" {
		t.Fatalf("applied = %+v", outcome)
	}
	if outcome := ReplaceExact(tree, "b.txt", "remove me\n", ""); outcome.State != Done {
		t.Fatalf("already deleted = %+v", outcome)
	}
	if outcome := ReplaceExact(tree, "c.txt", "x\n", ""); outcome.State != Manual {
		t.Fatalf("ambiguous = %+v", outcome)
	}
	if outcome := ReplaceExact(tree, "missing.txt", "remove me\n", ""); outcome.State != Manual {
		t.Fatalf("missing file = %+v", outcome)
	}
}
