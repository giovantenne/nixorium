package domain

import (
	"strings"
	"testing"
)

func TestAssistantPathBoundaries(t *testing.T) {
	for _, path := range []string{"flake.nix", "modules/course.nix", ".gitignore", "é.nix"} {
		if !ValidAssistantPath(path) {
			t.Fatal(path)
		}
	}
	for _, path := range []string{"", "/tmp/x", "../x", "a/../b", "a//b", "a/./b", "a\\b", "x\n", "x\x00", strings.Repeat("x", 513), strings.Repeat("a/", 12) + "x", string([]byte{255})} {
		if ValidAssistantPath(path) {
			t.Fatal(path)
		}
	}
	for _, path := range []string{".git/config", "keys/public", "lab-credentials.json", "modules/lab-credentials.json", "modules/.env.local", ".ssh/id", "secret-key.old", "auth.json", "admin-ssh.pub", "skills/x", ".pi/x", ".codex/x", ".agents/x"} {
		if !AssistantHiddenPath(path) {
			t.Fatal(path)
		}
	}
	if AssistantHiddenPath("lab-credentials.json.notes") {
		t.Fatal("credential filename boundary must be exact")
	}
	for _, path := range []string{"lab-settings.json", "flake.lock", "AGENTS.md", "CLAUDE.md", ".gitignore", ".gitattributes", ".gitmodules"} {
		if !AssistantReadOnlyPath(path) || AssistantHiddenPath(path) {
			t.Fatal(path)
		}
	}
}
func TestAssistantRedactsKnownSecrets(t *testing.T) {
	for _, secret := range []string{"$6$salt$hash", "Bearer abcdef_123", "sk-example", "ghp_example", "github_pat_example", "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----", "-----BEGIN OPENSSH PRIVATE KEY-----\nunfinished"} {
		if got := RedactForModel("before " + secret + " after"); strings.Contains(got, secret) || !strings.Contains(got, "[REDACTED]") {
			t.Fatal(got)
		}
	}
	if RedactForModel("ordinary content") != "ordinary content" {
		t.Fatal("ordinary content changed")
	}
}
