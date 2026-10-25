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
	put(t, sys, "bus/usb/devices/1-3/product", "Mouse with serial 99\n")
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
		t.Fatal("label mentioning a serial number kept", usb[1])
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
			if len(args) == 1 && args[0] == "-r" {
				return []byte("scheduler is running\n"), false, nil
			}
			if len(args) == 1 && args[0] == "-v" {
				return []byte("device for office: ipp://u:p@host/printers/office\n"), false, nil
			}
			if len(args) == 1 && args[0] == "-p" {
				return []byte("printer office is idle.  enabled since x\n"), false, nil
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
	if len(calls) != 4 || len(r.Journal) != 1 || len(r.Printers) != 1 || r.Printers[0].URI != "ipp://host/printers/office" {
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
