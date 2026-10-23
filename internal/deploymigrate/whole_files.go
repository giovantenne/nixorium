package deploymigrate

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
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
}

// ReplaceWholeFiles replaces template files the administrator never changed,
// all together or not at all: applied when each file is a released form or
// already the new one, done when all are already new, manual (nothing
// touched) when any file was changed by hand or is missing.
func ReplaceWholeFiles(tree Tree, replacements []WholeFile) Outcome {
	var edited, pending []string
	for _, file := range replacements {
		content, _, found, err := tree.Read(file.Path)
		if err != nil || !found {
			edited = append(edited, file.Path)
			continue
		}
		if string(content) == string(file.Content) {
			continue
		}
		sum := sha256.Sum256(content)
		if !slices.Contains(file.Released, hex.EncodeToString(sum[:])) {
			edited = append(edited, file.Path)
			continue
		}
		pending = append(pending, file.Path)
	}
	if len(edited) > 0 {
		return Outcome{State: Manual, Detail: fmt.Sprintf("%s changed by hand or missing. Copy the new versions of %s from templates/site in this Nixorium release.", strings.Join(edited, ", "), wholeFileNames(replacements))}
	}
	if len(pending) == 0 {
		return Outcome{State: Done}
	}
	for _, file := range replacements {
		if !slices.Contains(pending, file.Path) {
			continue
		}
		_, mode, _, _ := tree.Read(file.Path)
		if err := tree.Write(file.Path, file.Content, mode); err != nil {
			return Outcome{State: Manual, Detail: file.Path + " cannot be written"}
		}
	}
	return Outcome{State: Applied, Files: pending}
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
