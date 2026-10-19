package systemfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadablePaths(t *testing.T) {
	for _, path := range []string{"/etc/hosts", "/etc/ssh/sshd_config", "/etc/ssh/ssh_host_ed25519_key.pub", "/boot/grub/grub.cfg", "/proc/cmdline", "/var/lib/nixorium/identity.json", "/sys/class/net/eth0/address", "/etc"} {
		if !ValidRequest(path) || !Readable(path) {
			t.Errorf("%s should be readable", path)
		}
	}
	for _, path := range []string{
		"/etc/shadow", "/etc/gshadow-", "/etc/ssh/ssh_host_ed25519_key", "/etc/NetworkManager/system-connections/school.nmconnection",
		"/home/student/notes.txt", "/root/.ssh/id_ed25519", "/var/lib/nixorium/keys/harmonia-secret-key", "/var/lib/nixorium/home-reset/snapshot-1",
		"/var/lib/nixorium/remote-install/x.json", "/etc/nixos/secrets/db", "/etc/ssl/private/server.pem", "/sys/class/dmi/id/product_serial",
		"/var/lib/docker", "/nix/store/abc-lab-settings.json", "/", "/proc/self/environ",
	} {
		if Readable(path) {
			t.Errorf("%s should be refused", path)
		}
	}
	for _, path := range []string{"etc/hosts", "/etc/../home/student", "/etc//hosts", "/etc/./hosts", "/etc/hosts name", "/etc/$(id)", "/etc/hosts;rm"} {
		if ValidRequest(path) {
			t.Errorf("%q should be an invalid request", path)
		}
	}
}

func withRoots(t *testing.T, root string) {
	t.Helper()
	previousRoots, previousStore := readableRoots, storeRoot
	readableRoots, storeRoot = []string{root + "/etc", root + "/var/lib/nixorium"}, root+"/store"
	t.Cleanup(func() { readableRoots, storeRoot = previousRoots, previousStore })
}

func TestReadFollowsOnlySafeLinks(t *testing.T) {
	root := t.TempDir()
	withRoots(t, root)
	for _, directory := range []string{"etc/static", "store", "home/student", "var/lib/nixorium"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("store/hosts", "127.0.0.1 localhost\n")
	write("home/student/diary", "private\n")
	write("etc/grub.cfg", "set default=0\npassword_pbkdf2 admin grub.pbkdf2.sha512.10000.ABCDEF\nmenuentry \"NixOS\" {\n")
	if err := os.Symlink(filepath.Join(root, "store/hosts"), filepath.Join(root, "etc/hosts")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "home/student/diary"), filepath.Join(root, "etc/diary")); err != nil {
		t.Fatal(err)
	}

	result, err := Read(filepath.Join(root, "etc/hosts"))
	if err != nil || result.Kind != "file" || result.Content != "127.0.0.1 localhost\n" || result.Resolved == "" {
		t.Fatalf("store link: %+v, %v", result, err)
	}
	if _, err := Read(filepath.Join(root, "etc/diary")); err != ErrRefused {
		t.Fatalf("a link into a home folder was followed: %v", err)
	}
	grub, err := Read(filepath.Join(root, "etc/grub.cfg"))
	if err != nil || strings.Contains(grub.Content, "ABCDEF") || !strings.Contains(grub.Content, "password_pbkdf2 [withheld]") || !strings.Contains(grub.Content, `menuentry "NixOS"`) {
		t.Fatalf("grub: %q, %v", grub.Content, err)
	}
	listing, err := Read(filepath.Join(root, "etc"))
	if err != nil || listing.Kind != "directory" {
		t.Fatalf("listing: %+v, %v", listing, err)
	}
	names := []string{}
	for _, entry := range listing.Entries {
		names = append(names, entry.Name)
	}
	if strings.Join(names, ",") != "diary,grub.cfg,hosts,static" {
		t.Fatalf("entries = %v", names)
	}
}

func TestReadMarksBinaryAndTruncatesLargeFiles(t *testing.T) {
	root := t.TempDir()
	withRoots(t, root)
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/blob"), []byte{'a', 0, 'b'}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/large"), []byte(strings.Repeat("x", MaxFileBytes+10)), 0644); err != nil {
		t.Fatal(err)
	}
	if result, err := Read(filepath.Join(root, "etc/blob")); err != nil || !result.Binary || result.Content != "" {
		t.Fatalf("binary: %+v, %v", result, err)
	}
	if result, err := Read(filepath.Join(root, "etc/large")); err != nil || !result.Truncated || len(result.Content) != MaxFileBytes {
		t.Fatalf("large: truncated %v, %d bytes, %v", result.Truncated, len(result.Content), err)
	}
}

func TestDecodeRechecksTheAnswer(t *testing.T) {
	good, _ := Encode(Result{Path: "/etc/hosts", Kind: "file", Content: "x"})
	if _, err := Decode(good, "/etc/hosts"); err != nil {
		t.Fatal(err)
	}
	for name, answer := range map[string]Result{
		"other path":    {Path: "/etc/passwd", Kind: "file"},
		"refused link":  {Path: "/etc/hosts", Resolved: "/home/student/x", Kind: "file"},
		"refused entry": {Path: "/etc/hosts", Kind: "directory", Entries: []Entry{{Name: "shadow", Kind: "file"}}},
		"bad kind":      {Path: "/etc/hosts", Kind: "socket"},
	} {
		data, _ := Encode(answer)
		if _, err := Decode(data, "/etc/hosts"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := Decode([]byte(`{"path":"/etc/hosts","kind":"file","extra":1}`), "/etc/hosts"); err == nil {
		t.Error("unknown field accepted")
	}
}
