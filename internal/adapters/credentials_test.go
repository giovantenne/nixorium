package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashPasswordUsesOnlyStdin(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "mkpasswd")
	content := "#!/bin/sh\n" +
		"[ \"$1\" = '-m' ] && [ \"$2\" = 'sha-512' ] && [ \"$3\" = '--stdin' ] && [ \"$#\" = 3 ] || exit 20\n" +
		"IFS= read -r password\n" +
		"[ \"$password\" = 'test-secret' ] || exit 21\n" +
		"printf '%s\\n' '$6$salt$hash'\n"
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	hash, err := (Local{}).HashPassword(context.Background(), []byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if hash != "$6$salt$hash" {
		t.Fatalf("hash = %q", hash)
	}
}

func TestActivateBootstrapKeyboardUsesSelectedConsoleKeymap(t *testing.T) {
	directory := t.TempDir()
	logPath := filepath.Join(directory, "calls")
	for name, content := range map[string]string{
		"loadkeys": "#!/bin/sh\nexit 0\n",
		"sudo":     "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$KEYBOARD_LOG\"\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	t.Setenv("KEYBOARD_LOG", logPath)
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if err := (Local{}).ActivateBootstrapKeyboard(context.Background(), "it2"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "loadkeys it2" {
		t.Fatalf("sudo arguments = %q", data)
	}
}

func TestActivateBootstrapKeyboardRejectsGraphicalTerminal(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	t.Setenv("WAYLAND_DISPLAY", "")
	err := (Local{}).ActivateBootstrapKeyboard(context.Background(), "it2")
	if err == nil || !strings.Contains(err.Error(), "graphical terminal") {
		t.Fatalf("error = %v", err)
	}
}
