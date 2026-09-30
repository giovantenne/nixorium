package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sort"
	"strings"

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
	return mergeVerifiedKnownHost(ManagedKnownHostsPath, filepath.Join(filepath.Dir(ManagedKnownHostsPath), "backups"), host.IP, review.PublicKey, hex.EncodeToString(id), true, review.BaseFingerprint)
}
