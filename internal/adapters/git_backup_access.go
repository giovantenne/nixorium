package adapters

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/sys/unix"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The Git backup reaches its repository with a key that belongs to the backup
// alone: Nixorium creates it, shows its public half, and the administrator adds
// it to the repository (a deploy key with write access). It
// has no passphrase, so the automatic backup can use it without a prompt, and
// it is separate from the lab's own SSH key, as a replacement controller must
// reach the backup before it has the lab's keys.
const backupAccessKeyName = "nixorium-backup"

// pinnedGitHosts are the published SSH host keys of GitHub
// (api.github.com/meta) and GitLab.com (docs.gitlab.com/user/gitlab_com/), so
// the most common services need no first-connection check. Any other server
// is trusted only after the administrator compares its fingerprint
// (TrustGitHost), and recorded in ~/.ssh/nixorium-git-hosts.
var pinnedGitHosts = []string{
	"github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl",
	"github.com ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEmKSENjQEezOmxkZMy7opKgwFB9nkt5YRrYMjNuG5N87uRgg6CLrbo5wAdT/y6v0mKV0U2w0WZ2YB/++Tpockg=",
	"github.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCj7ndNxQowgcQnjshcLrqPEiiphnt+VTTvDP6mHBL9j1aNUkY4Ue1gvwnGLVlOhGeYrnZaMgRK6+PKCUXaDbC7qtbW8gIkhL7aGCsOr/C56SJMy/BCZfxd1nWzAOxSDPgVsmerOBYfNqltV9/hWCqBywINIR+5dIg6JTJ72pcEpEjcYgXkE2YEFXV1JHnsKgbLWNlhScqb2UmyRkQyytRLtL+38TGxkxCflmO+5Z8CSSNY7GidjMIZ7Q4zMjA2n1nGrlTDkzwDCsw+wqFPGQA179cnfGWOWRVruj16z6XyvxvjJwbz0wQZ75XK5tKSb7FNyeIEs4TT4jk+S4dhPeAUC5y+bDYirYgM4GC7uEnztnZyaVWQ7B381AK4Qdrwt51ZqExKbQpTUNn+EjqoTwvqNj4kqx5QUCI0ThS/YkOxJCXmPUWZbhjpCg56i+2aB6CmK2JGhn57K5mj0MNdBXA4/WnwH6XoPWJzK5Nyu2zB3nAZp+S5hpQs+p1vN1/wsjk=",
	"gitlab.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAfuCHKVTjquxvt6CM6tdG4SLp1Btn/nOeHHE5UOzRdf",
	"gitlab.com ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBFSMqzJeV9rUzU4kWitGjeR4PWSa29SPqJ1fVkhtj3Hw9xjLVXVYrU9QlYWrOLXBpQ6KWjbjTDTdDkoohFzgbEY=",
	"gitlab.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCsj2bNKTBSpIYDEGk9KxsGh3mySTRgMtXL583qmBpzeQ+jqCMRgBqB98u3z++J1sKlXHWfM9dyhSevkMwSbhoR8XIq/U0tCNyokEi/ueaBMCvbcTHhO7FcwzY92WK4Yt0aGROY5qX2UKSeOvuP4D6TPqKF1onrSzH9bx9XUf2lEdWT/ia1NEKjunUqu1xOB/StKDHMoX4/OKyIzuS0q/T1zOATthvasJFoPrAjkohTyaDUz2LN5JoH839hViyEG82yB+MjcFV5MU3N1l1QL3cVUCh93xSaua1N85qivl+siMkPGbO5xR/En4iEY6K2XPASUEMaieWVNTRCtJ4S8H+9",
}

// gitHostsFileName holds the Git servers the administrator trusted after
// comparing their fingerprint. Nixorium writes only this file; the
// administrator's own known_hosts is read, never changed.
const gitHostsFileName = "nixorium-git-hosts"

func backupAccessKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", backupAccessKeyName), nil
}

// BackupAccessKey returns the public key the Git backup uses, creating the key
// pair on first use. The private key never leaves ~/.ssh.
func BackupAccessKey() (string, error) {
	path, err := backupAccessKeyPath()
	if err != nil {
		return "", err
	}
	if data, err := os.ReadFile(path + ".pub"); err == nil {
		if _, _, _, _, parseErr := ssh.ParseAuthorizedKey(data); parseErr == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				return strings.TrimSpace(string(data)), nil
			}
		}
	}
	if _, err := os.Lstat(path); err == nil {
		// The private key exists but its public half is missing or damaged:
		// derive it again instead of creating another key.
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return "", fmt.Errorf("%s is not a usable key; move it away and retry", path)
		}
		line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) + " " + backupAccessKeyName
		if err := os.WriteFile(path+".pub", []byte(line+"\n"), 0644); err != nil {
			return "", err
		}
		return line, nil
	}
	if err := ensureRealDirectory(filepath.Dir(path), 0700, "SSH directory"); err != nil {
		return "", err
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	block, err := ssh.MarshalPrivateKey(private, backupAccessKeyName)
	if err != nil {
		return "", err
	}
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPublic))) + " " + backupAccessKeyName
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(pem.EncodeToMemory(block))
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		os.Remove(path)
		return "", err
	}
	if err := os.WriteFile(path+".pub", []byte(line+"\n"), 0644); err != nil {
		os.Remove(path)
		return "", err
	}
	return line, nil
}

// backupSSHCommand is the ssh command Git uses for backup transport. Host keys
// stay strictly checked: the administrator's known_hosts, the servers trusted
// through TrustGitHost and the pinned keys in hosts. The backup key comes first, then the
// default keys ssh would otherwise try, so a backup configured earlier with the
// administrator's own key keeps working.
func backupSSHCommand(hosts string) string {
	parts := []string{"ssh", "-oBatchMode=yes", "-oStrictHostKeyChecking=yes", "-oConnectTimeout=15"}
	home, err := os.UserHomeDir()
	if err != nil {
		return strings.Join(parts, " ")
	}
	parts = append(parts, shellQuote("-oUserKnownHostsFile="+filepath.Join(home, ".ssh", "known_hosts")+" "+filepath.Join(home, ".ssh", gitHostsFileName)+" "+hosts))
	// An explicit -i replaces ssh's default key list, so the defaults that
	// exist are named again after the backup key.
	for _, name := range []string{backupAccessKeyName, "id_ed25519", "id_ecdsa", "id_rsa"} {
		path := filepath.Join(home, ".ssh", name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			parts = append(parts, "-i", shellQuote(path))
		}
	}
	return strings.Join(parts, " ")
}

// writePinnedGitHosts writes the pinned host keys to a private temporary file
// and returns its path and a cleanup function.
func writePinnedGitHosts() (string, func(), error) {
	file, err := os.CreateTemp("", "nixorium-git-hosts-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.Remove(file.Name()) }
	_, writeErr := file.WriteString(strings.Join(pinnedGitHosts, "\n") + "\n")
	if err := errors.Join(writeErr, file.Close()); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return file.Name(), cleanup, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

var backupPushDenied = regexp.MustCompile(`permission to \S+ denied`)

// offendingKeyFile finds the file that holds the old key in ssh's warning.
var offendingKeyFile = regexp.MustCompile(`Offending \S+ key in (\S+):[0-9]+`)

// errOwnSSHSettings explains an address whose server Nixorium cannot verify
// itself, usually because ~/.ssh/config sends it elsewhere.
var errOwnSSHSettings = errors.New("this controller cannot verify the Git server by itself; if the address uses your own SSH settings (~/.ssh/config), add the server's key to ~/.ssh/known_hosts and try again")

// errUnknownGitHost marks a server ssh has no host key for; checkBackupAccess
// turns it into domain.UnknownGitHostError with the server's fingerprint.
var errUnknownGitHost = errors.New("this controller has not verified the Git server yet; check the repository address")

// explainBackupGitError turns what Git and ssh report into the one thing the
// administrator can do about it. Unknown errors keep the general advice.
func explainBackupGitError(message string) error {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "remote host identification has changed"):
		where := "~/.ssh/nixorium-git-hosts or ~/.ssh/known_hosts"
		if found := offendingKeyFile.FindStringSubmatch(message); found != nil {
			where = found[1]
		}
		return fmt.Errorf("the Git server's identity does not match the one saved on this controller, so nothing was sent. Ask your Git service whether the server was reinstalled; only then remove the old key with ssh-keygen -R <server> -f %s and back up again to compare the new fingerprint", where)
	case strings.Contains(lower, "host key verification failed"):
		return errUnknownGitHost
	case strings.Contains(lower, "could not resolve hostname"), strings.Contains(lower, "connection timed out"), strings.Contains(lower, "network is unreachable"), strings.Contains(lower, "connection refused"), strings.Contains(lower, "no route to host"):
		return errors.New("the Git server cannot be reached; check this controller's internet connection and the repository address")
	case strings.Contains(lower, "permission denied (publickey"):
		return errors.New("the Git server refused this controller's backup key; add it to the repository with write access (it is shown on the backup screen and by `nixorium backup key`)")
	case strings.Contains(lower, "protected branch"):
		return errors.New("the branch is protected on the Git server; allow the backup key to push to it (GitLab: Settings → Repository → Protected branches → Allowed to push), or remove the protection")
	case strings.Contains(lower, "marked as read only"), strings.Contains(lower, "does not have write access"), strings.Contains(lower, "not allowed to upload code"), strings.Contains(lower, "write access to repository not granted"), backupPushDenied.MatchString(lower):
		return errors.New("the backup key can read the repository but cannot write to it; give the key write access on the Git service")
	case strings.Contains(lower, "repository not found"), strings.Contains(lower, "does not appear to be a git repository"), strings.Contains(lower, "project you were looking for could not be found"):
		return errors.New("the repository was not found, or the backup key has no access to it; check the address and that the key is added to this repository")
	case strings.Contains(lower, "couldn't find remote ref"):
		return errors.New("the branch was not found in the repository; check the branch name")
	case strings.Contains(lower, "non-fast-forward"), strings.Contains(lower, "fetch first"), strings.Contains(lower, "[rejected]"):
		return errors.New("the repository already holds different history on this branch; use an empty private repository, or the branch of an earlier backup of this lab (Nixorium never overwrites it)")
	}
	general := "Git operation failed; check SSH access, the repository URL and branch, then retry (no force push is performed)"
	if last := lastGitLine(message); last != "" {
		general += ". Git said: " + last
	}
	return errors.New(general)
}

// gitRemoteHost returns the host and port ssh connects to for remote.
func gitRemoteHost(remote string) (host, port string, err error) {
	if u, parseErr := url.Parse(remote); parseErr == nil && u.Scheme == "ssh" {
		host, port = u.Hostname(), u.Port()
	} else if at := strings.Index(remote, "@"); at >= 0 {
		rest := remote[at+1:]
		if colon := strings.Index(rest, ":"); colon > 0 {
			host = rest[:colon]
		}
	}
	if host == "" {
		return "", "", errors.New("the repository address names no server")
	}
	if port == "" {
		port = "22"
	}
	return host, port, nil
}

// knownHostsName is the host as known_hosts writes it.
func knownHostsName(host, port string) string {
	return knownhosts.Normalize(net.JoinHostPort(host, port))
}

// scanGitHost connects to the server of remote only to read its host key and
// returns it as an UnknownGitHostError. No authentication is attempted.
func scanGitHost(ctx context.Context, remote string) (domain.UnknownGitHostError, error) {
	key, err := fetchGitHostKey(ctx, remote)
	if err != nil {
		return domain.UnknownGitHostError{}, err
	}
	host, port, _ := gitRemoteHost(remote)
	name := host
	if port != "22" {
		name = net.JoinHostPort(host, port)
	}
	return domain.UnknownGitHostError{Host: name, Algorithm: hostKeyAlgorithmName(key.Type()), Fingerprint: ssh.FingerprintSHA256(key)}, nil
}

// gitHostKeyAlgorithms prefers the same key type ssh prefers, so the shown
// fingerprint is the one a later connection checks.
var gitHostKeyAlgorithms = []string{ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}

var errHostKeyRead = errors.New("host key read")

func fetchGitHostKey(ctx context.Context, remote string) (ssh.PublicKey, error) {
	host, port, err := gitRemoteHost(remote)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, errors.New("the Git server cannot be reached; check this controller's internet connection and the repository address")
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	var key ssh.PublicKey
	config := &ssh.ClientConfig{
		User:              "git",
		HostKeyAlgorithms: gitHostKeyAlgorithms,
		HostKeyCallback: func(_ string, _ net.Addr, offered ssh.PublicKey) error {
			key = offered
			return errHostKeyRead
		},
	}
	_, _, _, err = ssh.NewClientConn(connection, net.JoinHostPort(host, port), config)
	if key == nil {
		if err == nil {
			err = errors.New("no host key")
		}
		return nil, fmt.Errorf("the Git server's identity could not be read: %w", err)
	}
	return key, nil
}

func hostKeyAlgorithmName(keyType string) string {
	switch {
	case keyType == ssh.KeyAlgoED25519:
		return "ED25519"
	case strings.HasPrefix(keyType, "ecdsa-"):
		return "ECDSA"
	case keyType == ssh.KeyAlgoRSA:
		return "RSA"
	}
	return strings.ToUpper(keyType)
}

// TrustGitHost records the server of remote as trusted, but only when the key
// it offers now has the fingerprint the administrator compared. The record is
// appended to ~/.ssh/nixorium-git-hosts; nothing else is changed.
func TrustGitHost(ctx context.Context, remote, fingerprint string) error {
	if err := (GitBackup{}).validateDestination(remote, "main"); err != nil {
		return err
	}
	key, err := fetchGitHostKey(ctx, remote)
	if err != nil {
		return err
	}
	if ssh.FingerprintSHA256(key) != strings.TrimSpace(fingerprint) {
		return errors.New("the Git server now shows a different fingerprint than the one you compared, so it was not trusted; check the address and try again")
	}
	host, port, err := gitRemoteHost(remote)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	directory := filepath.Join(home, ".ssh")
	if err := ensureRealDirectory(directory, 0700, "SSH directory"); err != nil {
		return err
	}
	path := filepath.Join(directory, gitHostsFileName)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > maximumKeyMaterialBytes {
		file.Close()
		return fmt.Errorf("%s is not a small regular file", path)
	}
	existing, err := io.ReadAll(file)
	if err != nil {
		file.Close()
		return err
	}
	line := knownhosts.Line([]string{knownHostsName(host, port)}, key)
	if strings.Contains("\n"+string(existing)+"\n", "\n"+line+"\n") {
		return file.Close()
	}
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		line = "\n" + line
	}
	_, writeErr := file.WriteString(line + "\n")
	return errors.Join(writeErr, file.Sync(), file.Close())
}

// lastGitLine is the last meaningful line Git or ssh printed, kept short.
func lastGitLine(message string) string {
	lines := strings.Split(strings.TrimSpace(message), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[index]), "git:"))
		if line == "" {
			continue
		}
		if runes := []rune(line); len(runes) > 160 {
			line = string(runes[:160]) + "…"
		}
		return line
	}
	return ""
}
