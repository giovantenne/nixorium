package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// HostNetwork says which laboratory address a computer answered on during a
// change of laboratory network.
type HostNetwork string

const (
	HostNetworkCurrent  HostNetwork = "current"
	HostNetworkPrevious HostNetwork = "previous"
	HostNetworkUnknown  HostNetwork = "unknown"
)

// UnmarshalJSON decodes labMeta and copies each client's previous address
// from Network.Transition into the host itself, so every consumer of the
// inventory carries both addresses without evaluating anything else.
func (meta *LabMeta) UnmarshalJSON(data []byte) error {
	type plain LabMeta
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*meta = LabMeta(decoded)
	meta.attachPreviousAddresses()
	return nil
}

func (meta *LabMeta) attachPreviousAddresses() {
	transition := meta.Network.Transition
	if transition == nil {
		return
	}
	previous := make(map[string]string, len(transition.Clients))
	for _, host := range transition.Clients {
		previous[host.Name] = host.IP
	}
	for index := range meta.Clients.Hosts {
		host := &meta.Clients.Hosts[index]
		if address := previous[host.Name]; address != "" && address != host.IP {
			host.PreviousIP = address
		}
	}
}

// Steps of the guided change of laboratory network.
const (
	NetworkMigrationStart  = "start"
	NetworkMigrationFinish = "finish"
	NetworkMigrationUndo   = "undo"
)

// States of one computer in a network change review or status.
const (
	// Review of start: the computer answers, or it is off and acknowledged.
	MigrationComputerReachable    = "reachable"
	MigrationComputerOff          = "off"
	MigrationComputerAcknowledged = "acknowledged"
	// Status during the change.
	MigrationComputerMigrated = "migrated"
	MigrationComputerOutdated = "outdated"
	MigrationComputerPrevious = "previous"
	MigrationComputerWaiting  = "waiting"
	MigrationComputerUnknown  = "unknown"
)

// NetworkMigrationComputer is one client in a review or status: its address
// before and after the step and what Nixorium observed.
type NetworkMigrationComputer struct {
	Name         string       `json:"name"`
	From         string       `json:"from"`
	To           string       `json:"to"`
	Reachability Reachability `json:"reachability,omitempty"`
	Network      HostNetwork  `json:"network,omitempty"`
	Revision     string       `json:"revision,omitempty"`
	State        string       `json:"state"`
	Detail       string       `json:"detail,omitempty"`
}

// NetworkMigrationPlan reviews one step of the change of laboratory network.
// Its token binds the action, the networks, the saved settings and revision,
// and the computers with their observed state.
type NetworkMigrationPlan struct {
	SchemaVersion  int                        `json:"schemaVersion"`
	Operation      string                     `json:"operation"`
	Action         string                     `json:"action"`
	State          string                     `json:"state"`
	Repository     string                     `json:"repository"`
	Revision       string                     `json:"revision,omitempty"`
	From           string                     `json:"from,omitempty"`
	To             string                     `json:"to,omitempty"`
	ControllerFrom string                     `json:"controllerFrom,omitempty"`
	ControllerTo   string                     `json:"controllerTo,omitempty"`
	Computers      []NetworkMigrationComputer `json:"computers"`
	Acknowledged   []string                   `json:"acknowledged"`
	Steps          []string                   `json:"steps"`
	ReviewToken    string                     `json:"reviewToken,omitempty"`
	Message        string                     `json:"message,omitempty"`
	Issues         []ValidationIssue          `json:"issues"`
	// Candidate and Settings carry the reviewed settings to the save; they
	// stay out of the JSON report, which already lists the networks.
	Candidate *LabSettingsFile `json:"-"`
	Settings  ConfigPlanReport `json:"-"`
}

func (p NetworkMigrationPlan) HasErrors() bool { return len(p.Issues) > 0 || p.State != "ready" }

// Off lists the computers that did not answer the review.
func (p NetworkMigrationPlan) Off() []string {
	names := []string{}
	for _, computer := range p.Computers {
		if computer.State == MigrationComputerOff || computer.State == MigrationComputerAcknowledged {
			names = append(names, computer.Name)
		}
	}
	return names
}

// NetworkMigrationResult is the outcome of saving one reviewed step. Saving
// changes no computer; Next names the reviewed operations that follow.
type NetworkMigrationResult struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Operation        string            `json:"operation"`
	Action           string            `json:"action"`
	State            string            `json:"state"`
	Repository       string            `json:"repository"`
	Revision         string            `json:"revision,omitempty"`
	RecoveryRequired bool              `json:"recoveryRequired,omitempty"`
	Rekeyed          []string          `json:"rekeyed,omitempty"`
	Message          string            `json:"message"`
	Next             []string          `json:"next"`
	Issues           []ValidationIssue `json:"issues"`
}

func (r NetworkMigrationResult) HasErrors() bool { return len(r.Issues) > 0 }

// NetworkMigrationStatus describes a change of laboratory network in
// progress, computer by computer, and the step that comes next.
type NetworkMigrationStatus struct {
	SchemaVersion   int                        `json:"schemaVersion"`
	Operation       string                     `json:"operation"`
	State           string                     `json:"state"`
	Repository      string                     `json:"repository"`
	Revision        string                     `json:"revision,omitempty"`
	From            string                     `json:"from,omitempty"`
	To              string                     `json:"to,omitempty"`
	StartedAt       string                     `json:"startedAt,omitempty"`
	ControllerFrom  string                     `json:"controllerFrom,omitempty"`
	ControllerTo    string                     `json:"controllerTo,omitempty"`
	ControllerReady bool                       `json:"controllerReady"`
	Computers       []NetworkMigrationComputer `json:"computers"`
	Migrated        int                        `json:"migrated"`
	Waiting         int                        `json:"waiting"`
	Previous        int                        `json:"previous"`
	Outdated        int                        `json:"outdated"`
	Unknown         int                        `json:"unknown"`
	CanFinish       bool                       `json:"canFinish"`
	Next            string                     `json:"next,omitempty"`
	Message         string                     `json:"message,omitempty"`
	Issues          []ValidationIssue          `json:"issues"`
}

func (s NetworkMigrationStatus) HasErrors() bool { return len(s.Issues) > 0 }

// InProgress reports a saved change of laboratory network.
func (s NetworkMigrationStatus) InProgress() bool { return s.State == "in-progress" }

// ParseLabNetwork reads "10.1.0.0/24" (or a base and a separate prefix) as a
// canonical IPv4 laboratory network.
func ParseLabNetwork(value string) (string, int, error) {
	value = strings.TrimSpace(value)
	base, prefixText, found := strings.Cut(value, "/")
	if !found {
		return "", 0, errors.New("write the network as an address and a prefix length, for example 10.1.0.0/24")
	}
	prefix, err := strconv.Atoi(prefixText)
	if err != nil {
		return "", 0, errors.New("the prefix length must be a number from 1 to 30")
	}
	return CheckLabNetwork(base, prefix)
}

// CheckLabNetwork validates a laboratory network base and prefix.
func CheckLabNetwork(base string, prefix int) (string, int, error) {
	parsed, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", strings.TrimSpace(base), prefix))
	if err != nil || !parsed.Addr().Is4() {
		return "", 0, errors.New("the network must be an IPv4 address such as 10.1.0.0 with a prefix length from 1 to 30")
	}
	if prefix < 1 || prefix > 30 {
		return "", 0, errors.New("the prefix length must be a number from 1 to 30")
	}
	if parsed.Masked() != parsed {
		return "", 0, fmt.Errorf("the network address must end where the prefix starts; for /%d use %s", prefix, parsed.Masked().Addr())
	}
	return parsed.Addr().String(), prefix, nil
}

// LabNetworkText is the network written as base/prefix.
func LabNetworkText(base string, prefix int) string {
	return fmt.Sprintf("%s/%d", base, prefix)
}

// ClientName is the inventory name of client number n (pc01, pc02, …).
func ClientName(number int) string {
	return fmt.Sprintf("pc%02d", number)
}

// MigrationAddresses lists, for the saved settings and a target network, the
// controller's and each client's address before and after.
func MigrationAddresses(lab LabSettings, toBase string, toPrefix int) (controllerFrom, controllerTo string, clients []NetworkMigrationComputer, err error) {
	target := lab
	target.NetworkBase, target.NetworkPrefix = toBase, toPrefix
	if controllerFrom, err = LabHostAddress(lab, lab.MasterHostNumber); err != nil {
		return "", "", nil, err
	}
	if controllerTo, err = LabHostAddress(target, lab.MasterHostNumber); err != nil {
		return "", "", nil, err
	}
	for number := 1; number <= lab.PCCount; number++ {
		from, fromErr := LabHostAddress(lab, number)
		to, toErr := LabHostAddress(target, number)
		if fromErr != nil || toErr != nil {
			return "", "", nil, fmt.Errorf("computer %s does not fit in the new network", ClientName(number))
		}
		clients = append(clients, NetworkMigrationComputer{Name: ClientName(number), From: from, To: to})
	}
	return controllerFrom, controllerTo, clients, nil
}

// StartNetworkCandidate moves the saved settings to a new laboratory network
// and records the saved one as the previous network.
func StartNetworkCandidate(saved LabSettingsFile, base string, prefix int, startedAt time.Time) (LabSettingsFile, error) {
	if saved.Lab.NetworkTransition != nil {
		return LabSettingsFile{}, errors.New("a change of laboratory network is already in progress; finish or undo it first")
	}
	if saved.Lab.PCCount <= 0 {
		return LabSettingsFile{}, errors.New("no client computers are configured; change the network in Settings instead")
	}
	if base == saved.Lab.NetworkBase && prefix == saved.Lab.NetworkPrefix {
		return LabSettingsFile{}, errors.New("this is already the laboratory network; choose a different one")
	}
	candidate := saved
	candidate.Lab.NetworkBase, candidate.Lab.NetworkPrefix = base, prefix
	candidate.Lab.NetworkTransition = &NetworkTransition{
		PreviousBase:         saved.Lab.NetworkBase,
		PreviousPrefixLength: saved.Lab.NetworkPrefix,
		StartedAt:            startedAt.UTC().Truncate(time.Second).Format(time.RFC3339),
	}
	return candidate, nil
}

// FinishNetworkCandidate keeps the new network and removes the record: the
// controller drops its previous address and clients trust only the new one.
func FinishNetworkCandidate(saved LabSettingsFile) (LabSettingsFile, error) {
	if saved.Lab.NetworkTransition == nil {
		return LabSettingsFile{}, errors.New("no change of laboratory network is in progress")
	}
	candidate := saved
	candidate.Lab.NetworkTransition = nil
	return candidate, nil
}

// UndoNetworkCandidate restores the previous network as the laboratory
// network and removes the record.
func UndoNetworkCandidate(saved LabSettingsFile) (LabSettingsFile, error) {
	transition := saved.Lab.NetworkTransition
	if transition == nil {
		return LabSettingsFile{}, errors.New("no change of laboratory network is in progress")
	}
	candidate := saved
	candidate.Lab.NetworkBase, candidate.Lab.NetworkPrefix = transition.PreviousBase, transition.PreviousPrefixLength
	candidate.Lab.NetworkTransition = nil
	return candidate, nil
}

// KnownHostMove names a computer whose trusted host keys are recorded for a
// new laboratory address as well.
type KnownHostMove struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// KnownHostRekey lists, by computer, what recording the keys for the new
// addresses did: Added copied them, Already found them, Missing had no
// trusted key for the previous address and Conflicts found another key
// recorded for the new address, which stays refused.
type KnownHostRekey struct {
	Added     []string `json:"added,omitempty"`
	Already   []string `json:"already,omitempty"`
	Missing   []string `json:"missing,omitempty"`
	Conflicts []string `json:"conflicts,omitempty"`
}

// TransitionKnownHostMoves lists the address moves of a change of laboratory
// network from the inventory: each client's previous address to its new one.
func TransitionKnownHostMoves(meta LabMeta) []KnownHostMove {
	moves := []KnownHostMove{}
	for _, host := range meta.Clients.Hosts {
		if host.PreviousIP != "" && host.PreviousIP != host.IP {
			moves = append(moves, KnownHostMove{Name: host.Name, From: host.PreviousIP, To: host.IP})
		}
	}
	return moves
}

// NetworkChangingDetail is the Overview detail of NETWORK-CHANGING.
func NetworkChangingDetail(saved LabSettings, controllerApplied bool) string {
	transition := saved.NetworkTransition
	if transition == nil {
		return ""
	}
	from := LabNetworkText(transition.PreviousBase, transition.PreviousPrefixLength)
	to := LabNetworkText(saved.NetworkBase, saved.NetworkPrefix)
	next := "Next: Apply to controller, so it answers on both networks."
	if controllerApplied {
		next = "Next: open it to update the computers, then finish or undo the change."
	}
	return fmt.Sprintf("From %s to %s. %s", from, to, next)
}
