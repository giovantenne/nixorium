package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/crypto/ssh"
)

func (Local) ObserveHostTrust(ctx context.Context, host domain.HostMeta) (domain.HostTrustInspection, error) {
	content, err := readOptionalKnownHosts(ManagedKnownHostsPath)
	if err != nil {
		return domain.HostTrustInspection{}, err
	}
	public, err := NewLiveBootstrap().observeHostPublicKey(ctx, host.IP)
	if err != nil {
		return domain.HostTrustInspection{}, err
	}
	return inspectHostTrust(content, host.IP, public)
}

func inspectHostTrust(content []byte, address, public string) (domain.HostTrustInspection, error) {
	result := domain.HostTrustInspection{BaseFingerprint: knownHostsFingerprint(content), PublicKey: public, Recorded: []string{}}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(public))
	if err != nil || key.Type() != ssh.KeyAlgoED25519 {
		return result, errors.New("offered host key is not valid Ed25519")
	}
	result.Offered = ssh.FingerprintSHA256(key)
	for _, line := range strings.Split(string(content), "\n") {
		matches, recorded, err := knownHostLineMatch(line, address)
		if err != nil {
			return result, err
		}
		if matches {
			result.Recorded = append(result.Recorded, ssh.FingerprintSHA256(recorded))
		}
	}
	sort.Strings(result.Recorded)
	return result, nil
}

func (Local) ReplaceHostTrust(_ context.Context, host domain.HostMeta, review domain.HostTrustInspection) error {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	if err := mergeVerifiedKnownHost(ManagedKnownHostsPath, filepath.Join(filepath.Dir(ManagedKnownHostsPath), "backups"), host.IP, review.PublicKey, hex.EncodeToString(id), true, review.BaseFingerprint); err != nil {
		return err
	}
	// Best effort: controller activation repairs the link too.
	_ = restoreKnownHostsLink(filepath.Join(filepath.Dir(filepath.Dir(ManagedKnownHostsPath)), "known_hosts"), ManagedKnownHostsPath)
	return nil
}

// restoreKnownHostsLink points the standard OpenSSH known_hosts back at the
// managed copy when a tool (OpenSSH host key updates, ssh-keygen -R)
// replaced the link with a regular file. ssh then keeps reading that stale
// copy, and a reviewed new key would stay "changed" until the next
// controller activation. The replaced file is kept next to the managed copy
// for review; keys are never merged from it.
func restoreKnownHostsLink(legacy, managed string) error {
	info, err := os.Lstat(legacy)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	case info.Mode()&os.ModeSymlink != 0:
		if target, err := os.Readlink(legacy); err != nil || target == managed {
			return err
		}
		return errors.New("known_hosts points elsewhere")
	case !info.Mode().IsRegular():
		return errors.New("known_hosts is not a regular file")
	default:
		content, err := readOptionalKnownHosts(legacy)
		if err != nil {
			return err
		}
		kept, err := os.CreateTemp(filepath.Dir(managed), "known_hosts-legacy-"+time.Now().UTC().Format("20060102T150405Z")+".*.bak")
		if err != nil {
			return err
		}
		_, writeErr := kept.Write(content)
		syncErr := kept.Sync()
		if closeErr := kept.Close(); writeErr != nil || syncErr != nil || closeErr != nil {
			return errors.Join(writeErr, syncErr, closeErr)
		}
	}
	link := filepath.Join(filepath.Dir(legacy), fmt.Sprintf(".known_hosts.link-%d", os.Getpid()))
	_ = os.Remove(link)
	if err := os.Symlink(managed, link); err != nil {
		return err
	}
	if err := os.Rename(link, legacy); err != nil {
		_ = os.Remove(link)
		return err
	}
	return nil
}
