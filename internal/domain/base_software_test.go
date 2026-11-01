package domain

import "testing"

func TestBaseSoftwareIDsAreValidPackageAttributes(t *testing.T) {
	for _, id := range baseSoftware {
		if !IsBaseSoftware(id) {
			t.Fatalf("base software %q is not reported as base software", id)
		}
		file := LabSoftwareFile{SchemaVersion: SoftwareSchemaVersion, Packages: []SoftwareDeclaration{{Package: id, Scope: SoftwareScope{Kind: SoftwareScopeAllClients}}}}
		if _, err := MarshalLabSoftware(file); err != nil {
			t.Fatalf("base software %q is not a valid package attribute: %v", id, err)
		}
	}
}
