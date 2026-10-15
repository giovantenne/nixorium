package domain

import (
	"bytes"
	"testing"
)

func TestNormalizeMACAcceptsCommonFormsInLowercase(t *testing.T) {
	for input, want := range map[string]string{
		"52:54:00:AB:cd:01":   "52:54:00:ab:cd:01",
		"52-54-00-ab-cd-01":   "52:54:00:ab:cd:01",
		"5254.00ab.cd01":      "52:54:00:ab:cd:01",
		"525400ABCD01":        "52:54:00:ab:cd:01",
		" 52:54:00:ab:cd:01 ": "52:54:00:ab:cd:01",
	} {
		if got, err := NormalizeMAC(input); err != nil || got != want {
			t.Errorf("NormalizeMAC(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{
		"", "52:54:00:ab:cd", "52:54:00:ab:cd:01:02", "52:54:00:ab:cd:zz", "5254 00ab cd01",
		"00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "01:00:5e:00:00:01",
		"00:00:00:00:fe:80:00:00:00:00:00:00:02:00:5e:10:00:00:00:01",
	} {
		if got, err := NormalizeMAC(input); err == nil {
			t.Errorf("NormalizeMAC(%q) = %q, want a refusal", input, got)
		}
	}
}

func TestMagicPacketIsSixFFsThenTheAddressSixteenTimes(t *testing.T) {
	packet, err := MagicPacket("52-54-00-AB-CD-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != 102 || !bytes.Equal(packet[:6], bytes.Repeat([]byte{0xff}, 6)) {
		t.Fatalf("packet header = % x (length %d)", packet[:6], len(packet))
	}
	address := []byte{0x52, 0x54, 0x00, 0xab, 0xcd, 0x01}
	if !bytes.Equal(packet[6:], bytes.Repeat(address, 16)) {
		t.Fatalf("packet body = % x", packet[6:])
	}
	if _, err := MagicPacket("ff:ff:ff:ff:ff:ff"); err == nil {
		t.Fatal("a broadcast address made a packet")
	}
}

func TestDirectedBroadcast(t *testing.T) {
	for _, test := range []struct {
		base   string
		prefix int
		want   string
	}{
		{"10.42.0.0", 24, "10.42.0.255"},
		{"192.168.16.0", 20, "192.168.31.255"},
		{"172.16.5.0", 30, "172.16.5.3"},
		{"10.0.0.0", 31, ""},
		{"10.0.0.0", 32, ""},
		{"10.0.0.0", 0, ""},
		{"fd00::", 64, ""},
		{"not an address", 24, ""},
	} {
		if got := DirectedBroadcast(test.base, test.prefix); got != test.want {
			t.Errorf("DirectedBroadcast(%q, %d) = %q, want %q", test.base, test.prefix, got, test.want)
		}
	}
}

func TestWakePlanCountsAndErrors(t *testing.T) {
	plan := WakePlan{State: "ready", Targets: []WakeTarget{{State: WakeTargetSend}, {State: WakeTargetOn}, {State: WakeTargetSend}}}
	if plan.HasErrors() || plan.Count(WakeTargetSend) != 2 || plan.Count(WakeTargetUnknown) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	plan.Issues = []ValidationIssue{{Field: "targets", Message: "pc99 is not a configured client"}}
	if !plan.HasErrors() {
		t.Fatal("a plan with an issue has no errors")
	}
	if (WakeReport{State: "partial"}).HasErrors() == false || (WakeReport{State: "completed"}).HasErrors() {
		t.Fatal("report errors follow its state")
	}
}
