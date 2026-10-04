package classroomview

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateFileEntries(t *testing.T) {
	good := []FileEntry{{Path: "Lesson", Dir: true}, {Path: "Lesson/notes 1.txt", Size: 12}, {Path: "image.png", Size: 0}}
	if err := ValidateFileEntries(good); err != nil {
		t.Fatal(err)
	}
	for name, entries := range map[string][]FileEntry{
		"empty":            nil,
		"absolute":         {{Path: "/etc/passwd", Size: 1}},
		"parent":           {{Path: "../x", Size: 1}},
		"inner parent":     {{Path: "a", Dir: true}, {Path: "a/../../x", Size: 1}},
		"dot":              {{Path: "./x", Size: 1}},
		"trailing slash":   {{Path: "a/", Dir: true}},
		"control":          {{Path: "a\nb", Size: 1}},
		"duplicate":        {{Path: "a", Size: 1}, {Path: "a", Size: 1}},
		"missing folder":   {{Path: "a/b", Size: 1}},
		"folder after":     {{Path: "a/b", Size: 1}, {Path: "a", Dir: true}},
		"file as folder":   {{Path: "a", Size: 1}, {Path: "a/b", Size: 1}},
		"folder with size": {{Path: "a", Dir: true, Size: 3}},
		"negative":         {{Path: "a", Size: -1}},
		"too large":        {{Path: "a", Size: MaxFileBytes}, {Path: "b", Size: 1}},
		"long name":        {{Path: strings.Repeat("x", 256), Size: 1}},
		"invalid utf8":     {{Path: "a\xff", Size: 1}},
	} {
		if err := ValidateFileEntries(entries); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	many := make([]FileEntry, MaxFileEntries+1)
	for index := range many {
		many[index] = FileEntry{Path: fmt.Sprintf("f%d", index)}
	}
	if err := ValidateFileEntries(many[:MaxFileEntries]); err != nil {
		t.Fatalf("the maximum was refused: %v", err)
	}
	if err := ValidateFileEntries(many); err == nil {
		t.Fatal("too many entries were accepted")
	}
}
