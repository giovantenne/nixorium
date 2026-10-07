package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// LoadAssistantConfig fails closed and never follows a symbolic link or blocks
// on a special file. Only an installed controller configuration enables it.
func LoadAssistantConfig(path string) (domain.AssistantConfig, error) {
	disabled := domain.AssistantConfig{}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return disabled, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return disabled, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return disabled, errors.New("invalid Assistant configuration file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return disabled, err
	}
	if len(data) > 4096 {
		return disabled, errors.New("Assistant configuration is too large")
	}
	var wire struct {
		SchemaVersion int    `json:"schemaVersion"`
		Enabled       bool   `json:"enabled"`
		Endpoint      string `json:"endpoint"`
		SubscribeURL  string `json:"subscribeUrl"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return disabled, errors.New("invalid Assistant configuration JSON")
	}
	if decoder.Decode(&struct{}{}) != io.EOF || wire.SchemaVersion != 1 ||
		!assistantConfigURL(wire.Endpoint, true) || !assistantConfigURL(wire.SubscribeURL, false) {
		return disabled, errors.New("invalid Assistant configuration")
	}
	return domain.AssistantConfig{Enabled: wire.Enabled, Endpoint: wire.Endpoint, SubscribeURL: wire.SubscribeURL}, nil
}

func assistantConfigURL(value string, localEndpoint bool) bool {
	if strings.Contains(value, "#") {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}
	if u.Scheme == "https" {
		return true
	}
	return localEndpoint && u.Scheme == "http" && u.Port() != "" &&
		(u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")
}
