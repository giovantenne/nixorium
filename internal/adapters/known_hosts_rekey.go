package adapters

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// RekeyKnownHosts records, for each computer that moves to a new laboratory
// address, the host keys already trusted for its previous address under the
// new address too. No computer is contacted and no key is ever replaced: a
// different key already recorded for the new address is reported as a
// conflict and left alone, so SSH keeps refusing it.
func (Local) RekeyKnownHosts(moves []domain.KnownHostMove) (domain.KnownHostRekey, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return domain.KnownHostRekey{}, err
	}
	return rekeyKnownHosts(ManagedKnownHostsPath, filepath.Join(filepath.Dir(ManagedKnownHostsPath), "backups"), hex.EncodeToString(id), moves)
}

func rekeyKnownHosts(path, backupRoot, operationID string, moves []domain.KnownHostMove) (domain.KnownHostRekey, error) {
	report := domain.KnownHostRekey{}
	if !remoteStateID(operationID) || !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		!filepath.IsAbs(backupRoot) || filepath.Clean(backupRoot) != backupRoot {
		return report, errors.New("known-host re-key identity is invalid")
	}
	for _, move := range moves {
		if canonicalRemoteIPv4(move.From) == "" || canonicalRemoteIPv4(move.To) == "" || move.From == move.To {
			return report, errors.New("known-host re-key addresses are invalid")
		}
	}
	if len(moves) == 0 {
		return report, nil
	}
	unlock, err := lockKnownHosts(path, false)
	if err != nil {
		return report, err
	}
	defer unlock()
	content, err := readOptionalKnownHosts(path)
	if err != nil {
		return report, err
	}
	lines := []string{}
	for _, line := range strings.Split(string(content), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	keysFor := func(address string) ([]ssh.PublicKey, error) {
		keys := []ssh.PublicKey{}
		for _, line := range lines {
			matches, key, err := knownHostLineMatch(line, address)
			if err != nil {
				return nil, err
			}
			if matches {
				keys = append(keys, key)
			}
		}
		return keys, nil
	}
	added := []string{}
	for _, move := range moves {
		previous, err := keysFor(move.From)
		if err != nil {
			return report, err
		}
		current, err := keysFor(move.To)
		if err != nil {
			return report, err
		}
		if len(previous) == 0 {
			report.Missing = append(report.Missing, move.Name)
			continue
		}
		conflict := false
		for _, key := range current {
			if !containsKey(previous, key) {
				conflict = true
			}
		}
		if conflict {
			report.Conflicts = append(report.Conflicts, move.Name)
			continue
		}
		missing := []ssh.PublicKey{}
		for _, key := range previous {
			if !containsKey(current, key) && !containsKey(missing, key) {
				missing = append(missing, key)
			}
		}
		if len(missing) == 0 {
			report.Already = append(report.Already, move.Name)
			continue
		}
		for _, key := range missing {
			added = append(added, knownhosts.Line([]string{move.To}, key))
		}
		report.Added = append(report.Added, move.Name)
	}
	if len(added) == 0 {
		return report, nil
	}
	updated := []byte(strings.Join(append(lines, added...), "\n") + "\n")
	if err := replaceKnownHosts(path, backupRoot, operationID, content, updated); err != nil {
		return domain.KnownHostRekey{}, err
	}
	return report, nil
}

func containsKey(keys []ssh.PublicKey, key ssh.PublicKey) bool {
	for _, candidate := range keys {
		if subtle.ConstantTimeCompare(candidate.Marshal(), key.Marshal()) == 1 {
			return true
		}
	}
	return false
}
