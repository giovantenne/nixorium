package classroomview

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateClipboardText(t *testing.T) {
	if got, err := ValidateClipboardText("a\x00b\nc"); err != nil || got != "a"+"b\nc" {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := ValidateClipboardText(strings.Repeat("x", MaxClipboardBytes)); err != nil || len(got) != MaxClipboardBytes {
		t.Fatalf("limit: %d, %v", len(got), err)
	}
	if _, err := ValidateClipboardText(strings.Repeat("x", MaxClipboardBytes+1)); !errors.Is(err, ErrClipboardTooLarge) {
		t.Fatalf("over the limit: %v", err)
	}
	if _, err := ValidateClipboardText("\xff\xfe"); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
