package adapters

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

var privateDeploymentPaths = []string{"secret-key", "admin-ssh", "veyon-private-key.pem"}

// deploymentFlakeReference deliberately uses the Git fetcher rather than the
// path fetcher. A private deployment contains ignored secret key files; the
// Git fetcher limits the copied Nix source to version-controlled content and
// therefore keeps those files out of the Nix store.
func deploymentFlakeReference(repository string) (string, error) {
	absolute, err := filepath.Abs(repository)
	if err != nil {
		return "", fmt.Errorf("resolve deployment path: %w", err)
	}
	reference := url.URL{Scheme: "git+file", Path: filepath.ToSlash(absolute)}
	return reference.String(), nil
}

func ensurePrivateFilesUntracked(ctx context.Context, repository string) error {
	arguments := []string{"-C", repository, "ls-files", "--"}
	arguments = append(arguments, privateDeploymentPaths...)
	output, err := runOutput(ctx, "git", arguments...)
	if err != nil {
		return fmt.Errorf("inspect private-key tracking state: %w", err)
	}
	if tracked := strings.Fields(output); len(tracked) > 0 {
		return fmt.Errorf("refusing Nix evaluation because private key file is tracked by Git: %s", strings.Join(tracked, ", "))
	}
	return nil
}
