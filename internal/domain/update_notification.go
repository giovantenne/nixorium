package domain

import "time"

// UpdateNotification is advisory. Opening it always starts an ordinary update
// check and review; cached discovery never authorizes a configuration change.
type UpdateNotification struct {
	Key      string        `json:"key"`
	Target   string        `json:"target"`
	Revision string        `json:"revision"`
	Channel  UpdateChannel `json:"channel"`
}

type UpdateNotificationState struct {
	Input     string             `json:"input"`
	CheckedAt time.Time          `json:"checkedAt"`
	Available UpdateNotification `json:"available"`
	Dismissed string             `json:"dismissed"`
}
