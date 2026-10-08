package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/domain"
)

const credentialsFile = "lab-credentials.json"

func decodeCredentials(data []byte) (domain.LabCredentials, error) {
	var c domain.LabCredentials
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, errors.New("invalid local account credentials")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return c, errors.New("trailing local account credentials")
	}
	return c, c.Validate()
}

func readCredentials(repository string) (domain.LabCredentials, []byte, error) {
	data, mode, err := readRecoveryFile(filepath.Join(repository, credentialsFile), 4096)
	if err != nil {
		return domain.LabCredentials{}, nil, err
	}
	if mode != 0600 {
		return domain.LabCredentials{}, nil, errors.New("lab-credentials.json must have mode 0600")
	}
	c, err := decodeCredentials(data)
	return c, data, err
}

// This merged snapshot is private editing input. It is never committed.
// A missing or interrupted credential save reopens the password wizard; it
// cannot authorize an installation with default passwords.
func hydrateCredentials(repository string, data []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) != nil {
		return data, nil
	}
	var lab map[string]json.RawMessage
	if json.Unmarshal(envelope["lab"], &lab) != nil || lab == nil {
		return data, nil
	}
	for _, name := range []string{"adminPassword", "teacherPassword", "studentPassword"} {
		if _, exists := lab[name]; exists {
			return nil, errors.New("password hashes must not be stored in lab-settings.json; use the current site template")
		}
	}
	c, _, err := readCredentials(repository)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	var version int
	if json.Unmarshal(lab["credentialsVersion"], &version) != nil || version != c.Version {
		return data, nil
	}
	lab["adminPassword"], _ = json.Marshal(c.Admin)
	lab["teacherPassword"], _ = json.Marshal(c.Teacher)
	lab["studentPassword"], _ = json.Marshal(c.Student)
	envelope["lab"], _ = json.Marshal(lab)
	merged, _ := json.Marshal(envelope)
	settings, issues := domain.DecodeLabSettings(merged)
	if len(issues) == 0 {
		return domain.MarshalLabSettings(settings)
	}
	return merged, nil
}

func saveCredentials(repository string, settings domain.LabSettingsFile) error {
	c := domain.CredentialsFromSettings(settings)
	if c.Admin == "" && c.Teacher == "" && c.Student == "" {
		return nil
	}
	if err := c.Validate(); err != nil {
		return err
	}
	old, _, err := readCredentials(repository)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if old == c {
			return nil
		}
		// Allow repairing an interrupted two-file save through the password
		// wizard. The public version still has to advance before it can be used.
		public, _, readErr := readRecoveryFile(filepath.Join(repository, settingsFileName), 1024*1024)
		var base domain.LabSettingsFile
		if readErr != nil || json.Unmarshal(public, &base) != nil || c.Version != base.Lab.CredentialsVersion+1 {
			return errors.New("a password change must increment credentialsVersion by one; review passwords again")
		}
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(repository, ".lab-credentials-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(file.Name(), filepath.Join(repository, credentialsFile)); err != nil {
		return err
	}
	return syncDirectory(repository)
}
