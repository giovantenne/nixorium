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

// parseBackupFlags reads the backup-only flags.
func parseBackupFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--to", "--passphrase-file":
		flag := arguments[index]
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, fmt.Errorf("%s requires a path", flag)
		}
		if flag == "--to" {
			result.backupTarget = arguments[index]
		} else {
			result.passphraseFile = arguments[index]
		}
	default:
		return index, false, nil
	}
	return index, true, nil
}

// parseBackupWord reads backup create/verify/restore (and install usb verify, which shares the word).
func parseBackupWord(result *options, word string) (bool, error) {
	switch word {
	case "create", "verify", "restore":
		if result.command == "backup" && result.subcommand == "" {
			result.subcommand = word
			return true, nil
		}
		if word != "verify" {
			return true, fmt.Errorf("%s must follow backup", word)
		}
		if result.command != "install" || result.subcommand != "usb" {
			return true, errors.New("verify must follow install usb or backup")
		}
		result.subcommand = "usb-verify"
		return true, nil
	}
	return false, nil
}

// parseBackupPositional takes the backup file after backup verify or restore.
func parseBackupPositional(result *options, word string) bool {
	if result.command == "backup" && (result.subcommand == "verify" || result.subcommand == "restore") && result.backupFile == "" {
		result.backupFile = word
		return true
	}
	return false
}

// validateBackup rejects option combinations that do not belong to the command.
func validateBackup(result *options) error {
	if result.command == "backup" {
		switch {
		case result.subcommand != "create" && result.subcommand != "verify" && result.subcommand != "restore":
			return errors.New("backup requires create, verify or restore")
		case result.subcommand == "create" && result.backupTarget == "":
			return errors.New("backup create requires --to <directory>")
		case result.subcommand == "create" && result.backupFile != "":
			return errors.New("backup create does not take a file")
		case result.subcommand != "create" && result.backupFile == "":
			return fmt.Errorf("backup %s requires the backup file", result.subcommand)
		case result.subcommand == "verify" && result.backupTarget != "":
			return errors.New("--to is only valid with backup create or restore")
		case result.subcommand == "restore" && result.backupTarget == "":
			return errors.New("backup restore requires --to <empty directory>")
		}
	} else if result.backupTarget != "" || result.passphraseFile != "" {
		return errors.New("--to and --passphrase-file are only valid with backup")
	}
	return nil
}
