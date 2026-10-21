package adapters

import (
	"encoding/json"
	"github.com/giovantenne/nixorium/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantPrivateTokenAndIndependentConsent(t *testing.T) {
	store := AssistantStateStore{Directory: t.TempDir()}
	if err := os.Chmod(store.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	if token, err := store.ReadToken(); err != nil || token != "" {
		t.Fatalf("missing: %q %v", token, err)
	}
	token := strings.Repeat("t", 43)
	if err := store.WriteToken(token); err != nil {
		t.Fatal(err)
	}
	if saved, err := store.ReadToken(); err != nil || saved != token {
		t.Fatal("token did not round trip")
	}
	if accepted, err := store.ReadConsent(); err != nil || accepted {
		t.Fatal("activation inherited consent")
	}
	if err := store.WriteConsent(); err != nil {
		t.Fatal(err)
	}
	if accepted, err := store.ReadConsent(); err != nil || !accepted {
		t.Fatal("consent not saved")
	}
	for _, name := range []string{"token", "consent.json"} {
		info, err := os.Stat(filepath.Join(store.Directory, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("unsafe %s", name)
		}
	}
	if err := store.WriteToken(strings.Repeat("u", 43)); err != nil {
		t.Fatal(err)
	}
	if accepted, _ := store.ReadConsent(); accepted {
		t.Fatal("new activation reused old consent")
	}
	if err := store.DeleteToken(); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteToken(); err != nil {
		t.Fatal(err)
	}
	if saved, _ := store.ReadToken(); saved != "" {
		t.Fatal("token not deleted")
	}
	entries, _ := os.ReadDir(store.Directory)
	if len(entries) != 0 {
		t.Fatal("temporary state files left behind")
	}
}

func TestAssistantStateRefusesSymlinksAndUnsafePermissions(t *testing.T) {
	store := AssistantStateStore{Directory: t.TempDir()}
	if err := os.Chmod(store.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte(strings.Repeat("t", 43)), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory, "token")
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadToken(); err == nil {
		t.Fatal("read followed symlink")
	}
	if err := store.WriteToken(strings.Repeat("u", 43)); err == nil {
		t.Fatal("write replaced symlink")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("t", 43)), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadToken(); err == nil {
		t.Fatal("public token accepted")
	}
	if err := os.Chmod(store.Directory, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadConsent(); err == nil {
		t.Fatal("public directory accepted")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != strings.Repeat("t", 43) {
		t.Fatal("outside file changed")
	}
}

func TestAssistantSharingDefaultPersistenceAndConsentCompatibility(t *testing.T) {
	store := AssistantStateStore{Directory: t.TempDir()}
	os.Chmod(store.Directory, 0700)
	if share, err := store.ReadSharing(); err != nil || share {
		t.Fatal("sharing default is not false", err)
	}
	if err := store.WriteToken(strings.Repeat("t", 43)); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSharing(true); err == nil {
		t.Fatal("sharing accepted without ordinary consent")
	}
	if err := store.WriteConsent(); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(filepath.Join(store.Directory, "consent.json"))
	for _, share := range []bool{true, false, true} {
		if err := store.WriteSharing(share); err != nil {
			t.Fatal(err)
		}
		reopened := AssistantStateStore{Directory: store.Directory}
		if actual, err := reopened.ReadSharing(); err != nil || actual != share {
			t.Fatal("sharing not persisted", err)
		}
		var before, after domain.AssistantConsent
		json.Unmarshal(original, &before)
		data, _ := os.ReadFile(filepath.Join(store.Directory, "consent.json"))
		json.Unmarshal(data, &after)
		if before.AcceptedAt != after.AcceptedAt || before.TextVersion != after.TextVersion {
			t.Fatal("toggle changed ordinary consent")
		}
	}
	if consentTextVersionForTest := assistantConsentTextVersion; consentTextVersionForTest != 4 {
		t.Fatal("saving-by-default text must be version 4")
	}
	// A version 3 consent (sharing off) must be asked again once, never authorize saving.
	os.WriteFile(filepath.Join(store.Directory, "consent.json"), []byte(`{"schemaVersion":1,"acceptedAt":"2026-10-10T00:00:00Z","textVersion":3,"shareConversations":false}`), 0600)
	if accepted, err := store.ReadConsent(); err != nil || accepted {
		t.Fatal("version 3 consent still accepted", err)
	}
	if share, err := store.ReadSharing(); err != nil || share {
		t.Fatal("old consent allows saving before the new consent", err)
	}
	// Earlier disclosure must be accepted again before logs or history are shared.
	os.WriteFile(filepath.Join(store.Directory, "consent.json"), []byte(`{"schemaVersion":1,"acceptedAt":"2026-10-10T00:00:00Z","textVersion":1}`), 0600)
	if share, err := store.ReadSharing(); err != nil || share {
		t.Fatal("legacy consent enabled sharing", err)
	}
	if accepted, err := store.ReadConsent(); err != nil || accepted {
		t.Fatal("legacy consent authorized new disclosure", err)
	}
	if err := store.WriteConsentSharing(true); err != nil {
		t.Fatal(err)
	}
	if share, err := store.ReadSharing(); err != nil || !share {
		t.Fatal("checkbox choice lost", err)
	}
	if err := store.WriteToken(strings.Repeat("u", 43)); err != nil {
		t.Fatal(err)
	}
	if share, err := store.ReadSharing(); err != nil || share {
		t.Fatal("replacement activation inherited sharing", err)
	}
}
