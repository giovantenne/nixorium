// Package deploymigrate holds the changes a release makes to a deployment's
// own files (ADR 0034). The release being installed runs them, through
// `nixorium deployment migrate`, on a copy of the deployment; the release
// that runs Update Nixorium only compares the copy with the original, shows
// the difference in the update review and saves it. Its contract with this
// package is the file difference, a JSON report and the exit status, so a
// release can migrate in ways older releases do not know.
package deploymigrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// ContractVersion is the version of the contract between the release that
// runs Update Nixorium and this migrator. A migrator accepts the versions in
// SupportedContracts; it keeps accepting older ones for at least a year.
const ContractVersion = 1

var SupportedContracts = []int{1}

// RecordFile lists, in the deployment, the migrations already applied or
// found done, so each runs once.
const RecordFile = "nixorium-migrations.json"

// Outcomes of a migration.
const (
	Applied = "applied" // the migration changed the files it lists
	Done    = "done"    // the deployment already has the change
	Manual  = "manual"  // the files were changed by hand; nothing touched
)

var idPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[a-z0-9-]{1,64}$`)

// Migration is one change. Released migrations are never edited or removed:
// their ids are recorded in deployments.
type Migration struct {
	ID          string
	Description string
	Apply       func(Tree) Outcome
}

// Outcome is what a migration did. Files lists every file an applied
// migration changed, created or removed.
type Outcome struct {
	State  string
	Detail string
	Files  []string
}

// Entry is one migration in the report.
type Entry struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Detail      string   `json:"detail,omitempty"`
	Files       []string `json:"files,omitempty"`
}

// Report is printed on standard output for the release that runs the update.
type Report struct {
	ContractVersion int     `json:"contractVersion"`
	TargetVersion   string  `json:"targetVersion"`
	Migrations      []Entry `json:"migrations"`
}

type record struct {
	SchemaVersion int      `json:"schemaVersion"`
	Applied       []string `json:"applied"`
}

// ErrRefused means this migrator cannot work on the deployment (for example
// an unreadable record); the update is refused with the reason.
var ErrRefused = errors.New("the deployment cannot be migrated")

// Run applies the registry, in order, to the deployment at root and updates
// its record. Recorded migrations are skipped; unknown recorded ids (from a
// newer release) are kept.
func Run(root, version string, registry []Migration) (Report, error) {
	report := Report{ContractVersion: ContractVersion, TargetVersion: version, Migrations: []Entry{}}
	tree := Tree{Root: root}
	current, err := readRecord(tree)
	if err != nil {
		return report, err
	}
	recorded := map[string]bool{}
	for _, id := range current.Applied {
		recorded[id] = true
	}
	changed := false
	for _, migration := range registry {
		if !idPattern.MatchString(migration.ID) {
			return report, fmt.Errorf("migration %q has an invalid id", migration.ID)
		}
		if recorded[migration.ID] {
			continue
		}
		outcome := migration.Apply(tree)
		switch outcome.State {
		case Applied, Done:
			current.Applied = append(current.Applied, migration.ID)
			recorded[migration.ID] = true
			changed = true
		case Manual:
		default:
			return report, fmt.Errorf("migration %s returned an unknown outcome", migration.ID)
		}
		report.Migrations = append(report.Migrations, Entry{
			ID: migration.ID, Description: migration.Description,
			State: outcome.State, Detail: outcome.Detail, Files: outcome.Files,
		})
	}
	if changed {
		content, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			return report, err
		}
		if err := tree.Write(RecordFile, append(content, '\n'), 0o644); err != nil {
			return report, err
		}
	}
	return report, nil
}

func readRecord(tree Tree) (record, error) {
	empty := record{SchemaVersion: 1, Applied: []string{}}
	content, _, found, err := tree.Read(RecordFile)
	if err != nil {
		return empty, fmt.Errorf("%w: %s cannot be read", ErrRefused, RecordFile)
	}
	if !found {
		return empty, nil
	}
	var current record
	if json.Unmarshal(content, &current) != nil || current.SchemaVersion != 1 || current.Applied == nil {
		return empty, fmt.Errorf("%w: %s is not a valid migration record", ErrRefused, RecordFile)
	}
	for _, id := range current.Applied {
		if !idPattern.MatchString(id) {
			return empty, fmt.Errorf("%w: %s names an invalid migration", ErrRefused, RecordFile)
		}
	}
	return current, nil
}

// Tree is the deployment copy a migration works on. Paths are relative,
// clean and inside it; symbolic links are never followed.
type Tree struct{ Root string }

func (tree Tree) path(name string) (string, error) {
	if name == "" || filepath.IsAbs(name) || filepath.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") || name == ".." {
		return "", fmt.Errorf("invalid deployment path %q", name)
	}
	directory := tree.Root
	parts := strings.Split(name, "/")
	for _, part := range parts[:len(parts)-1] {
		directory = filepath.Join(directory, part)
		if info, err := os.Lstat(directory); err == nil && !info.IsDir() {
			return "", fmt.Errorf("deployment path %q crosses a file or link", name)
		}
	}
	return filepath.Join(tree.Root, name), nil
}

// Read returns a regular file's content and mode; found is false when it does
// not exist.
func (tree Tree) Read(name string) (content []byte, mode fs.FileMode, found bool, err error) {
	path, err := tree.path(name)
	if err != nil {
		return nil, 0, false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("%s is not a regular file", name)
	}
	content, err = os.ReadFile(path)
	return content, info.Mode().Perm(), err == nil, err
}

// Write creates or replaces a regular file.
func (tree Tree) Write(name string, content []byte, mode fs.FileMode) error {
	path, err := tree.path(name)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// Remove deletes a regular file.
func (tree Tree) Remove(name string) error {
	path, err := tree.path(name)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", name)
	}
	return os.Remove(path)
}

// ReplaceExact replaces one exact, known text of file: applied when it is
// there exactly once, done when the new text is already there, manual
// otherwise (the file was changed by hand).
func ReplaceExact(tree Tree, file, from, to string) Outcome {
	content, mode, found, err := tree.Read(file)
	if err != nil || !found {
		return Outcome{State: Manual, Detail: file + " is missing or cannot be read"}
	}
	text := string(content)
	switch count := strings.Count(text, from); {
	case count == 1:
		if err := tree.Write(file, []byte(strings.Replace(text, from, to, 1)), mode); err != nil {
			return Outcome{State: Manual, Detail: file + " cannot be written"}
		}
		return Outcome{State: Applied, Files: []string{file}}
	case count == 0 && to != "" && strings.Contains(text, to):
		return Outcome{State: Done}
	default:
		return Outcome{State: Manual, Detail: manualReplacement(file, from, to)}
	}
}

// SoftwareFile is the deployment's software declaration.
const SoftwareFile = "lab-software.json"

// AddSharedPackage declares a package for the controller and all clients:
// applied when the deployment does not name it, done when it names it with
// any scope (an administrator's choice is kept), manual when the software
// declaration cannot be read.
func AddSharedPackage(tree Tree, name string) Outcome {
	content, mode, found, err := tree.Read(SoftwareFile)
	if err != nil || !found {
		return Outcome{State: Manual, Detail: fmt.Sprintf("%s is missing or cannot be read. Add %s in Software for this controller and all clients.", SoftwareFile, name)}
	}
	software, err := domain.DecodeLabSoftware(content)
	if err != nil {
		return Outcome{State: Manual, Detail: fmt.Sprintf("%s is not valid. Correct it, then add %s in Software for this controller and all clients.", SoftwareFile, name)}
	}
	for _, entry := range software.Packages {
		if entry.Package == name {
			return Outcome{State: Done}
		}
	}
	software.Packages = append(software.Packages, domain.SoftwareDeclaration{Package: name, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeShared}})
	updated, err := domain.MarshalLabSoftware(software)
	if err != nil {
		return Outcome{State: Manual, Detail: fmt.Sprintf("%s cannot take %s. Add it in Software for this controller and all clients.", SoftwareFile, name)}
	}
	if err := tree.Write(SoftwareFile, updated, mode); err != nil {
		return Outcome{State: Manual, Detail: SoftwareFile + " cannot be written"}
	}
	return Outcome{State: Applied, Files: []string{SoftwareFile}}
}

func manualReplacement(file, from, to string) string {
	return fmt.Sprintf("%s was changed by hand. Replace %q with %q.", file, strings.TrimRight(from, "\n"), strings.TrimRight(to, "\n"))
}

// SupportsContract reports whether this migrator speaks the given version.
func SupportsContract(version int) bool {
	return slices.Contains(SupportedContracts, version)
}
