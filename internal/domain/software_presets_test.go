package domain

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedSoftwarePresetValidationCases(t *testing.T) {
	type validationCase struct {
		Name    string          `json:"name"`
		Catalog json.RawMessage `json:"catalog"`
	}
	type validationCases struct {
		Valid   []validationCase `json:"valid"`
		Invalid []validationCase `json:"invalid"`
	}

	data, err := os.ReadFile("../../tests/software-preset-validation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases validationCases
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases.Valid) == 0 || len(cases.Invalid) == 0 {
		t.Fatal("shared preset validation corpus must contain valid and invalid cases")
	}
	for _, testCase := range cases.Valid {
		t.Run("valid/"+testCase.Name, func(t *testing.T) {
			catalog, err := DecodeSoftwarePresetCatalog(testCase.Catalog)
			if err != nil {
				t.Fatalf("valid catalog rejected: %v", err)
			}
			first, err := SoftwarePresetCatalogFingerprint(catalog)
			if err != nil {
				t.Fatal(err)
			}
			second, err := SoftwarePresetCatalogFingerprint(catalog)
			if err != nil || first != second {
				t.Fatalf("preset fingerprint is not deterministic: %q %q %v", first, second, err)
			}
		})
	}
	for _, testCase := range cases.Invalid {
		t.Run("invalid/"+testCase.Name, func(t *testing.T) {
			if _, err := DecodeSoftwarePresetCatalog(testCase.Catalog); err == nil {
				t.Fatal("invalid preset catalog accepted")
			}
		})
	}
}
