// Package hostdiagnostics collects a fixed, read-only system snapshot. It never
// accepts a file, unit name, executable or SSH destination from an Assistant.
package hostdiagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

const MaxBytes = 32 * 1024

type Config struct {
	Controller bool   `json:"controller"`
	Systemctl  string `json:"systemctl"`
	Journalctl string `json:"journalctl"`
}
type Space struct {
	Mount          string `json:"mount"`
	AvailableBytes uint64 `json:"availableBytes"`
	TotalBytes     uint64 `json:"totalBytes"`
}
type Unit struct {
	Name        string `json:"name"`
	LoadState   string `json:"loadState"`
	ActiveState string `json:"activeState"`
	Result      string `json:"result,omitempty"`
	ExitStatus  *int   `json:"exitStatus,omitempty"`
	ChangedAt   string `json:"changedAt,omitempty"`
}
type Entry struct {
	Unit     string `json:"unit"`
	At       string `json:"at"`
	Priority int    `json:"priority"`
	Excerpt  string `json:"excerpt"`
}
type Report struct {
	SchemaVersion    int      `json:"schemaVersion"`
	Area             string   `json:"area"`
	Controller       bool     `json:"controller"`
	GeneratedAt      string   `json:"generatedAt"`
	BootID           string   `json:"bootId,omitempty"`
	Revision         string   `json:"revision,omitempty"`
	Space            []Space  `json:"space"`
	Units            []Unit   `json:"units"`
	Journal          []Entry  `json:"journal"`
	HomeResetPending *bool    `json:"homeResetPending,omitempty"`
	RestartRequired  *bool    `json:"restartRequired,omitempty"`
	RestartNoticeAt  string   `json:"restartNoticeAt,omitempty"`
	Unavailable      []string `json:"unavailable"`
	Truncated        bool     `json:"truncated"`
}

func ValidArea(area string) bool {
	switch area {
	case "system", "home_reset", "updates", "network", "cache", "pxe":
		return true
	}
	return false
}
func Units(area string, controller bool) []string {
	switch area {
	case "system":
		return []string{"sshd.service", "display-manager.service", "systemd-user-sessions.service", "nix-daemon.service"}
	case "home_reset":
		return []string{"home-reset.service", "systemd-user-sessions.service", "display-manager.service"}
	case "network":
		if controller {
			return []string{"NetworkManager.service", "sshd.service"}
		}
		return []string{"NetworkManager.service", "sshd.service", "nixorium-internet-block.service"}
	case "updates":
		if controller {
			return []string{"nixorium-apply-controller.service", "nixorium-apply-controller@*.service", "nixorium-deferred-updates.service", "nix-daemon.service"}
		}
		return []string{"nix-daemon.service", "home-reset.service"}
	case "cache":
		if controller {
			return []string{"harmonia.service"}
		}
	case "pxe":
		if controller {
			return []string{"nixorium-prepare-pxe.service", "nixorium-pxe.service", "nixorium-pxe-network.service", "nixorium-pxe-recover.service"}
		}
	}
	return nil
}

var applyUnit = regexp.MustCompile(`^nixorium-apply-controller@[0-9a-f]{40}\.service$`)

func allowedUnit(name, area string, controller bool) bool {
	for _, unit := range Units(area, controller) {
		if unit == name || unit == "nixorium-apply-controller@*.service" && applyUnit.MatchString(name) {
			return true
		}
	}
	return false
}

// Free-form journal text cannot be reliably anonymized. Omit credential and
// environment/command lines, scrub common private identifiers, then bound text.
// The consent describes the remaining best-effort privacy limitation.
var sensitive = regexp.MustCompile(`(?i)(password|passwd|passphrase|secret|token|authorization|credential|private.key|BEGIN [A-Z ]*KEY|environment=|execstart=|command.line|https?://[^\s]*@)`)
var privateIdentifiers = regexp.MustCompile(`(?:[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}|(?:https?|ssh|file)://[^\s"'<>]+|/[A-Za-z0-9._~+%:@/\-=]+|SHA256:[A-Za-z0-9+/=]+)`)
var quotedText = regexp.MustCompile(`"[^"\n]*"|'[^'\n]*'`)

func Scrub(text string) string {
	if sensitive.MatchString(text) {
		return "[sensitive journal entry omitted]"
	}
	text = domain.RedactForModel(text)
	text = privateIdentifiers.ReplaceAllString(text, "[REDACTED]")
	text = quotedText.ReplaceAllString(text, "[REDACTED]")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:min(len(runes), 240)]))
}

type runner func(context.Context, string, ...string) ([]byte, bool, error)
type cappedWriter struct {
	data      []byte
	truncated bool
}

func (w *cappedWriter) Write(data []byte) (int, error) {
	n := len(data)
	remaining := 64*1024 - len(w.data)
	if n > remaining {
		w.truncated = true
		data = data[:remaining]
	}
	w.data = append(w.data, data...)
	return n, nil
}
func run(ctx context.Context, executable string, args ...string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = []string{"LC_ALL=C", "TZ=UTC", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	output, diagnostics := &cappedWriter{}, &cappedWriter{}
	cmd.Stdout = output
	cmd.Stderr = diagnostics
	err := cmd.Run()
	// journalctl can exit successfully while warning that the caller cannot
	// see system journals. Do not present that as an empty healthy journal.
	if len(diagnostics.data) != 0 && err == nil {
		err = errors.New("diagnostic command reported unavailable evidence")
	}
	return output.data, output.truncated || diagnostics.truncated, err
}
func Collect(ctx context.Context, config Config, area string) Report {
	return collect(ctx, config, area, run)
}
func collect(ctx context.Context, config Config, area string, execute runner) Report {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	r := Report{SchemaVersion: 1, Area: area, Controller: config.Controller, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Space: []Space{}, Units: []Unit{}, Journal: []Entry{}, Unavailable: []string{}}
	if !ValidArea(area) {
		r.Unavailable = append(r.Unavailable, "unsupported_area")
		return r
	}
	units := Units(area, config.Controller)
	if len(units) == 0 {
		r.Unavailable = append(r.Unavailable, "unsupported_role")
		return r
	}
	if data, err := readFixed("/proc/sys/kernel/random/boot_id", 128); err == nil && bootPattern.MatchString(strings.TrimSpace(string(data))) {
		r.BootID = strings.TrimSpace(string(data))
	} else {
		r.Unavailable = append(r.Unavailable, "boot_id")
	}
	// NixOS embeds this revision in its generated nixos-version executable;
	// there is no configuration-revision file in the active system closure.
	if data, truncated, err := execute(ctx, "/run/current-system/sw/bin/nixos-version", "--configuration-revision"); err == nil && !truncated && revisionPattern.MatchString(strings.TrimSpace(string(data))) {
		r.Revision = strings.TrimSpace(string(data))
	} else {
		r.Unavailable = append(r.Unavailable, "revision")
	}
	for _, mount := range []string{"/", "/nix/store", "/home"} {
		var stat unix.Statfs_t
		if err := unix.Statfs(mount, &stat); err != nil || stat.Bsize <= 0 {
			r.Unavailable = append(r.Unavailable, "disk:"+mount)
			continue
		}
		r.Space = append(r.Space, Space{mount, stat.Bavail * uint64(stat.Bsize), stat.Blocks * uint64(stat.Bsize)})
	}
	if area == "home_reset" || area == "system" {
		var stat unix.Stat_t
		err := unix.Lstat("/var/lib/home-snapshots/.workspace-reset/pending.json", &stat)
		if err == nil && stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Uid == 0 && stat.Mode&0077 == 0 || errors.Is(err, unix.ENOENT) {
			pending := err == nil
			r.HomeResetPending = &pending
		} else {
			r.Unavailable = append(r.Unavailable, "home_reset_pending")
		}
	}
	if area == "updates" {
		data, err := readFixed("/run/nixorium/update-notice.json", 1024)
		if restart, at, valid := parseRestartNotice(data, time.Now()); err == nil && valid {
			r.RestartRequired = &restart
			r.RestartNoticeAt = at
		} else {
			r.Unavailable = append(r.Unavailable, "restart_notice")
		}
	}
	args := append([]string{"show", "--no-pager", "--property=Id,LoadState,ActiveState,Result,ExecMainStatus,StateChangeTimestamp", "--timestamp=us+utc"}, units...)
	data, truncated, err := execute(ctx, config.Systemctl, args...)
	r.Truncated = truncated
	if truncated {
		r.Unavailable = append(r.Unavailable, "service_state")
	} else {
		var incomplete bool
		r.Units, incomplete = parseUnits(data, area, config.Controller)
		r.Truncated = r.Truncated || incomplete
		if err != nil || incomplete {
			r.Unavailable = append(r.Unavailable, "service_state")
		}
	}
	if r.BootID == "" {
		r.Unavailable = append(r.Unavailable, "journal")
		return r
	}
	// journalctl's boot descriptor uses compact hexadecimal, while procfs
	// exposes the same ID as a dashed UUID.
	args = []string{"--boot=" + strings.ReplaceAll(r.BootID, "-", ""), "--lines=50", "--no-pager", "--output=json", "--output-fields=__REALTIME_TIMESTAMP,PRIORITY,_SYSTEMD_UNIT,UNIT,MESSAGE"}
	for _, unit := range units {
		args = append(args, "--unit="+unit)
	}
	data, truncated, err = execute(ctx, config.Journalctl, args...)
	r.Truncated = r.Truncated || truncated
	if err != nil {
		r.Unavailable = append(r.Unavailable, "journal")
	} else {
		var incomplete bool
		r.Journal, incomplete = parseJournal(data, area, config.Controller)
		r.Truncated = r.Truncated || incomplete
		if incomplete {
			r.Unavailable = append(r.Unavailable, "journal")
		}
	}
	if boot, readErr := readFixed("/proc/sys/kernel/random/boot_id", 128); readErr != nil || strings.TrimSpace(string(boot)) != r.BootID {
		r.Journal = []Entry{}
		r.Units = []Unit{}
		r.Revision = ""
		r.Unavailable = append(r.Unavailable, "boot_changed", "service_state", "journal", "revision")
	}
	return r
}
func parseRestartNotice(data []byte, now time.Time) (bool, string, bool) {
	var notice struct {
		UpdatedAt int64 `json:"updatedAt"`
		Restart   *bool `json:"restart"`
	}
	if domain.ValidateStrictRemoteJSON(data, 1024) != nil {
		return false, "", false
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&notice) != nil || notice.Restart == nil || notice.UpdatedAt < 946684800 || notice.UpdatedAt > now.Unix()+60 {
		return false, "", false
	}
	return *notice.Restart, time.Unix(notice.UpdatedAt, 0).UTC().Format(time.RFC3339), true
}

func readFixed(path string, limit int64) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("invalid fixed observation file")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(data) > int(limit) {
		return nil, errors.New("observation too large")
	}
	return data, err
}
func LoadConfig() (Config, error) {
	var config Config
	data, err := readFixed("/etc/nixorium-host-diagnostics.json", 4096)
	// /etc entries are NixOS store symlinks. Read their fixed store destination;
	// never accept a config path or executable supplied by a tool argument.
	if errors.Is(err, unix.ELOOP) {
		path, linkErr := filepath.EvalSymlinks("/etc/nixorium-host-diagnostics.json")
		if linkErr != nil || !strings.HasPrefix(path, "/nix/store/") {
			return config, errors.New("unavailable diagnostics configuration")
		}
		data, err = readFixed(path, 4096)
	}
	if err != nil || domain.ValidateStrictRemoteJSON(data, 4096) != nil {
		return config, errors.New("unavailable diagnostics configuration")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || !storeExecutable(config.Systemctl, "systemctl") || !storeExecutable(config.Journalctl, "journalctl") {
		return config, errors.New("invalid diagnostics configuration")
	}
	return config, nil
}
func storeExecutable(path, binary string) bool {
	return strings.HasPrefix(path, "/nix/store/") && strings.HasSuffix(path, "/bin/"+binary) && !strings.Contains(path, "..") && !strings.ContainsAny(path, "\r\n\t ")
}

func parseUnits(data []byte, area string, controller bool) ([]Unit, bool) {
	result := []Unit{}
	seen := map[string]bool{}
	incomplete := false
	for _, block := range strings.Split(strings.TrimSpace(string(data)), "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		values := map[string]string{}
		valid := true
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				valid = false
				continue
			}
			if _, exists := values[key]; exists {
				valid = false
			}
			values[key] = value
		}
		unit := Unit{Name: values["Id"], LoadState: values["LoadState"], ActiveState: values["ActiveState"], Result: values["Result"]}
		if !valid || seen[unit.Name] || !allowedUnit(unit.Name, area, controller) || !safeLoad(unit.LoadState) || !safeActive(unit.ActiveState) || !safeResult(unit.Result) {
			incomplete = true
			continue
		}
		if value, exists := values["ExecMainStatus"]; exists {
			exit, err := strconv.Atoi(value)
			if err != nil || exit < 0 || exit > 255 {
				incomplete = true
				continue
			}
			unit.ExitStatus = &exit
		} else if unit.LoadState == "loaded" {
			incomplete = true
			continue
		}
		if timestamp := values["StateChangeTimestamp"]; timestamp != "" && timestamp != "n/a" {
			var at time.Time
			for _, layout := range []string{"Mon 2006-01-02 15:04:05 MST", "Mon 2006-01-02 15:04:05 -0700"} {
				value, err := time.Parse(layout, timestamp)
				if err == nil {
					at = value
					break
				}
			}
			if at.IsZero() {
				incomplete = true
			} else {
				unit.ChangedAt = at.UTC().Format(time.RFC3339)
			}
		}
		seen[unit.Name] = true
		if len(result) < 12 {
			result = append(result, unit)
		} else {
			incomplete = true
		}
	}
	for _, unit := range Units(area, controller) {
		if !strings.Contains(unit, "*") && !seen[unit] {
			incomplete = true
		}
	}
	return result, incomplete
}
func parseJournal(data []byte, area string, controller bool) ([]Entry, bool) {
	result := []Entry{}
	incomplete := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var fields map[string]json.RawMessage
		if domain.ValidateStrictRemoteJSON([]byte(line), 64*1024) != nil || json.Unmarshal([]byte(line), &fields) != nil {
			incomplete = true
			continue
		}
		get := func(key string) string { var value string; json.Unmarshal(fields[key], &value); return value }
		unit := get("_SYSTEMD_UNIT")
		if !allowedUnit(unit, area, controller) {
			unit = get("UNIT")
		}
		if !allowedUnit(unit, area, controller) {
			continue
		}
		micros, err := strconv.ParseInt(get("__REALTIME_TIMESTAMP"), 10, 64)
		if err != nil || micros < 946684800000000 || micros > 4102444800000000 {
			incomplete = true
			continue
		}
		priority, err := strconv.Atoi(get("PRIORITY"))
		if err != nil || priority < 0 || priority > 7 {
			incomplete = true
			continue
		}
		var message string
		if json.Unmarshal(fields["MESSAGE"], &message) != nil {
			incomplete = true
			continue
		}
		excerpt := Scrub(message)
		if excerpt == "" {
			continue
		}
		result = append(result, Entry{unit, time.UnixMicro(micros).UTC().Format(time.RFC3339), priority, excerpt})
		if len(result) > 50 {
			result = result[:50]
			incomplete = true
			break
		}
	}
	return result, incomplete
}
func safeLoad(state string) bool {
	switch state {
	case "loaded", "not-found", "masked", "error", "bad-setting", "stub", "merged":
		return true
	}
	return false
}
func safeActive(state string) bool {
	switch state {
	case "active", "inactive", "activating", "deactivating", "failed", "reloading", "maintenance", "refreshing":
		return true
	}
	return false
}
func safeResult(result string) bool {
	switch result {
	case "", "success", "exit-code", "signal", "core-dump", "timeout", "watchdog", "start-limit-hit", "resources", "protocol", "oom-kill", "exec-condition", "unknown":
		return true
	}
	return false
}

var bootPattern = regexp.MustCompile(`^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)
var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func Encode(report Report) ([]byte, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	for len(data) > MaxBytes-512 && len(report.Journal) > 0 {
		report.Journal = report.Journal[:len(report.Journal)-1]
		report.Truncated = true
		data, err = json.Marshal(report)
	}
	if err != nil || len(data) > MaxBytes {
		return nil, errors.New("diagnostics exceed limit")
	}
	return data, nil
}

// Decode refuses malformed or foreign observations and scrubs excerpts again
// at the controller boundary, before anything enters a transcript.
func Decode(data []byte, area string, controller bool) (Report, error) {
	var r Report
	if len(data) > MaxBytes || domain.ValidateStrictRemoteJSON(data, MaxBytes) != nil {
		return r, errors.New("invalid diagnostics")
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || r.SchemaVersion != 1 || r.Area != area || r.Controller != controller || !ValidArea(area) || len(r.Space) > 3 || len(r.Units) > 12 || len(r.Journal) > 50 || len(r.Unavailable) > 12 {
		return r, errors.New("invalid diagnostics")
	}
	if _, err := time.Parse(time.RFC3339, r.GeneratedAt); err != nil {
		return r, err
	}
	if r.BootID != "" && !bootPattern.MatchString(r.BootID) || r.Revision != "" && !revisionPattern.MatchString(r.Revision) {
		return r, errors.New("invalid identity")
	}
	for _, space := range r.Space {
		if space.Mount != "/" && space.Mount != "/home" && space.Mount != "/nix/store" || space.AvailableBytes > space.TotalBytes {
			return r, errors.New("invalid disk state")
		}
	}
	for _, unit := range r.Units {
		if !allowedUnit(unit.Name, area, controller) || !safeLoad(unit.LoadState) || !safeActive(unit.ActiveState) || !safeResult(unit.Result) || unit.ExitStatus != nil && (*unit.ExitStatus < 0 || *unit.ExitStatus > 255) || unit.LoadState == "loaded" && unit.ExitStatus == nil {
			return r, errors.New("invalid service state")
		}
		if unit.ChangedAt != "" {
			if _, err := time.Parse(time.RFC3339, unit.ChangedAt); err != nil {
				return r, err
			}
		}
	}
	for i := range r.Journal {
		entry := &r.Journal[i]
		if !allowedUnit(entry.Unit, area, controller) || entry.Priority < 0 || entry.Priority > 7 {
			return r, errors.New("invalid journal state")
		}
		if _, err := time.Parse(time.RFC3339, entry.At); err != nil {
			return r, err
		}
		entry.Excerpt = Scrub(entry.Excerpt)
	}
	for _, item := range r.Unavailable {
		switch item {
		case "unsupported_area", "unsupported_role", "boot_id", "boot_changed", "revision", "disk:/", "disk:/nix/store", "disk:/home", "home_reset_pending", "restart_notice", "service_state", "journal":
		default:
			return r, fmt.Errorf("invalid unavailable observation")
		}
	}
	unavailable := map[string]bool{}
	for _, code := range r.Unavailable {
		unavailable[code] = true
	}
	if unavailable["unsupported_role"] {
		if len(Units(area, controller)) != 0 || len(r.Units) != 0 || len(r.Journal) != 0 {
			return r, errors.New("invalid unsupported role")
		}
		return r, nil
	}
	if r.Space == nil || r.Units == nil || r.Journal == nil || r.Unavailable == nil {
		return r, errors.New("missing observation arrays")
	}
	if r.BootID == "" && !unavailable["boot_id"] || r.Revision == "" && !unavailable["revision"] {
		return r, errors.New("unreported missing identity")
	}
	mounts := map[string]bool{}
	for _, space := range r.Space {
		if mounts[space.Mount] {
			return r, errors.New("duplicate disk observation")
		}
		mounts[space.Mount] = true
	}
	for _, mount := range []string{"/", "/home", "/nix/store"} {
		if !mounts[mount] && !unavailable["disk:"+mount] {
			return r, errors.New("unreported missing disk observation")
		}
	}
	seen := map[string]bool{}
	for _, unit := range r.Units {
		if seen[unit.Name] {
			return r, errors.New("duplicate unit observation")
		}
		seen[unit.Name] = true
	}
	for _, unit := range Units(area, controller) {
		if !strings.Contains(unit, "*") && !seen[unit] && !unavailable["service_state"] {
			return r, errors.New("unreported missing service observation")
		}
	}
	if (area == "system" || area == "home_reset") && r.HomeResetPending == nil && !unavailable["home_reset_pending"] {
		return r, errors.New("unreported missing reset observation")
	}
	if area == "updates" && r.RestartRequired == nil && !unavailable["restart_notice"] {
		return r, errors.New("unreported missing restart observation")
	}

	if r.RestartRequired != nil {
		if area != "updates" {
			return r, errors.New("foreign restart observation")
		}
		if _, err := time.Parse(time.RFC3339, r.RestartNoticeAt); err != nil {
			return r, errors.New("missing restart notice time")
		}
	}
	if r.RestartNoticeAt != "" && r.RestartRequired == nil {
		return r, errors.New("foreign notice time")
	}
	if r.HomeResetPending != nil && area != "system" && area != "home_reset" {
		return r, errors.New("foreign reset observation")
	}

	return r, nil
}
