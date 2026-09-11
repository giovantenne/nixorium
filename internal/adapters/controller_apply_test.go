package adapters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestControllerAppliedReportsMissingActiveGeneration(t *testing.T) {
	if _, err := os.Lstat("/run/current-system"); err == nil {
		t.Skip("test host has an active NixOS generation")
	}
	directory := t.TempDir()
	writeExecutable(t, filepath.Join(directory, "git"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(directory, "nix"), `#!/bin/sh
case "$*" in
  *labMeta*) printf 'warning: synthetic unlocked input\n' >&2; printf '{"schemaVersion":2,"version":"test","controller":{"name":"pc99","number":99,"staticIp":"10.0.0.99","dhcpIp":"192.0.2.10"},"clients":{"count":0,"hosts":[]},"network":{"base":"10.0.0.0","prefixLength":24,"ifaceName":"eth0","cachePort":5000,"pxeHttpPort":8080},"users":{"student":"student","teacher":"teacher"}}' ;;
  *toplevel*) printf 'warning: synthetic unlocked input\n' >&2; printf '/nix/store/desired-system' ;;
  *) exit 2 ;;
esac
`)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	applied, detail := (Local{}).ControllerApplied(t.Context(), "/repo")
	if applied || detail != "no active NixOS system generation was found" {
		t.Fatalf("applied = %v, detail = %q", applied, detail)
	}
}
