package classroomview

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

var savedTree = []FileEntry{
	{Path: "2026-10-15 10.30", Dir: true},
	{Path: "2026-10-15 10.30/pc01", Dir: true},
	{Path: "2026-10-15 10.30/pc01/essay.odt", Size: 11},
	{Path: "2026-10-15 10.30/pc01/empty.txt"},
	{Path: "pc03-2026-10-15-10.30.12.jpg", Size: 4},
}

func writeSavedTree(t *testing.T, destination string) []string {
	t.Helper()
	writer, err := NewTreeWriter(destination, savedTree)
	if err != nil {
		t.Fatal(err)
	}
	for _, piece := range []struct {
		index int
		data  string
	}{{2, "hello "}, {2, "world"}, {4, "\xff\xd8\xff\xd9"}} {
		if err := writer.Write(piece.index, []byte(piece.data)); err != nil {
			t.Fatal(err)
		}
	}
	placed, err := writer.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return placed
}

func TestTreeWriterKeepsExistingNamesAndExplicitModes(t *testing.T) {
	// The umask must not change the modes, which are never executable.
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	destination := t.TempDir()
	if placed := writeSavedTree(t, destination); strings.Join(placed, ",") != "2026-10-15 10.30,pc03-2026-10-15-10.30.12.jpg" {
		t.Fatalf("placed = %v", placed)
	}
	for name, mode := range map[string]os.FileMode{
		"2026-10-15 10.30":                os.ModeDir | 0o755,
		"2026-10-15 10.30/pc01":           os.ModeDir | 0o755,
		"2026-10-15 10.30/pc01/essay.odt": 0o644,
		"2026-10-15 10.30/pc01/empty.txt": 0o644,
		"pc03-2026-10-15-10.30.12.jpg":    0o644,
	} {
		info, err := os.Lstat(filepath.Join(destination, filepath.FromSlash(name)))
		if err != nil || info.Mode() != mode {
			t.Fatalf("%s: mode %v, %v", name, info.Mode(), err)
		}
	}
	// A second save never replaces the first: its names get " (2)".
	if placed := writeSavedTree(t, destination); strings.Join(placed, ",") != "2026-10-15 10.30 (2),pc03-2026-10-15-10.30.12 (2).jpg" {
		t.Fatalf("second placed = %v", placed)
	}
	if content, _ := os.ReadFile(filepath.Join(destination, "2026-10-15 10.30", "pc01", "essay.odt")); string(content) != "hello world" {
		t.Fatalf("the first copy changed: %q", content)
	}
	// A link already using a name is never followed or replaced.
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(destination, "pc03-2026-10-15-10.30.12 (3).jpg")); err != nil {
		t.Fatal(err)
	}
	if placed := writeSavedTree(t, destination); placed[1] != "pc03-2026-10-15-10.30.12 (4).jpg" {
		t.Fatalf("third placed = %v", placed)
	}
	if content, _ := os.ReadFile(target); string(content) != "kept" {
		t.Fatalf("a link was followed: %q", content)
	}
	entries, _ := os.ReadDir(destination)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".nixorium-incoming-") {
			t.Fatal("a staging folder was left behind")
		}
	}
}

func TestTreeWriterRefusesUnsafeOrBrokenSaves(t *testing.T) {
	destination := t.TempDir()
	deep := strings.TrimSuffix(strings.Repeat("d/", maxPathElements+1), "/")
	for name, entries := range map[string][]FileEntry{
		"nothing":   nil,
		"parent":    {{Path: "../escape", Size: 1}},
		"inner":     {{Path: "a", Dir: true}, {Path: "a/../../escape", Size: 1}},
		"absolute":  {{Path: "/tmp/escape", Size: 1}},
		"backslash": {{Path: `..\escape`, Size: 1}},
		"control":   {{Path: "a\x1bb", Size: 1}},
		"deep":      {{Path: deep, Size: 1}},
		"unordered": {{Path: "a/b", Size: 1}, {Path: "a", Dir: true}},
	} {
		if _, err := NewTreeWriter(destination, entries); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	for name, step := range map[string]func(*TreeWriter) error{
		"out of order": func(writer *TreeWriter) error { return writer.Write(4, []byte("ab")) },
		"too large":    func(writer *TreeWriter) error { return writer.Write(2, []byte("hello world!")) },
		"a folder":     func(writer *TreeWriter) error { return writer.Write(0, []byte("x")) },
		"incomplete": func(writer *TreeWriter) error {
			if err := writer.Write(2, []byte("hello")); err != nil {
				return nil
			}
			_, err := writer.Finish()
			return err
		},
	} {
		writer, err := NewTreeWriter(destination, savedTree)
		if err != nil {
			t.Fatal(err)
		}
		if err := step(writer); err == nil {
			t.Fatalf("%s was accepted", name)
		}
		writer.Abort()
	}
	if entries, _ := os.ReadDir(destination); len(entries) != 0 {
		t.Fatalf("refused saves left files: %v", entries)
	}
	if _, err := NewTreeWriter(filepath.Join(destination, "missing"), savedTree); err == nil {
		t.Fatal("a missing destination was accepted")
	}
}

func TestEnsureFolderCreatesMissingFolders(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	home := t.TempDir()
	folder := filepath.Join(home, "Nixorium", "Screenshots")
	if err := EnsureFolder(folder); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join(home, "Nixorium"), folder} {
		if info, err := os.Stat(name); err != nil || info.Mode() != os.ModeDir|0o755 {
			t.Fatalf("%s = %v, %v", name, info.Mode(), err)
		}
	}
	if err := EnsureFolder("relative/folder"); err == nil {
		t.Fatal("a relative folder was accepted")
	}
	file := filepath.Join(home, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureFolder(filepath.Join(file, "inside")); err == nil {
		t.Fatal("a folder inside a file was accepted")
	}
}

func TestStagedTreeWriterStagesElsewhereAndCreatesTheDestination(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "home", "Desktop")
	staging := filepath.Join(root, "home", ".local", "share", "staging")
	send := func() []string {
		t.Helper()
		before, _ := os.ReadDir(destination)
		writer, err := NewStagedTreeWriter(destination, staging, []FileEntry{{Path: "Lesson", Dir: true}, {Path: "Lesson/a.txt", Size: 2}, {Path: "b.txt", Size: 1}})
		if err != nil {
			t.Fatal(err)
		}
		if during, _ := os.ReadDir(destination); len(during) != len(before) {
			t.Fatal("something appeared in the destination before the end")
		}
		for index, data := range map[int]string{1: "hi", 2: "x"} {
			if err := writer.Write(index, []byte(data)); err != nil {
				t.Fatal(err)
			}
		}
		placed, err := writer.Finish()
		if err != nil {
			t.Fatal(err)
		}
		if left, _ := os.ReadDir(staging); len(left) != 0 {
			t.Fatalf("staging left behind: %v", left)
		}
		return placed
	}
	if placed := send(); strings.Join(placed, ",") != "Lesson,b.txt" {
		t.Fatalf("placed = %v", placed)
	}
	// The folder is non-empty now: the next sending gets a new name and
	// nothing is replaced.
	if placed := send(); strings.Join(placed, ",") != "Lesson (2),b (2).txt" {
		t.Fatalf("second placed = %v", placed)
	}
	if content, err := os.ReadFile(filepath.Join(destination, "Lesson", "a.txt")); err != nil || string(content) != "hi" {
		t.Fatalf("first folder changed: %q, %v", content, err)
	}
	if _, err := NewStagedTreeWriter(destination, staging, []FileEntry{{Path: "../escape", Size: 1}}); err == nil {
		t.Fatal("a path outside the sending was accepted")
	}
}
