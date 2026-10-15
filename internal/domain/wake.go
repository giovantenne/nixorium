package domain

import (
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"time"
)

// Wake-on-LAN: the controller switches on student computers with a magic
// packet sent to the hardware address of their laboratory network card, which
// it learned while they were on. Sending a packet is not proof that a
// computer starts: the firmware must allow it and the computer must still
// have standby power.

// WakePort is the UDP port of the magic packet (the discard service).
const WakePort = 9

// WakeAddress is the hardware address of a computer's laboratory network
// card, as the controller last observed it.
type WakeAddress struct {
	MAC       string    `json:"mac"`
	LearnedAt time.Time `json:"learnedAt"`
}

// NormalizeMAC accepts a 48-bit Ethernet address written with colons,
// hyphens or dots, or as twelve hexadecimal digits, and returns it in
// lowercase colon form. The zero address and group (multicast or broadcast)
// addresses are refused: they never belong to one network card.
func NormalizeMAC(value string) (string, error) {
	value = strings.TrimSpace(value)
	hardware, err := net.ParseMAC(value)
	if err != nil && len(value) == 12 {
		hardware, err = hex.DecodeString(value)
	}
	if err != nil || len(hardware) != 6 {
		return "", errors.New("not a 48-bit Ethernet hardware address")
	}
	if hardware[0]&1 == 1 {
		return "", errors.New("a group address cannot switch on one computer")
	}
	zero := true
	for _, part := range hardware {
		zero = zero && part == 0
	}
	if zero {
		return "", errors.New("the zero hardware address is not a network card")
	}
	return net.HardwareAddr(hardware).String(), nil
}

// MagicPacket returns the standard Wake-on-LAN payload: six 0xFF bytes
// followed by the hardware address sixteen times (102 bytes).
func MagicPacket(mac string) ([]byte, error) {
	normalized, err := NormalizeMAC(mac)
	if err != nil {
		return nil, err
	}
	hardware, _ := net.ParseMAC(normalized)
	packet := make([]byte, 0, 6+16*6)
	for range 6 {
		packet = append(packet, 0xff)
	}
	for range 16 {
		packet = append(packet, hardware...)
	}
	return packet, nil
}

// DirectedBroadcast returns the broadcast address of an IPv4 network, or ""
// when the base is not IPv4 or the network has no broadcast address (/31
// and /32).
func DirectedBroadcast(base string, prefixLength int) string {
	address := net.ParseIP(base).To4()
	if address == nil || prefixLength < 1 || prefixLength > 30 {
		return ""
	}
	mask := net.CIDRMask(prefixLength, 32)
	broadcast := make(net.IP, 4)
	for index := range broadcast {
		broadcast[index] = address[index] | ^mask[index]
	}
	return broadcast.String()
}

// WakeNetwork says where the controller sends magic packets: the lab
// subnet's directed broadcast address (empty when unknown), and the
// controller's own lab address and interface to send from when present.
type WakeNetwork struct {
	Broadcast string
	Source    string
	Interface string
}

// Planned state of a computer in a switch-on review.
const (
	// WakeTargetSend: off or not reachable, with a known address.
	WakeTargetSend = "send"
	// WakeTargetOn: it answered on the lab network; nothing is sent.
	WakeTargetOn = "on"
	// WakeTargetUnknown: off, but its address was never learned.
	WakeTargetUnknown = "unknown"
	// WakeTargetInvalid: the name is not a client computer of the lab.
	WakeTargetInvalid = "invalid"
)

// Outcome of a computer after the packets were sent.
const (
	WakeOutcomeSent    = "sent"
	WakeOutcomeOn      = "on"
	WakeOutcomeUnknown = "unknown"
	WakeOutcomeFailed  = "failed"
	WakeOutcomeInvalid = "invalid"
)

// WakeUnknownDetail is what a teacher reads for a computer whose address the
// controller never learned.
const WakeUnknownDetail = "Address not known yet: it is learned while the computer is on and the classroom view is open or the computer inventory runs."

type WakeTarget struct {
	HostMeta
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// WakePlan is the review of a switch-on request. It carries names and
// addresses from the inventory only: applying it reads the hardware
// addresses again from the controller's own record, never from the plan.
type WakePlan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Requested     string            `json:"requested"`
	Targets       []WakeTarget      `json:"targets"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	Message       string            `json:"message"`
	Issues        []ValidationIssue `json:"issues"`
}

func (p WakePlan) HasErrors() bool { return p.State != "ready" || len(p.Issues) > 0 }

// Count returns how many targets have the planned state.
func (p WakePlan) Count(state string) int {
	count := 0
	for _, target := range p.Targets {
		if target.State == state {
			count++
		}
	}
	return count
}

type WakeOutcome struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// WakeReport counts what was sent. Sent means only that the controller put
// the packets on the lab network.
type WakeReport struct {
	SchemaVersion int           `json:"schemaVersion"`
	Operation     string        `json:"operation"`
	State         string        `json:"state"`
	Targets       []WakeOutcome `json:"targets"`
	Sent          int           `json:"sent"`
	AlreadyOn     int           `json:"alreadyOn"`
	Unknown       int           `json:"unknown"`
	Failed        int           `json:"failed"`
	Message       string        `json:"message"`
}

func (r WakeReport) HasErrors() bool { return r.State != "completed" }
