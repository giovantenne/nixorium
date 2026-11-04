package deploymigrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// RemovedFile is one template file a migration deletes.
type RemovedFile struct {
	Path     string   // in the deployment
	Released []string // SHA-256 of every form this migration deletes
	// Optional files are deleted only in a released form: a changed one is
	// kept and does not make the migration manual (an unused leftover).
	Optional bool
}

// RemoveFiles deletes template files and the one exact text of file that uses
// them (for example a module import), all together or not at all: applied
// when the text is there once and each required file is a released form or
// already gone, done when the text and the required files are gone, manual
// (nothing touched) otherwise.
func RemoveFiles(tree Tree, file, use string, removed []RemovedFile) Outcome {
	content, mode, found, err := tree.Read(file)
	if err != nil || !found {
		return Outcome{State: Manual, Detail: file + " is missing or cannot be read"}
	}
	uses := strings.Count(string(content), use)
	type original struct {
		path    string
		content []byte
		mode    fs.FileMode
	}
	var pending []original
	var edited, required []string
	for _, candidate := range removed {
		if !candidate.Optional {
			required = append(required, candidate.Path)
		}
		existing, existingMode, found, err := tree.Read(candidate.Path)
		if err == nil && !found {
			continue
		}
		digest := sha256.Sum256(existing)
		if err == nil && slices.Contains(candidate.Released, hex.EncodeToString(digest[:])) {
			pending = append(pending, original{path: candidate.Path, content: existing, mode: existingMode})
			continue
		}
		if !candidate.Optional {
			edited = append(edited, candidate.Path)
		}
	}
	manual := fmt.Sprintf("Remove %q from %s and delete %s.", strings.TrimSpace(use), file, strings.Join(required, ", "))
	switch {
	case len(edited) > 0:
		return Outcome{State: Manual, Detail: fmt.Sprintf("%s changed by hand. %s", strings.Join(edited, ", "), manual)}
	case uses > 1:
		return Outcome{State: Manual, Detail: fmt.Sprintf("%s was changed by hand. %s", file, manual)}
	case uses == 0 && len(pending) == 0:
		return Outcome{State: Done}
	}
	var changed []string
	if uses == 1 {
		// A file still imported elsewhere is never deleted: the text must be
		// the one use, so a deployment that removed it by hand is manual.
		if err := tree.Write(file, []byte(strings.Replace(string(content), use, "", 1)), mode); err != nil {
			return Outcome{State: Manual, Detail: file + " cannot be written. Nothing was changed."}
		}
		changed = append(changed, file)
	} else if slices.ContainsFunc(pending, func(item original) bool { return slices.Contains(required, item.path) }) {
		return Outcome{State: Manual, Detail: fmt.Sprintf("%s no longer has %q. %s", file, strings.TrimSpace(use), manual)}
	}
	for index, item := range pending {
		if err := tree.Remove(item.path); err != nil {
			var stuck []string
			if uses == 1 && tree.Write(file, content, mode) != nil {
				stuck = append(stuck, file)
			}
			for _, done := range pending[:index] {
				if tree.Write(done.path, done.content, done.mode) != nil {
					stuck = append(stuck, done.path)
				}
			}
			if len(stuck) > 0 {
				return Outcome{State: Manual, Detail: fmt.Sprintf("%s cannot be removed and %s could not be restored. Restore %s from Git.", item.path, strings.Join(stuck, ", "), strings.Join(stuck, ", "))}
			}
			return Outcome{State: Manual, Detail: item.path + " cannot be removed. Nothing was changed."}
		}
		changed = append(changed, item.path)
	}
	return Outcome{State: Applied, Files: changed}
}
