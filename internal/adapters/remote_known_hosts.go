package adapters

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func KnownHostConflict(path, host, expectedPublicKey string) (bool, error) {
	content, err := readOptionalKnownHosts(path)
	if err != nil {
		return false, err
	}
	if content == nil {
		return false, nil
	}
	expected, _, _, _, err := ssh.ParseAuthorizedKey([]byte(expectedPublicKey + "\n"))
	if err != nil || expected.Type() != ssh.KeyAlgoED25519 {
		return false, errors.New("reviewed known-host key is invalid")
	}
	for _, line := range strings.Split(string(content), "\n") {
		matches, key, err := knownHostLineMatch(line, host)
		if err != nil {
			return false, err
		}
		if !matches {
			continue
		}
		if subtle.ConstantTimeCompare(key.Marshal(), expected.Marshal()) != 1 {
			return true, nil
		}
	}
	return false, nil
}

func MergeVerifiedKnownHost(path, backupRoot, host, expectedPublicKey, operationID string, allowRotation bool) error {
	if !remoteStateID(operationID) || canonicalRemoteIPv4(host) == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		!filepath.IsAbs(backupRoot) || filepath.Clean(backupRoot) != backupRoot {
		return errors.New("known-host merge identity is invalid")
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(expectedPublicKey + "\n"))
	if err != nil || key.Type() != ssh.KeyAlgoED25519 {
		return errors.New("known-host merge key is invalid")
	}
	directoryPath := filepath.Dir(path)
	if err := ensurePrivateOwnedDirectory(directoryPath); err != nil {
		return fmt.Errorf("inspect administrator SSH directory: %w", err)
	}
	lockPath := filepath.Join(directoryPath, ".nixorium-known-hosts.lock")
	lockDescriptor, err := syscall.Open(lockPath, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	lock := os.NewFile(uintptr(lockDescriptor), lockPath)
	defer lock.Close()
	if err := validatePrivateOwnedFile(lock, syscall.S_IFREG, 0600); err != nil {
		return err
	}
	if err := syscall.Flock(lockDescriptor, syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(lockDescriptor, syscall.LOCK_UN)

	content, err := readOptionalKnownHosts(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	filtered := make([]string, 0, len(lines)+1)
	foundSame := false
	foundConflict := false
	for _, line := range lines {
		matches, observed, matchErr := knownHostLineMatch(line, host)
		if matchErr != nil {
			return matchErr
		}
		if !matches {
			if line != "" {
				filtered = append(filtered, line)
			}
			continue
		}
		if subtle.ConstantTimeCompare(observed.Marshal(), key.Marshal()) == 1 {
			foundSame = true
			filtered = append(filtered, line)
			continue
		}
		foundConflict = true
		if !allowRotation {
			return errors.New("known-host entry differs; explicit reviewed rotation is required")
		}
		remainder, err := removeHostFromKnownHostLine(line, host)
		if err != nil {
			return err
		}
		if remainder != "" {
			filtered = append(filtered, remainder)
		}
	}
	if foundSame && !foundConflict {
		return nil
	}
	filtered = append(filtered, knownhosts.Line([]string{host}, key))
	updated := []byte(strings.Join(filtered, "\n") + "\n")
	if content != nil {
		if err := ensurePrivateOwnedDirectory(backupRoot); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("inspect known-hosts backup directory: %w", err)
			}
			if err := os.Mkdir(backupRoot, 0700); err != nil {
				return fmt.Errorf("create known-hosts backup directory: %w", err)
			}
		}
		backup := filepath.Join(backupRoot, "known_hosts-"+operationID+".bak")
		if err := writeExclusivePrivateFile(backup, content); err != nil {
			return fmt.Errorf("create private known-hosts backup: %w", err)
		}
	}
	temporary, err := os.CreateTemp(directoryPath, ".known_hosts.nixorium-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		return err
	}
	if _, err := temporary.Write(updated); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	directory, err := os.Open(directoryPath)
	if err == nil {
		err = directory.Sync()
		_ = directory.Close()
	}
	return err
}

func readOptionalKnownHosts(path string) ([]byte, error) {
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	var stat syscall.Stat_t
	if err := syscall.Fstat(descriptor, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || (stat.Mode&0777 != 0600 && stat.Mode&0777 != 0644) || stat.Uid != uint32(os.Geteuid()) || stat.Size > 1024*1024 {
		return nil, errors.New("administrator known-hosts file is unsafe")
	}
	content, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil || len(content) > 1024*1024 {
		return nil, errors.New("administrator known-hosts file exceeds its safe limit")
	}
	return content, nil
}

func knownHostLineMatch(line, host string) (bool, ssh.PublicKey, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false, nil, nil
	}
	fields := strings.Fields(trimmed)
	hostIndex := 0
	if strings.HasPrefix(fields[0], "@") {
		hostIndex = 1
	}
	if len(fields) < hostIndex+3 {
		return false, nil, errors.New("administrator known-hosts contains a malformed entry")
	}
	matched := false
	for _, token := range strings.Split(fields[hostIndex], ",") {
		if knownHostTokenMatches(token, host) {
			matched = true
			break
		}
	}
	if !matched {
		return false, nil, nil
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.Join(fields[hostIndex+1:], " ") + "\n"))
	if err != nil {
		return false, nil, errors.New("administrator known-hosts matching entry has an invalid key")
	}
	return true, key, nil
}

func knownHostTokenMatches(token, host string) bool {
	if token == host || token == "["+host+"]:22" {
		return true
	}
	parts := strings.Split(token, "|")
	if len(parts) != 4 || parts[0] != "" || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	for _, candidate := range []string{host, "[" + host + "]:22"} {
		mac := hmac.New(sha1.New, salt)
		_, _ = mac.Write([]byte(candidate))
		if hmac.Equal(mac.Sum(nil), expected) {
			return true
		}
	}
	return false
}

func removeHostFromKnownHostLine(line, host string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	hostIndex := 0
	if strings.HasPrefix(fields[0], "@") {
		hostIndex = 1
	}
	remaining := []string{}
	for _, token := range strings.Split(fields[hostIndex], ",") {
		if !knownHostTokenMatches(token, host) {
			remaining = append(remaining, token)
		}
	}
	if len(remaining) == 0 {
		return "", nil
	}
	fields[hostIndex] = strings.Join(remaining, ",")
	return strings.Join(fields, " "), nil
}
