package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// CleanupKeepGenerations is the number of newest system generations the
// cleanup helper keeps on every computer (ADR 0022). The GRUB menu lists the
// same number of entries.
const CleanupKeepGenerations = 10

// CleanupConfirmation is the word typed to confirm a reviewed cleanup.
const CleanupConfirmation = "CLEAN"

// CleanupControllerTarget selects the controller in a cleanup request.
const CleanupControllerTarget = "controller"

type CleanupGeneration struct {
	Number int    `json:"number"`
	Reason string `json:"reason"`
}

// CleanupObservation is one computer's read-only answer from
// nixorium-clean-generations --plan.
type CleanupObservation struct {
	Reachability Reachability        `json:"reachability"`
	SSH          SSHAvailability     `json:"ssh"`
	Valid        bool                `json:"valid"`
	Keep         []CleanupGeneration `json:"keep"`
	Remove       []int               `json:"remove"`
	FreeBytes    uint64              `json:"freeBytes"`
	Expect       string              `json:"expect,omitempty"`
	Detail       string              `json:"detail,omitempty"`
}

type CleanupTargetPlan struct {
	Name         string              `json:"name"`
	IP           string              `json:"ip,omitempty"`
	Controller   bool                `json:"controller"`
	Reachability Reachability        `json:"reachability"`
	SSH          SSHAvailability     `json:"ssh"`
	Eligible     bool                `json:"eligible"`
	Keep         []CleanupGeneration `json:"keep"`
	Remove       []int               `json:"remove"`
	FreeBytes    uint64              `json:"freeBytes"`
	Expect       string              `json:"expect,omitempty"`
	Detail       string              `json:"detail,omitempty"`
}

type CleanupPlanReport struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	Operation       string              `json:"operation"`
	State           string              `json:"state"`
	Repository      string              `json:"repository"`
	Requested       string              `json:"requested"`
	KeepGenerations int                 `json:"keepGenerations"`
	Targets         []CleanupTargetPlan `json:"targets"`
	Eligible        int                 `json:"eligible"`
	ExpiresAt       time.Time           `json:"expiresAt,omitempty"`
	ReviewToken     string              `json:"reviewToken,omitempty"`
	Confirmation    string              `json:"confirmation,omitempty"`
	Message         string              `json:"message,omitempty"`
	Issues          []ValidationIssue   `json:"issues"`
}

func (r CleanupPlanReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || len(r.Issues) > 0
}

// CleanupDispatchResult is one computer's answer from --apply.
type CleanupDispatchResult struct {
	// State is cleaned, unchanged, changed or failed.
	State          string
	Removed        []int
	FreeBefore     uint64
	FreeAfter      uint64
	BootMenuFailed bool
	Detail         string
}

type CleanupTargetOutcome struct {
	Name            string `json:"name"`
	State           string `json:"state"`
	Removed         []int  `json:"removed,omitempty"`
	FreeBefore      uint64 `json:"freeBefore,omitempty"`
	FreeAfter       uint64 `json:"freeAfter,omitempty"`
	Detail          string `json:"detail,omitempty"`
	TechnicalDetail string `json:"technicalDetail,omitempty"`
}

type CleanupApplyReport struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Operation     string                 `json:"operation"`
	State         string                 `json:"state"`
	Repository    string                 `json:"repository"`
	Requested     string                 `json:"requested"`
	Targets       []CleanupTargetOutcome `json:"targets"`
	Cleaned       int                    `json:"cleaned"`
	Unchanged     int                    `json:"unchanged"`
	NotSent       int                    `json:"notSent"`
	Unconfirmed   int                    `json:"unconfirmed"`
	Message       string                 `json:"message,omitempty"`
	Issues        []ValidationIssue      `json:"issues"`
}

func (r CleanupApplyReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || r.State == "partial" || len(r.Issues) > 0
}

func CleanupReviewToken(report CleanupPlanReport) string {
	type boundTarget struct {
		Name       string
		IP         string
		Controller bool
		Eligible   bool
		Remove     []int
		Expect     string
	}
	targets := make([]boundTarget, 0, len(report.Targets))
	for _, target := range report.Targets {
		targets = append(targets, boundTarget{target.Name, target.IP, target.Controller, target.Eligible, target.Remove, target.Expect})
	}
	bound := struct {
		Repository string
		Requested  string
		Keep       int
		Targets    []boundTarget
		ExpiresAt  time.Time
	}{report.Repository, report.Requested, report.KeepGenerations, targets, report.ExpiresAt.UTC()}
	content, _ := json.Marshal(bound)
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}

// ParseCleanupPlan reads the helper's --plan contract. Any unexpected line
// makes the observation invalid.
func ParseCleanupPlan(output string) (CleanupObservation, bool) {
	observation := CleanupObservation{Keep: []CleanupGeneration{}, Remove: []int{}}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 || lines[0] != "format 1" {
		return observation, false
	}
	free, expect := false, false
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		switch {
		case len(fields) == 3 && fields[0] == "keep":
			number, ok := cleanupNumber(fields[1])
			if !ok || !cleanupReason(fields[2]) {
				return observation, false
			}
			observation.Keep = append(observation.Keep, CleanupGeneration{Number: number, Reason: fields[2]})
		case len(fields) == 2 && fields[0] == "remove":
			number, ok := cleanupNumber(fields[1])
			if !ok {
				return observation, false
			}
			observation.Remove = append(observation.Remove, number)
		case len(fields) == 2 && fields[0] == "free":
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return observation, false
			}
			observation.FreeBytes, free = value, true
		case len(fields) == 2 && fields[0] == "expect" && cleanupDigest(fields[1]):
			observation.Expect, expect = fields[1], true
		default:
			return observation, false
		}
	}
	observation.Valid = free && expect
	return observation, observation.Valid
}

// ParseCleanupApply reads the helper's --apply contract.
func ParseCleanupApply(output string) (CleanupDispatchResult, bool) {
	result := CleanupDispatchResult{}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "changed" {
		result.State = "changed"
		return result, true
	}
	if len(lines) < 3 || lines[0] != "format 1" {
		return result, false
	}
	before, after := false, false
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		switch {
		case len(fields) == 1 && fields[0] == "unchanged":
			result.State = "unchanged"
		case len(fields) >= 2 && fields[0] == "removed":
			for _, field := range fields[1:] {
				number, ok := cleanupNumber(field)
				if !ok {
					return result, false
				}
				result.Removed = append(result.Removed, number)
			}
			result.State = "cleaned"
		case len(fields) == 2 && (fields[0] == "free-before" || fields[0] == "free-after"):
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return result, false
			}
			if fields[0] == "free-before" {
				result.FreeBefore, before = value, true
			} else {
				result.FreeAfter, after = value, true
			}
		case len(fields) == 2 && fields[0] == "boot-menu" && (fields[1] == "ok" || fields[1] == "failed"):
			result.BootMenuFailed = fields[1] == "failed"
		default:
			return result, false
		}
	}
	return result, result.State != "" && before && after
}

func cleanupNumber(value string) (int, bool) {
	number, err := strconv.Atoi(value)
	return number, err == nil && number > 0 && strconv.Itoa(number) == value
}

func cleanupReason(value string) bool {
	return value == "newest" || value == "current" || value == "running" || value == "booted"
}

func cleanupDigest(value string) bool {
	if len(value) != 16 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
