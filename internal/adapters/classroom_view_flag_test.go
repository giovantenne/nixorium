package adapters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassroomViewAppliedFailsClosed(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for name, test := range map[string]struct {
		path string
		want bool
	}{
		"on":        {write("on", `{"schemaVersion":1,"enabled":true}`), true},
		"off":       {write("off", `{"schemaVersion":1,"enabled":false}`), false},
		"version":   {write("version", `{"schemaVersion":2,"enabled":true}`), false},
		"unknown":   {write("unknown", `{"schemaVersion":1,"enabled":true,"x":1}`), false},
		"malformed": {write("bad", `on`), false},
		"missing":   {filepath.Join(dir, "missing"), false},
	} {
		if got := classroomViewAppliedAt(test.path); got != test.want {
			t.Errorf("%s: got %v, want %v", name, got, test.want)
		}
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "on"), link); err != nil {
		t.Fatal(err)
	}
	if classroomViewAppliedAt(link) {
		t.Error("a symbolic link must not enable the view")
	}
}
