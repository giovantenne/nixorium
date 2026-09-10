package domain

type ActionReport struct {
	SchemaVersion int    `json:"schemaVersion"`
	Operation     string `json:"operation"`
	State         string `json:"state"`
	Unit          string `json:"unit"`
	Message       string `json:"message,omitempty"`
}

func (r ActionReport) HasErrors() bool {
	return r.State == "failed"
}
