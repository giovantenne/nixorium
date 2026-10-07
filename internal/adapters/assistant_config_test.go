package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLoadAssistantConfig(t *testing.T) {
	valid := `{"schemaVersion":1,"enabled":true,"endpoint":"https://assistant.example.org/v1","subscribeUrl":"https://example.org/assistant/"}`
	for _, test := range []struct {
		name, data        string
		invalid, disabled bool
	}{
		{name: "production", data: valid},
		{name: "disabled", data: strings.Replace(valid, `"enabled":true`, `"enabled":false`, 1), disabled: true},
		{name: "loopback", data: strings.Replace(valid, "https://assistant.example.org/v1", "http://127.0.0.1:18480/v1", 1)},
		{name: "localhost", data: strings.Replace(valid, "https://assistant.example.org/v1", "http://localhost:18480/v1", 1)},
		{name: "unknown field", data: strings.TrimSuffix(valid, "}") + `,"token":"not-a-real-token"}`, invalid: true},
		{name: "schema", data: strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":2`, 1), invalid: true},
		{name: "trailing JSON", data: valid + ` {}`, invalid: true},
		{name: "trailing garbage", data: valid + `x`, invalid: true},
		{name: "oversized", data: valid + strings.Repeat(" ", 4096), invalid: true},
		{name: "invalid JSON", data: `{`, invalid: true},
		{name: "missing URL", data: `{"schemaVersion":1,"enabled":true}`, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "assistant.json")
			if err := os.WriteFile(path, []byte(test.data), 0600); err != nil {
				t.Fatal(err)
			}
			config, err := LoadAssistantConfig(path)
			if (err != nil) != test.invalid || config.Enabled != (!test.invalid && !test.disabled) {
				t.Fatalf("config=%+v, err=%v", config, err)
			}
			if test.invalid && (config.Endpoint != "" || config.SubscribeURL != "") {
				t.Fatal("invalid configuration did not fail closed")
			}
		})
	}
	for _, endpoint := range []string{
		"http://example.org/v1", "http://127.0.0.1/v1", "http://127.0.0.1:0/v1", "http://localhost:65536/v1",
		"https://user:password@example.org/v1", "https://example.org/v1?token=x", "https://example.org/v1?",
		"https://example.org/v1#section", "https://example.org/v1#", "https:///v1", "//example.org/v1",
	} {
		t.Run(endpoint, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "assistant.json")
			data := fmt.Sprintf(`{"schemaVersion":1,"enabled":true,"endpoint":%q,"subscribeUrl":"https://example.org/"}`, endpoint)
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			config, err := LoadAssistantConfig(path)
			if err == nil || config.Enabled {
				t.Fatalf("accepted %s", endpoint)
			}
		})
	}
	if assistantConfigURL("http://localhost:18480/", false) {
		t.Fatal("subscription URL must use HTTPS")
	}
}

func TestLoadAssistantConfigRejectsUnsafeFiles(t *testing.T) {
	directory := t.TempDir()
	for name, prepare := range map[string]func(string) error{
		"missing":   func(string) error { return nil },
		"directory": func(path string) error { return os.Mkdir(path, 0700) },
		"symlink":   func(path string) error { return os.Symlink("/dev/null", path) },
		"fifo":      func(path string) error { return unix.Mkfifo(path, 0600) },
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(directory, name)
			if err := prepare(path); err != nil {
				t.Fatal(err)
			}
			config, err := LoadAssistantConfig(path)
			if err == nil || config.Enabled {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}
