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

func TestStaticEtcAndStoreKeepTheEtcRefusals(t *testing.T) {
	for _, path := range []string{"/etc/static/sudoers.d/x", "/etc/static/wpa_supplicant.conf", "/etc/wpa_supplicant.conf", "/etc/shadow-", "/etc/machine-id",
		"/var/lib/nixorium/prepared/roots/abc/pc01", "/run/nixorium/remote-install/x", "/sys/class/dmi/id/product_uuid"} {
		if Readable(path) {
			t.Errorf("%s should be refused", path)
		}
	}
	if !Readable("/etc/static/hosts") {
		t.Error("/etc/static/hosts should be readable")
	}
	for _, resolved := range []string{
		"/nix/store/" + strings.Repeat("a", 32) + "-etc/etc/sudoers.d/x",
		"/nix/store/" + strings.Repeat("a", 32) + "-etc/etc/NetworkManager/system-connections/school.nmconnection",
	} {
		if resolvedAllowed("/etc/static/x", resolved) {
			t.Errorf("%s should be refused", resolved)
		}
	}
	// A store hash may contain any letters: it is not a name.
	if !resolvedAllowed("/etc/hosts", "/nix/store/0psk"+strings.Repeat("a", 28)+"-etc-hosts") {
		t.Error("a store path whose hash contains psk was refused")
	}
}

func TestResolvedSystemLocationsOnlyForTheirRequests(t *testing.T) {
	for _, test := range []struct {
		request, resolved string
		allowed           bool
	}{
		{"/sys/class/net/eth0/address", "/sys/devices/pci0000:00/0000:00:03.0/net/eth0/address", true},
		{"/sys/class/dmi/id/product_name", "/sys/devices/virtual/dmi/id/product_name", true},
		{"/sys/class/dmi/id/product_serial", "/sys/devices/virtual/dmi/id/product_serial", false},
		{"/proc/mounts", "/proc/1234/mounts", true},
		{"/etc/mtab", "/proc/self/mounts", true},
		{"/etc/cups/printers.conf", "/var/lib/cups/printers.conf", true},
		{"/etc/cups/ssl/server.key", "/var/lib/cups/ssl/server.key", false},
		{"/etc/resolv.conf", "/run/systemd/resolve/stub-resolv.conf", true},
		{"/etc/hosts", "/sys/devices/virtual/dmi/id/product_name", false},
		{"/etc/hosts", "/proc/1/mounts", false},
		{"/etc/hosts", "/home/admin/x", false},
	} {
		if got := resolvedAllowed(test.request, test.resolved); got != test.allowed {
			t.Errorf("%s -> %s: %v", test.request, test.resolved, got)
		}
	}
	answer, _ := Encode(Result{Path: "/sys/class/net/eth0", Resolved: "/sys/devices/pci0000:00/net/eth0", Kind: "directory"})
	if _, err := Decode(answer, "/sys/class/net/eth0"); err != nil {
		t.Fatal(err)
	}
	misplaced, _ := Encode(Result{Path: "/etc/hosts", Resolved: "/sys/devices/x", Kind: "file"})
	if _, err := Decode(misplaced, "/etc/hosts"); err == nil {
		t.Fatal("a resolution reserved for /sys was accepted for /etc")
	}
}

func TestCompactSecretsAndKeyBlocksAreWithheld(t *testing.T) {
	text := withholdSecretLines(`{"password":"hunter2"}` + "\npassword:compact1\nbindpw ldapsecret\n" +
		"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----\n" +
		"<tls-auth>\nf00dfeedf00dfeed\n</tls-auth>\nremote vpn.example 1194\n")
	for _, leaked := range []string{"hunter2", "compact1", "ldapsecret", "MIIEvQIBADANBgkqhkiG9w0BAQEFAASC", "f00dfeedf00dfeed"} {
		if strings.Contains(text, leaked) {
			t.Errorf("leaked %q:\n%s", leaked, text)
		}
	}
	for _, kept := range []string{`{"password" [withheld]`, "password [withheld]", "-----BEGIN PRIVATE KEY-----", "</tls-auth>", "remote vpn.example 1194"} {
		if !strings.Contains(text, kept) {
			t.Errorf("lost %q:\n%s", kept, text)
		}
	}
	for _, refused := range []string{"/etc/openvpn/client.conf", "/etc/cups/certs/0", "/sys/class/net/eth0/device/resource0", "/sys/class/net/eth0/device/rom"} {
		if Readable(refused) {
			t.Errorf("%s is readable", refused)
		}
	}
}

func TestSecretFormsAreWithheld(t *testing.T) {
	text := withholdSecretLines("[Peer]\nPresharedKey = abc123\nnetwork={\n  passphrase=\"hunter2\"\n  wep_key0=\"x\"\n}\n" +
		"DeviceURI smb://teacher:s3cret@printserver/laser\nwpa_passphrase=topsecret\nInfo Laser\n")
	for _, leaked := range []string{"abc123", "hunter2", "s3cret", "topsecret", `"x"`} {
		if strings.Contains(text, leaked) {
			t.Errorf("leaked %q:\n%s", leaked, text)
		}
	}
	for _, kept := range []string{"[Peer]", "PresharedKey [withheld]", "smb://[withheld]@printserver/laser", "Info Laser"} {
		if !strings.Contains(text, kept) {
			t.Errorf("lost %q:\n%s", kept, text)
		}
	}
}
