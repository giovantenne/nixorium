package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/giovantenne/nixorium/internal/domain"
)

const maximumKeyMaterialBytes = 64 * 1024

type keyMaterialSpec struct {
	name        string
	privatePath string
	publicPath  string
}

var keyMaterialSpecs = []keyMaterialSpec{
	{name: "cache", privatePath: "secret-key", publicPath: filepath.Join("keys", "cache-public-key")},
	{name: "ssh", privatePath: "admin-ssh", publicPath: filepath.Join("keys", "admin-ssh.pub")},
	{name: "veyon", privatePath: "veyon-private-key.pem", publicPath: filepath.Join("keys", "veyon-public-key.pem")},
}

func (Local) ImportKeyMaterial(ctx context.Context, repository, name, sourcePath string) (domain.KeyImportEvidence, error) {
	spec, found := keyMaterialSpecByName(name)
	if !found {
		return domain.KeyImportEvidence{}, fmt.Errorf("unsupported key type %q", name)
	}
	if err := requireRealDirectory(repository, "deployment root"); err != nil {
		return domain.KeyImportEvidence{}, err
	}
	absoluteSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return domain.KeyImportEvidence{}, fmt.Errorf("resolve source path: %w", err)
	}
	for _, character := range absoluteSource {
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
			return domain.KeyImportEvidence{}, errors.New("source path contains terminal control characters")
		}
	}
	privateContent, sourceMode, err := readRegularFileNoFollowLimit(absoluteSource, maximumKeyMaterialBytes)
	if err != nil {
		return domain.KeyImportEvidence{}, fmt.Errorf("read source private key: %w", err)
	}
	if len(bytes.TrimSpace(privateContent)) == 0 {
		return domain.KeyImportEvidence{}, errors.New("source private key is empty")
	}
	if sourceMode&0077 != 0 {
		return domain.KeyImportEvidence{}, fmt.Errorf("source private key mode %04o permits group or other access; restrict it to 0600 before importing", sourceMode)
	}
	publicContent, err := derivePublicKey(ctx, spec.name, privateContent)
	if err != nil {
		if spec.name == "ssh" {
			return domain.KeyImportEvidence{}, fmt.Errorf("validate SSH private key: unencrypted file-based keys are required: %w", err)
		}
		return domain.KeyImportEvidence{}, fmt.Errorf("validate %s private key: %w", spec.name, err)
	}

	keysDirectory := filepath.Join(repository, "keys")
	if err := ensureRealDirectory(keysDirectory, 0755, "public key directory"); err != nil {
		return domain.KeyImportEvidence{}, err
	}
	privatePath := filepath.Join(repository, spec.privatePath)
	publicPath := filepath.Join(repository, spec.publicPath)
	for label, path := range map[string]string{"private": privatePath, "public": publicPath} {
		if _, statErr := os.Lstat(path); statErr == nil {
			return domain.KeyImportEvidence{}, fmt.Errorf("refuse to replace existing %s %s key", name, label)
		} else if !os.IsNotExist(statErr) {
			return domain.KeyImportEvidence{}, fmt.Errorf("inspect destination %s key: %w", label, statErr)
		}
	}
	if err := writeRegularFileCreateNew(privatePath, privateContent, 0600); err != nil {
		return domain.KeyImportEvidence{}, fmt.Errorf("store imported %s private key: %w", name, err)
	}
	if err := writeRegularFileCreateNew(publicPath, publicContent, 0644); err != nil {
		_ = os.Remove(privatePath)
		return domain.KeyImportEvidence{}, fmt.Errorf("store derived %s public key: %w", name, err)
	}
	if err := syncDirectory(keysDirectory); err != nil {
		return domain.KeyImportEvidence{}, fmt.Errorf("sync public key directory: %w", err)
	}
	if err := syncDirectory(repository); err != nil {
		return domain.KeyImportEvidence{}, fmt.Errorf("sync deployment root: %w", err)
	}
	digest := sha256.Sum256(bytes.TrimSpace(publicContent))
	return domain.KeyImportEvidence{
		Name:        name,
		Source:      absoluteSource,
		Fingerprint: "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]),
	}, nil
}

func keyMaterialSpecByName(name string) (keyMaterialSpec, bool) {
	for _, spec := range keyMaterialSpecs {
		if spec.name == name {
			return spec, true
		}
	}
	return keyMaterialSpec{}, false
}

func (Local) KeyMaterial(ctx context.Context, repository string) []domain.KeyMaterialState {
	states := make([]domain.KeyMaterialState, 0, len(keyMaterialSpecs))
	for _, spec := range keyMaterialSpecs {
		state := domain.KeyMaterialState{Name: spec.name}
		privateContent, mode, privateErr := readRegularFileNoFollowLimit(filepath.Join(repository, spec.privatePath), maximumKeyMaterialBytes)
		publicContent, _, publicErr := readRegularFileNoFollowLimit(filepath.Join(repository, spec.publicPath), maximumKeyMaterialBytes)

		if privateErr == nil {
			state.PrivatePresent = len(privateContent) > 0
			state.PrivateMode = mode
			state.Safe = mode&0077 == 0
		} else if !os.IsNotExist(privateErr) {
			state.Problem = fmt.Sprintf("private key is unsafe or unreadable: %v", privateErr)
		}
		if publicErr == nil {
			state.PublicPresent = len(publicContent) > 0
		} else if !os.IsNotExist(publicErr) && state.Problem == "" {
			state.Problem = fmt.Sprintf("public key is unsafe or unreadable: %v", publicErr)
		}

		switch {
		case state.Problem != "":
		case !state.PrivatePresent && !state.PublicPresent:
			state.Problem = "private and public keys are missing"
		case !state.PrivatePresent:
			state.Problem = "private key is missing"
		case !state.PublicPresent:
			state.Problem = "public key is missing"
		case !state.Safe:
			state.Problem = fmt.Sprintf("private key mode %04o permits group or other access", mode)
		default:
			derived, err := derivePublicKey(ctx, spec.name, privateContent)
			if err != nil {
				state.Problem = fmt.Sprintf("cannot verify key correspondence: %v", err)
				break
			}
			state.Verified = true
			state.Matches = publicKeysMatch(spec.name, derived, publicContent)
			if !state.Matches {
				state.Problem = "public key does not match private key"
			}
		}
		states = append(states, state)
	}
	return states
}

// ReconcileKeyMaterial creates only missing pairs and never replaces existing
// key material. A private key created before an interrupted public-key write is
// safely reusable on the next invocation.
func (Local) ReconcileKeyMaterial(ctx context.Context, repository string) error {
	if err := requireRealDirectory(repository, "deployment root"); err != nil {
		return err
	}
	keysDirectory := filepath.Join(repository, "keys")
	if err := ensureRealDirectory(keysDirectory, 0755, "public key directory"); err != nil {
		return err
	}

	for _, spec := range keyMaterialSpecs {
		privatePath := filepath.Join(repository, spec.privatePath)
		publicPath := filepath.Join(repository, spec.publicPath)
		privateContent, privateMode, privateErr := readRegularFileNoFollowLimit(privatePath, maximumKeyMaterialBytes)
		publicContent, _, publicErr := readRegularFileNoFollowLimit(publicPath, maximumKeyMaterialBytes)

		if privateErr != nil && !os.IsNotExist(privateErr) {
			return fmt.Errorf("inspect %s private key: %w", spec.name, privateErr)
		}
		if publicErr != nil && !os.IsNotExist(publicErr) {
			return fmt.Errorf("inspect %s public key: %w", spec.name, publicErr)
		}
		if os.IsNotExist(privateErr) && publicErr == nil {
			return fmt.Errorf("refuse to generate %s private key while a public key already exists", spec.name)
		}

		if os.IsNotExist(privateErr) {
			generated, err := generatePrivateKey(ctx, spec.name)
			if err != nil {
				return fmt.Errorf("generate %s private key: %w", spec.name, err)
			}
			if len(bytes.TrimSpace(generated)) == 0 {
				return fmt.Errorf("generate %s private key: command produced empty output", spec.name)
			}
			if err := writeRegularFileCreateNew(privatePath, generated, 0600); err != nil {
				return fmt.Errorf("store %s private key: %w", spec.name, err)
			}
			privateContent = generated
			privateMode = 0600
		}
		if len(privateContent) == 0 {
			return fmt.Errorf("%s private key is empty", spec.name)
		}
		if privateMode&0077 != 0 {
			if err := restrictRegularFileMode(privatePath, 0600); err != nil {
				return fmt.Errorf("secure %s private key: %w", spec.name, err)
			}
		}

		derived, err := derivePublicKey(ctx, spec.name, privateContent)
		if err != nil {
			return fmt.Errorf("derive %s public key: %w", spec.name, err)
		}
		if os.IsNotExist(publicErr) {
			if err := writeRegularFileCreateNew(publicPath, derived, 0644); err != nil {
				return fmt.Errorf("store %s public key: %w", spec.name, err)
			}
			continue
		}
		if len(publicContent) == 0 || !publicKeysMatch(spec.name, derived, publicContent) {
			return fmt.Errorf("refuse to replace mismatched %s public key", spec.name)
		}
	}
	if err := syncDirectory(keysDirectory); err != nil {
		return fmt.Errorf("sync public key directory: %w", err)
	}
	return syncDirectory(repository)
}

func generatePrivateKey(ctx context.Context, name string) ([]byte, error) {
	switch name {
	case "cache":
		return runKeyCommand(ctx, nil, "nix", "--extra-experimental-features", "nix-command flakes", "key", "generate-secret", "--key-name", "nixorium-cache")
	case "ssh":
		directory, err := os.MkdirTemp("", "nixorium-ssh-key-")
		if err != nil {
			return nil, err
		}
		privatePath := filepath.Join(directory, "key")
		defer func() {
			_ = os.Remove(privatePath + ".pub")
			_ = os.Remove(privatePath)
			_ = os.Remove(directory)
		}()
		if _, err := runKeyCommand(ctx, nil, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "admin@controller", "-f", privatePath); err != nil {
			return nil, err
		}
		content, _, err := readRegularFileNoFollowLimit(privatePath, maximumKeyMaterialBytes)
		return content, err
	case "veyon":
		return runKeyCommand(ctx, nil, "openssl", "genrsa", "4096")
	default:
		return nil, fmt.Errorf("unsupported key type %q", name)
	}
}

func derivePublicKey(ctx context.Context, name string, privateContent []byte) ([]byte, error) {
	var (
		content []byte
		err     error
	)
	switch name {
	case "cache":
		content, err = runKeyCommand(ctx, privateContent, "nix", "--extra-experimental-features", "nix-command flakes", "key", "convert-secret-to-public")
	case "ssh":
		directory, createErr := os.MkdirTemp("", "nixorium-ssh-verify-")
		if createErr != nil {
			return nil, createErr
		}
		privatePath := filepath.Join(directory, "key")
		defer func() {
			_ = os.Remove(privatePath)
			_ = os.Remove(directory)
		}()
		if createErr = writeRegularFileCreateNew(privatePath, privateContent, 0600); createErr != nil {
			return nil, createErr
		}
		content, err = runKeyCommand(ctx, nil, "ssh-keygen", "-y", "-P", "", "-f", privatePath)
	case "veyon":
		content, err = runKeyCommand(ctx, privateContent, "openssl", "pkey", "-pubout")
	default:
		return nil, fmt.Errorf("unsupported key type %q", name)
	}
	if err != nil {
		return nil, err
	}
	content = bytes.TrimSpace(content)
	if len(content) == 0 {
		return nil, errors.New("derived public key is empty")
	}
	return append(content, '\n'), nil
}

func publicKeysMatch(name string, expected, actual []byte) bool {
	switch name {
	case "ssh":
		expectedFields := strings.Fields(string(expected))
		actualFields := strings.Fields(string(actual))
		return len(expectedFields) >= 2 && len(actualFields) >= 2 && expectedFields[0] == actualFields[0] && expectedFields[1] == actualFields[1]
	case "veyon":
		return strings.Join(strings.Fields(string(expected)), "") == strings.Join(strings.Fields(string(actual)), "")
	default:
		return strings.TrimSpace(string(expected)) == strings.TrimSpace(string(actual))
	}
}

func runKeyCommand(ctx context.Context, input []byte, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", name, err)
	}
	if len(output) > maximumKeyMaterialBytes {
		return nil, fmt.Errorf("%s produced unexpectedly large output", name)
	}
	return output, nil
}

func requireRealDirectory(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s must be a directory and not a symlink", label)
	}
	return nil
}

func ensureRealDirectory(path string, mode os.FileMode, label string) error {
	if err := os.Mkdir(path, mode); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create %s: %w", label, err)
	}
	return requireRealDirectory(path, label)
}

func writeRegularFileCreateNew(path string, content []byte, mode uint32) error {
	fileDescriptor, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fileDescriptor), path)
	if _, err := file.Write(content); err != nil {
		file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func restrictRegularFileMode(path string, mode uint32) error {
	fileDescriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fileDescriptor)
	var stat syscall.Stat_t
	if err := syscall.Fstat(fileDescriptor, &stat); err != nil {
		return err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		return errors.New("not a regular file")
	}
	return syscall.Fchmod(fileDescriptor, mode)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
