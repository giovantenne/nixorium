package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func parseGitBackupArguments(args []string) (options, error) {
	o := options{command: "backup", subcommand: args[0], backupBranch: "main"}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--yes":
			o.yes = true
		case "--json":
			o.json = true
		case "--help", "-h":
			o.help = true
		case "--remote", "--branch", "--repo", "--to", "--expect", "--passphrase-file":
			flag := args[i]
			i++
			if i >= len(args) || args[i] == "" {
				return o, fmt.Errorf("%s requires a value", flag)
			}
			switch flag {
			case "--remote":
				o.backupRemote = args[i]
			case "--branch":
				o.backupBranch = args[i]
			case "--repo":
				o.repository = args[i]
			case "--to":
				o.backupTarget = args[i]
			case "--expect":
				o.expect = args[i]
			case "--passphrase-file":
				o.passphraseFile = args[i]
			}
		default:
			return o, fmt.Errorf("unknown backup option %q", args[i])
		}
	}
	if o.help {
		return o, nil
	}
	if o.subcommand == "clone" && len(args) == 1 {
		return o, nil
	}
	if o.backupRemote == "" {
		return o, errors.New("backup requires --remote <private-repository-ssh-url>")
	}
	switch o.subcommand {
	case "plan":
		if o.yes || o.expect != "" || o.backupTarget != "" || o.passphraseFile != "" {
			return o, errors.New("backup plan accepts only --remote, --branch, --repo and --json")
		}
	case "publish":
		if o.expect == "" || !o.yes || o.backupTarget != "" {
			return o, errors.New("backup publish requires --expect <token> --yes after reviewing backup plan and confirming the remote repository is private")
		}
	case "clone":
		if !o.yes || o.backupTarget == "" || o.repository != "" || o.expect != "" {
			return o, errors.New("backup clone requires --to <new-directory> --yes; omit all options for the Restore lab screen")
		}
	}
	return o, nil
}

func runGitBackupCommand(ctx context.Context, repository string, o options, stdout, stderr io.Writer) int {
	g := adapters.GitBackup{}
	if o.subcommand == "clone" && o.backupRemote == "" {
		err := presentation.RunRestoreLab(presentation.DashboardActions{RestoreLab: func(remote, branch, target string, passphrase []byte) domain.BackupReport {
			return g.Restore(ctx, remote, branch, target, passphrase)
		}, RestoreLabArchive: func(source, target string, passphrase []byte) domain.BackupReport {
			return adapters.Local{}.RestoreLab(ctx, source, passphrase, target)
		}})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	var plan domain.GitBackupPlan
	if o.subcommand != "clone" {
		var err error
		plan, err = g.Plan(ctx, repository, o.backupRemote, o.backupBranch)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		if o.subcommand == "plan" {
			if o.json {
				_ = presentation.JSON(stdout, plan)
			} else {
				fmt.Fprintf(stdout, "Private repository: %s\nBranch: %s\nSaved revision: %s\nTracked files: %d (ignored files excluded)\nRecovery: encrypted private keys, account hashes and trusted computer keys\nReview token: %s\n", plan.Remote, plan.Branch, plan.Revision, plan.Files, plan.ReviewToken)
				fmt.Fprintln(stdout, "Confirm the repository is PRIVATE in GitHub/GitLab before publishing. Configuration and Git history stay readable; private keys and account password hashes are encrypted. Git credentials and the passphrase must be kept separately.")
			}
			return 0
		}
		if o.expect != plan.ReviewToken {
			fmt.Fprintln(stderr, "Backup review changed; run backup plan again.")
			return 1
		}
	}
	secretOptions := o
	if o.subcommand == "publish" {
		secretOptions.subcommand = "create"
	}
	passphrase, err := backupPassphrase(secretOptions, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	defer clear(passphrase)
	var report domain.BackupReport
	if o.subcommand == "publish" {
		report = g.Publish(ctx, plan, passphrase)
	} else {
		target, err := filepath.Abs(o.backupTarget)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		report = g.Restore(ctx, o.backupRemote, o.backupBranch, target, passphrase)
	}
	if o.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.BackupText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}
