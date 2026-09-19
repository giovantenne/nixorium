package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (Local) ActivateBootstrapKeyboard(ctx context.Context, consoleKeyMap string) error {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return errors.New("the bootstrap keyboard cannot be verified safely from a graphical terminal; switch to a Linux text console and retry")
	}
	if _, err := exec.LookPath("loadkeys"); err != nil {
		return errors.New("the live environment does not provide loadkeys; use the official NixOS installer in a Linux text console")
	}
	if _, err := run(ctx, "sudo", "loadkeys", consoleKeyMap); err != nil {
		return fmt.Errorf("activate console keymap %q: %w", consoleKeyMap, err)
	}
	return nil
}

func (Local) HashPassword(ctx context.Context, password []byte) (string, error) {
	input := make([]byte, len(password)+1)
	copy(input, password)
	input[len(input)-1] = '\n'
	defer func() {
		for index := range input {
			input[index] = 0
		}
	}()
	output, err := runWithInput(ctx, input, "mkpasswd", "-m", "sha-512", "--stdin")
	if err != nil {
		return "", err
	}
	hash := strings.TrimSpace(output)
	if !domain.IsPasswordHash(hash) {
		return "", errors.New("mkpasswd returned an invalid SHA-512 crypt hash")
	}
	return hash, nil
}
