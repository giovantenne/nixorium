package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestWorkspaceSharedCorpus(t *testing.T) {
	data, err := os.ReadFile("../../tests/workspace-validation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	type testCase struct {
		Name       string          `json:"name"`
		JSON       string          `json:"json"`
		Normalized json.RawMessage `json:"normalized"`
	}
	var corpus struct{ Valid, Invalid []testCase }
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Valid {
		t.Run(c.Name, func(t *testing.T) {
			profile, issues := DecodeWorkspaceProfile([]byte(c.JSON))
			if len(issues) != 0 {
				t.Fatal(issues)
			}
			encoded, err := MarshalWorkspaceProfile(profile)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(c.Normalized, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %s, want %s", encoded, c.Normalized)
			}
			roundTrip, issues := DecodeWorkspaceProfile(encoded)
			if len(issues) != 0 || !reflect.DeepEqual(profile, roundTrip) {
				t.Fatalf("unstable round trip: %v", issues)
			}
		})
	}
	for _, c := range corpus.Invalid {
		t.Run(c.Name, func(t *testing.T) {
			if _, issues := DecodeWorkspaceProfile([]byte(c.JSON)); len(issues) == 0 {
				t.Fatal("accepted invalid profile")
			}
		})
	}
}

func TestWorkspaceLimits(t *testing.T) {
	list := func(n int, pattern string) []string {
		result := make([]string, n)
		for i := range result {
			result[i] = fmt.Sprintf(pattern, i)
		}
		return result
	}
	for _, c := range []struct {
		name    string
		profile map[string]any
		valid   bool
	}{
		{"favorites at limit", map[string]any{"desktop": map[string]any{"favorites": list(32, "app%d.desktop")}}, true},
		{"favorites over limit", map[string]any{"desktop": map[string]any{"favorites": list(33, "app%d.desktop")}}, false},
		{"extensions at limit", map[string]any{"vscode": map[string]any{"extensions": list(64, "publisher.extension%d")}}, true},
		{"extensions over limit", map[string]any{"vscode": map[string]any{"extensions": list(65, "publisher.extension%d")}}, false},
		{"desktop ID at limit", map[string]any{"browser": map[string]any{"defaultApplication": strings.Repeat("a", 120) + ".desktop"}}, true},
		{"desktop ID over limit", map[string]any{"browser": map[string]any{"defaultApplication": strings.Repeat("a", 121) + ".desktop"}}, false},
		{"extension ID at limit", map[string]any{"vscode": map[string]any{"extensions": []string{"a." + strings.Repeat("a", 126)}}}, true},
		{"extension ID over limit", map[string]any{"vscode": map[string]any{"extensions": []string{"a." + strings.Repeat("a", 127)}}}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.profile["schemaVersion"] = 1
			data, err := json.Marshal(c.profile)
			if err != nil {
				t.Fatal(err)
			}
			_, issues := DecodeWorkspaceProfile(data)
			if (len(issues) == 0) != c.valid {
				t.Fatalf("unexpected validation: %v", issues)
			}
		})
	}
	base := `{"schemaVersion":1}`
	for _, size := range []int{WorkspaceMaxBytes, WorkspaceMaxBytes + 1} {
		_, issues := DecodeWorkspaceProfile([]byte(base + strings.Repeat(" ", size-len(base))))
		if (len(issues) == 0) != (size == WorkspaceMaxBytes) {
			t.Fatalf("wrong byte limit at %d", size)
		}
	}
}

func TestWorkspaceMalformedJSON(t *testing.T) {
	for _, input := range []string{"", "{", `{"schemaVersion":1} {}`, `{"schemaVersion":1,}`, `{"schemaVersion":1}// comment`, `{"schemaVersion":1,"desktop":{"colorScheme":"` + string([]byte{0xff}) + `"}}`} {
		if _, issues := DecodeWorkspaceProfile([]byte(input)); len(issues) == 0 {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestWorkspaceMarshalDoesNotMutate(t *testing.T) {
	extensions := []string{"redhat.java", "a.extension"}
	favorites := []string{"z.desktop", "a.desktop"}
	p := WorkspaceProfile{SchemaVersion: 1, Desktop: &WorkspaceDesktop{Favorites: &favorites}, VSCode: &WorkspaceVSCode{Extensions: &extensions}}
	if _, err := MarshalWorkspaceProfile(p); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(extensions, []string{"redhat.java", "a.extension"}) || !slices.Equal(favorites, []string{"z.desktop", "a.desktop"}) {
		t.Fatal("marshal mutated input")
	}
	var nullList []string
	p.VSCode.Extensions = &nullList
	if len(p.Validate()) == 0 {
		t.Fatal("accepted a null list constructed in Go")
	}
	if _, err := MarshalWorkspaceProfile(p); err == nil {
		t.Fatal("encoded invalid profile")
	}
}

func FuzzWorkspaceDecode(f *testing.F) {
	f.Add(`{"schemaVersion":1}`)
	f.Add(`{"schemaVersion":1,"desktop":{"favorites":[]}}`)
	f.Fuzz(func(t *testing.T, data string) {
		p, issues := DecodeWorkspaceProfile([]byte(data))
		if len(issues) != 0 {
			return
		}
		encoded, err := MarshalWorkspaceProfile(p)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip, issues := DecodeWorkspaceProfile(encoded)
		if len(issues) != 0 || !reflect.DeepEqual(p, roundTrip) {
			t.Fatalf("unstable decode: %v", issues)
		}
	})
}
