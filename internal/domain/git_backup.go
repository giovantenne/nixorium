package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// GitBackupPlan binds the explicitly chosen destination to saved configuration
// and recovery material. It contains no private key or passphrase.
type GitBackupPlan struct {
	Repository     string `json:"repository"`
	Remote         string `json:"remote"`
	Branch         string `json:"branch"`
	Revision       string `json:"revision"`
	RecoveryDigest string `json:"recoveryDigest"`
	Files          int    `json:"files"`
	ReviewToken    string `json:"reviewToken"`
}

func (p GitBackupPlan) Token() string {
	p.ReviewToken = ""
	data, _ := json.Marshal(p)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// UnknownGitHostError reports a Git server that this controller has never
// verified. Its fingerprint is shown so the administrator can compare it with
// the one the Git service publishes before trusting it.
type UnknownGitHostError struct {
	Host        string `json:"host"`
	Algorithm   string `json:"algorithm"`
	Fingerprint string `json:"fingerprint"`
}

func (e UnknownGitHostError) Error() string {
	return "this controller has not verified the Git server " + e.Host + " yet (" + e.Algorithm + " " + e.Fingerprint + "); compare the fingerprint with the one your Git service publishes, then trust it on the Back up lab screen or with nixorium backup trust-host"
}
