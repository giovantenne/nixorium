package domain

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestHomeFilesSharedCorpus(t *testing.T) {
	data, err := os.ReadFile("../../tests/home-files-validation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Valid []struct {
			Name, JSON string
			Normalized json.RawMessage
		}
		Invalid []struct{ Name, JSON string }
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Valid {
		t.Run(c.Name, func(t *testing.T) {
			p, issues := DecodeHomeFilesProfile([]byte(c.JSON))
			if len(issues) != 0 {
				t.Fatal(issues)
			}
			got, _ := json.Marshal(p)
			var a, b any
			json.Unmarshal(got, &a)
			json.Unmarshal(c.Normalized, &b)
			ga, _ := json.Marshal(a)
			gb, _ := json.Marshal(b)
			if string(ga) != string(gb) {
				t.Fatalf("got %s want %s", ga, gb)
			}
		})
	}
	for _, c := range corpus.Invalid {
		t.Run(c.Name, func(t *testing.T) {
			_, issues := DecodeHomeFilesProfile([]byte(c.JSON))
			if len(issues) == 0 {
				t.Fatal("invalid accepted")
			}
		})
	}
}
func TestHomeFilesBounds(t *testing.T) {
	for _, size := range []int{HomeFilesMaxTextBytes, HomeFilesMaxTextBytes + 1} {
		text := strings.Repeat("a", size)
		data, _ := json.Marshal(HomeFilesProfile{SchemaVersion: 1, Files: []HomeFile{{Destination: "a", Text: &text}}})
		_, issues := DecodeHomeFilesProfile(data)
		if (len(issues) == 0) != (size == HomeFilesMaxTextBytes) {
			t.Fatal(size, issues)
		}
	}
	for _, depth := range []int{12, 13} {
		if ValidHomeFilesDestination(strings.TrimSuffix(strings.Repeat("a/", depth), "/"), false) != (depth == 12) {
			t.Fatal(depth)
		}
	}
	data := append([]byte(`{"schemaVersion":1}`), 0xff)
	if _, issues := DecodeHomeFilesProfile(data); len(issues) == 0 {
		t.Fatal("invalid UTF-8 accepted")
	}
}
