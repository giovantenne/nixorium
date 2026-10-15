package classroomview

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestValidateCollectEntriesRefusesUnsafeNames(t *testing.T) {
	if err := ValidateCollectEntries(nil); err != nil {
		t.Fatalf("an empty desktop was refused: %v", err)
	}
	good := []FileEntry{{Path: "Homework", Dir: true}, {Path: "Homework/essay 1.odt", Size: 12}, {Path: "notes.txt"}}
	if err := ValidateCollectEntries(good); err != nil {
		t.Fatal(err)
	}
	deep := strings.TrimSuffix(strings.Repeat("d/", maxPathElements+1), "/")
	for name, entries := range map[string][]FileEntry{
		"parent":             {{Path: "../x", Size: 1}},
		"inner parent":       {{Path: "a", Dir: true}, {Path: "a/../../x", Size: 1}},
		"only parent":        {{Path: "..", Dir: true}},
		"absolute":           {{Path: "/etc/passwd", Size: 1}},
		"dot":                {{Path: "./x", Size: 1}},
		"backslash":          {{Path: `a\b`, Size: 1}},
		"backslash parent":   {{Path: `..\..\x`, Size: 1}},
		"newline":            {{Path: "a\nb", Size: 1}},
		"nul":                {{Path: "a\x00b", Size: 1}},
		"escape sequence":    {{Path: "\x1b[31mred", Size: 1}},
		"delete":             {{Path: "a\x7fb", Size: 1}},
		"invalid utf8":       {{Path: "a\xff", Size: 1}},
		"long name":          {{Path: strings.Repeat("x", 256), Size: 1}},
		"deep":               {{Path: deep, Size: 1}},
		"duplicate":          {{Path: "a", Size: 1}, {Path: "a", Size: 1}},
		"missing folder":     {{Path: "a/b", Size: 1}},
		"folder after":       {{Path: "a/b", Size: 1}, {Path: "a", Dir: true}},
		"file as folder":     {{Path: "a", Size: 1}, {Path: "a/b", Size: 1}},
		"folder with size":   {{Path: "a", Dir: true, Size: 3}},
		"negative size":      {{Path: "a", Size: -1}},
		"empty name":         {{Path: "", Size: 1}},
		"trailing separator": {{Path: "a/", Dir: true}},
	} {
		err := ValidateCollectEntries(entries)
		if err == nil || errors.Is(err, ErrCollectTooMuch) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	many := make([]FileEntry, MaxFileEntries+1)
	for index := range many {
		many[index] = FileEntry{Path: fmt.Sprintf("f%d", index)}
	}
	if err := ValidateCollectEntries(many[:MaxFileEntries]); err != nil {
		t.Fatalf("the maximum was refused: %v", err)
	}
	// Names add up past MaxCollectPathBytes well before 2000 items.
	folder := strings.Repeat("f", 255)
	longNames := []FileEntry{{Path: folder, Dir: true}}
	for index := 1; index < MaxFileEntries; index++ {
		longNames = append(longNames, FileEntry{Path: fmt.Sprintf("%s/%0250d", folder, index)})
	}
	for name, entries := range map[string][]FileEntry{
		"too many items": many,
		"too large":      {{Path: "a", Size: MaxFileBytes}, {Path: "b", Size: 1}},
		"one huge file":  {{Path: "a", Size: MaxFileBytes + 1}},
		"too many names": longNames,
	} {
		if err := ValidateCollectEntries(entries); !errors.Is(err, ErrCollectTooMuch) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func writeTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListDesktopLeavesOutHiddenLinksAndSpecialFiles(t *testing.T) {
	desktop := filepath.Join(t.TempDir(), "Desktop")
	for name, content := range map[string]string{
		"essay.odt":            "essay",
		"Homework/a.txt":       "a",
		"Homework/Empty/.k":    "",
		".hidden":              "x",
		".config/secret":       "x",
		"Homework/.cache/tmp":  "x",
		"bad\nname.txt":        "x",
		`back\slash.txt`:       "x",
		"Bad\x1bFolder/in.txt": "x",
	} {
		writeTestFile(t, filepath.Join(desktop, filepath.FromSlash(name)), content)
	}
	deep := filepath.Join(desktop, "Deep")
	for level := 0; level < maxPathElements; level++ {
		deep = filepath.Join(deep, "d")
	}
	writeTestFile(t, filepath.Join(deep, "too-deep.txt"), "x")
	if err := os.Symlink("/etc/passwd", filepath.Join(desktop, "passwd")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(desktop, "Elsewhere")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(desktop, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, skipped, err := ListDesktop(desktop)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, "Deep") {
			continue
		}
		got = append(got, fmt.Sprintf("%s:%d:%t", entry.Path, entry.Size, entry.Dir))
	}
	if strings.Join(got, ",") != "Homework:0:true,Homework/Empty:0:true,Homework/a.txt:1:false,essay.odt:5:false" {
		t.Fatalf("entries = %v", got)
	}
	// The newline, the backslash, the control character and the folder
	// nested too deeply are counted; hidden names, links and the pipe are not.
	if skipped != 4 {
		t.Fatalf("skipped = %d", skipped)
	}
	for _, entry := range entries {
		if strings.Count(entry.Path, "/") >= maxPathElements {
			t.Fatalf("a path nested too deeply was listed: %s", entry.Path)
		}
	}
	if err := ValidateCollectEntries(entries); err != nil {
		t.Fatalf("the list is not valid: %v", err)
	}
}

func TestListDesktopMissingOrLinkedIsEmptyAndLimitsStopIt(t *testing.T) {
	home := t.TempDir()
	if entries, skipped, err := ListDesktop(filepath.Join(home, "Desktop")); err != nil || len(entries) != 0 || skipped != 0 {
		t.Fatalf("missing desktop = %v %d %v", entries, skipped, err)
	}
	target := t.TempDir()
	writeTestFile(t, filepath.Join(target, "secret.txt"), "x")
	if err := os.Symlink(target, filepath.Join(home, "Linked")); err != nil {
		t.Fatal(err)
	}
	if entries, _, err := ListDesktop(filepath.Join(home, "Linked")); err != nil || len(entries) != 0 {
		t.Fatalf("linked desktop = %v %v", entries, err)
	}
	many := filepath.Join(home, "Many")
	for index := 0; index <= MaxFileEntries; index++ {
		writeTestFile(t, filepath.Join(many, fmt.Sprintf("f%04d", index)), "")
	}
	if _, _, err := ListDesktop(many); !errors.Is(err, ErrCollectTooMuch) {
		t.Fatalf("too many items = %v", err)
	}
	large := filepath.Join(home, "Large")
	writeTestFile(t, filepath.Join(large, "video.mkv"), "")
	if err := os.Truncate(filepath.Join(large, "video.mkv"), MaxFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ListDesktop(large); !errors.Is(err, ErrCollectTooMuch) {
		t.Fatalf("too large = %v", err)
	}
}

func TestCollectMessagesRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	listed := Message{Type: TypeCollectListed, Entries: []FileEntry{{Path: "a", Dir: true}, {Path: "a/b.txt", Size: 3}}, Skipped: 2}
	if err := Write(&buffer, listed); err != nil {
		t.Fatal(err)
	}
	message, err := Read(&buffer)
	if err != nil || message.Type != TypeCollectListed || len(message.Entries) != 2 || message.Entries[1].Size != 3 || message.Skipped != 2 {
		t.Fatalf("message = %+v, %v", message, err)
	}
	// The largest list an agent sends still fits in one message.
	entries := []FileEntry{}
	names := 0
	for index := 0; names+256 <= MaxCollectPathBytes && index < MaxFileEntries; index++ {
		name := fmt.Sprintf("%s%05d", strings.Repeat("<", 250), index)
		entries = append(entries, FileEntry{Path: name, Size: 1 << 20})
		names += len(name)
	}
	if err := Write(&bytes.Buffer{}, Message{Type: TypeCollectListed, Entries: entries}); err != nil {
		t.Fatalf("a list within the limits does not fit: %v", err)
	}
}
