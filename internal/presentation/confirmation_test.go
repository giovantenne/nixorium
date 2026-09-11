package presentation

import (
	"bytes"
	"strings"
	"testing"
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
