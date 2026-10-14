package main

import (
	"fmt"
	"os"
	"syscall"
)

// sanitizeRuntimeDir keeps child systemd tools from watching another user's
// runtime directory, which `su` without a dash leaves in XDG_RUNTIME_DIR.
func sanitizeRuntimeDir(getenv func(string) string, setenv func(string, string) error, unsetenv func(string) error, owner func(string) (int, bool), uid int) {
	current := getenv("XDG_RUNTIME_DIR")
	if current == "" {
		return
	}
	if ownerUID, ok := owner(current); ok && ownerUID == uid {
		return
	}
	if own := fmt.Sprintf("/run/user/%d", uid); own != current {
		if ownerUID, ok := owner(own); ok && ownerUID == uid {
			_ = setenv("XDG_RUNTIME_DIR", own)
			return
		}
	}
	_ = unsetenv("XDG_RUNTIME_DIR")
}

func pathOwner(path string) (int, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}
