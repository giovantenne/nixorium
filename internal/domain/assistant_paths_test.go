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

func TestAssistantRedactionPatterns(t *testing.T) {
	secretKey := "cache.example-1:" + strings.Repeat("A1b+", 21) + "xY" + "=="
	publicKey := "cache.example-1:" + strings.Repeat("A1b+", 10) + "xyz="
	for _, test := range []struct{ name, input, want string }{
		{"yescrypt", "hash = \"$y$j9T$salt$digest\";", "hash = \"[REDACTED]\";"},
		{"sha256 crypt", "x $5$salt$digest y", "x [REDACTED] y"},
		{"bcrypt", "$2b$12$abcdefghijklmnopqrstuv", "[REDACTED]"},
		{"bcrypt 2y", "$2y$10$abcdefghijklmnopqrstuv", "[REDACTED]"},
		{"age", "key AGE-SECRET-KEY-1QQQQ3QQQ9 end", "key [REDACTED] end"},
		{"cache secret", "secret-key-file: " + secretKey + "\n", "secret-key-file: [REDACTED]\n"},
		{"psk unquoted", "psk=hunter2hunter2\nssid=lab", "psk=[REDACTED]\nssid=lab"},
		{"psk quoted", "networking.wireless.networks.lab.psk = \"two words\";", "networking.wireless.networks.lab.psk = [REDACTED];"},
		{"wpa_passphrase", "wpa_passphrase=abc12345", "wpa_passphrase=[REDACTED]"},
		{"pskRaw", "pskRaw = \"0123456789abcdef\";", "pskRaw = [REDACTED];"},
		{"public cache key", "trusted-public-keys = " + publicKey, "trusted-public-keys = " + publicKey},
		{"ordinary base64", "data: " + strings.Repeat("QUJD", 30) + "==", "data: " + strings.Repeat("QUJD", 30) + "=="},
		{"sha256 hash", "sha256-" + strings.Repeat("A1b+", 10) + "xyz=", "sha256-" + strings.Repeat("A1b+", 10) + "xyz="},
		{"dollar variable", "echo $5 $y $2", "echo $5 $y $2"},
		{"psk file option", "pskFile = /run/keys/psk;", "pskFile = /run/keys/psk;"},
	} {
		if got := RedactForModel(test.input); got != test.want {
			t.Errorf("%s: got %q want %q", test.name, got, test.want)
		}
	}
}
