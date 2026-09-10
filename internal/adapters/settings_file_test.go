package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func adapterSettings() domain.LabSettingsFile {
	return domain.LabSettingsFile{
		SchemaVersion: domain.SettingsSchemaVersion,
		Lab: domain.LabSettings{
			MasterDHCPIP:     "192.0.2.10",
			NetworkBase:      "10.0.0.0",
			NetworkPrefix:    24,
			PCCount:          1,
			MasterHostNumber: 99,
			InterfaceName:    "eth0",
			TeacherUser:      "teacher",
			StudentUser:      "student",
			TeacherPassword:  "$6$salt$teacher",
			StudentPassword:  "$6$salt$student",
			AdminPassword:    "$6$salt$admin",
			HomepageURL:      "https://example.org",
			StudentGitName:   "Student",
			StudentGitEmail:  "student@example.org",
			AdminGitName:     "Admin",
			AdminGitEmail:    "admin@example.org",
			TimeZone:         "Europe/Rome",
			DefaultLocale:    "en_US.UTF-8",
			ExtraLocale:      "it_IT.UTF-8",
			KeyboardLayout:   "it",
			ConsoleKeyMap:    "it2",
			VeyonNativeHosts: []string{},
		},
	}
}

func TestWriteSettingsIsAtomicAndPrivate(t *testing.T) {
	directory := t.TempDir()
	local := Local{}
	if err := local.WriteSettings(directory, adapterSettings()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, settingsFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	data, err := local.ReadSettings(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, issues := domain.DecodeLabSettings(data); len(issues) != 0 {
		t.Fatalf("written settings have issues: %#v", issues)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".lab-settings.json.") {
			t.Fatalf("temporary file remained: %s", entry.Name())
		}
	}
}

func TestWriteSettingsRejectsSymlinkTarget(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, settingsFileName)); err != nil {
		t.Fatal(err)
	}
	if err := (Local{}).WriteSettings(directory, adapterSettings()); err == nil {
		t.Fatal("WriteSettings accepted a symlink target")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "untouched" {
		t.Fatalf("symlink target changed: %q", content)
	}
}
