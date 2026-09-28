package adapters

import (
	"os"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// ReadWorkspaceCandidate reads only a bounded regular file. In particular, a
// named pipe must not block the management command before schema validation.
func ReadWorkspaceCandidate(path string) ([]byte, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, filepath.Dir(absolute), &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return nil, err
	}
	root := os.NewFile(uintptr(fd), filepath.Dir(absolute))
	defer root.Close()
	data, _, err := readWorkspaceFile(root, filepath.Base(absolute), domain.WorkspaceMaxBytes)
	return data, err
}
