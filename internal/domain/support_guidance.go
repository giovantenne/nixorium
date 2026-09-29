package domain

import "sort"

const SupportGuidanceVersion = 1

// Routes refer to the canonical guide in the same product source, not a
// separately maintained remediation engine. Looking up a route performs no IO.
var supportGuides = map[string]string{
	"CONFIG-EVAL":            "configuration-is-invalid",
	"NETWORK-SUBNET":         "configuration-is-invalid",
	"DEPLOYMENT-READY":       "configuration-is-invalid",
	"GIT-WORKTREE":           "the-git-tree-is-dirty",
	"NETWORK-INTERFACE":      "the-controller-network-is-inconsistent",
	"NETWORK-STATIC-IP":      "the-controller-network-is-inconsistent",
	"NETWORK-DHCP-IP":        "the-controller-dhcp-lease-changed",
	"PXE-PREPARATION":        "the-pxe-menu-appears-but-boot-fails",
	"ARTIFACT-KERNEL":        "the-pxe-menu-appears-but-boot-fails",
	"ARTIFACT-INITRD":        "the-pxe-menu-appears-but-boot-fails",
	"ARTIFACT-IPXE-SCRIPT":   "the-pxe-menu-appears-but-boot-fails",
	"ARTIFACT-IPXE-FIRMWARE": "the-pxe-menu-appears-but-boot-fails",
	"SERVICE-HARMONIA":       "the-binary-cache-is-unavailable",
	"CACHE-HEALTH":           "the-binary-cache-is-unavailable",
	"SERVICE-PXE":            "a-pxe-client-does-not-appear",
	"SERVICE-PXE-NETWORK":    "a-pxe-client-does-not-appear",
	"PXE-PORTS":              "a-pxe-client-does-not-appear",
	"PXE-LIFECYCLE":          "interrupted-operation-retry-matrix",
	"CACHE-SIGNING-KEY":      "keys-are-missing-or-inconsistent",
	"DISK-FREE":              "build-resources-and-controller-tools",
	"COMMAND-NIX":            "build-resources-and-controller-tools",
	"COMMAND-GIT":            "build-resources-and-controller-tools",
	"COMMAND-SYSTEMCTL":      "build-resources-and-controller-tools",
	"COMMAND-SSH":            "build-resources-and-controller-tools",
	"COMMAND-COLMENA":        "build-resources-and-controller-tools",
	"CLIENT-SSH":             "one-client-is-offline-or-unknown",
	"CONTROLLER-BUILD":       "controller-apply-failed",
}

// SupportFindingIDs is an exact allowlist, never a prefix match for dynamic
// artifact/service names or future IDs that might contain private input.
func SupportFindingIDs() []string {
	ids := make([]string, 0, len(supportGuides))
	for id := range supportGuides {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func SupportFindingGuide(id string) string {
	if fragment, ok := supportGuides[id]; ok {
		return "docs/troubleshooting.md#" + fragment
	}
	return ""
}
