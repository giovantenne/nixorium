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
