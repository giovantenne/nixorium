package adapters

import (
	"context"
	"errors"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

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
