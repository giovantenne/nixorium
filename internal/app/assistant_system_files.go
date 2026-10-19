package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/systemfiles"
)

// AssistantSystemFilesSource reads fresh inventory and one allowlisted path.
type AssistantSystemFilesSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	ReadSystemFile(context.Context, domain.HostMeta, bool, string) (systemfiles.Result, error)
}

// AssistantSystemFiles is the read_system_file tool (ADR 0032).
type AssistantSystemFiles struct {
	Source AssistantSystemFilesSource
	// Errors maps the source's typed errors to tool error codes.
	Errors map[error]string
}

// Execute reads fresh declared inventory: the conversation cannot choose a
// repository, an address or a computer outside it.
func (files AssistantSystemFiles) Execute(ctx context.Context, repository, host, path string) string {
	failure := func(code string) string {
		data, _ := json.Marshal(map[string]any{"ok": false, "error": code})
		return string(data)
	}
	if !diagnosticHostName.MatchString(host) || !systemfiles.ValidRequest(path) {
		return failure("invalid_arguments")
	}
	if !systemfiles.Readable(path) {
		return failure("refused_path")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if files.Source == nil {
		return failure("system_files_unavailable")
	}
	meta, err := files.Source.LabMeta(ctx, repository)
	if err != nil || ctx.Err() != nil {
		return failure("inventory_unavailable")
	}
	selected, controller := domain.HostMeta{}, false
	all := append([]domain.HostMeta{{Name: meta.Controller.Name, IP: meta.Controller.StaticIP}}, meta.Clients.Hosts...)
	for index, declared := range all {
		if !diagnosticHostName.MatchString(declared.Name) || index != 0 && net.ParseIP(declared.IP).To4() == nil {
			return failure("inventory_unavailable")
		}
		if declared.Name == host {
			selected, controller = declared, index == 0
		}
	}
	if selected.Name == "" {
		return failure("unknown_host")
	}
	result, err := files.Source.ReadSystemFile(ctx, selected, controller, path)
	if err != nil {
		for typed, code := range files.Errors {
			if errors.Is(err, typed) {
				return failure(code)
			}
		}
		return failure("system_files_unavailable")
	}
	data, err := json.Marshal(struct {
		OK   bool               `json:"ok"`
		Host string             `json:"host"`
		File systemfiles.Result `json:"file"`
	}{true, host, result})
	if err != nil || len(data) > systemfiles.MaxOutputBytes {
		return failure("system_files_unavailable")
	}
	return domain.RedactForModel(string(data))
}
