package homereset

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func writeFixture(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
}

func intact(t *testing.T, name string) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil || string(data) != "sentinel" {
		t.Fatalf("sentinel changed: %s, %q, %v", name, data, err)
	}
}

func linkFixture(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveEphemeral(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, filepath.Join(outside, "keep"))
	writeFixture(t, filepath.Join(home, ".cache/tool/state"))
	writeFixture(t, filepath.Join(home, "lesson.txt"))
	linkFixture(t, outside, filepath.Join(home, ".cache/tool/link"))
	linkFixture(t, filepath.Join(outside, "absent"), filepath.Join(home, "dangling"))
	if err := os.Link(filepath.Join(outside, "keep"), filepath.Join(home, ".cache/tool/hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(home, ".cache/tool/pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEphemeral(home, []string{".cache/tool", "dangling", ".missing/tool"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".cache/tool", "dangling"} {
		if _, err := os.Lstat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatalf("path was not removed: %s, %v", name, err)
		}
	}
	intact(t, filepath.Join(home, "lesson.txt"))
	intact(t, filepath.Join(outside, "keep"))
	if _, err := os.Stat(home); err != nil {
		t.Fatal("home root was removed:", err)
	}
}

func TestRemoveEphemeralValidatesAllPathsBeforeRemoval(t *testing.T) {
	cases := [][]string{
		{".first", ""}, {".first", "."}, {".first", "../outside"},
		{".first", "/outside"}, {".first", "a/../b"}, {".first", "a/./b"},
		{".first", "a//b"}, {".first", "a/"}, {".first", "a\x00b"},
		{".first", "a\nb"}, {".first", "a\tb"}, {".first", "a\\b"},
		{".first", ".first"}, {".first", ".first/nested"},
		{".first/nested", ".first"}, {".first", strings.Repeat("x", 4097)},
	}
	for _, paths := range cases {
		t.Run(strings.Join(paths, ","), func(t *testing.T) {
			home := t.TempDir()
			writeFixture(t, filepath.Join(home, ".first/nested"))
			if err := RemoveEphemeral(home, paths); err == nil {
				t.Fatal("invalid list was accepted")
			}
			intact(t, filepath.Join(home, ".first/nested"))
		})
	}
	if err := validatePaths(make([]string, 129)); err == nil {
		t.Fatal("unbounded list was accepted")
	}
}

func TestRemoveEphemeralRejectsSymlinkParentsBeforeRemoval(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, filepath.Join(home, ".first/state"))
	writeFixture(t, filepath.Join(outside, "tool/state"))
	linkFixture(t, outside, filepath.Join(home, ".config"))
	if err := RemoveEphemeral(home, []string{".first", ".config/tool"}); err == nil {
		t.Fatal("symlink parent was accepted")
	}
	intact(t, filepath.Join(home, ".first/state"))
	intact(t, filepath.Join(outside, "tool/state"))
	// Selecting the symlink itself may unlink it without traversing its target.
	if err := RemoveEphemeral(home, []string{".config"}); err != nil {
		t.Fatal(err)
	}
	intact(t, filepath.Join(outside, "tool/state"))
}

func TestRemoveEphemeralRejectsUnsafeRoots(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "real")
	writeFixture(t, filepath.Join(home, ".first/state"))
	linkFixture(t, home, filepath.Join(parent, "link"))
	for _, root := range []string{"/", "/home", ".", "relative", home + "/", filepath.Join(parent, "link")} {
		if err := RemoveEphemeral(root, []string{".first"}); err == nil {
			t.Fatalf("unsafe root accepted: %q", root)
		}
	}
	// A symlink in an ancestor is unsafe even when the final directory is real.
	if err := RemoveEphemeral(filepath.Join(parent, "link/.first"), []string{"state"}); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	intact(t, filepath.Join(home, ".first/state"))
}

func TestRemoveEphemeralRejectsDeepTreesBeforeRemoval(t *testing.T) {
	home := t.TempDir()
	deep := filepath.Join(home, strings.Repeat("d/", 130), "state")
	writeFixture(t, deep)
	writeFixture(t, filepath.Join(home, ".first/state"))
	if err := RemoveEphemeral(home, []string{".first", "d"}); err == nil {
		t.Fatal("unbounded recursion accepted")
	}
	intact(t, deep)
	intact(t, filepath.Join(home, ".first/state"))
}

func TestRemoveEphemeralMountsVM(t *testing.T) {
	if os.Getenv("NIXORIUM_HOME_RESET_VM_TEST") != "1" {
		t.Skip("mount cases run only in the disposable home-reset VM")
	}
	if os.Geteuid() != 0 {
		t.Fatal("VM mount check requires its root test process")
	}
	for _, kind := range []string{"directory", "file", "parent"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			outside := t.TempDir()
			writeFixture(t, filepath.Join(home, ".first/state"))
			writeFixture(t, filepath.Join(home, ".cache/tool/state"))
			writeFixture(t, filepath.Join(outside, "sentinel"))
			source, target := outside, filepath.Join(home, ".cache/tool/mounted")
			selected := []string{".first", ".cache"}
			if kind == "file" {
				source = filepath.Join(outside, "sentinel")
				writeFixture(t, target)
			} else {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if kind == "parent" {
					selected = []string{".first", ".cache/tool/mounted/sentinel"}
				}
			}
			// A bind mount of the same filesystem defeats device-number-only
			// checks. Both file and directory mount identities must be checked.
			if err := unix.Mount(source, target, "", unix.MS_BIND, ""); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := unix.Unmount(target, 0); err != nil {
					t.Error(err)
				}
			})
			if err := RemoveEphemeral(home, selected); err == nil {
				t.Fatal("nested bind mount was accepted")
			}
			intact(t, filepath.Join(home, ".first/state"))
			intact(t, filepath.Join(home, ".cache/tool/state"))
			intact(t, filepath.Join(outside, "sentinel"))
		})
	}
}

func TestRemoveEphemeralSubvolumesVM(t *testing.T) {
	if os.Getenv("NIXORIUM_HOME_RESET_VM_TEST") != "1" {
		t.Skip("Btrfs case runs only in the disposable home-reset VM")
	}
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".first/state"))
	subvolume := filepath.Join(home, "nested")
	if output, err := exec.Command("btrfs", "subvolume", "create", subvolume).CombinedOutput(); err != nil {
		t.Fatalf("create test subvolume: %v, %s", err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.Command("btrfs", "subvolume", "delete", subvolume).CombinedOutput(); err != nil {
			t.Errorf("remove test subvolume: %v, %s", err, output)
		}
	})
	writeFixture(t, filepath.Join(subvolume, "state"))
	for _, selected := range [][]string{{".first", "nested"}, {".first", "nested/state"}} {
		if err := RemoveEphemeral(home, selected); err == nil {
			t.Fatal("nested Btrfs subvolume accepted")
		}
		intact(t, filepath.Join(home, ".first/state"))
		intact(t, filepath.Join(subvolume, "state"))
	}
}
