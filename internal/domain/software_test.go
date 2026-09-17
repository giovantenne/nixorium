package domain

import (
	"bytes"
	"testing"
)

func TestLabSoftwareRoundTripNormalizesDeclarations(t *testing.T) {
	software := LabSoftwareFile{SchemaVersion: SoftwareSchemaVersion, Packages: []SoftwareDeclaration{
		{Package: "vlc", Scope: SoftwareScope{Kind: SoftwareScopeClients, Clients: []string{"pc02", "pc01"}}},
		{Package: "gimp", Scope: SoftwareScope{Kind: SoftwareScopeAllClients}},
	}}
	data, err := MarshalLabSoftware(software)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeLabSoftware(data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := MarshalLabSoftware(decoded)
	if err != nil || !bytes.Equal(data, again) || decoded.Packages[0].Package != "gimp" || decoded.Packages[1].Scope.Clients[0] != "pc01" {
		t.Fatalf("software encoding is not deterministic: %v\n%s", err, data)
	}
}

func TestLabSoftwareRejectsUnsafeDeclarations(t *testing.T) {
	tests := []LabSoftwareFile{
		{SchemaVersion: 2, Packages: []SoftwareDeclaration{}},
		{SchemaVersion: 1, Packages: []SoftwareDeclaration{{Package: "${pkgs.vlc}", Scope: SoftwareScope{Kind: SoftwareScopeAllClients}}}},
		{SchemaVersion: 1, Packages: []SoftwareDeclaration{{Package: "vlc", Scope: SoftwareScope{Kind: SoftwareScopeClients, Clients: []string{"pc01", "pc01"}}}}},
		{SchemaVersion: 1, Packages: []SoftwareDeclaration{{Package: "vlc", Scope: SoftwareScope{Kind: SoftwareScopeGroup, Group: "../room"}}}},
		{SchemaVersion: 1, Packages: []SoftwareDeclaration{
			{Package: "vlc", Scope: SoftwareScope{Kind: SoftwareScopeAllClients}},
			{Package: "vlc", Scope: SoftwareScope{Kind: SoftwareScopeAllClients}},
		}},
	}
	for index, software := range tests {
		if _, err := MarshalLabSoftware(software); err == nil {
			t.Errorf("unsafe software fixture %d was accepted", index)
		}
	}
}

func TestLabSoftwareDecoderRejectsUnknownAndTrailingJSON(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(`{"schemaVersion":1,"packages":[],"unknown":true}`),
		[]byte(`{"schemaVersion":1,"packages":[]} {}`),
	} {
		if _, err := DecodeLabSoftware(data); err == nil {
			t.Fatal("unsafe JSON was accepted")
		}
	}
}

func TestControllerSoftwareScopesRoundTripWithoutMigratingClients(t *testing.T) {
	for _, kind := range []string{SoftwareScopeShared, SoftwareScopeController, SoftwareScopeAllClients} {
		software := LabSoftwareFile{SchemaVersion: 1, Packages: []SoftwareDeclaration{{Package: "hello", Scope: SoftwareScope{Kind: kind}}}}
		data, err := MarshalLabSoftware(software)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeLabSoftware(data)
		if err != nil || decoded.Packages[0].Scope.Kind != kind {
			t.Fatalf("scope changed: %s %v", data, err)
		}
		for _, scope := range []SoftwareScope{{Kind: kind, Group: "graphics"}, {Kind: kind, Clients: []string{"pc01"}}} {
			if ValidateSoftwareScope(scope) == nil {
				t.Fatalf("accepted unrelated fields: %+v", scope)
			}
		}
	}
}

func TestEmptySoftwarePackagesEncodeAsList(t *testing.T) {
	for _, packages := range [][]SoftwareDeclaration{nil, {}} {
		data, err := MarshalLabSoftware(LabSoftwareFile{SchemaVersion: 1, Packages: packages})
		if err != nil || !bytes.Contains(data, []byte(`"packages": []`)) {
			t.Fatalf("empty declaration must stay a JSON list: %s %v", data, err)
		}
	}
}
