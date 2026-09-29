package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSupportSSHDoesNotEnrollTrustOrRunUserConfiguration(t *testing.T) {
	args := strings.Join(supportSSHArguments(domain.HostMeta{IP: "192.0.2.83"}, time.Second), " ")
	for _, required := range []string{"-F /dev/null", "StrictHostKeyChecking=yes", "UpdateHostKeys=no", "ControlPath=none", "PermitLocalCommand=no", "ProxyCommand=none", "IdentityAgent=none", "root@192.0.2.83 nixorium-host-state"} {
		if !strings.Contains(args, required) {
			t.Fatalf("missing %s", required)
		}
	}
	if strings.Contains(args, "accept-new") {
		t.Fatal("diagnostics can change SSH trust")
	}
}

func supportTestSnapshot(t *testing.T) domain.SupportSnapshot {
	t.Helper()
	snapshot, err := domain.NewSupportSnapshot(domain.SupportInput{Version: "2.0.0", Collected: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestSupportExportPrivateExactAndNeverOverwrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	snapshot := supportTestSnapshot(t)
	for range 2 {
		result := writeSupport(context.Background(), root, snapshot, nil)
		if result.State != "saved" {
			t.Fatalf("export: %+v", result)
		}
		data, err := os.ReadFile(result.Path)
		if err != nil || string(data) != snapshot.JSON() || result.SHA256 != snapshot.Digest() {
			t.Fatalf("different saved content: %v", err)
		}
		info, err := os.Stat(result.Path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("unsafe file mode")
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "nixorium", "support"))
	if err != nil || len(entries) != 2 {
		t.Fatal("export replaced an earlier report")
	}
	for _, directory := range []string{root, filepath.Join(root, "nixorium"), filepath.Join(root, "nixorium", "support")} {
		info, err := os.Stat(directory)
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatal("unsafe directory mode")
		}
	}
}

func TestSupportExportRefusesSymlinksAndUnsafeDirectories(t *testing.T) {
	for _, component := range []string{"state", "state/nixorium", "state/nixorium/support"} {
		t.Run(component, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			path := filepath.Join(root, component)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
			result := writeSupport(context.Background(), filepath.Join(root, "state"), supportTestSnapshot(t), nil)
			if result.State != "failed" || result.Path != "" {
				t.Fatal("followed a symlink")
			}
			entries, _ := os.ReadDir(outside)
			if len(entries) != 0 {
				t.Fatal("wrote outside destination")
			}
		})
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nixorium"), 0755); err != nil {
		t.Fatal(err)
	}
	if result := writeSupport(context.Background(), root, supportTestSnapshot(t), nil); result.State != "failed" {
		t.Fatal("accepted public product directory")
	}
}

func TestSupportExportCancellationAndDurability(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := writeSupport(ctx, root, supportTestSnapshot(t), nil); result.State != "failed" {
		t.Fatal("cancelled write accepted")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("cancelled write created directories")
	}
	result := writeSupport(context.Background(), root, supportTestSnapshot(t), func(*os.File) error { return errors.New("SECRET storage error") })
	if result.State != "partial" || result.Path == "" {
		t.Fatal("durability failure hidden")
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatal("published report not retained")
	}
}

func TestSupportExportDetectsRenamedDirectory(t *testing.T) {
	root := t.TempDir()
	result := writeSupport(context.Background(), root, supportTestSnapshot(t), func(file *os.File) error {
		return os.Rename(file.Name(), file.Name()+"-moved")
	})
	if result.State != "partial" {
		t.Fatal("renamed destination claimed verified publication")
	}
}
