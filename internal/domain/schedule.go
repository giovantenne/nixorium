package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Scheduled switch-on and shut-down of the student computers: an operational
// aid like blocking the Internet, not laboratory configuration. The schedule
// lives on the controller only (outside Git and the backup) and applies to
// every client computer: at most one switch-on time and one shut-down time
// for a set of weekdays, in the controller's time zone.

// Classroom operations of the schedule. Setting an empty schedule clears it.
const (
	ClassroomScheduleGetOperation ClassroomOperation = "schedule-get"
	ClassroomScheduleSetOperation ClassroomOperation = "schedule-set"
)

// scheduleOperations are the classroom operations of this file.
var scheduleOperations = map[ClassroomOperation]bool{
	ClassroomScheduleGetOperation: true,
	ClassroomScheduleSetOperation: true,
}

// ScheduleWeekdays are the weekday codes, Monday first.
var ScheduleWeekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

var scheduleWeekdayNames = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

var (
	scheduleTimePattern   = regexp.MustCompile(`^([0-9]{1,2}):([0-9]{2})$`)
	scheduleMinutePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}$`)
)

// Schedule is what a person sets: times as HH:MM on the 24-hour clock of the
// controller (empty means that half is off) and the weekdays both apply to.
type Schedule struct {
	On   string   `json:"on,omitempty"`
	Off  string   `json:"off,omitempty"`
	Days []string `json:"days"`
}

// Empty tells that neither a switch-on nor a shut-down is scheduled.
func (s Schedule) Empty() bool { return s.On == "" && s.Off == "" }

// NormalizeScheduleTime accepts H:MM or HH:MM and returns HH:MM.
func NormalizeScheduleTime(value string) (string, error) {
	match := scheduleTimePattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return "", fmt.Errorf("%q is not a time; write it as HH:MM, for example 07:50", value)
	}
	var hour, minute int
	fmt.Sscanf(match[1], "%d", &hour)
	fmt.Sscanf(match[2], "%d", &minute)
	if hour > 23 || minute > 59 {
		return "", fmt.Errorf("%q is not a time of day; use 00:00 to 23:59", value)
	}
	return fmt.Sprintf("%02d:%02d", hour, minute), nil
}

// scheduleWeekdayIndex accepts mon..sun and the English day names.
func scheduleWeekdayIndex(word string) int {
	word = strings.ToLower(strings.TrimSpace(word))
	for index, code := range ScheduleWeekdays {
		if word == code || word == strings.ToLower(scheduleWeekdayNames[index]) {
			return index
		}
	}
	return -1
}

// ParseScheduleDays reads "all", a range such as mon-fri (sat-mon wraps
// over the weekend), or a comma-separated list that may mix both, such as
// mon-wed,fri. The result is in week order without repeats.
func ParseScheduleDays(value string) ([]string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil, errors.New("choose the weekdays: mon-fri, all, or a list such as mon,wed,fri")
	}
	chosen := [7]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "all" {
			for index := range chosen {
				chosen[index] = true
			}
			continue
		}
		first, last, isRange := strings.Cut(part, "-")
		start := scheduleWeekdayIndex(first)
		end := start
		if isRange {
			end = scheduleWeekdayIndex(last)
		}
		if start < 0 || end < 0 {
			return nil, fmt.Errorf("%q is not a weekday; use mon, tue, wed, thu, fri, sat, sun, a range such as mon-fri, or all", part)
		}
		for index := start; ; index = (index + 1) % 7 {
			chosen[index] = true
			if index == end {
				break
			}
		}
	}
	return scheduleDaysFrom(chosen), nil
}

func scheduleDaysFrom(chosen [7]bool) []string {
	days := []string{}
	for index, code := range ScheduleWeekdays {
		if chosen[index] {
			days = append(days, code)
		}
	}
	return days
}

// Normalize checks a schedule and returns it in canonical form: times as
// HH:MM and weekdays in week order. A schedule without times is the empty
// schedule, whatever its weekdays.
func (s Schedule) Normalize() (Schedule, error) {
	result := Schedule{Days: []string{}}
	var err error
	if s.On != "" {
		if result.On, err = NormalizeScheduleTime(s.On); err != nil {
			return Schedule{}, fmt.Errorf("switch-on time: %w", err)
		}
	}
	if s.Off != "" {
		if result.Off, err = NormalizeScheduleTime(s.Off); err != nil {
			return Schedule{}, fmt.Errorf("shut-down time: %w", err)
		}
	}
	if result.Empty() {
		return result, nil
	}
	if result.On == result.Off {
		return Schedule{}, errors.New("the switch-on and the shut-down time must differ")
	}
	chosen := [7]bool{}
	for _, day := range s.Days {
		index := scheduleWeekdayIndex(day)
		if index < 0 {
			return Schedule{}, fmt.Errorf("%q is not a weekday", day)
		}
		chosen[index] = true
	}
	result.Days = scheduleDaysFrom(chosen)
	if len(result.Days) == 0 {
		return Schedule{}, errors.New("choose at least one weekday")
	}
	return result, nil
}

// ScheduleMinute names a local minute the way the record keeps the last one
// that ran, for example 2026-10-15T07:50.
func ScheduleMinute(now time.Time) string { return now.Format("2006-01-02T15:04") }

// Due tells which half of the schedule starts in the minute of now, read in
// now's own time zone (the controller's, when now is time.Now()).
func (s Schedule) Due(now time.Time) (on, off bool) {
	day := ScheduleWeekdays[(int(now.Weekday())+6)%7]
	listed := false
	for _, candidate := range s.Days {
		listed = listed || candidate == day
	}
	if !listed {
		return false, false
	}
	clock := now.Format("15:04")
	return s.On != "" && s.On == clock, s.Off != "" && s.Off == clock
}

// FormatScheduleDays says "every day", "Monday to Friday" for three or more
// consecutive days, or lists the days.
func FormatScheduleDays(days []string) string {
	chosen := [7]bool{}
	for _, day := range days {
		if index := scheduleWeekdayIndex(day); index >= 0 {
			chosen[index] = true
		}
	}
	names := []string{}
	first, last := -1, -1
	for index := range chosen {
		if chosen[index] {
			names = append(names, scheduleWeekdayNames[index])
			if first < 0 {
				first = index
			}
			last = index
		}
	}
	switch {
	case len(names) == 7:
		return "every day"
	case len(names) >= 3 && last-first+1 == len(names):
		return scheduleWeekdayNames[first] + " to " + scheduleWeekdayNames[last]
	case len(names) == 0:
		return "no day"
	case len(names) == 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// Summary describes the schedule in one sentence.
func (s Schedule) Summary() string {
	days := FormatScheduleDays(s.Days)
	switch {
	case s.On != "" && s.Off != "":
		return fmt.Sprintf("Switch on at %s and shut down at %s, %s.", s.On, s.Off, days)
	case s.On != "":
		return fmt.Sprintf("Switch on at %s, %s.", s.On, days)
	case s.Off != "":
		return fmt.Sprintf("Shut down at %s, %s.", s.Off, days)
	}
	return "No schedule."
}

// ScheduleRecord is the content of the schedule file: the schedule and the
// local minute of the last switch-on or shut-down that was started, so that
// one minute never runs twice.
type ScheduleRecord struct {
	SchemaVersion int `json:"schemaVersion"`
	Schedule
	LastRun string `json:"lastRun,omitempty"`
}

// Valid tells whether the record is well formed: a canonical schedule and a
// well-formed last minute.
func (r ScheduleRecord) Valid() bool {
	normalized, err := r.Schedule.Normalize()
	if err != nil || normalized.On != r.On || normalized.Off != r.Off || len(normalized.Days) != len(r.Days) {
		return false
	}
	for index, day := range normalized.Days {
		if r.Days[index] != day {
			return false
		}
	}
	return r.LastRun == "" || scheduleMinutePattern.MatchString(r.LastRun)
}

// ScheduleReport is the answer to showing or setting the schedule.
type ScheduleReport struct {
	SchemaVersion int      `json:"schemaVersion"`
	Operation     string   `json:"operation"`
	State         string   `json:"state"`
	Schedule      Schedule `json:"schedule"`
	// Zone is the controller's time zone, such as CEST.
	Zone    string            `json:"zone,omitempty"`
	Summary string            `json:"summary"`
	Message string            `json:"message"`
	Issues  []ValidationIssue `json:"issues"`
}

func (r ScheduleReport) HasErrors() bool { return r.State != "completed" }

// Scheduled action of a run.
const (
	ScheduleActionOn  = "on"
	ScheduleActionOff = "off"
)

// Outcomes of a computer in a scheduled run.
const (
	// Switch-on.
	ScheduleOutcomeSent           = "sent"
	ScheduleOutcomeAlreadyOn      = "already-on"
	ScheduleOutcomeUnknownAddress = "unknown-address"
	ScheduleOutcomeNotSent        = "not-sent"
	// Shut-down.
	ScheduleOutcomeAccepted       = "accepted"
	ScheduleOutcomeInUse          = "in-use"
	ScheduleOutcomeNotReachable   = "not-reachable"
	ScheduleOutcomeSessionUnknown = "session-unknown"
	ScheduleOutcomeUnconfirmed    = "unconfirmed"
)

type ScheduleOutcome struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// ScheduleRunReport is what one run of the timer did. State is idle when
// nothing was due (or the minute already ran), completed, partial when a
// computer's request failed or could not be confirmed, or blocked when
// nothing could be done (the inventory could not be read, another client
// operation is running, a network installation is active).
type ScheduleRunReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Minute        string            `json:"minute"`
	Action        string            `json:"action,omitempty"`
	Targets       []ScheduleOutcome `json:"targets"`
	Message       string            `json:"message,omitempty"`
}

func (r ScheduleRunReport) HasErrors() bool { return r.State == "partial" || r.State == "blocked" }
