package deploymigrate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Compare only the module and script trees covered by the compatibility check,
// including nested files. The scripts directory may disappear after the last
// script moves into core; modules remains required. Other read failures and
// non-regular entries are fixture errors, not evidence that a tree is empty.
func migrationTemplateFiles(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, directory := range []string{"modules", "scripts"} {
		path := filepath.Join(root, directory)
		info, err := os.Lstat(path)
		if directory == "scripts" && errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", path)
		}
		if err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("%s is not a regular file", path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(relative)] = content
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func compareMigratedTemplate(migratedRoot, currentRoot string) error {
	migrated, err := migrationTemplateFiles(migratedRoot)
	if err != nil {
		return fmt.Errorf("read migrated template: %w", err)
	}
	current, err := migrationTemplateFiles(currentRoot)
	if err != nil {
		return fmt.Errorf("read current template: %w", err)
	}
	names := map[string]bool{}
	for name := range migrated {
		names[name] = true
	}
	for name := range current {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	var differences []error
	for _, name := range ordered {
		if _, allowed := releasedDrift[name]; allowed {
			continue
		}
		before, beforeFound := migrated[name]
		after, afterFound := current[name]
		if !beforeFound || !afterFound || !bytes.Equal(before, after) {
			differences = append(differences, fmt.Errorf("%s differs from this release's template after migrating: add or widen a migration", name))
		}
	}
	return errors.Join(differences...)
}

func TestMigrationTemplateComparison(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		change     func(string) error
		diagnostic string
	}{
		{"identical nested files", func(string) error { return nil }, ""},
		{"changed nested file", func(root string) error {
			return os.WriteFile(filepath.Join(root, "modules", "nested", "example.nix"), []byte("changed"), 0600)
		}, "modules/nested/example.nix differs"},
		{"missing nested file", func(root string) error {
			return os.Remove(filepath.Join(root, "modules", "nested", "example.nix"))
		}, "modules/nested/example.nix differs"},
		{"extra empty file", func(root string) error {
			return os.WriteFile(filepath.Join(root, "scripts", "extra.sh"), nil, 0600)
		}, "scripts/extra.sh differs"},
		{"absent empty scripts directory", func(root string) error {
			return os.Remove(filepath.Join(root, "scripts"))
		}, ""},
		{"missing modules directory", func(root string) error {
			return os.RemoveAll(filepath.Join(root, "modules"))
		}, "modules"},
		{"directory replaced by a file", func(root string) error {
			if err := os.Remove(filepath.Join(root, "scripts")); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(root, "scripts"), nil, 0600)
		}, "not a directory"},
		{"symlink", func(root string) error {
			return os.Symlink("../modules/nested/example.nix", filepath.Join(root, "scripts", "link"))
		}, "not a regular file"},
		{"documented drift", func(root string) error {
			return os.WriteFile(filepath.Join(root, "modules", "development.nix"), []byte("kept local web server"), 0600)
		}, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			makeTemplate := func() string {
				root := t.TempDir()
				for _, directory := range []string{"modules/nested", "scripts"} {
					if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(root, "modules", "nested", "example.nix"), []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
				return root
			}
			left, right := makeTemplate(), makeTemplate()
			if err := scenario.change(left); err != nil {
				t.Fatal(err)
			}
			// Both a damaged migrated tree and a damaged current template must
			// fail; an absent empty file must not compare equal to an empty file.
			for _, pair := range [][2]string{{left, right}, {right, left}} {
				err := compareMigratedTemplate(pair[0], pair[1])
				if scenario.diagnostic == "" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), scenario.diagnostic) {
					t.Fatalf("expected %q, got %v", scenario.diagnostic, err)
				}
			}
		})
	}
}
