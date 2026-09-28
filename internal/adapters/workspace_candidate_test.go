package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func TestReadWorkspaceCandidateIsBoundedAndNoFollow(t *testing.T) {
	for _, kind := range []string{"regular", "symlink", "fifo", "directory", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "candidate.json")
			var err error
			switch kind {
			case "regular":
				err = os.WriteFile(path, []byte(`{"schemaVersion":1}`), 0600)
			case "symlink":
				err = os.Symlink("/etc/passwd", path)
			case "fifo":
				err = unix.Mkfifo(path, 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "oversized":
				err = os.WriteFile(path, []byte(strings.Repeat(" ", domain.WorkspaceMaxBytes+1)), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := ReadWorkspaceCandidate(path)
			if kind == "regular" {
				if err != nil || string(data) != `{"schemaVersion":1}` {
					t.Fatalf("candidate: %s %v", data, err)
				}
			} else if err == nil {
				t.Fatalf("accepted %s", kind)
			}
		})
	}
}
