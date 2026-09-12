package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestConfirmControllerApplyRequiresExactToken(t *testing.T) {
	for _, test := range []struct {
		input string
		want  bool
	}{{"APPLY\n", true}, {"yes\n", false}, {"apply\n", false}} {
		output := &bytes.Buffer{}
		got, err := ConfirmControllerApply(strings.NewReader(test.input), output, "pc99")
		if err != nil || got != test.want || !strings.Contains(output.String(), "services and networking may restart") {
			t.Fatalf("input %q: got %v, error %v, output %q", test.input, got, err, output)
		}
	}
}

func TestConfirmPXEStartRequiresExactToken(t *testing.T) {
	report := domain.PXELifecycleReport{Interface: "enp1s0", DHCPAddress: "192.0.2.10", StaticCIDR: "10.0.0.99/8"}
	for _, test := range []struct {
		input string
		want  bool
	}{{"START PXE\n", true}, {"start pxe\n", false}, {"yes\n", false}} {
		output := &bytes.Buffer{}
		got, err := ConfirmPXEStart(strings.NewReader(test.input), output, report)
		if err != nil || got != test.want || !strings.Contains(output.String(), "10.0.0.99/8") || !strings.Contains(output.String(), "reboot recovery is enabled") {
			t.Fatalf("input %q: got %v, error %v, output %q", test.input, got, err, output)
		}
	}
}
