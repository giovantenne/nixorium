package adapters

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestCredentialsStayOutsidePublicSettingsAndStaleReviewsFail(t *testing.T) {
	repository := t.TempDir()
	local := Local{}
	candidate := adapterSettings()
	if err := local.WriteSettings(repository, candidate); err != nil {
		t.Fatal(err)
	}
	public, _ := os.ReadFile(filepath.Join(repository, settingsFileName))
	if bytes.Contains(public, []byte("$6$")) || bytes.Contains(public, []byte("Password")) {
		t.Fatal("public configuration contains a password hash")
	}
	current, err := local.ReadSettings(repository)
	if err != nil {
		t.Fatal(err)
	}
	decoded, issues := domain.DecodeLabSettings(current)
	if len(issues) != 0 || decoded.Lab.AdminPassword != candidate.Lab.AdminPassword {
		t.Fatal("local editor lost credentials")
	}
	candidate.Lab.AdminPassword = "$6$new$new-admin"
	if err := local.WriteSettingsIfUnchanged(repository, current, candidate); err == nil {
		t.Fatal("password change without a new public version accepted")
	}
	candidate.Lab.CredentialsVersion++
	if err := local.WriteSettingsIfUnchanged(repository, current, candidate); err != nil {
		t.Fatal(err)
	}
	if err := local.WriteSettingsIfUnchanged(repository, current, candidate); err != domain.ErrSettingsConflict {
		t.Fatalf("stale review: %v", err)
	}
}

func TestInterruptedCredentialSaveCanBeRepairedThroughSettings(t *testing.T) {
	repo := t.TempDir()
	settings := adapterSettings()
	local := Local{}
	if err := local.WriteSettings(repo, settings); err != nil {
		t.Fatal(err)
	}
	settings.Lab.CredentialsVersion++
	settings.Lab.AdminPassword = "$6$pending$admin"
	// Model a crash between the private and public renames.
	if err := saveCredentials(repo, settings); err != nil {
		t.Fatal(err)
	}
	data, err := local.ReadSettings(repo)
	if err != nil {
		t.Fatal(err)
	}
	editing, issues := domain.DecodeLabSettings(data)
	if len(issues) != 0 || domain.CredentialsFromSettings(editing).Ready() {
		t.Fatal("interrupted save claimed credential readiness")
	}
	settings.Lab.AdminPassword = "$6$retry$admin"
	if err := local.WriteSettingsIfUnchanged(repo, data, settings); err != nil {
		t.Fatal(err)
	}
	repaired, err := local.ReadSettings(repo)
	if err != nil || !bytes.Contains(repaired, []byte("$6$retry$admin")) {
		t.Fatal("password wizard could not repair the save")
	}
}

func TestCredentialFileRejectsSymlinksAndBroadPermissions(t *testing.T) {
	repo := t.TempDir()
	if err := (Local{}).WriteSettings(repo, adapterSettings()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, credentialsFile)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readCredentials(repo); err == nil {
		t.Fatal("readable credentials accepted")
	}
	if err := os.Rename(path, path+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path+".real", path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readCredentials(repo); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestEvaluationSourceCopyRefusesIgnoredSecretsAndEscapingLinks(t *testing.T) {
	for _, name := range []string{"secret-key", "admin-ssh", credentialsFile, ".nixorium-source-revision.json"} {
		t.Run(name, func(t *testing.T) {
			source := t.TempDir()
			if err := os.WriteFile(filepath.Join(source, name), []byte("secret"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := copyEvaluationSource(source, t.TempDir()); err == nil {
				t.Fatal("reserved file copied")
			}
		})
	}
	source := t.TempDir()
	if err := os.Symlink("../../private", filepath.Join(source, "module.nix")); err != nil {
		t.Fatal(err)
	}
	if err := copyEvaluationSource(source, t.TempDir()); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatal("escaping link copied")
	}
}
