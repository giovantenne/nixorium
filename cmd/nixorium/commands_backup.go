package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func runBackupCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	passphrase, err := backupPassphrase(options, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	defer func() {
		for index := range passphrase {
			passphrase[index] = 0
		}
	}()
	local := adapters.Local{}
	var report domain.BackupReport
	switch options.subcommand {
	case "create":
		target, err := filepath.Abs(options.backupTarget)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		report = local.CreateBackup(ctx, repository, target, passphrase, nixoriumVersion)
	case "verify":
		report = local.VerifyBackup(options.backupFile, passphrase)
	default:
		target, err := filepath.Abs(options.backupTarget)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		report = local.RestoreBackup(options.backupFile, passphrase, target)
	}
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.BackupText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

// backupPassphrase reads the passphrase from a private file or the terminal.
// A new backup asks twice.
func backupPassphrase(options options, stderr io.Writer) ([]byte, error) {
	if options.passphraseFile != "" {
		info, err := os.Stat(options.passphraseFile)
		if err != nil {
			return nil, err
		}
		if info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("the passphrase file must not be readable by other users")
		}
		content, err := os.ReadFile(options.passphraseFile)
		if err != nil {
			return nil, err
		}
		return []byte(strings.TrimRight(string(content), "\r\n")), nil
	}
	reader := presentation.TerminalSecretReader{Input: os.Stdin, Output: stderr}
	passphrase, err := reader.ReadSecret("Backup passphrase: ")
	if err != nil {
		return nil, err
	}
	if options.subcommand == "create" {
		if len([]rune(string(passphrase))) < domain.BackupMinimumPassphrase {
			return nil, fmt.Errorf("use a passphrase of at least %d characters", domain.BackupMinimumPassphrase)
		}
		repeat, err := reader.ReadSecret("Repeat the passphrase: ")
		if err != nil {
			return nil, err
		}
		if string(repeat) != string(passphrase) {
			return nil, errors.New("the two passphrases differ")
		}
	}
	return passphrase, nil
}

// defaultBackupDestination suggests the first mounted removable drive of the
// administrator, so the backup does not stay on the controller.
func defaultBackupDestination() string {
	if user := os.Getenv("USER"); user != "" {
		if entries, err := os.ReadDir(filepath.Join("/run/media", user)); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					return filepath.Join("/run/media", user, entry.Name())
				}
			}
		}
	}
	return ""
}
