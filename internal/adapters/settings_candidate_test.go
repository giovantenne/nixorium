package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCandidateUsesPrivateTemporaryFileAndCleansIt(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "marker")
	writeExecutable(t, filepath.Join(directory, "nix"), `#!/bin/sh
test -n "$NIXORIUM_CANDIDATE_FILE"
test "$(stat -c '%a' "$NIXORIUM_CANDIDATE_FILE")" = 600
grep -q 'masterDhcpIp' "$NIXORIUM_CANDIDATE_FILE"
printf '%s' "$NIXORIUM_CANDIDATE_FILE" > "$NIXORIUM_TEST_MARKER"
printf 'true\n'
`)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NIXORIUM_TEST_MARKER", marker)

	if err := (Local{}).ValidateCandidate(context.Background(), "/deployment", adapterSettings()); err != nil {
		t.Fatal(err)
	}
	path, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(string(path)); !os.IsNotExist(err) {
		t.Fatalf("candidate temporary file remains: %v", err)
	}
}

func TestValidateCandidateRedactsNixOutput(t *testing.T) {
	directory := t.TempDir()
	writeExecutable(t, filepath.Join(directory, "nix"), "#!/bin/sh\nprintf '$6$secret$hash' >&2\nexit 7\n")
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := (Local{}).ValidateCandidate(context.Background(), "/deployment", adapterSettings())
	if err == nil || strings.Contains(err.Error(), "$6$secret$hash") {
		t.Fatalf("candidate error leaked command output: %v", err)
	}
}
