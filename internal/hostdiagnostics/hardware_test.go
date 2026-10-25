package hostdiagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func put(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func fakeTree(t *testing.T) hardwareRoot {
	sys, proc := filepath.Join(t.TempDir(), "sys"), filepath.Join(t.TempDir(), "proc")
	usb := "bus/usb/devices/1-2/"
	put(t, sys, usb+"idVendor", "2341\n")
	put(t, sys, usb+"idProduct", "0043\n")
	put(t, sys, usb+"manufacturer", "Arduino (www.arduino.cc)\n")
	put(t, sys, usb+"product", "Arduino Uno\n")
	put(t, sys, usb+"serial", "SYNTHETICSERIAL123\n")
	put(t, sys, usb+"bDeviceClass", "02\n")
	put(t, sys, usb+"1-2:1.0/bInterfaceClass", "02\n")
	put(t, sys, usb+"1-2:1.1/bInterfaceClass", "0a\n")
	put(t, sys, "bus/usb/devices/1-3/idVendor", "046d\n")
	put(t, sys, "bus/usb/devices/1-3/idProduct", "c077\n")
	put(t, sys, "bus/usb/devices/1-3/product", "USB Serial SerialNumber: 99\n")
	put(t, sys, "bus/usb/devices/1-3:1.0/bInterfaceClass", "03\n") // an interface directory is not a device
	put(t, sys, "bus/usb/devices/notadevice/idVendor", "ffff\n")
	pci := "bus/pci/devices/0000:00:02.0/"
	put(t, sys, pci+"vendor", "0x8086\n")
	put(t, sys, pci+"device", "0x9a49\n")
	put(t, sys, pci+"class", "0x030000\n")
	if err := os.Symlink("../../../bus/pci/drivers/i915", filepath.Join(sys, pci, "driver")); err != nil {
		t.Fatal(err)
	}
	put(t, sys, "class/drm/card0-HDMI-A-1/status", "connected\n")
	put(t, sys, "class/drm/card0-HDMI-A-1/enabled", "enabled\n")
	put(t, sys, "class/drm/card0-HDMI-A-1/modes", "1920x1080\n1280x720\n1024x768\n800x600\n640x480\n320x200\n")
	put(t, sys, "class/drm/card0-HDMI-A-1/edid", "binary-must-not-be-read")
	put(t, sys, "class/drm/card0-VGA-1/status", "disconnected\n")
	put(t, sys, "class/drm/version", "x")
	put(t, proc, "asound/cards", " 0 [PCH            ]: HDA-Intel - HDA Intel PCH\n                      HDA Intel PCH at 0xb1200000 irq 140\n 1 [Device         ]: USB-Audio - USB Audio Device\n                      C-Media USB Audio Device at usb-0000:00:14.0-2\n")
	return hardwareRoot{Sys: sys, Proc: proc}
}

func TestUSBPCIDisplayAndAudioFromFakeTrees(t *testing.T) {
	root := fakeTree(t)
	usb, ok, _ := usbDevices(root.Sys)
	if !ok || len(usb) != 2 || usb[0].Path != "1-2" || usb[0].VendorID != "2341" || usb[0].ProductID != "0043" || usb[0].Product != "Arduino Uno" || usb[0].Class != "02" || strings.Join(usb[0].InterfaceClasses, ",") != "02,0a" {
		t.Fatal(usb)
	}
	if usb[1].Product != "" {
		t.Fatal("label carrying a serial value kept", usb[1])
	}
	pci, ok, _ := pciDevices(root.Sys)
	if !ok || len(pci) != 1 || pci[0] != (PciDevice{"0000:00:02.0", "8086", "9a49", "030000", "i915"}) {
		t.Fatal(pci)
	}
	displays, ok, cut := displays(root.Sys)
	if !ok || !cut || len(displays) != 2 || displays[0].Connector != "card0-HDMI-A-1" || displays[0].Status != "connected" || displays[0].Enabled != "enabled" || len(displays[0].Modes) != 5 || displays[1].Status != "disconnected" {
		t.Fatal(displays, cut)
	}
	audio, ok, _ := audioCards(root.Proc)
	if !ok || len(audio) != 2 || audio[1] != (AudioCard{1, "Device", "USB-Audio", "USB Audio Device"}) {
		t.Fatal(audio)
	}
	data, _ := json.Marshal([]any{usb, pci, displays, audio})
	for _, forbidden := range []string{"SYNTHETICSERIAL", "serial", "binary-must-not"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("leaked %q: %s", forbidden, data)
		}
	}
}

func TestMissingSourcesAreListedAsUnavailable(t *testing.T) {
	empty := t.TempDir()
	r := collectHardware(t.Context(), Config{Journalctl: "j"}, Report{Unavailable: []string{}, BootID: ""}, func(context.Context, string, ...string) ([]byte, bool, error) {
		return nil, false, errors.New("unused")
	}, hardwareRoot{Sys: empty, Proc: empty})
	got := strings.Join(r.Unavailable, ",")
	for _, code := range []string{"usb", "pci", "displays", "audio", "printers", "kernel_log"} {
		if !strings.Contains(got, code) {
			t.Fatal(code, got)
		}
	}
	if len(r.UsbDevices)+len(r.PciDevices)+len(r.Displays)+len(r.AudioCards)+len(r.Printers) != 0 {
		t.Fatal(r)
	}
}

func TestLimitsAreEnforcedAndMarkedTruncated(t *testing.T) {
	sys := t.TempDir()
	for i := 0; i < maxUsb+5; i++ {
		put(t, sys, fmt.Sprintf("bus/usb/devices/1-%d/idVendor", i+1), "1234")
		put(t, sys, fmt.Sprintf("bus/usb/devices/1-%d/idProduct", i+1), "5678")
	}
	for i := 0; i < maxPci+5; i++ {
		put(t, sys, fmt.Sprintf("bus/pci/devices/0000:%02x:00.0/vendor", i), "0x1234")
		put(t, sys, fmt.Sprintf("bus/pci/devices/0000:%02x:00.0/device", i), "0x5678")
	}
	usb, _, cut := usbDevices(sys)
	pci, _, cutPci := pciDevices(sys)
	if len(usb) != maxUsb || !cut || len(pci) != maxPci || !cutPci {
		t.Fatal(len(usb), cut, len(pci), cutPci)
	}
	var lines strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&lines, " %d [Card%d ]: HDA-Intel - Card\n", i, i)
	}
	proc := t.TempDir()
	put(t, proc, "asound/cards", lines.String())
	audio, _, cutAudio := audioCards(proc)
	if len(audio) != maxAudio || !cutAudio {
		t.Fatal(len(audio), cutAudio)
	}
	r := Report{SchemaVersion: 1, Area: "hardware", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Space: []Space{}, Units: []Unit{}, Journal: []Entry{}, Unavailable: []string{}, UsbDevices: usb, PciDevices: pci}
	data, err := Encode(r)
	if err != nil || len(data) > MaxBytes {
		t.Fatal(len(data), err)
	}
}

func TestPrinterURICredentialsAndSerialsAreRemoved(t *testing.T) {
	devices := "device for office: ipp://teacher:hunter2@printer.example:631/printers/office?token=abc\n" +
		"device for usbq: usb://HP/LaserJet%20P1102?serial=SYNTHETICSERIAL\n" +
		"device for lan: socket://192.0.2.9:9100\n" +
		"device for pdf: cups-pdf:/\n"
	states := "printer office is idle.  enabled since Mon 12 Oct 2026\nprinter usbq now printing usbq-3.  enabled since Mon\nprinter lan disabled since Mon -\nprinter bad;name is idle\n"
	printers, cut := parsePrinters(devices, states)
	if cut || len(printers) != 4 {
		t.Fatal(printers, cut)
	}
	data, _ := json.Marshal(printers)
	for _, secret := range []string{"hunter2", "teacher", "token", "SYNTHETICSERIAL", "serial", "abc"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("leaked %q: %s", secret, data)
		}
	}
	if printers[0] != (Printer{"office", "ipp://printer.example:631/printers/office", "idle"}) || printers[1].State != "printing" || printers[2] != (Printer{"lan", "socket://192.0.2.9:9100", "disabled"}) {
		t.Fatal(printers)
	}
	many := strings.Repeat("printer p is idle\n", 1)
	for i := 0; i < 30; i++ {
		many += fmt.Sprintf("printer q%d is idle\n", i)
	}
	if got, cut := parsePrinters("", many); len(got) != maxPrinters || !cut {
		t.Fatal(len(got), cut)
	}
}

func TestKernelMessagesAreKernelOnlyBoundedAndScrubbed(t *testing.T) {
	var input strings.Builder
	add := func(transport, message string) {
		data, _ := json.Marshal(map[string]string{"_TRANSPORT": transport, "__REALTIME_TIMESTAMP": "1780000000000000", "PRIORITY": "6", "MESSAGE": message})
		input.Write(data)
		input.WriteByte('\n')
	}
	add("journal", "usb: a user process pretending to be the kernel")
	add("kernel", "usb 1-2: SerialNumber: SYNTHETICSERIAL")
	add("kernel", "usb 1-2: password=synthetic")
	for i := 0; i < 70; i++ {
		add("kernel", "usb 1-2: new full-speed USB device number 2 using xhci_hcd")
	}
	entries, incomplete := parseKernel([]byte(input.String()))
	if len(entries) != maxKernel || !incomplete {
		t.Fatal(len(entries), incomplete)
	}
	data, _ := json.Marshal(entries)
	for _, forbidden := range []string{"SYNTHETICSERIAL", "pretending", "synthetic", "Serial"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("leaked %q", forbidden)
		}
	}
	if entries[0].Unit != "kernel" || !strings.Contains(entries[0].Excerpt, "new full-speed USB device") {
		t.Fatal(entries[0])
	}
}

func TestHardwareCollectUsesOnlyFixedReadOnlyCommandsAndRoundTrips(t *testing.T) {
	root := fakeTree(t)
	var calls []string
	r := Report{SchemaVersion: 1, Area: "hardware", Controller: false, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Space: []Space{}, Units: []Unit{}, Journal: []Entry{}, Unavailable: []string{"disk:/", "disk:/nix/store", "disk:/home", "revision"}}
	if boot, err := readFixed("/proc/sys/kernel/random/boot_id", 128); err == nil {
		r.BootID = strings.TrimSpace(string(boot))
	} else {
		t.Skip("no boot id")
	}
	r = collectHardware(t.Context(), Config{Journalctl: "fixed-journalctl", Lpstat: "fixed-lpstat"}, r, func(_ context.Context, name string, args ...string) ([]byte, bool, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		switch name {
		case "fixed-lpstat":
			if strings.Join(args, " ") == "-r -v -p" {
				return []byte("scheduler is running\ndevice for office: ipp://u:p@host/printers/office\nprinter office is idle.  enabled since x\n"), false, nil
			}
		case "fixed-journalctl":
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "--dmesg") || !strings.Contains(joined, "--lines=51") || strings.Contains(joined, "--unit") || strings.Contains(joined, "--all") {
				t.Fatal(joined)
			}
			return []byte(`{"_TRANSPORT":"kernel","__REALTIME_TIMESTAMP":"1780000000000000","PRIORITY":"6","MESSAGE":"usb 1-2: new device"}` + "\n"), false, nil
		}
		t.Fatal("unexpected command", name, args)
		return nil, false, nil
	}, root)
	if len(calls) != 2 || len(r.Journal) != 1 || len(r.Printers) != 1 || r.Printers[0].URI != "ipp://host/printers/office" {
		t.Fatal(calls, r)
	}
	data, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data, "hardware", false)
	if err != nil || len(decoded.UsbDevices) != 2 || len(decoded.Displays) != 2 {
		t.Fatal(err, string(data))
	}
	if strings.Contains(string(data), "u:p") {
		t.Fatal(string(data))
	}
	// A foreign area may not carry hardware sections, and hardware may not
	// carry foreign journal sources.
	foreign, _ := json.Marshal(Report{SchemaVersion: 1, Area: "system", GeneratedAt: r.GeneratedAt, Space: []Space{}, Units: []Unit{}, Journal: []Entry{}, Unavailable: []string{}, UsbDevices: r.UsbDevices})
	if _, err := Decode(foreign, "system", false); err == nil {
		t.Fatal("hardware section accepted in another area")
	}
	r.Journal = []Entry{{"sshd.service", r.GeneratedAt, 3, "x"}}
	data, _ = json.Marshal(r)
	if _, err := Decode(data, "hardware", false); err == nil {
		t.Fatal("foreign journal source accepted")
	}
	// An older helper's answer for an area it does not know is still decodable
	// as the refusal today's controllers already expect.
	if !ValidArea("hardware") || !Supported("hardware", true) || !Supported("hardware", false) {
		t.Fatal("hardware must be valid for both roles")
	}
}

func TestKeptLabelsAndSerialValues(t *testing.T) {
	sys := t.TempDir()
	put(t, sys, "bus/usb/devices/1-1/idVendor", "1a86")
	put(t, sys, "bus/usb/devices/1-1/idProduct", "7523")
	put(t, sys, "bus/usb/devices/1-1/product", "USB-SERIAL CH340")
	put(t, sys, "bus/usb/devices/1-1/manufacturer", "USB Serial")
	usb, _, _ := usbDevices(sys)
	if len(usb) != 1 || usb[0].Product != "USB-SERIAL CH340" || usb[0].Manufacturer != "USB Serial" {
		t.Fatal(usb)
	}
	entries, _ := parseKernel([]byte(`{"_TRANSPORT":"kernel","__REALTIME_TIMESTAMP":"1780000000000000","PRIORITY":"6","MESSAGE":"usb 1-1: SerialNumber: ABC123"}` + "\n" +
		`{"_TRANSPORT":"kernel","__REALTIME_TIMESTAMP":"1780000000000000","PRIORITY":"6","MESSAGE":"usb 1-1: Product: USB-SERIAL CH340"}` + "\n"))
	if len(entries) != 1 || !strings.Contains(entries[0].Excerpt, "USB-SERIAL CH340") {
		t.Fatal(entries)
	}
}

func TestBluetoothLinesAreDroppedAndMACAddressesRedacted(t *testing.T) {
	line := func(message string) string {
		data, _ := json.Marshal(map[string]string{"_TRANSPORT": "kernel", "__REALTIME_TIMESTAMP": "1780000000000000", "PRIORITY": "6", "MESSAGE": message})
		return string(data) + "\n"
	}
	input := line("hid-generic 0005:05AC:0267.0004: input,hidraw3: BLUETOOTH HID v0.50 Keyboard [Mario's Magic Keyboard] on 00:11:22:33:44:55") +
		line("Bluetooth: hci0: unexpected event") + line("usb 1-1: reset by AA:BB:CC:DD:EE:FF adapter") + line("usb 1-1: hub ok")
	entries, _ := parseKernel([]byte(input))
	data, _ := json.Marshal(entries)
	if len(entries) != 2 || strings.Contains(string(data), "Mario") || strings.Contains(strings.ToLower(string(data)), "aa:bb") || strings.Contains(string(data), "00:11") || !strings.Contains(string(data), "[REDACTED]") {
		t.Fatal(string(data))
	}
	if got := Scrub("label 00:11:22:33:44:55"); strings.Contains(got, "00:11") {
		t.Fatal(got)
	}
}

func TestPrinterURIFallbackKeepsOnlyScheme(t *testing.T) {
	for raw, want := range map[string]string{
		"dnssd://HP%20LaserJet%20Pro._ipp._tcp.local/?uuid=1234": "dnssd://[omitted]",
		"ipps://my%20printer.local:631/ipp/print":                "ipps://[omitted]",
		"nonsense": "[unparseable uri]",
	} {
		got := SanitizePrinterURI(raw)
		if got != want || SanitizePrinterURI(got) != got {
			t.Fatal(raw, got, want)
		}
	}
}

func TestPrinterModes(t *testing.T) {
	config := Config{Journalctl: "j", Lpstat: "lp"}
	run := func(out string, cut bool, err error) runner {
		return func(_ context.Context, name string, _ ...string) ([]byte, bool, error) {
			if name == "lp" {
				return []byte(out), cut, err
			}
			return nil, false, errors.New("no journal")
		}
	}
	empty := t.TempDir()
	for _, test := range []struct {
		name        string
		out         string
		cut         bool
		err         error
		unavailable bool
		printers    int
	}{
		{"running with queues", "scheduler is running\ndevice for a: socket://h:9100\nprinter a is idle\n", false, nil, false, 1},
		{"no queue, lpstat exit 1", "scheduler is running\n", false, errors.New("exit 1"), false, 0},
		{"queues but -v exit 1", "scheduler is running\nprinter a is idle\n", false, errors.New("exit 1"), false, 1},
		{"scheduler stopped", "scheduler is not running\n", false, errors.New("exit 1"), true, 0},
		{"no output", "", false, errors.New("exit 1"), true, 0},
		{"truncated", "scheduler is running\n", true, nil, true, 0},
	} {
		r := collectHardware(t.Context(), config, Report{Unavailable: []string{}}, run(test.out, test.cut, test.err), hardwareRoot{Sys: empty, Proc: empty})
		has := strings.Contains(strings.Join(r.Unavailable, ","), "printers")
		if has != test.unavailable || len(r.Printers) != test.printers {
			t.Fatal(test.name, r.Unavailable, r.Printers)
		}
	}
}

func TestMissingBootIDDoesNotInventBootChange(t *testing.T) {
	empty := t.TempDir()
	r := collectHardware(t.Context(), Config{Journalctl: "j"}, Report{Unavailable: []string{"boot_id", "revision"}}, func(context.Context, string, ...string) ([]byte, bool, error) {
		t.Fatal("journal queried without a boot id")
		return nil, false, nil
	}, hardwareRoot{Sys: empty, Proc: empty})
	count := map[string]int{}
	for _, code := range r.Unavailable {
		count[code]++
	}
	if count["boot_changed"] != 0 || count["kernel_log"] != 1 || count["revision"] != 1 {
		t.Fatal(r.Unavailable)
	}
}

func hardwareBase() Report {
	return Report{SchemaVersion: 1, Area: "hardware", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Space: []Space{}, Units: []Unit{}, Journal: []Entry{}, Unavailable: []string{"boot_id", "revision", "disk:/", "disk:/nix/store", "disk:/home"}}
}

func TestDecodeRefusesMalformedHardwareFields(t *testing.T) {
	good := UsbDevice{Path: "1-2", VendorID: "2341", ProductID: "0043", Class: "02"}
	var tooMany []string
	for i := 0; i < 9; i++ {
		tooMany = append(tooMany, "03")
	}
	for name, modify := range map[string]func(*Report){
		"usb path":       func(r *Report) { d := good; d.Path = "../../etc"; r.UsbDevices = []UsbDevice{d} },
		"usb vendor":     func(r *Report) { d := good; d.VendorID = "zzzz"; r.UsbDevices = []UsbDevice{d} },
		"usb class":      func(r *Report) { d := good; d.Class = "0x2"; r.UsbDevices = []UsbDevice{d} },
		"interfaces":     func(r *Report) { d := good; d.InterfaceClasses = tooMany; r.UsbDevices = []UsbDevice{d} },
		"long label":     func(r *Report) { d := good; d.Product = strings.Repeat("a", 127); r.UsbDevices = []UsbDevice{d} },
		"control label":  func(r *Report) { d := good; d.Product = "a\x1b[31mb"; r.UsbDevices = []UsbDevice{d} },
		"pci slot":       func(r *Report) { r.PciDevices = []PciDevice{{Slot: "x", VendorID: "8086", DeviceID: "1234"}} },
		"display status": func(r *Report) { r.Displays = []Display{{Connector: "card0-HDMI-A-1", Status: "on"}} },
		"display mode": func(r *Report) {
			r.Displays = []Display{{Connector: "card0-HDMI-A-1", Status: "connected", Modes: []string{"big"}}}
		},
		"audio label":     func(r *Report) { r.AudioCards = []AudioCard{{ID: "x", Name: "a\nb"}} },
		"printer state":   func(r *Report) { r.Printers = []Printer{{Name: "p", State: "on fire"}} },
		"hardware code":   func(r *Report) { r.Unavailable = append(r.Unavailable, "bogus") },
		"foreign journal": func(r *Report) { r.Journal = []Entry{{"sshd.service", r.GeneratedAt, 3, "x"}} },
	} {
		r := hardwareBase()
		modify(&r)
		data, _ := json.Marshal(r)
		if _, err := Decode(data, "hardware", false); err == nil {
			t.Fatal(name, string(data))
		}
	}
	// Hardware unavailable codes belong only to the hardware area.
	r := Report{SchemaVersion: 1, Area: "system", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Space: []Space{}, Units: []Unit{}, Journal: []Entry{}, Unavailable: []string{"usb"}}
	data, _ := json.Marshal(r)
	if _, err := Decode(data, "system", false); err == nil {
		t.Fatal("usb code accepted in system area")
	}
	// A label the scrubber withholds shows a neutral placeholder.
	r = hardwareBase()
	r.UsbDevices = []UsbDevice{{Path: "1-1", VendorID: "1234", ProductID: "5678", Product: "Token2 FIDO"}}
	data, _ = json.Marshal(r)
	decoded, err := Decode(data, "hardware", false)
	if err != nil || decoded.UsbDevices[0].Product != "[omitted]" {
		t.Fatal(err, decoded.UsbDevices)
	}
}

func TestEncodeTrimsAnOversizedHardwareReport(t *testing.T) {
	r := hardwareBase()
	label := strings.Repeat("x", maxLabelRunes)
	for i := 0; i < maxUsb; i++ {
		r.UsbDevices = append(r.UsbDevices, UsbDevice{Path: fmt.Sprintf("1-%d", i+1), VendorID: "1234", ProductID: "5678", Manufacturer: label, Product: label, InterfaceClasses: []string{"01", "02", "03", "04", "05", "06", "07", "08"}})
	}
	for i := 0; i < maxPrinters; i++ {
		r.Printers = append(r.Printers, Printer{Name: fmt.Sprintf("printer%d", i), URI: "socket://" + strings.Repeat("h", 150), State: "idle"})
	}
	for i := 0; i < maxAudio; i++ {
		r.AudioCards = append(r.AudioCards, AudioCard{Index: i, ID: "card", Driver: "HDA-Intel", Name: label})
	}
	for i := 0; i < 50; i++ {
		r.Journal = append(r.Journal, Entry{"kernel", r.GeneratedAt, 6, strings.Repeat("usb ", 60)})
	}
	if raw, _ := json.Marshal(r); len(raw) <= MaxBytes {
		t.Skip("fixture is not oversized", len(raw))
	}
	data, err := Encode(r)
	if err != nil || len(data) > MaxBytes {
		t.Fatal(len(data), err)
	}
	decoded, err := Decode(data, "hardware", false)
	if err != nil || !decoded.Truncated {
		t.Fatal(err, decoded.Truncated)
	}
}
