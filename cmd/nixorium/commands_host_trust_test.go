package main

import "testing"

func TestHostTrustArgumentsRequireExplicitTargetAndReviewedApply(t *testing.T) {
	for _, test := range []struct {
		args  []string
		valid bool
	}{
		{[]string{"host-key", "plan", "--host", "pc01"}, true},
		{[]string{"host-key", "apply", "--host", "pc01", "--expect", "sha256:review", "--yes"}, true},
		{[]string{"host-key"}, false}, {[]string{"host-key", "plan"}, false},
		{[]string{"host-key", "apply", "--host", "pc01"}, false},
		{[]string{"host-key", "plan", "--host", "all"}, false},
		{[]string{"host-key", "plan", "--host", "pc01", "--yes"}, false},
		{[]string{"host-key", "plan", "--host", "pc01", "--expect", "review"}, false},
		{[]string{"host-key", "plan", "--host", "pc01", "--on", "pc02"}, false},
	} {
		_, err := parseArguments(test.args)
		if (err == nil) != test.valid {
			t.Fatalf("%v: %v", test.args, err)
		}
	}
}
