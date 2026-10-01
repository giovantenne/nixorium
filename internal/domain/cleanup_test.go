package domain

import "testing"

func TestParseCleanupPlan(t *testing.T) {
	plan, ok := ParseCleanupPlan("format 1\nkeep 15 newest\nkeep 2 running\nremove 1\nfree 1000\nexpect 0123456789abcdef\n")
	if !ok || len(plan.Keep) != 2 || plan.Keep[1].Reason != "running" || len(plan.Remove) != 1 || plan.FreeBytes != 1000 || plan.Expect != "0123456789abcdef" {
		t.Fatalf("plan = %+v ok=%v", plan, ok)
	}
	for _, invalid := range []string{
		"",
		"format 2\nfree 1\nexpect 0123456789abcdef",
		"format 1\nfree 1",
		"format 1\nkeep 01 newest\nfree 1\nexpect 0123456789abcdef",
		"format 1\nkeep 3 favourite\nfree 1\nexpect 0123456789abcdef",
		"format 1\nremove -1\nfree 1\nexpect 0123456789abcdef",
		"format 1\nfree 1\nexpect 0123456789ABCDEF",
		"format 1\nfree 1\nexpect 0123456789abcdef\nrm -rf /",
	} {
		if _, ok := ParseCleanupPlan(invalid); ok {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

func TestParseCleanupApply(t *testing.T) {
	result, ok := ParseCleanupApply("format 1\nremoved 1 4\nfree-before 10\nfree-after 20\nboot-menu failed\n")
	if !ok || result.State != "cleaned" || len(result.Removed) != 2 || result.FreeAfter != 20 || !result.BootMenuFailed {
		t.Fatalf("result = %+v ok=%v", result, ok)
	}
	if result, ok := ParseCleanupApply("changed\n"); !ok || result.State != "changed" {
		t.Fatalf("changed = %+v", result)
	}
	if result, ok := ParseCleanupApply("format 1\nunchanged\nfree-before 1\nfree-after 1\n"); !ok || result.State != "unchanged" {
		t.Fatalf("unchanged = %+v", result)
	}
	if _, ok := ParseCleanupApply("format 1\nremoved 1\n"); ok {
		t.Fatal("accepted a result without free space")
	}
}
