package domain

import "testing"

func TestVSCodeEngineAccepts(t *testing.T) {
	for _, c := range []struct {
		requirement, editor string
		accepted            bool
	}{
		{"*", "1.119.0", true},
		{"^1.95.0", "1.119.0", true},
		{"^1.119.0", "1.119.0", true},
		{"^1.120.0", "1.119.0", false},
		{"^1.110.0-20260204", "1.119.0", true},
		{">=1.70.0", "1.119.0", true},
		{">=1.200.0", "1.119.0", false},
		{"1.119.1", "1.119.0", false},
		{"^2.0.0", "1.119.0", false},
		{"^0.10.x", "1.119.0", true},
		{"^1.x.x", "1.119.0", true},
	} {
		accepted, err := VSCodeEngineAccepts(c.requirement, c.editor)
		if err != nil || accepted != c.accepted {
			t.Fatalf("%s with %s: %t %v", c.requirement, c.editor, accepted, err)
		}
	}
	for _, requirement := range []string{"latest", "~1.2.3", "1.2", ""} {
		if _, err := VSCodeEngineAccepts(requirement, "1.119.0"); err == nil {
			t.Fatalf("interpreted %q", requirement)
		}
	}
	if _, err := VSCodeEngineAccepts("^1.0.0", "unknown"); err == nil {
		t.Fatal("accepted an unknown editor version")
	}
}

func TestNormalizeWorkspaceMarketplaceID(t *testing.T) {
	if id, err := NormalizeWorkspaceMarketplaceID(" PlatformIO.platformio-IDE "); err != nil || id != "platformio.platformio-ide" {
		t.Fatalf("got %q %v", id, err)
	}
	for _, id := range []string{"", "platformio", "a.b.c", "../x.y", "a b.c", "https://x.y"} {
		if _, err := NormalizeWorkspaceMarketplaceID(id); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
}
