package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/deploymigrate"
	"github.com/giovantenne/nixorium/internal/domain"
)

// No test reaches a real target release unless it says so.
func init() {
	buildTargetMigrator = func(context.Context, string) (string, error) { return "", nil }
}

const oldControllerTest = "  isController = args ? hostName && args.hostName == (labSettings.masterHostName or null);\n"

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

// useMigrator makes the target release a migrator that runs in-process with
// the given registry, plus an optional extra change made behind its back.
func useMigrator(t *testing.T, registry []deploymigrate.Migration, extra func(directory string)) {
	t.Helper()
	build, runner := buildTargetMigrator, runMigrator
	buildTargetMigrator = func(context.Context, string) (string, error) {
		return "/nix/store/00000000000000000000000000000000-nixorium-3.10.0", nil
	}
	runMigrator = func(_ context.Context, _ string, directory string) ([]byte, error) {
		report, err := deploymigrate.Run(directory, "3.10.0", registry)
		if err != nil {
			return nil, err
		}
		if extra != nil {
			extra(directory)
		}
		return json.Marshal(report)
	}
	t.Cleanup(func() { buildTargetMigrator, runMigrator = build, runner })
}

func TestTheTargetReleaseMigratesACopyAndTheDifferenceIsReviewed(t *testing.T) {
	repository := migrationRepository(t, map[string]string{"modules/workstation.nix": "let\n" + oldControllerTest + "in { }\n"})
	useMigrator(t, deploymigrate.Registry, nil)
	results, files, raw, migrator, err := planDeploymentMigrations(context.Background(), repository, "path:/nix/store/00000000000000000000000000000000-source", true)
	if err != nil || len(results) != 1 || results[0].State != domain.MigrationApplied || migrator == "" || len(raw) == 0 {
		t.Fatalf("results = %+v, migrator = %q, error = %v", results, migrator, err)
	}
	if len(files) != 2 || files[0].Path != "modules/workstation.nix" || files[1].Path != domain.DeploymentMigrationsFile || !files[1].Create {
		t.Fatalf("files = %+v", files)
	}
	if !strings.Contains(string(files[0].After), "args.isController or false") {
		t.Fatalf("migrated = %q", files[0].After)
	}
	// The saved deployment itself is untouched until the update is saved.
	if content, _ := os.ReadFile(filepath.Join(repository, "modules/workstation.nix")); !strings.Contains(string(content), "args.hostName") {
		t.Fatal("planning changed the deployment")
	}
	if err := applyMigratedFiles(repository, files); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(filepath.Join(repository, domain.DeploymentMigrationsFile)); !strings.Contains(string(content), "2026-10-workstation-is-controller") {
		t.Fatalf("record = %q", content)
	}
}

func TestMigrationsRunOnlyWhenTheReleaseChanges(t *testing.T) {
	repository := migrationRepository(t, map[string]string{"modules/workstation.nix": oldControllerTest})
	useMigrator(t, deploymigrate.Registry, nil)
	if results, files, _, _, err := planDeploymentMigrations(context.Background(), repository, "path:/nix/store/00000000000000000000000000000000-source", false); err != nil || results != nil || files != nil {
		t.Fatalf("a package-base update ran migrations: %+v %+v %v", results, files, err)
	}
	buildTargetMigrator = func(context.Context, string) (string, error) { return "", nil }
	if results, files, _, _, err := planDeploymentMigrations(context.Background(), repository, "path:/nix/store/00000000000000000000000000000000-source", true); err != nil || results != nil || files != nil {
		t.Fatalf("a release without migrator ran migrations: %+v %+v %v", results, files, err)
	}
}

func TestUndeclaredOrForbiddenChangesRefuseTheUpdate(t *testing.T) {
	cases := map[string]func(directory string){
		"undeclared file": func(directory string) {
			_ = os.WriteFile(filepath.Join(directory, "README.md"), []byte("changed\n"), 0o644)
		},
		"flake.nix":      func(directory string) { _ = os.WriteFile(filepath.Join(directory, "flake.nix"), []byte("{}\n"), 0o644) },
		"keys":           func(directory string) { _ = os.WriteFile(filepath.Join(directory, "keys", "new"), []byte("x"), 0o644) },
		"symbolic link":  func(directory string) { _ = os.Symlink("/etc/passwd", filepath.Join(directory, "modules", "link.nix")) },
		"record removed": func(directory string) { _ = os.Remove(filepath.Join(directory, domain.DeploymentMigrationsFile)) },
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			repository := migrationRepository(t, map[string]string{"modules/workstation.nix": oldControllerTest, "README.md": "x\n", "flake.nix": "{ }\n", "keys/cache-public-key": "k\n",
				domain.DeploymentMigrationsFile: "{\"schemaVersion\":1,\"applied\":[]}\n"})
			useMigrator(t, deploymigrate.Registry, extra)
			if _, _, _, _, err := planDeploymentMigrations(context.Background(), repository, "path:/nix/store/00000000000000000000000000000000-source", true); err == nil {
				t.Fatal("the update was not refused")
			}
		})
	}
	// A migration that claims a change it did not make is refused too.
	repository := migrationRepository(t, map[string]string{"modules/workstation.nix": oldControllerTest})
	useMigrator(t, []deploymigrate.Migration{{ID: "2027-01-claims-too-much", Description: "x", Apply: func(deploymigrate.Tree) deploymigrate.Outcome {
		return deploymigrate.Outcome{State: deploymigrate.Applied, Files: []string{"modules/workstation.nix"}}
	}}}, nil)
	if _, _, _, _, err := planDeploymentMigrations(context.Background(), repository, "path:/nix/store/00000000000000000000000000000000-source", true); err == nil || !strings.Contains(err.Error(), "did not make") {
		t.Fatalf("error = %v", err)
	}
}

func TestMigrationsMayCreateAndRemoveFiles(t *testing.T) {
	repository := migrationRepository(t, map[string]string{"modules/old.nix": "{ }\n"})
	useMigrator(t, []deploymigrate.Migration{{ID: "2027-01-move-module", Description: "Move a module.", Apply: func(tree deploymigrate.Tree) deploymigrate.Outcome {
		content, mode, _, _ := tree.Read("modules/old.nix")
		if tree.Write("modules/new/module.nix", content, mode) != nil || tree.Remove("modules/old.nix") != nil {
			return deploymigrate.Outcome{State: deploymigrate.Manual, Detail: "cannot move"}
		}
		return deploymigrate.Outcome{State: deploymigrate.Applied, Files: []string{"modules/old.nix", "modules/new/module.nix"}}
	}}}, nil)
	_, files, _, _, err := planDeploymentMigrations(context.Background(), repository, "path:/nix/store/00000000000000000000000000000000-source", true)
	if err != nil || len(files) != 3 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	if err := applyMigratedFiles(repository, files); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, "modules/old.nix")); !os.IsNotExist(err) {
		t.Fatal("the old module is still there")
	}
	if content, _ := os.ReadFile(filepath.Join(repository, "modules/new/module.nix")); string(content) != "{ }\n" {
		t.Fatalf("new module = %q", content)
	}
	// Restoring puts the deployment back as reviewed.
	if err := restoreMigratedFiles(repository, files); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, "modules/old.nix")); err != nil {
		t.Fatal("the old module was not restored")
	}
	if _, err := os.Stat(filepath.Join(repository, "modules/new/module.nix")); !os.IsNotExist(err) {
		t.Fatal("the new module was not removed")
	}
}

func TestMigratedFilesAreWrittenOnlyAsReviewed(t *testing.T) {
	repository := migrationRepository(t, map[string]string{"modules/workstation.nix": oldControllerTest})
	useMigrator(t, deploymigrate.Registry, nil)
	_, files, _, _, err := planDeploymentMigrations(context.Background(), repository, "path:/nix/store/00000000000000000000000000000000-source", true)
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

func TestMigrationReportsAreChecked(t *testing.T) {
	for name, report := range map[string]string{
		"contract":      `{"contractVersion":2,"migrations":[]}`,
		"state":         `{"contractVersion":1,"migrations":[{"id":"2027-01-x","description":"x","state":"maybe"}]}`,
		"no files":      `{"contractVersion":1,"migrations":[{"id":"2027-01-x","description":"x","state":"applied"}]}`,
		"files on done": `{"contractVersion":1,"migrations":[{"id":"2027-01-x","description":"x","state":"done","files":["a.nix"]}]}`,
		"protected":     `{"contractVersion":1,"migrations":[{"id":"2027-01-x","description":"x","state":"applied","files":["flake.lock"]}]}`,
		"id":            `{"contractVersion":1,"migrations":[{"id":"x","description":"x","state":"done"}]}`,
		"not json":      `nope`,
	} {
		if _, _, err := parseMigrationReport([]byte(report)); err == nil {
			t.Fatalf("%s: report accepted", name)
		}
	}
	results, declared, err := parseMigrationReport([]byte(`{"contractVersion":1,"targetVersion":"3.10.0","future":true,"migrations":[{"id":"2027-01-x","description":"x","state":"applied","files":["modules/a.nix"]},{"id":"2027-01-y","description":"y","state":"skipped"}]}`))
	if err != nil || len(results) != 1 || !declared["modules/a.nix"] {
		t.Fatalf("results = %+v, %v", results, err)
	}
}
