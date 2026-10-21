package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// reviewTokenDigest hashes a plan whose volatile fields were already cleared
// by the caller.
func reviewTokenDigest(plan any) string {
	content, _ := json.Marshal(plan)
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
