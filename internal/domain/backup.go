package domain

import "time"

// Backups keep the deployment repository with its history, the private keys
// that are never committed, and the controller's verified host keys, in one
// passphrase-encrypted file (ADR 0024).
const (
	BackupSchemaVersion     = 1
	BackupMinimumPassphrase = 12
	BackupReminderAge       = 30 * 24 * time.Hour
)

type BackupFile struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
	Link   string `json:"link,omitempty"`
}

type BackupManifest struct {
	SchemaVersion   int          `json:"schemaVersion"`
	CreatedAt       time.Time    `json:"createdAt"`
	NixoriumVersion string       `json:"nixoriumVersion"`
	Revision        string       `json:"revision"`
	Files           []BackupFile `json:"files"`
}

type BackupReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Path          string            `json:"path,omitempty"`
	Bytes         int64             `json:"bytes,omitempty"`
	CreatedAt     time.Time         `json:"createdAt,omitempty"`
	Revision      string            `json:"revision,omitempty"`
	Files         int               `json:"files"`
	PrivateKeys   []string          `json:"privateKeys"`
	Message       string            `json:"message,omitempty"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r BackupReport) HasErrors() bool {
	return r.State != "completed" || len(r.Issues) > 0
}

// BackupRecord is the administrator's private note of the last backup.
type BackupRecord struct {
	Repository   string    `json:"repository"`
	CreatedAt    time.Time `json:"createdAt"`
	Path         string    `json:"path"`
	Revision     string    `json:"revision"`
	KeysDigest   string    `json:"keysDigest"`
	SettingsHash string    `json:"settingsHash"`
}

// RecoveryBackupDue is reported while no recent backup exists.
const RecoveryBackupDue = "backup-due"
