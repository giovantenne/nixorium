package homereset

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func refusal(err error) string {
	var failure recoverFailure
	if errors.As(err, &failure) {
		return failure.code
	}
	return ""
}

func TestRecoverySessionsListsPublishedSnapshots(t *testing.T) {
	directory := t.TempDir()
	if sessions, err := recoverySessions(filepath.Join(directory, "missing")); err != nil || len(sessions) != 0 {
		t.Fatalf("missing folder = %v, %v", sessions, err)
	}
	for _, name := range []string{"snapshot-3", "snapshot-1", "snapshot-10", "snapshot-01", "other"} {
		if err := os.Mkdir(filepath.Join(directory, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "snapshot-2"), nil, 0o640); err != nil {
		t.Fatal(err)
	}
	sessions, err := recoverySessions(directory)
	if err != nil {
		t.Fatal(err)
	}
	numbers := []int{}
	for _, session := range sessions {
		numbers = append(numbers, session.Number)
		if session.SavedAt.IsZero() {
			t.Fatalf("session %d has no saving time", session.Number)
		}
	}
	if len(numbers) != 3 || numbers[0] != 1 || numbers[1] != 3 || numbers[2] != 10 {
		t.Fatalf("sessions = %v", numbers)
	}
	if _, err := sessionDirectory(directory, 4); refusal(err) != domain.FileRecoveryCodeNoSession {
		t.Fatalf("absent session = %v", err)
	}
	if _, err := sessionDirectory(directory, 31); refusal(err) != domain.FileRecoveryCodeInvalid {
		t.Fatalf("invalid session = %v", err)
	}
	if name, err := sessionDirectory(directory, 3); err != nil || name != filepath.Join(directory, "snapshot-3") {
		t.Fatalf("session 3 = %q, %v", name, err)
	}
}

func TestRecoveryListShowsFilesAndFoldersOnly(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"Desktop/notes.txt":   "hello",
		"Desktop/Project/a.c": "int main;",
		"Desktop/.hidden":     "x",
		"Documents/":          "",
		"zeta.txt":            "z",
		"Alpha.txt":           "a",
	})
	if err := os.Symlink("/etc/shadow", filepath.Join(root, "Desktop", "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "Documents"), filepath.Join(root, "Linked")); err != nil {
		t.Fatal(err)
	}
	listing, err := recoveryList(root, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range listing.Entries {
		names = append(names, entry.Name)
	}
	if strings.Join(names, ",") != "Desktop,Documents,Alpha.txt,zeta.txt" || listing.Session != 2 {
		t.Fatalf("home listing = %v", names)
	}
	listing, err = recoveryList(root, 2, "Desktop")
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 2 || listing.Entries[0].Name != "Project" || !listing.Entries[0].Dir || listing.Entries[1].Name != "notes.txt" || listing.Entries[1].Size != 5 {
		t.Fatalf("desktop listing = %+v", listing.Entries)
	}
	for _, name := range []string{"Linked", "Desktop/link", "missing", "Alpha.txt"} {
		if _, err := recoveryList(root, 2, name); refusal(err) != domain.FileRecoveryCodeNotFound {
			t.Fatalf("listing %q = %v", name, err)
		}
	}
	for _, name := range []string{"../etc", "/etc", "Desktop/../..", "a//b"} {
		if _, err := recoveryList(root, 2, name); refusal(err) != domain.FileRecoveryCodeInvalid {
			t.Fatalf("listing %q = %v", name, err)
		}
	}
}

func TestRecoveryTreeAndWriting(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"Desktop/notes.txt":           "hello",
		"Desktop/Project/main.c":      "int main;",
		"Desktop/Project/.git/config": "secret",
		"Desktop/Project/empty.txt":   "",
		"Documents/notes.txt":         "other notes",
	})
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "Desktop", "Project", "passwd")); err != nil {
		t.Fatal(err)
	}
	entries, sources, err := recoveryTree(root, []string{"Desktop/notes.txt", "Desktop/Project", "Documents/notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	if strings.Join(paths, ",") != "notes.txt,Project,Project/empty.txt,Project/main.c,notes (2).txt" || len(sources) != 4 {
		t.Fatalf("tree = %v, sources = %v", paths, sources)
	}

	var stream bytes.Buffer
	if err := streamRecovery(&stream, root, entries, sources); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	writeTree(t, home, map[string]string{"Desktop/Recovered files/notes.txt": "kept"})
	restored, err := writeRecoveredInto(&stream, home)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Folder != "Desktop/Recovered files" || restored.Files != 4 || restored.Bytes != int64(len("hello")+len("int main;")+len("other notes")) {
		t.Fatalf("restored = %+v", restored)
	}
	if strings.Join(restored.Placed, ",") != "notes (2).txt,Project,notes (2) (2).txt" {
		t.Fatalf("placed = %v", restored.Placed)
	}
	for name, want := range map[string]string{
		"notes.txt":         "kept",
		"notes (2).txt":     "hello",
		"Project/main.c":    "int main;",
		"Project/empty.txt": "",
	} {
		data, err := os.ReadFile(filepath.Join(home, "Desktop", domain.FileRecoveryFolder, filepath.FromSlash(name)))
		if err != nil || string(data) != want {
			t.Fatalf("%s = %q, %v", name, data, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, "Desktop", domain.FileRecoveryFolder, "Project", ".git")); !os.IsNotExist(err) {
		t.Fatalf("hidden folder recovered: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, "Desktop", domain.FileRecoveryFolder, "Project", "passwd")); !os.IsNotExist(err) {
		t.Fatalf("link recovered: %v", err)
	}
}

func TestRecoveryTreeRefusals(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"Desktop/notes.txt": "hello"})
	if err := os.Symlink("/etc", filepath.Join(root, "Desktop", "etc")); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		paths []string
		code  string
	}{
		{nil, domain.FileRecoveryCodeInvalid},
		{[]string{""}, domain.FileRecoveryCodeInvalid},
		{[]string{"../x"}, domain.FileRecoveryCodeInvalid},
		{[]string{"Desktop", "Desktop/notes.txt"}, domain.FileRecoveryCodeInvalid},
		{[]string{"Desktop/missing"}, domain.FileRecoveryCodeNotFound},
		{[]string{"Desktop/etc"}, domain.FileRecoveryCodeNotFound},
		{[]string{"Desktop/etc/passwd"}, domain.FileRecoveryCodeNotFound},
	} {
		if _, _, err := recoveryTree(root, test.paths); refusal(err) != test.code {
			t.Fatalf("%v = %v, want %s", test.paths, err, test.code)
		}
	}
	big := filepath.Join(root, "Big")
	if err := os.Mkdir(big, 0o755); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < classroomview.MaxFileEntries; index++ {
		if err := os.WriteFile(filepath.Join(big, fmt.Sprintf("f%04d", index)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := recoveryTree(root, []string{"Big"}); refusal(err) != domain.FileRecoveryCodeTooLarge {
		t.Fatalf("too many files = %v", err)
	}
}

func TestWritingRefusesBrokenStreams(t *testing.T) {
	home := t.TempDir()
	if _, err := writeRecoveredInto(bytes.NewReader([]byte{0, 0, 0, 0}), home); err == nil {
		t.Fatal("an empty header was accepted")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "abcdef"})
	entries, sources, err := recoveryTree(root, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	if err := streamRecovery(&stream, root, entries, sources); err != nil {
		t.Fatal(err)
	}
	short := stream.Bytes()[:stream.Len()-2]
	if _, err := writeRecoveredInto(bytes.NewReader(short), home); err == nil {
		t.Fatal("a short stream was accepted")
	}
	if _, err := os.Lstat(filepath.Join(home, "Desktop", domain.FileRecoveryFolder, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("a partial file appeared: %v", err)
	}
}

func TestUniqueName(t *testing.T) {
	used := map[string]bool{}
	got := []string{uniqueName("a.txt", used), uniqueName("a.txt", used), uniqueName("Folder", used), uniqueName("Folder", used), uniqueName(".bashrc", used), uniqueName(".bashrc", used)}
	if strings.Join(got, ",") != "a.txt,a (2).txt,Folder,Folder (2),.bashrc,.bashrc (2)" {
		t.Fatalf("names = %v", got)
	}
}

// TestMain lets the test binary act as the writing child that
// recoverAsStudent starts again with RecoverWriteArgument.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == RecoverWriteArgument {
		if err := WriteRecovered(os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
