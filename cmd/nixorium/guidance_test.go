//go:build guidance

package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Compiled separately from the product; documentation is read at runtime so
// editing prose does not invalidate the application or checker build cache.
func guidanceRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("NIXORIUM_GUIDANCE_ROOT")
	if root == "" {
		t.Fatal("NIXORIUM_GUIDANCE_ROOT must name the checkout to validate")
	}
	return root
}

func guidanceFiles(t *testing.T, root string) []string {
	t.Helper()
	files := []string{"AGENTS.md", "templates/site/AGENTS.md", "docs/agent-guidance.md"}
	err := filepath.WalkDir(filepath.Join(root, "skills"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".md") {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func guidanceRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Only simple documented commands are accepted; never evaluate shell syntax.
func guidanceCommands(body string) ([][]string, error) {
	var commands [][]string
	var pending string
	inShell := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			if pending != "" {
				return nil, fmt.Errorf("unfinished command continuation")
			}
			inShell = line == "```sh" || line == "```bash"
			continue
		}
		if !inShell {
			continue
		}
		line = pending + line
		if strings.HasSuffix(line, "\\") {
			pending = strings.TrimSuffix(line, "\\") + " "
			continue
		}
		pending = ""
		var args string
		switch {
		case strings.HasPrefix(line, "nixorium "):
			args = strings.TrimPrefix(line, "nixorium ")
		case strings.HasPrefix(line, "nix run .#nixorium -- "):
			args = strings.TrimPrefix(line, "nix run .#nixorium -- ")
		default:
			continue
		}
		fields := strings.Fields(args)
		for i, field := range fields {
			if strings.HasPrefix(field, "'") || strings.HasPrefix(field, "\"") {
				if len(field) < 2 || field[len(field)-1] != field[0] {
					return nil, fmt.Errorf("use simple single-token example arguments: %s", line)
				}
				field = field[1 : len(field)-1]
			}
			if strings.ContainsAny(field, "|;&$`") {
				return nil, fmt.Errorf("unsupported shell expression in example: %s", line)
			}
			fields[i] = field
		}
		commands = append(commands, fields)
	}
	if pending != "" {
		return nil, fmt.Errorf("unfinished command continuation")
	}
	return commands, nil
}

func TestAgentGuidanceCommands(t *testing.T) {
	root := guidanceRoot(t)
	count := 0
	for _, path := range guidanceFiles(t, root) {
		commands, err := guidanceCommands(string(guidanceRead(t, filepath.Join(root, path))))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, args := range commands {
			count++
			if _, err := parseArguments(args); err != nil {
				t.Errorf("%s: nixorium %s: %v", path, strings.Join(args, " "), err)
			}
		}
	}
	if count == 0 {
		t.Fatal("no documented CLI examples were checked")
	}
	t.Logf("validated %d CLI examples without executing operations", count)
}

func TestAgentGuidanceCommandExtraction(t *testing.T) {
	body := "```sh\nnix run .#nixorium -- config apply --file candidate.json \\\n  --expect 'sha256:example'\nnixorium software plan --package vlc --scope shared\n```\n"
	commands, err := guidanceCommands(body)
	if err != nil || len(commands) != 2 {
		t.Fatalf("commands = %v, error = %v", commands, err)
	}
	for _, args := range commands {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(err)
		}
	}
	for _, example := range []string{
		"nixorium software plan --package vlc --scope nonexistent",
		"nixorium deploy apply --on @lab", // missing review token
		"nixorium nonexistent",
	} {
		commands, err := guidanceCommands("```sh\n" + example + "\n```\n")
		if err != nil || len(commands) != 1 {
			t.Fatalf("invalid example not extracted: %s", example)
		}
		if _, err := parseArguments(commands[0]); err == nil {
			t.Errorf("broken example accepted: %s", example)
		}
	}
	if _, err := guidanceCommands("```sh\nnixorium status; touch /tmp/never-executed\n```\n"); err == nil {
		t.Fatal("shell expression accepted")
	}
}

func TestAgentGuidanceLinks(t *testing.T) {
	root := guidanceRoot(t)
	link := regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
	for _, path := range guidanceFiles(t, root) {
		body := guidanceRead(t, filepath.Join(root, path))
		for _, match := range link.FindAllSubmatch(body, -1) {
			target := strings.SplitN(string(match[1]), "#", 2)[0]
			if target == "" || strings.Contains(target, "://") {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.Dir(path), target)); err != nil {
				t.Errorf("%s: broken relative link %s: %v", path, target, err)
			}
		}
	}
}

func guidanceTree(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected non-regular skill file: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel], err = os.ReadFile(path)
		return err
	})
	return files, err
}

func TestAgentGuidanceDistribution(t *testing.T) {
	root := guidanceRoot(t)
	upstream, err := guidanceTree(filepath.Join(root, "skills/nixorium-maintainer"))
	if err != nil {
		t.Fatal(err)
	}
	site, err := guidanceTree(filepath.Join(root, "templates/site/skills/nixorium-maintainer"))
	if err != nil {
		t.Fatal(err)
	}
	if len(upstream) == 0 || len(upstream) != len(site) {
		t.Fatal("maintainer distribution has missing or extra files")
	}
	for path, expected := range upstream {
		if actual, ok := site[path]; !ok || !bytes.Equal(actual, expected) {
			t.Errorf("maintainer distribution differs: %s", path)
		}
	}
	for _, base := range []string{"", "templates/site"} {
		for _, discovery := range []string{".agents", ".claude", ".pi"} {
			for _, skill := range []string{"nixorium-developer", "nixorium-maintainer"} {
				path := filepath.Join(root, base, discovery, "skills", skill)
				if base != "" && skill == "nixorium-developer" {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Errorf("developer skill must not ship in deployment: %s", path)
					}
					continue
				}
				actual, err := filepath.EvalSymlinks(path)
				expected := filepath.Join(root, base, "skills", skill)
				if err != nil || actual != expected {
					t.Errorf("discovery %s resolves to %s, want %s: %v", path, actual, expected, err)
				}
				body := guidanceRead(t, filepath.Join(expected, "SKILL.md"))
				if !bytes.HasPrefix(body, []byte("---\nname: "+skill+"\n")) {
					t.Errorf("skill entrypoint name/frontmatter disagrees with discovery: %s", expected)
				}
			}
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "templates/site/skills/nixorium-developer")); !os.IsNotExist(err) {
		t.Fatal("developer skill must not be distributed in site skills")
	}
}
