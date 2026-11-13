package homereset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"golang.org/x/sys/unix"
)

func TestCommandsEndWhenTheyDoNotFinish(t *testing.T) {
	old := commandTimeout
	commandTimeout = 200 * time.Millisecond
	t.Cleanup(func() { commandTimeout = old })
	started := time.Now()
	err := command("sleep", "30")
	if err == nil || time.Since(started) > 5*time.Second {
		t.Fatalf("a command that never finishes returned %v after %v", err, time.Since(started))
	}
	if output, err := commandOutput("sh", "-c", "echo $LANG"); err != nil || strings.TrimSpace(string(output)) != "C" {
		t.Fatalf("output = %q, %v", output, err)
	}
}

func resetVMConfig(t *testing.T) Config {
	t.Helper()
	if os.Getenv("NIXORIUM_HOME_RESET_VM_TEST") != "1" {
		t.Skip("reset lifecycle requires the disposable Btrfs VM")
	}
	data, err := os.ReadFile("/etc/home-reset-test.json")
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := decodeStrict(data, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestResetLifecycleVM(t *testing.T) {
	config := resetVMConfig(t)
	home := "/home/" + config.User
	writeFixture(t, home+"/lesson.txt")
	writeFixture(t, "/home/teacher/keep")
	writeFixture(t, "/home/admin/keep")
	writeFixture(t, "/outside-home/sentinel")
	intactHome := func(t *testing.T) {
		t.Helper()
		intact(t, home+"/lesson.txt")
		intact(t, "/outside-home/sentinel")
		intact(t, "/home/teacher/keep")
		intact(t, "/home/admin/keep")
		if _, err := os.Lstat(resetState + "/pending.json"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("preflight created a pending record", err)
		}
	}
	rejected := func(t *testing.T, candidate Config, expected string) {
		t.Helper()
		if err := Reset(candidate); err == nil {
			t.Fatal("unsafe reset accepted")
		} else if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %q, got %v", expected, err)
		} else {
			t.Log("expected preflight refusal:", err)
		}
		intactHome(t)
	}
	t.Run("active-login-services", func(t *testing.T) { rejected(t, config, "login barrier is not held") })
	if err := command(config.Systemctl, "stop", "systemd-user-sessions.service"); err != nil {
		t.Fatal(err)
	}
	t.Run("invalid-inputs", func(t *testing.T) {
		for _, test := range []struct {
			mutate   func(*Config)
			expected string
		}{
			{func(c *Config) { c.User = "admin" }, "invalid managed account"},
			{func(c *Config) { c.EphemeralPaths = []string{".first", "../outside-home"} }, "canonical relative paths"},
			{func(c *Config) { c.Seed = "/var/lib/home-template/student" }, "direct store output"},
			{func(c *Config) { c.Btrfs = "/run/current-system/sw/bin/btrfs" }, "canonical immutable store path"},
			{func(c *Config) { c.Seed = os.Getenv("NIXORIUM_BAD_SEED_LINK") }, "undeclared seed link"},
			{func(c *Config) { c.Seed = os.Getenv("NIXORIUM_BAD_SEED_PROFILE") }, "invalid seed profile"},
		} {
			candidate := config
			test.mutate(&candidate)
			rejected(t, candidate, test.expected)
		}
	})
	t.Run("wrong-ownership", func(t *testing.T) {
		if err := os.Chown(home, 0, 100); err != nil {
			t.Fatal(err)
		}
		rejected(t, config, "unexpected home ownership")
		if err := os.Chown(home, 2000, 100); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("active-student-process", func(t *testing.T) {
		process := exec.Command("/run/current-system/sw/bin/sleep", "100")
		process.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 2000, Gid: 100}}
		if err := process.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = process.Process.Kill(); _ = process.Wait() }()
		// The kernel truncates the process name, so match only the PID.
		rejected(t, config, fmt.Sprintf("still has running processes (pid %d, ", process.Process.Pid))
	})
	t.Run("symlink-parent", func(t *testing.T) {
		linkFixture(t, "/outside-home", home+"/.config")
		rejected(t, config, "inspect cleanup parent")
		if err := os.Remove(home + "/.config"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("nested-mount-outside-exclusions", func(t *testing.T) {
		if err := os.Mkdir(home+"/mounted", 0700); err != nil {
			t.Fatal(err)
		}
		if err := unix.Mount("/outside-home", home+"/mounted", "", unix.MS_BIND, ""); err != nil {
			t.Fatal(err)
		}
		rejected(t, config, "nested mount")
		if err := unix.Unmount(home+"/mounted", 0); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("nested-subvolume-outside-exclusions", func(t *testing.T) {
		if err := command(config.Btrfs, "subvolume", "create", home+"/nested"); err != nil {
			t.Fatal(err)
		}
		rejected(t, config, "nested")
		if err := command(config.Btrfs, "subvolume", "delete", home+"/nested"); err != nil {
			t.Fatal(err)
		}
	})
	before, err := mountedSubvolume(home, "/@home-"+config.User)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("preparation-failure-discards-its-work", func(t *testing.T) {
		candidate := config
		candidate.Dconf = os.Getenv("NIXORIUM_FAILING_DCONF")
		if err := Reset(candidate); err == nil || !strings.Contains(err.Error(), "dconf failed") {
			t.Fatal("preparation failure not reported", err)
		}
		// Nothing changed the home, so the failure must not block later boots.
		for _, name := range []string{"prepared", "keyfiles", "previous", "pending.new", "recovery.json"} {
			if _, err := os.Lstat(resetState + "/" + name); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("preparation failure left", name, err)
			}
		}
		intactHome(t)
	})
	t.Run("invalid-existing-snapshot", func(t *testing.T) {
		if err := os.Mkdir(publishedSnapshots+"/snapshot-1", 0750); err != nil {
			t.Fatal(err)
		}
		rejected(t, config, "invalid managed snapshot subvolume")
		if err := os.Remove(publishedSnapshots + "/snapshot-1"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("successful-reset-and-rotation", func(t *testing.T) {
		for iteration := 0; iteration < 7; iteration++ {
			writeFixture(t, home+"/.config/opencode/auth.json")
			writeFixture(t, home+"/.local/npm/private-token")
			writeFixture(t, home+"/lesson.txt")
			linkFixture(t, "/outside-home", home+"/external-link")
			if err := Reset(config); err != nil {
				t.Fatal(err)
			}
			intact(t, publishedSnapshots+"/snapshot-1/lesson.txt")
			for _, excluded := range []string{".config/opencode", ".local/npm"} {
				if _, err := os.Lstat(publishedSnapshots + "/snapshot-1/" + excluded); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("ephemeral data in snapshot", excluded, err)
				}
			}
			if err := os.WriteFile(publishedSnapshots+"/snapshot-1/no-write", []byte("no"), 0600); !errors.Is(err, syscall.EROFS) {
				t.Fatal("snapshot is not read-only", err)
			}
			intact(t, "/outside-home/sentinel")
			intact(t, "/home/teacher/keep")
			intact(t, "/home/admin/keep")
			after, err := mountedSubvolume(home, "/@home-"+config.User)
			if err != nil || after.Subvol != before.Subvol || after.Mnt_id != before.Mnt_id {
				t.Fatal("mounted home was replaced", err)
			}
			if err := filepath.WalkDir(home, func(name string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if stat := info.Sys().(*syscall.Stat_t); stat.Uid != 2000 || stat.Gid != 100 {
					t.Errorf("wrong ownership: %s", name)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			settings, err := os.ReadFile(home + "/.config/Code/User/settings.json")
			if err != nil || !strings.Contains(string(settings), `"editor.fontSize":15`) {
				t.Fatal("profile not restored", string(settings), err)
			}
			for key, expected := range map[string]string{
				"/org/gnome/desktop/interface/color-scheme":              "'prefer-dark'",
				"/org/gnome/shell/extensions/dash-to-dock/dock-position": "'LEFT'",
				"/org/gnome/desktop/background/picture-uri":              "'file://" + config.Wallpapers[0] + "'",
			} {
				read := exec.Command(config.Dconf, "read", key)
				read.Env = []string{"XDG_CONFIG_HOME=" + home + "/.config"}
				output, err := read.Output()
				if err != nil || strings.TrimSpace(string(output)) != expected {
					t.Fatalf("dconf %s: %q, %v", key, output, err)
				}
			}
			// Prove editability with the actual student credentials, without
			// opening a login session behind the deliberately held login barrier.
			edit := exec.Command("/run/current-system/sw/bin/sh", "-c", `printf '%s' '{"editor.fontSize":25}' > "$1"`, "edit-settings", home+"/.config/Code/User/settings.json")
			edit.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 2000, Gid: 100}}
			if err := edit.Run(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(resetState + "/pending.json"); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("successful reset left pending evidence", err)
			}
			data, err := os.ReadFile(resetState + "/success.json")
			if err != nil {
				t.Fatal(err)
			}
			var receipt map[string]any
			if err := json.Unmarshal(data, &receipt); err != nil || receipt["seed"] != config.Seed || receipt["user"] != config.User || receipt["bootID"] == "" {
				t.Fatal("invalid reset receipt", err)
			}
		}
		entries, err := os.ReadDir(publishedSnapshots)
		if err != nil || len(entries) != 5 {
			t.Fatal("snapshot retention differs from five", err)
		}
		for i, entry := range entries {
			if entry.Name() != "snapshot-"+strconv.Itoa(i+1) {
				t.Fatal("unexpected snapshot", entry.Name())
			}
		}
	})
	snapshotCount := func(t *testing.T) int {
		t.Helper()
		entries, err := os.ReadDir(publishedSnapshots)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	recovered := func(t *testing.T) {
		t.Helper()
		if err := Reset(config); err != nil {
			t.Fatal("interrupted reset not recovered", err)
		}
		entries, err := os.ReadDir(resetState)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Name() != "lock" && entry.Name() != "success.json" {
				t.Fatal("recovery left", entry.Name())
			}
		}
		settings, err := os.ReadFile(home + "/.config/Code/User/settings.json")
		if err != nil || !strings.Contains(string(settings), `"editor.fontSize":15`) {
			t.Fatal("profile not restored", string(settings), err)
		}
	}
	legacyPending := func(t *testing.T) {
		t.Helper()
		record := `{"schemaVersion":1,"seed":` + strconv.Quote(config.Seed) + `,"user":` + strconv.Quote(config.User) + "}\n"
		if err := os.WriteFile(resetState+"/pending.json", []byte(record), 0600); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"prepared", "keyfiles"} {
			if err := os.Mkdir(resetState+"/"+name, 0700); err != nil {
				t.Fatal(err)
			}
			writeFixture(t, resetState+"/"+name+"/partial")
		}
	}
	staff, err := user.LookupGroup("nixorium-staff")
	if err != nil {
		t.Fatal(err)
	}
	staffGID, err := strconv.Atoi(staff.Gid)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce a retained snapshot exactly as the reset leaves it before the
	// restore phase: sanitized permissions, then read-only.
	retainHome := func(t *testing.T) {
		t.Helper()
		if err := command(config.Btrfs, "subvolume", "snapshot", home, resetState+"/previous"); err != nil {
			t.Fatal(err)
		}
		if err := snapshotPermissions(resetState+"/previous", staffGID); err != nil {
			t.Fatal(err)
		}
		if err := command(config.Btrfs, "property", "set", "-ts", resetState+"/previous", "ro", "true"); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("interrupted-preparation-restarts", func(t *testing.T) {
		// An older reset interrupted before its snapshot: the home is untouched.
		writeFixture(t, home+"/before-preparation.txt")
		legacyPending(t)
		before := snapshotCount(t)
		recovered(t)
		intact(t, publishedSnapshots+"/snapshot-1/before-preparation.txt")
		if after := snapshotCount(t); after != before && after != before+1 {
			t.Fatal("unexpected snapshot count", before, after)
		}
	})
	t.Run("interrupted-restore-completes-from-retained-snapshot", func(t *testing.T) {
		// An older reset interrupted after its read-only snapshot, with the
		// home partly removed: the snapshot is the only copy of the lesson.
		writeFixture(t, home+"/retained-lesson.txt")
		legacyPending(t)
		retainHome(t)
		if err := os.Remove(home + "/retained-lesson.txt"); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, home+"/partially-copied")
		recovered(t)
		intact(t, publishedSnapshots+"/snapshot-1/retained-lesson.txt")
		if _, err := os.Lstat(home + "/partially-copied"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("partial restore survived recovery", err)
		}
	})
	t.Run("interrupted-publication-is-not-repeated", func(t *testing.T) {
		writeFixture(t, home+"/published-lesson.txt")
		record, err := pendingJSON(config, phaseRestore)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(resetState+"/pending.json", record, 0600); err != nil {
			t.Fatal(err)
		}
		retainHome(t)
		if err := publishSnapshot(config, staffGID); err != nil {
			t.Fatal(err)
		}
		// Interrupt a second rotation halfway: slot 3 is free, 1 and 2 remain.
		if err := os.Rename(publishedSnapshots+"/snapshot-3", publishedSnapshots+"/snapshot-3-moved"); err != nil {
			t.Fatal(err)
		}
		if err := command(config.Btrfs, "subvolume", "delete", publishedSnapshots+"/snapshot-3-moved"); err != nil {
			t.Fatal(err)
		}
		before := snapshotCount(t)
		recovered(t)
		if after := snapshotCount(t); after != before {
			t.Fatal("publication repeated after interruption", before, after)
		}
		intact(t, publishedSnapshots+"/snapshot-1/published-lesson.txt")
		// The next rotation fills the free slot instead of deleting the oldest.
		if err := Reset(config); err != nil {
			t.Fatal(err)
		}
		if after := snapshotCount(t); after != before+1 {
			t.Fatal("rotation discarded a snapshot while a slot was free", before, after)
		}
	})
	t.Run("configured-retention", func(t *testing.T) {
		keep := func(count int) Config {
			candidate := config
			candidate.Snapshots = &count
			return candidate
		}
		names := func(t *testing.T, expected int) {
			t.Helper()
			entries, err := os.ReadDir(publishedSnapshots)
			if err != nil || len(entries) != expected {
				t.Fatal("unexpected snapshot count", len(entries), expected, err)
			}
			for i, entry := range entries {
				if entry.Name() != "snapshot-"+strconv.Itoa(i+1) {
					t.Fatal("unexpected snapshot", entry.Name())
				}
			}
		}
		for snapshotCount(t) < 5 {
			if err := Reset(config); err != nil {
				t.Fatal(err)
			}
		}
		// A lower number deletes the oldest snapshots at the next reset.
		writeFixture(t, home+"/three-lesson.txt")
		if err := Reset(keep(3)); err != nil {
			t.Fatal(err)
		}
		names(t, 3)
		intact(t, publishedSnapshots+"/snapshot-1/three-lesson.txt")
		// Zero publishes nothing, removes every snapshot and the retained copy.
		if err := Reset(keep(0)); err != nil {
			t.Fatal(err)
		}
		names(t, 0)
		if _, err := os.Lstat(resetState + "/previous"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("retained snapshot kept with no snapshots configured", err)
		}
		if err := Reset(keep(0)); err != nil {
			t.Fatal(err)
		}
		names(t, 0)
		// An attempt interrupted after publication with none kept completes under
		// a positive number by publishing its retained copy instead of blocking.
		writeFixture(t, home+"/zero-lesson.txt")
		record, err := pendingJSON(config, phasePublished)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(resetState+"/pending.json", record, 0600); err != nil {
			t.Fatal(err)
		}
		retainHome(t)
		recovered(t)
		names(t, 1)
		intact(t, publishedSnapshots+"/snapshot-1/zero-lesson.txt")
		for _, count := range []int{2, 3} {
			if err := Reset(config); err != nil {
				t.Fatal(err)
			}
			names(t, count)
		}
		// A value outside 0-30 or a missing one changes nothing.
		for _, candidate := range []Config{keep(-1), keep(31), func() Config { c := config; c.Snapshots = nil; return c }()} {
			if err := Reset(candidate); err == nil || !strings.Contains(err.Error(), "snapshot count") {
				t.Fatal("invalid snapshot count accepted", err)
			}
			names(t, 3)
		}
	})
	t.Run("file-recovery-writes-as-the-student", func(t *testing.T) {
		writeFixture(t, home+"/Documents/essay.txt")
		writeFixture(t, home+"/Desktop/Project/main.c")
		writeFixture(t, home+"/Desktop/Project/.git/config")
		linkFixture(t, "/etc/shadow", home+"/Desktop/Project/shadow")
		if err := Reset(config); err != nil {
			t.Fatal(err)
		}
		sessions, err := recoverySessions(publishedSnapshots)
		if err != nil || len(sessions) == 0 || sessions[0].Number != 1 || sessions[0].SavedAt.IsZero() || time.Since(sessions[0].SavedAt) > time.Hour {
			t.Fatal("unexpected saved sessions", sessions, err)
		}
		root, err := sessionDirectory(publishedSnapshots, 1)
		if err != nil {
			t.Fatal(err)
		}
		listing, err := recoveryList(root, 1, "Desktop/Project")
		if err != nil || len(listing.Entries) != 1 || listing.Entries[0].Name != "main.c" {
			t.Fatal("unexpected listing", listing, err)
		}
		restored, err := recoverAsStudent(config, root, []string{"Documents/essay.txt", "Desktop/Project"})
		if err != nil {
			t.Fatal(err)
		}
		desktop, err := classroomview.DesktopDirectory(home)
		if err != nil {
			t.Fatal(err)
		}
		folder := filepath.Join(desktop, "Recovered files")
		if restored.Files != 2 || strings.Join(restored.Placed, ",") != "essay.txt,Project" {
			t.Fatal("unexpected recovery", restored)
		}
		intact(t, folder+"/essay.txt")
		intact(t, folder+"/Project/main.c")
		for _, absent := range []string{"Project/.git", "Project/shadow"} {
			if _, err := os.Lstat(folder + "/" + absent); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("hidden file or link recovered", absent, err)
			}
		}
		for name, mode := range map[string]os.FileMode{folder: 0755, folder + "/Project": 0755, folder + "/essay.txt": 0644, folder + "/Project/main.c": 0644} {
			info, err := os.Lstat(name)
			if err != nil {
				t.Fatal(err)
			}
			if stat := info.Sys().(*syscall.Stat_t); stat.Uid != 2000 || stat.Gid != 100 || info.Mode().Perm() != mode {
				t.Fatalf("%s: uid %d gid %d mode %v", name, stat.Uid, stat.Gid, info.Mode().Perm())
			}
		}
		// A second recovery never replaces the first.
		if restored, err := recoverAsStudent(config, root, []string{"Documents/essay.txt"}); err != nil || restored.Placed[0] != "essay (2).txt" {
			t.Fatal("second recovery", restored, err)
		}
		intact(t, folder+"/essay (2).txt")
		// The student cannot redirect the copy: a desktop folder replaced by a
		// link to a root-owned folder makes the write fail as the student.
		if err := os.RemoveAll(folder); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/etc", folder); err != nil {
			t.Fatal(err)
		}
		if _, err := recoverAsStudent(config, root, []string{"Documents/essay.txt"}); err == nil {
			t.Fatal("recovery wrote through a link to a root-owned folder")
		}
		if _, err := os.Lstat("/etc/essay.txt"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("recovery wrote into /etc", err)
		}
		if err := os.Remove(folder); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("failed-recovery-blocks-retry", func(t *testing.T) {
		writeFixture(t, home+"/last-lesson.txt")
		failure := errors.New("injected copy failure")
		failingRestore := func(source, destination string, uid, gid int) error {
			writeFixture(t, destination+"/partially-copied")
			return failure
		}
		if err := runReset(config, failingRestore); !errors.Is(err, failure) {
			t.Fatal("injected failure not reported", err)
		}
		intact(t, resetState+"/previous/last-lesson.txt")
		// The next boot makes one automatic attempt; it fails again.
		if err := runReset(config, failingRestore); !errors.Is(err, failure) {
			t.Fatal("automatic recovery not attempted", err)
		}
		if _, err := os.Stat(resetState + "/recovery.json"); err != nil {
			t.Fatal("recovery attempt not recorded", err)
		}
		if err := Reset(config); err == nil || !strings.Contains(err.Error(), "already failed") {
			t.Fatal("failed recovery was retried", err)
		}
		intact(t, home+"/partially-copied")
		intact(t, resetState+"/previous/last-lesson.txt")
	})
}

func TestResetFailureSurvivesRebootVM(t *testing.T) {
	if os.Getenv("NIXORIUM_HOME_RESET_REBOOT_TEST") != "1" {
		t.Skip("separate post-reboot VM phase")
	}
	config := resetVMConfig(t)
	if err := command(config.Systemctl, "stop", "systemd-user-sessions.service"); err != nil {
		t.Fatal(err)
	}
	if err := Reset(config); err == nil || !strings.Contains(err.Error(), "already failed") {
		t.Fatal("failed recovery was retried after reboot", err)
	}
	intact(t, "/home/"+config.User+"/partially-copied")
	intact(t, resetState+"/previous/last-lesson.txt")
}

func TestCleanHomePreflight(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, home+"/keep-until-removal")
	writeFixture(t, outside+"/sentinel")
	linkFixture(t, outside, home+"/outside")
	if err := cleanHome(home, false); err != nil {
		t.Fatal(err)
	}
	intact(t, home+"/keep-until-removal")
	if err := cleanHome(home, true); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatal("home root not preserved empty", err)
	}
	intact(t, outside+"/sentinel")
}

func TestSnapshotNames(t *testing.T) {
	for name, expected := range map[string]int{"snapshot-1": 1, "snapshot-9": 9, "snapshot-10": 10, "snapshot-30": 30} {
		if number, valid := snapshotNumber(name); !valid || number != expected {
			t.Fatal("valid snapshot name refused", name, number)
		}
	}
	for _, name := range []string{"snapshot-0", "snapshot-01", "snapshot-31", "snapshot-100", "snapshot-", "snapshot-1a", "snapshot--1", "snapshot-3-moved", "other"} {
		if _, valid := snapshotNumber(name); valid {
			t.Fatal("unexpected snapshot name accepted", name)
		}
	}
}

func TestSnapshotLimit(t *testing.T) {
	for _, count := range []int{0, 5, 30} {
		if got, err := snapshotLimit(Config{Snapshots: &count}); err != nil || got != count {
			t.Fatal("valid snapshot count refused", count, err)
		}
	}
	for _, count := range []int{-1, 31} {
		if _, err := snapshotLimit(Config{Snapshots: &count}); err == nil {
			t.Fatal("invalid snapshot count accepted", count)
		}
	}
	if _, err := snapshotLimit(Config{}); err == nil {
		t.Fatal("missing snapshot count accepted")
	}
}
