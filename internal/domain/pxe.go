package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"
)

const PXEPreparationSchemaVersion = 1

var (
	gitRevisionPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	clientNamePattern   = regexp.MustCompile(`^pc[0-9]+$`)
	storePathPattern    = regexp.MustCompile(`^/nix/store/[0-9a-z]{32}-[^/\n\r\t ]+$`)
	artifactNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
)

type PXEPreparedArtifact struct {
	StorePath    string `json:"storePath"`
	RelativePath string `json:"relativePath"`
}

type PXEPreparedClient struct {
	Name      string `json:"name"`
	StorePath string `json:"storePath"`
}

type PXEPreparationRecord struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      string    `json:"revision"`
	PreparedAt    time.Time `json:"preparedAt"`
	Controller    struct {
		DHCPIP   string `json:"dhcpIp"`
		StaticIP string `json:"staticIp"`
	} `json:"controller"`
	Network struct {
		Interface   string `json:"ifaceName"`
		CachePort   int    `json:"cachePort"`
		PXEHTTPPort int    `json:"pxeHttpPort"`
	} `json:"network"`
	Artifacts struct {
		Kernel     PXEPreparedArtifact `json:"kernel"`
		Initrd     PXEPreparedArtifact `json:"initrd"`
		IPXEScript PXEPreparedArtifact `json:"ipxeScript"`
		Firmware   PXEPreparedArtifact `json:"firmware"`
	} `json:"artifacts"`
	Clients []PXEPreparedClient `json:"clients"`
}

type PXEPreparationState struct {
	Present     bool                `json:"present"`
	Ready       bool                `json:"ready"`
	Detail      string              `json:"detail,omitempty"`
	Revision    string              `json:"revision,omitempty"`
	DHCPAddress string              `json:"dhcpAddress,omitempty"`
	PreparedAt  time.Time           `json:"preparedAt,omitempty"`
	Artifacts   []ArtifactState     `json:"artifacts,omitempty"`
	Clients     []PXEPreparedClient `json:"clients,omitempty"`
}

func DecodePXEPreparation(data []byte) (PXEPreparationRecord, error) {
	var record PXEPreparationRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return record, fmt.Errorf("decode PXE preparation: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return record, errors.New("decode PXE preparation: trailing JSON data")
	}
	if record.SchemaVersion != PXEPreparationSchemaVersion {
		return record, fmt.Errorf("unsupported PXE preparation schema version %d", record.SchemaVersion)
	}
	if !gitRevisionPattern.MatchString(record.Revision) {
		return record, errors.New("PXE preparation revision is not a full Git object ID")
	}
	if record.PreparedAt.IsZero() {
		return record, errors.New("PXE preparation timestamp is missing")
	}
	if !validIPv4(record.Controller.DHCPIP) || !validIPv4(record.Controller.StaticIP) {
		return record, errors.New("PXE preparation contains an invalid controller address")
	}
	if strings.TrimSpace(record.Network.Interface) == "" {
		return record, errors.New("PXE preparation interface is missing")
	}
	if record.Network.CachePort < 1 || record.Network.CachePort > 65535 || record.Network.PXEHTTPPort < 1 || record.Network.PXEHTTPPort > 65535 {
		return record, errors.New("PXE preparation contains an invalid network port")
	}
	artifacts := []struct {
		artifact PXEPreparedArtifact
		name     string
	}{
		{artifact: record.Artifacts.Kernel, name: "bzImage"},
		{artifact: record.Artifacts.Initrd, name: "initrd"},
		{artifact: record.Artifacts.IPXEScript, name: "netboot.ipxe"},
		{artifact: record.Artifacts.Firmware, name: "snponly.efi"},
	}
	for _, expected := range artifacts {
		artifact := expected.artifact
		if !validStorePath(artifact.StorePath) || !artifactNamePattern.MatchString(artifact.RelativePath) || artifact.RelativePath != expected.name {
			return record, errors.New("PXE preparation contains an invalid artifact path")
		}
	}
	if len(record.Clients) == 0 {
		return record, errors.New("PXE preparation contains no client closures")
	}
	seen := map[string]bool{}
	for _, client := range record.Clients {
		if !clientNamePattern.MatchString(client.Name) || !validStorePath(client.StorePath) || seen[client.Name] {
			return record, errors.New("PXE preparation contains an invalid or duplicate client closure")
		}
		seen[client.Name] = true
	}
	return record, nil
}

func validStorePath(path string) bool {
	return storePathPattern.MatchString(path)
}

func validIPv4(address string) bool {
	parsed := net.ParseIP(address)
	return parsed != nil && parsed.To4() != nil && parsed.String() == address
}
