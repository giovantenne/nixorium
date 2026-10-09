package domain

import (
	"encoding/json"
	"errors"
)

// CredentialsVersion binds the local hashes to the public configuration.
// Hashes are retained in memory for the existing private settings editor only.
type LabCredentials struct {
	Version int    `json:"version"`
	Admin   string `json:"admin"`
	Teacher string `json:"teacher"`
	Student string `json:"student"`
}

func CredentialsFromSettings(s LabSettingsFile) LabCredentials {
	return LabCredentials{s.Lab.CredentialsVersion, s.Lab.AdminPassword, s.Lab.TeacherPassword, s.Lab.StudentPassword}
}

func (c LabCredentials) Ready() bool {
	for _, hash := range []string{c.Admin, c.Teacher, c.Student} {
		if !IsPasswordHash(hash) || hash == DefaultPasswordHash {
			return false
		}
	}
	return true
}

func (c LabCredentials) Validate() error {
	if c.Version < 1 || !c.Ready() {
		return errors.New("account credentials are incomplete; set all passwords in Maintenance → Change settings → Passwords")
	}
	return nil
}

func MarshalPublicLabSettings(s LabSettingsFile) ([]byte, error) {
	if _, err := MarshalLabSettings(s); err != nil {
		return nil, err
	}
	s.Lab.AdminPassword, s.Lab.TeacherPassword, s.Lab.StudentPassword = "", "", ""
	data, err := json.MarshalIndent(s, "", "  ")
	return append(data, '\n'), err
}
