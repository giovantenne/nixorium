package domain

type PXELifecycleState struct {
	Mode     string       `json:"mode"`
	Listener ServiceState `json:"listener"`
	Network  ServiceState `json:"network"`
	Detail   string       `json:"detail,omitempty"`
}

type PXELifecycleReport struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Operation     string              `json:"operation"`
	State         string              `json:"state"`
	Mode          string              `json:"mode"`
	Interface     string              `json:"interface,omitempty"`
	DHCPAddress   string              `json:"dhcpAddress,omitempty"`
	StaticCIDR    string              `json:"staticCidr,omitempty"`
	Preparation   PXEPreparationState `json:"preparation"`
	Services      []ServiceState      `json:"services"`
	Message       string              `json:"message,omitempty"`
}

func (r PXELifecycleReport) HasErrors() bool {
	return r.State == "failed"
}
