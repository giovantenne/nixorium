package app

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

const minimumPasswordBytes = 8

type SecretReader interface {
	ReadSecret(prompt string) ([]byte, error)
}

type PasswordHasher interface {
	HashPassword(ctx context.Context, password []byte) (string, error)
}

func CollectPasswordHash(ctx context.Context, reader SecretReader, hasher PasswordHasher) (string, error) {
	return CollectNamedPasswordHash(ctx, reader, hasher, "Password")
}

func CollectNamedPasswordHash(ctx context.Context, reader SecretReader, hasher PasswordHasher, label string) (string, error) {
	password, err := reader.ReadSecret(label + ": ")
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	defer wipe(password)
	confirmation, err := reader.ReadSecret("Confirm " + strings.ToLower(label) + ": ")
	if err != nil {
		return "", fmt.Errorf("read password confirmation: %w", err)
	}
	defer wipe(confirmation)
	if len(password) < minimumPasswordBytes {
		return "", fmt.Errorf("password must contain at least %d bytes", minimumPasswordBytes)
	}
	if bytes.Equal(password, []byte("nixos")) {
		return "", errors.New("password must not use the public default")
	}
	if subtle.ConstantTimeCompare(password, confirmation) != 1 {
		return "", errors.New("password confirmation does not match")
	}
	hash, err := hasher.HashPassword(ctx, password)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	if !domain.IsPasswordHash(hash) {
		return "", errors.New("password hasher returned an invalid SHA-512 crypt hash")
	}
	return hash, nil
}

func wipe(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
