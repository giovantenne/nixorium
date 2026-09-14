package domain

import (
	"strings"
	"testing"
)

const validControllerActivation = `{
  "schemaVersion": 1,
  "revision": "0123456789abcdef0123456789abcdef01234567",
  "systemPath": "/nix/store/0123456789abcdfghijklmnpqrsvwxyz-nixos-system-pc99",
  "activatedAt": "2026-09-14T17:00:00Z"
}`

func TestDecodeControllerActivationStrictly(t *testing.T) {
	record, err := DecodeControllerActivation([]byte(validControllerActivation))
	if err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != ControllerActivationSchemaVersion || record.Revision != "0123456789abcdef0123456789abcdef01234567" || record.ActivatedAt.IsZero() {
		t.Fatalf("unexpected record: %+v", record)
	}

	mutations := []string{
		strings.Replace(validControllerActivation, `"schemaVersion": 1`, `"schemaVersion": 2`, 1),
		strings.Replace(validControllerActivation, `0123456789abcdef0123456789abcdef01234567`, `short`, 1),
		strings.Replace(validControllerActivation, `/nix/store/0123456789abcdfghijklmnpqrsvwxyz-nixos-system-pc99`, `/tmp/system`, 1),
		strings.Replace(validControllerActivation, `2026-09-14T17:00:00Z`, `0001-01-01T00:00:00Z`, 1),
		strings.Replace(validControllerActivation, `"activatedAt"`, `"unexpected"`, 1),
		validControllerActivation + `{}`,
	}
	for _, mutation := range mutations {
		if _, err := DecodeControllerActivation([]byte(mutation)); err == nil {
			t.Fatalf("accepted invalid activation record: %s", mutation)
		}
	}
}
