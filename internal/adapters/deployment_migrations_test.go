package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

const oldControllerTest = "  isController = args ? hostName && args.hostName == (labSettings.masterHostName or null);\n"

func workstationMigration() domain.DeploymentMigration {
	return domain.DeploymentMigration{
		ID: "2026-10-workstation-is-controller", File: "modules/workstation.nix",
		Description: "Find the controller with isController.",
		From:        oldControllerTest, To: "  isController = args.isController or false;\n",
	}
}

func migrationRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	repository := t.TempDir()
	git := func(arguments ...string) {
		t.Helper()
		if _, err := run(context.Background(), "git", append([]string{"-C", repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, arguments...)...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	for name, content := range files {
		path := filepath.Join(repository, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	return repository
}

func TestDeploymentMigrationsNeverTouchProtectedFiles(t *testing.T) {
	valid := workstationMigration()
	if err := validateDeploymentMigrations([]domain.DeploymentMigration{valid}); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"flake.nix", "flake.lock", "lab-credentials.json", "secret-key", "keys/cache-public-key",
		"admin-ssh/id", ".git/config", "../outside", "/etc/passwd", "modules/../flake.nix", ".hidden", domain.DeploymentMigrationsFile} {
		migration := valid
		migration.File = file
		if validateDeploymentMigrations([]domain.DeploymentMigration{migration}) == nil {
			t.Fatalf("migration of %q accepted", file)
		}
	}
	for _, broken := range []func(*domain.DeploymentMigration){
		func(m *domain.DeploymentMigration) { m.ID = "workstation" },
		func(m *domain.DeploymentMigration) { m.From = "" },
		func(m *domain.DeploymentMigration) { m.To = m.From },
		func(m *domain.DeploymentMigration) { m.Description = "" },
	} {
		migration := valid
		broken(&migration)
		if validateDeploymentMigrations([]domain.DeploymentMigration{migration}) == nil {
			t.Fatalf("invalid migration accepted: %+v", migration)
		}
	}
	if validateDeploymentMigrations([]domain.DeploymentMigration{valid, valid}) == nil {
		t.Fatal("a repeated id was accepted")
	}
}

func TestDeploymentMigrationsApplyOnceAndRecordIt(t *testing.T) {
	repository := migrationRepository(t, map[string]string{"modules/workstation.nix": "args@{ ... }:\nlet\n" + oldControllerTest + "in { }\n"})
	results, files, err := planDeploymentMigrations(context.Background(), repository, []domain.DeploymentMigration{workstationMigration()})
	if err != nil || len(results) != 1 || results[0].State != domain.MigrationApplied || len(files) != 2 {
		t.Fatalf("results = %+v, files = %d, error = %v", results, len(files), err)
	}
	if !strings.Contains(string(files[0].After), "isController = args.isController or false;") || strings.Contains(string(files[0].After), "args.hostName") {
		t.Fatalf("migrated file = %q", files[0].After)
	}
	if files[1].Path != domain.DeploymentMigrationsFile || !files[1].Create || string(files[1].After) != "{\n  \"schemaVersion\": 1,\n  \"applied\": [\n    \"2026-10-workstation-is-controller\"\n  ]\n}\n" {
		t.Fatalf("record = %+v %q", files[1], files[1].After)
	}
	if err := applyMigratedFiles(repository, files); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(filepath.Join(repository, "modules/workstation.nix")); string(content) != string(files[0].After) {
		t.Fatalf("written = %q", content)
	}
	// Recorded: a later update skips it, even if the old text comes back.
	if _, err := run(context.Background(), "git", "-C", repository, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "modules/workstation.nix"), files[0].Before, 0o644); err != nil {
		t.Fatal(err)
	}
	results, files, err = planDeploymentMigrations(context.Background(), repository, []domain.DeploymentMigration{workstationMigration()})
	if err != nil || len(results) != 0 || len(files) != 0 {
		t.Fatalf("recorded migration ran again: %+v %d %v", results, len(files), err)
	}
}

func TestDeploymentMigrationsLeaveChangedFilesToTheAdministrator(t *testing.T) {
	cases := map[string]struct {
		content string
		state   string
		files   int
	}{
		"already done": {"  isController = args.isController or false;\n", domain.MigrationDone, 1},
		"changed":      {"  isController = true;\n", domain.MigrationManual, 0},
		"twice":        {oldControllerTest + oldControllerTest, domain.MigrationManual, 0},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			repository := migrationRepository(t, map[string]string{"modules/workstation.nix": test.content})
			results, files, err := planDeploymentMigrations(context.Background(), repository, []domain.DeploymentMigration{workstationMigration()})
			if err != nil || len(results) != 1 || results[0].State != test.state || len(files) != test.files {
				t.Fatalf("results = %+v, files = %d, error = %v", results, len(files), err)
			}
			if test.files == 1 && files[0].Path != domain.DeploymentMigrationsFile {
				t.Fatalf("only the record may change: %+v", files[0])
			}
		})
	}
	repository := migrationRepository(t, map[string]string{"README.md": "x\n"})
	results, _, err := planDeploymentMigrations(context.Background(), repository, []domain.DeploymentMigration{workstationMigration()})
	if err != nil || results[0].State != domain.MigrationManual || !strings.Contains(results[0].Detail, "not part of the saved deployment") {
		t.Fatalf("missing file = %+v, %v", results, err)
	}
}

func TestDeploymentMigrationsRefuseAFileChangedAfterReview(t *testing.T) {
	repository := migrationRepository(t, map[string]string{"modules/workstation.nix": oldControllerTest})
	_, files, err := planDeploymentMigrations(context.Background(), repository, []domain.DeploymentMigration{workstationMigration()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "modules/workstation.nix"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyMigratedFiles(repository, files); err == nil || !strings.Contains(err.Error(), "changed after update review") {
		t.Fatalf("apply = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, domain.DeploymentMigrationsFile)); !os.IsNotExist(err) {
		t.Fatal("the record was written although the review was stale")
	}
}

func TestDeploymentMigrationRecordMustBeValid(t *testing.T) {
	repository := migrationRepository(t, map[string]string{"modules/workstation.nix": oldControllerTest, domain.DeploymentMigrationsFile: "{\"applied\":[\"bad id\"]}\n"})
	if _, _, err := planDeploymentMigrations(context.Background(), repository, []domain.DeploymentMigration{workstationMigration()}); err == nil {
		t.Fatal("an invalid record was accepted")
	}
}
