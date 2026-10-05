package adapters

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/domain"
)

// These per-administrator UI preferences are outside the deployment and Nix
// store. They are neither telemetry consent nor operation authorization.
func (Local) DisclaimerAccepted(repository string) (bool, error) {
	var state struct {
		Version int `json:"version"`
	}
	err := readDashboardPreference(repository, "disclaimer", &state)
	return state.Version == 1, err
}
func (Local) AcceptDisclaimer(repository string) error {
	return writeDashboardPreference(repository, "disclaimer", struct {
		Version int `json:"version"`
	}{1})
}
func (Local) LoadUpdateNotification(repository string) (domain.UpdateNotificationState, error) {
	var state domain.UpdateNotificationState
	err := readDashboardPreference(repository, "updates", &state)
	return state, err
}
func (Local) SaveUpdateNotification(repository string, state domain.UpdateNotificationState) error {
	return writeDashboardPreference(repository, "updates", state)
}
func dashboardPreferencePath(repository, kind string) (string, error) {
	root, err := userStateRoot()
	if err != nil {
		return "", err
	}
	repository, err = filepath.Abs(repository)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(repository))
	return filepath.Join(root, "nixorium", "dashboard", hex.EncodeToString(sum[:])+"-"+kind+".json"), nil
}
func readDashboardPreference(repository, kind string, value any) error {
	path, err := dashboardPreferencePath(repository, kind)
	if err != nil {
		return err
	}
	for _, directory := range []string{filepath.Dir(filepath.Dir(path)), filepath.Dir(path)} {
		if err := checkPrivateDirectory(directory); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
	}
	data, mode, err := readRegularFileNoFollowLimit(path, 16*1024)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if mode != 0600 {
		return errors.New("dashboard preferences must have mode 0600")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return rejectTrailingJSON(decoder)
}
func writeDashboardPreference(repository, kind string, value any) error {
	path, err := dashboardPreferencePath(repository, kind)
	if err != nil {
		return err
	}
	for _, directory := range []string{filepath.Dir(filepath.Dir(path)), filepath.Dir(path)} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			return err
		}
		if err := checkPrivateDirectory(directory); err != nil {
			return err
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > 16*1024 {
		return errors.New("dashboard preferences exceed size limit")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("dashboard preferences must be a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
