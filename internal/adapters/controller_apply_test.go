package adapters

import (
	"fmt"
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

func TestControllerActivationRecordMatchesRevisionAndSystem(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	systemPath := "/nix/store/0123456789abcdfghijklmnpqrsvwxyz-nixos-system-pc99"
	valid := []byte(fmt.Sprintf(`{"schemaVersion":1,"revision":%q,"systemPath":%q,"activatedAt":"2026-09-14T17:00:00Z"}`, revision, systemPath))

	tests := []struct {
		name     string
		data     []byte
		mode     uint32
		revision string
		desired  string
		current  bool
	}{
		{name: "matching", data: valid, mode: 0o644, revision: revision, desired: systemPath, current: true},
		{name: "unsafe mode", data: valid, mode: 0o600, revision: revision, desired: systemPath},
		{name: "stale revision", data: valid, mode: 0o644, revision: "fedcba9876543210fedcba9876543210fedcba98", desired: systemPath},
		{name: "wrong system", data: valid, mode: 0o644, revision: revision, desired: "/nix/store/zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz-other"},
		{name: "invalid", data: []byte(`{}`), mode: 0o644, revision: revision, desired: systemPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current, _ := controllerActivationMatches(test.data, test.mode, test.revision, test.desired)
			if current != test.current {
				t.Fatalf("current = %v, want %v", current, test.current)
			}
		})
	}
}
