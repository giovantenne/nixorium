package domain

import "testing"

func TestFileRecoveryPathsRefuseHiddenAndDisguisedNames(t *testing.T) {
	for _, name := range []string{"", "Desktop", "Desktop/essay (2).odt", "Documenti/città.txt"} {
		if err := ValidFileRecoveryPath(name); err != nil {
			t.Fatalf("%q refused: %v", name, err)
		}
	}
	for _, name := range []string{".ssh", "Desktop/.git", "..", "a/../b", "/etc", "a//b", "a\\b", "a\x1b[2J", "a\u009bb", "evil\u202etxt.exe", "a/"} {
		if err := ValidFileRecoveryPath(name); err == nil {
			t.Fatalf("%q accepted", name)
		}
	}
	if got := SafeFileRecoveryText("bad\x1b[2J\u202ename\u009b", 100); got != "bad [2J name " {
		t.Fatalf("safe text = %q", got)
	}
	if got := SafeFileRecoveryText("abcdef", 3); got != "abc" {
		t.Fatalf("cut text = %q", got)
	}
	if !ValidFileRecoverySessionID("3f1c2b7e-9d4a-4c1e-8f0a-2b6d5e7c9a10") || ValidFileRecoverySessionID("-") || ValidFileRecoverySessionID("3F1C2B7E-9D4A-4C1E-8F0A-2B6D5E7C9A10") {
		t.Fatal("session identities are not checked")
	}
}
