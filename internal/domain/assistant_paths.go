package domain

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidAssistantPath(path string) bool {
	if path == "" || len(path) > 512 || !utf8.ValidString(path) || strings.ContainsAny(path, "\\") || strings.HasPrefix(path, "/") {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) > 12 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}
func AssistantHiddenPath(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == ".git" || part == "keys" || part == "lab-credentials.json" {
			return true
		}
		for _, prefix := range []string{".env", ".ssh", "secret-key", "auth.json", "admin-ssh", "skills", ".pi", ".codex", ".agents"} {
			if strings.HasPrefix(part, prefix) {
				return true
			}
		}
	}
	return false
}
func AssistantReadOnlyPath(path string) bool {
	for _, part := range strings.Split(path, "/") {
		switch part {
		case "lab-settings.json", "flake.lock", "AGENTS.md", "CLAUDE.md", ".gitignore", ".gitattributes", ".gitmodules":
			return true
		}
	}
	return false
}

var assistantRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?(?:-----END [A-Z0-9 ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`\$6\$[^\s"'\\]+`),
	regexp.MustCompile(`(?i)Bearer[ \t]+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?:sk-|ghp_|github_pat_)[A-Za-z0-9_-]+`),
}

func RedactForModel(text string) string {
	for _, pattern := range assistantRedactions {
		text = pattern.ReplaceAllString(text, "[REDACTED]")
	}
	return text
}
