package domain

import (
	"strings"
	"testing"
)

func TestDiffLabSettingsIsOrderedAndRedactsPasswordHashes(t *testing.T) {
	before := validSettings()
	after := before
	after.Lab.NetworkBase = "10.1.0.0"
	after.Lab.TeacherPassword = "$6$new$privatehash"
	changes := DiffLabSettings(before, after)
	if len(changes) != 2 || changes[0].Field != "lab.networkBase" || changes[1].Field != "lab.teacherPassword" || !changes[1].Sensitive {
		t.Fatalf("changes = %+v", changes)
	}
	for _, change := range changes {
		if strings.Contains(change.Before.(string), "$6$") || strings.Contains(change.After.(string), "$6$") {
			t.Fatalf("change leaked password hash: %+v", change)
		}
	}
}

func TestSettingsFingerprintIsStableAndContentSensitive(t *testing.T) {
	first := SettingsFingerprint([]byte("one"))
	if first != SettingsFingerprint([]byte("one")) || first == SettingsFingerprint([]byte("two")) || !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("unexpected fingerprints: %q", first)
	}
}
