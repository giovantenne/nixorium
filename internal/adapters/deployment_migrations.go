package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Deployment migrations (ADR 0034). The release being installed carries its
// own migrator (`nixorium deployment migrate`); this release runs it in a
// sandbox on a copy of the saved deployment, compares the copy with the
// original under fixed rules, and shows the difference in the update review.
// This file is the stable side of that contract: it never interprets what a
// migration does, only which files changed and how.

const (
	migrationContract       = 1
	migrationContractMarker = "share/nixorium/deployment-migrate-contract"
	maximumMigrationReport  = 1024 * 1024
	maximumMigrationEntries = 64
	maximumMigratedFile     = 1024 * 1024
	maximumMigratedFiles    = 256
	maximumMigratedBytes    = 16 * 1024 * 1024
	migratorTimeout         = 120 * time.Second
)

var (
	migrationIDPattern       = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[a-z0-9-]{1,64}$`)
	migrationCandidatePrefix = regexp.MustCompile(`^path:/nix/store/[0-9a-z]{32}-[A-Za-z0-9+._?=-]+$`)
)

// Files a migration never touches: the update input itself (Update Nixorium
// changes it on its own), keys and credentials.
var protectedMigrationFiles = map[string]bool{
	"flake.nix": true, "flake.lock": true, "lab-credentials.json": true, "secret-key": true,
}

// validMigrationFile accepts a plain relative path inside the deployment
// outside the protected files, keys and hidden paths.
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

// buildTargetMigrator builds the Nixorium package of the release the
// candidate pins and returns it, or "" when that release predates
// deployment migrations. Only the release's own package is evaluated, not
// the deployment's systems. Tests replace it.
var buildTargetMigrator = func(ctx context.Context, candidate string) (string, error) {
	if !migrationCandidatePrefix.MatchString(candidate) {
		return "", errors.New("deployment migrations need a prepared candidate source")
	}
	expression := `(builtins.getFlake "` + candidate + `").inputs.nixorium.packages.${builtins.currentSystem}.nixorium`
	output, err := runBoundedNix(ctx, 64*1024, "build", "--no-link", "--print-out-paths", "--impure", "--expr", expression)
	if err != nil {
		return "", fmt.Errorf("build the target release's Nixorium for its deployment migrations: %w", err)
	}
	path := strings.TrimSpace(output)
	if !domain.ValidStorePath(path) {
		return "", errors.New("the target release's Nixorium package is not a store path")
	}
	marker, err := os.ReadFile(filepath.Join(path, migrationContractMarker))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !strings.Contains("\n"+string(marker)+"\n", fmt.Sprintf("\n%d\n", migrationContract)) {
		return "", errors.New("the target release migrates deployments with a newer contract; update to an intermediate release first")
	}
	if info, err := os.Stat(filepath.Join(path, "bin", "nixorium")); err != nil || info.IsDir() {
		return "", errors.New("the target release's Nixorium package has no nixorium command")
	}
	return path, nil
}

// runMigrator runs the target's migrator on the copy in a sandbox without
// network, environment or access outside the copy and the store. Tests
// replace it.
var runMigrator = func(ctx context.Context, migrator, directory string) ([]byte, error) {
	sandbox := os.Getenv("NIXORIUM_NPM_BWRAP")
	if sandbox == "" {
		var err error
		if sandbox, err = exec.LookPath("bwrap"); err != nil {
			return nil, errors.New("bubblewrap is needed to run deployment migrations")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, migratorTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, sandbox,
		"--unshare-all", "--die-with-parent", "--new-session", "--clearenv",
		"--ro-bind", "/nix/store", "/nix/store",
		"--bind", directory, "/work",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
		"--chdir", "/work",
		"--", filepath.Join(migrator, "bin", "nixorium"),
		"deployment", "migrate", "--root", "/work", "--contract", fmt.Sprint(migrationContract), "--json")
	configureCommandCancellation(command)
	stdout := &boundedCommandBuffer{limit: maximumMigrationReport}
	stderr := &boundedCommandBuffer{limit: 64 * 1024}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	detail := strings.TrimSpace(string(sanitizeOperationLog(stderr.buffer.Bytes())))
	var exit *exec.ExitError
	switch {
	case err == nil && stdout.truncated:
		return nil, errors.New("the target release's migration report is too large")
	case err == nil:
		return stdout.buffer.Bytes(), nil
	case ctx.Err() != nil:
		return nil, errors.New("the target release's deployment migrations did not finish in time")
	case errors.As(err, &exit) && exit.ExitCode() == 2:
		return nil, fmt.Errorf("the target release cannot migrate deployments for this release; update to an intermediate release first (%s)", detail)
	case errors.As(err, &exit) && exit.ExitCode() == 3:
		return nil, fmt.Errorf("the target release refuses to migrate this deployment: %s", detail)
	default:
		return nil, fmt.Errorf("the target release's deployment migrations failed: %s", detail)
	}
}

type migrationReport struct {
	ContractVersion int `json:"contractVersion"`
	Migrations      []struct {
		ID          string   `json:"id"`
		Description string   `json:"description"`
		State       string   `json:"state"`
		Detail      string   `json:"detail"`
		Files       []string `json:"files"`
	} `json:"migrations"`
}

// planDeploymentMigrations runs the target release's migrations when the
// update changes the Nixorium revision. It returns the outcomes, the changed
// files, and the raw report and migrator that the review token binds.
func planDeploymentMigrations(ctx context.Context, repository, candidate string, revisionChanges bool) ([]domain.DeploymentMigrationResult, []domain.MigratedFile, []byte, string, error) {
	if !revisionChanges {
		return nil, nil, nil, "", nil
	}
	migrator, err := buildTargetMigrator(ctx, candidate)
	if err != nil || migrator == "" {
		return nil, nil, nil, "", err
	}
	revision, err := runOutput(ctx, "git", "-C", repository, "rev-parse", "--verify", "HEAD^{commit}")
	revision = strings.TrimSpace(revision)
	if err != nil || !validGitRevision(revision) {
		return nil, nil, nil, "", errors.New("deployment migrations need a committed deployment revision")
	}
	original, err := os.MkdirTemp("", "nixorium-migration-original-*")
	if err != nil {
		return nil, nil, nil, "", err
	}
	defer os.RemoveAll(original)
	work, err := os.MkdirTemp("", "nixorium-migration-work-*")
	if err != nil {
		return nil, nil, nil, "", err
	}
	defer os.RemoveAll(work)
	for _, directory := range []string{original, work} {
		if err := extractCommittedTree(ctx, repository, revision, directory); err != nil {
			return nil, nil, nil, "", fmt.Errorf("copy the saved deployment for its migrations: %w", err)
		}
	}
	raw, err := runMigrator(ctx, migrator, work)
	if err != nil {
		return nil, nil, nil, "", err
	}
	results, declared, err := parseMigrationReport(raw)
	if err != nil {
		return nil, nil, nil, "", err
	}
	files, err := diffMigratedTrees(original, work)
	if err != nil {
		return nil, nil, nil, "", err
	}
	// A changed file keeps the permissions it has in the deployment.
	for index, file := range files {
		if info, err := os.Lstat(filepath.Join(repository, file.Path)); err == nil && !file.Create && !file.Delete {
			mode := info.Mode().Perm()
			if file.Mode == 0o755 {
				mode |= 0o111
			} else {
				mode &^= 0o111
			}
			files[index].Mode = uint32(mode)
		}
	}
	// The difference is the truth; the report only names it.
	changed := map[string]bool{}
	for _, file := range files {
		changed[file.Path] = true
		if file.Path != domain.DeploymentMigrationsFile && !declared[file.Path] {
			return nil, nil, nil, "", fmt.Errorf("the target release changed %s without declaring it in its migrations", file.Path)
		}
	}
	for path := range declared {
		if !changed[path] {
			return nil, nil, nil, "", fmt.Errorf("the target release declared a change to %s that it did not make", path)
		}
	}
	for _, file := range files {
		if file.Path == domain.DeploymentMigrationsFile {
			if err := checkMigrationRecord(file, results); err != nil {
				return nil, nil, nil, "", err
			}
		}
	}
	return results, files, raw, migrator, nil
}

// checkMigrationRecord accepts a new record only if it keeps every id the
// deployment had and adds just the migrations reported applied or done.
func checkMigrationRecord(file domain.MigratedFile, results []domain.DeploymentMigrationResult) error {
	type record struct {
		SchemaVersion int      `json:"schemaVersion"`
		Applied       []string `json:"applied"`
	}
	var before, after record
	if !file.Create && (json.Unmarshal(file.Before, &before) != nil || before.SchemaVersion != 1) {
		return fmt.Errorf("%s is not a valid migration record; restore it from Git", domain.DeploymentMigrationsFile)
	}
	if json.Unmarshal(file.After, &after) != nil || after.SchemaVersion != 1 || after.Applied == nil {
		return errors.New("the target release wrote an invalid migration record")
	}
	kept := map[string]bool{}
	for _, id := range after.Applied {
		if !migrationIDPattern.MatchString(id) || kept[id] {
			return errors.New("the target release wrote an invalid migration record")
		}
		kept[id] = true
	}
	for _, id := range before.Applied {
		if !kept[id] {
			return fmt.Errorf("the target release removed %s from the migration record", id)
		}
		delete(kept, id)
	}
	reported := map[string]bool{}
	for _, result := range results {
		if result.State == domain.MigrationApplied || result.State == domain.MigrationDone {
			reported[result.ID] = true
		}
	}
	for id := range kept {
		if !reported[id] {
			return fmt.Errorf("the target release recorded %s without reporting it", id)
		}
	}
	return nil
}

// parseMigrationReport checks the report and returns the outcomes and the
// files the applied migrations declare.
func parseMigrationReport(raw []byte) ([]domain.DeploymentMigrationResult, map[string]bool, error) {
	var report migrationReport
	if err := json.Unmarshal(raw, &report); err != nil || report.ContractVersion != migrationContract || report.Migrations == nil {
		return nil, nil, errors.New("the target release returned an invalid migration report")
	}
	if len(report.Migrations) > maximumMigrationEntries {
		return nil, nil, errors.New("the target release returned too many migrations")
	}
	results := []domain.DeploymentMigrationResult{}
	declared := map[string]bool{}
	seen := map[string]bool{}
	for _, entry := range report.Migrations {
		if !migrationIDPattern.MatchString(entry.ID) || seen[entry.ID] || entry.Description == "" || len(entry.Description) > 512 || len(entry.Detail) > 2048 {
			return nil, nil, errors.New("the target release returned an invalid migration report")
		}
		seen[entry.ID] = true
		switch entry.State {
		case domain.MigrationApplied:
			if len(entry.Files) == 0 {
				return nil, nil, fmt.Errorf("migration %s changed no declared file", entry.ID)
			}
			for _, file := range entry.Files {
				if !validMigrationFile(file) {
					return nil, nil, fmt.Errorf("migration %s names a file it may not change", entry.ID)
				}
				declared[file] = true
			}
		case domain.MigrationDone, domain.MigrationManual:
			if len(entry.Files) > 0 {
				return nil, nil, fmt.Errorf("migration %s lists files although it changed none", entry.ID)
			}
		case "skipped":
			continue
		default:
			return nil, nil, fmt.Errorf("migration %s has an unknown outcome", entry.ID)
		}
		results = append(results, domain.DeploymentMigrationResult{ID: entry.ID, Description: entry.Description, State: entry.State, Detail: entry.Detail, Files: entry.Files})
	}
	return results, declared, nil
}

type treeEntry struct {
	content []byte
	mode    fs.FileMode
	regular bool
	// large: over the size a migration may change; content is a size and
	// time fingerprint, not the file.
	large bool
}

func (entry treeEntry) executable() bool { return entry.mode&0o100 != 0 }

// diffMigratedTrees compares the migrated copy with the original under the
// fixed rules: only regular files, allowed paths, modes 0644 or 0755, and
// size limits. Anything else refuses the update.
func diffMigratedTrees(original, migrated string) ([]domain.MigratedFile, error) {
	before, err := readMigrationTree(original)
	if err != nil {
		return nil, err
	}
	after, err := readMigrationTree(migrated)
	if err != nil {
		return nil, err
	}
	paths := map[string]bool{}
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	files := []domain.MigratedFile{}
	total := 0
	for _, path := range sortedKeys(paths) {
		old, hadOld := before[path]
		updated, hasNew := after[path]
		// The copies are private (0600); only the executable bit is part of
		// the deployment's content.
		if hadOld && hasNew && old.regular == updated.regular && old.executable() == updated.executable() && bytes.Equal(old.content, updated.content) {
			continue
		}
		allowed := validMigrationFile(path) || path == domain.DeploymentMigrationsFile
		switch {
		case !allowed:
			return nil, fmt.Errorf("the target release's migrations changed %s, which they may not change", path)
		case (hadOld && !old.regular) || (hasNew && !updated.regular):
			return nil, fmt.Errorf("the target release's migrations changed %s, which is not a regular file", path)
		case !hasNew && path == domain.DeploymentMigrationsFile:
			return nil, errors.New("the target release's migrations removed the migration record")
		case (hadOld && old.large) || (hasNew && (updated.large || len(updated.content) > maximumMigratedFile)):
			return nil, fmt.Errorf("the target release's migrations changed %s, which is too large", path)
		}
		file := domain.MigratedFile{Path: path, Create: !hadOld, Delete: !hasNew, Mode: 0o644}
		if hadOld {
			file.Before = old.content
		}
		if hasNew {
			file.After = updated.content
			if updated.executable() {
				file.Mode = 0o755
			}
			total += len(updated.content)
		}
		files = append(files, file)
	}
	if len(files) > maximumMigratedFiles || total > maximumMigratedBytes {
		return nil, errors.New("the target release's migrations change too much of the deployment")
	}
	return files, nil
}

func readMigrationTree(root string) (map[string]treeEntry, error) {
	entries := map[string]treeEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := treeEntry{mode: info.Mode().Perm(), regular: info.Mode().IsRegular()}
		if item.regular {
			// Larger files are kept by size only: the diff rule refuses a
			// change to them without reading them whole.
			if info.Size() > maximumMigratedFile {
				item.large = true
				item.content = []byte(fmt.Sprintf("%d %d", info.Size(), info.ModTime().UnixNano()))
				entries[relative] = item
				return nil
			}
			if item.content, err = os.ReadFile(path); err != nil {
				return err
			}
		} else if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			item.content = []byte(target)
		}
		entries[relative] = item
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the deployment copy: %w", err)
	}
	return entries, nil
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// lockedNixoriumRevision is the Nixorium revision a flake.lock pins, or "".
func lockedNixoriumRevision(lock []byte) string {
	var document flakeLockDocument
	if json.Unmarshal(lock, &document) != nil {
		return ""
	}
	root, ok := document.Nodes[document.Root]
	if !ok {
		return ""
	}
	input, _ := root.Inputs["nixorium"].(string)
	return document.Nodes[input].Locked.Rev
}
