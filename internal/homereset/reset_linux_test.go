package homereset

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

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
		rejected(t, config, "still has running processes")
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
	t.Run("preparation-failure-preserves-home", func(t *testing.T) {
		candidate := config
		candidate.Dconf = os.Getenv("NIXORIUM_FAILING_DCONF")
		if err := Reset(candidate); err == nil || !strings.Contains(err.Error(), "dconf failed") {
			t.Fatal("preparation failure not reported", err)
		}
		intact(t, home+"/lesson.txt")
		if _, err := os.Stat(resetState + "/pending.json"); err != nil {
			t.Fatal("preparation failure evidence missing", err)
		}
		if err := Reset(config); err == nil || !strings.Contains(err.Error(), "administrator recovery") {
			t.Fatal("preparation failure retried", err)
		}
		// Fixture-only recovery: no home mutation or snapshot has happened.
		// Production deliberately offers no automatic evidence-removal command.
		for _, directory := range []string{resetState + "/prepared", resetState + "/keyfiles"} {
			if err := cleanHome(directory, true); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(directory); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Remove(resetState + "/pending.json"); err != nil {
			t.Fatal(err)
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
	t.Run("partial-copy-blocks-retry", func(t *testing.T) {
		writeFixture(t, home+"/last-lesson.txt")
		failure := errors.New("injected copy failure")
		err := runReset(config, func(source, destination string, uid, gid int) error {
			writeFixture(t, destination+"/partially-copied")
			return failure
		})
		if !errors.Is(err, failure) {
			t.Fatal("injected failure not reported", err)
		}
		intact(t, resetState+"/previous/last-lesson.txt")
		if _, err := os.Stat(resetState + "/pending.json"); err != nil {
			t.Fatal("failure evidence missing", err)
		}
		if err := Reset(config); err == nil {
			t.Fatal("failed reset was retried")
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
	if err := Reset(config); err == nil {
		t.Fatal("incomplete reset was retried after reboot")
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
