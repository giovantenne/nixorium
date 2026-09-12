package domain

import (
	"strings"
	"testing"
)

const validPXEPreparation = `{
  "schemaVersion": 1,
  "revision": "0123456789abcdef0123456789abcdef01234567",
  "preparedAt": "2026-09-11T12:00:00Z",
  "controller": {"dhcpIp": "192.0.2.10", "staticIp": "10.0.0.99"},
  "network": {"ifaceName": "eth0", "cachePort": 5000, "pxeHttpPort": 8080},
  "artifacts": {
    "kernel": {"storePath": "/nix/store/00000000000000000000000000000000-kernel", "relativePath": "bzImage"},
    "initrd": {"storePath": "/nix/store/11111111111111111111111111111111-initrd", "relativePath": "initrd"},
    "ipxeScript": {"storePath": "/nix/store/22222222222222222222222222222222-ipxe", "relativePath": "netboot.ipxe"},
    "firmware": {"storePath": "/nix/store/33333333333333333333333333333333-firmware", "relativePath": "snponly.efi"}
  },
  "clients": [{"name": "pc01", "storePath": "/nix/store/44444444444444444444444444444444-client"}]
}`

func TestDecodePXEPreparationStrictly(t *testing.T) {
	record, err := DecodePXEPreparation([]byte(validPXEPreparation))
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision == "" || len(record.Clients) != 1 {
		t.Fatalf("record = %+v", record)
	}
	for _, mutation := range []string{
		strings.Replace(validPXEPreparation, `"schemaVersion": 1`, `"schemaVersion": 2`, 1),
		strings.Replace(validPXEPreparation, `"revision": "0123456789abcdef0123456789abcdef01234567"`, `"revision": "short"`, 1),
		strings.Replace(validPXEPreparation, `"dhcpIp": "192.0.2.10"`, `"dhcpIp": "2001:db8::10"`, 1),
		strings.Replace(validPXEPreparation, `"relativePath": "bzImage"`, `"relativePath": "../bzImage"`, 1),
		strings.Replace(validPXEPreparation, `"relativePath": "bzImage"`, `"relativePath": "kernel"`, 1),
		strings.Replace(validPXEPreparation, `/nix/store/00000000000000000000000000000000-kernel`, `/nix/store/00000000000000000000000000000000-kernel/../secret`, 1),
		strings.Replace(validPXEPreparation, `"clients": [`, `"unknown": true, "clients": [`, 1),
		validPXEPreparation + `{}`,
	} {
		if _, err := DecodePXEPreparation([]byte(mutation)); err == nil {
			t.Fatalf("invalid preparation accepted: %s", mutation)
		}
	}
}
