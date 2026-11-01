package adapters

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ReadFixedDeploymentPath reads the canonical deployment path from the
// root-owned configuration file that the controller workers trust.
func ReadFixedDeploymentPath(path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("deployment path configuration filename is invalid")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect deployment path configuration: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || !ok || stat.Uid != 0 || info.Size() < 2 || info.Size() > 4096 {
		return "", errors.New("deployment path configuration must be a small root-owned non-writable regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	repository := strings.TrimSpace(string(content))
	if strings.ContainsAny(repository, "\r\n\x00") || !filepath.IsAbs(repository) || filepath.Clean(repository) != repository {
		return "", errors.New("configured deployment path is not canonical")
	}
	return repository, nil
}
