package main

import (
	"errors"
	"fmt"
	"strings"
)

// parseGitFlags reads the git commit/discard flags.
func parseGitFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--paths":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--paths requires comma-separated repository paths")
		}
		result.paths = arguments[index]
	default:
		return index, false, nil
	}
	return index, true, nil
}

// parseGitWord reads the words that only mean something after git.
func parseGitWord(result *options, word string) (bool, error) {
	switch word {
	case "review":
		if result.command != "git" || result.subcommand != "" {
			return true, errors.New("review must follow git")
		}
		result.subcommand = "review"
		return true, nil
	case "commit", "discard":
		if result.command != "git" || result.subcommand != "" {
			return true, fmt.Errorf("%s must follow git", word)
		}
		result.subcommand = word
		return true, nil
	case "plan":
		if result.command == "git" && result.subcommand == "discard" {
			result.subcommand = "discard-plan"
			return true, nil
		}
		if result.command == "git" && result.subcommand == "commit" {
			result.subcommand = "commit-plan"
			return true, nil
		}
	case "apply":
		if result.command == "git" && result.subcommand == "discard" {
			result.subcommand = "discard-apply"
			return true, nil
		}
		if result.command == "git" && result.subcommand == "commit" {
			result.subcommand = "commit-apply"
			return true, nil
		}
	}
	return false, nil
}

// validateGitPaths rejects option combinations that do not belong to the command.
func validateGitPaths(result *options) error {
	gitPaths := result.subcommand == "commit-plan" || result.subcommand == "commit-apply" || result.subcommand == "discard-plan" || result.subcommand == "discard-apply"
	if result.paths != "" && (result.command != "git" || !gitPaths) {
		return errors.New("--paths is only valid with git commit or git discard plan/apply")
	}
	if result.command == "git" && result.subcommand != "review" && !gitPaths {
		return errors.New("git requires review, commit plan/apply, or discard plan/apply")
	}
	if result.command == "git" && gitPaths && result.paths == "" {
		return fmt.Errorf("git %s requires --paths", strings.ReplaceAll(result.subcommand, "-", " "))
	}
	return nil
}

// validateGitApplyExpect rejects option combinations that do not belong to the command.
func validateGitApplyExpect(result *options) error {
	if result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply") && result.expect == "" {
		return errors.New("git commit apply requires --expect from git commit plan")
	}
	return nil
}
