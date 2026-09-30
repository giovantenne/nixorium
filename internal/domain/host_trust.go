package domain

import "time"

type HostKeyCondition string

const HostKeyChanged HostKeyCondition = "changed"

type HostTrustInspection struct {
	Recorded        []string `json:"recordedFingerprints"`
	Offered         string   `json:"offeredFingerprint"`
	PublicKey       string   `json:"offeredPublicKey"`
	BaseFingerprint string   `json:"baseFingerprint"`
}

type HostTrustPlan struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Operation     string              `json:"operation"`
	State         string              `json:"state"`
	Repository    string              `json:"repository"`
	Revision      string              `json:"revision"`
	Host          HostMeta            `json:"host"`
	Inspection    HostTrustInspection `json:"inspection"`
	ReviewToken   string              `json:"reviewToken,omitempty"`
	Confirmation  string              `json:"confirmation,omitempty"`
	ExpiresAt     time.Time           `json:"expiresAt"`
	Message       string              `json:"message"`
}

func (p HostTrustPlan) HasErrors() bool { return p.State != "ready" }

type HostTrustResult struct {
	Operation string `json:"operation"`
	State     string `json:"state"`
	Host      string `json:"host"`
	Message   string `json:"message"`
}

func (r HostTrustResult) HasErrors() bool { return r.State != "saved" }
