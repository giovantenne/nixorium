package domain

import (
	"reflect"
	"testing"
)

func TestAssistantCoreOverrideFiles(t *testing.T) {
	candidate := map[string]TemplateFile{
		"b.nix":     {Mode: "100644", Data: []byte("{ lib, ... }: { services.x.enable = lib.mkForce false; }")},
		"a.nix":     {Mode: "100644", Data: []byte("{ boot.x = mkOverride 10 1; }")},
		"plain.nix": {Mode: "100644", Data: []byte("{ lib, ... }: { x = lib.mkDefault 1; y = lib.mkForceful; z = mkOptionDefault; }")},
		"note.md":   {Mode: "100644", Data: []byte("# mkForce is only mentioned in a comment\n")},
		"link.nix":  {Mode: "120000", Data: []byte("mkForce")},
	}
	paths := []string{"b.nix", "a.nix", "plain.nix", "note.md", "link.nix", "missing.nix"}
	got := AssistantCoreOverrideFiles(candidate, paths)
	if want := []string{"a.nix", "b.nix", "note.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := AssistantCoreOverrideFiles(candidate, nil); got == nil || len(got) != 0 {
		t.Fatalf("empty result must be a non-nil empty slice: %#v", got)
	}
}
