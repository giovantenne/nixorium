package hostdiagnostics

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// kernelSource is the Entry.Unit of kernel messages in the hardware area.
const kernelSource = "kernel"

const (
	maxUsb      = 32
	maxPci      = 64
	maxDisplays = 16
	maxModes    = 5
	maxAudio    = 8
	maxPrinters = 16
	maxKernel   = 50
)

type UsbDevice struct {
	Path             string   `json:"path"`
	VendorID         string   `json:"vendorId"`
	ProductID        string   `json:"productId"`
	Manufacturer     string   `json:"manufacturer,omitempty"`
	Product          string   `json:"product,omitempty"`
	Class            string   `json:"class,omitempty"`
	InterfaceClasses []string `json:"interfaceClasses,omitempty"`
}
type PciDevice struct {
	Slot     string `json:"slot"`
	VendorID string `json:"vendorId"`
	DeviceID string `json:"deviceId"`
	Class    string `json:"class,omitempty"`
	Driver   string `json:"driver,omitempty"`
}
type Display struct {
	Connector string   `json:"connector"`
	Status    string   `json:"status"`
	Enabled   string   `json:"enabled,omitempty"`
	Modes     []string `json:"modes,omitempty"`
}
type AudioCard struct {
	Index  int    `json:"index"`
	ID     string `json:"id"`
	Driver string `json:"driver,omitempty"`
	Name   string `json:"name,omitempty"`
}
type Printer struct {
	Name  string `json:"name"`
	URI   string `json:"uri,omitempty"`
	State string `json:"state"`
}

// hardwareRoot lets tests point the collectors at a fake sysfs and procfs.
type hardwareRoot struct{ Sys, Proc string }

var (
	hexID        = regexp.MustCompile(`^[0-9a-f]{4}$`)
	hexByte      = regexp.MustCompile(`^[0-9a-f]{2}$`)
	hexClass     = regexp.MustCompile(`^[0-9a-f]{6}$`)
	usbPath      = regexp.MustCompile(`^(?:usb[0-9]+|[0-9]+-[0-9]+(?:\.[0-9]+)*)$`)
	usbInterface = regexp.MustCompile(`^[0-9]+-[0-9]+(?:\.[0-9]+)*:[0-9]+\.[0-9]+$`)
	pciSlot      = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)
	driverName   = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,64}$`)
	drmConnector = regexp.MustCompile(`^card[0-9]+-[A-Za-z0-9\-]{1,48}$`)
	drmMode      = regexp.MustCompile(`^[0-9]{3,5}x[0-9]{3,5}[a-z]?$`)
	audioCard    = regexp.MustCompile(`^\s*([0-9]{1,2}) \[([A-Za-z0-9_\-]{1,32})\s*\]:\s*(\S+) - (.*)$`)
	printerName  = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,127}$`)
	// Serial numbers can identify a student's personal device: nothing that
	// mentions one is ever kept, from sysfs, kernel messages or a URI.
	serialText = regexp.MustCompile(`(?i)serial`)
	// Kernel messages about the device families Assistant can advise on.
	kernelPattern = `(?i)(\busb\b|\bdrm\b|\[drm|i915|amdgpu|nouveau|radeon|\bsnd\b|snd_|\bsound\b|\bhid\b|hid-|\bcups\b)`
)

// Supported reports whether the area exists for this role.
func Supported(area string, controller bool) bool {
	return area == "hardware" || len(Units(area, controller)) != 0
}

func readText(path string, limit int64) (string, bool) {
	data, err := readFixed(path, limit)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}
func readHex(path string, pattern *regexp.Regexp, strip string) (string, bool) {
	text, ok := readText(path, 64)
	text = strings.TrimPrefix(strings.ToLower(text), strip)
	return text, ok && pattern.MatchString(text)
}
func readLabel(path string) string {
	text, ok := readText(path, 256)
	if !ok || serialText.MatchString(text) {
		return ""
	}
	if text = Scrub(text); strings.Contains(text, "omitted]") {
		return ""
	}
	return text
}
func listDir(path string) ([]string, bool) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, false
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, true
}

// usbDevices reads only the fixed identity attributes. The `serial` attribute
// is not in the list on purpose.
func usbDevices(root string) ([]UsbDevice, bool, bool) {
	names, ok := listDir(filepath.Join(root, "bus/usb/devices"))
	if !ok {
		return nil, false, false
	}
	result := []UsbDevice{}
	truncated := false
	for _, name := range names {
		if !usbPath.MatchString(name) {
			continue
		}
		dir := filepath.Join(root, "bus/usb/devices", name)
		vendor, okVendor := readHex(filepath.Join(dir, "idVendor"), hexID, "")
		product, okProduct := readHex(filepath.Join(dir, "idProduct"), hexID, "")
		if !okVendor || !okProduct {
			continue
		}
		if len(result) == maxUsb {
			truncated = true
			break
		}
		device := UsbDevice{Path: name, VendorID: vendor, ProductID: product, Manufacturer: readLabel(filepath.Join(dir, "manufacturer")), Product: readLabel(filepath.Join(dir, "product"))}
		device.Class, _ = readHex(filepath.Join(dir, "bDeviceClass"), hexByte, "")
		if children, ok := listDir(dir); ok {
			seen := map[string]bool{}
			for _, child := range children {
				if !usbInterface.MatchString(child) {
					continue
				}
				if class, ok := readHex(filepath.Join(dir, child, "bInterfaceClass"), hexByte, ""); ok && !seen[class] && len(device.InterfaceClasses) < 8 {
					seen[class] = true
					device.InterfaceClasses = append(device.InterfaceClasses, class)
				}
			}
		}
		result = append(result, device)
	}
	return result, true, truncated
}

func pciDevices(root string) ([]PciDevice, bool, bool) {
	names, ok := listDir(filepath.Join(root, "bus/pci/devices"))
	if !ok {
		return nil, false, false
	}
	result := []PciDevice{}
	truncated := false
	for _, name := range names {
		if !pciSlot.MatchString(name) {
			continue
		}
		dir := filepath.Join(root, "bus/pci/devices", name)
		vendor, okVendor := readHex(filepath.Join(dir, "vendor"), hexID, "0x")
		device, okDevice := readHex(filepath.Join(dir, "device"), hexID, "0x")
		if !okVendor || !okDevice {
			continue
		}
		if len(result) == maxPci {
			truncated = true
			break
		}
		item := PciDevice{Slot: name, VendorID: vendor, DeviceID: device}
		item.Class, _ = readHex(filepath.Join(dir, "class"), hexClass, "0x")
		if target, err := os.Readlink(filepath.Join(dir, "driver")); err == nil && driverName.MatchString(filepath.Base(target)) {
			item.Driver = filepath.Base(target)
		}
		result = append(result, item)
	}
	return result, true, truncated
}

func displays(root string) ([]Display, bool, bool) {
	names, ok := listDir(filepath.Join(root, "class/drm"))
	if !ok {
		return nil, false, false
	}
	result := []Display{}
	truncated := false
	for _, name := range names {
		if !drmConnector.MatchString(name) {
			continue
		}
		dir := filepath.Join(root, "class/drm", name)
		status, ok := readText(filepath.Join(dir, "status"), 64)
		if !ok || status != "connected" && status != "disconnected" && status != "unknown" {
			continue
		}
		if len(result) == maxDisplays {
			truncated = true
			break
		}
		item := Display{Connector: name, Status: status}
		if enabled, ok := readText(filepath.Join(dir, "enabled"), 64); ok && (enabled == "enabled" || enabled == "disabled") {
			item.Enabled = enabled
		}
		if modes, ok := readText(filepath.Join(dir, "modes"), 4096); ok {
			for _, line := range strings.Split(modes, "\n") {
				line = strings.TrimSpace(line)
				if !drmMode.MatchString(line) {
					continue
				}
				if len(item.Modes) == maxModes {
					truncated = true
					break
				}
				item.Modes = append(item.Modes, line)
			}
		}
		result = append(result, item)
	}
	return result, true, truncated
}

func audioCards(root string) ([]AudioCard, bool, bool) {
	text, ok := readText(filepath.Join(root, "asound/cards"), 8192)
	if !ok {
		return nil, false, false
	}
	result := []AudioCard{}
	truncated := false
	for _, line := range strings.Split(text, "\n") {
		match := audioCard.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if len(result) == maxAudio {
			truncated = true
			break
		}
		index, _ := strconv.Atoi(match[1])
		name := Scrub(match[4])
		if serialText.MatchString(name) || strings.Contains(name, "omitted]") {
			name = ""
		}
		driver := match[3]
		if !driverName.MatchString(driver) {
			driver = ""
		}
		result = append(result, AudioCard{Index: index, ID: match[2], Driver: driver, Name: name})
	}
	return result, true, truncated
}

var (
	printerLine = regexp.MustCompile(`^printer (\S+) (.*)$`)
	deviceLine  = regexp.MustCompile(`^device for (\S+): (\S+)$`)
)

// SanitizePrinterURI drops credentials, queries (USB URIs carry the printer's
// serial number there) and fragments, keeping scheme, host, port and path.
func SanitizePrinterURI(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !regexp.MustCompile(`^[a-z][a-z0-9+.\-]{0,31}$`).MatchString(u.Scheme) {
		return "[unparseable uri]"
	}
	out := u.Scheme + ":"
	if u.Host != "" {
		out += "//" + u.Host
	}
	path := u.EscapedPath()
	if u.Opaque != "" {
		path = u.Opaque
		if i := strings.IndexAny(path, "?#"); i >= 0 {
			path = path[:i]
		}
	}
	if serialText.MatchString(path) || strings.ContainsAny(path, "@") {
		path = "/[omitted]"
	}
	out += path
	out = domain.RedactForModel(out)
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}

func parsePrinters(devices, states string) ([]Printer, bool) {
	uris := map[string]string{}
	for _, line := range strings.Split(devices, "\n") {
		if match := deviceLine.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			uris[match[1]] = SanitizePrinterURI(match[2])
		}
	}
	result := []Printer{}
	seen := map[string]bool{}
	truncated := false
	add := func(name, state string) {
		if seen[name] || !printerName.MatchString(name) {
			return
		}
		if len(result) == maxPrinters {
			truncated = true
			return
		}
		seen[name] = true
		result = append(result, Printer{Name: name, URI: uris[name], State: state})
	}
	for _, line := range strings.Split(states, "\n") {
		match := printerLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		state := "unknown"
		switch {
		case strings.HasPrefix(match[2], "is idle"):
			state = "idle"
		case strings.HasPrefix(match[2], "now printing"):
			state = "printing"
		case strings.HasPrefix(match[2], "disabled"):
			state = "disabled"
		}
		add(match[1], state)
	}
	// Queues that only appear in the device list are still worth reporting.
	for name := range uris {
		if !seen[name] {
			add(name, "unknown")
		}
	}
	return result, truncated
}

func collectHardware(ctx context.Context, config Config, r Report, execute runner, root hardwareRoot) Report {
	truncated := false
	note := func(code string, ok, cut bool) {
		if !ok {
			r.Unavailable = append(r.Unavailable, code)
		}
		truncated = truncated || cut
	}
	var ok, cut bool
	r.UsbDevices, ok, cut = usbDevices(root.Sys)
	note("usb", ok, cut)
	r.PciDevices, ok, cut = pciDevices(root.Sys)
	note("pci", ok, cut)
	r.Displays, ok, cut = displays(root.Sys)
	note("displays", ok, cut)
	r.AudioCards, ok, cut = audioCards(root.Proc)
	note("audio", ok, cut)

	r.Printers = nil
	if config.Lpstat == "" {
		note("printers", false, false)
	} else {
		// lpstat fails when there is no queue at all, so first ask whether the
		// scheduler answers; only then is an empty list a real "no printers".
		running, cutRunning, errRunning := execute(ctx, config.Lpstat, "-r")
		devices, cutDevices, errDevices := execute(ctx, config.Lpstat, "-v")
		states, cutStates, errStates := execute(ctx, config.Lpstat, "-p")
		noQueue := func(data []byte, err error) bool { return err != nil && strings.TrimSpace(string(data)) == "" }
		if errRunning != nil || cutRunning || !strings.Contains(string(running), "scheduler is running") || cutDevices || cutStates || (errDevices != nil && !noQueue(devices, errDevices)) || (errStates != nil && !noQueue(states, errStates)) {
			note("printers", false, cutRunning || cutDevices || cutStates)
		} else {
			var cutList bool
			r.Printers, cutList = parsePrinters(string(devices), string(states))
			note("printers", true, cutList)
		}
	}

	if r.BootID == "" {
		r.Unavailable = append(r.Unavailable, "kernel_log")
	} else {
		args := []string{"--boot=" + strings.ReplaceAll(r.BootID, "-", ""), "--dmesg", "--grep=" + kernelPattern, "--lines=" + strconv.Itoa(maxKernel+1), "--no-pager", "--output=json", "--output-fields=__REALTIME_TIMESTAMP,PRIORITY,_TRANSPORT,MESSAGE"}
		data, cutLog, err := execute(ctx, config.Journalctl, args...)
		r.Truncated = r.Truncated || cutLog
		if err != nil || cutLog {
			r.Unavailable = append(r.Unavailable, "kernel_log")
		} else {
			var incomplete bool
			r.Journal, incomplete = parseKernel(data)
			truncated = truncated || incomplete
		}
	}
	r.Truncated = r.Truncated || truncated
	if boot, err := readFixed("/proc/sys/kernel/random/boot_id", 128); err != nil || strings.TrimSpace(string(boot)) != r.BootID {
		r.Journal = []Entry{}
		r.Revision = ""
		r.Unavailable = append(r.Unavailable, "boot_changed", "kernel_log", "revision")
	}
	return r
}

// parseKernel keeps only kernel-transport records and drops any message that
// mentions a serial number.
func parseKernel(data []byte) ([]Entry, bool) {
	result := []Entry{}
	incomplete := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields, ok := journalFields(line)
		if !ok {
			incomplete = true
			continue
		}
		if fields.get("_TRANSPORT") != "kernel" {
			continue
		}
		micros, err := strconv.ParseInt(fields.get("__REALTIME_TIMESTAMP"), 10, 64)
		priority, errPriority := strconv.Atoi(fields.get("PRIORITY"))
		message := fields.get("MESSAGE")
		if err != nil || micros < 946684800000000 || micros > 4102444800000000 || errPriority != nil || priority < 0 || priority > 7 {
			incomplete = true
			continue
		}
		if serialText.MatchString(message) {
			continue
		}
		excerpt := Scrub(message)
		if excerpt == "" || excerpt == "[sensitive journal entry omitted]" {
			continue
		}
		if len(result) == maxKernel {
			incomplete = true
			break
		}
		result = append(result, Entry{kernelSource, time.UnixMicro(micros).UTC().Format(time.RFC3339), priority, excerpt})
	}
	// journalctl may print a filtered tail newest first; report oldest first.
	sort.SliceStable(result, func(i, j int) bool { return result[i].At < result[j].At })
	return result, incomplete
}

// validateHardware re-checks every field on the controller before anything
// reaches a transcript, and scrubs free text again.
func validateHardware(r *Report) error {
	bad := errors.New("invalid hardware observation")
	if len(r.UsbDevices) > maxUsb || len(r.PciDevices) > maxPci || len(r.Displays) > maxDisplays || len(r.AudioCards) > maxAudio || len(r.Printers) > maxPrinters || len(r.Units) != 0 {
		return bad
	}
	for i := range r.UsbDevices {
		d := &r.UsbDevices[i]
		if !usbPath.MatchString(d.Path) || !hexID.MatchString(d.VendorID) || !hexID.MatchString(d.ProductID) || d.Class != "" && !hexByte.MatchString(d.Class) || len(d.InterfaceClasses) > 8 {
			return bad
		}
		for _, class := range d.InterfaceClasses {
			if !hexByte.MatchString(class) {
				return bad
			}
		}
		for _, text := range []*string{&d.Manufacturer, &d.Product} {
			if serialText.MatchString(*text) {
				*text = ""
			} else if *text != "" {
				*text = Scrub(*text)
			}
		}
	}
	for _, d := range r.PciDevices {
		if !pciSlot.MatchString(d.Slot) || !hexID.MatchString(d.VendorID) || !hexID.MatchString(d.DeviceID) || d.Class != "" && !hexClass.MatchString(d.Class) || d.Driver != "" && !driverName.MatchString(d.Driver) {
			return bad
		}
	}
	for _, d := range r.Displays {
		if !drmConnector.MatchString(d.Connector) || d.Status != "connected" && d.Status != "disconnected" && d.Status != "unknown" || d.Enabled != "" && d.Enabled != "enabled" && d.Enabled != "disabled" || len(d.Modes) > maxModes {
			return bad
		}
		for _, mode := range d.Modes {
			if !drmMode.MatchString(mode) {
				return bad
			}
		}
	}
	for i := range r.AudioCards {
		c := &r.AudioCards[i]
		if c.Index < 0 || c.Index > 99 || !regexp.MustCompile(`^[A-Za-z0-9_\-]{1,32}$`).MatchString(c.ID) || c.Driver != "" && !driverName.MatchString(c.Driver) {
			return bad
		}
		c.Name = Scrub(c.Name)
	}
	for i := range r.Printers {
		p := &r.Printers[i]
		if !printerName.MatchString(p.Name) || p.State != "idle" && p.State != "printing" && p.State != "disabled" && p.State != "unknown" {
			return bad
		}
		if p.URI != "" {
			p.URI = SanitizePrinterURI(p.URI)
		}
	}
	for _, entry := range r.Journal {
		if entry.Unit != kernelSource {
			return bad
		}
	}
	return nil
}
