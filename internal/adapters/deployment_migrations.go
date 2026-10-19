package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Deployment migrations (ADR 0034): a release lists, as data, the exact
// changes it needs in a deployment's own files. Update Nixorium reads the
// list of the release it updates to, applies it to the reviewed candidate,
// shows it in the review and saves it with flake.nix and flake.lock.

const (
	maximumDeploymentMigrations = 64
	maximumMigrationText        = 64 * 1024
	maximumMigratedFile         = 1024 * 1024
)

var (
	migrationIDPattern       = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[a-z0-9-]{1,64}$`)
	migrationCandidatePrefix = regexp.MustCompile(`^path:/nix/store/[0-9a-z]{32}-[A-Za-z0-9+._?=-]+$`)
)

// Files a migration never touches: the update input itself (Update Nixorium
// changes it on its own), keys, credentials and Git metadata.
var protectedMigrationFiles = map[string]bool{
	"flake.nix": true, "flake.lock": true, "lab-credentials.json": true, "secret-key": true,
}

// candidateDeploymentMigrations reads the migration list of the Nixorium
// release the candidate pins. A release without one has no migrations.
func candidateDeploymentMigrations(ctx context.Context, candidate string) ([]domain.DeploymentMigration, error) {
	if !migrationCandidatePrefix.MatchString(candidate) {
		return nil, errors.New("deployment migrations need a prepared candidate source")
	}
	expression := `let release = (builtins.getFlake "` + candidate + `").inputs.nixorium or {}; in (release.lib or {}).deploymentMigrations or []`
	output, err := runBoundedNix(ctx, 1024*1024, "eval", "--impure", "--json", "--expr", expression)
	if err != nil {
		return nil, fmt.Errorf("read the deployment migrations of the target release: %w", err)
	}
	var migrations []domain.DeploymentMigration
	if err := json.Unmarshal([]byte(output), &migrations); err != nil {
		return nil, errors.New("the target release lists invalid deployment migrations")
	}
	if err := validateDeploymentMigrations(migrations); err != nil {
		return nil, err
	}
	return migrations, nil
}

func validateDeploymentMigrations(migrations []domain.DeploymentMigration) error {
	if len(migrations) > maximumDeploymentMigrations {
		return errors.New("the target release lists too many deployment migrations")
	}
	seen := map[string]bool{}
	for _, migration := range migrations {
		switch {
		case !migrationIDPattern.MatchString(migration.ID) || seen[migration.ID]:
			return fmt.Errorf("deployment migration %q has an invalid or repeated id", migration.ID)
		case !validMigrationFile(migration.File) || migration.File == domain.DeploymentMigrationsFile:
			return fmt.Errorf("deployment migration %s names a file it may not change", migration.ID)
		case migration.Description == "" || len(migration.Description) > 512:
			return fmt.Errorf("deployment migration %s needs a short description", migration.ID)
		case migration.From == "" || migration.From == migration.To ||
			len(migration.From) > maximumMigrationText || len(migration.To) > maximumMigrationText:
			return fmt.Errorf("deployment migration %s has an invalid replacement", migration.ID)
		}
		seen[migration.ID] = true
	}
	return nil
}

// validMigrationFile accepts a plain relative path inside the deployment
// outside the protected files, keys and hidden directories.
func validMigrationFile(name string) bool {
	if name == "" || len(name) > 256 || filepath.IsAbs(name) || filepath.Clean(name) != name || strings.ContainsAny(name, "\\\x00\n") {
		return false
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || part == ".." || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return !protectedMigrationFiles[name] && parts[0] != "keys" && parts[0] != "admin-ssh"
}

// planDeploymentMigrations works out each migration on the committed
// deployment (the update needs a clean worktree, so it is HEAD). Migrations
// in the deployment's record are skipped; several migrations of one file
// apply in order to the same content. Applied and already done migrations
// join the record, which is saved with them.
func planDeploymentMigrations(ctx context.Context, repository string, migrations []domain.DeploymentMigration) ([]domain.DeploymentMigrationResult, []domain.MigratedFile, error) {
	record, recordFile, err := readMigrationRecord(ctx, repository)
	if err != nil {
		return nil, nil, err
	}
	recorded := map[string]bool{}
	for _, id := range record.Applied {
		recorded[id] = true
	}
	results := make([]domain.DeploymentMigrationResult, 0, len(migrations))
	files := map[string]*domain.MigratedFile{}
	order := []string{}
	for _, migration := range migrations {
		if recorded[migration.ID] {
			continue
		}
		file := files[migration.File]
		if file == nil {
			content, mode, err := readMigrationFile(ctx, repository, migration.File)
			if err != nil {
				results = append(results, domain.DeploymentMigrationResult{Migration: migration, State: domain.MigrationManual, Detail: err.Error()})
				continue
			}
			file = &domain.MigratedFile{Path: migration.File, Before: content, After: content, Mode: mode}
			files[migration.File] = file
			order = append(order, migration.File)
		}
		current := string(file.After)
		switch count := strings.Count(current, migration.From); {
		case count == 1:
			file.After = []byte(strings.Replace(current, migration.From, migration.To, 1))
			results = append(results, domain.DeploymentMigrationResult{Migration: migration, State: domain.MigrationApplied})
			record.Applied = append(record.Applied, migration.ID)
		case count == 0 && migration.To != "" && strings.Contains(current, migration.To):
			results = append(results, domain.DeploymentMigrationResult{Migration: migration, State: domain.MigrationDone})
			record.Applied = append(record.Applied, migration.ID)
		case count == 0:
			results = append(results, domain.DeploymentMigrationResult{Migration: migration, State: domain.MigrationManual,
				Detail: "the file was changed and does not contain the text this release replaces"})
		default:
			results = append(results, domain.DeploymentMigrationResult{Migration: migration, State: domain.MigrationManual,
				Detail: fmt.Sprintf("the text this release replaces appears %d times", count)})
		}
	}
	changed := []domain.MigratedFile{}
	for _, path := range order {
		if file := files[path]; string(file.Before) != string(file.After) {
			changed = append(changed, *file)
		}
	}
	if len(record.Applied) > len(recorded) {
		content, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		recordFile.After = append(content, '\n')
		changed = append(changed, recordFile)
	}
	return results, changed, nil
}

type migrationRecord struct {
	SchemaVersion int      `json:"schemaVersion"`
	Applied       []string `json:"applied"`
}

// readMigrationRecord reads the deployment's record of migrations, or an
// empty one to create when the deployment has none yet.
func readMigrationRecord(ctx context.Context, repository string) (migrationRecord, domain.MigratedFile, error) {
	empty := migrationRecord{SchemaVersion: 1, Applied: []string{}}
	path := filepath.Join(repository, domain.DeploymentMigrationsFile)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return empty, domain.MigratedFile{Path: domain.DeploymentMigrationsFile, Mode: 0o644, Create: true}, nil
	}
	content, mode, err := readMigrationFile(ctx, repository, domain.DeploymentMigrationsFile)
	if err != nil {
		return empty, domain.MigratedFile{}, fmt.Errorf("%s: %w", domain.DeploymentMigrationsFile, err)
	}
	var record migrationRecord
	if err := json.Unmarshal(content, &record); err != nil || record.SchemaVersion != 1 || record.Applied == nil {
		return empty, domain.MigratedFile{}, fmt.Errorf("%s is not a valid migration record", domain.DeploymentMigrationsFile)
	}
	for _, id := range record.Applied {
		if !migrationIDPattern.MatchString(id) {
			return empty, domain.MigratedFile{}, fmt.Errorf("%s names an invalid migration", domain.DeploymentMigrationsFile)
		}
	}
	return record, domain.MigratedFile{Path: domain.DeploymentMigrationsFile, Before: content, Mode: mode}, nil
}

// readMigrationFile reads a tracked regular file of the deployment.
func readMigrationFile(ctx context.Context, repository, name string) ([]byte, uint32, error) {
	tracked, err := runOutput(ctx, "git", "-C", repository, "ls-files", "--error-unmatch", "--", name)
	if err != nil || strings.TrimSpace(tracked) != name {
		return nil, 0, errors.New("the file is not part of the saved deployment")
	}
	path := filepath.Join(repository, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, 0, errors.New("the file is missing or is not a regular file")
	}
	if info.Size() > maximumMigratedFile {
		return nil, 0, errors.New("the file is too large to migrate")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, errors.New("the file cannot be read")
	}
	return content, uint32(info.Mode().Perm()), nil
}
