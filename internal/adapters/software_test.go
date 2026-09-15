package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSoftwareWriterIsAtomicAndConflictAware(t *testing.T) {
	repository := t.TempDir()
	base := []byte("{\n  \"schemaVersion\": 1,\n  \"packages\": []\n}\n")
	path := filepath.Join(repository, softwareFileName)
	if err := os.WriteFile(path, base, 0600); err != nil {
		t.Fatal(err)
	}
	candidate := domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}}}
	if err := (Local{}).WriteSoftwareIfUnchanged(repository, []byte("stale"), candidate); !errors.Is(err, domain.ErrSoftwareConflict) {
		t.Fatalf("stale write error = %v", err)
	}
	if err := (Local{}).WriteSoftwareIfUnchanged(repository, base, candidate); err != nil {
		t.Fatal(err)
	}
	content, err := (Local{}).ReadSoftware(repository)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := domain.DecodeLabSoftware(content)
	if err != nil || len(stored.Packages) != 1 || stored.Packages[0].Package != "vlc" {
		t.Fatalf("stored=%+v error=%v", stored, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("software file mode=%v error=%v", info.Mode().Perm(), err)
	}
	if drafts, _ := filepath.Glob(filepath.Join(repository, ".lab-software.json.*")); len(drafts) != 0 {
		t.Fatalf("drafts remain: %v", drafts)
	}
}

func TestSoftwareWriterRejectsSymlink(t *testing.T) {
	repository := t.TempDir()
	path := filepath.Join(repository, softwareFileName)
	if err := os.Symlink("/etc/passwd", path); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).ReadSoftware(repository); err == nil {
		t.Fatal("software reader followed a symlink")
	}
	candidate := domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{}}
	if err := (Local{}).WriteSoftwareIfUnchanged(repository, []byte("anything"), candidate); err == nil {
		t.Fatal("software writer accepted a symlink")
	}
}
