package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestClassroomBrowserProfileKeepsSystemFrames(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	profile, err := classroomBrowserProfile()
	if err != nil {
		t.Fatal(err)
	}
	if profile != filepath.Join(state, "nixorium", "classroom-browser") {
		t.Fatalf("profile = %s", profile)
	}
	preferences := filepath.Join(profile, "Default", "Preferences")
	content, err := os.ReadFile(preferences)
	if err != nil || string(content) != classroomBrowserPreferences {
		t.Fatalf("preferences = %q, %v", content, err)
	}
	// Chromium owns the file afterwards; a later launch leaves it alone.
	if err := os.WriteFile(preferences, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := classroomBrowserProfile(); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(preferences); string(content) != "{}" {
		t.Fatalf("existing preferences were replaced: %q", content)
	}
	arguments := classroomBrowserArguments("http://127.0.0.1:1/open?token=x", profile)
	for _, want := range []string{"--user-data-dir=" + profile, "--ozone-platform=x11", "--app=http://127.0.0.1:1/open?token=x"} {
		if !slices.Contains(arguments, want) {
			t.Fatalf("arguments %v lack %s", arguments, want)
		}
	}
}
