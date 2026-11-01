package deploymigrate

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// files holds the frozen new content of whole-file migrations, one directory
// per migration id. A released copy is never edited: a later template change
// needs a new migration with its own directory.
//
//go:embed files
var files embed.FS

// WholeFile is one template file a migration replaces as a whole.
type WholeFile struct {
	Path     string   // in the deployment
	Released []string // SHA-256 of every released form this migration replaces
	Content  []byte   // the new form
	// Later lists the SHA-256 of forms a later migration or template made
	// from the new one: such a file already has this change. Without it, a
	// laboratory created after the later change would read as changed by
	// hand at every update.
	Later []string
}

// ReplaceWholeFiles replaces template files the administrator never changed,
// all together or not at all: applied when each file is a released form or
// already the new one (or a later form), done when all are already new or
// later, manual (nothing touched) when any file was changed by hand or is
// missing.
func ReplaceWholeFiles(tree Tree, replacements []WholeFile) Outcome {
	// What is found is kept: a failed write restores it, and each file keeps
	// the mode it had.
	type original struct {
		file    WholeFile
		content []byte
		mode    fs.FileMode
	}
	var edited []string
	var pending []original
	for _, file := range replacements {
		content, mode, found, err := tree.Read(file.Path)
		if err != nil || !found {
			edited = append(edited, file.Path)
			continue
		}
		if string(content) == string(file.Content) {
			continue
		}
		digest := sha256.Sum256(content)
		sum := hex.EncodeToString(digest[:])
		if slices.Contains(file.Later, sum) {
			continue
		}
		if !slices.Contains(file.Released, sum) {
			edited = append(edited, file.Path)
			continue
		}
		pending = append(pending, original{file: file, content: content, mode: mode})
	}
	if len(edited) > 0 {
		return Outcome{State: Manual, Detail: fmt.Sprintf("%s changed by hand or missing. Copy the new versions of %s from templates/site in this Nixorium release.", strings.Join(edited, ", "), wholeFileNames(replacements))}
	}
	if len(pending) == 0 {
		return Outcome{State: Done}
	}
	paths := make([]string, 0, len(pending))
	for index, item := range pending {
		paths = append(paths, item.file.Path)
		if err := tree.Write(item.file.Path, item.file.Content, item.mode); err != nil {
			// The failed file may be half written: restore it too.
			var stuck []string
			for _, written := range pending[:index+1] {
				if tree.Write(written.file.Path, written.content, written.mode) != nil {
					stuck = append(stuck, written.file.Path)
				}
			}
			if len(stuck) > 0 {
				return Outcome{State: Manual, Detail: fmt.Sprintf("%s cannot be written and %s could not be restored. Restore %s from Git.", item.file.Path, strings.Join(stuck, ", "), strings.Join(stuck, ", "))}
			}
			return Outcome{State: Manual, Detail: item.file.Path + " cannot be written. Nothing was changed."}
		}
	}
	return Outcome{State: Applied, Files: paths}
}

func wholeFileNames(replacements []WholeFile) string {
	names := make([]string, 0, len(replacements))
	for _, file := range replacements {
		names = append(names, file.Path)
	}
	return strings.Join(names, ", ")
}

// frozen reads a migration's frozen file; a missing one is a build mistake
// the unit tests catch.
func frozen(id, name string) []byte {
	content, err := files.ReadFile("files/" + id + "/" + name)
	if err != nil {
		panic(err)
	}
	return content
}
