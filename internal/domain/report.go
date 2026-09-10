package domain

import "time"

const SchemaVersion = 1

type Level string

const (
	LevelOK      Level = "OK"
	LevelWarning Level = "WARNING"
	LevelError   Level = "ERROR"
)

type LabMeta struct {
	SchemaVersion int    `json:"schemaVersion"`
	Version       string `json:"version"`
	Controller    struct {
		Name     string `json:"name"`
		Number   int    `json:"number"`
		StaticIP string `json:"staticIp"`
		DHCPIP   string `json:"dhcpIp"`
	} `json:"controller"`
	Clients struct {
		Count int        `json:"count"`
		Hosts []HostMeta `json:"hosts"`
	} `json:"clients"`
	Network struct {
		Base         string `json:"base"`
		PrefixLength int    `json:"prefixLength"`
		Interface    string `json:"ifaceName"`
		CachePort    int    `json:"cachePort"`
		PXEHTTPPort  int    `json:"pxeHttpPort"`
	} `json:"network"`
}

type HostMeta struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type DeploymentStatus struct {
	Ready  bool     `json:"ready"`
	Issues []string `json:"issues"`
}

type GitState struct {
	Available bool `json:"available"`
	Dirty     bool `json:"dirty"`
	Changes   int  `json:"changes"`
}

type ServiceState struct {
	Name   string `json:"name"`
	Loaded bool   `json:"loaded"`
	Active bool   `json:"active"`
	State  string `json:"state"`
}

type ArtifactState struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Present bool   `json:"present"`
}

type CacheKeyState struct {
	PrivatePresent bool   `json:"privatePresent"`
	PublicPresent  bool   `json:"publicPresent"`
	PrivateMode    uint32 `json:"privateMode,omitempty"`
	Matches        bool   `json:"matches"`
}

type PortUse struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

type StatusReport struct {
	SchemaVersion int              `json:"schemaVersion"`
	Operation     string           `json:"operation"`
	GeneratedAt   time.Time        `json:"generatedAt"`
	State         string           `json:"state"`
	Repository    string           `json:"repository"`
	Meta          LabMeta          `json:"lab"`
	Deployment    DeploymentStatus `json:"deployment"`
	Git           GitState         `json:"git"`
	Services      []ServiceState   `json:"services"`
	Artifacts     []ArtifactState  `json:"artifacts"`
	Warnings      []string         `json:"warnings,omitempty"`
}

type Finding struct {
	ID          string `json:"id"`
	Level       Level  `json:"level"`
	Summary     string `json:"summary"`
	Evidence    string `json:"evidence,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

type DoctorReport struct {
	SchemaVersion int       `json:"schemaVersion"`
	Operation     string    `json:"operation"`
	GeneratedAt   time.Time `json:"generatedAt"`
	State         string    `json:"state"`
	Repository    string    `json:"repository"`
	Findings      []Finding `json:"findings"`
}

func (r DoctorReport) HasErrors() bool {
	for _, finding := range r.Findings {
		if finding.Level == LevelError {
			return true
		}
	}
	return false
}
