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
	DeploymentMode string `json:"deploymentMode,omitempty"`
	SchemaVersion  int    `json:"schemaVersion"`
	Version        string `json:"version"`
	Controller     struct {
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

type Reachability string

const (
	ReachabilityReachable   Reachability = "reachable"
	ReachabilityUnreachable Reachability = "unreachable"
	ReachabilityUnknown     Reachability = "unknown"
)

type SSHAvailability string

const (
	SSHAvailable   SSHAvailability = "available"
	SSHUnavailable SSHAvailability = "unavailable"
	SSHUnknown     SSHAvailability = "unknown"
)

type SSHProbe struct {
	Reachability Reachability    `json:"reachability"`
	SSH          SSHAvailability `json:"ssh"`
	Detail       string          `json:"detail,omitempty"`
}

type DeploymentCurrency string

const (
	DeploymentCurrent  DeploymentCurrency = "current"
	DeploymentOutdated DeploymentCurrency = "outdated"
	DeploymentUnknown  DeploymentCurrency = "unknown"
)

type HostSystemProbe struct {
	SystemPath string `json:"systemPath,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type HostDeploymentSummary struct {
	Current  int `json:"current"`
	Outdated int `json:"outdated"`
	Unknown  int `json:"unknown"`
}

type HostStatus struct {
	Name                 string                    `json:"name"`
	IP                   string                    `json:"ip"`
	Role                 string                    `json:"role"`
	Reachability         Reachability              `json:"reachability"`
	SSH                  SSHAvailability           `json:"ssh"`
	Deployment           DeploymentCurrency        `json:"deployment"`
	CurrentSystem        string                    `json:"currentSystem,omitempty"`
	CurrentRevision      string                    `json:"currentRevision,omitempty"`
	DesiredRevision      string                    `json:"desiredRevision,omitempty"`
	Detail               string                    `json:"detail,omitempty"`
	DeploymentDetail     string                    `json:"deploymentDetail,omitempty"`
	LastSuccessfulDeploy *LastSuccessfulDeployment `json:"lastSuccessfulDeploy,omitempty"`
}

type HostsReport struct {
	SchemaVersion   int                   `json:"schemaVersion"`
	Operation       string                `json:"operation"`
	GeneratedAt     time.Time             `json:"generatedAt"`
	State           string                `json:"state"`
	Repository      string                `json:"repository"`
	DesiredRevision string                `json:"desiredRevision,omitempty"`
	Deployment      HostDeploymentSummary `json:"deployment"`
	HistoryDetail   string                `json:"historyDetail,omitempty"`
	Hosts           []HostStatus          `json:"hosts"`
}

type DeploymentStatus struct {
	Ready      bool                 `json:"ready"`
	Issues     []string             `json:"issues"`
	Controller *ControllerReadiness `json:"controller,omitempty"`
}

type ControllerReadiness struct {
	Ready        bool     `json:"ready"`
	Issues       []string `json:"issues"`
	RequiresKeys bool     `json:"requiresKeys"`
}

// Older upstreams have only fleet readiness. Preserve that strict fallback.
func (s DeploymentStatus) ControllerReadiness() ControllerReadiness {
	if s.Controller != nil {
		return *s.Controller
	}
	return ControllerReadiness{Ready: s.Ready, Issues: s.Issues, RequiresKeys: true}
}

type GitState struct {
	Available bool     `json:"available"`
	Dirty     bool     `json:"dirty"`
	Changes   int      `json:"changes"`
	Paths     []string `json:"paths,omitempty"`
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
	SchemaVersion  int                 `json:"schemaVersion"`
	Operation      string              `json:"operation"`
	GeneratedAt    time.Time           `json:"generatedAt"`
	State          string              `json:"state"`
	Repository     string              `json:"repository"`
	Meta           LabMeta             `json:"lab"`
	Deployment     DeploymentStatus    `json:"deployment"`
	Git            GitState            `json:"git"`
	Services       []ServiceState      `json:"services"`
	PXE            PXELifecycleState   `json:"pxe"`
	PXEPreparation PXEPreparationState `json:"pxePreparation"`
	Artifacts      []ArtifactState     `json:"artifacts"`
	Warnings       []string            `json:"warnings,omitempty"`
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
