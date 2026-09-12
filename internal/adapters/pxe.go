package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	pxePreparationPath      = "/var/lib/nixorium/prepared/prepared.json"
	maximumPreparationBytes = 1024 * 1024
)

func (Local) PXEPreparation(ctx context.Context, repository string, meta domain.LabMeta) domain.PXEPreparationState {
	state := domain.PXEPreparationState{}
	data, _, err := readRegularFileNoFollowLimit(pxePreparationPath, maximumPreparationBytes)
	if err != nil {
		if os.IsNotExist(err) {
			state.Detail = "managed PXE preparation has not run"
		} else {
			state.Detail = fmt.Sprintf("read managed PXE preparation: %v", err)
		}
		return state
	}
	state.Present = true
	record, err := domain.DecodePXEPreparation(data)
	if err != nil {
		state.Detail = err.Error()
		return state
	}
	state.Revision = record.Revision
	state.PreparedAt = record.PreparedAt
	state.Clients = record.Clients

	revision, err := run(ctx, "git", "-C", repository, "rev-parse", "HEAD")
	if err != nil {
		state.Detail = fmt.Sprintf("resolve deployment revision: %v", err)
		return state
	}
	revision = strings.TrimSpace(revision)
	if revision != record.Revision {
		state.Detail = fmt.Sprintf("prepared revision %s differs from deployment revision %s", record.Revision, revision)
		return state
	}
	if record.Controller.DHCPIP != meta.Controller.DHCPIP || record.Controller.StaticIP != meta.Controller.StaticIP ||
		record.Network.Interface != meta.Network.Interface || record.Network.CachePort != meta.Network.CachePort ||
		record.Network.PXEHTTPPort != meta.Network.PXEHTTPPort {
		state.Detail = "prepared controller/network values differ from current labMeta"
		return state
	}
	if len(record.Clients) != len(meta.Clients.Hosts) {
		state.Detail = "prepared client closure count differs from current labMeta"
		return state
	}
	for index, host := range meta.Clients.Hosts {
		if record.Clients[index].Name != host.Name {
			state.Detail = "prepared client identities differ from current labMeta"
			return state
		}
		if info, statErr := os.Stat(record.Clients[index].StorePath); statErr != nil || !info.IsDir() {
			state.Detail = fmt.Sprintf("prepared client closure is unavailable: %s", host.Name)
			return state
		}
	}

	artifacts := []struct {
		name   string
		record domain.PXEPreparedArtifact
	}{
		{name: "kernel", record: record.Artifacts.Kernel},
		{name: "initrd", record: record.Artifacts.Initrd},
		{name: "iPXE script", record: record.Artifacts.IPXEScript},
		{name: "iPXE firmware", record: record.Artifacts.Firmware},
	}
	for _, artifact := range artifacts {
		path := filepath.Join(artifact.record.StorePath, artifact.record.RelativePath)
		info, statErr := os.Stat(path)
		present := statErr == nil && info.Mode().IsRegular()
		state.Artifacts = append(state.Artifacts, domain.ArtifactState{
			Name:    artifact.name,
			Path:    path,
			Present: present,
		})
		if !present {
			state.Detail = fmt.Sprintf("prepared %s is unavailable", artifact.name)
			return state
		}
	}
	state.Ready = true
	state.Detail = fmt.Sprintf("prepared %d client closures at revision %s", len(record.Clients), record.Revision)
	return state
}
